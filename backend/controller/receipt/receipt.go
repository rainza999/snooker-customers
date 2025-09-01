package receipt

import (
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

func DeleteReceipt(c *fiber.Ctx) error {
	// Struct สำหรับ JSON Payload
	id := c.Params("id")
	// Debug Logs เพื่อตรวจสอบค่า
	log.Printf("Payload Received: %+v", id)
	// แปลง JSON Payload เป็น Struct
	var product_receipt_item model.ProductReceiptItem
	if err := db.Db.First(&product_receipt_item, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "product_receipt_item not found"})
	}

	// ทำการ Soft Delete โดยใช้ GORM
	if err := db.Db.Delete(&product_receipt_item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to soft delete product_receipt_item"})
	}
	return c.JSON(fiber.Map{
		"message": "success",
	})

}

func EditReceipt(c *fiber.Ctx) error {
	var drafts []struct {
		ID                   uint    `json:"id"`
		Supplier             uint    `json:"supplier"`
		Product              uint    `json:"product"`
		Quantity             int     `json:"quantity"`
		TotalPrice           float64 `json:"totalPrice"`
		PurchaseOrderNumber  string  `json:"purchaseOrderNumber"`
		Status               string  `json:"status"`
		ProductReceiptItemID uint    `json:"product_receipt_item_id"`
		RemainingQuantity    int     `json:"remaining_quantity"`
	}
	id := c.Params("id")
	if err := db.Db.Debug().Table("product_receipts").
		Select(`
			product_receipts.id, 
			product_receipts.supplier_id AS supplier, 
			product_receipt_items.product_id AS product, 
			product_receipt_items.quantity, 
			product_receipt_items.total_price, 
			product_receipt_items.id AS product_receipt_item_id,
			stock_entries.remaining_qty AS remaining_quantity,
			product_receipts.receipt_number AS purchase_order_number, 
			product_receipts.receipt_status AS status
		`).
		Joins("JOIN product_receipt_items ON product_receipts.id = product_receipt_items.receipt_id").
		Joins("JOIN stock_entries ON product_receipt_items.id = stock_entries.product_receipt_item_id").
		Where("product_receipts.id = ?", id).
		// Where("product_receipts.receipt_status = ?", "draft").
		Find(&drafts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch draft receipts"})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"data":    drafts,
	})
}

func DraftReceipt(c *fiber.Ctx) error {
	var drafts []struct {
		ID                  uint    `json:"id"`
		Supplier            uint    `json:"supplier"`
		Product             uint    `json:"product"`
		Quantity            int     `json:"quantity"`
		TotalPrice          float64 `json:"totalPrice"`
		PurchaseOrderNumber string  `json:"purchaseOrderNumber"`
		Status              string  `json:"status"`
	}

	if err := db.Db.Debug().Table("product_receipts").
		Select(`
			product_receipts.id, 
			product_receipts.supplier_id AS supplier, 
			product_receipt_items.product_id AS product, 
			product_receipt_items.quantity, 
			product_receipt_items.total_price, 
			product_receipts.receipt_number AS purchase_order_number, 
			product_receipts.receipt_status AS status
		`).
		Joins("JOIN product_receipt_items ON product_receipts.id = product_receipt_items.receipt_id").
		Where("product_receipts.receipt_status = ?", "draft").
		Find(&drafts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch draft receipts"})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"data":    drafts,
	})
}

