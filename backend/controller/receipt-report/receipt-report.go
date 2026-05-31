package receiptreport

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

type ProductReceiptWithTotal struct {
	model.ProductReceipt
	SumTotalPrice float64 `json:"sum_total_price"`
}

func AnyData(c *fiber.Ctx) error {
	fmt.Println("hello AnyData")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" || endDate == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "start_date and end_date are required"})
	}

	layout := "2006-01-02 15:04:05"
	start, err := time.Parse(layout, startDate+" 00:00:00")
	end, err2 := time.Parse(layout, endDate+" 23:59:59")

	if err != nil || err2 != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid date format"})
	}

	var lists []model.ProductReceipt

	result := db.Db.Preload("ProductItems", "deleted_at IS NULL").
		Preload("Supplier").
		Where("receipt_status = ?", "save").
		Where("received_date BETWEEN ? AND ?", start, end).
		Where("deleted_at IS NULL").Find(&lists)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	// คำนวณยอดรวมของ ProductItems สำหรับแต่ละใบเสร็จ
	var resultList []ProductReceiptWithTotal
	for _, receipt := range lists {
		var sumTotalPrice float64
		for _, item := range receipt.ProductItems {
			sumTotalPrice += item.TotalPrice // สมมติว่า TotalPrice เป็นฟิลด์ใน ProductItems
		}
		resultList = append(resultList, ProductReceiptWithTotal{
			ProductReceipt: receipt,
			SumTotalPrice:  sumTotalPrice,
		})
	}
	return c.JSON(resultList)
}

func EditView(c *fiber.Ctx) error {
	id := c.Params("id")

	return c.JSON(fiber.Map{
		"message": "success EditView",
		"id":      id,
	})
}

func Update(c *fiber.Ctx) error {
	id := c.Params("id")

	return c.JSON(fiber.Map{
		"message": "success Update",
		"id":      id,
	})
}

func Delete(c *fiber.Ctx) error {
	id := c.Params("id")

	return c.JSON(fiber.Map{
		"message": "success Delete",
		"id":      id,
	})
}

func SupplierUpdate(c *fiber.Ctx) error {
	productReceiptReportID := c.Params("id")

	var data map[string]interface{}

	if err := c.BodyParser(&data); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}
	// ดึงค่า supplier_id จาก JSON payload
	supplierIDValue, ok := data["receipts"].(float64) // JSON number เป็น float64 เสมอ
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid supplier_id format"})
	}

	supplierID := uint(supplierIDValue) // แปลง float64 เป็น int

	// อัปเดต supplier_id ใน table product_receipts
	var productReceipt model.ProductReceipt
	if err := db.Db.First(&productReceipt, productReceiptReportID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "ProductReceipt not found"})
	}

	if productReceipt.ReceiptStatus != "draft" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Cannot update supplier because this receipt has already been saved",
		})
	}

	productReceipt.SupplierID = supplierID

	// บันทึกข้อมูลที่อัปเดตลงในฐานข้อมูล
	if err := db.Db.Save(&productReceipt).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update ProductReceipt"})
	}
	// result := db.Db.Model(&ProductReceipt{}).
	//     Where("id = ?", productReceiptReportID).
	//     Update("supplier_id", supplierID)

	// if result.Error != nil {
	//     return c.Status(500).JSON(fiber.Map{"error": "Failed to update supplier_id"})
	// }

	fmt.Println("Updated supplier_id:", supplierID, "for product_receipt ID:", productReceiptReportID)

	return c.JSON(fiber.Map{
		"message":                "successfully SupplierUpdate",
		"productReceiptReportID": productReceiptReportID,
		"receipts":               supplierID, // ส่งค่ากลับเพื่อเช็ค
	})
}

