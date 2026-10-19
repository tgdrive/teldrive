//go:build integration

package botgateway

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/tg"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestUploadAwareRunnerFallsBackWithoutEnabledBots(t *testing.T) {
	db := testpostgres.New(t)
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{3}, 32), bytes.NewReader(bytes.Repeat([]byte{4}, 24)))
	if err != nil {
		t.Fatal(err)
	}
	factory, err := telegramstore.NewFactory(telegramstore.FactoryConfig{AppID: 12345, AppHash: "test-hash"})
	if err != nil {
		t.Fatal(err)
	}
	fallback := &recordingRunner{}
	runner, err := NewUploadAwareRunner(db.Pool, cipher, factory, fallback, 2)
	if err != nil {
		t.Fatal(err)
	}
	callback := func(context.Context, *tg.Client) error { return nil }
	if err := runner.Run(context.Background(), 1001, telegramstore.OperationUpload, callback); err != nil {
		t.Fatalf("upload fallback error = %v", err)
	}
	if err := runner.Run(context.Background(), 1001, telegramstore.OperationDownload, callback); err != nil {
		t.Fatalf("download fallback error = %v", err)
	}
	if fallback.calls != 2 || fallback.operations[0] != telegramstore.OperationUpload || fallback.operations[1] != telegramstore.OperationDownload {
		t.Fatalf("fallback calls = %d, operations=%v", fallback.calls, fallback.operations)
	}
}

