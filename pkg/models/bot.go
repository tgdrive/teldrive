package models

type Bot struct {
	Token  string `gorm:"type:text;primaryKey"`
	UserId int64  `gorm:"type:bigint"`
	BotId  int64  `gorm:"type:bigint"`
}
