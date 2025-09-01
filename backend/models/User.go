package model

import (
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	EmployeeID uint `gorm:"foreignKey:EmployeeID"`
	DivisionID uint `gorm:"foreignKey:DivisionID"`
	RoleID     uint `gorm:"foreignKEy:RoleID"`
	Username   string
	Password   string
	IsActive   uint8    `gorm:"default:1"`
	Employee   Employee `gorm:"foreignKey:EmployeeID"`
	Division   Division `gorm:"foreignKey:DivisionID"`
	Role       Role     `gorm:"foreignKey:RoleID"`
}
