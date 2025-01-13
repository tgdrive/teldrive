package models

import (
	"time"
)

type Upload struct {
	UploadId  string    `gorm:"type:text"`
	UserId    int64     `gorm:"type:bigint"`
	Name      string    `gorm:"type:text"`
	PartNo    int       `gorm:"type:integer"`
	PartId    int       `gorm:"type:integer"`
	Encrypted bool      `gorm:"default:false"`
	Salt      string    `gorm:"type:text"`
	ChannelId int64     `gorm:"type:bigint"`
	Size      int64     `gorm:"type:bigint"`
	CreatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
}
