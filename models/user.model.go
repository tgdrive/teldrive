package models

import (
	"time"
)

type User struct {
	UserId    int       `gorm:"type:int;primaryKey"`
	Name      string    `gorm:"type:text"`
	UserName  string    `gorm:"type:text"`
	IsPremium bool      `gorm:"type:bool"`
	TgSession string    `gorm:"type:text"`
	UpdatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
	CreatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
}
