package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type MultiToken string

type Config struct {
	AppId                  int      `envconfig:"APP_ID" required:"true"`
	AppHash                string   `envconfig:"APP_HASH" required:"true"`
	JwtSecret              string   `envconfig:"JWT_SECRET" required:"true"`
	Https                  bool     `envconfig:"HTTPS" default:"false"`
	CookieSameSite         bool     `envconfig:"COOKIE_SAME_SITE" default:"true"`
	AllowedUsers           []string `envconfig:"ALLOWED_USERS"`
	DatabaseUrl            string   `envconfig:"DATABASE_URL" required:"true"`
	RateLimit              bool     `envconfig:"RATE_LIMIT" default:"true"`
	RateBurst              int      `envconfig:"RATE_BURST" default:"5"`
	Rate                   int      `envconfig:"RATE" default:"100"`
	TgClientDeviceModel    string   `envconfig:"TG_CLIENT_DEVICE_MODEL" default:"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0"`
	TgClientSystemVersion  string   `envconfig:"TG_CLIENT_SYSTEM_VERSION" default:"Win32"`
	TgClientAppVersion     string   `envconfig:"TG_CLIENT_APP_VERSION" default:"2.1.9 K"`
	TgClientLangCode       string   `envconfig:"TG_CLIENT_LANG_CODE" default:"en"`
	TgClientSystemLangCode string   `envconfig:"TG_CLIENT_SYSTEM_LANG_CODE" default:"en"`
	TgClientLangPack       string   `envconfig:"TG_CLIENT_LANG_PACK" default:"webk"`
	RunMigrations          bool     `envconfig:"RUN_MIGRATIONS" default:"true"`
	Port                   int      `envconfig:"PORT" default:"8080"`
	BgBotsLimit            int      `envconfig:"BG_BOTS_LIMIT" default:"5"`
	UploadRetention        int      `envconfig:"UPLOAD_RETENTION" default:"15"`
	DisableStreamBots      bool     `envconfig:"DISABLE_STREAM_BOTS" default:"false"`
	Dev                    bool     `envconfig:"DEV" default:"false"`
	LogSql                 bool     `envconfig:"LOG_SQL" default:"false"`
	EncryptionKey          string   `envconfig:"ENCRYPTION_KEY"`
	ExecDir                string
}

var config Config

func InitConfig() {

	execDir := getExecutableDir()

	godotenv.Load(filepath.Join(execDir, "teldrive.env"))
	err := envconfig.Process("", &config)
	if err != nil {
		panic(err)
	}
	config.ExecDir = execDir
}

func GetConfig() *Config {
	return &config
}

func getExecutableDir() string {

	path, _ := os.Executable()

	executableDir := filepath.Dir(path)

	return executableDir
}
