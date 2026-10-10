package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopInfoValidatesWithoutConnectingOrExposingSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	signing := strings.Repeat("private-signing-key-", 3)
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{5}, 32))
	data := "[http]\naddress = \"127.0.0.1:9123\"\n[database]\nurl = \"postgres://user:private-password@127.0.0.1:1/unreachable\"\n[security]\nsigning-key = \"" + signing + "\"\ndata-key = \"" + key + "\"\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"check", "--config", path, "--desktop-info"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var info map[string]string
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if len(info) != 2 || info["address"] != "127.0.0.1:9123" || info["databaseAddress"] != "127.0.0.1:1" {
		t.Fatalf("unexpected desktop info: %v", info)
	}
	for _, secret := range []string{signing, key, "private-password"} {
		if strings.Contains(out.String(), secret) {
			t.Fatal("desktop info exposed a credential")
		}
	}
}
