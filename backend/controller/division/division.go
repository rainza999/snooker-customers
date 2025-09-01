package division

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"

	"path/filepath"
)

func AnyData(c *fiber.Ctx) error {

	fmt.Println("hello AnyData")
	var lists []model.Division

	result := db.Db.Find(&lists)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	return c.JSON(lists)
}
func Store(c *fiber.Ctx) error {
	// ดึงข้อมูลฟิลด์ต่างๆ จาก FormData
	name := c.FormValue("name")
	code := c.FormValue("code")
	shortName := c.FormValue("shortName")
	tel := c.FormValue("tel")
	line := c.FormValue("line")
	address := c.FormValue("address")
	isActiveStr := c.FormValue("isActive")
	isActive := uint8(0)
	if isActiveStr == "1" {
		isActive = 1
	}

	// ตรวจสอบและแปลงวันที่เปิดสาขา
	openingDateStr := c.FormValue("openingDate")
	openingDate, err := time.Parse("2006-01-02", openingDateStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid date format for openingDate"})
	}

	// จัดการกับไฟล์ QR code ถ้ามีการอัปโหลด
	var qrPath string
	file, err := c.FormFile("qrCode")
	if err == nil {
		uploadDir := "uploads"
		uploadPath := filepath.Join(uploadDir, file.Filename)
		if err := c.SaveFile(file, uploadPath); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save QR code file"})
		}
		qrPath = uploadPath
	}

	// กำหนดข้อมูล Division
	division := model.Division{
		Name:        name,
		Code:        code,
		ShortName:   shortName,
		Tel:         tel,
		Line:        line,
		Address:     address,
		OpeningDate: openingDate,
		IsActive:    isActive,
		QRPath:      qrPath,
		Display:     1,
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

func Edit(c *fiber.Ctx) error {
	fmt.Println("hello Edit")
	var division model.Division

	result := db.Db.Where("id = ?", c.Params("id")).First(&division)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting table not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	// ส่งข้อมูล division
	return c.JSON(division)
}
func Update(c *fiber.Ctx) error {
	// ดึงข้อมูลจากฟอร์ม
	name := c.FormValue("name")
	code := c.FormValue("code")
	shortName := c.FormValue("shortName")
	tel := c.FormValue("tel")
	line := c.FormValue("line")
	address := c.FormValue("address")
	isActive, _ := strconv.Atoi(c.FormValue("isActive")) // แปลงเป็น int

	// ตรวจสอบและแปลงวันที่เปิดสาขา
	openingDateStr := c.FormValue("openingDate")
	openingDate, err := time.Parse("2006-01-02", openingDateStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid date format for openingDate"})
	}

	// ดึง division จากฐานข้อมูลตาม id
	id := c.Params("id")
	var division model.Division
	if err := db.Db.First(&division, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Division not found"})
	}

	// จัดการกับไฟล์ QR code ถ้ามีการอัปโหลด
	file, err := c.FormFile("qrCode")
	if err == nil {
		uploadDir := "uploads"
		uploadPath := filepath.Join(uploadDir, file.Filename)
		if err := c.SaveFile(file, uploadPath); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save QR code file"})
		}
		division.QRPath = uploadPath
	}

	// อัปเดตข้อมูล Division
	division.Name = name
	division.Code = code
	division.ShortName = shortName
	division.Tel = tel
	division.Line = line
	division.Address = address
	division.OpeningDate = openingDate
	division.IsActive = uint8(isActive)

	// บันทึกข้อมูลที่อัปเดตลงในฐานข้อมูล
	if err := db.Db.Save(&division).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update division"})
	}

	return c.JSON(fiber.Map{
		"message":  "success",
		"division": division,
	})
}

func Delete(c *fiber.Ctx) error {
	id := c.Params("id")

	// ตรวจสอบว่ามีการใช้งาน division_id ในตาราง Visitation หรือไม่ และ deleted_at ต้องเป็น NULL
	var visitationCount int64
	if err := db.Db.Model(&model.Visitation{}).
		Where("division_id = ? AND deleted_at IS NULL", id).
		Count(&visitationCount).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to check visitation records"})
	}

	// ถ้ามีการใช้งาน division_id ใน Visitation จะไม่อนุญาตให้ลบ
	if visitationCount > 0 {
		return c.Status(400).JSON(fiber.Map{"error": "ไม่สามารถลบได้ เนื่องจากมีข้อมูลการใช้บริการของสาขานี้แล้ว"})
	}

	// ดึงข้อมูล division เพื่อทำการ soft delete
	var division model.Division
	if err := db.Db.First(&division, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Division not found"})
	}

	// ทำการ Soft Delete โดยใช้ GORM
	if err := db.Db.Delete(&division).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to soft delete division"})
	}

	return c.Status(200).JSON(fiber.Map{
		"message": "success",
	})
}
