package model

import (
	"gorm.io/gorm"
)

type Menu struct {
	gorm.Model
	Name  string
	Route string
	Level uint8
	// Relation uint `gorm:"foreignKey:ID;references:ID;default:NULL;OnDelete:SET NULL"`
	Relation uint `gorm:"foreignKey:ID;references:ID;default:NULL"`
	HasSub   uint `gorm:"default:0"`
	Order    uint8
	Icon     string `gorm:"default:NULL"`
	IsActive uint8  `gorm:"default:1"`
}
