package telethonsession

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/gotd/td/session"
)

func TestEncodeRoundTrip(t *testing.T) {
	authKey := bytes.Repeat([]byte{0x5a}, 256)
	original := &session.Data{DC: 2, Addr: "ignored", AuthKey: authKey}

	encoded, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := session.TelethonSession(encoded)
	if err != nil {
		t.Fatalf("TelethonSession() error = %v", err)
	}
	wantAddr := net.JoinHostPort(productionDCIPv4[2], "443")
	if decoded.DC != original.DC || decoded.Addr != wantAddr || !bytes.Equal(decoded.AuthKey, original.AuthKey) {
		t.Fatalf("decoded session = %#v", decoded)
	}
}

func TestEncodeUsesFixedProductionDCAddress(t *testing.T) {
	for dc, host := range productionDCIPv4 {
		t.Run(host, func(t *testing.T) {
			authKey := bytes.Repeat([]byte{0x7a}, 256)
			encoded, err := Encode(&session.Data{DC: dc, Addr: "malformed address without port", AuthKey: authKey})
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			decoded, err := session.TelethonSession(encoded)
			if err != nil {
				t.Fatalf("TelethonSession() error = %v", err)
			}
			wantAddr := net.JoinHostPort(host, "443")
			if decoded.Addr != wantAddr || decoded.DC != dc || !bytes.Equal(decoded.AuthKey, authKey) {
				t.Fatalf("decoded session = %#v, want addr %q", decoded, wantAddr)
			}
		})
	}
}

func TestGotdRoundTrip(t *testing.T) {
	ctx := context.Background()
	memory := new(session.StorageMemory)
	original := &session.Data{DC: 4, Addr: "149.154.167.91:443", AuthKey: bytes.Repeat([]byte{0x33}, 256)}
	if err := (&session.Loader{Storage: memory}).Save(ctx, original); err != nil {
		t.Fatal(err)
	}
	raw, err := memory.Bytes(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeGotd(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := DecodeToGotd(ctx, encoded)
	if err != nil {
		t.Fatal(err)
	}
	loadedMemory := new(session.StorageMemory)
	if err := loadedMemory.StoreSession(ctx, roundTrip); err != nil {
		t.Fatal(err)
	}
	loaded, err := (&session.Loader{Storage: loadedMemory}).Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DC != original.DC || loaded.Addr != original.Addr || !bytes.Equal(loaded.AuthKey, original.AuthKey) {
		t.Fatalf("loaded session = %#v", loaded)
	}
}

func TestEncodeRejectsInvalidData(t *testing.T) {
	for _, data := range []*session.Data{
		nil,
		{DC: 0, AuthKey: make([]byte, 256)},
		{DC: 6, AuthKey: make([]byte, 256)},
		{DC: 2, AuthKey: make([]byte, 255)},
	} {
		if _, err := Encode(data); err == nil {
			t.Fatalf("Encode(%#v) succeeded", data)
		}
	}
}

func TestDecodeRejectsGotdJSON(t *testing.T) {
	if _, err := DecodeToGotd(context.Background(), `{"Version":1}`); err == nil {
		t.Fatal("DecodeToGotd accepted gotd JSON")
	}
}
