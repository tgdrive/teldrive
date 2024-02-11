package config

import (
	"encoding/json"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeremywohl/flatten"
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
)

type Config struct {
	ServerConfig  ServerConfig   `json:"server"`
	LoggingConfig LoggingConfig  `json:"logging"`
	JwtConfig     JWTConfig      `json:"jwt"`
	DBConfig      DBConfig       `json:"db"`
	Telegram      TelegramConfig `json:"telegram"`
}

type ServerConfig struct {
	Port             int           `json:"port"`
	ReadTimeout      time.Duration `json:"readTimeout"`
	WriteTimeout     time.Duration `json:"writeTimeout"`
	GracefulShutdown time.Duration `json:"gracefulShutdown"`
}

type TelegramConfig struct {
	AppId             int    `json:"appId"`
	AppHash           string `json:"appHash"`
	RateLimit         bool   `json:"rateLimit"`
	RateBurst         int    `json:"rateBurst"`
	Rate              int    `json:"rate"`
	DeviceModel       string `json:"deviceModel"`
	SystemVersion     string `json:"systemVersion"`
	AppVersion        string `json:"appVersion"`
	LangCode          string `json:"langCode"`
	SystemLangCode    string `json:"systemLangCode"`
	LangPack          string `json:"langPack"`
	BgBotsLimit       int    `json:"bgBotsLimit"`
	DisableStreamBots bool   `json:"disableStreamBots"`
	Uploads           struct {
		EncrptionKey string        `json:"encrptionKey"`
		Threads      int           `json:"threads"`
		Retention    time.Duration `json:"retention"`
	} `json:"uploads"`
}

type LoggingConfig struct {
	Level       int    `json:"level"`
	Encoding    string `json:"encoding"`
	Development bool   `json:"development"`
}

type JWTConfig struct {
	Secret       string        `json:"secret"`
	SessionTime  time.Duration `json:"sessionTime"`
	AllowedUsers []string      `json:"allowedUsers"`
}

type DBConfig struct {
	DataSourceName string `json:"dataSourceName"`
	LogLevel       int    `json:"logLevel"`
	Migrate        struct {
		Enable bool `json:"enable"`
	} `json:"migrate"`
	Pool struct {
		MaxOpen     int           `json:"maxOpen"`
		MaxIdle     int           `json:"maxIdle"`
		MaxLifetime time.Duration `json:"maxLifetime"`
	} `json:"pool"`
}

func Load(configPath string) (*Config, error) {
	k := koanf.New(".")

	// load from default config
	err := k.Load(confmap.Provider(defaultConfig, "."), nil)
	if err != nil {
		log.Printf("failed to load default config. err: %v", err)
		return nil, err
	}

	// load from env
	err = k.Load(env.Provider("TELDRIVE_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "TELDRIVE_")), "_", ".", -1)
	}), nil)
	if err != nil {
		log.Printf("failed to load config from env. err: %v", err)
	}

	// load from config file if exist
	if configPath != "" {
		path, err := filepath.Abs(configPath)
		if err != nil {
			log.Printf("failed to get absoulute config path. configPath:%s, err: %v", configPath, err)
			return nil, err
		}
		log.Printf("load config file from %s", path)
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			log.Printf("failed to load config from file. err: %v", err)
			return nil, err
		}
	}

	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "json", FlatPaths: false}); err != nil {
		log.Printf("failed to unmarshal with conf. err: %v", err)
		return nil, err
	}
	return &cfg, err
}

func (c *Config) MarshalJSON() ([]byte, error) {
	type conf Config
	alias := conf(*c)

	data, err := json.Marshal(&alias)
	if err != nil {
		return nil, err
	}

	flat, err := flatten.FlattenString(string(data), "", flatten.DotStyle)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	err = json.Unmarshal([]byte(flat), &m)
	if err != nil {
		return nil, err
	}

	return json.Marshal(&m)
}
