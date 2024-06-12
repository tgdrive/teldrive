package models

import (
	"time"
)

type Session struct {
	UserId      int64     `gorm:"type:bigint;primaryKey"`
	Hash        string    `gorm:"type:text"`
	SessionDate int       `gorm:"type:text"`
	Session     string    `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"default:timezone('utc'::text, now())"`
}
