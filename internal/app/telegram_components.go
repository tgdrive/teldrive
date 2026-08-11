package app

import (
	"fmt"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/botgateway"
	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/config"
	"github.com/tgdrive/teldrive/v2/internal/localtelegram"
	"github.com/tgdrive/teldrive/v2/internal/logingateway"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

type telegramComponents struct {
	login           authn.TelegramLogin
	verifier        bots.Verifier
	account         telegramstore.Account
	storage         telegramstore.Storage
	downloadClients *telegramstore.DownloadClientPool
}

func buildTelegramComponents(cfg config.Config, pool *pgxpool.Pool, cipher *secureblob.Cipher, injected telegramstore.Storage) (telegramComponents, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Telegram.Backend)) {
	case "filesystem":
		root, err := expandHomePath(cfg.Telegram.LocalRoot)
		if err != nil {
			return telegramComponents{}, fmt.Errorf("resolve local Telegram root: %w", err)
		}
		server, err := localtelegram.Open(root)
		if err != nil {
			return telegramComponents{}, fmt.Errorf("open local Telegram emulator: %w", err)
		}
		runner, err := localtelegram.NewRunner(server)
		if err != nil {
			return telegramComponents{}, err
		}
		account, err := telegramstore.NewGotdAccount(runner)
		if err != nil {
			return telegramComponents{}, fmt.Errorf("create local Telegram account gateway: %w", err)
		}
		storage := injected
		if storage == nil {
			storage = telegramstore.NewGotdStorage(runner)
		}
		return telegramComponents{
			login: localTelegramLogin{}, verifier: localBotVerifier{}, account: account, storage: storage,
		}, nil

	case "remote":
		factory, err := telegramstore.NewFactory(telegramstore.FactoryConfig{
			AppID: cfg.Telegram.AppID, AppHash: cfg.Telegram.AppHash,
			Device: telegram.DeviceConfig{
				DeviceModel: cfg.Telegram.DeviceModel, SystemVersion: cfg.Telegram.SystemVersion,
				AppVersion: cfg.Telegram.AppVersion, LangCode: cfg.Telegram.LanguageCode,
				SystemLangCode: cfg.Telegram.SystemLanguageCode, LangPack: cfg.Telegram.LanguagePack,
			},
			DialTimeout: cfg.Telegram.DialTimeout, ReconnectTimeout: cfg.Telegram.ReconnectTimeout,
			MaxRetries: cfg.Telegram.MaxRetries, RateLimit: cfg.Telegram.RateLimit,
			RateInterval: cfg.Telegram.RateInterval, RateBurst: cfg.Telegram.RateBurst,
			Proxy: cfg.Telegram.Proxy, MTProxyAddress: cfg.Telegram.MTProxy.Address,
			MTProxySecret: cfg.Telegram.MTProxy.Secret, AllowCDN: cfg.Telegram.AllowCDN,
		})
		if err != nil {
			return telegramComponents{}, fmt.Errorf("create Telegram client factory: %w", err)
		}
		login, err := logingateway.New(factory)
		if err != nil {
			return telegramComponents{}, fmt.Errorf("create Telegram login gateway: %w", err)
		}
		verifier, err := botgateway.NewGotdVerifier(factory)
		if err != nil {
			return telegramComponents{}, fmt.Errorf("create bot verifier: %w", err)
		}
		provider, err := telegramstore.NewDatabaseClientProvider(pool, cipher, factory)
		if err != nil {
			return telegramComponents{}, fmt.Errorf("create Telegram session provider: %w", err)
		}
		userRunner := telegramstore.ClientRunner{Provider: provider, Factory: factory}
		account, err := telegramstore.NewGotdAccount(userRunner)
		if err != nil {
			return telegramComponents{}, fmt.Errorf("create Telegram account gateway: %w", err)
		}
		storage := injected
		var downloadClients *telegramstore.DownloadClientPool
		if storage == nil {
			channelBotProvider, err := botgateway.NewChannelBotProvider(pool)
			if err != nil {
				return telegramComponents{}, fmt.Errorf("create channel bot provider: %w", err)
			}
			uploadRunner, err := botgateway.NewUploadAwareRunner(pool, cipher, factory, userRunner, cfg.Telegram.DownloadBots, cfg.Telegram.BotRotationBackend)
			if err != nil {
				return telegramComponents{}, fmt.Errorf("create upload-aware Telegram runner: %w", err)
			}
			options := []telegramstore.GotdStorageOption{telegramstore.WithBotProvider(channelBotProvider)}
			if cfg.Telegram.DownloadClientPool {
				downloadClients, err = telegramstore.NewDownloadClientPool(uploadRunner, telegramstore.DownloadClientPoolConfig{
					ClientsPerUser: cfg.Telegram.DownloadClientPoolSize,
					MaxClients:     cfg.Telegram.DownloadClientPoolMax,
					MaxSessions:    cfg.Telegram.DownloadClientMaxSessions,
					IdleTimeout:    cfg.Telegram.DownloadClientIdleTimeout,
					AcquireTimeout: cfg.Telegram.DownloadClientAcquireTimeout,
				})
				if err != nil {
					return telegramComponents{}, fmt.Errorf("create Telegram download client pool: %w", err)
				}
				options = append(options, telegramstore.WithDownloadClientPool(downloadClients))
			}
			storage = telegramstore.NewGotdStorage(uploadRunner, options...)
		}
		return telegramComponents{
			login: login, verifier: verifier, account: account, storage: storage, downloadClients: downloadClients,
		}, nil

	default:
		return telegramComponents{}, fmt.Errorf("unsupported Telegram backend %q", cfg.Telegram.Backend)
	}
}
