package supplier

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

func AnyData(c *fiber.Ctx) error {
	var lists []model.Supplier
	result := db.Db.Find(&lists)
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	return c.JSON(lists)
}

func Store(c *fiber.Ctx) error {
	var data map[string]interface{}

	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	supplier := model.Supplier{
		Name:     data["name"].(string),
		Contact:  data["contact"].(string),
		Address:  data["address"].(string),
		IsActive: data["isActive"].(bool), // ตรวจสอบ type ให้ตรงกัน

	}

	if err := db.Db.Create(&supplier).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create product"})
	}

	return c.JSON(fiber.Map{
		"message":  "success",
		"supplier": supplier,
	})
}

func GetView(c *fiber.Ctx) error {

	return c.JSON(fiber.Map{
		"status": "ok",
	})

}

func GetCreateView(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}

func GetEditView(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}

func Edit(c *fiber.Ctx) error {
	var supplier model.Supplier

	result := db.Db.Where("id = ?", c.Params("id")).First(&supplier)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting table not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	return c.JSON(supplier)
}

func Update(c *fiber.Ctx) error {
	var data map[string]interface{}

	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	id := c.Params("id")
	var supplier model.Supplier
	if err := db.Db.First(&supplier, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "product not found"})
	}

	supplier.Name = data["name"].(string)
	supplier.Contact = data["contact"].(string)
	supplier.Address = data["address"].(string)
	supplier.IsActive = data["isActive"].(bool)

	if err := db.Db.Save(&supplier).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update product"})
	}

	return c.JSON(fiber.Map{
		"message":  "success",
		"supplier": supplier,
	})
}

func Delete(c *fiber.Ctx) error {
	var supplier model.Supplier
	supplierID := c.Params("id")

	// 🔍 1. เช็คว่ามี supplier นี้ไหม
	if err := db.Db.First(&supplier, "id = ?", supplierID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Supplier not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// 🔍 2. เช็คว่ามีการใช้งาน supplier_id ใน product_receipts หรือไม่
	var count int64
	err := db.Db.Table("product_receipts").
		Where("supplier_id = ? AND deleted_at IS NULL AND is_active = ?", supplierID, true).
		Count(&count).Error
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to check product_receipts: " + err.Error(),
		})
	}

	if count > 0 {
		// 🚫 ถ้ามีการใช้งาน ห้ามลบ
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot delete supplier — this supplier is still used in active product receipts",
		})
	}

	// ✅ 3. Soft Delete supplier ได้เลย
	if err := db.Db.Delete(&supplier).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Supplier deleted successfully",
	})
}
