package model

import (
	"time"

	"gorm.io/gorm"
)

type ProductStock struct {
	ID         uint           `gorm:"primaryKey"`
	ProductID  uint           `gorm:"not null"` // รหัสสินค้าที่เกี่ยวข้อง
	Product    Product        `gorm:"foreignKey:ProductID"`
	LocationID uint           `gorm:"not null"` // รหัสสถานที่เก็บสินค้าที่เกี่ยวข้อง
	Location   Location       `gorm:"foreignKey:LocationID"`
	Quantity   float64        `gorm:"not null"`          // ปริมาณสต็อกที่มีในหน่วยหลัก เช่น ขวด
	UnitID     uint           `gorm:"not null"`          // รหัสหน่วยที่ใช้เก็บข้อมูลในสต็อก เช่น ลัง
	Unit       Unit           `gorm:"foreignKey:UnitID"` // หน่วยที่ใช้เก็บข้อมูลในสต็อก
	CreatedAt  time.Time      `gorm:"autoCreateTime"`
	UpdatedAt  time.Time      `gorm:"autoUpdateTime"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}
