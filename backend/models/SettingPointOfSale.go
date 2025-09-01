package model

import (
	"gorm.io/gorm"
)

type SettingPointOfSale struct {
	gorm.Model
	CalProcess uint8 `gorm:"default:1"`
	IsActive   uint8 `gorm:"default:1"`
}
