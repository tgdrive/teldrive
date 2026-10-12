package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/media"
)

type cachedTracks struct {
	generation int64
	tracks     []media.Track
}

func (h *Handler) fileMediaSource(grant playbackGrant, file *sqlcgen.File) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var value gen.OptString
		if header := r.Header.Get("Range"); header != "" {
			value = gen.NewOptString(header)
		}
		if err := NewRawHandler(h).streamFile(r.Context(), w, grant.ownerID, grant.fileID, file, value, gen.OptETag{}, false); err != nil {
			http.Error(w, "Source unavailable", http.StatusServiceUnavailable)
		}
	})
}
func (h *Handler) mediaTracks(ctx context.Context, grant playbackGrant, file *sqlcgen.File) ([]media.Track, error) {
	h.playback.Lock()
	cached, ok := h.playback.tracks[grant.fileID.String()]
	h.playback.Unlock()
	if ok && cached.generation == file.Generation {
		return cached.tracks, nil
	}
	tracks, err := h.Media.Probe(ctx, h.fileMediaSource(grant, file))
	if err != nil {
		return nil, problem(503, "media_probe_unavailable", "No se pudieron detectar las pistas. Comprueba FFprobe en el servidor", err)
	}
	h.playback.Lock()
	if h.playback.tracks == nil || len(h.playback.tracks) >= 128 {
		h.playback.tracks = make(map[string]cachedTracks)
	}
	h.playback.tracks[grant.fileID.String()] = cachedTracks{file.Generation, tracks}
	h.playback.Unlock()
	return tracks, nil
}
func (h *Handler) GetPlaybackTracks(ctx context.Context, params gen.GetPlaybackTracksParams) (gen.GetPlaybackTracksRes, error) {
	if h.Catalog == nil || h.Downloader == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	grant, file, err := h.resolvePlayback(ctx, params.Ticket)
	if err != nil {
		return nil, err
	}
	result := &gen.MediaTracks{Available: h.Media.ProbeAvailable(), Tracks: []gen.MediaTrack{}}
	if !result.Available {
		return result, nil
	}
	tracks, err := h.mediaTracks(ctx, grant, file)
	if err != nil {
		return nil, err
	}
	for _, track := range tracks {
		result.Tracks = append(result.Tracks, gen.MediaTrack{Index: track.Index, Kind: gen.MediaTrackKind(track.Kind), Codec: track.Codec, Language: track.Language, Title: track.Title, Default: track.Default, Bitmap: track.Bitmap})
	}
	return result, nil
}
func (h *Handler) selectMediaTracks(ctx context.Context, grant playbackGrant, file *sqlcgen.File, audio, subtitle gen.OptInt32) (media.Selection, error) {
	var selection media.Selection
	if !audio.IsSet() && !subtitle.IsSet() {
		return selection, nil
	}
	tracks, err := h.mediaTracks(ctx, grant, file)
	if err != nil {
		return selection, err
	}
	ordinal := 0
	for _, track := range tracks {
		if value, ok := audio.Get(); ok && track.Index == value && track.Kind == "audio" {
			selection.Audio = &value
		}
		if track.Kind == "subtitle" {
			if value, ok := subtitle.Get(); ok && track.Index == value {
				copy := track
				selection.Subtitle = &copy
				selection.SubtitleOrdinal = ordinal
			}
			ordinal++
		}
	}
	if audio.IsSet() && selection.Audio == nil || subtitle.IsSet() && selection.Subtitle == nil {
		return selection, problem(422, "media_track_invalid", "La pista seleccionada no pertenece a este archivo", nil)
	}
	return selection, nil
}
func (h *Handler) subtitleContent(ctx context.Context, ticket string, index int32, start float64) ([]byte, error) {
	grant, file, err := h.resolvePlayback(ctx, ticket)
	if err != nil {
		return nil, err
	}
	selection, err := h.selectMediaTracks(ctx, grant, file, gen.OptInt32{}, gen.NewOptInt32(index))
	if err != nil {
		return nil, err
	}
	if selection.Subtitle.Bitmap {
		return nil, problem(422, "bitmap_subtitles", "Los subtítulos de imagen necesitan reproducción compatible", nil)
	}
	data, err := h.Media.Subtitles(ctx, h.fileMediaSource(grant, file), index, start)
	if err != nil {
		return nil, problem(503, "subtitles_unavailable", "No se pudieron extraer los subtítulos", err)
	}
	return data, nil
}
func (h *RawHandler) GetPlaybackSubtitles(ctx context.Context, params gen.GetPlaybackSubtitlesParams, w http.ResponseWriter) error {
	data, err := h.handler.subtitleContent(ctx, params.Ticket, params.Track, params.Start.Or(0))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	_, err = w.Write(data)
	return err
}
