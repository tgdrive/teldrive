package media

import (
	"bytes"
	"context"
	"github.com/tgdrive/teldrive/v2/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConversionUsesRestrictedProtocols(t *testing.T) {
	args := strings.Join(Arguments("http://127.0.0.1:5000/private", "audio", 5), " ")
	if !strings.Contains(args, "-protocol_whitelist http,tcp") || !strings.Contains(args, "-ss 5.000") || !strings.Contains(args, "-f mp3 pipe:1") {
		t.Fatal(args)
	}
	if strings.Contains(args, "file,crypto") || strings.Contains(args, " concat") {
		t.Fatal("unsafe protocols")
	}
}

func TestRealFFmpegConvertsAndDecodesAudioAndVideo(t *testing.T) {
	executable := os.Getenv("TEST_FFMPEG_BINARY")
	if executable == "" {
		t.Skip("set TEST_FFMPEG_BINARY to validate actual conversion")
	}
	cfg := config.Default().Media
	cfg.FFmpegPath = executable
	cfg.Timeout = time.Minute
	svc := New(cfg)
	if !svc.Available() {
		t.Fatal("bundled FFmpeg lacks required encoders")
	}
	fixture := filepath.Join(t.TempDir(), "sample.avi")
	command := exec.Command(executable, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=128x72:d=0.5", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.5", "-c:v", "mpeg4", "-c:a", "pcm_s16le", "-shortest", "-y", fixture)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	source := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, err := os.Open(fixture)
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		http.ServeContent(w, r, "sample.avi", time.Now(), file)
	})
	for _, mode := range []string{"audio", "video"} {
		t.Run(mode, func(t *testing.T) {
			response := httptest.NewRecorder()
			if err := svc.Convert(context.Background(), response, source, mode, 0); err != nil {
				t.Fatal(err)
			}
			if response.Body.Len() < 1000 {
				t.Fatalf("empty conversion: %d", response.Body.Len())
			}
			decode := exec.Command(executable, "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-f", "null", "-")
			decode.Stdin = bytes.NewReader(response.Body.Bytes())
			if output, err := decode.CombinedOutput(); err != nil {
				t.Fatalf("converted output cannot be decoded: %v %s", err, output)
			}
		})
	}
}
