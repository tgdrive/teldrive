// Package media converts catalog content through a private loopback source.
package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/config"
)

var ErrUnavailable = errors.New("FFmpeg with libx264 and libmp3lame is required")
var ErrBusy = errors.New("all conversion slots are in use")
var ErrFormat = errors.New("media cannot be converted")

type Service struct {
	executable string
	slots      chan struct{}
	timeout    time.Duration
	available  bool
}

func New(cfg config.Media) *Service {
	s := &Service{slots: make(chan struct{}, max(1, min(8, cfg.MaxConcurrent))), timeout: cfg.Timeout}
	if !cfg.Enabled {
		return s
	}
	executable, err := exec.LookPath(cfg.FFmpegPath)
	if err != nil {
		return s
	}
	s.executable = executable
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	encoders, err := exec.CommandContext(ctx, executable, "-hide_banner", "-encoders").Output()
	s.available = err == nil && strings.Contains(string(encoders), "libx264") && strings.Contains(string(encoders), "libmp3lame")
	return s
}

func (s *Service) Available() bool { return s != nil && s.available }

func Arguments(source, mode string, start float64) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-protocol_whitelist", "http,tcp", "-format_whitelist", "mov,matroska,avi,flv,mpegts,mpeg,mpegvideo,asf,mp3,aac,wav,aiff,flac,ogg,amr,ac3,dts,ape", "-rw_timeout", "30000000", "-i", source}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
	}
	if mode == "audio" {
		return append(args, "-map", "0:a:0", "-vn", "-c:a", "libmp3lame", "-b:a", "192k", "-f", "mp3", "pipe:1")
	}
	return append(args, "-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn", "-c:v", "libx264", "-threads", "2", "-preset", "veryfast", "-crf", "23", "-vf", "scale=w='trunc(min(1920,iw)/2)*2':h=-2", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-ac", "2", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", "pipe:1")
}

// Convert exposes only this request's authorized file to FFmpeg; neither an
// incoming Host header nor a user supplied URL can become the input source.
func (s *Service) Convert(ctx context.Context, w http.ResponseWriter, source http.Handler, mode string, start float64) error {
	if !s.Available() {
		return ErrUnavailable
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		listener.Close()
		return err
	}
	path := "/" + hex.EncodeToString(key)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path || (r.Method != "GET" && r.Method != "HEAD") {
			http.NotFound(w, r)
			return
		}
		select {
		case <-ctx.Done():
			http.Error(w, "Conversion ended", http.StatusGone)
			return
		default:
		}
		source.ServeHTTP(w, r)
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	command := exec.CommandContext(ctx, s.executable, Arguments("http://"+listener.Addr().String()+path, mode, start)...)
	command.Stderr = io.Discard
	output, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err = command.Start(); err != nil {
		return err
	}
	prefix := make([]byte, 1024)
	n, readErr := output.Read(prefix)
	if n == 0 && readErr != nil {
		cancel()
		_ = command.Wait()
		return ErrFormat
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Accel-Buffering", "no")
	if mode == "audio" {
		w.Header().Set("Content-Type", "audio/mpeg")
	} else {
		w.Header().Set("Content-Type", "video/mp4")
	}
	w.WriteHeader(http.StatusOK)
	_, writeErr := w.Write(prefix[:n])
	if writeErr == nil {
		_ = http.NewResponseController(w).Flush()
		_, writeErr = io.Copy(w, output)
	}
	if writeErr != nil {
		cancel()
	}
	_ = command.Wait()
	// Once streaming starts, a transport failure must not append a JSON response.
	return nil
}
