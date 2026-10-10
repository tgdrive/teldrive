package config

import "testing"

func TestValidateFilesystemTelegramBackendWithoutCredentials(t *testing.T) {
	t.Parallel()
	cfg := validTestConfig()
	cfg.Telegram.Backend = "filesystem"
	cfg.Telegram.LocalRoot = t.TempDir()
	cfg.Telegram.AppID = 0
	cfg.Telegram.AppHash = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("filesystem backend rejected: %v", err)
	}
}

func TestValidateTelegramBackendSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "unknown", mutate: func(cfg *Config) { cfg.Telegram.Backend = "mock" }},
		{name: "filesystem missing root", mutate: func(cfg *Config) {
			cfg.Telegram.Backend = "filesystem"
			cfg.Telegram.LocalRoot = ""
		}},
		{name: "remote missing credentials", mutate: func(cfg *Config) {
			cfg.Telegram.Backend = "remote"
			cfg.Telegram.AppID = 0
			cfg.Telegram.AppHash = ""
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfg := validTestConfig()
			test.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid Telegram backend configuration was accepted")
			}
		})
	}
}
