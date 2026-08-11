//go:build telegramlive

package telegramstore

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
)

// TestLiveTelegramAccountSmoke is read-only and opt-in. Run it with:
//
//	TELEGRAM_LIVE_APP_ID=... \
//	TELEGRAM_LIVE_APP_HASH=... \
//	TELEGRAM_LIVE_SESSION=... \
//	go test -tags=telegramlive ./internal/telegramstore -run TestLiveTelegramAccountSmoke
func TestLiveTelegramAccountSmoke(t *testing.T) {
	appIDText := os.Getenv("TELEGRAM_LIVE_APP_ID")
	appHash := os.Getenv("TELEGRAM_LIVE_APP_HASH")
	sessionText := os.Getenv("TELEGRAM_LIVE_SESSION")
	if appIDText == "" || appHash == "" || sessionText == "" {
		t.Skip("TELEGRAM_LIVE_APP_ID, TELEGRAM_LIVE_APP_HASH, and TELEGRAM_LIVE_SESSION are required")
	}
	appID, err := strconv.Atoi(appIDText)
	if err != nil || appID <= 0 {
		t.Fatalf("invalid TELEGRAM_LIVE_APP_ID: %q", appIDText)
	}
	data, err := session.TelethonSession(sessionText)
	if err != nil {
		t.Fatalf("parse TELEGRAM_LIVE_SESSION: %v", err)
	}
	memory := &session.StorageMemory{}
	loader := session.Loader{Storage: memory}
	if err := loader.Save(context.Background(), data); err != nil {
		t.Fatalf("load Telegram session: %v", err)
	}
	factory, err := NewFactory(FactoryConfig{
		AppID: appID, AppHash: appHash, DialTimeout: 15 * time.Second,
		ReconnectTimeout: time.Minute, MaxRetries: 3,
		RateLimit: true, RateInterval: 100 * time.Millisecond, RateBurst: 3,
		Device: telegram.DeviceConfig{DeviceModel: "TelDrive live smoke test", AppVersion: "2"},
	})
	if err != nil {
		t.Fatalf("create Telegram factory: %v", err)
	}
	runner := ClientRunner{Provider: ClientProviderFunc(func(context.Context, int64, Operation) (*telegram.Client, error) {
		return factory.New(memory)
	})}
	account, err := NewGotdAccount(runner)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	channels, err := account.DiscoverChannels(ctx, 1)
	if err != nil {
		t.Fatalf("discover channels: %v", err)
	}
	t.Logf("discovered %d manageable Telegram channels", len(channels))
	photo, found, err := account.ProfilePhoto(ctx, 1)
	if err != nil {
		t.Fatalf("get profile photo: %v", err)
	}
	if found && (photo.PhotoID == 0 || len(photo.Content) == 0) {
		t.Fatalf("invalid profile photo result: %#v", photo)
	}
	t.Logf("profile photo found=%v bytes=%d", found, len(photo.Content))
}
