package product

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

type ProductRemainInfo struct {
	ID             uint    `json:"id"`
	Name           string  `json:"name"`
	TotalRemaining float64 `json:"total_remaining"`
	TotalValue     float64 `json:"total_value"`
}

func RemainAnyData(c *fiber.Ctx) error {
	fmt.Println("📦 Loading Remaining Products")

	var results []ProductRemainInfo

	query := `
	SELECT 
		p.id,
		p.name,
		COALESCE(SUM(se.remaining_qty), 0) AS total_remaining,
		COALESCE(SUM(se.remaining_qty * se.cost_per_unit), 0) AS total_value
	FROM products p
	LEFT JOIN stock_entries se ON se.product_id = p.id AND se.deleted_at IS NULL
	WHERE p.is_active = 1 and p.category_id != 3 and p.id != 1
	GROUP BY p.id, p.name
	ORDER BY p.id ASC
	`

	if err := db.Db.Raw(query).Scan(&results).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(results)
}

func AnyData(c *fiber.Ctx) error {

	fmt.Println("hello AnyData Product")
	var lists []model.Product

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

	// Handle price
	price, ok := data["price"].(float64)
	if !ok {
		switch v := data["price"].(type) {
		case string:
			parsedPrice, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid price format"})
			}
			price = parsedPrice
		case int:
			price = float64(v)
		case float64:
			price = v
		default:
			return c.Status(400).JSON(fiber.Map{"error": "Invalid price type"})
		}
	}

	// Handle category
	categoryIDFloat, ok := data["category"].(float64)
	if !ok {
		switch v := data["category"].(type) {
		case float64:
			categoryIDFloat = v
		case int:
			categoryIDFloat = float64(v)
		default:
			return c.Status(400).JSON(fiber.Map{"error": "Invalid category type"})
		}
	}
	categoryID := uint(categoryIDFloat)

	// Create Product
	product := model.Product{
		Name:       data["name"].(string),
		Price:      price,
		Unit:       data["unit"].(string),
		CategoryID: categoryID,
		IsActive:   data["isActive"].(bool),
	}

	if err := db.Db.Create(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create product"})
	}

	// ✅ Add StockEntry if CategoryID == 3
	if categoryID == 3 {
		stockEntry := model.StockEntry{
			ProductID:       product.ID,
			StockLocationID: 1,
			Quantity:        99999,
			CostPerUnit:     0,
			EntryDate:       time.Now(),
			RemainingQty:    99999,
		}

		if err := db.Db.Create(&stockEntry).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to add stock entry"})
		}
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"product": product,
	})
}

// func Store(c *fiber.Ctx) error {
// 	var data map[string]interface{}

// 	if err := c.BodyParser(&data); err != nil {
// 		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
// 	}

// 	price, ok := data["price"].(float64)
// 	if !ok {
// 		switch v := data["price"].(type) {
// 		case string:
// 			println("Price is of type string")
// 			parsedPrice, err := strconv.ParseFloat(v, 64)
// 			if err != nil {
// 				return c.Status(400).JSON(fiber.Map{"error": "Invalid price format"})
// 			}
// 			price = parsedPrice
// 		case int:
// 			println("Price is of type int")
// 			price = float64(v)
// 		case float64:
// 			println("Price is already float64")
// 			price = v
// 		default:
// 			println("Price is of an unknown type")
// 			return c.Status(400).JSON(fiber.Map{"error": "Invalid price type"})
// 		}
// 	}

// 	categoryIDFloat, ok := data["category"].(float64)
// 	if !ok {
// 		switch v := data["category"].(type) {
// 		case float64:
// 			println("Category is already float64")
// 			categoryIDFloat = v
// 		case int:
// 			println("Category is of type int")
// 			categoryIDFloat = float64(v)
// 		default:
// 			println("Category is of an unknown type")
// 			return c.Status(400).JSON(fiber.Map{"error": "Invalid category type"})
// 		}
// 	}
// 	categoryID := uint(categoryIDFloat)

// 	product := model.Product{
// 		Name:       data["name"].(string),
// 		Price:      price,
// 		Unit:       data["unit"].(string),
// 		CategoryID: categoryID,
// 		IsActive:   data["isActive"].(bool), // ตรวจสอบ type ให้ตรงกัน

// 	}

// 	if err := db.Db.Create(&product).Error; err != nil {
// 		return c.Status(500).JSON(fiber.Map{"error": "Failed to create product"})
// 	}

// 	// Add stock entry
// 	// stockEntry := model.StockEntry{
// 	// 	ProductID:       product.ID,
// 	// 	StockLocationID: 1,
// 	// 	Quantity:        99999,
// 	// 	CostPerUnit:     0,
// 	// 	EntryDate:       time.Now(),
// 	// 	RemainingQty:    99999,
// 	// }

// 	// if err := db.Db.Create(&stockEntry).Error; err != nil {
// 	// 	return c.Status(500).JSON(fiber.Map{"error": "Failed to add stock entry"})
// 	// }

// 	return c.JSON(fiber.Map{
// 		"message": "success",
// 		"product": product,
// 	})
// }

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
	var product model.Product

	result := db.Db.Where("id = ?", c.Params("id")).First(&product)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting table not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	// ส่งข้อมูล division
	return c.JSON(product)
}

func Update(c *fiber.Ctx) error {
	var data map[string]interface{}

	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	id := c.Params("id")
	var product model.Product
	if err := db.Db.First(&product, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "product not found"})
	}

	product.Name = data["name"].(string)
	product.IsActive = data["isActive"].(bool)
	product.Price = data["price"].(float64)
	product.Unit = data["unit"].(string)
	product.CategoryID = uint(data["category"].(float64))

	if err := db.Db.Save(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update product"})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"product": product,
	})
}
