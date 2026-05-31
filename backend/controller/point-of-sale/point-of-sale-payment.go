package pointofsale

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	"github.com/rainza999/fiber-test/inventory"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

var (
	errPaymentServicesStale  = errors.New("order items changed, reload before payment")
	errVisitationAlreadyPaid = errors.New("visitation has already been paid")
)

func PaymentStore(c *fiber.Ctx) error {
	var payment PaymentData
	if err := c.BodyParser(&payment); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if payment.UUID == "" || payment.TotalCost == "" || payment.NetPrice == "" ||
		payment.PaidAmount == "" || payment.EndTime == "" {
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
	paidAmount, err := parseNonNegativeMoney(payment.PaidAmount)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid paid_amount format"})
	}
	if paidAmount < netPrice {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Paid amount is less than net price"})
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
		if visitation.BillCode == "" {
			billCode, err := nextBillCode(tx, visitation.DivisionID)
			if err != nil {
				return err
			}
			visitation.BillCode = billCode
		}
		visitation.TotalCost = totalCost
		visitation.NetPrice = netPrice
		visitation.PaidAmount = paidAmount
		visitation.ChangeAmount = paidAmount - netPrice
		visitation.IsPaid = payment.IsPaid
		visitation.TableType = uint(payment.TableType)
		visitation.EndTime = endTime.In(location)
		if err := tx.Save(&visitation).Error; err != nil {
			return err
		}

		for _, service := range services {
			updates := map[string]interface{}{"status": "paid"}
			if service.ProductID == 1 {
				updates["use_time"] = visitation.UseTime
				if gameSnapshot, ok := requestedServices[service.ProductID]; ok {
					gameTotal, err := parseNonNegativeMoney(gameSnapshot.TotalCost)
					if err != nil {
						return err
					}
					gameNet, err := parseNonNegativeMoney(gameSnapshot.NetPrice)
					if err != nil {
						return err
					}
					updates["sell_quantity"] = gameSnapshot.SellQuantity
					updates["total_cost"] = gameTotal
					updates["net_price"] = gameNet
				}
			}
			if err := tx.Model(&model.Service{}).Where("id = ?", service.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Record not found"})
	}
	if errors.Is(err, errPaymentServicesStale) || errors.Is(err, errVisitationAlreadyPaid) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
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
	if !visitation.PauseTime.IsZero() && visitation.PauseTime.Year() != 2000 {
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
