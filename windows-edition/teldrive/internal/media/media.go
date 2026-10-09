// Package media streams authenticated multimedia through FFmpeg without a shell.
package media

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/tgdrive/teldrive/internal/config"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Source struct{ URL, Headers string }
type Resolve func(*http.Request, string) (Source, error)
type Service struct {
	executable string
	slots      chan struct{}
	timeout    time.Duration
	resolve    Resolve
	available  bool
}

func New(cfg config.MediaConfig, resolve Resolve) *Service {
	count := max(1, min(8, cfg.MaxConcurrent))
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Hour
	}
	executable, err := exec.LookPath(cfg.FFmpegPath)
	s := &Service{executable: executable, slots: make(chan struct{}, count), timeout: timeout, resolve: resolve}
	if err != nil {
		return s
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	codecs, err := exec.CommandContext(ctx, executable, "-hide_banner", "-encoders").Output()
	s.available = err == nil && strings.Contains(string(codecs), "libx264") && strings.Contains(string(codecs), "libmp3lame")
	return s
}

func (s *Service) Capabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(map[string]any{"compatiblePlayback": s.available, "maxConcurrent": cap(s.slots)}); err != nil {
		return
	}
}

func Arguments(source Source, mode string, start float64) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-protocol_whitelist", "http,https,tcp,tls,crypto", "-format_whitelist", "mov,matroska,avi,flv,mpegts,mpeg,mpegvideo,asf,mp3,aac,wav,aiff,flac,ogg,amr,ac3,dts,ape", "-rw_timeout", "30000000"}
	if source.Headers != "" {
		args = append(args, "-headers", source.Headers)
	}
	args = append(args, "-i", source.URL)
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
	}
	if mode == "audio" {
		return append(args, "-map", "0:a:0", "-vn", "-c:a", "libmp3lame", "-b:a", "192k", "-f", "mp3", "pipe:1")
	}
	return append(args, "-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn", "-c:v", "libx264", "-threads", "2", "-preset", "veryfast", "-crf", "23", "-vf", "scale=w='trunc(min(1920,iw)/2)*2':h=-2", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-ac", "2", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", "pipe:1")
}

func (s *Service) Handler(w http.ResponseWriter, r *http.Request) {
	if !s.available {
		http.Error(w, "Instala FFmpeg con libx264 y libmp3lame en el servidor", 503)
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode != "audio" && mode != "video" {
		http.Error(w, "Tipo de reproducción inválido", 400)
		return
	}
	start := 0.0
	if value := r.URL.Query().Get("start"); value != "" {
		var err error
		start, err = strconv.ParseFloat(value, 64)
		if err != nil || !(start >= 0 && start <= 86400) {
			http.Error(w, "Tiempo de inicio inválido", 400)
			return
		}
	}
	source, err := s.resolve(r, chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "No tienes acceso a este archivo", 403)
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		http.Error(w, "El servidor está convirtiendo otros archivos. Inténtalo en unos instantes", 429)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	command := exec.CommandContext(ctx, s.executable, Arguments(source, mode, start)...)
	// Diagnostics may include authenticated URLs. Never log or return them.
	command.Stderr = io.Discard
	output, err := command.StdoutPipe()
	if err != nil {
		http.Error(w, "No se pudo iniciar la conversión", 500)
		return
	}
	if err := command.Start(); err != nil {
		http.Error(w, "No se pudo iniciar FFmpeg", 503)
		return
	}
	prefix := make([]byte, 1024)
	n, readErr := output.Read(prefix)
	if n == 0 && readErr != nil {
		cancel()
		_ = command.Wait()
		http.Error(w, "No se pudo convertir el archivo. Descárgalo para abrirlo en VLC", 422)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Accel-Buffering", "no")
	if mode == "audio" {
		w.Header().Set("Content-Type", "audio/mpeg")
	} else {
		w.Header().Set("Content-Type", "video/mp4")
	}
	_, writeErr := w.Write(prefix[:n])
	if writeErr == nil {
		_, writeErr = io.Copy(w, output)
	}
	if writeErr != nil {
		cancel()
	}
	_ = command.Wait()
}

func LocalURL(port int, path string) string { return fmt.Sprintf("http://127.0.0.1:%d%s", port, path) }
