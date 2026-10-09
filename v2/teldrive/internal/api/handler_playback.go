package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/media"
	"github.com/tgdrive/teldrive/v2/internal/shares"
)

type playbackGrant struct {
	actorID, ownerID int64
	fileID           uuid.UUID
	generation       int64
	expires          time.Time
	receipt          *shares.Public
}

type playbackRegistry struct {
	sync.Mutex
	grants map[string]playbackGrant
}

func (h *Handler) addPlayback(grant playbackGrant) (*gen.PlaybackSession, error) {
	h.playback.Lock()
	defer h.playback.Unlock()
	if h.playback.grants == nil {
		h.playback.grants = make(map[string]playbackGrant)
	}
	now := time.Now()
	for key, g := range h.playback.grants {
		if !g.expires.After(now) {
			delete(h.playback.grants, key)
		}
	}
	if len(h.playback.grants) >= 1024 {
		return nil, problem(429, "playback_busy", "Hay demasiadas sesiones de reproducción abiertas", nil)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	ticket := hex.EncodeToString(key)
	grant.expires = now.Add(2 * time.Hour)
	if grant.receipt != nil && grant.receipt.Share.ExpiresAt.Valid && grant.receipt.Share.ExpiresAt.Time.Before(grant.expires) {
		grant.expires = grant.receipt.Share.ExpiresAt.Time
	}
	h.playback.grants[ticket] = grant
	return &gen.PlaybackSession{Ticket: ticket, ExpiresAt: grant.expires, ConversionAvailable: h.Media.Available()}, nil
}

func (h *Handler) CreatePlayback(ctx context.Context, params gen.CreatePlaybackParams) (gen.CreatePlaybackRes, error) {
	if h.Catalog == nil || h.Downloader == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	fileID := googleUUID(params.FileId)
	access, err := h.resolveAuthenticatedFileAccess(ctx, fileID, false)
	if err != nil {
		return nil, mapServiceError(err)
	}
	file, err := h.Catalog.Get(ctx, access.OwnerID, fileID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if file.Kind != sqlcgen.FileKindFile || file.Status != sqlcgen.FileStatusActive {
		return nil, mapServiceError(shares.ErrNotFound)
	}
	actor, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return h.addPlayback(playbackGrant{actorID: actor, ownerID: access.OwnerID, fileID: fileID, generation: file.Generation})
}

func (h *Handler) CreatePublicPlayback(ctx context.Context, params gen.CreatePublicPlaybackParams) (gen.CreatePublicPlaybackRes, error) {
	if h.Shares == nil || h.Downloader == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	fileID := googleUUID(params.FileId)
	// One reservation covers all Range requests, conversion retries and seeking.
	resolved, err := h.Shares.ReserveFileDownload(ctx, params.Token, params.XSharePassword.Or(""), fileID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if resolved.File.Kind != sqlcgen.FileKindFile {
		return nil, mapServiceError(shares.ErrNotFound)
	}
	return h.addPlayback(playbackGrant{ownerID: resolved.Share.OwnerID, fileID: fileID, generation: resolved.File.Generation, receipt: resolved})
}

func (h *RawHandler) StreamPlayback(ctx context.Context, params gen.StreamPlaybackParams, w http.ResponseWriter) error {
	if h.handler == nil || h.handler.Catalog == nil || h.handler.Downloader == nil {
		return mapServiceError(ErrOperationUnavailable)
	}
	h.handler.playback.Lock()
	grant, ok := h.handler.playback.grants[params.Ticket]
	h.handler.playback.Unlock()
	if !ok || !grant.expires.After(time.Now()) {
		return problem(401, "playback_expired", "La sesión de reproducción venció. Vuelve a abrir el archivo", nil)
	}
	if grant.receipt != nil {
		if err := h.handler.Shares.ValidateDownloadReceipt(ctx, grant.receipt, grant.fileID); err != nil {
			return mapServiceError(err)
		}
	} else if h.handler.Shares != nil {
		access, err := h.handler.Shares.ResolveAccess(ctx, grant.actorID, grant.fileID, false)
		if err != nil {
			return mapServiceError(err)
		}
		if access.OwnerID != grant.ownerID {
			return mapServiceError(shares.ErrForbidden)
		}
	}
	file, err := h.handler.Catalog.Get(ctx, grant.ownerID, grant.fileID)
	if err != nil {
		return mapServiceError(err)
	}
	if file.Status != sqlcgen.FileStatusActive || file.Generation != grant.generation {
		return mapServiceError(shares.ErrNotFound)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	mode := string(params.Mode.Or(gen.StreamPlaybackModeOriginal))
	if mode == "original" {
		return h.streamFile(ctx, w, grant.ownerID, grant.fileID, file, params.Range, params.IfNoneMatch, params.Download.IsSet())
	}
	err = h.handler.Media.Convert(ctx, w, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rangeValue gen.OptString
		if value := r.Header.Get("Range"); value != "" {
			rangeValue = gen.NewOptString(value)
		}
		if err := h.streamFile(r.Context(), w, grant.ownerID, grant.fileID, file, rangeValue, gen.OptETag{}, false); err != nil {
			http.Error(w, "Source unavailable", 503)
		}
	}), mode, params.Start.Or(0))
	switch {
	case errors.Is(err, media.ErrUnavailable):
		return problem(503, "media_unavailable", "Instala FFmpeg con libx264 y libmp3lame en el servidor", err)
	case errors.Is(err, media.ErrBusy):
		return problem(429, "media_busy", "El servidor está convirtiendo otros archivos. Inténtalo de nuevo", err)
	case errors.Is(err, media.ErrFormat):
		return problem(422, "media_format", "No se pudo convertir el archivo. Puedes descargarlo para abrirlo en otro reproductor", err)
	}
	return err
}
