package billing

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	model "github.com/rainza999/fiber-test/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed
}

func assertMoney(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("money: got %.2f want %.2f", got, want)
	}
}

func TestCalculateGameFeeSegmentsSplitsWeekendPromotion(t *testing.T) {
	start := mustTime(t, "2026-07-03 22:00:00")
	promoStart := mustTime(t, "2026-07-04 00:00:00")
	promoEnd := mustTime(t, "2026-07-05 23:59:59")

	segments, total := CalculateGameFeeSegments(start, 4*60*60, 0, TablePrice{
		TableID:       1,
		NormalPrice:   150,
		PracticePrice: 100,
	}, []ActivePromotionPrice{{
		PromotionID:   7,
		Name:          "Weekend",
		StartAt:       promoStart,
		EndAt:         promoEnd,
		NormalPrice:   100,
		PracticePrice: 50,
	}})

	if len(segments) != 2 {
		t.Fatalf("segments: got %d want 2: %#v", len(segments), segments)
	}
	assertMoney(t, total, 500)
	assertMoney(t, segments[0].Amount, 300)
	assertMoney(t, segments[1].Amount, 200)
	if segments[0].Source != SegmentSourceStandard || segments[1].Source != SegmentSourcePromotion {
		t.Fatalf("unexpected sources: %#v", segments)
	}
}

func TestCalculateGameFeeSegmentsSplitsPracticePromotion(t *testing.T) {
	start := mustTime(t, "2026-07-03 22:00:00")
	promoStart := mustTime(t, "2026-07-04 00:00:00")
	promoEnd := mustTime(t, "2026-07-05 23:59:59")

	segments, total := CalculateGameFeeSegments(start, 4*60*60, 1, TablePrice{
		TableID:       1,
		NormalPrice:   150,
		PracticePrice: 100,
	}, []ActivePromotionPrice{{
		PromotionID:   7,
		Name:          "Weekend",
		StartAt:       promoStart,
		EndAt:         promoEnd,
		NormalPrice:   100,
		PracticePrice: 50,
	}})

	if len(segments) != 2 {
		t.Fatalf("segments: got %d want 2: %#v", len(segments), segments)
	}
	assertMoney(t, total, 300)
	assertMoney(t, segments[0].Amount, 200)
	assertMoney(t, segments[1].Amount, 100)
}

func TestCalculateGameFeeSegmentsUsesLastRateForRegularMinimumTopup(t *testing.T) {
	start := mustTime(t, "2026-07-03 23:50:00")
	promoStart := mustTime(t, "2026-07-04 00:00:00")
	promoEnd := mustTime(t, "2026-07-05 23:59:59")

	segments, total := CalculateGameFeeSegments(start, 20*60, 0, TablePrice{
		TableID:       1,
		NormalPrice:   150,
		PracticePrice: 100,
	}, []ActivePromotionPrice{{
		PromotionID:   7,
		Name:          "Weekend",
		StartAt:       promoStart,
		EndAt:         promoEnd,
		NormalPrice:   100,
		PracticePrice: 50,
	}})

	if len(segments) != 3 {
		t.Fatalf("segments: got %d want 3: %#v", len(segments), segments)
	}
	assertMoney(t, total, 59)
	if segments[2].Source != SegmentSourceMinimumTopup {
		t.Fatalf("last segment should be minimum topup: %#v", segments[2])
	}
}

func TestCalculateGameFeeSegmentsRoundsEachPromotionSegmentToMinute(t *testing.T) {
	start := mustTime(t, "2026-07-04 05:53:40")
	promoStart := mustTime(t, "2026-07-04 00:00:00")
	promoEnd := mustTime(t, "2026-07-04 07:00:00")

	segments, total := CalculateGameFeeSegments(start, int64((7*time.Hour+46*time.Minute+20*time.Second)/time.Second), 0, TablePrice{
		TableID:       1,
		NormalPrice:   160,
		PracticePrice: 100,
	}, []ActivePromotionPrice{{
		PromotionID:   7,
		Name:          "Rain Test",
		StartAt:       promoStart,
		EndAt:         promoEnd,
		NormalPrice:   100,
		PracticePrice: 50,
	}})

	if len(segments) != 2 {
		t.Fatalf("segments: got %d want 2: %#v", len(segments), segments)
	}
	if segments[0].Source != SegmentSourcePromotion || segments[0].DurationSeconds != int64((time.Hour+7*time.Minute)/time.Second) {
		t.Fatalf("promotion segment should round up to the next minute: %#v", segments[0])
	}
	if segments[1].Source != SegmentSourceStandard || segments[1].DurationSeconds != int64((6*time.Hour+40*time.Minute)/time.Second) {
		t.Fatalf("standard segment should use its own rounded duration: %#v", segments[1])
	}
	assertMoney(t, total, 1179)
}

