package model

import (
	"time"

	"gorm.io/gorm"
)

type Unit struct {
	ID             uint           `gorm:"primaryKey"`
	Name           string         `gorm:"type:varchar(50);not null"` // ชื่อหน่วย เช่น "ลัง", "ขวด"
	BaseUnit       *Unit          `gorm:"foreignKey:BaseUnitID"`     // หน่วยฐาน เช่น "ขวด" (ในกรณีที่หน่วยนี้ไม่ใช่หน่วยฐาน)
	BaseUnitID     *uint          `gorm:""`                          // ID ของหน่วยฐาน
	ConversionRate float64        `gorm:"not null;default:1"`        // อัตราการแปลง เช่น 1 ลัง = 12 ขวด
	CreatedAt      time.Time      `gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}
