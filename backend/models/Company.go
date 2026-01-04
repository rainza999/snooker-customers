package model

import (
	"gorm.io/gorm"
)

type Company struct {
	gorm.Model
	Name     string
	Status   string
	IsActive uint8 `gorm:"default:1"`
}
