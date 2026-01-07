package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/maps"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/tgdrive/teldrive/internal/duration"
)

type ServerCmdConfig struct {
	Server   ServerConfig  `koanf:"server"`
	Log      LoggingConfig `koanf:"log"`
	JWT      JWTConfig     `koanf:"jwt"`
	DB       DBConfig      `koanf:"db"`
	TG       TGConfig      `koanf:"tg"`
	CronJobs CronJobConfig `koanf:"cronjobs"`
	Cache    CacheConfig   `koanf:"cache"`
}

type CheckCmdConfig struct {
	Log          LoggingConfig `koanf:"log" skipPflag:"true"`
	DB           DBConfig      `koanf:"db" skipPflag:"true"`
	TG           TGConfig      `koanf:"tg" skipPflag:"true"`
	ExportFile   string        `koanf:"export-file" default:"results.json" description:"Path for exported JSON file"`
	DryRun       bool          `koanf:"dry-run" default:"false" description:"Simulate check/clean process without making changes"`
	User         string        `koanf:"user" default:"" description:"Telegram username to check (prompts if not specified)"`
	Concurrent   int           `koanf:"concurrent" default:"4" description:"Number of concurrent channel processing"`
	CleanUploads bool          `koanf:"clean-uploads" default:"false" description:"Clean incomplete uploads"`
	CleanPending bool          `koanf:"clean-pending" default:"false" description:"Clean files with pending_deletion status"`
}

type ServerConfig struct {
	Port             int           `koanf:"port" default:"8080" description:"HTTP port for the server to listen on"`
	GracefulShutdown time.Duration `koanf:"graceful-shutdown" default:"10s" description:"Grace period for server shutdown"`
	EnablePprof      bool          `koanf:"enable-pprof" default:"false" description:"Enable pprof debugging endpoints"`
	ReadTimeout      time.Duration `koanf:"read-timeout" default:"1h" description:"Maximum duration for reading entire request"`
	WriteTimeout     time.Duration `koanf:"write-timeout" default:"1h" description:"Maximum duration for writing response"`
}

type CacheConfig struct {
	MaxSize   int    `koanf:"max-size" default:"10485760" description:"Maximum cache size in bytes"`
	RedisAddr string `koanf:"redis-addr" default:"" description:"Redis server address"`
	RedisPass string `koanf:"redis-pass" default:"" description:"Redis server password"`
}

type LoggingConfig struct {
	Level string `koanf:"level" default:"info" description:"Logging level (debug, info, warn, error)"`
	File  string `koanf:"file" default:"" description:"Log file path, if empty logs to stdout"`
}

type JWTConfig struct {
	Secret       string        `validate:"required" koanf:"secret" default:"" description:"JWT signing secret key"`
	SessionTime  time.Duration `koanf:"session-time" default:"30d" description:"JWT token validity duration"`
	AllowedUsers []string      `koanf:"allowed-users" default:"" description:"List of allowed usernames"`
}

type DBPool struct {
	Enable             bool          `koanf:"enable" default:"true" description:"Enable connection pooling"`
	MaxOpenConnections int           `koanf:"max-open-connections" default:"25" description:"Maximum number of open connections"`
	MaxIdleConnections int           `koanf:"max-idle-connections" default:"25" description:"Maximum number of idle connections"`
	MaxLifetime        time.Duration `koanf:"max-lifetime" default:"10m" description:"Maximum connection lifetime"`
}
type DBConfig struct {
	DataSource  string `validate:"required" koanf:"data-source" default:"" description:"Database connection string"`
	PrepareStmt bool   `koanf:"prepare-stmt" default:"true" description:"Use prepared statements"`
	LogLevel    string `koanf:"log-level" default:"error" description:"Database logging level"`
	Pool        DBPool `koanf:"pool"`
}

type CronJobConfig struct {
	Enable               bool          `koanf:"enable" default:"true" description:"Enable scheduled background jobs"`
	LockerInstance       string        `koanf:"locker-instance" default:"cron-locker" description:"Distributed unique cron locker name"`
	CleanFilesInterval   time.Duration `koanf:"clean-files-interval" default:"1h" description:"Interval for cleaning expired files"`
	CleanUploadsInterval time.Duration `koanf:"clean-uploads-interval" default:"12h" description:"Interval for cleaning incomplete uploads"`
	FolderSizeInterval   time.Duration `koanf:"folder-size-interval" default:"2h" description:"Interval for updating folder sizes"`
}

type TGStream struct {
	Concurrency  int           `koanf:"concurrency" default:"1" description:"Number of concurrent threads for concurrent reader"`
	Buffers      int           `koanf:"buffers" default:"8" description:"Number of stream buffers"`
	ChunkTimeout time.Duration `koanf:"chunk-timeout" default:"30s" description:"Chunk download timeout"`
}

