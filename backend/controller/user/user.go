package user

import (
	// "github.com/dgrijalva/jwt-go"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"

	AuthController "github.com/rainza999/fiber-test/controller/auth"
)

type UserBody struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	// Password string `json:"password"`
	Fullname string `json:"fullname"`
}

// func (UserBody) TableName() string {
// 	return "users"
// }

func AnyData(c *fiber.Ctx) error {

	fmt.Println("hello AnyData")
	// var users []UserBody
	var users []model.User

	// result := db.Db.Find(&users)
	// result := db.Db.Preload("Employee").Find(&users)
	result := db.Db.Preload("Employee").Find(&users)
	// result := db.Db.Joins("Employee").Find(&users)

	if result.Error != nil {
		// หากเกิดข้อผิดพลาดในการดึงข้อมูล
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	// return c.JSON(fiber.Map{"message": "success"})
	return c.JSON(users)
}

type StoreBody struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	Division  int8   `json:"division"`
	Email     string `json:"email"`
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
	NickName  string `json:"nickname"`
	Phone     string `json:"telephone"`
	Role      uint   `json:"role"`
}

func Store(c *fiber.Ctx) error {
	fmt.Println("hello store")

	var json StoreBody

	if err := c.BodyParser(&json); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	employeesAndUsers := []struct {
		Employee model.Employee
		User     model.User
	}{
		{
			Employee: model.Employee{
				FirstName: json.FirstName, LastName: json.LastName, NickName: json.NickName, Email: json.Email,
				Telephone: json.Phone, DateOfJoining: time.Now().UTC().Truncate(24 * time.Hour), Status: "active",
			},
			User: model.User{
				Username: json.Username, Password: json.Password, DivisionID: uint(json.Division), RoleID: json.Role,
			},
		},
	}

	for _, item := range employeesAndUsers {
		// Hash the password before creating the user
		hashedPassword, err := AuthController.HashPassword(item.User.Password)
		if err != nil {
			log.Fatal(err)
		}
		item.User.Password = hashedPassword

		// Create employee
		db.Db.Create(&item.Employee)

		// Create associated user
		item.User.EmployeeID = item.Employee.ID
		db.Db.Create(&item.User)
	}

	return c.JSON(fiber.Map{"message": "success"})
}

func Edit(c *fiber.Ctx) error {
	fmt.Println("hello Edit")
	var user model.User

	result := db.Db.Preload("Employee").
		Preload("Division").
		Preload("Role").
		Joins("JOIN employees ON users.employee_id = employees.id").
		Where("employees.uuid = ?", c.Params("id")).First(&user)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	return c.JSON(user)
}

type UpdateBody struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password"`
	Division  int8   `json:"division"`
	Email     string `json:"email"`
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
	NickName  string `json:"nickname"`
	Phone     string `json:"telephone"`
	IsActive  uint8  `json:"isActive"`
	Role      uint   `json:"role"`
}

func Update(c *fiber.Ctx) error {
	fmt.Println("hello update")
	var json UpdateBody
	if err := c.BodyParser(&json); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	fmt.Println(uint8(json.IsActive))

	employeesAndUsers := []struct {
		Employee model.Employee
		User     model.User
	}{
		{
			Employee: model.Employee{
				FirstName: json.FirstName, LastName: json.LastName, NickName: json.NickName, Email: json.Email, Uuid: c.Params("uuid"),
				Telephone: json.Phone, Status: "active",
			},
			User: model.User{
				Username: json.Username, DivisionID: uint(json.Division), RoleID: uint(json.Role), IsActive: json.IsActive,
			},
		},
	}

	log.Println("hello password")
	log.Println(json.Password)

	for _, item := range employeesAndUsers {
		// Check if the employee with the given UUID exists
		existingEmployee := model.Employee{}
		if err := db.Db.Where("uuid = ?", item.Employee.Uuid).First(&existingEmployee).Error; err != nil {
			log.Println(err)
			return c.Status(500).JSON(fiber.Map{
				"error": "Internal server error",
			})
		}

		// Update the existing employee with the new data
		db.Db.Model(&existingEmployee).Updates(item.Employee)
		if json.Password != "" {
			hashedPassword, err := AuthController.HashPassword(json.Password)
			if err != nil {
				log.Println(err)
				return c.Status(500).JSON(fiber.Map{
					"error": "Internal server error",
				})
			}
			item.User.Password = hashedPassword
		}
		log.Println(item.User.Password)
		log.Println(item.User)
		// db.Db.Model(&model.User{}).Where("employee_id = ?", existingEmployee.ID).Select("is_active").Updates(item.User)
		db.Db.Model(&model.User{}).Where("employee_id = ?", existingEmployee.ID).Select("username", "division_id", "role_id", "is_active").Updates(item.User)

		// db.Db.Where("employees.uuid = ?", c.Params("uuid")).Save(&item.Employee)
		// item.User.EmployeeID = item.Employee.ID
		// db.Db.Save(&item.User)

	}

	return c.JSON(fiber.Map{"message": "success"})
}

func Delete(c *fiber.Ctx) error {
	uuid := c.Params("uuid")

	// ตรวจสอบว่ามีการใช้งาน division_id ในตาราง Visitation หรือไม่ และ deleted_at ต้องเป็น NULL
	// var userCount int64
	// if err := db.Db.Model(&model.User{}).
	// 	Where("role_id = ? AND deleted_at IS NULL", uuid).
	// 	Count(&userCount).Error; err != nil {
	// 	return c.Status(500).JSON(fiber.Map{"error": "Failed to check users records"})
	// }

	// if userCount > 0 {
	// 	return c.Status(400).JSON(fiber.Map{"error": "ไม่สามารถลบได้ เนื่องจากมีข้อมูลการใช้บริการของสาขานี้แล้ว"})
	// }

	var user model.User

	if err := db.Db.Preload("Employee").
		Preload("Division").
		Preload("Role").
		Joins("JOIN employees ON users.employee_id = employees.id").
		Where("employees.uuid = ?", uuid).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "role not found"})
	}
	// if err := db.Db.Where("uuid = ?", uuid).First(&user).Error; err != nil {
	// 	return c.Status(404).JSON(fiber.Map{"error": "role not found"})
	// }

	// ทำการ Soft Delete โดยใช้ GORM
	if err := db.Db.Delete(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to soft delete role"})
	}

	return c.Status(200).JSON(fiber.Map{
		"message": "success",
	})
}
