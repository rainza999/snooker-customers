package model

import "time"

type ActivationKey struct {
	ID        uint    `gorm:"primaryKey"`
	Key       string  `gorm:"uniqueIndex"`
	IsUsed    bool    `gorm:"default:false"`
	MachineID *string `gorm:"index"` // null ก่อนใช้
	UsedAt    *time.Time
	CreatedAt time.Time
}
