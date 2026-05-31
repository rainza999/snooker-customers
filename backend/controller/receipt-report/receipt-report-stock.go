package receiptreport

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	"github.com/rainza999/fiber-test/inventory"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

func SubmitDraft(c *fiber.Ctx) error {
	receiptID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || receiptID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid receipt id"})
	}
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

	var savedItem model.ProductReceiptItem
	err = inventory.WithTransaction(db.Db, func(tx *gorm.DB) error {
		var receipt model.ProductReceipt
		if err := tx.First(&receipt, uint(receiptID)).Error; err != nil {
			return err
		}
		if receipt.ReceiptStatus != "draft" {
			return errors.New("receipt has already been saved")
		}
		if receipt.SupplierID != draft.SupplierID || receipt.ReceiptNumber != strings.TrimSpace(draft.PurchaseOrderNumber) {
			return errors.New("receipt data changed, reload before editing")
		}

		findErr := tx.Where("receipt_id = ? AND product_id = ?", receipt.ID, draft.ProductID).
			First(&savedItem).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			savedItem = model.ProductReceiptItem{
				ReceiptID:         receipt.ID,
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
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "saved") || strings.Contains(strings.ToLower(err.Error()), "changed") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update receipt draft"})
	}
	return c.JSON(fiber.Map{"message": "success", "items": savedItem})
}
