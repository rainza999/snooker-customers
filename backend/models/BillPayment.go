package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	BillPaymentMethodCash     = "cash"
	BillPaymentMethodTransfer = "transfer"
	BillPaymentMethodCredit   = "credit"
)

type BillPayment struct {
	gorm.Model
	VisitationID uint      `gorm:"not null;index"`
	Method       string    `gorm:"type:varchar(32);not null;index"`
	Amount       float64   `gorm:"type:decimal(10,2);not null"`
	Note         string    `gorm:"type:text"`
	PaidAt       time.Time `gorm:"type:datetime;not null;index"`
	CreatedBy    uint      `gorm:"index"`
	IsActive     uint8     `gorm:"default:1;index"`
}
