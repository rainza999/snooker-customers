package settingpointofsale

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
)

func GetView(c *fiber.Ctx) error {
	id := 1

	var setting model.SettingPointOfSale // ประกาศ model SettingSystem

	// ค้นหา setting ตาม id
	if err := db.Db.First(&setting, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting system not found"})
	}

	return c.JSON(setting) // ส่งข้อมูล setting กลับไป
	// var settingTable model.SettingTable

	// result := db.Db.Where("id = ?", c.Params("id")).First(&settingTable)

	// if result.Error != nil {
	// 	if result.Error == gorm.ErrRecordNotFound {
	// 		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting table not found"})
	// 	}
	// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	// }

	// // ส่งข้อมูล settingTable กลับไป รวมถึง relay
	// return c.JSON(settingTable)
}

func GetSettingSystem(c *fiber.Ctx) error {
	id := c.Params("id") // รับ id จาก URL

	var setting model.SettingSystem // ประกาศ model SettingSystem

	// ค้นหา setting ตาม id
	if err := db.Db.First(&setting, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting system not found"})
	}

	return c.JSON(setting) // ส่งข้อมูล setting กลับไป
}
func SaveSettingPointOfSale(c *fiber.Ctx) error {
	var setting model.SettingPointOfSale

	if err := db.Db.First(&setting, 1).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Settings not found"})
	}

	billingInterval := c.FormValue("billing_interval")
	if billingInterval != "" {
		if val, err := strconv.Atoi(billingInterval); err == nil {
			setting.CalProcess = uint8(val)
		}
	}

	// ✅ บันทึกลงฐานข้อมูล
	if err := db.Db.Save(&setting).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update settings"})
	}

	fmt.Println("Settings updated successfully!")
	return c.JSON(fiber.Map{"message": "Settings updated successfully!"})
}
