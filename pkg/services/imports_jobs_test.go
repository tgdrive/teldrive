package services

import (
	"testing"

	"github.com/tgdrive/teldrive/internal/api"
)

func TestRemoteFSForSourceSupportsDavs(t *testing.T) {
	fs, err := remoteFSForSource("davs://user:pass@example.com/root/path")
	if err != nil {
		t.Fatalf("remoteFSForSource() error = %v", err)
	}
	if fs == nil {
		t.Fatal("expected non-nil fs")
	}
}

func TestRemoteFSForSourceRejectsUnsupportedScheme(t *testing.T) {
	if _, err := remoteFSForSource("https://example.com/root/path"); err == nil {
		t.Fatal("expected unsupported scheme error")
	}
}

func TestNormalizeSyncPartSize(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  int64
	}{
		{name: "default", input: 0, want: defaultSyncChunkSize},
		{name: "min clamp", input: 1, want: minSyncChunkSize},
		{name: "default aligned", input: defaultSyncChunkSize, want: defaultSyncChunkSize},
		{name: "round to nearest block", input: 500 * 1024 * 1024, want: 496 * 1024 * 1024},
		{name: "max clamp", input: 5000 * 1024 * 1024, want: maxSyncChunkSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeSyncPartSize(tt.input); got != tt.want {
				t.Fatalf("normalizeSyncPartSize(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestToQueueOptionsUsesNormalizedSyncChunkSize(t *testing.T) {
	if got := toQueueOptions(api.OptSyncOptions{}).PartSize; got != defaultSyncChunkSize {
		t.Fatalf("default part size = %d, want %d", got, defaultSyncChunkSize)
	}

	options := api.SyncOptions{}
	options.PartSize = api.NewOptInt64(500 * 1024 * 1024)
	wrapped := api.NewOptSyncOptions(options)
	if got := toQueueOptions(wrapped).PartSize; got != 496*1024*1024 {
		t.Fatalf("normalized part size = %d, want %d", got, 496*1024*1024)
	}
}
