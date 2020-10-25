package model

import "gorm.io/gorm"

type LoginSession struct {
	gorm.Model
	SessionId        string
	UserId           *uint
	User             *SimpleUser `gorm:"foreignKey:user_id"`
	Active           bool
	CallbackRedirect string
}
