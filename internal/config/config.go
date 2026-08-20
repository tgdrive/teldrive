package config

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/database"
	"github.com/tgdrive/teldrive/v2/internal/size"
)

const envPrefix = "TELDRIVE_"

var ErrInvalid = errors.New("invalid configuration")

type HTTP struct {
	Address           string        `koanf:"address" default:"127.0.0.1:8080" validate:"required" description:"HTTP listen address"`
	ReadHeaderTimeout time.Duration `koanf:"read-header-timeout" default:"10s" validate:"gte=0" description:"Maximum time to read request headers"`
	ReadTimeout       time.Duration `koanf:"read-timeout" default:"30s" validate:"gte=0" description:"Maximum time to read an entire request"`
	WriteTimeout      time.Duration `koanf:"write-timeout" default:"0s" validate:"gte=0" description:"Maximum response write duration; zero disables it for streaming"`
	IdleTimeout       time.Duration `koanf:"idle-timeout" default:"2m" validate:"gte=0" description:"HTTP keep-alive idle timeout"`
	ShutdownTimeout   time.Duration `koanf:"shutdown-timeout" default:"10s" validate:"gte=0" description:"Graceful shutdown timeout"`
	TrustedProxies    []string      `koanf:"trusted-proxies" default:"" description:"Proxy IP addresses or CIDRs trusted to set forwarding headers"`
}

type TelegramMTProxy struct {
	Address string `koanf:"address" default:"" description:"MTProto proxy address in host:port form"`
	Secret  string `koanf:"secret" default:"" description:"MTProto proxy secret in hexadecimal form"`
}

type Telegram struct {
	Backend                      string          `koanf:"backend" default:"remote" validate:"oneof=remote filesystem" description:"Telegram backend: remote or filesystem"`
	LocalRoot                    string          `koanf:"local-root" default:"./var/local-telegram" validate:"required_if=Backend filesystem" description:"Filesystem root used by the local Telegram emulator"`
	AppID                        int             `koanf:"app-id" default:"2496" validate:"required_if=Backend remote,omitempty,gt=0" description:"Telegram application ID"`
	AppHash                      string          `koanf:"app-hash" default:"8da85b0d5bfe62527e5b244c209159c3" validate:"required_if=Backend remote" description:"Telegram application hash"`
	DeviceModel                  string          `koanf:"device-model" default:"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0" validate:"required" description:"Telegram client device model"`
	SystemVersion                string          `koanf:"system-version" default:"Win32" validate:"required" description:"Telegram client system version"`
	AppVersion                   string          `koanf:"app-version" default:"6.1.4 K" validate:"required" description:"Telegram client application version"`
	LanguageCode                 string          `koanf:"language-code" default:"en" validate:"required" description:"Telegram client language code"`
	SystemLanguageCode           string          `koanf:"system-language-code" default:"en-US" validate:"required" description:"Telegram client system language code"`
	LanguagePack                 string          `koanf:"language-pack" default:"webk" description:"Telegram client language pack"`
	DialTimeout                  time.Duration   `koanf:"dial-timeout" default:"10s" validate:"gt=0" description:"Telegram connection timeout"`
	ReconnectTimeout             time.Duration   `koanf:"reconnect-timeout" default:"5m" validate:"gt=0" description:"Maximum Telegram reconnect backoff duration"`
	MaxRetries                   int             `koanf:"max-retries" default:"10" validate:"gte=0" description:"Maximum Telegram transport retry attempts"`
	RateLimit                    bool            `koanf:"rate-limit" default:"true" description:"Enable Telegram API request rate limiting"`
	RateInterval                 time.Duration   `koanf:"rate-interval" default:"100ms" validate:"gt=0" description:"Minimum interval between Telegram API requests"`
	RateBurst                    int             `koanf:"rate-burst" default:"5" validate:"gt=0" description:"Telegram API request burst allowance"`
	Proxy                        string          `koanf:"proxy" default:"" description:"HTTP, HTTPS, or SOCKS5 proxy URL"`
	MTProxy                      TelegramMTProxy `koanf:"mtproxy"`
	AllowCDN                     bool            `koanf:"allow-cdn" default:"false" description:"Allow Telegram CDN redirects"`
	UploadThreads                int             `koanf:"upload-threads" default:"8" validate:"min=1,max=32" description:"Concurrent Telegram upload workers"`
	DownloadBots                 int             `koanf:"download-bots" default:"0" validate:"min=0,max=32" description:"Maximum enabled bots used for download rotation; zero uses the authenticated user"`
	DownloadClientPool           bool            `koanf:"download-client-pool" default:"false" description:"Keep authenticated Telegram download clients warm between HTTP requests"`
	DownloadClientPoolSize       int             `koanf:"download-client-pool-size" default:"4" validate:"min=1,max=32" description:"Maximum warm Telegram download clients per user"`
	DownloadClientPoolMax        int             `koanf:"download-client-pool-max" default:"32" validate:"min=1,max=256" description:"Maximum warm Telegram download clients on this instance"`
	DownloadClientMaxSessions    int             `koanf:"download-client-max-sessions" default:"4" validate:"min=1,max=64" description:"Maximum concurrent download sessions sharing one Telegram client"`
	DownloadClientIdleTimeout    time.Duration   `koanf:"download-client-idle-timeout" default:"5m" validate:"gt=0" description:"Idle time before a warm Telegram download client is closed"`
	DownloadClientAcquireTimeout time.Duration   `koanf:"download-client-acquire-timeout" default:"10s" validate:"gt=0" description:"Maximum wait for a Telegram download client lease"`
	RandomizePartNames           bool            `koanf:"randomize-part-names" default:"true" description:"Randomize Telegram document names"`
	AutoChannelCreate            bool            `koanf:"auto-channel-create" default:"true" description:"Create storage channels automatically"`
	BotRotationBackend           string          `koanf:"bot-rotation-backend" default:"memory" validate:"oneof=memory database" description:"Bot rotation backend: memory for single-instance speed or database for cluster-wide coordination"`
	ChannelPartLimit             int64           `koanf:"channel-part-limit" default:"500000" validate:"gt=0" description:"Maximum parts stored in one Telegram channel"`
	ChannelNamePrefix            string          `koanf:"channel-name-prefix" default:"teldrive" validate:"required" description:"Prefix for automatically created channels"`
}

