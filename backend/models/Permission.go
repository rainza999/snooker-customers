package model

import (
	"gorm.io/gorm"
)

type Permission struct {
	gorm.Model
	Name     string
	Title    string
	MenuID   uint
	IsActive uint8 `gorm:"default:1"`
	Menu     Menu  `gorm:"foreignKey:MenuID"`
}
