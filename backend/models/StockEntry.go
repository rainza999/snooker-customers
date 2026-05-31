package model

import (
	"time"

	"gorm.io/gorm"
)

type StockEntry struct {
	ID                   uint      `gorm:"primaryKey"`
	ProductID            uint      `gorm:"not null"`
	StockLocationID      uint      `gorm:"not null"`
	Quantity             int       `gorm:"not null"`
	RemainingQty         int       `gorm:"not null;default:0"`
	CostPerUnit          float64   `gorm:"not null"`
	EntryDate            time.Time `gorm:"autoCreateTime"`
	ProductReceiptItemID *uint     `gorm:"index"`
	ReceiptItemKey       *string   `gorm:"type:varchar(100);index"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            gorm.DeletedAt `gorm:"index"`
}