type Encryption struct {
	ActiveKeyVersion int32            `koanf:"active-key-version" default:"0" validate:"gte=0" description:"Active server-managed encryption key version for new uploads"`
	Keys             map[int32]string `koanf:"keys" default:"" description:"Encryption keys as comma-separated version:key entries"`
}

type Security struct {
	SigningKey      string        `koanf:"signing-key" default:"" validate:"required,min=32" description:"JWT signing key"`
	DataKey         string        `koanf:"data-key" default:"" validate:"required" description:"Key used to encrypt stored Telegram credentials"`
	Issuer          string        `koanf:"issuer" default:"teldrive-v2" validate:"required" description:"JWT issuer"`
	AllowedUsers    []string      `koanf:"allowed-users" default:"" description:"Allowed Telegram usernames; empty permits every user"`
	AccessTokenTTL  time.Duration `koanf:"access-token-ttl" default:"15m" validate:"gt=0" description:"Access-token lifetime"`
	RefreshTokenTTL time.Duration `koanf:"refresh-token-ttl" default:"720h" validate:"gt=0" description:"Refresh-token lifetime"`
	LoginFlowTTL    time.Duration `koanf:"login-flow-ttl" default:"10m" validate:"gt=0" description:"Telegram login-flow lifetime"`
}

type Logging struct {
	LogLevel  string `koanf:"log-level" default:"info" validate:"oneof=debug info warn error" description:"Log level: debug, info, warn, or error"`
	LogFormat string `koanf:"log-format" default:"json" validate:"oneof=json text" description:"Log format: json or text"`
}

type Events struct {
	BatchSize             int           `koanf:"batch-size" default:"100" validate:"min=1,max=1000" description:"Maximum events read from PostgreSQL per SSE batch"`
	MaxConnectionsPerUser int           `koanf:"max-connections-per-user" default:"5" validate:"min=1,max=1000" description:"Maximum concurrent SSE connections per user on one API instance"`
	Heartbeat             time.Duration `koanf:"heartbeat" default:"20s" validate:"gt=0" description:"SSE heartbeat interval"`
	WriteTimeout          time.Duration `koanf:"write-timeout" default:"10s" validate:"gt=0" description:"Maximum duration for one SSE write and flush"`
	TicketTTL             time.Duration `koanf:"ticket-ttl" default:"2m" validate:"gt=0" description:"Lifetime of browser event stream tickets"`
	Retention             time.Duration `koanf:"retention" default:"168h" validate:"gt=0" description:"Duration to retain replayable user events"`
	CleanupInterval       time.Duration `koanf:"cleanup-interval" default:"1h" validate:"gt=0" description:"Expired event and ticket cleanup interval"`
	ConnectTimeout        time.Duration `koanf:"connect-timeout" default:"10s" validate:"gt=0" description:"PostgreSQL event listener connection timeout"`
	PingInterval          time.Duration `koanf:"ping-interval" default:"5s" validate:"gt=0" description:"PostgreSQL event listener health-check interval"`
	ReconnectMin          time.Duration `koanf:"reconnect-min" default:"100ms" validate:"gt=0" description:"Minimum PostgreSQL listener reconnect delay"`
	ReconnectMax          time.Duration `koanf:"reconnect-max" default:"30s" validate:"gt=0" description:"Maximum PostgreSQL listener reconnect delay"`
}

type Uploads struct {
	SessionTTL time.Duration `koanf:"session-ttl" default:"168h" validate:"gt=0" description:"Lifetime of resumable upload sessions"`
}

