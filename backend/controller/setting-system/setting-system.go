package settingsystem

import (
	"fmt"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
)

func GetSettingSystem(c *fiber.Ctx) error {
	id := c.Params("id") // รับ id จาก URL

	var setting model.SettingSystem // ประกาศ model SettingSystem

	// ค้นหา setting ตาม id
	if err := db.Db.First(&setting, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting system not found"})
	}

	return c.JSON(setting) // ส่งข้อมูล setting กลับไป
}
func SaveSettingSystem(c *fiber.Ctx) error {
	var setting model.SettingSystem

	// รับข้อมูลที่มี id = 1 จากฐานข้อมูล
	if err := db.Db.First(&setting, 1).Error; err != nil {
		// ถ้าไม่พบ ให้แสดงข้อผิดพลาด
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Settings not found"})
	}

	// รับค่ารหัสผ่านจาก form data
	closeTablePassword := c.FormValue("closeTablePassword")
	editReportPassword := c.FormValue("editReportPassword")

	// แสดงค่าที่ได้รับใน console
	fmt.Println("closeTablePassword:", closeTablePassword)
	fmt.Println("editReportPassword:", editReportPassword)

	// ตั้งค่าและเข้ารหัสรหัสผ่านปิดโต๊ะ
	if err := setting.SetCloseTablePassword(closeTablePassword); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash close table password"})
	}

	// ตั้งค่าและเข้ารหัสรหัสผ่านสำหรับแก้ไขรายงาน
	if err := setting.SetEditReportPassword(editReportPassword); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash edit report password"})
	}

	file, err := c.FormFile("logo")
	if err == nil {
		// หากมีไฟล์ ให้ทำการบันทึก
		fmt.Println("Uploaded file name:", file.Filename)

		uploadDir := "uploads"
		uploadPath := filepath.Join(uploadDir, file.Filename)

		// บันทึกไฟล์และแสดงเส้นทางที่บันทึกใน console
		if err := c.SaveFile(file, uploadPath); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save logo file"})
		}
		setting.LogoPath = uploadPath // เก็บ path ของรูปภาพในฐานข้อมูล

		fmt.Println("File saved at:", uploadPath)
	} else if err.Error() == "there is no uploaded file associated with the given key" {
		// ไม่มีการอัปโหลดไฟล์ใหม่ ให้เก็บ path เดิม
		fmt.Println("No new file uploaded. Using existing LogoPath:", setting.LogoPath)
	} else {
		// กรณีมีข้อผิดพลาดอื่น ๆ
		fmt.Println("File upload error:", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid file upload"})
	}

	// ==== จัดการอัปโหลด logo หน้า Login ====
	logoLoginFile, errLogin := c.FormFile("logo_login")
	if errLogin == nil {
		// มีการอัปโหลดโลโก้หน้า Login ใหม่
		fmt.Println("Uploaded Login Logo file name:", logoLoginFile.Filename)

		uploadDir := "uploads"
		uploadPathLogin := filepath.Join(uploadDir, logoLoginFile.Filename)

		if err := c.SaveFile(logoLoginFile, uploadPathLogin); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save logo_login file"})
		}

		setting.LogoLoginPath = uploadPathLogin
		fmt.Println("Login Logo saved at:", uploadPathLogin)
	} else if errLogin.Error() == "there is no uploaded file associated with the given key" {
		// ไม่มีการอัปโหลดไฟล์ใหม่ ให้ใช้ path เดิม
		fmt.Println("No new login logo uploaded. Using existing LogoLoginPath:", setting.LogoLoginPath)
	} else {
		fmt.Println("Login Logo upload error:", errLogin)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid logo_login file upload"})
	}

	// อัปเดตข้อมูลการตั้งค่าในฐานข้อมูล
	if err := db.Db.Save(&setting).Error; err != nil {
		fmt.Println("Database save error:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save settings"})
	}

	fmt.Println("Settings updated successfully!")
	return c.JSON(fiber.Map{"message": "Settings updated successfully!"})
}
