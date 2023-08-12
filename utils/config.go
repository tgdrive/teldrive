package utils

import (
	"github.com/kelseyhightower/envconfig"
)

type MultiToken string

type Config struct {
	AppId       int    `envconfig:"APP_ID" required:"true"`
	AppHash     string `envconfig:"APP_HASH" required:"true"`
	ChannelID   int64  `envconfig:"CHANNEL_ID" required:"true"`
	JwtSecret   string `envconfig:"JWT_SECRET" required:"true"`
	MultiClient bool   `envconfig:"MULTI_CLIENT" default:"false"`
	DatabaseUrl string `envconfig:"DATABASE_URL" required:"true"`
}

var config Config

func InitConfig() {
	err := envconfig.Process("", &config)
	if err != nil {
		panic(err)
	}
}

func GetConfig() *Config {
	return &config
}
