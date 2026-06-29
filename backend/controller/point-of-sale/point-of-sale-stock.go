package pointofsale

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	"github.com/rainza999/fiber-test/inventory"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

var (
	errInvalidOrderQty  = errors.New("stock quantity must be a whole number greater than or equal to zero")
	errNoOrderChanges   = errors.New("no changes made to the order")
	errOrderAlreadyPaid = errors.New("paid order cannot be changed")
)

func OrderStore(c *fiber.Ctx) error {
	uuid := c.Params("uuid")
	var order struct {
		VisitationID uint       `json:"visitation_id"`
		ProductID    uint       `json:"product_id"`
		Quantity     float64    `json:"quantity"`
		Price        float64    `json:"price"`
		Action       string     `json:"action"`
		Status       *string    `json:"status"`
		DeletedAt    *time.Time `json:"deleted_at"`
	}
	if err := c.BodyParser(&order); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse order data"})
	}
	if order.ProductID == 0 || math.IsNaN(order.Quantity) || math.IsInf(order.Quantity, 0) ||
		math.IsNaN(order.Price) || math.IsInf(order.Price, 0) || order.Quantity < 0 || order.Price < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid order quantity or price"})
	}
	action := normalizeOrderAction(order.Action)
	if action == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid order action"})
	}
	if (action == "increment" || action == "decrement") && order.Quantity == 0 {
		order.Quantity = 1
	}

	var visitation model.Visitation
	var product model.Product
	var savedService model.Service
	event := "order-updated"
	err := inventory.WithTransaction(db.Db, func(tx *gorm.DB) error {
		if err := tx.Where("uuid = ? AND is_active = 1 AND is_paid <> 1", uuid).First(&visitation).Error; err != nil {
			return err
		}
		if err := tx.First(&product, order.ProductID).Error; err != nil {
			return err
		}
		managed, err := inventory.ShouldManageStock(tx, product.ID)
		if err != nil {
			return err
		}
		if managed && math.Trunc(order.Quantity) != order.Quantity {
			return errInvalidOrderQty
		}

		var service model.Service
		findErr := tx.Where("visitation_id = ? AND product_id = ?", visitation.ID, product.ID).First(&service).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			if action == "delete" || action == "decrement" || order.Quantity <= 0 {
				savedService = model.Service{
					VisitationID: visitation.ID,
					ProductID:    product.ID,
					SellQuantity: 0,
					TotalCost:    0,
					NetPrice:     0,
					Status:       "delete",
				}
				return errNoOrderChanges
			}
			targetQuantity := order.Quantity
			if action == "increment" {
				targetQuantity = order.Quantity
			}
			if targetQuantity <= 0 {
				return errInvalidOrderQty
			}
			service = model.Service{
				VisitationID: visitation.ID,
				ProductID:    product.ID,
				SellQuantity: targetQuantity,
				TotalCost:    targetQuantity * order.Price,
				NetPrice:     targetQuantity * order.Price,
				UseTime:      visitation.UseTime,
				Status:       "draft",
			}
			if err := tx.Create(&service).Error; err != nil {
				return err
			}
			fifoCost, err := inventory.DeductFIFO(tx, product.ID, service.ID, visitation.ID, int(targetQuantity))
			if err != nil {
				return err
			}
			service.TotalFIFO_Cost = fifoCost
			if err := tx.Save(&service).Error; err != nil {
				return err
			}
			savedService = service
			event = "order-created"
			return nil
		}
		if service.Status == "paid" {
			return errOrderAlreadyPaid
		}

		targetQuantity := order.Quantity
		switch action {
		case "increment":
			targetQuantity = service.SellQuantity + order.Quantity
		case "decrement":
			targetQuantity = service.SellQuantity - order.Quantity
			if targetQuantity < 0 {
				targetQuantity = 0
			}
		case "delete":
			targetQuantity = 0
		}

		delta := targetQuantity - service.SellQuantity
		if math.Abs(delta) < 0.000001 {
			savedService = service
			return errNoOrderChanges
		}
		if managed && math.Trunc(delta) != delta {
			return errInvalidOrderQty
		}
		if delta > 0 {
			fifoCost, err := inventory.DeductFIFO(tx, product.ID, service.ID, visitation.ID, int(delta))
			if err != nil {
				return err
			}
			service.TotalFIFO_Cost += fifoCost
		} else {
			returnedCost, err := inventory.ReturnFIFO(tx, product.ID, service.ID, visitation.ID, int(-delta))
			if err != nil {
				return err
			}
			service.TotalFIFO_Cost -= returnedCost
			if service.TotalFIFO_Cost < 0.000001 {
				service.TotalFIFO_Cost = 0
			}
		}

		service.SellQuantity = targetQuantity
		service.TotalCost = targetQuantity * order.Price
		service.NetPrice = service.TotalCost
		if targetQuantity == 0 || action == "delete" || (order.Status != nil && *order.Status == "delete") {
			service.Status = "delete"
			service.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
		} else if order.Status != nil {
			service.Status = *order.Status
		}
		if err := tx.Save(&service).Error; err != nil {
			return err
		}
		savedService = service
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Record not found"})
	}
	if errors.Is(err, inventory.ErrInsufficientStock) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Not enough stock"})
	}
	if errors.Is(err, errNoOrderChanges) {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Order already up to date",
			"service": savedService,
		})
	}
	if errors.Is(err, errInvalidOrderQty) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if errors.Is(err, errOrderAlreadyPaid) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	broadcastPOSUpdate(visitation.DivisionID, event)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Order saved successfully",
		"service": savedService,
	})
}

func normalizeOrderAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "set", "update":
		return "set"
	case "increment", "add":
		return "increment"
	case "decrement", "subtract":
		return "decrement"
	case "delete", "remove":
		return "delete"
	default:
		return ""
	}
}

func CalculateFIFO(productID uint, quantity int, database *gorm.DB) (float64, error) {
	var total float64
	err := inventory.WithTransaction(database, func(tx *gorm.DB) error {
		var err error
		total, err = inventory.DeductFIFO(tx, productID, 0, 0, quantity)
		return err
	})
	return total, err
}

func ReturnStockFIFO(productID uint, quantity int, database *gorm.DB) (float64, error) {
	var total float64
	err := inventory.WithTransaction(database, func(tx *gorm.DB) error {
		var err error
		total, err = inventory.ReturnFIFO(tx, productID, 0, 0, quantity)
		return err
	})
	return total, err
}

func ShouldManageStock(productID uint, tx *gorm.DB) (bool, error) {
	return inventory.ShouldManageStock(tx, productID)
}
