package model

import (
	"time"

	"gorm.io/gorm"
)

type StockEntry struct {
	ID                   uint      `gorm:"primaryKey"`
	ProductID            uint      `gorm:"not null"`           // อ้างถึงสินค้า
	StockLocationID      uint      `gorm:"not null"`           // อ้างถึงสถานที่เก็บ
	Quantity             int       `gorm:"not null"`           // จำนวนสินค้าที่รับเข้ามา
	RemainingQty         int       `gorm:"not null;default:0"` // จำนวนคงเหลือในแต่ละล็อต
	CostPerUnit          float64   `gorm:"not null"`           // ต้นทุนต่อหน่วยของสินค้า
	EntryDate            time.Time `gorm:"autoCreateTime"`     // วันที่รับเข้ามา
	ProductReceiptItemID *uint     `gorm:"index"`              // เชื่อมกับ product_receipt_items
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            gorm.DeletedAt `gorm:"index"`
}
