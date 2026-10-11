//go:build integration

package telegramstore

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/session"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/telethonsession"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestDatabaseSessionStoragePersistsEncryptedUpdates(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{1}, 32), bytes.NewReader(bytes.Repeat([]byte{2}, 24*8)))
	if err != nil {
		t.Fatal(err)
	}
	initialRaw := gotdSessionBytes(t, 2, "149.154.167.51:443", 0x11)
	initialTelethon, err := telethonsession.EncodeGotd(ctx, initialRaw)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := cipher.Seal("telegram-session", []byte(initialTelethon))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	sessionID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO sessions (id,user_id,telegram_session,refresh_token_hash,expires_at)
VALUES ($1,1001,$2,$3,$4)`, sessionID, initial, bytes.Repeat([]byte{9}, 32), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	storage := &databaseSessionStorage{
		queries: sqlcgen.New(db.Pool), cipher: cipher, sessionID: dbtypes.UUID(sessionID), userID: 1001,
	}
	loaded, err := storage.LoadSession(ctx)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	assertSessionEquivalent(t, initialRaw, loaded)
	rotatedRaw := gotdSessionBytes(t, 4, "149.154.167.91:443", 0x22)
	if err := storage.StoreSession(ctx, rotatedRaw); err != nil {
		t.Fatalf("StoreSession() error = %v", err)
	}
	loaded, err = storage.LoadSession(ctx)
	if err != nil {
		t.Fatalf("LoadSession(after store) error = %v", err)
	}
	assertSessionEquivalent(t, rotatedRaw, loaded)
	var ciphertext []byte
	if err := db.Pool.QueryRow(ctx, "SELECT telegram_session FROM sessions WHERE id=$1", sessionID).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	plain, err := cipher.Open("telegram-session", ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) == 0 || plain[0] != '1' {
		t.Fatalf("stored session is not Telethon StringSession: %q", plain)
	}
}

func gotdSessionBytes(t *testing.T, dc int, addr string, fill byte) []byte {
	t.Helper()
	memory := new(session.StorageMemory)
	if err := (&session.Loader{Storage: memory}).Save(context.Background(), &session.Data{
		DC: dc, Addr: addr, AuthKey: bytes.Repeat([]byte{fill}, 256),
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := memory.Bytes(nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertSessionEquivalent(t *testing.T, wantRaw, gotRaw []byte) {
	t.Helper()
	load := func(raw []byte) *session.Data {
		memory := new(session.StorageMemory)
		if err := memory.StoreSession(context.Background(), raw); err != nil {
			t.Fatal(err)
		}
		data, err := (&session.Loader{Storage: memory}).Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	want, got := load(wantRaw), load(gotRaw)
	if want.DC != got.DC || want.Addr != got.Addr || !bytes.Equal(want.AuthKey, got.AuthKey) {
		t.Fatalf("session mismatch: want=%#v got=%#v", want, got)
	}
}
