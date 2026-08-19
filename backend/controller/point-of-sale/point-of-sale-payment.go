package pointofsale

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/billing"
	"github.com/rainza999/fiber-test/db"
	"github.com/rainza999/fiber-test/inventory"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

var (
	errPaymentServicesStale  = errors.New("order items changed, reload before payment")
	errVisitationAlreadyPaid = errors.New("visitation has already been paid")
	errPaymentAmountMismatch = errors.New("payment amount changed, reload before payment")
	errPaymentInsufficient   = errors.New("paid amount is less than net price")
)

func PaymentStore(c *fiber.Ctx) error {
	var payment paymentRequest
	if err := c.BodyParser(&payment); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if payment.UUID == "" || payment.TotalCost == "" || payment.NetPrice == "" ||
		(payment.PaidAmount == "" && len(payment.Payments) == 0) || payment.EndTime == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Missing required payment field"})
	}
	if payment.TableType != 0 && payment.TableType != 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "TableType must be 0 or 1"})
	}

	totalCost, err := parseNonNegativeMoney(payment.TotalCost)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid total_cost format"})
	}
	netPrice, err := parseNonNegativeMoney(payment.NetPrice)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid net_price format"})
	}
	legacyPaidAmount := 0.0
	if payment.PaidAmount != "" {
		legacyPaidAmount, err = parseNonNegativeMoney(payment.PaidAmount)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid paid_amount format"})
		}
	}
	endTime, err := time.Parse(time.RFC3339, payment.EndTime)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid end_time format"})
	}
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load timezone"})
	}

	requestedServices, err := paymentServiceSnapshot(payment.Services)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	var visitation model.Visitation
	var billPayments []model.BillPayment
	err = inventory.WithTransaction(db.Db, func(tx *gorm.DB) error {
		if err := tx.Where("uuid = ? AND is_active = 1", payment.UUID).First(&visitation).Error; err != nil {
			return err
		}
		if visitation.IsPaid == 1 {
			return errVisitationAlreadyPaid
		}

		var services []model.Service
		if err := tx.Where("visitation_id = ?", visitation.ID).Find(&services).Error; err != nil {
			return err
		}
		if err := validatePaymentServiceSnapshot(services, requestedServices); err != nil {
			return err
		}

		now := time.Now()
		visitation.UseTime = elapsedVisitationSeconds(visitation, now)
		computedTotalCost, computedNetPrice, gameSegments, err := computeVisitationPaymentTotals(tx, visitation, services, payment.TableType)
		if err != nil {
			return err
		}
		if math.Abs(netPrice-computedNetPrice) > 0.01 || math.Abs(totalCost-computedTotalCost) > 0.01 {
			return errPaymentAmountMismatch
		}
		parsedPayments, appliedPaidAmount, changeAmount, err := payment.ToBillPayments(computedNetPrice, legacyPaidAmount, now)
		if err != nil {
			return err
		}
		billPayments = parsedPayments
		if visitation.BillCode == "" {
			billCode, err := nextBillCode(tx, visitation.DivisionID)
			if err != nil {
				return err
			}
			visitation.BillCode = billCode
		}
		visitation.TotalCost = computedTotalCost
		visitation.NetPrice = computedNetPrice
		visitation.PaidAmount = appliedPaidAmount
		visitation.ChangeAmount = changeAmount
		visitation.IsPaid = payment.IsPaid
		visitation.TableType = uint(payment.TableType)
		visitation.EndTime = endTime.In(location)
		if payment.IsPaid == 1 {
			visitation.IsActive = 0
			visitation.IsRunning = 0
		}
		if err := tx.Save(&visitation).Error; err != nil {
			return err
		}

		for _, service := range services {
			updates := map[string]interface{}{"status": "paid"}
			if service.ProductID == 1 {
				updates["use_time"] = visitation.UseTime
				updates["sell_quantity"] = float64(visitation.UseTime)
				updates["total_cost"] = computedGameAmount(gameSegments)
				updates["net_price"] = computedGameAmount(gameSegments)
			}
			if err := tx.Model(&model.Service{}).Where("id = ?", service.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		if err := billing.ReplaceBillPriceSegments(tx, visitation.ID, visitation.TableID, gameSegments); err != nil {
			return err
		}
		if err := billing.ReplaceBillPayments(tx, visitation.ID, billPayments); err != nil {
			return err
		}
		if err := billing.FinishPausePeriod(tx, visitation.ID, visitation.EndTime); err != nil {
			return err
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Record not found"})
	}
	if errors.Is(err, errPaymentServicesStale) || errors.Is(err, errVisitationAlreadyPaid) || errors.Is(err, errPaymentAmountMismatch) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	}
	if errors.Is(err, billing.ErrInvalidPaymentMethod) || errors.Is(err, errPaymentInsufficient) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save payment"})
	}

	broadcastPOSUpdate(visitation.DivisionID, "payment-updated")
	return c.JSON(fiber.Map{
		"relay":     publishPaidVisitationRelay(visitation, payment.IsPaid),
		"message":   "PaymentStore updated successfully",
		"bill_code": visitation.BillCode,
	})
}

func PaymentQuote(c *fiber.Ctx) error {
	uuid := c.Params("uuid")
	tableTypeValue, err := strconv.Atoi(c.Query("table_type", "0"))
	if err != nil || (tableTypeValue != 0 && tableTypeValue != 1) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "TableType must be 0 or 1"})
	}

	var visitation model.Visitation
	if err := db.Db.Where("uuid = ? AND is_active = 1", uuid).First(&visitation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Record not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load visitation"})
	}
	if visitation.IsPaid == 1 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": errVisitationAlreadyPaid.Error()})
	}

	var services []model.Service
	if err := db.Db.Where("visitation_id = ? AND deleted_at IS NULL", visitation.ID).Find(&services).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load services"})
	}

	now := time.Now()
	visitation.UseTime = elapsedVisitationSeconds(visitation, now)
	totalCost, netPrice, gameSegments, err := computeVisitationPaymentTotals(db.Db, visitation, services, uint8(tableTypeValue))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to calculate payment quote"})
	}
	pausePeriods, pauseLogTotalSeconds, err := billing.LoadPausePeriods(db.Db, visitation.ID, now)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load pause details"})
	}

	gameFee := computedGameAmount(gameSegments)
	return c.JSON(fiber.Map{
		"total_cost":              totalCost,
		"net_price":               netPrice,
		"game_fee":                gameFee,
		"order_fee":               roundMoney(netPrice - gameFee),
		"use_time":                visitation.UseTime,
		"table_type":              tableTypeValue,
		"segments":                gameSegments,
		"pause_periods":           pausePeriods,
		"paused_duration":         visitation.PausedDuration,
		"pause_log_total_seconds": pauseLogTotalSeconds,
	})
}

