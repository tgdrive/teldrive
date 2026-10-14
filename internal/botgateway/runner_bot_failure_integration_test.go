//go:build integration

package botgateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

// Bots run through runBotFunc. A bot listed in failures fails before the
// callback runs, the way runBot reports a connect or authentication failure.
type botFailureFixture struct {
	pool     *pgxpool.Pool
	runner   *UploadAwareRunner
	fallback *recordingRunner

	mu       sync.Mutex
	attempts []int64
}

func newBotFailureFixture(t *testing.T, botIDs []int64, failures map[int64]error) *botFailureFixture {
	t.Helper()
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	for _, botID := range botIDs {
		if _, err := db.Pool.Exec(ctx, `
INSERT INTO bots (bot_id, user_id, token_ciphertext, enabled) VALUES ($1, 1001, $2, true)`, botID, []byte("ciphertext")); err != nil {
			t.Fatal(err)
		}
	}
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{3}, 32), bytes.NewReader(bytes.Repeat([]byte{4}, 24)))
	if err != nil {
		t.Fatal(err)
	}
	factory, err := telegramstore.NewFactory(telegramstore.FactoryConfig{AppID: 12345, AppHash: "test-hash"})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &botFailureFixture{pool: db.Pool, fallback: &recordingRunner{}}
	fixture.runner, err = NewUploadAwareRunner(db.Pool, cipher, factory, fixture.fallback, len(botIDs))
	if err != nil {
		t.Fatal(err)
	}
	fixture.runner.runBotFunc = func(ctx context.Context, _ int64, bot *sqlcgen.Bot, _ int, fn func(context.Context, *tg.Client) error) error {
		fixture.mu.Lock()
		fixture.attempts = append(fixture.attempts, bot.BotID)
		fixture.mu.Unlock()
		if err := failures[bot.BotID]; err != nil {
			return err
		}
		return fn(telegramstore.WithClientID(ctx, bot.BotID), new(tg.Client))
	}
	return fixture
}

func (f *botFailureFixture) attempted() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.attempts)
}

type botState struct {
	enabled      bool
	failures     int32
	lastError    string
	backedOff    bool
	retryAfterAt time.Time
}

func (f *botFailureFixture) bot(t *testing.T, botID int64) botState {
	t.Helper()
	bot, err := sqlcgen.New(f.pool).GetBot(context.Background(), sqlcgen.GetBotParams{UserID: 1001, BotID: botID})
	if err != nil {
		t.Fatal(err)
	}
	return botState{
		enabled: bot.Enabled, failures: bot.ConsecutiveFailures, lastError: bot.LastError.String,
		backedOff: bot.RetryAfter.Valid, retryAfterAt: bot.RetryAfter.Time,
	}
}

func revokedToken(kind string) error {
	return fmt.Errorf("authenticate: %w", tgerr.New(400, kind))
}

func TestUploadAwareRunnerDisablesRevokedBotAndRetriesOnNextBot(t *testing.T) {
	for _, tc := range []struct {
		operation telegramstore.Operation
		kind      string
	}{
		{telegramstore.OperationUpload, "ACCESS_TOKEN_EXPIRED"},
		{telegramstore.OperationUpload, "ACCESS_TOKEN_INVALID"},
		{telegramstore.OperationDownload, "ACCESS_TOKEN_EXPIRED"},
		{telegramstore.OperationDownload, "ACCESS_TOKEN_INVALID"},
	} {
		t.Run(string(tc.operation)+"/"+tc.kind, func(t *testing.T) {
			// Rotation starts at bot 101, so the revoked bot is tried first.
			f := newBotFailureFixture(t, []int64{101, 102}, map[int64]error{101: revokedToken(tc.kind)})
			var served []int64
			callback := func(ctx context.Context, _ *tg.Client) error {
				botID, _ := telegramstore.ClientID(ctx)
				served = append(served, botID)
				return nil
			}
			for range 3 {
				if err := f.runner.Run(context.Background(), 1001, tc.operation, callback); err != nil {
					t.Fatalf("Run() error = %v", err)
				}
			}
			if want := []int64{102, 102, 102}; !slices.Equal(served, want) {
				t.Fatalf("served by bots %v, want %v", served, want)
			}
			if want := []int64{101, 102, 102, 102}; !slices.Equal(f.attempted(), want) {
				t.Fatalf("attempted bots %v, want %v", f.attempted(), want)
			}
			revoked := f.bot(t, 101)
			if revoked.enabled || revoked.backedOff || revoked.failures != 1 || !strings.Contains(revoked.lastError, tc.kind) {
				t.Fatalf("revoked bot state = %+v, want disabled with %s and no backoff", revoked, tc.kind)
			}
			if healthy := f.bot(t, 102); !healthy.enabled || healthy.failures != 0 {
				t.Fatalf("healthy bot state = %+v", healthy)
			}
		})
	}
}

