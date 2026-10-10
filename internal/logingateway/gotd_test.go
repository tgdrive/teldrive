package logingateway

import (
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/tg"
)

func TestImportMigratedQRTokenUsesReturnedToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wantToken := []byte("accepted-login-token")
	migratedDC := 0
	importCalls := 0
	wantResult := &tg.AuthLoginTokenSuccess{}

	result, err := importMigratedQRToken(
		ctx,
		5,
		wantToken,
		func(_ context.Context, dcID int) error {
			migratedDC = dcID
			return nil
		},
		func(_ context.Context, token []byte) (tg.AuthLoginTokenClass, error) {
			importCalls++
			if string(token) != string(wantToken) {
				t.Fatalf("import token = %q, want %q", token, wantToken)
			}
			return wantResult, nil
		},
	)
	if err != nil {
		t.Fatalf("importMigratedQRToken() error = %v", err)
	}
	if migratedDC != 5 {
		t.Fatalf("migrated DC = %d, want 5", migratedDC)
	}
	if importCalls != 1 {
		t.Fatalf("import calls = %d, want 1", importCalls)
	}
	if result != wantResult {
		t.Fatalf("result = %T, want original success result", result)
	}
}

func TestImportMigratedQRTokenStopsWhenMigrationFails(t *testing.T) {
	t.Parallel()

	migrationErr := errors.New("migration failed")
	importCalled := false
	_, err := importMigratedQRToken(
		context.Background(),
		5,
		[]byte("token"),
		func(context.Context, int) error { return migrationErr },
		func(context.Context, []byte) (tg.AuthLoginTokenClass, error) {
			importCalled = true
			return nil, nil
		},
	)
	if !errors.Is(err, migrationErr) {
		t.Fatalf("error = %v, want migration error", err)
	}
	if importCalled {
		t.Fatal("import called after migration failure")
	}
}
