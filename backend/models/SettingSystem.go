package model

import (
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type SettingSystem struct {
	gorm.Model
	LogoPath           string
	LogoLoginPath      string
	CloseTablePassword string
	EditReportPassword string
	FirstTime          bool
	IsActive           uint8 `gorm:"default:1"`
}

func (s *SettingSystem) SetCloseTablePassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.CloseTablePassword = string(hashedPassword)
	return nil
}

func (s *SettingSystem) SetEditReportPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.EditReportPassword = string(hashedPassword)
	return nil
}
