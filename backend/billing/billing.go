package billing

import (
	"errors"
	"math"
	"sort"
	"time"

	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

const (
	PaymentMethodCash     = model.BillPaymentMethodCash
	PaymentMethodTransfer = model.BillPaymentMethodTransfer
	PaymentMethodCredit   = model.BillPaymentMethodCredit

	SegmentSourceStandard     = "standard"
	SegmentSourcePromotion    = "promotion"
	SegmentSourceMinimumTopup = "minimum_topup"
	SegmentSourceRoundUp      = "round_up"
)

var ErrInvalidPaymentMethod = errors.New("invalid payment method")

type TablePrice struct {
	TableID       uint
	NormalPrice   float64
	PracticePrice float64
}

type ActivePromotionPrice struct {
	PromotionID   uint
	Name          string
	StartAt       time.Time
	EndAt         time.Time
	Priority      int
	NormalPrice   float64
	PracticePrice float64
}

type PriceSegment struct {
	PromotionID     *uint     `json:"promotion_id"`
	PromotionName   string    `json:"promotion_name"`
	SegmentStart    time.Time `json:"segment_start"`
	SegmentEnd      time.Time `json:"segment_end"`
	DurationSeconds int64     `json:"duration_seconds"`
	TableType       uint8     `json:"table_type"`
	UnitPrice       float64   `json:"unit_price"`
	Source          string    `json:"source"`
	Amount          float64   `json:"amount"`
}

type PausePeriod struct {
	PauseStart      time.Time  `json:"pause_start"`
	PauseEnd        *time.Time `json:"pause_end"`
	DurationSeconds int64      `json:"duration_seconds"`
	IsOpen          bool       `json:"is_open"`
}

func Migrate(database *gorm.DB) error {
	if err := migrateBillingTables(database); err != nil {
		return err
	}

	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_promotions_active_range ON promotions(is_active, start_at, end_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_promotion_table_prices_active
			ON promotion_table_prices(promotion_id, table_id)
			WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_bill_price_segments_visitation ON bill_price_segments(visitation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_bill_payments_visitation ON bill_payments(visitation_id, is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_visitation_pause_periods_visitation ON visitation_pause_periods(visitation_id, pause_start)`,
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			return err
		}
	}
	if err := EnsureBillingMenus(database); err != nil {
		return err
	}
	return BackfillLegacyCashPayments(database)
}

func migrateBillingTables(database *gorm.DB) error {
	tables := []interface{}{
		&model.Promotion{},
		&model.PromotionTablePrice{},
		&model.BillPriceSegment{},
		&model.BillPayment{},
		&model.VisitationPausePeriod{},
	}
	for _, table := range tables {
		if database.Migrator().HasTable(table) {
			continue
		}
		if err := database.AutoMigrate(table); err != nil {
			return err
		}
	}
	return nil
}

func StartPausePeriod(tx *gorm.DB, visitationID uint, pauseStart time.Time) error {
	var openCount int64
	if err := tx.Model(&model.VisitationPausePeriod{}).
		Where("visitation_id = ? AND pause_end IS NULL AND deleted_at IS NULL", visitationID).
		Count(&openCount).Error; err != nil {
		return err
	}
	if openCount > 0 {
		return nil
	}
	return tx.Create(&model.VisitationPausePeriod{
		VisitationID: visitationID,
		PauseStart:   pauseStart,
	}).Error
}

func FinishPausePeriod(tx *gorm.DB, visitationID uint, pauseEnd time.Time) error {
	var period model.VisitationPausePeriod
	err := tx.Where("visitation_id = ? AND pause_end IS NULL AND deleted_at IS NULL", visitationID).
		Order("pause_start DESC, id DESC").
		First(&period).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	durationSeconds := int64(pauseEnd.Sub(period.PauseStart).Seconds())
	if durationSeconds < 0 {
		durationSeconds = 0
	}
	return tx.Model(&period).Updates(map[string]interface{}{
		"pause_end":        pauseEnd,
		"duration_seconds": durationSeconds,
	}).Error
}

func LoadPausePeriods(tx *gorm.DB, visitationID uint, now time.Time) ([]PausePeriod, int64, error) {
	var rows []model.VisitationPausePeriod
	if err := tx.Where("visitation_id = ? AND deleted_at IS NULL", visitationID).
		Order("pause_start ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	periods := make([]PausePeriod, 0, len(rows))
	totalSeconds := int64(0)
	for _, row := range rows {
		durationSeconds := row.DurationSeconds
		isOpen := row.PauseEnd == nil
		if isOpen {
			durationSeconds = int64(now.Sub(row.PauseStart).Seconds())
		} else if durationSeconds <= 0 && row.PauseEnd.After(row.PauseStart) {
			durationSeconds = int64(row.PauseEnd.Sub(row.PauseStart).Seconds())
		}
		if durationSeconds < 0 {
			durationSeconds = 0
		}
		totalSeconds += durationSeconds
		periods = append(periods, PausePeriod{
			PauseStart:      row.PauseStart,
			PauseEnd:        row.PauseEnd,
			DurationSeconds: durationSeconds,
			IsOpen:          isOpen,
		})
	}
	return periods, totalSeconds, nil
}

type menuPermissionSeed struct {
	MenuName    string
	Route       string
	Level       uint8
	Relation    uint
	Order       uint8
	Icon        string
	Permissions []permissionSeed
}

type permissionSeed struct {
	Name  string
	Title string
}

func EnsureBillingMenus(database *gorm.DB) error {
	if err := database.AutoMigrate(
		&model.Menu{},
		&model.Permission{},
		&model.Role{},
		&model.RoleHasPermission{},
	); err != nil {
		return err
	}

	seeds := []menuPermissionSeed{
		{
			MenuName: "โปรโมชั่น",
			Route:    "/promotions",
			Level:    1,
			Relation: 2,
			Order:    8,
			Icon:     "AppsIcon",
			Permissions: []permissionSeed{
				{Name: "promotions-access", Title: "เข้าถึง"},
				{Name: "promotions-create-access", Title: "เพิ่มข้อมูล"},
				{Name: "promotions-edit-access", Title: "แก้ไขข้อมูล"},
				{Name: "promotions-delete-access", Title: "ลบข้อมูล"},
			},
		},
		{
			MenuName: "รายงานการชำระเงิน",
			Route:    "/payment-reports",
			Level:    1,
			Relation: 10,
			Order:    7,
			Icon:     "AppsIcon",
			Permissions: []permissionSeed{
				{Name: "payment-reports-access", Title: "เข้าถึง"},
			},
		},
	}

	return database.Transaction(func(tx *gorm.DB) error {
		var roles []model.Role
		if err := tx.Where("is_active = 1").Find(&roles).Error; err != nil {
			return err
		}

		for _, seed := range seeds {
			menu := model.Menu{}
			if err := tx.Where("route = ?", seed.Route).First(&menu).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				menu = model.Menu{
					Name:     seed.MenuName,
					Route:    seed.Route,
					Level:    seed.Level,
					Relation: seed.Relation,
					Order:    seed.Order,
					Icon:     seed.Icon,
					IsActive: 1,
				}
				if err := tx.Create(&menu).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&menu).Updates(map[string]interface{}{
					"name":      seed.MenuName,
					"level":     seed.Level,
					"relation":  seed.Relation,
					"order":     seed.Order,
					"icon":      seed.Icon,
					"is_active": 1,
				}).Error; err != nil {
					return err
				}
			}

			for _, permissionSeed := range seed.Permissions {
				permission := model.Permission{}
				if err := tx.Where("name = ?", permissionSeed.Name).First(&permission).Error; err != nil {
					if !errors.Is(err, gorm.ErrRecordNotFound) {
						return err
					}
					permission = model.Permission{
						Name:     permissionSeed.Name,
						Title:    permissionSeed.Title,
						MenuID:   menu.ID,
						IsActive: 1,
					}
					if err := tx.Create(&permission).Error; err != nil {
						return err
					}
				} else {
					if err := tx.Model(&permission).Updates(map[string]interface{}{
						"title":     permissionSeed.Title,
						"menu_id":   menu.ID,
						"is_active": 1,
					}).Error; err != nil {
						return err
					}
				}

				for _, role := range roles {
					var count int64
					if err := tx.Model(&model.RoleHasPermission{}).
						Where("role_id = ? AND permission_id = ?", role.ID, permission.ID).
						Count(&count).Error; err != nil {
						return err
					}
					if count == 0 {
						if err := tx.Create(&model.RoleHasPermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	})
}

func BackfillLegacyCashPayments(database *gorm.DB) error {
	return database.Exec(`
		INSERT INTO bill_payments (created_at, updated_at, visitation_id, method, amount, note, paid_at, is_active)
		SELECT
			CURRENT_TIMESTAMP,
			CURRENT_TIMESTAMP,
			v.id,
			?,
			v.net_price,
			'legacy paid bill backfill',
			CASE WHEN v.end_time IS NOT NULL AND v.end_time != '0001-01-01 00:00:00+00:00' THEN v.end_time ELSE v.updated_at END,
			1
		FROM visitations v
		WHERE v.is_paid = 1
			AND v.deleted_at IS NULL
			AND v.net_price > 0
			AND NOT EXISTS (
				SELECT 1
				FROM bill_payments bp
				WHERE bp.visitation_id = v.id
					AND bp.deleted_at IS NULL
					AND bp.is_active = 1
			)
	`, PaymentMethodCash).Error
}

func CalculateGameFee(timeInSeconds int64, normalPrice float64, practicePrice float64, isPractice bool) float64 {
	totalMinutes := int(math.Ceil(float64(maxInt64(timeInSeconds, 0)) / 60))
	if totalMinutes < 0 {
		totalMinutes = 0
	}

	ratePerHour := normalPrice
	if isPractice {
		ratePerHour = practicePrice
	}
	ratePerMinute := ratePerHour / 60

	var fee float64
	if isPractice && totalMinutes <= 60 {
		fee = ratePerHour
	} else if totalMinutes <= 30 {
		fee = ratePerMinute * 30
	} else if totalMinutes <= 60 {
		fee = ratePerHour
	} else {
		fee = ratePerHour + float64(totalMinutes-60)*ratePerMinute
	}

	return math.Ceil(fee)
}

func CalculateGameFeeSegments(startAt time.Time, timeInSeconds int64, tableType uint8, standard TablePrice, promotions []ActivePromotionPrice) ([]PriceSegment, float64) {
	if timeInSeconds < 0 {
		timeInSeconds = 0
	}
	chargeableSeconds := chargeableGameSeconds(timeInSeconds, tableType == 1)
	if chargeableSeconds == 0 {
		return nil, 0
	}

	actualEnd := startAt.Add(time.Duration(timeInSeconds) * time.Second)
	if !actualEnd.After(startAt) {
		actualEnd = startAt
	}

	baseSegments := splitByPromotion(startAt, actualEnd, tableType, standard, promotions)
	if len(baseSegments) == 0 {
		baseSegments = []PriceSegment{{
			SegmentStart: startAt,
			SegmentEnd:   startAt,
			TableType:    tableType,
			UnitPrice:    rateForType(tableType, standard.NormalPrice, standard.PracticePrice),
			Source:       SegmentSourceStandard,
		}}
	}

	chargedSeconds := int64(0)
	rawAmounts := make([]float64, 0, len(baseSegments)+1)
	segments := make([]PriceSegment, 0, len(baseSegments)+1)
	for _, segment := range baseSegments {
		duration := segment.SegmentEnd.Sub(segment.SegmentStart)
		seconds := int64(math.Ceil(duration.Seconds()))
		if seconds < 0 {
			seconds = 0
		}
		if seconds == 0 {
			continue
		}
		used := roundUpToMinuteSeconds(seconds)
		segment.DurationSeconds = used
		raw := float64(used) / 3600 * segment.UnitPrice
		rawAmounts = append(rawAmounts, raw)
		segments = append(segments, segment)
		chargedSeconds += used
	}

	remaining := chargeableSeconds - chargedSeconds
	if remaining > 0 {
		last := baseSegments[len(baseSegments)-1]
		if len(segments) > 0 {
			last = segments[len(segments)-1]
		}
		topup := PriceSegment{
			PromotionID:     last.PromotionID,
			PromotionName:   last.PromotionName,
			SegmentStart:    actualEnd,
			SegmentEnd:      actualEnd.Add(time.Duration(remaining) * time.Second),
			DurationSeconds: remaining,
			TableType:       tableType,
			UnitPrice:       last.UnitPrice,
			Source:          SegmentSourceMinimumTopup,
		}
		rawAmounts = append(rawAmounts, float64(remaining)/3600*topup.UnitPrice)
		segments = append(segments, topup)
	}

	totalRaw := 0.0
	for _, amount := range rawAmounts {
		totalRaw += amount
	}
	total := math.Ceil(totalRaw)
	roundedSum := 0.0
	for index := range segments {
		if index == len(segments)-1 {
			segments[index].Amount = roundMoney(total - roundedSum)
			break
		}
		segments[index].Amount = roundMoney(rawAmounts[index])
		roundedSum += segments[index].Amount
	}

	return segments, total
}

func LoadPromotionPrices(tx *gorm.DB, tableID uint, startAt time.Time, endAt time.Time) ([]ActivePromotionPrice, error) {
	var rows []ActivePromotionPrice
	err := tx.Table("promotion_table_prices ptp").
		Select(`p.id AS promotion_id, p.name, p.start_at, p.end_at, p.priority,
			ptp.normal_price, ptp.practice_price`).
		Joins("JOIN promotions p ON p.id = ptp.promotion_id").
		Where("ptp.table_id = ? AND ptp.is_active = 1 AND ptp.deleted_at IS NULL", tableID).
		Where("p.is_active = 1 AND p.deleted_at IS NULL").
		Order("p.priority DESC, p.start_at DESC, p.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	activeRows := make([]ActivePromotionPrice, 0, len(rows))
	for _, row := range rows {
		if row.StartAt.Before(endAt) && row.EndAt.After(startAt) {
			activeRows = append(activeRows, row)
		}
	}
	return activeRows, nil
}

func ReplaceBillPriceSegments(tx *gorm.DB, visitationID uint, tableID uint, segments []PriceSegment) error {
	if err := tx.Where("visitation_id = ?", visitationID).Delete(&model.BillPriceSegment{}).Error; err != nil {
		return err
	}
	for _, segment := range segments {
		record := model.BillPriceSegment{
			VisitationID:    visitationID,
			TableID:         tableID,
			PromotionID:     segment.PromotionID,
			SegmentStart:    segment.SegmentStart,
			SegmentEnd:      segment.SegmentEnd,
			DurationSeconds: segment.DurationSeconds,
			TableType:       segment.TableType,
			UnitPrice:       segment.UnitPrice,
			Source:          segment.Source,
			Amount:          segment.Amount,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
}

func ReplaceBillPayments(tx *gorm.DB, visitationID uint, payments []model.BillPayment) error {
	if err := tx.Where("visitation_id = ?", visitationID).Delete(&model.BillPayment{}).Error; err != nil {
		return err
	}
	for _, payment := range payments {
		if err := ValidatePaymentMethod(payment.Method); err != nil {
			return err
		}
		if payment.Amount < 0 || math.IsNaN(payment.Amount) || math.IsInf(payment.Amount, 0) {
			return errors.New("invalid payment amount")
		}
		payment.VisitationID = visitationID
		payment.Amount = roundMoney(payment.Amount)
		if payment.PaidAt.IsZero() {
			payment.PaidAt = time.Now()
		}
		if payment.IsActive == 0 {
			payment.IsActive = 1
		}
		if err := tx.Create(&payment).Error; err != nil {
			return err
		}
	}
	return nil
}

func ValidatePaymentMethod(method string) error {
	switch method {
	case PaymentMethodCash, PaymentMethodTransfer, PaymentMethodCredit:
		return nil
	default:
		return ErrInvalidPaymentMethod
	}
}

func SumPayments(payments []model.BillPayment) float64 {
	total := 0.0
	for _, payment := range payments {
		total += payment.Amount
	}
	return roundMoney(total)
}

func splitByPromotion(startAt time.Time, endAt time.Time, tableType uint8, standard TablePrice, promotions []ActivePromotionPrice) []PriceSegment {
	if endAt.Before(startAt) {
		endAt = startAt
	}
	boundaries := []time.Time{startAt, endAt}
	for _, promotion := range promotions {
		if promotion.EndAt.After(startAt) && promotion.StartAt.Before(endAt) {
			if promotion.StartAt.After(startAt) {
				boundaries = append(boundaries, promotion.StartAt)
			}
			if promotion.EndAt.Before(endAt) {
				boundaries = append(boundaries, promotion.EndAt)
			}
		}
	}
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i].Before(boundaries[j]) })
	boundaries = uniqueTimes(boundaries)

	segments := make([]PriceSegment, 0, len(boundaries)-1)
	for index := 0; index < len(boundaries)-1; index++ {
		segmentStart := boundaries[index]
		segmentEnd := boundaries[index+1]
		if !segmentEnd.After(segmentStart) {
			continue
		}
		midpoint := segmentStart.Add(segmentEnd.Sub(segmentStart) / 2)
		promotion := activePromotionAt(midpoint, promotions)
		source := SegmentSourceStandard
		var promotionID *uint
		promotionName := ""
		unitPrice := rateForType(tableType, standard.NormalPrice, standard.PracticePrice)
		if promotion != nil {
			source = SegmentSourcePromotion
			promotionID = &promotion.PromotionID
			promotionName = promotion.Name
			unitPrice = rateForType(tableType, promotion.NormalPrice, promotion.PracticePrice)
		}
		segments = append(segments, PriceSegment{
			PromotionID:   promotionID,
			PromotionName: promotionName,
			SegmentStart:  segmentStart,
			SegmentEnd:    segmentEnd,
			TableType:     tableType,
			UnitPrice:     unitPrice,
			Source:        source,
		})
	}
	return segments
}

func activePromotionAt(moment time.Time, promotions []ActivePromotionPrice) *ActivePromotionPrice {
	var selected *ActivePromotionPrice
	for index := range promotions {
		promotion := &promotions[index]
		if (moment.Equal(promotion.StartAt) || moment.After(promotion.StartAt)) && moment.Before(promotion.EndAt) {
			if selected == nil ||
				promotion.Priority > selected.Priority ||
				(promotion.Priority == selected.Priority && promotion.StartAt.After(selected.StartAt)) ||
				(promotion.Priority == selected.Priority && promotion.StartAt.Equal(selected.StartAt) && promotion.PromotionID > selected.PromotionID) {
				selected = promotion
			}
		}
	}
	return selected
}

func chargeableGameSeconds(timeInSeconds int64, isPractice bool) int64 {
	totalMinutes := int64(math.Ceil(float64(maxInt64(timeInSeconds, 0)) / 60))
	if isPractice {
		return maxInt64(60, totalMinutes) * 60
	}
	if totalMinutes <= 30 {
		return 30 * 60
	}
	if totalMinutes <= 60 {
		return 60 * 60
	}
	return totalMinutes * 60
}

func roundUpToMinuteSeconds(seconds int64) int64 {
	if seconds <= 0 {
		return 0
	}
	return int64(math.Ceil(float64(seconds)/60)) * 60
}

func rateForType(tableType uint8, normalPrice float64, practicePrice float64) float64 {
	if tableType == 1 {
		return practicePrice
	}
	return normalPrice
}

func uniqueTimes(values []time.Time) []time.Time {
	if len(values) == 0 {
		return values
	}
	result := []time.Time{values[0]}
	for _, value := range values[1:] {
		if !value.Equal(result[len(result)-1]) {
			result = append(result, value)
		}
	}
	return result
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
