package settingtable

import (
	// "github.com/dgrijalva/jwt-go"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"github.com/rainza999/fiber-test/realtime"
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

	divisionID, err := currentUserDivisionID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	result := db.Db.
		Where("division_id = ?", divisionID).
		Order("CASE WHEN sort_order IS NULL OR sort_order = 0 THEN id ELSE sort_order END ASC").
		Order("id ASC").
		Find(&lists)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	return c.JSON(lists)
}

type StoreBody struct {
	Name          string  `json:"nameTable"`
	Type          uint8   `json:"typeTable"`
	Price         float64 `json:"price"`
	Price2        float64 `json:"price2"`
	Relay         uint8   `json:"relayNumber"` // เพิ่มฟิลด์ relay
	Address       string  `json:"address"`
	RelayDisabled bool    `json:"relayDisabled"`
	NoRelay       bool    `json:"noRelay"`
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

	divisionID, err := currentUserDivisionID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	var maxSortOrder int
	db.Db.Model(&model.SettingTable{}).
		Where("division_id = ?", divisionID).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxSortOrder)

	relay, address, err := normalizeRelayConfig(json.Relay, json.Address, json.RelayDisabled || json.NoRelay)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	var settingTable = model.SettingTable{
		DivisionID: divisionID,
		Code:       "xxx",
		Name:       json.Name,
		Ma:         1,
		Type:       json.Type,
		Status:     "active",
		Price:      json.Price,
		Price2:     json.Price2,
		Relay:      json.Relay, // เพิ่ม relay ที่ได้รับจาก body
		Address:    json.Address,
		SortOrder:  maxSortOrder + 1,
	}

	settingTable.Relay = relay
	settingTable.Address = address

	db.Db.Create(&settingTable)
	return c.JSON(fiber.Map{"message": "success"})
}

func Edit(c *fiber.Ctx) error {
	fmt.Println("hello Edit")
	var settingTable model.SettingTable

	divisionID, err := currentUserDivisionID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	result := db.Db.Where("id = ? AND division_id = ?", c.Params("id"), divisionID).First(&settingTable)

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
	Name          string  `json:"nameTable"`
	Type          uint8   `json:"typeTable"`
	Price         float64 `json:"price"`
	Price2        float64 `json:"price2"`
	Relay         uint8   `json:"relay"`
	RelayNumber   uint8   `json:"relayNumber"`
	Address       string  `json:"address"`
	RelayDisabled bool    `json:"relayDisabled"`
	NoRelay       bool    `json:"noRelay"`
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
	relay := json.Relay
	if relay == 0 && json.RelayNumber > 0 {
		relay = json.RelayNumber
	}
	relay, address, err := normalizeRelayConfig(relay, json.Address, json.RelayDisabled || json.NoRelay)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// ค้นหา settingTable ตาม id
	var settingTable model.SettingTable
	divisionID, err := currentUserDivisionID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	if err := db.Db.Where("id = ? AND division_id = ?", c.Params("id"), divisionID).First(&settingTable).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "SettingTable not found",
		})
	}

	// อัปเดตข้อมูลใน settingTable
	settingTable.Name = json.Name
	settingTable.Type = json.Type
	settingTable.Price = price
	settingTable.Price2 = price2
	settingTable.Relay = relay
	settingTable.Address = address
	if err := db.Db.Save(&settingTable).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Internal server error",
		})
	}

	return c.JSON(fiber.Map{"message": "success"})
}

type ReorderItem struct {
	ID        uint `json:"id"`
	SortOrder int  `json:"sort_order"`
}

type ReorderBody struct {
	Items []ReorderItem `json:"items"`
}

func normalizeRelayConfig(relay uint8, address string, disabled bool) (uint8, string, error) {
	if disabled || relay == 0 {
		return 0, "", nil
	}
	if relay > 8 {
		return 0, "", fmt.Errorf("relay number must be 1-8 or disabled")
	}
	return relay, strings.TrimSpace(address), nil
}

func currentUserDivisionID(c *fiber.Ctx) (uint, error) {
	userID, ok := c.Locals("userID").(int)
	if !ok {
		return 0, fmt.Errorf("missing user context")
	}

	var user model.User
	if err := db.Db.First(&user, userID).Error; err != nil {
		return 0, err
	}

	return user.DivisionID, nil
}

func Reorder(c *fiber.Ctx) error {
	var json ReorderBody

	if err := c.BodyParser(&json); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if len(json.Items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "No table order items provided"})
	}

	divisionID, err := currentUserDivisionID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	seen := make(map[uint]struct{}, len(json.Items))
	for i, item := range json.Items {
		if item.ID == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid table id"})
		}
		if _, exists := seen[item.ID]; exists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Duplicate table id"})
		}
		seen[item.ID] = struct{}{}
		if json.Items[i].SortOrder <= 0 {
			json.Items[i].SortOrder = i + 1
		}
	}

	err = db.Db.Transaction(func(tx *gorm.DB) error {
		for _, item := range json.Items {
			result := tx.Model(&model.SettingTable{}).
				Where("id = ? AND division_id = ?", item.ID, divisionID).
				Update("sort_order", item.SortOrder)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("table id %d not found in current division", item.ID)
			}
		}
		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	realtime.PublishPOS(divisionID, "table-order-updated")

	return c.JSON(fiber.Map{"message": "success"})
}
