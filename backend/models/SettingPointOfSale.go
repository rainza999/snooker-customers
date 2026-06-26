package model

import (
	"gorm.io/gorm"
)

type SettingPointOfSale struct {
	gorm.Model
	CalProcess       uint8  `gorm:"default:1"`
	PromptPayID      string `gorm:"column:prompt_pay_id"`
	PaymentQRMode    string `gorm:"column:payment_qr_mode;size:32;default:promptpay"`
	PaymentQRPayload string `gorm:"column:payment_qr_payload;type:text"`
	IsActive         uint8  `gorm:"default:1"`
}
