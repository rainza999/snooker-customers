package model

import (
	"time"

	"gorm.io/gorm"
)

type Category struct {
	ID        uint           `gorm:"primaryKey"`
	Name      string         `gorm:"type:varchar(100);not null"`
	Status    string         `gorm:"type:varchar(50);default:'active'"`
	IsActive  uint8          `gorm:"default:1"`
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
