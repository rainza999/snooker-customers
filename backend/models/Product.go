package model

import (
	"time"

	"gorm.io/gorm"
)

type Product struct {
	ID            uint           `gorm:"primaryKey"`
	CategoryID    uint           `gorm:"foreignKey:CategoryID"`
	Name          string         `gorm:"type:varchar(100);not null"`
	Description   string         `gorm:"type:text"`
	Price         float64        `gorm:"not null"`
	Unit          string         `gorm:"type:varchar(50);not null"`
	IsSnookerTime bool           `gorm:"default:false"` // ฟิลด์ใหม่เพื่อระบุว่าสินค้านี้เป็นการจับเวลาของโต๊ะสนุ๊ก
	Status        string         `gorm:"type:varchar(50);default:'active'"`
	CreatedAt     time.Time      `gorm:"autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime"`
	DeletedAt     gorm.DeletedAt `gorm:"index"`
	IsActive      bool           `gorm:"default:1"`
}
