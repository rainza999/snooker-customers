package supplier

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

func AnyData(c *fiber.Ctx) error {

	fmt.Println("hello AnyData Supplier")
	var lists []model.Supplier

	result := db.Db.Where("is_active = ?", 1).Find(&lists)

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
