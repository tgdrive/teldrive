package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tgdrive/teldrive/internal/duration"
	"go.uber.org/zap/zapcore"
)

type ServerConfig struct {
	Port             int           `mapstructure:"port"`
	GracefulShutdown time.Duration `mapstructure:"graceful-shutdown"`
	EnablePprof      bool          `mapstructure:"enable-pprof"`
	ReadTimeout      time.Duration `mapstructure:"read-timeout"`
	WriteTimeout     time.Duration `mapstructure:"write-timeout"`
}

type CacheConfig struct {
	MaxSize   int    `mapstructure:"max-size"`
	RedisAddr string `mapstructure:"redis-addr"`
	RedisPass string `mapstructure:"redis-pass"`
}

type LoggingConfig struct {
	Level string `mapstructure:"level"`
	File  string `mapstructure:"file"`
}

type JWTConfig struct {
	Secret       string        `mapstructure:"secret"`
	SessionTime  time.Duration `mapstructure:"session-time"`
	AllowedUsers []string      `mapstructure:"allowed-users"`
}

type DBConfig struct {
	DataSource  string `mapstructure:"data-source"`
	PrepareStmt bool   `mapstructure:"prepare-stmt"`
	LogLevel    string `mapstructure:"log-level"`
	Pool        struct {
		Enable             bool          `mapstructure:"enable"`
		MaxOpenConnections int           `mapstructure:"max-open-connections"`
		MaxIdleConnections int           `mapstructure:"max-idle-connections"`
		MaxLifetime        time.Duration `mapstructure:"max-lifetime"`
	} `mapstructure:"pool"`
}

type CronJobConfig struct {
	Enable               bool          `mapstructure:"enable"`
	CleanFilesInterval   time.Duration `mapstructure:"clean-files-interval"`
	CleanUploadsInterval time.Duration `mapstructure:"clean-uploads-interval"`
	FolderSizeInterval   time.Duration `mapstructure:"folder-size-interval"`
}

type TGConfig struct {
	AppId             int           `mapstructure:"app-id"`
	AppHash           string        `mapstructure:"app-hash"`
	RateLimit         bool          `mapstructure:"rate-limit"`
	RateBurst         int           `mapstructure:"rate-burst"`
	Rate              int           `mapstructure:"rate"`
	UserName          string        `mapstructure:"user-name"`
	DeviceModel       string        `mapstructure:"device-model"`
	SystemVersion     string        `mapstructure:"system-version"`
	AppVersion        string        `mapstructure:"app-version"`
	LangCode          string        `mapstructure:"lang-code"`
	SystemLangCode    string        `mapstructure:"system-lang-code"`
	LangPack          string        `mapstructure:"lang-pack"`
	Ntp               bool          `mapstructure:"ntp"`
	SessionFile       string        `mapstructure:"session-file"`
	DisableStreamBots bool          `mapstructure:"disable-stream-bots"`
	Proxy             string        `mapstructure:"proxy"`
	ReconnectTimeout  time.Duration `mapstructure:"reconnect-timeout"`
	PoolSize          int64         `mapstructure:"pool-size"`
	EnableLogging     bool          `mapstructure:"enable-logging"`
	Uploads           struct {
		EncryptionKey string        `mapstructure:"encryption-key"`
		Threads       int           `mapstructure:"threads"`
		MaxRetries    int           `mapstructure:"max-retries"`
		Retention     time.Duration `mapstructure:"retention"`
	} `mapstructure:"uploads"`
	Stream struct {
		MultiThreads int           `mapstructure:"multi-threads"`
		Buffers      int           `mapstructure:"buffers"`
		ChunkTimeout time.Duration `mapstructure:"chunk-timeout"`
	} `mapstructure:"stream"`
}

type ServerCmdConfig struct {
	Server   ServerConfig  `mapstructure:"server"`
	Log      LoggingConfig `mapstructure:"log"`
	JWT      JWTConfig     `mapstructure:"jwt"`
	DB       DBConfig      `mapstructure:"db"`
	TG       TGConfig      `mapstructure:"tg"`
	CronJobs CronJobConfig `mapstructure:"cronjobs"`
	Cache    CacheConfig   `mapstructure:"cache"`
}

type MigrateCmdConfig struct {
	DB  DBConfig      `mapstructure:"db"`
	Log LoggingConfig `mapstructure:"log"`
}

type ConfigLoader struct {
	v *viper.Viper
}

func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		v: viper.New(),
	}
}

func StringToDurationHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}

		if t != reflect.TypeOf(time.Duration(0)) {
			return data, nil
		}

		str, ok := data.(string)
		if !ok {
			return data, nil
		}
		return duration.ParseDuration(str)
	}
}

func (cl *ConfigLoader) InitializeConfig(cmd *cobra.Command) error {
	cl.v.SetConfigType("toml")

	cfgFile := cmd.Flags().Lookup("config").Value.String()

	if cfgFile != "" {
		cl.v.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("error getting home directory: %v", err)
		}
		cl.v.AddConfigPath(filepath.Join(home, ".teldrive"))
		cl.v.AddConfigPath(".")
		cl.v.SetConfigName("config")
	}

	cl.v.SetEnvPrefix("teldrive")
	cl.v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	cl.v.AutomaticEnv()

	if err := cl.v.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("error binding flags: %v", err)
	}

	if err := cl.v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("error reading config file: %v", err)
		}
	}

	return nil
}

func (cl *ConfigLoader) Load(cfg interface{}) error {
	config := &mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			StringToDurationHook(),
		),
		WeaklyTypedInput: true,
		Result:           cfg,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return fmt.Errorf("failed to create decoder: %v", err)
	}

	if err := decoder.Decode(cl.v.AllSettings()); err != nil {
		return fmt.Errorf("failed to decode config: %v", err)
	}

	return nil
}

func AddCommonFlags(flags *pflag.FlagSet, config *ServerCmdConfig) {

	flags.StringP("config", "c", "", "Config file path (default $HOME/.teldrive/config.toml)")

	// Log config
	flags.StringVar(&config.Log.Level, "log-level", zapcore.InfoLevel.String(), "Logging level")
	flags.StringVar(&config.Log.File, "log-file", "", "Logging file path")

	// DB config
	flags.StringVar(&config.DB.DataSource, "db-data-source", "", "Database connection string")
	flags.StringVar(&config.DB.LogLevel, "db-log-level", zapcore.InfoLevel.String(), "Database log level")
	flags.BoolVar(&config.DB.PrepareStmt, "db-prepare-stmt", true, "Enable prepared statements")
	flags.BoolVar(&config.DB.Pool.Enable, "db-pool-enable", true, "Enable database pool")
	flags.IntVar(&config.DB.Pool.MaxIdleConnections, "db-pool-max-open-connections", 25, "Database max open connections")
	flags.IntVar(&config.DB.Pool.MaxIdleConnections, "db-pool-max-idle-connections", 25, "Database max idle connections")
	duration.DurationVar(flags, &config.DB.Pool.MaxLifetime, "db-pool-max-lifetime", 10*time.Minute, "Database max connection lifetime")

	// Telegram config
	flags.IntVar(&config.TG.AppId, "tg-app-id", 0, "Telegram app ID")
	flags.StringVar(&config.TG.AppHash, "tg-app-hash", "", "Telegram app hash")
	flags.StringVar(&config.TG.SessionFile, "tg-session-file", "", "Bot session file path")
	flags.BoolVar(&config.TG.RateLimit, "tg-rate-limit", true, "Enable rate limiting for telegram client")
	flags.IntVar(&config.TG.RateBurst, "tg-rate-burst", 5, "Limiting burst for telegram client")
	flags.IntVar(&config.TG.Rate, "tg-rate", 100, "Limiting rate for telegram client")
	flags.StringVar(&config.TG.DeviceModel, "tg-device-model",
		"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0", "Device model")
	flags.StringVar(&config.TG.SystemVersion, "tg-system-version", "Win32", "System version")
	flags.StringVar(&config.TG.AppVersion, "tg-app-version", "4.6.3 K", "App version")
	flags.StringVar(&config.TG.LangCode, "tg-lang-code", "en", "Language code")
	flags.StringVar(&config.TG.SystemLangCode, "tg-system-lang-code", "en-US", "System language code")
	flags.StringVar(&config.TG.LangPack, "tg-lang-pack", "webk", "Language pack")
	flags.StringVar(&config.TG.Proxy, "tg-proxy", "", "HTTP OR SOCKS5 proxy URL")
	flags.BoolVar(&config.TG.DisableStreamBots, "tg-disable-stream-bots", false, "Disable Stream bots")
	flags.BoolVar(&config.TG.Ntp, "tg-ntp", false, "Use NTP server time")
	flags.BoolVar(&config.TG.EnableLogging, "tg-enable-logging", false, "Enable telegram client logging")
	flags.Int64Var(&config.TG.PoolSize, "tg-pool-size", 8, "Telegram Session pool size")
}
