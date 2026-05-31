package model

import "time"

type InventoryTransaction struct {
	ID                   uint      `gorm:"primaryKey"`
	ProductID            uint      `gorm:"not null;index"`
	StockEntryID         *uint     `gorm:"index"`
	ProductReceiptItemID *uint     `gorm:"index"`
	ServiceID            *uint     `gorm:"index"`
	VisitationID         *uint     `gorm:"index"`
	TransactionType      string    `gorm:"type:varchar(50);not null;index"`
	Quantity             int       `gorm:"not null"`
	BalanceChange        int       `gorm:"not null"`
	CostPerUnit          float64   `gorm:"not null;default:0"`
	Notes                string    `gorm:"type:text"`
	CreatedAt            time.Time `gorm:"autoCreateTime;index"`
}
