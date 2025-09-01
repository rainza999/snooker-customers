package category

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

func AnyData(c *fiber.Ctx) error {

	fmt.Println("hello AnyData Categories")
	var lists []model.Category

	result := db.Db.Find(&lists)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	return c.JSON(lists)
}

func Store(c *fiber.Ctx) error {
	var data map[string]interface{}

	// อ่าน request body มายังตัวแปร data
	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// กำหนดข้อมูล Division
	division := model.Category{
		Name:     data["name"].(string),
		IsActive: uint8(data["isActive"].(float64)), // ตรวจสอบ type ให้ตรงกัน

	}

	// บันทึกข้อมูลลงในฐานข้อมูล
	if err := db.Db.Create(&division).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create division"})
	}

	return c.JSON(fiber.Map{
		"message":  "success",
		"division": division,
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
	var category model.Category

	result := db.Db.Where("id = ?", c.Params("id")).First(&category)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting table not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	// ส่งข้อมูล division
	return c.JSON(category)
}

func Update(c *fiber.Ctx) error {
	var data map[string]interface{}

	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	id := c.Params("id")
	var category model.Category
	if err := db.Db.First(&category, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Category not found"})
	}

	category.Name = data["name"].(string)
	category.IsActive = uint8(data["isActive"].(float64))

	if err := db.Db.Save(&category).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update category"})
	}

	return c.JSON(fiber.Map{
		"message":  "success",
		"category": category,
	})
}

// func Delete(c *fiber.Ctx) error {
// 	id := c.Params("id")

// 	// ตรวจสอบว่ามีการใช้งาน division_id ในตาราง Visitation หรือไม่ และ deleted_at ต้องเป็น NULL
// 	var visitationCount int64
// 	if err := db.Db.Model(&model.Visitation{}).
// 		Where("division_id = ? AND deleted_at IS NULL", id).
// 		Count(&visitationCount).Error; err != nil {
// 		return c.Status(500).JSON(fiber.Map{"error": "Failed to check visitation records"})
// 	}

// 	// ถ้ามีการใช้งาน division_id ใน Visitation จะไม่อนุญาตให้ลบ
// 	if visitationCount > 0 {
// 		return c.Status(400).JSON(fiber.Map{"error": "ไม่สามารถลบได้ เนื่องจากมีข้อมูลการใช้บริการของสาขานี้แล้ว"})
// 	}

// 	// ดึงข้อมูล division เพื่อทำการ soft delete
// 	var division model.D
// 	if err := db.Db.First(&division, id).Error; err != nil {
// 		return c.Status(404).JSON(fiber.Map{"error": "Division not found"})
// 	}

// 	// ทำการ Soft Delete โดยใช้ GORM
// 	if err := db.Db.Delete(&division).Error; err != nil {
// 		return c.Status(500).JSON(fiber.Map{"error": "Failed to soft delete division"})
// 	}

// 	return c.Status(200).JSON(fiber.Map{
// 		"message": "success",
// 	})
// }