func TestUploadAwareRunnerFallsBackWhenEveryBotIsRevoked(t *testing.T) {
	f := newBotFailureFixture(t, []int64{101, 102}, map[int64]error{
		101: revokedToken("ACCESS_TOKEN_EXPIRED"),
		102: revokedToken("ACCESS_TOKEN_INVALID"),
	})
	called := false
	if err := f.runner.Run(context.Background(), 1001, telegramstore.OperationUpload, func(context.Context, *tg.Client) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called {
		t.Fatal("callback ran on a revoked bot")
	}
	if f.fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", f.fallback.calls)
	}
	for _, botID := range []int64{101, 102} {
		if f.bot(t, botID).enabled {
			t.Fatalf("bot %d still enabled", botID)
		}
	}
}

// Once the callback has run, its side effects may already exist on Telegram,
// so the call must fail instead of repeating the work on another bot.
func TestUploadAwareRunnerDoesNotRetryAfterCallbackStarted(t *testing.T) {
	f := newBotFailureFixture(t, []int64{101, 102}, nil)
	calls := 0
	err := f.runner.Run(context.Background(), 1001, telegramstore.OperationUpload, func(context.Context, *tg.Client) error {
		calls++
		return revokedToken("ACCESS_TOKEN_EXPIRED")
	})
	if !errors.Is(err, telegramstore.ErrAccountRevoked) || !tgerr.Is(err, "ACCESS_TOKEN_EXPIRED") {
		t.Fatalf("Run() error = %v, want revoked ACCESS_TOKEN_EXPIRED", err)
	}
	if calls != 1 {
		t.Fatalf("callback calls = %d, want 1", calls)
	}
	if f.bot(t, 101).enabled {
		t.Fatal("revoked bot still enabled")
	}
}

func TestUploadAwareRunnerBacksOffOtherUploadFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		// A stale session is re-authorized from the token on the next attempt.
		{"stale session", tgerr.New(401, "AUTH_KEY_UNREGISTERED")},
		// User-account errors are not bot token revocations.
		{"user account error", tgerr.New(401, "SESSION_REVOKED")},
		{"flood wait", tgerr.New(420, "FLOOD_WAIT_5")},
		{"network", errors.New("connection reset by peer")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBotFailureFixture(t, []int64{101, 102}, map[int64]error{101: tc.err})
			before := time.Now()
			err := f.runner.Run(context.Background(), 1001, telegramstore.OperationUpload, func(context.Context, *tg.Client) error { return nil })
			if err == nil || errors.Is(err, telegramstore.ErrAccountRevoked) {
				t.Fatalf("Run() error = %v, want transient bot failure", err)
			}
			if want := []int64{101}; !slices.Equal(f.attempted(), want) {
				t.Fatalf("attempted bots %v, want %v", f.attempted(), want)
			}
			state := f.bot(t, 101)
			if !state.enabled || !state.backedOff || state.failures != 1 {
				t.Fatalf("bot state = %+v, want enabled and backed off", state)
			}
			if wait := state.retryAfterAt.Sub(before); wait < 25*time.Second || wait > 35*time.Second {
				t.Fatalf("retry_after in %v, want about 30s", wait)
			}
			// The backed-off bot is skipped until retry_after passes, even when
			// rotation comes back around to it.
			for range 2 {
				if err := f.runner.Run(context.Background(), 1001, telegramstore.OperationUpload, func(context.Context, *tg.Client) error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if want := []int64{101, 102, 102}; !slices.Equal(f.attempted(), want) {
				t.Fatalf("attempted bots %v, want %v", f.attempted(), want)
			}
		})
	}
}

