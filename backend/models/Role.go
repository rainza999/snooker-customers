package model

import (
	uuid "github.com/satori/go.uuid"
	"gorm.io/gorm"
)

type Role struct {
	gorm.Model
	Name               string
	Uuid               string              `gorm:"unique"`
	IsActive           uint8               `gorm:"default:1"`
	RoleHasPermissions []RoleHasPermission `gorm:"foreignKey:RoleID"`
	// Users              []User              `gorm:"foreignKey:RoleID"`
}

func (e *Role) BeforeCreate(tx *gorm.DB) (err error) {
	uuid := uuid.NewV4()
	e.Uuid = uuid.String()
	return nil
}
