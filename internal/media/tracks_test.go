package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/tgdrive/teldrive/v2/internal/config"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMKVTracksSubtitlesAndSelectedAudio(t *testing.T) {
	executable := os.Getenv("TEST_FFMPEG_BINARY")
	if executable == "" {
		t.Skip("set TEST_FFMPEG_BINARY for actual container tests")
	}
	cfg := config.Default().Media
	cfg.FFmpegPath = executable
	cfg.Timeout = time.Minute
	service := New(cfg)
	if !service.ProbeAvailable() {
		t.Fatal("ffprobe missing")
	}
	folder := t.TempDir()
	sub := filepath.Join(folder, "fixture.srt")
	fixture := filepath.Join(folder, "multi.mkv")
	os.WriteFile(sub, []byte("1\n00:00:00,200 --> 00:00:01,700\nSubtítulo de prueba\n"), 0600)
	command := exec.Command(executable, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x90:d=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-f", "lavfi", "-i", "sine=frequency=880:duration=2", "-i", sub, "-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s", "-metadata:s:a:0", "language=spa", "-metadata:s:a:1", "language=eng", "-metadata:s:s:0", "language=spa", "-c:v", "ffv1", "-c:a", "pcm_s16le", "-c:s", "srt", "-y", fixture)
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
		http.ServeContent(w, r, "multi.mkv", time.Time{}, file)
	})
	tracks, err := service.Probe(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 4 || tracks[1].Language != "spa" || tracks[2].Language != "eng" || tracks[3].Kind != "subtitle" {
		t.Fatalf("tracks: %+v", tracks)
	}
	subtitles, err := service.Subtitles(context.Background(), source, tracks[3].Index)
	if err != nil || !bytes.Contains(subtitles, []byte("WEBVTT")) || !bytes.Contains(subtitles, []byte("Subtítulo de prueba")) {
		t.Fatalf("subtitles: %v %s", err, subtitles)
	}
	shifted, err := service.Subtitles(context.Background(), source, tracks[3].Index, 1)
	if err != nil || bytes.Contains(shifted, []byte("00:01.700")) {
		t.Fatalf("subtitle seeking: %v %s", err, shifted)
	}
	selected := tracks[2].Index
	response := httptest.NewRecorder()
	if err = service.Convert(context.Background(), response, source, "video", 0, Selection{Audio: &selected}); err != nil {
		t.Fatal(err)
	}
	decode := exec.Command(executable, "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-map", "0:a:0", "-t", "0.5", "-ar", "8000", "-ac", "1", "-f", "f64le", "pipe:1")
	decode.Stdin = bytes.NewReader(response.Body.Bytes())
	data, err := decode.Output()
	if err != nil {
		t.Fatal(err)
	}
	crossings := 0
	last := 0.0
	for i := 0; i+8 <= len(data); i += 8 {
		value := math.Float64frombits(binary.LittleEndian.Uint64(data[i : i+8]))
		if i > 0 && last < 0 && value >= 0 {
			crossings++
		}
		last = value
	}
	frequency := float64(crossings) / (float64(len(data)/8) / 8000)
	if frequency < 830 || frequency > 930 {
		t.Fatalf("wrong audio track frequency: %.1f", frequency)
	}
	response = httptest.NewRecorder()
	if err = service.Convert(context.Background(), response, source, "video", 0, Selection{Subtitle: &tracks[3]}); err != nil {
		t.Fatalf("burn subtitles: %v", err)
	}
	if response.Body.Len() < 1000 || !strings.Contains(response.Header().Get("Content-Type"), "video/mp4") {
		t.Fatal("missing captioned video")
	}
}
