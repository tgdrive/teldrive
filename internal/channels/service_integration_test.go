//go:build integration

package channels_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/channels"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestResolveSelectedAndExplicitChannels(t *testing.T) {
	db := testpostgres.New(t)
	seedChannelOwner(t, db.Pool, 1001)
	insertChannel(t, db.Pool, 1001, 9001, true)
	insertChannel(t, db.Pool, 2002, 9002, true)
	creator := &fakeCreator{nextID: 9100}
	svc := channels.NewService(db.Pool, creator, channels.Config{PartLimit: 2, AutoCreate: true})

	selected, err := svc.Resolve(context.Background(), 1001, 0)
	if err != nil || selected != 9001 {
		t.Fatalf("Resolve selected = %d, %v", selected, err)
	}
	explicit, err := svc.Resolve(context.Background(), 1001, 9001)
	if err != nil || explicit != 9001 {
		t.Fatalf("Resolve explicit = %d, %v", explicit, err)
	}
	if _, err := svc.Resolve(context.Background(), 1001, 9002); !errors.Is(err, channels.ErrInvalidChannel) {
		t.Fatalf("foreign channel error = %v", err)
	}
	if creator.createCalls() != 0 {
		t.Fatalf("unexpected channel creation count %d", creator.createCalls())
	}
}

func TestConcurrentRolloverCreatesOneChannel(t *testing.T) {
	db := testpostgres.New(t)
	seedChannelOwner(t, db.Pool, 1001)
	insertChannel(t, db.Pool, 1001, 9001, true)
	insertStoredPart(t, db.Pool, 1001, 9001, 1)
	creator := &fakeCreator{nextID: 9100}
	svc := channels.NewService(db.Pool, creator, channels.Config{PartLimit: 1, AutoCreate: true, NamePrefix: "rollover"})

	const callers = 12
	results := make(chan int64, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := svc.Resolve(context.Background(), 1001, 0)
			if err != nil {
				errs <- err
				return
			}
			results <- id
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("Resolve() error = %v", err)
	}
	for id := range results {
		if id != 9101 {
			t.Fatalf("resolved channel = %d, want 9101", id)
		}
	}
	if creator.createCalls() != 1 {
		t.Fatalf("created %d channels, want 1", creator.createCalls())
	}

	var selectedCount int
	if err := db.Pool.QueryRow(context.Background(), "SELECT count(*) FROM channels WHERE user_id = $1 AND selected", 1001).Scan(&selectedCount); err != nil {
		t.Fatal(err)
	}
	if selectedCount != 1 {
		t.Fatalf("selected channel count = %d", selectedCount)
	}
}

func TestExplicitFullChannelDoesNotRollover(t *testing.T) {
	db := testpostgres.New(t)
	seedChannelOwner(t, db.Pool, 1001)
	insertChannel(t, db.Pool, 1001, 9001, true)
	insertStoredPart(t, db.Pool, 1001, 9001, 1)
	creator := &fakeCreator{nextID: 9100}
	svc := channels.NewService(db.Pool, creator, channels.Config{PartLimit: 1, AutoCreate: true})

	if _, err := svc.Resolve(context.Background(), 1001, 9001); !errors.Is(err, channels.ErrChannelFull) {
		t.Fatalf("Resolve() error = %v, want ErrChannelFull", err)
	}
	if creator.createCalls() != 0 {
		t.Fatalf("explicit resolution created %d channels", creator.createCalls())
	}
}

func TestRolloverCompensatesDatabaseFailure(t *testing.T) {
	db := testpostgres.New(t)
	seedChannelOwner(t, db.Pool, 1001)
	insertChannel(t, db.Pool, 1001, 9001, true)
	insertChannel(t, db.Pool, 1001, 9999, false)
	insertStoredPart(t, db.Pool, 1001, 9001, 1)
	creator := &fakeCreator{fixedID: 9999}
	svc := channels.NewService(db.Pool, creator, channels.Config{PartLimit: 1, AutoCreate: true})

	if _, err := svc.Resolve(context.Background(), 1001, 0); err == nil {
		t.Fatal("expected database conflict")
	}
	if creator.deleteCalls() != 1 {
		t.Fatalf("compensating deletes = %d, want 1", creator.deleteCalls())
	}
}

func seedChannelOwner(t testing.TB, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "INSERT INTO users (user_id) VALUES ($1) ON CONFLICT DO NOTHING", userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if userID != 2002 {
		if _, err := pool.Exec(context.Background(), "INSERT INTO users (user_id) VALUES (2002) ON CONFLICT DO NOTHING"); err != nil {
			t.Fatalf("seed secondary user: %v", err)
		}
	}
}

func insertChannel(t testing.TB, pool *pgxpool.Pool, userID, channelID int64, selected bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "INSERT INTO channels (channel_id, user_id, name, selected) VALUES ($1, $2, $3, $4)", channelID, userID, "storage", selected); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
}

func insertStoredPart(t testing.TB, pool *pgxpool.Pool, userID, channelID, messageID int64) {
	t.Helper()
	fileID := uuid.New()
	if _, err := pool.Exec(context.Background(), `
INSERT INTO files (id, user_id, name, normalized_name, kind, mime_type, size, encryption, status, mod_time)
VALUES ($1, $2, 'file.bin', 'file.bin', 'file', 'application/octet-stream', 1, false, 'active', now())`, fileID, userID); err != nil {
		t.Fatalf("insert file: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
INSERT INTO file_parts (file_id, part_no, channel_id, message_id, plain_size, stored_size)
VALUES ($1, 1, $2, $3, 1, 1)`, fileID, channelID, messageID); err != nil {
		t.Fatalf("insert stored part: %v", err)
	}
}

type fakeCreator struct {
	mu      sync.Mutex
	nextID  int64
	fixedID int64
	creates int
	deletes int
}

func (f *fakeCreator) Create(_ context.Context, _ int64, name string) (channels.RemoteChannel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	if f.fixedID != 0 {
		return channels.RemoteChannel{ID: f.fixedID, Name: name}, nil
	}
	f.nextID++
	return channels.RemoteChannel{ID: f.nextID, Name: name}, nil
}

func (f *fakeCreator) Delete(context.Context, int64, int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	return nil
}

func (f *fakeCreator) createCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates
}

func (f *fakeCreator) deleteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deletes
}
