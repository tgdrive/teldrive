package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestDefaultIncludesLegacyPublicTelegramClientIdentity(t *testing.T) {
	t.Parallel()

	cfg := Default()
	if !cfg.Database.AutoMigrateLegacy {
		t.Fatal("Database AutoMigrateLegacy = false, want true")
	}
	if cfg.Telegram.AppID != 2496 {
		t.Fatalf("Telegram AppID = %d, want 2496", cfg.Telegram.AppID)
	}
	if cfg.Telegram.AppHash != "8da85b0d5bfe62527e5b244c209159c3" {
		t.Fatalf("Telegram AppHash = %q", cfg.Telegram.AppHash)
	}
	if cfg.Telegram.DeviceModel == "" || cfg.Telegram.SystemVersion != "Win32" || cfg.Telegram.AppVersion != "6.1.4 K" || cfg.Telegram.LanguagePack != "webk" {
		t.Fatalf("Telegram device defaults = %#v", cfg.Telegram)
	}
	if cfg.Telegram.DownloadBots != 0 {
		t.Fatalf("Telegram DownloadBots = %d, want authenticated-user fallback", cfg.Telegram.DownloadBots)
	}
	if cfg.Telegram.DownloadClientPool || cfg.Telegram.DownloadClientPoolSize != 4 || cfg.Telegram.DownloadClientMaxSessions != 4 || cfg.Telegram.DownloadReadBuffers != 32 || cfg.Telegram.DownloadReadParallel != 4 {
		t.Fatalf("Telegram download defaults = %#v", cfg.Telegram)
	}
	if cfg.Telegram.ClientLogging {
		t.Fatal("Telegram ClientLogging = true, want false")
	}
	if cfg.Telegram.RateInterval != 50*time.Millisecond || cfg.Telegram.RateBurst != 10 {
		t.Fatalf("Telegram rate defaults = interval %s, burst %d", cfg.Telegram.RateInterval, cfg.Telegram.RateBurst)
	}
	if cfg.Uploads.SessionTTL != 7*24*time.Hour {
		t.Fatalf("Uploads SessionTTL = %s, want 168h", cfg.Uploads.SessionTTL)
	}
	if cfg.Cache.Stream.Dir != "" || cfg.Cache.Stream.MaxSize.String() != "50GB" || cfg.Cache.Stream.ShardDepth != 1 || cfg.Cache.Stream.ChunkStreams != 4 {
		t.Fatalf("stream cache defaults = %#v", cfg.Cache.Stream)
	}
}

