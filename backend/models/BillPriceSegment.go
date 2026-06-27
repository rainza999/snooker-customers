package model

import (
	"time"

	"gorm.io/gorm"
)

type BillPriceSegment struct {
	gorm.Model
	VisitationID    uint      `gorm:"not null;index"`
	TableID         uint      `gorm:"not null;index"`
	PromotionID     *uint     `gorm:"index"`
	SegmentStart    time.Time `gorm:"type:datetime;not null"`
	SegmentEnd      time.Time `gorm:"type:datetime;not null"`
	DurationSeconds int64     `gorm:"not null"`
	TableType       uint8     `gorm:"not null"`
	UnitPrice       float64   `gorm:"type:decimal(10,2);not null"`
	Source          string    `gorm:"type:varchar(32);not null;default:'standard'"`
	Amount          float64   `gorm:"type:decimal(10,2);not null"`
}
