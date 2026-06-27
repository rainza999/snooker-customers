package model

import (
	"time"

	"gorm.io/gorm"
)

type Promotion struct {
	gorm.Model
	Name     string    `gorm:"type:varchar(255);not null"`
	StartAt  time.Time `gorm:"type:datetime;not null;index"`
	EndAt    time.Time `gorm:"type:datetime;not null;index"`
	Priority int       `gorm:"not null;default:0"`
	IsActive uint8     `gorm:"default:1;index"`
}

type PromotionTablePrice struct {
	gorm.Model
	PromotionID   uint      `gorm:"not null;index"`
	TableID       uint      `gorm:"not null;index"`
	NormalPrice   float64   `gorm:"type:decimal(10,2);not null"`
	PracticePrice float64   `gorm:"type:decimal(10,2);not null"`
	IsActive      uint8     `gorm:"default:1;index"`
	Promotion     Promotion `gorm:"foreignKey:PromotionID"`
}