func paymentServiceSnapshot(services []ServiceData) (map[uint]ServiceData, error) {
	snapshot := make(map[uint]ServiceData, len(services))
	for _, service := range services {
		if service.ProductID <= 0 || service.SellQuantity <= 0 || math.IsNaN(service.SellQuantity) || math.IsInf(service.SellQuantity, 0) {
			return nil, errors.New("invalid payment service")
		}
		productID := uint(service.ProductID)
		if _, exists := snapshot[productID]; exists {
			return nil, errors.New("duplicated payment service")
		}
		if _, err := parseNonNegativeMoney(service.TotalCost); err != nil {
			return nil, errors.New("invalid service total cost")
		}
		if _, err := parseNonNegativeMoney(service.NetPrice); err != nil {
			return nil, errors.New("invalid service net price")
		}
		snapshot[productID] = service
	}
	return snapshot, nil
}

func validatePaymentServiceSnapshot(services []model.Service, requested map[uint]ServiceData) error {
	persistedProducts := make(map[uint]struct{}, len(services))
	for _, service := range services {
		persistedProducts[service.ProductID] = struct{}{}
		if service.ProductID == 1 {
			continue
		}
		snapshot, ok := requested[service.ProductID]
		if !ok || math.Abs(snapshot.SellQuantity-service.SellQuantity) > 0.000001 {
			return errPaymentServicesStale
		}
	}
	for productID := range requested {
		if productID == 1 {
			continue
		}
		if _, ok := persistedProducts[productID]; !ok {
			return errPaymentServicesStale
		}
	}
	return nil
}

func elapsedVisitationSeconds(visitation model.Visitation, now time.Time) int64 {
	if visitation.StartTime.IsZero() {
		return 0
	}
	elapsed := now.Sub(visitation.StartTime).Seconds() - float64(visitation.PausedDuration)
	if visitation.IsRunning == 0 && !visitation.PauseTime.IsZero() && visitation.PauseTime.Year() != 2000 {
		elapsed = visitation.PauseTime.Sub(visitation.StartTime).Seconds() - float64(visitation.PausedDuration)
	}
	if elapsed < 0 {
		return 0
	}
	return int64(elapsed)
}

