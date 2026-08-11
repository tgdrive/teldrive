//go:build integration

package bots

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestBotCRUDEncryptsTokenAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{2}, 32), bytes.NewReader(bytes.Repeat([]byte{4}, 24*4)))
	if err != nil {
		t.Fatal(err)
	}
	verifier := &fakeVerifier{identity: Identity{ID: 777, Username: "storage_bot"}}
	service, err := NewService(db.Pool, cipher, verifier)
	if err != nil {
		t.Fatal(err)
	}
	token := "777:super-secret-token"
	created, err := service.Create(ctx, 1001, token)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.BotID != 777 || !created.Username.Valid || created.Username.String != "storage_bot" {
		t.Fatalf("created bot = %#v", created)
	}
	if bytes.Contains(created.TokenCiphertext, []byte(token)) {
		t.Fatal("bot token ciphertext contains plaintext")
	}
	plain, err := cipher.Open("bot-token", created.TokenCiphertext)
	if err != nil || string(plain) != token {
		t.Fatalf("decrypt bot token = %q, %v", plain, err)
	}
	verifier.identity = Identity{ID: 778, Username: "second_bot"}
	if _, err := service.Create(ctx, 1001, "778:second-secret"); err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}
	defaultRows, err := service.List(ctx, ListInput{UserID: 1001})
	if err != nil || len(defaultRows) != 2 {
		t.Fatalf("List(default) = %#v, %v", defaultRows, err)
	}
	rows, err := service.List(ctx, ListInput{UserID: 1001, Limit: 10})
	if err != nil || len(rows) != 2 {
		t.Fatalf("List() = %#v, %v", rows, err)
	}
	page, err := service.List(ctx, ListInput{UserID: 1001, Limit: 1})
	if err != nil || len(page) != 1 {
		t.Fatalf("List(page) = %#v, %v", page, err)
	}
	pageTime, pageID := page[0].CreatedAt.Time, page[0].BotID
	nextPage, err := service.List(ctx, ListInput{UserID: 1001, AfterCreatedAt: &pageTime, AfterBotID: &pageID, Limit: 500})
	if err != nil || len(nextPage) != 1 {
		t.Fatalf("List(next) = %#v, %v", nextPage, err)
	}
	if err := service.Delete(ctx, 1001, 778); err != nil {
		t.Fatalf("Delete(second) error = %v", err)
	}
	if err := service.Delete(ctx, 1001, 777); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := service.Delete(ctx, 1001, 777); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete() error = %v", err)
	}
	if verifier.calls != 2 {
		t.Fatalf("verifier calls = %d", verifier.calls)
	}
}

type fakeVerifier struct {
	identity Identity
	err      error
	calls    int
}

func (f *fakeVerifier) Verify(context.Context, string) (Identity, error) {
	f.calls++
	return f.identity, f.err
}