type TGUpload struct {
	EncryptionKey string        `koanf:"encryption-key" default:"" description:"Encryption key for uploads"`
	Threads       int           `koanf:"threads" default:"8" description:"Number of upload threads"`
	MaxRetries    int           `koanf:"max-retries" default:"10" description:"Maximum upload retry attempts"`
	Retention     time.Duration `koanf:"retention" default:"7d" description:"Upload retention period"`
}
type TGConfig struct {
	RateLimit         bool          `koanf:"rate-limit" default:"true" description:"Enable rate limiting for API calls"`
	RateBurst         int           `koanf:"rate-burst" default:"5" description:"Maximum burst size for rate limiting"`
	Rate              int           `koanf:"rate" default:"100" description:"Rate limit in requests per minute"`
	Ntp               bool          `koanf:"ntp" default:"false" description:"Use NTP for time synchronization"`
	Proxy             string        `koanf:"proxy" default:"" description:"HTTP/SOCKS5 proxy URL"`
	ReconnectTimeout  time.Duration `koanf:"reconnect-timeout" default:"5m" description:"Client reconnection timeout"`
	PoolSize          int           `koanf:"pool-size" default:"8" description:"Session pool size"`
	EnableLogging     bool          `koanf:"enable-logging" default:"false" description:"Enable Telegram client logging"`
	AppId             int           `koanf:"app-id" default:"2496" description:"Telegram app ID"`
	AppHash           string        `koanf:"app-hash" default:"8da85b0d5bfe62527e5b244c209159c3" description:"Telegram app hash"`
	DeviceModel       string        `koanf:"device-model" default:"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0" description:"Device model"`
	SystemVersion     string        `koanf:"system-version" default:"Win32" description:"System version"`
	AppVersion        string        `koanf:"app-version" default:"6.1.4 K" description:"App version"`
	LangCode          string        `koanf:"lang-code" default:"en" description:"Language code"`
	SystemLangCode    string        `koanf:"system-lang-code" default:"en-US" description:"System language code"`
	LangPack          string        `koanf:"lang-pack" default:"webk" description:"Language pack"`
	SessionInstance   string        `koanf:"session-instance" default:"teldrive" description:"Bot Sessions Instance Name"`
	AutoChannelCreate bool          `koanf:"auto-channel-create" default:"true" description:"Auto Create Channel"`
	ChannelLimit      int64         `koanf:"channel-limit" default:"500000" description:"Channel message limit before auto channel creation"`
	Uploads           TGUpload      `koanf:"uploads"`
	Stream            TGStream      `koanf:"stream"`
}

type ConfigLoader struct {
	k       *koanf.Koanf
	flagMap map[string]string
}

func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		k:       koanf.New("."),
		flagMap: make(map[string]string),
	}
}

// customFlagProvider loads flags from a pflag.FlagSet.
type customFlagProvider struct {
	f           *pflag.FlagSet
	flagMap     map[string]string
	onlyChanged bool
	defaults    bool
}

func (p *customFlagProvider) Read() (map[string]any, error) {
	m := make(map[string]any)
	p.f.VisitAll(func(f *pflag.Flag) {
		if p.defaults && f.Changed {
			return
		}
		if p.onlyChanged && !f.Changed {
			return
		}

		var key string
		if mapped, ok := p.flagMap[f.Name]; ok {
			key = mapped
		} else {
			// Fallback: simple dash replacement if not mapped (should not happen if registered correctly)
			key = strings.ReplaceAll(f.Name, "-", ".")
		}

		// Handle slices
		if sliceVal, ok := f.Value.(pflag.SliceValue); ok {
			m[key] = sliceVal.GetSlice()
		} else {
			m[key] = f.Value.String()
		}
	})
	return maps.Unflatten(m, "."), nil
}

func (p *customFlagProvider) ReadBytes() ([]byte, error) {
	return nil, nil
}

type unflattenProvider struct {
	p     koanf.Provider
	delim string
}

func (p *unflattenProvider) Read() (map[string]any, error) {
	m, err := p.p.Read()
	if err != nil {
		return nil, err
	}
	return maps.Unflatten(m, p.delim), nil
}

func (p *unflattenProvider) ReadBytes() ([]byte, error) {
	return nil, nil
}

