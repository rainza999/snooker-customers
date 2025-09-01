package model

import (
	"time"

	"gorm.io/gorm"
)

type SettingTable struct {
	gorm.Model
	DivisionID  uint `gorm:"foreignKey:DivisionID"`
	Code        string
	Name        string
	Price       float64   `gorm:"type:decimal(10,2);"` // กำหนดชนิดข้อมูลเป็น decimal
	Price2      float64   `gorm:"type:decimal(10,2);"` // กำหนดชนิดข้อมูลเป็น decimal
	OpeningDate time.Time `gorm:"type:date;default:NULL"`
	ClosedDate  time.Time `gorm:"type:date;default:NULL"`
	Ma          uint8
	Type        uint8
	Status      string
	IsActive    uint8 `gorm:"default:1"`
	Relay       uint8
	Address     string
}
