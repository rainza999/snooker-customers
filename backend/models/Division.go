package model

import (
	"time"

	"gorm.io/gorm"
)

type Division struct {
	gorm.Model
	Code        string
	MaxDigits   string
	Name        string
	ShortName   string
	Address     string
	OpeningDate time.Time `gorm:"type:date;default:NULL"`
	ClosedDate  time.Time `gorm:"type:date;default:NULL"`
	Tel         string
	Line        string
	Display     uint8
	Status      string
	IsActive    uint8 `gorm:"default:1"`
	QRPath      string
}
