package middleware

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
)

// PermissionMiddleware เป็น middleware ที่ตรวจสอบการอนุญาต
func PermissionMiddleware(permissionName string) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		// ดึงข้อมูล roleID จาก context ที่ได้จาก middleware ก่อนหน้า
		log.Println("permission_middleware")
		log.Println("Test")
		roleID, ok := c.Locals("roleID").(int)
		log.Println(roleID)
		log.Println("test")
		if !ok {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized: Invalid role ID")
		}

		// ตรวจสอบการอนุญาต
		if !hasPermission(int(roleID), permissionName) {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden: Insufficient permissions")
		}

		// ทำงานต่อไป
		return c.Next()
	}
}

// hasPermission ตรวจสอบว่า user มีการอนุญาตที่ต้องการหรือไม่
func hasPermission(roleID int, permissionName string) bool {
	// ทำการ query ในฐานข้อมูลเพื่อตรวจสอบการอนุญาต

	var permission model.Permission
	result := db.Db.Where("name = ?", permissionName).First(&permission)
	if result.Error != nil {
		// หากไม่พบ permission ในฐานข้อมูล
		return false
	}

	var roleHasPermission model.RoleHasPermission
	result = db.Db.Where("role_id = ? AND permission_id = ?", roleID, permission.ID).First(&roleHasPermission)

	return result.Error == nil
}
