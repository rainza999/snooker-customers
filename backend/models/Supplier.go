package model

import (
	"gorm.io/gorm"
)

type Supplier struct {
	gorm.Model
	Name     string `gorm:"type:varchar(255);not null"` // ชื่อผู้จำหน่าย
	Contact  string `gorm:"type:varchar(255)"`          // ข้อมูลติดต่อ
	Address  string `gorm:"type:text"`                  // ที่อยู่
	IsActive bool   `gorm:"default:1"`
}
