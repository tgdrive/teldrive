package models

import (
	"time"
)

type Upload struct {
	ID         string    `gorm:"type:text;primary_key;default:generate_uid(16)"`
	UploadId   string    `gorm:"type:text"`
	UserId     string    `gorm:"type:bigint"`
	Name       string    `gorm:"type:text"`
	PartNo     int       `gorm:"type:integer"`
	TotalParts int       `gorm:"type:integer"`
	PartId     int       `gorm:"type:integer"`
	ChannelID  int64     `gorm:"type:bigint"`
	Size       int64     `gorm:"type:bigint"`
	CreatedAt  time.Time `gorm:"default:timezone('utc'::text, now())"`
}
