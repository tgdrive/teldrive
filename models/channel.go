package models

type Channel struct {
	ChannelID   int64  `gorm:"type:bigint;primaryKey"`
	ChannelName string `gorm:"type:text"`
	UserID      int64  `gorm:"type:bigint;"`
	Selected    bool   `gorm:"type:boolean;"`
}
