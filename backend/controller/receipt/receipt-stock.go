package receipt

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	"github.com/rainza999/fiber-test/inventory"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

var (
	errReceiptAlreadySaved = errors.New("receipt has already been saved")
	errReceiptHasStock     = errors.New("receipt item already has stock entries")
	errReceiptHasNoItems   = errors.New("receipt has no items")
)

func DeleteReceipt(c *fiber.Ctx) error {
	id := c.Params("id")
	err := inventory.WithTransaction(db.Db, func(tx *gorm.DB) error {
		var item model.ProductReceiptItem
		if err := tx.First(&item, id).Error; err != nil {
			return err
		}
		var receipt model.ProductReceipt
		if err := tx.First(&receipt, item.ReceiptID).Error; err != nil {
			return err
		}
		if receipt.ReceiptStatus != "draft" {
			return errReceiptAlreadySaved
		}
		var stockCount int64
		if err := tx.Model(&model.StockEntry{}).
			Where("product_receipt_item_id = ?", item.ID).
			Count(&stockCount).Error; err != nil {
			return err
		}
		if stockCount > 0 {
			return errReceiptHasStock
		}
		return tx.Delete(&item).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "product_receipt_item not found"})
	}
	if errors.Is(err, errReceiptAlreadySaved) || errors.Is(err, errReceiptHasStock) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete receipt item"})
	}
	return c.JSON(fiber.Map{"message": "success"})
}

func SubmitReceipt(c *fiber.Ctx) error {
	var payload struct {
		Drafts struct {
			SupplierID          uint    `json:"supplier"`
			ProductID           uint    `json:"product"`
			Quantity            int     `json:"quantity"`
			TotalPrice          float64 `json:"totalPrice"`
			PurchaseOrderNumber string  `json:"purchaseOrderNumber"`
		} `json:"drafts"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	draft := payload.Drafts
	if draft.SupplierID == 0 || draft.ProductID == 0 || draft.Quantity <= 0 ||
		draft.TotalPrice <= 0 || strings.TrimSpace(draft.PurchaseOrderNumber) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid data, please check the input values"})
	}

	var savedReceipt model.ProductReceipt
	var savedItem model.ProductReceiptItem
	err := inventory.WithTransaction(db.Db, func(tx *gorm.DB) error {
		findErr := tx.Where("supplier_id = ? AND receipt_number = ?", draft.SupplierID, strings.TrimSpace(draft.PurchaseOrderNumber)).
			First(&savedReceipt).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			savedReceipt = model.ProductReceipt{
				SupplierID:    draft.SupplierID,
				ReceiptNumber: strings.TrimSpace(draft.PurchaseOrderNumber),
				ReceivedDate:  time.Now(),
				ReceiptStatus: "draft",
				IsActive:      1,
			}
			if err := tx.Create(&savedReceipt).Error; err != nil {
				return err
			}
		}
		if savedReceipt.ReceiptStatus != "draft" {
			return errReceiptAlreadySaved
		}

		findErr = tx.Where("receipt_id = ? AND product_id = ?", savedReceipt.ID, draft.ProductID).
			First(&savedItem).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			savedItem = model.ProductReceiptItem{
				ReceiptID:         savedReceipt.ID,
				ProductID:         draft.ProductID,
				Quantity:          draft.Quantity,
				UnitPrice:         draft.TotalPrice / float64(draft.Quantity),
				TotalPrice:        draft.TotalPrice,
				ReceiptItemStatus: "draft",
				IsActive:          1,
			}
			return tx.Create(&savedItem).Error
		}

		savedItem.Quantity += draft.Quantity
		savedItem.TotalPrice += draft.TotalPrice
		savedItem.UnitPrice = savedItem.TotalPrice / float64(savedItem.Quantity)
		return tx.Save(&savedItem).Error
	})
	if errors.Is(err, errReceiptAlreadySaved) || isUniqueConstraintError(err) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Cannot add or update items because this receipt has already been saved or duplicated"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save receipt item"})
	}
	return c.JSON(fiber.Map{"message": "success", "receipt": savedReceipt, "items": savedItem})
}

func FinalizeReceipt(c *fiber.Ctx) error {
	var payload struct {
		Receipts []struct {
			ReceiptID uint `json:"receipt_id"`
		} `json:"receipts"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if len(payload.Receipts) == 0 || payload.Receipts[0].ReceiptID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Receipt is required"})
	}
	receiptID := payload.Receipts[0].ReceiptID

	err := inventory.WithTransaction(db.Db, func(tx *gorm.DB) error {
		var receipt model.ProductReceipt
		if err := tx.Where("id = ? AND receipt_status = ?", receiptID, "draft").First(&receipt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errReceiptAlreadySaved
			}
			return err
		}

		var items []model.ProductReceiptItem
		if err := tx.Where("receipt_id = ?", receipt.ID).Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return errReceiptHasNoItems
		}
		for _, item := range items {
			if item.Quantity <= 0 || item.TotalPrice <= 0 {
				return errors.New("receipt contains an invalid item")
			}
			if _, err := inventory.CreateReceiptStock(tx, item); err != nil {
				return err
			}
			if err := tx.Model(&model.ProductReceiptItem{}).
				Where("id = ? AND receipt_item_status = ?", item.ID, "draft").
				Update("receipt_item_status", "save").Error; err != nil {
				return err
			}
		}
		result := tx.Model(&model.ProductReceipt{}).
			Where("id = ? AND receipt_status = ?", receipt.ID, "draft").
			Update("receipt_status", "save")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errReceiptAlreadySaved
		}
		return nil
	})
	if errors.Is(err, errReceiptAlreadySaved) || isUniqueConstraintError(err) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Cannot finalize because this receipt has already been saved"})
	}
	if errors.Is(err, errReceiptHasNoItems) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to finalize receipt"})
	}
	return c.JSON(fiber.Map{"message": "success"})
}

func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
