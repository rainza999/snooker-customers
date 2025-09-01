package role

import (
	"fmt"
	// "log"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	// "context"
	// helper "github.com/rainza999/fiber-test/controller/helper"
	// "github.com/go-redis/redis/v8"
)

func AnyData(c *fiber.Ctx) error {

	fmt.Println("hello AnyData")
	var lists []model.Role

	result := db.Db.Find(&lists)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	return c.JSON(lists)
}

func CreateAnyData(c *fiber.Ctx) error {
	fmt.Println("hello Create AnyData")

	var mainMenus []model.Menu

	db.Db.
		Where("is_active = ?", true).
		Where("level = ?", 0).
		// Order("order asc").
		Find(&mainMenus)

	// fmt.Println(mainMenus)

	// var menus []model.Menu
	menus := make([]map[string]interface{}, len(mainMenus))

	for key, mainMenu := range mainMenus {
		menu := make(map[string]interface{})
		menu["id"] = mainMenu.ID
		menu["name"] = mainMenu.Name
		menu["route"] = mainMenu.Route
		menu["level"] = mainMenu.Level
		menu["relation"] = mainMenu.Relation
		menu["order"] = mainMenu.Order
		menu["has_sub"] = mainMenu.HasSub
		menu["icon"] = mainMenu.Icon
		menu["is_active"] = mainMenu.IsActive

		if mainMenu.HasSub != 0 {
			level := mainMenu.Level + 1
			subMenus := getSubMenu(mainMenu.ID, level)
			menu["sub"] = subMenus
			// subMenuPermissions := getSubMenuPermission(mainMenu.ID,level)
			// menu["sub_permission"] = subMenuPermissions
		}

		permissionMenu := getPermissionMenu(mainMenu.ID)
		menu["permission"] = permissionMenu
		menus[key] = menu
	}

	fmt.Println(menus)

	// return c.JSON(menus)
	return c.JSON(fiber.Map{
		"menus": menus,
	})
}

func getPermissionMenu(parentID uint) interface{} {
	var permissionMenus []model.Permission
	db.Db.Where("is_active = ?", true).
		Where("menu_id = ?", parentID).
		Find(&permissionMenus)

	permissionMenu := make([]map[string]interface{}, len(permissionMenus))
	for key, permissionRecord := range permissionMenus {
		permissionMenuRecord := make(map[string]interface{})
		permissionMenuRecord["id"] = permissionRecord.ID
		permissionMenuRecord["name"] = permissionRecord.Name
		permissionMenuRecord["title"] = permissionRecord.Title

		permissionMenu[key] = permissionMenuRecord
	}

	return permissionMenu
}

func getSubMenu(parentID uint, level uint8) interface{} {
	var subMenus []model.Menu
	db.Db.Select("id", "name", "route", "level", "relation", "order", "has_sub", "icon").
		Where("is_active = ?", true).
		Where("relation = ?", parentID).
		// Order("order asc").
		Find(&subMenus)

	subMenu := make([]map[string]interface{}, len(subMenus))
	for key, menuRecord := range subMenus {
		subMenuRecord := make(map[string]interface{})
		subMenuRecord["id"] = menuRecord.ID
		subMenuRecord["name"] = menuRecord.Name
		subMenuRecord["route"] = menuRecord.Route
		subMenuRecord["level"] = menuRecord.Level
		subMenuRecord["relation"] = menuRecord.Relation
		subMenuRecord["order"] = menuRecord.Order
		subMenuRecord["has_sub"] = menuRecord.HasSub
		subMenuRecord["icon"] = menuRecord.Icon
		subMenuRecord["is_active"] = menuRecord.IsActive

		if menuRecord.HasSub != 0 {
			nextLevel := menuRecord.Level + 1
			subMenuRecords := getSubMenu(menuRecord.ID, nextLevel)
			subMenuRecord["sub"] = subMenuRecords
		}

		permissionMenu := getPermissionMenu(menuRecord.ID)
		subMenuRecord["permission"] = permissionMenu

		subMenu[key] = subMenuRecord
	}

	return subMenu
}

// RolePermissionRequest is a struct to represent the request from the frontend
type RolePermissionRequest struct {
	RoleName            string `json:"roleName"`
	SelectedPermissions []uint `json:"selectedPermissions"`
}

func Store(c *fiber.Ctx) error {
	var rolePermissionRequest RolePermissionRequest
	if err := c.BodyParser(&rolePermissionRequest); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request payload",
		})
	}
	role := model.Role{Name: rolePermissionRequest.RoleName}
	tx := db.Db.Begin()

	if err := tx.Create(&role).Error; err != nil {
		fmt.Println(err)
		tx.Rollback()
		return err
	}

	for _, permissionID := range rolePermissionRequest.SelectedPermissions {
		roleHasPermission := model.RoleHasPermission{PermissionID: permissionID, RoleID: role.ID}
		if err := tx.Create(&roleHasPermission).Error; err != nil {
			fmt.Println(err)
			tx.Rollback()
			return err
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}

	// Set cache ใหม่หลังจากสร้าง role สำเร็จ
	// ctx := context.Background()
	// client := redis.NewClient(&redis.Options{
	// 	Addr:     "redis:6379",
	// 	Password: "",
	// 	DB:       0,
	// })

	// if err := helper.SetCacheMenu(ctx, client, int(role.ID)); err != nil {
	// 	log.Println("Error setting cache:", err)
	// }

	return c.JSON("successfully")
}

