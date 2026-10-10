package media

import (
	"context"
	"errors"
	"github.com/tgdrive/teldrive/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompatiblePlaybackFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg not installed")
	}
	directory := t.TempDir()
	fixtures := []struct {
		name, mode string
		args       []string
	}{
		{"audio.flac", "audio", []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "flac"}},
		{"video.mkv", "video", []string{"-f", "lavfi", "-i", "color=c=blue:s=160x90:d=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:v", "mpeg4", "-c:a", "pcm_s16le", "-shortest"}},
	}
	for _, fixture := range fixtures {
		args := append([]string{"-y", "-hide_banner", "-loglevel", "error"}, fixture.args...)
		args = append(args, filepath.Join(directory, fixture.name))
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		output, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("fixture: %v %s", err, output)
		}
	}
	input := httptest.NewServer(http.FileServer(http.Dir(directory)))
	defer input.Close()
	for _, fixture := range fixtures {
		t.Run(fixture.mode, func(t *testing.T) {
			service := New(config.MediaConfig{FFmpegPath: ffmpeg, MaxConcurrent: 1, Timeout: 20 * time.Second}, func(_ *http.Request, _ string) (Source, error) {
				return Source{URL: input.URL + "/" + fixture.name}, nil
			})
			if !service.available {
				t.Fatal("required encoders unavailable")
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/compatible?mode="+fixture.mode, nil)
			service.Handler(recorder, request)
			if recorder.Code != 200 {
				t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
			}
			if recorder.Body.Len() < 1000 {
				t.Fatal("converted output is empty")
			}
			extension := ".mp3"
			if fixture.mode == "video" {
				extension = ".mp4"
				if !strings.Contains(recorder.Body.String(), "ftyp") {
					t.Fatal("missing MP4 container")
				}
			}
			path := filepath.Join(directory, "converted"+extension)
			if err := os.WriteFile(path, recorder.Body.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if output, err := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput(); err != nil {
				t.Fatalf("output not decodable: %v %s", err, output)
			}
		})
	}
}

func TestMediaValidation(t *testing.T) {
	service := &Service{available: true, slots: make(chan struct{}, 1), resolve: func(_ *http.Request, _ string) (Source, error) { return Source{}, errors.New("denied") }}
	for _, query := range []string{"mode=invalid", "mode=video&start=NaN", "mode=audio&start=-1", "mode=video&start=Inf"} {
		recorder := httptest.NewRecorder()
		service.Handler(recorder, httptest.NewRequest(http.MethodGet, "/compatible?"+query, nil))
		if recorder.Code != 400 {
			t.Fatal("invalid request accepted", query)
		}
	}
	recorder := httptest.NewRecorder()
	service.Handler(recorder, httptest.NewRequest(http.MethodGet, "/compatible?mode=video", nil))
	if recorder.Code != 403 {
		t.Fatal("unauthorized media must not be converted")
	}
}