func SubmitReceipt(c *fiber.Ctx) error {
	// Struct สำหรับ JSON Payload
	var payload struct {
		Drafts struct {
			SupplierID          uint    `json:"supplier"`
			ProductID           uint    `json:"product"`
			Quantity            int     `json:"quantity"`
			TotalPrice          float64 `json:"totalPrice"`
			PurchaseOrderNumber string  `json:"purchaseOrderNumber"`
			Status              string  `json:"status"`
		} `json:"drafts"`
	}

	// แปลง JSON Payload เป็น Struct
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Debug Logs เพื่อตรวจสอบค่า
	log.Printf("Payload Received: %+v", payload)

	// ดึงข้อมูลจาก Payload
	drafts := payload.Drafts
	if drafts.SupplierID == 0 || drafts.ProductID == 0 || drafts.Quantity <= 0 || drafts.TotalPrice <= 0 || drafts.PurchaseOrderNumber == "" || drafts.Status == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid data, please check the input values"})
	}

	// แปลง TotalPrice จาก string เป็น float64
	// totalPrice, err := strconv.ParseFloat(drafts.TotalPrice, 64)
	// if err != nil {
	// 	return c.Status(400).JSON(fiber.Map{"error": "Invalid totalPrice value"})
	// }

	// คำนวณราคาต่อหน่วย
	unitPrice := drafts.TotalPrice / float64(drafts.Quantity)

	// ตรวจสอบว่ามี ProductReceipt ที่ใช้ PurchaseOrderNumber นี้อยู่หรือไม่
	var existingReceipt model.ProductReceipt
	if err := db.Db.Where("receipt_number = ?", drafts.PurchaseOrderNumber).Where("supplier_id = ?", drafts.SupplierID).First(&existingReceipt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// หากไม่มีใบเสร็จในฐานข้อมูล ให้สร้างใหม่
			productReceipt := model.ProductReceipt{
				SupplierID:    drafts.SupplierID,
				ReceiptNumber: drafts.PurchaseOrderNumber,
				ReceivedDate:  time.Now(),
				ReceiptStatus: drafts.Status,
				IsActive:      1,
			}

			// บันทึก ProductReceipt ลงในฐานข้อมูล
			if err := db.Db.Create(&productReceipt).Error; err != nil {
				log.Printf("Error creating ProductReceipt: %v", err)
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create receipt"})
			}

			existingReceipt = productReceipt
		} else {
			// หากเกิดข้อผิดพลาดอื่นๆ
			log.Printf("Error checking existing ProductReceipt: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "Error checking receipt"})
		}
	}

	var existingItem model.ProductReceiptItem
	if err := db.Db.Where("receipt_id = ? AND product_id = ?", existingReceipt.ID, drafts.ProductID).First(&existingItem).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// หากไม่มีรายการเดิม ให้สร้างใหม่
			productReceiptItem := model.ProductReceiptItem{
				ReceiptID:         existingReceipt.ID,
				ProductID:         drafts.ProductID,
				Quantity:          drafts.Quantity,
				UnitPrice:         unitPrice,
				TotalPrice:        drafts.TotalPrice,
				ReceiptItemStatus: drafts.Status,
				IsActive:          1,
			}

			// บันทึก ProductReceiptItem ลงในฐานข้อมูล
			if err := db.Db.Create(&productReceiptItem).Error; err != nil {
				log.Printf("Error creating ProductReceiptItem: %v", err)
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create receipt item"})
			}

			return c.JSON(fiber.Map{
				"message": "success",
				"receipt": existingReceipt,
				"items":   productReceiptItem,
			})
		} else {
			log.Printf("Error checking existing ProductReceiptItem: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "Error checking receipt item"})
		}
	}

	// หากมีรายการเดิม ให้อัปเดตข้อมูล
	existingItem.Quantity += drafts.Quantity
	existingItem.TotalPrice += drafts.TotalPrice
	existingItem.UnitPrice = existingItem.TotalPrice / float64(existingItem.Quantity)

	// บันทึกการอัปเดตลงในฐานข้อมูล
	if err := db.Db.Save(&existingItem).Error; err != nil {
		log.Printf("Error updating ProductReceiptItem: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update receipt item"})
	}

	// ส่ง JSON ตอบกลับ
	return c.JSON(fiber.Map{
		"message": "success",
		"receipt": existingReceipt,
		"items":   existingItem,
	})
}

