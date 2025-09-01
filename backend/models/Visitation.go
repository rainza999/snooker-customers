package model

import (
	"fmt"
	"strconv"
	"time"

	uuid "github.com/satori/go.uuid"
	"gorm.io/gorm"
)

type Visitation struct {
	gorm.Model
	Uuid           string `gorm:"unique"`
	Code           string
	BillCode       string
	BillType       uint8
	CustomerID     uint
	TableID        uint
	TableType      uint
	IsVisit        uint
	DivisionID     uint
	VisitDate      time.Time `gorm:"type:datetime"`
	StartTime      time.Time `gorm:"type:datetime"`
	EndTime        time.Time `gorm:"type:datetime"`
	UseTime        time.Time `gorm:"type:datetime"`
	PauseTime      time.Time `gorm:"type:datetime"`
	PausedDuration int64     // เก็บระยะเวลาที่หยุดเล่น (หน่วยเป็นวินาที)
	TotalCost      float64
	NetPrice       float64
	PaidAmount     float64 // จำนวนเงินที่ลูกค้าชำระมา
	ChangeAmount   float64 // จำนวนเงินทอน
	IsPaid         uint8
	IsActive       uint8 `gorm:"default:1"`
}

func (v *Visitation) BeforeCreate(tx *gorm.DB) (err error) {
	// Generate UUID
	uuid := uuid.NewV4()
	v.Uuid = uuid.String()

	// Generate Code
	currentYear := time.Now().Year() + 543 - 2500 // Convert to Thai year and get last 2 digits
	yearCode := fmt.Sprintf("%02d", currentYear%100)

	var division Division
	if err := tx.First(&division, v.DivisionID).Error; err != nil {
		return err
	}
	divisionCode := division.Code

	// Convert MaxDigits to int, increment, and convert back to string
	maxDigitsInt, err := strconv.Atoi(division.MaxDigits)
	if err != nil {
		return err
	}
	maxDigitsInt++
	maxDigits := fmt.Sprintf("%06d", maxDigitsInt)
	v.Code = yearCode + divisionCode + maxDigits

	// Update max_digits in divisions table
	division.MaxDigits = maxDigits
	if err := tx.Save(&division).Error; err != nil {
		return err
	}

	// Set default values
	v.VisitDate = time.Now().Truncate(24 * time.Hour)
	v.StartTime = time.Now()
	v.UseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)   // ตั้งค่าปีเป็นปีที่เหมาะสม
	v.PauseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC) // ตั้งค่าปีเป็นปีที่เหมาะสม
	v.TotalCost = 0
	v.NetPrice = 0
	v.IsPaid = 0
	v.IsVisit = 0
	v.IsActive = 1

	return nil
}
