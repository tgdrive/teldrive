package models

import (
	"time"
)

type FileShare struct {
	ID        string     `gorm:"type:uuid;default:uuid_generate_v4();primary_key"`
	FileId    string     `gorm:"type:uuid;not null"`
	Password  *string    `gorm:"type:text"`
	ExpiresAt *time.Time `gorm:"type:timestamp"`
	CreatedAt time.Time  `gorm:"type:timestamp;not null;default:current_timestamp"`
	UpdatedAt time.Time  `gorm:"type:timestamp;not null;default:current_timestamp"`
	UserId    int64      `gorm:"type:bigint;not null"`
}