func legacySubmitDraft(c *fiber.Ctx) error {
	productReceiptReportID := c.Params("id")

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
	// ดึงข้อมูลจาก Payload
	drafts := payload.Drafts
	if drafts.SupplierID == 0 || drafts.ProductID == 0 || drafts.PurchaseOrderNumber == "" || drafts.Status == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid data, please check the input values"})
	}

	// // คำนวณราคาต่อหน่วย
	unitPrice := drafts.TotalPrice / float64(drafts.Quantity)

	productReceiptReportIDUint64, err := strconv.ParseUint(productReceiptReportID, 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid productReceiptReportID format"})
	}
	productReceiptReportIDUint := uint(productReceiptReportIDUint64)

	var productReceipt model.ProductReceipt
	if err := db.Db.First(&productReceipt, productReceiptReportIDUint).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "ProductReceipt not found"})
	}

	if productReceipt.ReceiptStatus != "draft" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Cannot add or update items because this receipt has already been saved",
		})
	}

	var existingItem model.ProductReceiptItem

	if err := db.Db.Where("receipt_id = ? AND product_id = ?", productReceiptReportID, drafts.ProductID).First(&existingItem).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// หากไม่มีรายการเดิม ให้สร้างใหม่
			productReceiptItem := model.ProductReceiptItem{
				ReceiptID:         productReceiptReportIDUint,
				ProductID:         drafts.ProductID,
				Quantity:          drafts.Quantity,
				UnitPrice:         unitPrice,
				TotalPrice:        drafts.TotalPrice,
				ReceiptItemStatus: "save",
				IsActive:          1,
			}

			// บันทึก ProductReceiptItem ลงในฐานข้อมูล
			if err := db.Db.Create(&productReceiptItem).Error; err != nil {
				log.Printf("Error creating ProductReceiptItem: %v", err)
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create receipt item"})
			}

			// 3. 🔍 ตรวจสอบว่า GORM เซ็ต ID แล้วหรือยัง
			if productReceiptItem.ID == 0 {
				log.Println("❌ GORM did not set ProductReceiptItem.ID")
				return c.Status(500).JSON(fiber.Map{"error": "Could not get inserted item ID"})
			}

			// เพิ่มข้อมูลลง StockEntry
			stockEntry := model.StockEntry{
				ProductID:            productReceiptItem.ProductID,
				StockLocationID:      1,                                                                    // กำหนด StockLocationID เป็น 1
				Quantity:             productReceiptItem.Quantity,                                          // ดึงจาก quantity
				RemainingQty:         productReceiptItem.Quantity,                                          // RemainingQty เท่ากับ quantity
				CostPerUnit:          productReceiptItem.TotalPrice / float64(productReceiptItem.Quantity), // ราคาต่อหน่วย
				ProductReceiptItemID: &productReceiptItem.ID,                                               // เพิ่มคอลัมน์ ProductReceiptItemID
				EntryDate:            time.Now(),
			}

			// บันทึก StockEntry ลงฐานข้อมูล
			if err := db.Db.Create(&stockEntry).Error; err != nil {
				log.Printf("Error creating StockEntry: %v", err)
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to save stock entry",
				})
			}

			type ProductReceiptItemResponse struct {
				ID                uint       `json:"id"`
				ReceiptID         uint       `json:"receipt_id"`
				ProductID         uint       `json:"product_id"`
				Quantity          int        `json:"quantity"`
				UnitPrice         float64    `json:"unit_price"`
				TotalPrice        float64    `json:"total_price"`
				ReceiptItemStatus string     `json:"receipt_item_status"`
				IsActive          uint8      `json:"is_active"`
				RemainingQuantity int        `json:"remaining_qty"`
				CreatedAt         time.Time  `json:"created_at"`
				UpdatedAt         time.Time  `json:"updated_at"`
				DeletedAt         *time.Time `json:"deleted_at,omitempty"` // สามารถเป็น NULL ได้
			}

			var productReceiptItemX ProductReceiptItemResponse

			err := db.Db.Table("product_receipt_items").
				Joins(`
					LEFT JOIN (
						SELECT product_receipt_item_id, SUM(remaining_qty) AS remaining_quantity
						FROM stock_entries
						WHERE deleted_at IS NULL
						GROUP BY product_receipt_item_id
					) stock_totals ON product_receipt_items.id = stock_totals.product_receipt_item_id
				`).
				Select("product_receipt_items.id, product_receipt_items.receipt_id, product_receipt_items.product_id, "+
					"product_receipt_items.quantity, product_receipt_items.unit_price, product_receipt_items.total_price, "+
					"product_receipt_items.receipt_item_status, product_receipt_items.is_active, "+
					"COALESCE(stock_totals.remaining_quantity, 0) AS remaining_quantity, "+
					"product_receipt_items.created_at, product_receipt_items.updated_at, product_receipt_items.deleted_at").
				// Where("product_receipt_items.receipt_id = ? AND product_receipt_items.product_id = ?", productReceiptReportID, drafts.ProductID).
				Where("product_receipt_items.receipt_id = ? AND product_receipt_items.id = ?", productReceiptReportID, productReceiptItem.ID).
				First(&productReceiptItemX).Error

			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Error fetching data"})
			}

			return c.JSON(fiber.Map{
				"message": "success",
				"items":   productReceiptItemX,
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
	var existingStockEntry model.StockEntry

	//กรณีที่ไม่มีข้อมูลใน StockEntry
	if err := db.Db.Where("product_receipt_item_id = ?", existingItem.ID).First(&existingStockEntry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(500).JSON(fiber.Map{"error": "Error checking receipt item"})
		} else {
			log.Printf("Error checking existing ProductReceiptItem: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "Error checking receipt item"})
		}
	}

	if existingItem.Quantity < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "จำนวนไม่สามารถน้อยกว่าศูนย์ได้"})
	}

	if existingItem.TotalPrice < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "ราคารวมไม่สามารถน้อยกว่าศูนย์ได้"})
	}

	existingStockEntry.Quantity += drafts.Quantity
	existingStockEntry.RemainingQty += drafts.Quantity
	existingStockEntry.CostPerUnit = existingItem.TotalPrice / float64(existingItem.Quantity)

	if existingStockEntry.Quantity < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "จำนวนในStockไม่สามารถน้อยกว่าศูนย์ได้"})
	}

	if existingStockEntry.RemainingQty < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "จำนวนคงเหลือStockไม่สามารถน้อยกว่าศูนย์ได้"})
	}
	// บันทึกการอัปเดตลงในฐานข้อมูล
	if err := db.Db.Save(&existingItem).Error; err != nil {
		log.Printf("Error updating ProductReceiptItem: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update receipt item"})
	}

	// บันทึกการอัปเดตลงในฐานข้อมูล
	if err := db.Db.Save(&existingStockEntry).Error; err != nil {
		log.Printf("Error updating existingStockEntry: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update existingStockEntry"})
	}

	type ProductReceiptItemResponse struct {
		ID                uint       `json:"id"`
		ReceiptID         uint       `json:"receipt_id"`
		ProductID         uint       `json:"product_id"`
		Quantity          int        `json:"quantity"`
		UnitPrice         float64    `json:"unit_price"`
		TotalPrice        float64    `json:"total_price"`
		ReceiptItemStatus string     `json:"receipt_item_status"`
		IsActive          uint8      `json:"is_active"`
		RemainingQuantity int        `json:"remaining_qty"`
		CreatedAt         time.Time  `json:"created_at"`
		UpdatedAt         time.Time  `json:"updated_at"`
		DeletedAt         *time.Time `json:"deleted_at,omitempty"`
	}

	var productReceiptItemX ProductReceiptItemResponse

	err2 := db.Db.Table("product_receipt_items").
		Joins(`
			LEFT JOIN (
				SELECT product_receipt_item_id, SUM(remaining_qty) AS remaining_quantity
				FROM stock_entries
				WHERE deleted_at IS NULL
				GROUP BY product_receipt_item_id
			) stock_totals ON product_receipt_items.id = stock_totals.product_receipt_item_id
		`).
		Select("product_receipt_items.id, product_receipt_items.receipt_id, product_receipt_items.product_id, "+
			"product_receipt_items.quantity, product_receipt_items.unit_price, product_receipt_items.total_price, "+
			"product_receipt_items.receipt_item_status, product_receipt_items.is_active, "+
			"COALESCE(stock_totals.remaining_quantity, 0) AS remaining_quantity, "+
			"product_receipt_items.created_at, product_receipt_items.updated_at, product_receipt_items.deleted_at").
		Where("product_receipt_items.receipt_id = ? AND product_receipt_items.id = ?", productReceiptReportID, existingItem.ID).
		First(&productReceiptItemX).Error

	if err2 != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error fetching data"})
	}
	// ส่ง JSON ตอบกลับ
	return c.JSON(fiber.Map{
		"message": "success",
		"items":   productReceiptItemX,
	})
}
