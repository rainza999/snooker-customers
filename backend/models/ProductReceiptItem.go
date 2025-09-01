package model

import "gorm.io/gorm"

type ProductReceiptItem struct {
	gorm.Model
	ReceiptID         uint    `gorm:"not null"`           // เชื่อมกับ ProductReceipt
	ProductID         uint    `gorm:"not null"`           // รหัสสินค้า
	Quantity          int     `gorm:"not null"`           // จำนวนที่รับเข้า
	UnitPrice         float64 `gorm:"type:decimal(10,2)"` // ราคาต่อหน่วย
	TotalPrice        float64 `gorm:"type:decimal(10,2)"` // ราคารวม
	IsActive          uint8   `gorm:"default:1"`
	ReceiptItemStatus string  `gorm:"type:varchar(50);default:'draft'"` // สถานะ (draft, saved)
}
