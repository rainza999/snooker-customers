package model

import (
	"time"

	uuid "github.com/satori/go.uuid"
	"gorm.io/gorm"
)

type Employee struct {
	gorm.Model
	Uuid          string `gorm:"unique"`
	FirstName     string
	LastName      string
	NickName      string
	Email         string
	Telephone     string
	DateOfJoining time.Time `gorm:"type:date"`
	Status        string
	IsActive      uint8  `gorm:"default:1"`
	Users         []User `gorm:"foreignKey:EmployeeID"`
}

func (e *Employee) BeforeCreate(tx *gorm.DB) (err error) {
	uuid := uuid.NewV4()
	e.Uuid = uuid.String()
	return nil
}