func (cl *ConfigLoader) Load(cmd *cobra.Command, cfg any) error {

	cfgFile := cmd.Flags().Lookup("config").Value.String()
	var parser koanf.Parser

	if cfgFile != "" {
		if strings.HasSuffix(cfgFile, ".yaml") || strings.HasSuffix(cfgFile, ".yml") {
			parser = yaml.Parser()
		} else {
			parser = toml.Parser()
		}
	} else {
		parser = toml.Parser()
	}

	// 1. Load defaults from flags
	if err := cl.k.Load(&customFlagProvider{f: cmd.Flags(), flagMap: cl.flagMap, defaults: true}, nil); err != nil {
		return err
	}

	// Load defaults for skipped flags
	cl.loadSkippedDefaults(reflect.TypeOf(cfg), "")

	// 2. Load config file
	if cfgFile != "" {
		if err := cl.k.Load(file.Provider(cfgFile), parser); err != nil {
			return fmt.Errorf("error reading config file: %v", err)
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("error getting home directory: %v", err)
		}
		paths := []string{
			filepath.Join(home, ".teldrive", "config.toml"),
			"config.toml",
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				if err := cl.k.Load(file.Provider(path), toml.Parser()); err != nil {
					return fmt.Errorf("error reading config file: %v", err)
				}
				break
			}
		}
	}

	// 3. Load environment variables
	if err := cl.k.Load(&unflattenProvider{
		p: env.Provider("TELDRIVE_", ".", func(s string) string {
			return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "TELDRIVE_")), "_", ".")
		}),
		delim: ".",
	}, nil); err != nil {
		return err
	}

	// 4. Load explicit flags
	if err := cl.k.Load(&customFlagProvider{f: cmd.Flags(), flagMap: cl.flagMap, onlyChanged: true}, nil); err != nil {
		return err
	}

	unmarshalCfg := koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook: func(f reflect.Type, t reflect.Type, data any) (any, error) {
				if f.Kind() != reflect.String {
					return data, nil
				}
				if t != reflect.TypeFor[time.Duration]() {
					return data, nil
				}
				return duration.ParseDuration(data.(string))
			},
			Result:           cfg,
			WeaklyTypedInput: true,
		},
	}

	if err := cl.k.UnmarshalWithConf("", cfg, unmarshalCfg); err != nil {
		return err
	}

	return nil
}

func (cl *ConfigLoader) Validate(cfg any) error {
	validate := validator.New()
	return validate.Struct(cfg)
}

func (cl *ConfigLoader) RegisterFlags(flags *pflag.FlagSet, t reflect.Type) {
	flags.StringP("config", "c", "", "Config file path (default $HOME/.teldrive/config.toml)")
	cl.registerStruct(flags, "", t)
}

func (cl *ConfigLoader) loadSkippedDefaults(t reflect.Type, prefix string) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		koanfTag := field.Tag.Get("koanf")
		if koanfTag == "" {
			continue
		}

		key := koanfTag
		if prefix != "" {
			key = prefix + "." + koanfTag
		}

		if field.Tag.Get("skipPflag") == "true" {
			cl.registerDefaultsRecursive(field.Type, key)
			continue
		}

		if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeFor[time.Duration]() {
			cl.loadSkippedDefaults(field.Type, key)
		}
	}
}

func (cl *ConfigLoader) registerDefaultsRecursive(t reflect.Type, prefix string) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		koanfTag := field.Tag.Get("koanf")
		if koanfTag == "" {
			continue
		}

		key := prefix + "." + koanfTag

		if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeFor[time.Duration]() {
			cl.registerDefaultsRecursive(field.Type, key)
			continue
		}

		defaultValue := field.Tag.Get("default")
		if defaultValue != "" {
			var val any = defaultValue
			switch field.Type.Kind() {
			case reflect.Int:
				val, _ = strconv.Atoi(defaultValue)
			case reflect.Int64:
				if field.Type != reflect.TypeFor[time.Duration]() {
					val, _ = strconv.ParseInt(defaultValue, 10, 64)
				}
			case reflect.Bool:
				val, _ = strconv.ParseBool(defaultValue)
			case reflect.Slice:
				if field.Type.Elem().Kind() == reflect.String {
					val = strings.Split(defaultValue, ",")
				}
			}
			cl.k.Set(key, val)
		}
	}
}

func (cl *ConfigLoader) registerStruct(flags *pflag.FlagSet, prefix string, t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		koanfTag := field.Tag.Get("koanf")

		if koanfTag == "" {
			continue
		}

		key := koanfTag
		if prefix != "" {
			key = prefix + "." + koanfTag
		}

		if field.Tag.Get("skipPflag") == "true" {
			continue
		}

		if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeFor[time.Duration]() {
			cl.registerStruct(flags, key, field.Type)
			continue
		}

		defaultValue := field.Tag.Get("default")
		description := field.Tag.Get("description")
		name := strings.ReplaceAll(key, ".", "-")
		cl.flagMap[name] = key

		switch field.Type.Kind() {
		case reflect.String:
			flags.String(name, defaultValue, description)
		case reflect.Int:
			val, _ := strconv.Atoi(defaultValue)
			flags.Int(name, val, description)
		case reflect.Int64:
			if field.Type == reflect.TypeFor[time.Duration]() {
				val, _ := duration.ParseDuration(defaultValue)
				d := duration.Duration(val)
				flags.Var(&d, name, description)
			} else {
				val, _ := strconv.ParseInt(defaultValue, 10, 64)
				flags.Int64(name, val, description)
			}
		case reflect.Bool:
			val, _ := strconv.ParseBool(defaultValue)
			flags.Bool(name, val, description)
		case reflect.Slice:
			if field.Type.Elem().Kind() == reflect.String {
				var val []string
				if defaultValue != "" {
					val = strings.Split(defaultValue, ",")
				}
				flags.StringSlice(name, val, description)
			}
		}
	}
}
