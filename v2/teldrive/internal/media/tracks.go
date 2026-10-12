package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Track struct {
	Index                        int32
	Kind, Codec, Language, Title string
	Default, Bitmap              bool
}
type Selection struct {
	Audio           *int32
	Subtitle        *Track
	SubtitleOrdinal int
}

func (s *Service) ProbeAvailable() bool { return s != nil && s.probe != "" }

// withSource authorizes only the supplied file and never accepts a caller URL.
func withSource(ctx context.Context, source http.Handler, run func(string) error) error {
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
		source.ServeHTTP(w, r)
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	return run("http://" + listener.Addr().String() + path)
}
func (s *Service) Probe(ctx context.Context, source http.Handler) ([]Track, error) {
	if !s.ProbeAvailable() {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var result []Track
	err := withSource(ctx, source, func(address string) error {
		command := exec.CommandContext(ctx, s.probe, "-v", "error", "-protocol_whitelist", "http,tcp", "-format_whitelist", "mov,matroska,avi,flv,mpegts,mpeg,mpegvideo,asf,mp3,aac,wav,aiff,flac,ogg,amr,ac3,dts,ape", "-analyzeduration", "5000000", "-probesize", "10000000", "-show_streams", "-of", "json", address)
		reader, err := command.StdoutPipe()
		if err != nil {
			return err
		}
		command.Stderr = io.Discard
		if err = command.Start(); err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(reader, 2*1024*1024+1))
		if err != nil || len(data) > 2*1024*1024 {
			cancel()
			_ = command.Wait()
			return ErrFormat
		}
		if command.Wait() != nil {
			return ErrFormat
		}
		var metadata struct {
			Streams []struct {
				Index       int32                            `json:"index"`
				Kind        string                           `json:"codec_type"`
				Codec       string                           `json:"codec_name"`
				Tags        struct{ Language, Title string } `json:"tags"`
				Disposition struct{ Default int }            `json:"disposition"`
			} `json:"streams"`
		}
		if json.Unmarshal(data, &metadata) != nil {
			return ErrFormat
		}
		for _, stream := range metadata.Streams {
			if len(result) >= 128 {
				break
			}
			if stream.Kind != "video" && stream.Kind != "audio" && stream.Kind != "subtitle" {
				continue
			}
			bitmap := stream.Codec == "hdmv_pgs_subtitle" || stream.Codec == "dvd_subtitle" || stream.Codec == "dvb_subtitle" || stream.Codec == "xsub"
			result = append(result, Track{Index: stream.Index, Kind: stream.Kind, Codec: stream.Codec, Language: stream.Tags.Language, Title: stream.Tags.Title, Default: stream.Disposition.Default != 0, Bitmap: bitmap})
		}
		return nil
	})
	return result, err
}
func (s *Service) subtitleData(ctx context.Context, address string, index int32, format string, starts ...float64) ([]byte, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-protocol_whitelist", "http,tcp", "-i", address}
	if len(starts) > 0 && starts[0] > 0 {
		args = append(args, "-ss", strconv.FormatFloat(starts[0], 'f', 3, 64))
	}
	args = append(args, "-map", "0:"+strconv.Itoa(int(index)), "-c:s", format, "-f", format, "pipe:1")
	command := exec.CommandContext(ctx, s.executable, args...)
	command.Stderr = io.Discard
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = command.Start(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(output, 8*1024*1024+1))
	if err != nil || len(data) > 8*1024*1024 {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, ErrFormat
	}
	if command.Wait() != nil {
		return nil, ErrFormat
	}
	return data, nil
}
func (s *Service) Subtitles(ctx context.Context, source http.Handler, index int32, starts ...float64) ([]byte, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var data []byte
	err := withSource(ctx, source, func(address string) error {
		var err error
		data, err = s.subtitleData(ctx, address, index, "webvtt", starts...)
		return err
	})
	return data, err
}
func (s *Service) selectionArguments(ctx context.Context, source, mode string, start float64, selection Selection) ([]string, func(), error) {
	args := Arguments(source, mode, start)
	cleanup := func() {}
	if selection.Audio != nil {
		for i, value := range args {
			if value == "0:a:0" || value == "0:a:0?" {
				args[i] = "0:" + strconv.Itoa(int(*selection.Audio))
			}
		}
	}
	if selection.Subtitle == nil || mode != "video" {
		return args, cleanup, nil
	}
	if selection.Subtitle.Bitmap {
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-map" && args[i+1] == "0:v:0" {
				args[i+1] = "[captioned]"
			}
			if args[i] == "-vf" {
				scale := args[i+1]
				args = append(args[:i], args[i+2:]...)
				args = append(args, "-filter_complex", "[0:v:0][0:"+strconv.Itoa(int(selection.Subtitle.Index))+"]overlay,"+scale+"[captioned]")
				break
			}
		}
		return args, cleanup, nil
	}
	data, err := s.subtitleData(ctx, source, selection.Subtitle.Index, "ass")
	if err != nil {
		return nil, cleanup, err
	}
	file, err := os.CreateTemp("", "teldrive-subtitles-*.ass")
	if err != nil {
		return nil, cleanup, err
	}
	cleanup = func() { _ = os.Remove(file.Name()) }
	if _, err = file.Write(data); err != nil {
		file.Close()
		cleanup()
		return nil, func() {}, err
	}
	_ = file.Close()
	path := filepath.ToSlash(file.Name())
	path = strings.NewReplacer("\\", "\\\\", ":", "\\:", "'", "\\'").Replace(path)
	for i, value := range args {
		if value == "-vf" && i+1 < len(args) {
			args[i+1] = "subtitles=filename='" + path + "'," + args[i+1]
			return args, cleanup, nil
		}
	}
	cleanup()
	return nil, func() {}, errors.New("video filter unavailable")
}
