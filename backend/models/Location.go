package model

import (
	"time"

	"gorm.io/gorm"
)

type Location struct {
	ID        uint           `gorm:"primaryKey"`
	Name      string         `gorm:"type:varchar(100);not null"` // ชื่อสถานที่เก็บสินค้า เช่น "ตู้ขาย" หรือ "Main Stock"
	Address   string         `gorm:"type:varchar(255)"`          // ที่อยู่ของสถานที่เก็บสินค้า
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