func TestUploadAwareRunnerBacksOffDownloadBotThatCannotConnect(t *testing.T) {
	f := newBotFailureFixture(t, []int64{101, 102}, map[int64]error{101: errors.New("dial tcp: connection refused")})
	before := time.Now()
	called := false
	if err := f.runner.Run(context.Background(), 1001, telegramstore.OperationDownload, func(context.Context, *tg.Client) error {
		called = true
		return nil
	}); err == nil || called {
		t.Fatalf("Run() error = %v, callback ran = %v; want connect failure before callback", err, called)
	}
	state := f.bot(t, 101)
	if !state.enabled || !state.backedOff || state.failures != 1 {
		t.Fatalf("bot state = %+v, want enabled and backed off", state)
	}
	if wait := state.retryAfterAt.Sub(before); wait < 25*time.Second || wait > 35*time.Second {
		t.Fatalf("retry_after in %v, want about 30s", wait)
	}
	// Rotation comes back around to bot 101, which is skipped while backing off.
	for range 2 {
		if err := f.runner.Run(context.Background(), 1001, telegramstore.OperationDownload, func(context.Context, *tg.Client) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if want := []int64{101, 102, 102}; !slices.Equal(f.attempted(), want) {
		t.Fatalf("attempted bots %v, want %v", f.attempted(), want)
	}
}

// Runs a real gotd client through a proxy that refuses connections. gotd would
// keep reconnecting for ReconnectTimeout, much longer than the request waits,
// so the bot must be backed off on the first failed connection attempt.
func TestUploadAwareRunnerBacksOffDownloadBotOnFirstFailedConnection(t *testing.T) {
	f := newBotFailureFixture(t, []int64{101}, nil)
	factory, err := telegramstore.NewFactory(telegramstore.FactoryConfig{
		AppID: 12345, AppHash: "test-hash", Proxy: "socks5://127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond, ReconnectTimeout: 5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.runner.factory = factory
	f.runner.runBotFunc = nil
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	called := false
	err = f.runner.Run(ctx, 1001, telegramstore.OperationDownload, func(context.Context, *tg.Client) error {
		called = true
		return nil
	})
	if !errors.Is(err, errBotUnreachable) || called {
		t.Fatalf("Run() error = %v, callback ran = %v; want %v before the callback", err, called, errBotUnreachable)
	}
	if state := f.bot(t, 101); !state.enabled || !state.backedOff || state.failures != 1 {
		t.Fatalf("bot state = %+v, want enabled and backed off", state)
	}
	if err := f.runner.Run(context.Background(), 1001, telegramstore.OperationDownload, func(context.Context, *tg.Client) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if f.fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want the backed-off bot skipped", f.fallback.calls)
	}
}

// Once the bot is connected, errors such as an expired file reference would
// fail on any bot.
func TestUploadAwareRunnerKeepsDownloadBotOnCallbackFailure(t *testing.T) {
	f := newBotFailureFixture(t, []int64{101, 102}, nil)
	if err := f.runner.Run(context.Background(), 1001, telegramstore.OperationDownload, func(context.Context, *tg.Client) error {
		return tgerr.New(400, "FILE_REFERENCE_EXPIRED")
	}); !tgerr.Is(err, "FILE_REFERENCE_EXPIRED") {
		t.Fatalf("Run() error = %v, want FILE_REFERENCE_EXPIRED", err)
	}
	if state := f.bot(t, 101); !state.enabled || state.backedOff || state.failures != 0 {
		t.Fatalf("bot state = %+v, want untouched", state)
	}
}

// A backed-off bot leaves a gap in the first download-bots bots rather than
// letting a bot beyond the limit serve downloads.
func TestUploadAwareRunnerKeepsDownloadBotLimitWhileBackingOff(t *testing.T) {
	f := newBotFailureFixture(t, []int64{101, 102, 103}, nil)
	f.runner.downloadBots = 2
	backOff := func(botID int64) {
		t.Helper()
		if _, err := f.pool.Exec(context.Background(), `
UPDATE bots SET retry_after = now() + interval '30 seconds' WHERE user_id = 1001 AND bot_id = $1`, botID); err != nil {
			t.Fatal(err)
		}
	}
	backOff(101)
	for range 3 {
		if err := f.runner.Run(context.Background(), 1001, telegramstore.OperationDownload, func(context.Context, *tg.Client) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if want := []int64{102, 102, 102}; !slices.Equal(f.attempted(), want) {
		t.Fatalf("attempted bots %v, want %v", f.attempted(), want)
	}
	backOff(102)
	if err := f.runner.Run(context.Background(), 1001, telegramstore.OperationDownload, func(context.Context, *tg.Client) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(f.attempted()) != 3 || f.fallback.calls != 1 {
		t.Fatalf("attempted bots %v, fallback calls %d; want user account fallback", f.attempted(), f.fallback.calls)
	}
}

func TestDownloadClientPoolBacksOffBotThatCannotConnect(t *testing.T) {
	f := newBotFailureFixture(t, []int64{101, 102}, map[int64]error{101: errors.New("dial tcp: connection refused")})
	pool, err := telegramstore.NewDownloadClientPool(f.runner, telegramstore.DownloadClientPoolConfig{ReadParallel: 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := pool.Close(closeCtx); err != nil {
			t.Error(err)
		}
	})
	if _, err := pool.OpenDownloadSession(context.Background(), 1001); err == nil {
		t.Fatal("OpenDownloadSession() error = nil, want connect failure")
	}
	if state := f.bot(t, 101); !state.enabled || !state.backedOff {
		t.Fatalf("bot 101 state = %+v, want backed off", state)
	}
	for range 2 {
		session, err := pool.OpenDownloadSession(context.Background(), 1001)
		if err != nil {
			t.Fatalf("OpenDownloadSession() error = %v", err)
		}
		_ = session.Close()
	}
	// The second session reuses the warm client on bot 102.
	if want := []int64{101, 102}; !slices.Equal(f.attempted(), want) {
		t.Fatalf("attempted bots %v, want %v", f.attempted(), want)
	}
}

func TestDownloadClientPoolSkipsRevokedBot(t *testing.T) {
	f := newBotFailureFixture(t, []int64{101, 102}, map[int64]error{101: revokedToken("ACCESS_TOKEN_EXPIRED")})
	pool, err := telegramstore.NewDownloadClientPool(f.runner, telegramstore.DownloadClientPoolConfig{ReadParallel: 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := pool.Close(closeCtx); err != nil {
			t.Error(err)
		}
	})
	for range 2 {
		session, err := pool.OpenDownloadSession(context.Background(), 1001)
		if err != nil {
			t.Fatalf("OpenDownloadSession() error = %v", err)
		}
		_ = session.Close()
	}
	// The second session reuses the warm client on bot 102.
	if want := []int64{101, 102}; !slices.Equal(f.attempted(), want) {
		t.Fatalf("attempted bots %v, want %v", f.attempted(), want)
	}
	if f.bot(t, 101).enabled {
		t.Fatal("revoked bot still enabled")
	}
}