type RolePermissionEditRequest struct {
	RoleName            string `json:"roleName"`
	SelectedPermissions []uint `json:"selectedPermissions"`
	UUID                string `json:"uuid"`
}

func Update(c *fiber.Ctx) error {
	var rolePermissionRequest RolePermissionEditRequest
	if err := c.BodyParser(&rolePermissionRequest); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request payload",
		})
	}

	tx := db.Db.Begin()

	// ลบข้อมูลใน RoleHasPermission ที่เกี่ยวข้องกับ uuid
	if err := tx.Exec("DELETE FROM role_has_permissions WHERE role_id IN (SELECT id FROM roles WHERE uuid = ?)", rolePermissionRequest.UUID).Error; err != nil {
		tx.Rollback()
		return err
	}

	// อัปเดตข้อมูลในตาราง roles
	if err := tx.Exec("UPDATE roles SET name = ? WHERE uuid = ?", rolePermissionRequest.RoleName, rolePermissionRequest.UUID).Error; err != nil {
		tx.Rollback()
		return err
	}

	var updatedRole model.Role
	if err := tx.Model(&model.Role{}).Where("uuid = ?", rolePermissionRequest.UUID).First(&updatedRole).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, permissionID := range rolePermissionRequest.SelectedPermissions {
		roleHasPermission := model.RoleHasPermission{PermissionID: permissionID, RoleID: updatedRole.ID}
		if err := tx.Create(&roleHasPermission).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}

	// Set cache ใหม่หลังจากอัปเดต role สำเร็จ
	// ctx := context.Background()
	// client := redis.NewClient(&redis.Options{
	// 	Addr:     "redis:6379",
	// 	Password: "",
	// 	DB:       0,
	// })

	// if err := helper.SetCacheMenu(ctx, client, int(updatedRole.ID)); err != nil {
	// 	log.Println("Error setting cache:", err)
	// }

	return c.JSON("successfully")
}

func Edit(c *fiber.Ctx) error {
	fmt.Println("hello Edit")

	uuid := c.Params("uuid")
	var rolePermissions []uint
	// rolepermission := db.Db.
	// 	Joins("right join role_has_permissions on roles.id = role_has_permissions.role_id").
	// 	Joins("left join permissions on role_has_permissions.permission_id = permissions.id").
	// 	Where("roles.uuid = ?", uuid).Find(&role)

	// db.Db.Preload("RoleHasPermissions.Permission").Where("uuid = ?", uuid).Find(&role)

	db.Db.Table("role_has_permissions").
		Joins("left join roles on role_has_permissions.role_id = roles.id").
		Where("roles.uuid = ?", uuid).
		Pluck("role_has_permissions.permission_id", &rolePermissions)

	var mainMenus []model.Menu

	db.Db.
		Where("is_active = ?", true).
		Where("level = ?", 0).
		// Order("order asc").
		Find(&mainMenus)

	menus := make([]map[string]interface{}, len(mainMenus))

	for key, mainMenu := range mainMenus {
		menu := make(map[string]interface{})
		menu["id"] = mainMenu.ID
		menu["name"] = mainMenu.Name
		menu["route"] = mainMenu.Route
		menu["level"] = mainMenu.Level
		menu["relation"] = mainMenu.Relation
		menu["order"] = mainMenu.Order
		menu["has_sub"] = mainMenu.HasSub
		menu["icon"] = mainMenu.Icon
		menu["is_active"] = mainMenu.IsActive

		if mainMenu.HasSub != 0 {
			level := mainMenu.Level + 1
			subMenus := getSubMenu(mainMenu.ID, level)
			menu["sub"] = subMenus
		}

		permissionMenu := getPermissionMenu(mainMenu.ID)
		menu["permission"] = permissionMenu
		menus[key] = menu
	}

	var role model.Role
	db.Db.Where("roles.uuid = ?", uuid).Find(&role)
	return c.JSON(fiber.Map{
		"menus":           menus,
		"rolePermissions": rolePermissions,
		"role":            role,
	})
}

func Delete(c *fiber.Ctx) error {
	uuid := c.Params("uuid")

	// ตรวจสอบว่ามีการใช้งาน division_id ในตาราง Visitation หรือไม่ และ deleted_at ต้องเป็น NULL
	var userCount int64
	if err := db.Db.Model(&model.User{}).
		Where("role_id = ? AND deleted_at IS NULL", uuid).
		Count(&userCount).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to check users records"})
	}

	// ถ้ามีการใช้งาน division_id ใน Visitation จะไม่อนุญาตให้ลบ
	if userCount > 0 {
		return c.Status(400).JSON(fiber.Map{"error": "ไม่สามารถลบได้ เนื่องจากมีข้อมูลการใช้บริการของสาขานี้แล้ว"})
	}

	// ดึงข้อมูล division เพื่อทำการ soft delete
	var role model.Role
	if err := db.Db.Where("uuid = ?", uuid).First(&role).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "role not found"})
	}

	// ทำการ Soft Delete โดยใช้ GORM
	if err := db.Db.Delete(&role).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to soft delete role"})
	}

	return c.Status(200).JSON(fiber.Map{
		"message": "success",
	})
}
