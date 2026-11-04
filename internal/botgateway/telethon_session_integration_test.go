//go:build integration

package botgateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"github.com/gotd/td/session"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestBotSessionStoragePreservesEachBotsDCAndKey(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{3}, 32), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	queries := sqlcgen.New(db.Pool)
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	for _, botID := range []int64{101, 104} {
		if _, err := db.Pool.Exec(ctx, `
INSERT INTO bots (bot_id, user_id, token_ciphertext, enabled)
VALUES ($1, 1001, $2, true)`, botID, []byte("test-token")); err != nil {
			t.Fatal(err)
		}
	}
	makeSession := func(dc int, key byte) []byte {
		t.Helper()
		memory := new(session.StorageMemory)
		if err := (&session.Loader{Storage: memory}).Save(ctx, &session.Data{
			DC: dc, AuthKey: bytes.Repeat([]byte{key}, 256),
		}); err != nil {
			t.Fatal(err)
		}
		raw, err := memory.Bytes(nil)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	assertStored := func(botID int64, dc int, key byte) {
		t.Helper()
		row, err := queries.GetBot(ctx, sqlcgen.GetBotParams{UserID: 1001, BotID: botID})
		if err != nil {
			t.Fatal(err)
		}
		// Reconstruct storage like a fresh request/process, not the instance
		// that just wrote the session.
		storage := &botSessionStorage{queries: queries, cipher: cipher, userID: 1001, botID: botID, stored: row.Session}
		raw, err := storage.LoadSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		memory := new(session.StorageMemory)
		if err := memory.StoreSession(ctx, raw); err != nil {
			t.Fatal(err)
		}
		data, err := (&session.Loader{Storage: memory}).Load(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if data.DC != dc || !bytes.Equal(data.AuthKey, bytes.Repeat([]byte{key}, 256)) {
			t.Fatalf("bot %d session DC/key mismatch (DC=%d, want %d)", botID, data.DC, dc)
		}
		plain, err := cipher.Open("bot-session", row.Session)
		if err != nil {
			t.Fatal(err)
		}
		if len(plain) == 0 || plain[0] != telethonSessionVersion {
			t.Fatal("persisted bot session is not a Telethon StringSession")
		}
	}
	for _, tc := range []struct {
		botID int64
		dc    int
		key   byte
	}{
		{101, 1, 0x11},
		{104, 4, 0x44},
	} {
		storage := &botSessionStorage{queries: queries, cipher: cipher, userID: 1001, botID: tc.botID}
		if err := storage.StoreSession(ctx, makeSession(tc.dc, tc.key)); err != nil {
			t.Fatal(err)
		}
		assertStored(tc.botID, tc.dc, tc.key)
	}
	// A login redirect/session refresh must update just that bot's DC/key.
	storage := &botSessionStorage{queries: queries, cipher: cipher, userID: 1001, botID: 104}
	if err := storage.StoreSession(ctx, makeSession(1, 0x55)); err != nil {
		t.Fatal(err)
	}
	assertStored(104, 1, 0x55)
	assertStored(101, 1, 0x11)
}