func TestCalculateGameFeeSegmentsRoundsPartialSecondsPerPriceSegment(t *testing.T) {
	start := mustTime(t, "2026-07-04 05:53:40")
	promoStart := mustTime(t, "2026-07-04 00:00:00")
	promoEnd := mustTime(t, "2026-07-04 07:00:00")

	segments, total := CalculateGameFeeSegments(start, int64((7*time.Hour+46*time.Minute+40*time.Second)/time.Second), 0, TablePrice{
		TableID:       1,
		NormalPrice:   160,
		PracticePrice: 100,
	}, []ActivePromotionPrice{{
		PromotionID:   7,
		Name:          "Rain Test",
		StartAt:       promoStart,
		EndAt:         promoEnd,
		NormalPrice:   100,
		PracticePrice: 50,
	}})

	if len(segments) != 2 {
		t.Fatalf("segments: got %d want 2: %#v", len(segments), segments)
	}
	if segments[0].DurationSeconds != int64((time.Hour+7*time.Minute)/time.Second) {
		t.Fatalf("promotion segment should round its partial minute: %#v", segments[0])
	}
	if segments[1].DurationSeconds != int64((6*time.Hour+41*time.Minute)/time.Second) {
		t.Fatalf("standard segment should round its own partial minute: %#v", segments[1])
	}
	assertMoney(t, total, 1182)
}

func TestPausePeriodsTrackCompletedAndOpenDurations(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pause.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	defer sqlDB.Close()
	if err := database.AutoMigrate(&model.Menu{}, &model.Permission{}, &model.Role{}, &model.RoleHasPermission{}, &model.Visitation{}); err != nil {
		t.Fatalf("base migrate: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	visitationID := uint(7)
	firstPause := mustTime(t, "2026-07-04 10:00:00")
	if err := StartPausePeriod(database, visitationID, firstPause); err != nil {
		t.Fatalf("start first pause: %v", err)
	}
	if err := StartPausePeriod(database, visitationID, firstPause.Add(time.Minute)); err != nil {
		t.Fatalf("duplicate open pause should be ignored: %v", err)
	}
	if err := FinishPausePeriod(database, visitationID, firstPause.Add(5*time.Minute)); err != nil {
		t.Fatalf("finish first pause: %v", err)
	}
	secondPause := mustTime(t, "2026-07-04 10:10:00")
	if err := StartPausePeriod(database, visitationID, secondPause); err != nil {
		t.Fatalf("start second pause: %v", err)
	}

	periods, totalSeconds, err := LoadPausePeriods(database, visitationID, secondPause.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("load pause periods: %v", err)
	}
	if len(periods) != 2 {
		t.Fatalf("pause periods: got %d want 2: %#v", len(periods), periods)
	}
	if periods[0].IsOpen || periods[0].DurationSeconds != int64((5*time.Minute).Seconds()) {
		t.Fatalf("first pause should be completed for 5 minutes: %#v", periods[0])
	}
	if !periods[1].IsOpen || periods[1].DurationSeconds != int64((2*time.Minute).Seconds()) {
		t.Fatalf("second pause should be open for 2 minutes: %#v", periods[1])
	}
	if totalSeconds != int64((7 * time.Minute).Seconds()) {
		t.Fatalf("total pause seconds: got %d want %d", totalSeconds, int64((7 * time.Minute).Seconds()))
	}
}

func TestMigrateCanRunTwice(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "billing.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	defer sqlDB.Close()
	if err := database.AutoMigrate(&model.Menu{}, &model.Permission{}, &model.Role{}, &model.RoleHasPermission{}, &model.Visitation{}); err != nil {
		t.Fatalf("base migrate: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestLoadPromotionPricesFiltersByTimeInGo(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "promotion.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	defer sqlDB.Close()
	if err := database.AutoMigrate(&model.Menu{}, &model.Permission{}, &model.Role{}, &model.RoleHasPermission{}, &model.Visitation{}); err != nil {
		t.Fatalf("base migrate: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	start := time.Date(2026, 6, 28, 1, 0, 0, 0, time.FixedZone("ICT", 7*3600))
	end := start.Add(2 * time.Hour)
	promotion := model.Promotion{
		Name:     "Timezone promotion",
		StartAt:  start,
		EndAt:    end,
		Priority: 1,
		IsActive: 1,
	}
	if err := database.Create(&promotion).Error; err != nil {
		t.Fatalf("create promotion: %v", err)
	}
	if err := database.Create(&model.PromotionTablePrice{
		PromotionID:   promotion.ID,
		TableID:       1,
		NormalPrice:   100,
		PracticePrice: 50,
		IsActive:      1,
	}).Error; err != nil {
		t.Fatalf("create promotion table price: %v", err)
	}

	rows, err := LoadPromotionPrices(database, 1, start.Add(30*time.Minute), start.Add(45*time.Minute))
	if err != nil {
		t.Fatalf("load promotion prices: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("promotion rows: got %d want 1", len(rows))
	}
	assertMoney(t, rows[0].NormalPrice, 100)
}
