package model

import (
	"time"

	"gorm.io/gorm"
)

type InventoryTransaction struct {
	ID              uint           `gorm:"primaryKey"`
	ProductID       uint           `gorm:"not null"` // รหัสสินค้าที่เกี่ยวข้อง
	Product         Product        `gorm:"foreignKey:ProductID"`
	FromLocationID  uint           `gorm:""` // รหัสสถานที่ต้นทาง (กรณีย้ายสินค้า)
	FromLocation    Location       `gorm:"foreignKey:FromLocationID"`
	ToLocationID    uint           `gorm:""` // รหัสสถานที่ปลายทาง
	ToLocation      Location       `gorm:"foreignKey:ToLocationID"`
	Quantity        float64        `gorm:"not null"`                  // ปริมาณที่เคลื่อนไหว
	UnitID          uint           `gorm:"not null"`                  // รหัสหน่วยที่เคลื่อนไหว
	Unit            Unit           `gorm:"foreignKey:UnitID"`         // หน่วยที่เคลื่อนไหว เช่น ลัง หรือ ขวด
	TransactionType string         `gorm:"type:varchar(50);not null"` // ประเภทการเคลื่อนไหว เช่น 'receive', 'transfer', 'sell'
	CreatedAt       time.Time      `gorm:"autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"autoUpdateTime"`
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}
