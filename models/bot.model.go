package models

type Bot struct {
	Token       string `gorm:"type:text;primaryKey"`
	UserID      int64  `gorm:"type:bigint"`
	BotID       int64  `gorm:"type:bigint"`
	BotUserName string `gorm:"type:text"`
	ChannelID   int64  `gorm:"type:bigint"`
}
