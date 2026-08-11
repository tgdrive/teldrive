package config

import (
	"testing"
	"time"
)

func TestEventDefaults(t *testing.T) {
	t.Parallel()
	cfg := Default()
	if cfg.Events.BatchSize != 100 || cfg.Events.MaxConnectionsPerUser != 5 || cfg.Events.Heartbeat != 20*time.Second || cfg.Events.WriteTimeout != 10*time.Second {
		t.Fatalf("event defaults = %#v", cfg.Events)
	}
	if cfg.Events.TicketTTL != 2*time.Minute || cfg.Events.Retention != 7*24*time.Hour {
		t.Fatalf("event lifetime defaults = %#v", cfg.Events)
	}
	if cfg.Events.ReconnectMin != 100*time.Millisecond || cfg.Events.ReconnectMax != 30*time.Second {
		t.Fatalf("event reconnect defaults = %#v", cfg.Events)
	}
}

func TestLoadFromParsesEventEnvironment(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"TELDRIVE_DATABASE_URL":                    "postgres://example/teldrive",
		"TELDRIVE_TELEGRAM_APP_ID":                 "12345",
		"TELDRIVE_TELEGRAM_APP_HASH":               "telegram-app-hash",
		"TELDRIVE_SECURITY_SIGNING_KEY":            "0123456789abcdef0123456789abcdef",
		"TELDRIVE_SECURITY_DATA_KEY":               "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"TELDRIVE_EVENTS_BATCH_SIZE":               "250",
		"TELDRIVE_EVENTS_MAX_CONNECTIONS_PER_USER": "7",
		"TELDRIVE_EVENTS_HEARTBEAT":                "15s",
		"TELDRIVE_EVENTS_WRITE_TIMEOUT":            "4s",
		"TELDRIVE_EVENTS_TICKET_TTL":               "90s",
		"TELDRIVE_EVENTS_RETENTION":                "48h",
		"TELDRIVE_EVENTS_CLEANUP_INTERVAL":         "30m",
		"TELDRIVE_EVENTS_CONNECT_TIMEOUT":          "3s",
		"TELDRIVE_EVENTS_PING_INTERVAL":            "2s",
		"TELDRIVE_EVENTS_RECONNECT_MIN":            "50ms",
		"TELDRIVE_EVENTS_RECONNECT_MAX":            "5s",
	}
	cfg, err := LoadFrom(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.Events.BatchSize != 250 || cfg.Events.MaxConnectionsPerUser != 7 || cfg.Events.Heartbeat != 15*time.Second || cfg.Events.WriteTimeout != 4*time.Second || cfg.Events.TicketTTL != 90*time.Second {
		t.Fatalf("loaded event config = %#v", cfg.Events)
	}
	if cfg.Events.Retention != 48*time.Hour || cfg.Events.CleanupInterval != 30*time.Minute {
		t.Fatalf("loaded event retention = %#v", cfg.Events)
	}
	if cfg.Events.ConnectTimeout != 3*time.Second || cfg.Events.PingInterval != 2*time.Second || cfg.Events.ReconnectMin != 50*time.Millisecond || cfg.Events.ReconnectMax != 5*time.Second {
		t.Fatalf("loaded listener config = %#v", cfg.Events)
	}
}

func TestValidateEvents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "zero batch", mutate: func(cfg *Config) { cfg.Events.BatchSize = 0 }},
		{name: "large batch", mutate: func(cfg *Config) { cfg.Events.BatchSize = 1001 }},
		{name: "zero connections", mutate: func(cfg *Config) { cfg.Events.MaxConnectionsPerUser = 0 }},
		{name: "zero heartbeat", mutate: func(cfg *Config) { cfg.Events.Heartbeat = 0 }},
		{name: "zero write timeout", mutate: func(cfg *Config) { cfg.Events.WriteTimeout = 0 }},
		{name: "inverted reconnect", mutate: func(cfg *Config) { cfg.Events.ReconnectMin = time.Second; cfg.Events.ReconnectMax = time.Millisecond }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfg := validTestConfig()
			test.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid event configuration was accepted")
			}
		})
	}
}