type StreamCache struct {
	Dir          string        `koanf:"dir" default:"" description:"Sparse stream cache directory; empty disables caching"`
	MaxAge       time.Duration `koanf:"max-age" default:"168h" validate:"gt=0" description:"Maximum age of cached stream data"`
	MaxSize      size.Size     `koanf:"max-size" default:"50GB" validate:"gt=0" description:"Maximum stream cache size"`
	MinFreeSpace size.Size     `koanf:"min-free-space" default:"10GB" validate:"gte=0" description:"Minimum free disk space preserved by cache eviction"`
	PollInterval time.Duration `koanf:"poll-interval" default:"5m" validate:"gt=0" description:"Stream cache eviction interval"`
	ShardDepth   int           `koanf:"shard-depth" default:"1" validate:"min=0,max=16" description:"Number of two-character cache directory shard levels"`
	ChunkSize    size.Size     `koanf:"chunk-size" default:"32MB" validate:"gt=0" description:"Origin range fetch chunk size"`
	ChunkStreams int           `koanf:"chunk-streams" default:"4" validate:"min=0,max=32" description:"Concurrent origin range streams"`
	ReadAhead    size.Size     `koanf:"read-ahead" default:"4MB" validate:"gte=0" description:"Bytes fetched ahead of requested stream ranges"`
}

type Cache struct {
	Stream StreamCache `koanf:"stream"`
}

type Config struct {
	HTTP       HTTP            `koanf:"http"`
	Database   database.Config `koanf:"database"`
	Telegram   Telegram        `koanf:"telegram"`
	Encryption Encryption      `koanf:"encryption"`
	Security   Security        `koanf:"security"`
	Logging    Logging         `koanf:"logging"`
	Events     Events          `koanf:"events"`
	Uploads    Uploads         `koanf:"uploads"`
	Cache      Cache           `koanf:"cache"`
}

func Default() Config {
	var cfg Config
	if err := applyDefaults(&cfg); err != nil {
		panic(err)
	}
	return cfg
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Security.DataKey) == "" {
		return fmt.Errorf("%w: security.data-key is required; set security.data-key or TELDRIVE_SECURITY_DATA_KEY before starting Teldrive", ErrInvalid)
	}
	problems := validateTaggedFields(c)

	if c.Database.MinConnections > c.Database.MaxConnections {
		problems = append(problems, "database min connections cannot exceed max connections")
	}
	for _, proxy := range c.HTTP.TrustedProxies {
		value := strings.TrimSpace(proxy)
		if _, err := netip.ParseAddr(value); err != nil {
			if _, prefixErr := netip.ParsePrefix(value); prefixErr != nil {
				problems = append(problems, fmt.Sprintf("HTTP trusted proxy %q is not an IP address or CIDR", proxy))
			}
		}
	}

	mtAddress := strings.TrimSpace(c.Telegram.MTProxy.Address)
	mtSecret := strings.TrimSpace(c.Telegram.MTProxy.Secret)
	if (mtAddress == "") != (mtSecret == "") {
		problems = append(problems, "Telegram MTProxy address and secret must be configured together")
	}
	if mtAddress != "" && strings.TrimSpace(c.Telegram.Proxy) != "" {
		problems = append(problems, "Telegram proxy and MTProxy cannot be used together")
	}
	if c.Telegram.DownloadClientPoolSize > c.Telegram.DownloadClientPoolMax {
		problems = append(problems, "Telegram download client pool size cannot exceed the global maximum")
	}

	if len(c.Encryption.Keys) > 0 && c.Encryption.ActiveKeyVersion == 0 {
		problems = append(problems, "active encryption key version is required when encryption keys are configured")
	}
	if c.Encryption.ActiveKeyVersion > 0 {
		key, ok := c.Encryption.Keys[c.Encryption.ActiveKeyVersion]
		if !ok || strings.TrimSpace(key) == "" {
			problems = append(problems, "active encryption key version has no configured key")
		}
	}
	for version, key := range c.Encryption.Keys {
		if version <= 0 || strings.TrimSpace(key) == "" {
			problems = append(problems, fmt.Sprintf("encryption key version %d is invalid", version))
		}
	}

	for _, username := range c.Security.AllowedUsers {
		if strings.TrimSpace(strings.TrimPrefix(username, "@")) == "" {
			problems = append(problems, "security allowed users cannot contain an empty username")
			break
		}
	}
	if c.Events.ReconnectMax < c.Events.ReconnectMin {
		problems = append(problems, "event reconnect maximum must not be less than minimum")
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return nil
}

func parseEncryptionKeys(raw string) (map[int32]string, error) {
	keys := make(map[int32]string)
	if strings.TrimSpace(raw) == "" {
		return keys, nil
	}
	for entry := range strings.SplitSeq(raw, ",") {
		versionText, key, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok {
			return nil, fmt.Errorf("%w: encryption keys must use version:key entries", ErrInvalid)
		}
		version64, err := strconv.ParseInt(strings.TrimSpace(versionText), 10, 32)
		if err != nil || version64 <= 0 || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("%w: an encryption key entry is invalid", ErrInvalid)
		}
		version := int32(version64)
		if _, exists := keys[version]; exists {
			return nil, fmt.Errorf("%w: duplicate encryption key version %d", ErrInvalid, version)
		}
		keys[version] = key
	}
	return keys, nil
}
