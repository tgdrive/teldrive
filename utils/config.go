package utils

import (
	"github.com/kelseyhightower/envconfig"
)

type MultiToken string

type Config struct {
	AppId                  int    `envconfig:"APP_ID" required:"true"`
	AppHash                string `envconfig:"APP_HASH" required:"true"`
	ChannelID              int64  `envconfig:"CHANNEL_ID" required:"true"`
	JwtSecret              string `envconfig:"JWT_SECRET" required:"true"`
	MultiClient            bool   `envconfig:"MULTI_CLIENT" default:"false"`
	Https                  bool   `envconfig:"HTTPS" default:"false"`
	CookieSameSite         bool   `envconfig:"COOKIE_SAME_SITE" default:"true"`
	DatabaseUrl            string `envconfig:"DATABASE_URL" required:"true"`
	RateLimit              bool   `envconfig:"RATE_LIMIT" default:"true"`
	TgClientDeviceModel    string `envconfig:"TG_CLIENT_DEVICE_MODEL" required:"true"`
	TgClientSystemVersion  string `envconfig:"TG_CLIENT_SYSTEM_VERSION" default:"Win32"`
	TgClientAppVersion     string `envconfig:"TG_CLIENT_APP_VERSION" default:"2.1.9 K"`
	TgClientLangCode       string `envconfig:"TG_CLIENT_LANG_CODE" default:"en"`
	TgClientSystemLangCode string `envconfig:"TG_CLIENT_SYSTEM_LANG_CODE" default:"en"`
	TgClientLangPack       string `envconfig:"TG_CLIENT_LANG_PACK" default:"webk"`
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
