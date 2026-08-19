package model

import (
	"time"

	"gorm.io/gorm"
)

type VisitationPausePeriod struct {
	gorm.Model
	VisitationID    uint       `gorm:"not null;index"`
	PauseStart      time.Time  `gorm:"type:datetime;not null"`
	PauseEnd        *time.Time `gorm:"type:datetime;index"`
	DurationSeconds int64      `gorm:"not null;default:0"`
}
