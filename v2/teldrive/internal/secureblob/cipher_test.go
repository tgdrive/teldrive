package secureblob

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func TestCipherRoundTripAndPurposeBinding(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{7}, 32)
	nonce := bytes.Repeat([]byte{3}, 24)
	cipher, err := NewWithKey(key, bytes.NewReader(nonce))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.Seal("telegram-session", []byte("secret-session"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("secret-session")) {
		t.Fatal("ciphertext contains plaintext")
	}
	plain, err := cipher.Open("telegram-session", sealed)
	if err != nil || string(plain) != "secret-session" {
		t.Fatalf("Open() = %q, %v", plain, err)
	}
	if _, err := cipher.Open("bot-token", sealed); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("cross-purpose Open() error = %v", err)
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := cipher.Open("telegram-session", sealed); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("tampered Open() error = %v", err)
	}
}

func TestCipherValidation(t *testing.T) {
	t.Parallel()
	if _, err := New("not-base64"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := NewWithKey([]byte("short"), bytes.NewReader(nil)); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("NewWithKey() error = %v", err)
	}
	cipher, _ := NewWithKey(bytes.Repeat([]byte{1}, 32), bytes.NewReader(nil))
	if _, err := cipher.Seal("purpose", []byte("value")); err == nil {
		t.Fatal("expected random source failure")
	}
}

func TestCipherAdditionalValidationBranches(t *testing.T) {
	t.Parallel()
	encoded := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32))
	cipher, err := New(encoded)
	if err != nil {
		t.Fatalf("New(valid) error = %v", err)
	}
	if _, err := cipher.Open("purpose", []byte{1, 2, 3}); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("short ciphertext error = %v", err)
	}
	var nilCipher *Cipher
	if _, err := nilCipher.Seal("purpose", []byte("value")); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("nil Seal() error = %v", err)
	}
	if _, err := nilCipher.Open("purpose", []byte("value")); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("nil Open() error = %v", err)
	}
	if _, err := NewWithKey(bytes.Repeat([]byte{1}, 32), nil); err == nil {
		t.Fatal("expected nil random source error")
	}
}