func TestUploadAwareRunnerLimitsAndRotatesDownloadBots(t *testing.T) {
	db := testpostgres.New(t)
	if _, err := db.Pool.Exec(context.Background(), "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	for _, botID := range []int64{101, 102, 103} {
		if _, err := db.Pool.Exec(context.Background(), `
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
	fallback := &recordingRunner{}
	runner, err := NewUploadAwareRunner(db.Pool, cipher, factory, fallback, 2)
	if err != nil {
		t.Fatal(err)
	}
	var selected []int64
	runner.runBotFunc = func(_ context.Context, _ int64, bot *sqlcgen.Bot, _ int, _ func(context.Context, *tg.Client) error) error {
		selected = append(selected, bot.BotID)
		return nil
	}
	callback := func(context.Context, *tg.Client) error { return nil }
	for range 4 {
		if err := runner.Run(context.Background(), 1001, telegramstore.OperationDownload, callback); err != nil {
			t.Fatal(err)
		}
	}
	if want := []int64{101, 102, 101, 102}; !slices.Equal(selected, want) {
		t.Fatalf("selected download bots = %v, want %v", selected, want)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallback.calls)
	}

	runner.downloadBots = 0
	if err := runner.Run(context.Background(), 1001, telegramstore.OperationDownload, callback); err != nil {
		t.Fatal(err)
	}
	if fallback.calls != 1 || fallback.operations[0] != telegramstore.OperationDownload {
		t.Fatalf("disabled download bot fallback = %d, %v", fallback.calls, fallback.operations)
	}
}

func TestUploadAwareRunnerSkipsBusyBots(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	for _, botID := range []int64{101, 102, 103} {
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
	runner, err := NewUploadAwareRunner(db.Pool, cipher, factory, &recordingRunner{}, 3)
	if err != nil {
		t.Fatal(err)
	}
	selected := make(chan int64, 1)
	runner.runBotFunc = func(ctx context.Context, _ int64, bot *sqlcgen.Bot, _ int, fn func(context.Context, *tg.Client) error) error {
		selected <- bot.BotID
		return fn(ctx, nil)
	}
	quick := func(context.Context, *tg.Client) error { return nil }
	pick := func(operation telegramstore.Operation) int64 {
		t.Helper()
		if err := runner.Run(ctx, 1001, operation, quick); err != nil {
			t.Fatal(err)
		}
		return <-selected
	}

	// Bot 101 serves a long stream with 4 connections.
	release := make(chan struct{})
	streamDone := make(chan error, 1)
	go func() {
		streamDone <- runner.RunPooled(ctx, 1001, telegramstore.OperationDownload, 4, func(context.Context, *tg.Client) error {
			<-release
			return nil
		})
	}()
	if busy := <-selected; busy != 101 {
		t.Fatalf("stream bot = %d, want 101", busy)
	}

	// Round-robin would return to 101 on the third download and first upload;
	// it must be skipped while idle bots take the work.
	var got []int64
	for _, operation := range []telegramstore.Operation{
		telegramstore.OperationDownload, telegramstore.OperationDownload, telegramstore.OperationDownload,
		telegramstore.OperationUpload, telegramstore.OperationUpload, telegramstore.OperationUpload,
	} {
		got = append(got, pick(operation))
	}
	if want := []int64{102, 103, 102, 102, 102, 103}; !slices.Equal(got, want) {
		t.Fatalf("bots while 101 is busy = %v, want %v", got, want)
	}

	close(release)
	if err := <-streamDone; err != nil {
		t.Fatal(err)
	}
	// Once idle again, bot 101 is back in rotation (upload rotation is at offset 3).
	if got := pick(telegramstore.OperationUpload); got != 101 {
		t.Fatalf("bot after stream finished = %d, want 101", got)
	}
}

// An upload that overlaps warm-pool initialization must not pin pooled
// downloads to the other bot: account selection happens per download session,
// and idle warm clients do not count as load.
func TestDownloadClientPoolUsesAllBotsAfterOverlappingUpload(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	for _, botID := range []int64{101, 102} {
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
	runner, err := NewUploadAwareRunner(db.Pool, cipher, factory, &recordingRunner{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var warmClients []int64
	uploadStarted := make(chan int64, 1)
	runner.runBotFunc = func(ctx context.Context, _ int64, bot *sqlcgen.Bot, connections int, fn func(context.Context, *tg.Client) error) error {
		if connections == 8 {
			uploadStarted <- bot.BotID
		} else {
			mu.Lock()
			warmClients = append(warmClients, bot.BotID)
			mu.Unlock()
		}
		return fn(telegramstore.WithClientID(ctx, bot.BotID), new(tg.Client))
	}
	pool, err := telegramstore.NewDownloadClientPool(runner, telegramstore.DownloadClientPoolConfig{ReadParallel: 4}, nil)
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
	load := func() map[int64]int {
		runner.load.mu.Lock()
		defer runner.load.mu.Unlock()
		return maps.Clone(runner.load.connections)
	}
	open := func(n int) []telegramstore.DownloadSession {
		t.Helper()
		sessions := make([]telegramstore.DownloadSession, n)
		for i := range sessions {
			if sessions[i], err = pool.OpenDownloadSession(ctx, 1001); err != nil {
				t.Fatal(err)
			}
		}
		return sessions
	}
	closeAll := func(sessions []telegramstore.DownloadSession) {
		for _, session := range sessions {
			_ = session.Close()
		}
	}

	// Bot 101 uploads with eight connections while the warm pool fills.
	finishUpload := make(chan struct{})
	uploadDone := make(chan error, 1)
	go func() {
		uploadDone <- runner.RunPooled(ctx, 1001, telegramstore.OperationUpload, 8, func(context.Context, *tg.Client) error {
			<-finishUpload
			return nil
		})
	}()
	if bot := <-uploadStarted; bot != 101 {
		t.Fatalf("upload bot = %d, want 101", bot)
	}
	during := open(2)
	if got, want := load(), map[int64]int{101: 8, 102: 8}; !maps.Equal(got, want) {
		t.Fatalf("load during upload = %v, want %v", got, want)
	}
	closeAll(during)
	if got, want := load(), map[int64]int{101: 8}; !maps.Equal(got, want) {
		t.Fatalf("load with idle warm client = %v, want %v", got, want)
	}
	close(finishUpload)
	if err := <-uploadDone; err != nil {
		t.Fatal(err)
	}

	// With the upload gone, concurrent downloads spread over both bots.
	after := open(4)
	if got, want := load(), map[int64]int{101: 8, 102: 8}; !maps.Equal(got, want) {
		t.Fatalf("load after upload = %v, want %v", got, want)
	}
	closeAll(after)
	if got := load(); len(got) != 0 {
		t.Fatalf("load with only idle warm clients = %v, want none", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []int64{102, 101}; !slices.Equal(warmClients, want) {
		t.Fatalf("warm clients started for bots %v, want %v", warmClients, want)
	}
}

func TestUploadAwareRunnerValidation(t *testing.T) {
	t.Parallel()
	if _, err := NewUploadAwareRunner(nil, nil, nil, nil, 0); !errors.Is(err, ErrUploadRunnerConfiguration) {
		t.Fatalf("NewUploadAwareRunner() error = %v", err)
	}
	var runner *UploadAwareRunner
	if err := runner.Run(context.Background(), 1, telegramstore.OperationUpload, func(context.Context, *tg.Client) error { return nil }); !errors.Is(err, ErrUploadRunnerConfiguration) {
		t.Fatalf("Run() error = %v", err)
	}
}

type recordingRunner struct {
	calls      int
	operations []telegramstore.Operation
}

func (r *recordingRunner) Run(_ context.Context, _ int64, operation telegramstore.Operation, _ func(context.Context, *tg.Client) error) error {
	r.calls++
	r.operations = append(r.operations, operation)
	return nil
}
