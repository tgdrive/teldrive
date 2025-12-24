package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tgdrive/teldrive/internal/duration"
)

type ServerCmdConfig struct {
	Server   ServerConfig  `config:"server"`
	Log      LoggingConfig `config:"log"`
	JWT      JWTConfig     `config:"jwt"`
	DB       DBConfig      `config:"db"`
	TG       TGConfig      `config:"tg"`
	CronJobs CronJobConfig `config:"cronjobs"`
	Cache    CacheConfig   `config:"cache"`
}

type ServerConfig struct {
	Port             int           `config:"port" description:"HTTP port for the server to listen on" default:"8080"`
	GracefulShutdown time.Duration `config:"graceful-shutdown" description:"Grace period for server shutdown" default:"10s"`
	EnablePprof      bool          `config:"enable-pprof" description:"Enable pprof debugging endpoints"`
	ReadTimeout      time.Duration `config:"read-timeout" description:"Maximum duration for reading entire request" default:"1h"`
	WriteTimeout     time.Duration `config:"write-timeout" description:"Maximum duration for writing response" default:"1h"`
}

type CacheConfig struct {
	MaxSize   int    `config:"max-size" description:"Maximum cache size in bytes" default:"10485760"`
	RedisAddr string `config:"redis-addr" description:"Redis server address"`
	RedisPass string `config:"redis-pass" description:"Redis server password"`
}

type LoggingConfig struct {
	Level string `config:"level" description:"Logging level (debug, info, warn, error)" default:"info"`
	File  string `config:"file" description:"Log file path, if empty logs to stdout"`
}

type JWTConfig struct {
	Secret       string        `config:"secret" description:"JWT signing secret key" required:"true"`
	SessionTime  time.Duration `config:"session-time" description:"JWT token validity duration" default:"30d"`
	AllowedUsers []string      `config:"allowed-users" description:"List of allowed usernames"`
}

type DBPool struct {
	Enable             bool          `config:"enable" description:"Enable connection pooling" default:"true"`
	MaxOpenConnections int           `config:"max-open-connections" description:"Maximum number of open connections" default:"25"`
	MaxIdleConnections int           `config:"max-idle-connections" description:"Maximum number of idle connections" default:"25"`
	MaxLifetime        time.Duration `config:"max-lifetime" description:"Maximum connection lifetime" default:"10m"`
}
type DBConfig struct {
	DataSource  string `config:"data-source" description:"Database connection string" required:"true"`
	PrepareStmt bool   `config:"prepare-stmt" description:"Use prepared statements" default:"true"`
	LogLevel    string `config:"log-level" description:"Database logging level" default:"error"`
	Pool        DBPool `config:"pool"`
}

type CronJobConfig struct {
	Enable               bool          `config:"enable" description:"Enable scheduled background jobs" default:"true"`
	LockerInstance       string        `config:"locker-instance" description:"Distributed unique cron locker name" default:"cron-locker"`
	CleanFilesInterval   time.Duration `config:"clean-files-interval" description:"Interval for cleaning expired files" default:"1h"`
	CleanUploadsInterval time.Duration `config:"clean-uploads-interval" description:"Interval for cleaning incomplete uploads" default:"12h"`
	FolderSizeInterval   time.Duration `config:"folder-size-interval" description:"Interval for updating folder sizes" default:"2h"`
}

type TGStream struct {
	Concurrency  int           `config:"concurrency" description:"Number of concurrent threads for concurrent reader" default:"1"`
	Buffers      int           `config:"buffers" description:"Number of stream buffers" default:"8"`
	ChunkTimeout time.Duration `config:"chunk-timeout" description:"Chunk download timeout" default:"30s"`
}

