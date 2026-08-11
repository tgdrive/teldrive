package telethonsession

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/gotd/td/session"
)

const (
	version      byte   = '1'
	telegramPort uint16 = 443
)

var productionDCIPv4 = map[int]string{
	1: "149.154.175.53",
	2: "149.154.167.51",
	3: "149.154.175.100",
	4: "149.154.167.91",
	5: "91.108.56.130",
}

// Encode serializes gotd session data as a Telethon v1 StringSession.
func Encode(data *session.Data) (string, error) {
	if data == nil || len(data.AuthKey) != 256 {
		return "", errors.New("invalid Telegram session data")
	}
	host, ok := productionDCIPv4[data.DC]
	if !ok {
		return "", fmt.Errorf("unsupported Telegram production DC: %d", data.DC)
	}
	packedIP := net.ParseIP(host).To4()
	if packedIP == nil {
		return "", fmt.Errorf("invalid Telegram production DC address: %q", host)
	}
	packed := make([]byte, 1+len(packedIP)+2+len(data.AuthKey))
	packed[0] = byte(data.DC)
	copy(packed[1:], packedIP)
	binary.BigEndian.PutUint16(packed[1+len(packedIP):], telegramPort)
	copy(packed[3+len(packedIP):], data.AuthKey)
	return string(version) + base64.URLEncoding.EncodeToString(packed), nil
}

// EncodeGotd converts gotd's raw session blob to a Telethon StringSession.
func EncodeGotd(ctx context.Context, raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("empty gotd session")
	}
	memory := new(session.StorageMemory)
	if err := memory.StoreSession(ctx, raw); err != nil {
		return "", err
	}
	data, err := (&session.Loader{Storage: memory}).Load(ctx)
	if err != nil {
		return "", fmt.Errorf("decode gotd session: %w", err)
	}
	return Encode(data)
}

// DecodeToGotd converts a Telethon StringSession to gotd's raw session blob.
func DecodeToGotd(ctx context.Context, encoded string) ([]byte, error) {
	data, err := session.TelethonSession(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode Telethon session: %w", err)
	}
	memory := new(session.StorageMemory)
	if err := (&session.Loader{Storage: memory}).Save(ctx, data); err != nil {
		return nil, fmt.Errorf("encode gotd session: %w", err)
	}
	return memory.Bytes(nil)
}
