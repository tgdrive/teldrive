package services

import (
	"github.com/tgdrive/teldrive/internal/api"
	"golang.org/x/crypto/bcrypt"
	"testing"
	"time"
)

func TestShareSettingsOmissionAndRemoval(t *testing.T) {
	updates, err := shareUpdateValues(&api.FileShareCreate{})
	if err != nil || len(updates) != 0 {
		t.Fatal("omitted fields must preserve saved settings", updates, err)
	}
	updates, err = shareUpdateValues(&api.FileShareCreate{Password: api.NewOptString(""), ExpiresAt: api.NewOptDateTime(time.Time{})})
	if err != nil || len(updates) != 2 || updates["password"] != nil || updates["expires_at"] != nil {
		t.Fatal("removing protection and expiry must write SQL NULL", updates, err)
	}
}

func TestShareSettingsPasswordAndExpiration(t *testing.T) {
	expires := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	updates, err := shareUpdateValues(&api.FileShareCreate{Password: api.NewOptString("test:password"), ExpiresAt: api.NewOptDateTime(expires)})
	if err != nil {
		t.Fatal(err)
	}
	hash, ok := updates["password"].(string)
	if !ok || hash == "test:password" || bcrypt.CompareHashAndPassword([]byte(hash), []byte("test:password")) != nil {
		t.Fatal("password must be stored as bcrypt")
	}
	if updates["expires_at"] != expires {
		t.Fatal("expiration was not preserved")
	}
}
