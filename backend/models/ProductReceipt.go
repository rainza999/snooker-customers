package model

import (
	"time"

	"gorm.io/gorm"
)

type ProductReceipt struct {
	gorm.Model
	SupplierID    uint                 `gorm:"not null"`                            // Foreign Key ไปยัง Supplier
	Supplier      Supplier             `gorm:"foreignKey:SupplierID;references:ID"` // ความสัมพันธ์กับ Supplier
	ReceiptNumber string               `gorm:"type:varchar(255);not null"`          // เลขที่ใบเสร็จ
	ReceivedDate  time.Time            `gorm:"not null"`                            // วันที่-เวลารับสินค้า
	Notes         string               `gorm:"type:text"`                           // หมายเหตุเพิ่มเติม
	IsActive      uint8                `gorm:"default:1"`                           // สถานะ Active
	ReceiptStatus string               `gorm:"type:varchar(50);default:'draft'"`    // สถานะ (draft, saved)
	ProductItems  []ProductReceiptItem `gorm:"foreignKey:ReceiptID"`                // ความสัมพันธ์กับ ProductReceiptItem
}
