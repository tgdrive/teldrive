package jobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseUploadSize(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]int64{
		"0": 0, "1024": 1024, "10MiB": 10 * 1024 * 1024, "1.5GiB": 3 * 1024 * 1024 * 1024 / 2,
	} {
		got, err := parseUploadSize(input)
		if err != nil {
			t.Fatalf("parseUploadSize(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("parseUploadSize(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestUploadSourceWorkerDetectsMIME(t *testing.T) {
	t.Run("sniffs HTTP content", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("plain text content"))
		}))
		defer server.Close()
		worker := NewUploadSourceWorker(nil, nil, nil, nil, server.Client(), 0)
		got, err := worker.detectSourceMIME(context.Background(), UploadFileSource{Type: "http", Location: server.URL, DestinationPath: "file.bin", MIMEType: "application/octet-stream"})
		if err != nil || got != "text/plain" {
			t.Fatalf("detectSourceMIME() = %q, %v", got, err)
		}
	})

	t.Run("falls back to destination extension", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "source")
		if err := os.WriteFile(file, make([]byte, 512), 0o600); err != nil {
			t.Fatal(err)
		}
		worker := NewUploadSourceWorker(nil, nil, nil, nil, nil, 0)
		got, err := worker.detectSourceMIME(context.Background(), UploadFileSource{Type: "local", Location: file, DestinationPath: "video.mp4"})
		if err != nil || got != "video/mp4" {
			t.Fatalf("detectSourceMIME() = %q, %v", got, err)
		}
	})
}

func TestNormalizeUploadChunkSizeMatchesRclone(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		input int64
		want  int64
	}{
		{name: "default", want: 512 * 1024 * 1024},
		{name: "aligned", input: 64 * 1024 * 1024, want: 64 * 1024 * 1024},
		{name: "round down", input: 71 * 1024 * 1024, want: 64 * 1024 * 1024},
		{name: "round up", input: 73 * 1024 * 1024, want: 80 * 1024 * 1024},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeUploadChunkSize(test.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("normalizeUploadChunkSize(%d) = %d, want %d", test.input, got, test.want)
			}
		})
	}
	for _, input := range []int64{63 * 1024 * 1024, 2001 * 1024 * 1024} {
		if _, err := normalizeUploadChunkSize(input); err == nil {
			t.Fatalf("normalizeUploadChunkSize(%d) succeeded", input)
		}
	}
}

func TestExpandLocalSourceRecursesAndFilters(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"keep.bin": "1234567890", "small.bin": "x", "nested/skip.tmp": "1234567890"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	filter, err := newUploadFilter([]string{"*.tmp"}, "10", "")
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandLocalSource(UploadSource{Type: "local", Path: root, DestinationPath: "backup"}, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].DestinationPath != "backup/keep.bin" || files[0].Size != 10 {
		t.Fatalf("expanded files = %#v", files)
	}
}

func TestInspectHTTPSourceMergesHeadersAndMetadata(t *testing.T) {
	modified := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer source" || request.Header.Get("Referer") != "https://batch.example/" {
			t.Errorf("headers = %#v", request.Header)
		}
		response.Header().Set("Content-Length", "42")
		response.Header().Set("Last-Modified", modified.Format(http.TimeFormat))
		response.Header().Set("Content-Disposition", `inline; filename="remote.bin"`)
	}))
	defer server.Close()
	file, err := inspectHTTPSource(context.Background(), server.Client(), UploadSource{
		Type: "http", URL: server.URL, Headers: map[string]string{"Authorization": "Bearer source"},
	}, map[string]string{"Authorization": "Bearer batch", "Referer": "https://batch.example/"})
	if err != nil {
		t.Fatal(err)
	}
	if file.DestinationPath != "remote.bin" || file.Size != 42 || !file.HasModTime || !file.ModTime.Equal(modified) {
		t.Fatalf("HTTP source = %#v", file)
	}
}

func TestUploadFilterUsesRcloneStyleExclusionsAndInclusiveSizes(t *testing.T) {
	t.Parallel()
	filter, err := newUploadFilter([]string{"*.tmp", ".git/**", "**/node_modules/**"}, "10", "20")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		size int64
		want string
	}{
		{path: "keep.bin", size: 10},
		{path: "keep.bin", size: 20},
		{path: "scratch.tmp", size: 15, want: skipExcluded},
		{path: ".git/objects/a", size: 15, want: skipExcluded},
		{path: "app/node_modules/pkg/index.js", size: 15, want: skipExcluded},
		{path: "node_modules/pkg/index.js", size: 15, want: skipExcluded},
		{path: "small.bin", size: 9, want: skipBelowMinSize},
		{path: "large.bin", size: 21, want: skipAboveMaxSize},
	} {
		if got := filter.skipReason(test.path, test.size); got != test.want {
			t.Errorf("skipReason(%q, %d) = %q, want %q", test.path, test.size, got, test.want)
		}
	}
}
