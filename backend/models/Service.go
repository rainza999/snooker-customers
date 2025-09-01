package model

import (
	"time"

	"gorm.io/gorm"
)

type Service struct {
	ID             uint           `gorm:"primaryKey"`
	VisitationID   uint           `gorm:"not null"`
	ProductID      uint           `gorm:"not null"`
	SellQuantity   float64        `gorm:"not null"`
	SellUnitID     string         `gorm:"type:varchar(50);not null"`
	TotalCost      float64        `gorm:"not null"`
	TotalFIFO_Cost float64        `gorm:"not null;default:0"`
	NetPrice       float64        `gorm:"not null"`
	Status         string         `gorm:"type:varchar(50);default:'draft'"`
	UseTime        time.Time      `gorm:""` // ฟิลด์ใหม่เพื่อเก็บ UseTime จาก Visitation
	CreatedAt      time.Time      `gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}
