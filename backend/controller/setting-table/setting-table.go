package settingtable

import (
	// "github.com/dgrijalva/jwt-go"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
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
	var lists []model.SettingTable

	result := db.Db.Find(&lists)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	return c.JSON(lists)
}

type StoreBody struct {
	Name    string  `json:"nameTable"`
	Type    uint8   `json:"typeTable"`
	Price   float64 `json:"price"`
	Price2  float64 `json:"price2"`
	Relay   uint8   `json:"relayNumber"` // เพิ่มฟิลด์ relay
	Address string  `json:"address"`
}

func Store(c *fiber.Ctx) error {
	fmt.Println("hello store")

	var json StoreBody

	if err := c.BodyParser(&json); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	fmt.Printf("Received JSON data: %+v\n", json)

	var settingTable = model.SettingTable{
		Code:    "xxx",
		Name:    json.Name,
		Ma:      1,
		Type:    json.Type,
		Status:  "active",
		Price:   json.Price,
		Price2:  json.Price2,
		Relay:   json.Relay, // เพิ่ม relay ที่ได้รับจาก body
		Address: json.Address,
	}

	db.Db.Create(&settingTable)
	return c.JSON(fiber.Map{"message": "success"})
}

func Edit(c *fiber.Ctx) error {
	fmt.Println("hello Edit")
	var settingTable model.SettingTable

	result := db.Db.Where("id = ?", c.Params("id")).First(&settingTable)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting table not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	// ส่งข้อมูล settingTable กลับไป รวมถึง relay
	return c.JSON(settingTable)
}

type UpdateBody struct {
	Name    string  `json:"nameTable"`
	Type    uint8   `json:"typeTable"`
	Price   float64 `json:"price"`
	Price2  float64 `json:"price2"`
	Relay   uint8   `json:"relay"`
	Address string  `json:"address"`
}

func Update(c *fiber.Ctx) error {
	fmt.Println("hello update")
	var json UpdateBody

	if err := c.BodyParser(&json); err != nil {
		fmt.Println("BodyParser Error:", err)
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// แปลง string เป็น float64
	// price, err := strconv.ParseFloat(json.Price, 64)
	// if err != nil {
	// 	return c.Status(400).JSON(fiber.Map{
	// 		"error": "Invalid price format",
	// 	})
	// }

	// price2, err := strconv.ParseFloat(json.Price2, 64)
	// if err != nil {
	// 	return c.Status(400).JSON(fiber.Map{
	// 		"error": "Invalid price2 format",
	// 	})
	// }

	price := json.Price
	price2 := json.Price2

	// ค้นหา settingTable ตาม id
	var settingTable model.SettingTable
	if err := db.Db.Where("id = ?", c.Params("id")).First(&settingTable).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "SettingTable not found",
		})
	}

	// อัปเดตข้อมูลใน settingTable
	settingTable.Name = json.Name
	settingTable.Type = json.Type
	settingTable.Price = price
	settingTable.Price2 = price2
	settingTable.Relay = json.Relay
	settingTable.Address = json.Address
	if err := db.Db.Save(&settingTable).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Internal server error",
		})
	}

	return c.JSON(fiber.Map{"message": "success"})
}
