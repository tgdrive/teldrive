package models

import (
	"time"
)

type Channel struct {
	ChannelId   int64     `gorm:"type:bigint;primaryKey"`
	ChannelName string    `gorm:"type:text"`
	UserId      int64     `gorm:"type:bigint"`
	Selected    bool      `gorm:"type:boolean"`
	CreatedAt   time.Time `gorm:"type:timestamptz"`
}