type TGUpload struct {
	EncryptionKey string        `config:"encryption-key" description:"Encryption key for uploads" required:"true"`
	Threads       int           `config:"threads" description:"Number of upload threads" default:"8"`
	MaxRetries    int           `config:"max-retries" description:"Maximum upload retry attempts" default:"10"`
	Retention     time.Duration `config:"retention" description:"Upload retention period" default:"7d"`
}
type TGConfig struct {
	RateLimit         bool          `config:"rate-limit" description:"Enable rate limiting for API calls" default:"true"`
	RateBurst         int           `config:"rate-burst" description:"Maximum burst size for rate limiting" default:"5"`
	Rate              int           `config:"rate" description:"Rate limit in requests per minute" default:"100"`
	Ntp               bool          `config:"ntp" description:"Use NTP for time synchronization"`
	Proxy             string        `config:"proxy" description:"HTTP/SOCKS5 proxy URL"`
	ReconnectTimeout  time.Duration `config:"reconnect-timeout" description:"Client reconnection timeout" default:"5m"`
	PoolSize          int           `config:"pool-size" description:"Session pool size" default:"8"`
	EnableLogging     bool          `config:"enable-logging" description:"Enable Telegram client logging"`
	AppId             int           `config:"app-id" description:"Telegram app ID" default:"2496"`
	AppHash           string        `config:"app-hash" description:"Telegram app hash" default:"8da85b0d5bfe62527e5b244c209159c3"`
	DeviceModel       string        `config:"device-model" description:"Device model" default:"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0"`
	SystemVersion     string        `config:"system-version" description:"System version" default:"Win32"`
	AppVersion        string        `config:"app-version" description:"App version" default:"6.1.4 K"`
	LangCode          string        `config:"lang-code" description:"Language code" default:"en"`
	SystemLangCode    string        `config:"system-lang-code" description:"System language code" default:"en-US"`
	LangPack          string        `config:"lang-pack" description:"Language pack" default:"webk"`
	SessionInstance   string        `config:"session-instance" description:"Bot Sessions Instance Name" default:"teldrive"`
	AutoChannelCreate bool          `config:"auto-channel-create" description:"Auto Create Channel" default:"true"`
	ChannelLimit      int64         `config:"channel-limit" description:"Channel message limit before auto channel creation" default:"500000"`
	Uploads           TGUpload      `config:"uploads"`
	Stream            TGStream      `config:"stream"`
}

type ConfigLoader struct {
	v              *viper.Viper
	requiredFields []string
}

func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		v: viper.New(),
	}
}

func StringToDurationHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
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

func (cl *ConfigLoader) Load(cmd *cobra.Command, cfg any) error {

	cfgFile := cmd.Flags().Lookup("config").Value.String()
	if cfgFile != "" {
		cl.v.SetConfigFile(cfgFile)
		// Set config type based on file extension
		if strings.HasSuffix(cfgFile, ".yaml") || strings.HasSuffix(cfgFile, ".yml") {
			cl.v.SetConfigType("yaml")
		} else {
			cl.v.SetConfigType("toml")
		}
	} else {
		cl.v.SetConfigType("toml")
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("error getting home directory: %v", err)
		}
		cl.v.AddConfigPath(filepath.Join(home, ".teldrive"))
		cl.v.AddConfigPath(".")
		cl.v.SetConfigName("config")
	}

	if err := cl.v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("error reading config file: %v", err)
		}
	}

	return cl.load(cfg)
}

func (cl *ConfigLoader) Validate() error {
	missingFields := []string{}
	for _, key := range cl.requiredFields {
		if !cl.v.IsSet(key) {
			missingFields = append(missingFields, strings.ReplaceAll(key, ".", "-"))
		}
	}
	if len(missingFields) > 0 {
		return fmt.Errorf("required configuration values not set: %s", strings.Join(missingFields, ", "))
	}
	return nil
}

func (cl *ConfigLoader) RegisterFlags(flags *pflag.FlagSet, prefix string, v any, skipFlags bool) error {
	flags.StringP("config", "c", "", "Config file path (default $HOME/.teldrive/config.toml)")
	return cl.walkStruct(v, prefix, func(key string, field reflect.StructField, value reflect.Value) error {
		return cl.setDefault(flags, key, field, skipFlags)
	})
}

