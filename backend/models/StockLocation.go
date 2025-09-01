package model

import (
	"time"

	"gorm.io/gorm"
)

type StockLocation struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"type:varchar(100);not null"` // ชื่อของสถานที่เก็บ เช่น "Main Stock"
	IsPrimary bool   `gorm:"default:false"`              // ถ้าเป็นสถานที่เก็บหลัก
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