func TestValidateRejectsInvalidTrustedProxy(t *testing.T) {
	t.Parallel()
	cfg := validTestConfig()
	cfg.HTTP.TrustedProxies = []string{"not-an-address"}
	if err := cfg.Validate(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "trusted proxy") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsDownloadPoolSizeAboveGlobalMaximum(t *testing.T) {
	t.Parallel()
	cfg := validTestConfig()
	cfg.Telegram.DownloadClientPoolSize = 4
	cfg.Telegram.DownloadClientPoolMax = 2
	if err := cfg.Validate(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "pool size") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadFromAppliesEnvironment(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"TELDRIVE_DATABASE_URL":                    "postgres://example/teldrive",
		"TELDRIVE_TELEGRAM_APP_ID":                 "12345",
		"TELDRIVE_TELEGRAM_APP_HASH":               "telegram-app-hash",
		"TELDRIVE_SECURITY_SIGNING_KEY":            "0123456789abcdef0123456789abcdef",
		"TELDRIVE_SECURITY_DATA_KEY":               "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"TELDRIVE_HTTP_ADDRESS":                    "0.0.0.0:9090",
		"TELDRIVE_DATABASE_MAX_CONNECTIONS":        "12",
		"TELDRIVE_DATABASE_MIN_CONNECTIONS":        "3",
		"TELDRIVE_DATABASE_AUTO_MIGRATE_LEGACY":    "false",
		"TELDRIVE_TELEGRAM_UPLOAD_THREADS":         "6",
		"TELDRIVE_TELEGRAM_DOWNLOAD_BOTS":          "3",
		"TELDRIVE_TELEGRAM_DOWNLOAD_READ_BUFFERS":  "24",
		"TELDRIVE_TELEGRAM_DOWNLOAD_READ_PARALLEL": "8",
		"TELDRIVE_TELEGRAM_RANDOMIZE_PART_NAMES":   "false",
		"TELDRIVE_TELEGRAM_AUTO_CHANNEL_CREATE":    "false",
		"TELDRIVE_TELEGRAM_CHANNEL_PART_LIMIT":     "12345",
		"TELDRIVE_TELEGRAM_CLIENT_LOGGING":         "true",
		"TELDRIVE_ENCRYPTION_ACTIVE_KEY_VERSION":   "2",
		"TELDRIVE_ENCRYPTION_KEYS":                 "1:first-secret,2:second-secret",
		"TELDRIVE_UPLOADS_SESSION_TTL":             "72h",
		"TELDRIVE_CACHE_STREAM_DIR":                "/tmp/teldrive-stream-cache",
		"TELDRIVE_CACHE_STREAM_MAX_SIZE":           "12GB",
		"TELDRIVE_CACHE_STREAM_SHARD_DEPTH":        "2",
		"TELDRIVE_LOGGING_LOG_LEVEL":               "debug",
		"TELDRIVE_LOGGING_LOG_FORMAT":              "text",
	}
	cfg, err := LoadFrom(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.HTTP.Address != "0.0.0.0:9090" || cfg.Database.URL != values["TELDRIVE_DATABASE_URL"] {
		t.Fatalf("loaded addresses = %#v", cfg)
	}
	if cfg.Database.MaxConnections != 12 || cfg.Database.MinConnections != 3 || cfg.Database.AutoMigrateLegacy {
		t.Fatalf("database pool = %#v", cfg.Database)
	}
	if cfg.Telegram.UploadThreads != 6 || cfg.Telegram.DownloadBots != 3 || cfg.Telegram.DownloadReadBuffers != 24 || cfg.Telegram.DownloadReadParallel != 8 || !cfg.Telegram.ClientLogging || cfg.Telegram.RandomizePartNames || cfg.Telegram.AutoChannelCreate || cfg.Telegram.ChannelPartLimit != 12345 {
		t.Fatalf("Telegram config = %#v", cfg.Telegram)
	}
	if cfg.Encryption.ActiveKeyVersion != 2 || cfg.Encryption.Keys[2] != "second-secret" {
		t.Fatalf("encryption config = %#v", cfg.Encryption)
	}
	if cfg.Logging.LogLevel != "debug" || cfg.Logging.LogFormat != "text" {
		t.Fatalf("logging config = %#v", cfg.Logging)
	}
	if cfg.Uploads.SessionTTL != 72*time.Hour {
		t.Fatalf("upload config = %#v", cfg.Uploads)
	}
	if cfg.Cache.Stream.Dir != "/tmp/teldrive-stream-cache" || cfg.Cache.Stream.MaxSize.String() != "12GB" || cfg.Cache.Stream.ShardDepth != 2 {
		t.Fatalf("stream cache config = %#v", cfg.Cache.Stream)
	}
}

func TestLoadFromRejectsInvalidValuesWithoutLeakingSecrets(t *testing.T) {
	t.Parallel()
	secret := "do-not-print-this-secret"
	_, err := LoadFrom(func(key string) (string, bool) {
		switch key {
		case "TELDRIVE_DATABASE_URL":
			return "postgres://example/teldrive", true
		case "TELDRIVE_ENCRYPTION_KEYS":
			return "bad-version:" + secret, true
		default:
			return "", false
		}
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("LoadFrom() error = %v, want ErrInvalid", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("configuration error leaked secret: %v", err)
	}
}

func TestValidateRequiresConfiguredActiveEncryptionKey(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Database.URL = "postgres://example/teldrive"
	cfg.Encryption.ActiveKeyVersion = 3
	cfg.Encryption.Keys = map[int32]string{2: "other"}
	if err := cfg.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRequiresActiveVersionWhenEncryptionKeysConfigured(t *testing.T) {
	t.Parallel()
	cfg := validTestConfig()
	cfg.Encryption.Keys = map[int32]string{1: "key"}
	if err := cfg.Validate(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "active encryption key version is required") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestParseEncryptionKeysRejectsDuplicates(t *testing.T) {
	t.Parallel()
	if _, err := parseEncryptionKeys("1:first,1:second"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("parseEncryptionKeys() error = %v", err)
	}
}

func TestLoadUsesProcessEnvironment(t *testing.T) {
	t.Setenv("TELDRIVE_DATABASE_URL", "postgres://example/process")
	t.Setenv("TELDRIVE_TELEGRAM_APP_ID", "12345")
	t.Setenv("TELDRIVE_TELEGRAM_APP_HASH", "telegram-app-hash")
	t.Setenv("TELDRIVE_SECURITY_SIGNING_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("TELDRIVE_SECURITY_DATA_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("TELDRIVE_TELEGRAM_UPLOAD_THREADS", "4")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Database.URL != "postgres://example/process" || cfg.Telegram.UploadThreads != 4 {
		t.Fatalf("Load() = %#v", cfg)
	}
}

func TestLoadFromRejectsMalformedScalarValues(t *testing.T) {
	t.Parallel()
	invalid := []struct {
		name  string
		key   string
		value string
	}{
		{name: "duration", key: "TELDRIVE_HTTP_READ_TIMEOUT", value: "tomorrow"},
		{name: "boolean", key: "TELDRIVE_TELEGRAM_AUTO_CHANNEL_CREATE", value: "sometimes"},
		{name: "integer", key: "TELDRIVE_TELEGRAM_UPLOAD_THREADS", value: "many"},
		{name: "int32", key: "TELDRIVE_DATABASE_MAX_CONNECTIONS", value: "huge"},
		{name: "int64", key: "TELDRIVE_TELEGRAM_CHANNEL_PART_LIMIT", value: "lots"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadFrom(func(key string) (string, bool) {
				if key == "TELDRIVE_DATABASE_URL" {
					return "postgres://example/teldrive", true
				}
				if key == test.key {
					return test.value, true
				}
				return "", false
			})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("LoadFrom() error = %v", err)
			}
		})
	}
	if _, err := LoadFrom(nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil lookup error = %v", err)
	}
}

func TestValidateCoversAllCriticalStartupConstraints(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.HTTP.Address = ""
	cfg.HTTP.ReadHeaderTimeout = -1
	cfg.HTTP.ReadTimeout = -1
	cfg.HTTP.WriteTimeout = -1
	cfg.HTTP.IdleTimeout = -1
	cfg.HTTP.ShutdownTimeout = -1
	cfg.Database.URL = ""
	cfg.Database.MaxConnections = 0
	cfg.Database.MinConnections = -1
	cfg.Telegram.AppID = 0
	cfg.Telegram.AppHash = ""
	cfg.Telegram.UploadThreads = 0
	cfg.Telegram.ChannelPartLimit = 0
	cfg.Telegram.ChannelNamePrefix = ""
	cfg.Encryption.ActiveKeyVersion = -1
	cfg.Encryption.Keys = map[int32]string{0: "", 2: ""}
	cfg.Security.SigningKey = "short"
	cfg.Security.DataKey = ""
	cfg.Security.Issuer = ""
	cfg.Security.AccessTokenTTL = 0
	cfg.Security.RefreshTokenTTL = 0
	cfg.Security.LoginFlowTTL = 0
	if err := cfg.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Validate() error = %v", err)
	}

	valid := Default()
	valid.Database.URL = "postgres://example/teldrive"
	valid.Telegram.AppID = 12345
	valid.Telegram.AppHash = "telegram-app-hash"
	valid.Security.SigningKey = "0123456789abcdef0123456789abcdef"
	valid.Security.DataKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid configuration rejected: %v", err)
	}
}

func TestLoaderPrecedenceFileEnvironmentAndFlags(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := `
http:
  address: "127.0.0.1:7000"
database:
  url: "postgres://file/teldrive"
telegram:
  app-id: 100
  app-hash: "from-file"
  upload-threads: 3
security:
  signing-key: "0123456789abcdef0123456789abcdef"
  data-key: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"TELDRIVE_HTTP_ADDRESS":            "127.0.0.1:8000",
		"TELDRIVE_TELEGRAM_UPLOAD_THREADS": "5",
	}
	loader := newLoader(func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}, func() (string, error) { return "", nil })
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	loader.RegisterFlags(flags)
	if err := flags.Parse([]string{
		"--config", path,
		"--http-address", "127.0.0.1:9000",
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := loader.Load(flags)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Address != "127.0.0.1:9000" {
		t.Fatalf("flag did not override environment and file: %q", cfg.HTTP.Address)
	}
	if cfg.Telegram.UploadThreads != 5 {
		t.Fatalf("environment did not override file: %d", cfg.Telegram.UploadThreads)
	}
	if cfg.Database.URL != "postgres://file/teldrive" || cfg.Telegram.AppHash != "from-file" {
		t.Fatalf("config file values not loaded: %#v", cfg)
	}
}

func TestLoaderRejectsUnsupportedConfigExtension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := newLoader(func(string) (string, bool) { return "", false }, func() (string, error) { return "", nil })
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	loader.RegisterFlags(flags)
	if err := flags.Parse([]string{"--config", path}); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(flags); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Load() error = %v", err)
	}
}