func (cl *ConfigLoader) setDefault(flags *pflag.FlagSet, key string, field reflect.StructField, skipFlags bool) error {
	description := field.Tag.Get("description")
	defaultVal := field.Tag.Get("default")

	if defaultVal != "" {
		description += fmt.Sprintf(" (default %s)", defaultVal)
	}
	if required := field.Tag.Get("required"); required == "true" {
		cl.requiredFields = append(cl.requiredFields, key)
	}

	flagKey := strings.ReplaceAll(key, ".", "-")

	if defaultVal != "" {
		cl.v.SetDefault(key, defaultVal)
	}

	if skipFlags {
		return nil
	}

	// Handle time.Duration first since it's a named type that would otherwise be treated as int64
	if field.Type == reflect.TypeOf(time.Duration(0)) {
		defaultDuration := time.Duration(0)
		if defaultVal != "" {
			if val, err := duration.ParseDuration(defaultVal); err == nil {
				defaultDuration = val
			}
		}
		flags.Duration(flagKey, defaultDuration, description)
	} else {
		switch field.Type.Kind() {
		case reflect.String:
			defaultStr := ""
			if defaultVal != "" {
				defaultStr = defaultVal
			}
			flags.String(flagKey, defaultStr, description)
		case reflect.Int:
			defaultInt := 0
			if defaultVal != "" {
				if val, err := strconv.Atoi(defaultVal); err == nil {
					defaultInt = val
				}
			}
			flags.Int(flagKey, defaultInt, description)
		case reflect.Int64:
			defaultInt64 := int64(0)
			if defaultVal != "" {
				if val, err := strconv.ParseInt(defaultVal, 10, 64); err == nil {
					defaultInt64 = val
				}
			}
			flags.Int64(flagKey, defaultInt64, description)
		case reflect.Bool:
			defaultBool := false
			if defaultVal != "" {
				if val, err := strconv.ParseBool(defaultVal); err == nil {
					defaultBool = val
				}
			}
			flags.Bool(flagKey, defaultBool, description)
		case reflect.Slice:
			switch field.Type.Elem().Kind() {
			case reflect.String:
				var defaultStrSlice []string
				if defaultVal != "" {
					defaultStrSlice = strings.Split(defaultVal, ",")
					for i, s := range defaultStrSlice {
						defaultStrSlice[i] = strings.TrimSpace(s)
					}
				}
				flags.StringSlice(flagKey, defaultStrSlice, description)
			case reflect.Int:
				var defaultIntSlice []int
				if defaultVal != "" {
					parts := strings.Split(defaultVal, ",")
					for _, part := range parts {
						if val, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
							defaultIntSlice = append(defaultIntSlice, val)
						}
					}
				}
				flags.IntSlice(flagKey, defaultIntSlice, description)
			}
		case reflect.Map:
			// Skip map fields as they are not supported for command line flags
			return nil
		default:
			// Skip unsupported field types
			return nil
		}
	}
	if err := cl.v.BindPFlag(key, flags.Lookup(flagKey)); err != nil {
		return fmt.Errorf("error binding flag %s: %w", key, err)
	}

	return nil
}

func (cl *ConfigLoader) walkStruct(v any, prefix string, fn func(key string, field reflect.StructField, value reflect.Value) error) error {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		value := val.Field(i)
		configTag := field.Tag.Get("config")
		if configTag == "" {
			continue
		}
		key := configTag
		if prefix != "" {
			key = prefix + "." + configTag
		}
		if field.Type.Kind() == reflect.Struct {
			var nestedValue any
			if value.CanAddr() {
				nestedValue = value.Addr().Interface()
			} else {
				nestedValue = value.Interface()
			}
			if err := cl.walkStruct(nestedValue, key, fn); err != nil {
				return err
			}
			continue
		}

		if err := fn(key, field, value); err != nil {
			return err
		}
	}

	return nil
}

func decodeTag(tag string) viper.DecoderConfigOption {
	return func(c *mapstructure.DecoderConfig) {
		c.TagName = tag
	}
}

func (cl *ConfigLoader) load(cfg any) error {
	return cl.v.Unmarshal(&cfg, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		StringToDurationHook(),
	)), decodeTag("config"))

}