func nextBillCode(tx *gorm.DB, divisionID uint) (string, error) {
	var division model.Division
	if err := tx.First(&division, divisionID).Error; err != nil {
		return "", err
	}
	prefix := division.Code + time.Now().Format("060102")
	var latest model.Visitation
	err := tx.Where("bill_code LIKE ?", prefix+"%").Order("bill_code DESC").First(&latest).Error
	number := 1
	if err == nil {
		if len(latest.BillCode) < 3 {
			return "", errors.New("invalid existing bill code")
		}
		last, err := strconv.Atoi(latest.BillCode[len(latest.BillCode)-3:])
		if err != nil {
			return "", err
		}
		number = last + 1
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	return fmt.Sprintf("%s%03d", prefix, number), nil
}

func parseNonNegativeMoney(value string) (float64, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return 0, errors.New("invalid money value")
	}
	return number, nil
}

type paymentRequest struct {
	UUID       string        `json:"uuid"`
	TotalCost  string        `json:"total_cost"`
	NetPrice   string        `json:"net_price"`
	IsPaid     uint8         `json:"is_paid"`
	EndTime    string        `json:"end_time"`
	PaidAmount string        `json:"paid_amount"`
	TableType  uint8         `json:"table_type"`
	Services   []ServiceData `json:"services"`
	Payments   []paymentPart `json:"payments"`
}

type paymentPart struct {
	Method string `json:"method"`
	Amount string `json:"amount"`
	Note   string `json:"note"`
}

func (payment paymentRequest) ToBillPayments(netPrice float64, legacyPaidAmount float64, paidAt time.Time) ([]model.BillPayment, float64, float64, error) {
	if len(payment.Payments) == 0 {
		if legacyPaidAmount < netPrice {
			return nil, 0, 0, errPaymentInsufficient
		}
		return []model.BillPayment{{
			Method:   billing.PaymentMethodCash,
			Amount:   roundMoney(netPrice),
			PaidAt:   paidAt,
			IsActive: 1,
		}}, roundMoney(legacyPaidAmount), roundMoney(legacyPaidAmount - netPrice), nil
	}

	payments := make([]model.BillPayment, 0, len(payment.Payments))
	total := 0.0
	for _, part := range payment.Payments {
		if part.Method == "" || part.Amount == "" {
			return nil, 0, 0, errors.New("Missing payment method or amount")
		}
		if err := billing.ValidatePaymentMethod(part.Method); err != nil {
			return nil, 0, 0, err
		}
		amount, err := parseNonNegativeMoney(part.Amount)
		if err != nil || amount <= 0 {
			return nil, 0, 0, errors.New("Invalid payment amount")
		}
		amount = roundMoney(amount)
		total += amount
		payments = append(payments, model.BillPayment{
			Method:   part.Method,
			Amount:   amount,
			Note:     part.Note,
			PaidAt:   paidAt,
			IsActive: 1,
		})
	}

	total = roundMoney(total)
	if math.Abs(total-netPrice) > 0.01 {
		return nil, 0, 0, errPaymentAmountMismatch
	}
	return payments, total, 0, nil
}

func computeVisitationPaymentTotals(tx *gorm.DB, visitation model.Visitation, services []model.Service, tableType uint8) (float64, float64, []billing.PriceSegment, error) {
	var table model.SettingTable
	if err := tx.First(&table, visitation.TableID).Error; err != nil {
		return 0, 0, nil, err
	}

	chargeStart := visitation.StartTime
	chargeEnd := chargeStart.Add(time.Duration(visitation.UseTime) * time.Second)
	if !chargeEnd.After(chargeStart) {
		chargeEnd = chargeStart.Add(time.Second)
	}
	promotions, err := billing.LoadPromotionPrices(tx, visitation.TableID, chargeStart, chargeEnd)
	if err != nil {
		return 0, 0, nil, err
	}
	segments, gameAmount := billing.CalculateGameFeeSegments(chargeStart, visitation.UseTime, tableType, billing.TablePrice{
		TableID:       visitation.TableID,
		NormalPrice:   table.Price,
		PracticePrice: table.Price2,
	}, promotions)

	totalCost := 0.0
	netPrice := 0.0
	for _, service := range services {
		if service.ProductID == 1 {
			totalCost += gameAmount
			netPrice += gameAmount
			continue
		}
		totalCost += service.TotalCost
		netPrice += service.NetPrice
	}
	return roundMoney(totalCost), roundMoney(netPrice), segments, nil
}

func computedGameAmount(segments []billing.PriceSegment) float64 {
	total := 0.0
	for _, segment := range segments {
		total += segment.Amount
	}
	return roundMoney(total)
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