func FinalizeReceipt(c *fiber.Ctx) error {

	println("FinalizeReceipt called")
	body := c.Body()
	log.Printf("Raw Body: %s", string(body))

	// แปลงข้อมูล JSON เป็น struct
	var payload struct {
		Receipts []struct {
			SupplierID          uint    `json:"supplier_id"`
			ProductID           uint    `json:"product_id"`
			ReceiptID           uint    `json:"receipt_id"`
			Product             string  `json:"product"`
			Quantity            int     `json:"quantity"`
			TotalPrice          float64 `json:"totalPrice"`
			PurchaseOrderNumber string  `json:"purchaseOrderNumber"`
			ReceiptItemStatus   string  `json:"receipt_item_status"`
		} `json:"receipts"`
	}
	if err := c.BodyParser(&payload); err != nil {
		log.Printf("Error parsing body: %v", err)
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// อัปเดตสถานะใบเสร็จเป็น save
	var receiptX model.ProductReceipt
	// ค้นหาใบเสร็จที่ต้องการ
	err := db.Db.Model(&model.ProductReceipt{}).
		// Where("receipt_number = ? AND supplier_id = ?", payload.Receipts[0].PurchaseOrderNumber, payload.Receipts[0].SupplierID).
		Where("receipt_number = ? AND supplier_id = ? AND id = ?", payload.Receipts[0].PurchaseOrderNumber, payload.Receipts[0].SupplierID, payload.Receipts[0].ReceiptID).
		First(&receiptX).Error
	if err != nil {
		log.Printf("Error fetching receipt: %v", err)
		return c.Status(404).JSON(fiber.Map{
			"error": "Receipt not found",
		})
	}

	// อัปเดตสถานะใบเสร็จ
	err = db.Db.Model(&model.ProductReceipt{}).
		Where("id = ?", receiptX.ID).
		Update("receipt_status", "save").Error
	if err != nil {
		log.Printf("Error updating receipt status: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update receipt status",
		})
	}

	log.Printf("Updated Receipt ID: %d", receiptX.ID)

	for _, receipt := range payload.Receipts {
		// อัปเดตสถานะ receipt_item_status เป็น save ในฐานข้อมูล
		if err := db.Db.Model(&model.ProductReceiptItem{}).
			Where("product_id = ? AND receipt_id = ?", receipt.ProductID, receiptX.ID).
			Update("receipt_item_status", "save").Error; err != nil {
			log.Printf("Error updating receipt status: %v", err)
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to update receipt status",
			})
		}
		var productReceiptItem model.ProductReceiptItem
		err := db.Db.Where("product_id = ? AND receipt_id = ?", receipt.ProductID, receiptX.ID).
			First(&productReceiptItem).Error
		if err != nil {
			log.Printf("❌ Error finding ProductReceiptItem: %v", err)
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to find matching ProductReceiptItem",
			})
		}
		// เพิ่มข้อมูลลง StockEntry
		stockEntry := model.StockEntry{
			ProductID:            receipt.ProductID,
			StockLocationID:      1,                                              // กำหนด StockLocationID เป็น 1
			Quantity:             receipt.Quantity,                               // ดึงจาก quantity
			RemainingQty:         receipt.Quantity,                               // RemainingQty เท่ากับ quantity
			CostPerUnit:          receipt.TotalPrice / float64(receipt.Quantity), // ราคาต่อหน่วย
			ProductReceiptItemID: &productReceiptItem.ID,                         // ✅ ใส่ตรงนี้
			EntryDate:            time.Now(),
		}

		// บันทึก StockEntry ลงฐานข้อมูล
		if err := db.Db.Create(&stockEntry).Error; err != nil {
			log.Printf("Error creating StockEntry: %v", err)
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to save stock entry",
			})
		}
	}

	return c.JSON(fiber.Map{
		"message": "success",
	})
}

func Store(c *fiber.Ctx) error {
	var data map[string]interface{}

	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	supplier := model.Supplier{
		Name:     data["name"].(string),
		Contact:  data["contact"].(string),
		Address:  data["address"].(string),
		IsActive: data["isActive"].(bool), // ตรวจสอบ type ให้ตรงกัน

	}

	if err := db.Db.Create(&supplier).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create product"})
	}

	return c.JSON(fiber.Map{
		"message":  "success",
		"supplier": supplier,
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
	var supplier model.Supplier

	result := db.Db.Where("id = ?", c.Params("id")).First(&supplier)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting table not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	return c.JSON(supplier)
}

func Update(c *fiber.Ctx) error {
	var data map[string]interface{}

	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	id := c.Params("id")
	var supplier model.Supplier
	if err := db.Db.First(&supplier, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "product not found"})
	}

	supplier.Name = data["name"].(string)
	supplier.Contact = data["contact"].(string)
	supplier.Address = data["address"].(string)
	supplier.IsActive = data["isActive"].(bool)

	if err := db.Db.Save(&supplier).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update product"})
	}

	return c.JSON(fiber.Map{
		"message":  "success",
		"supplier": supplier,
	})
}
