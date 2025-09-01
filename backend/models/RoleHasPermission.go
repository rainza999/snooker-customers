package model

type RoleHasPermission struct {
	PermissionID uint `gorm:"primaryKey;foreignKey:ID;references:Permission"`
	RoleID       uint `gorm:"primaryKey;foreignKey:ID;references:Role"`
}
