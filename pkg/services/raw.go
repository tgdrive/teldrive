package services

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/internal/events"
	"github.com/tgdrive/teldrive/internal/http_range"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/md5"
	"github.com/tgdrive/teldrive/internal/reader"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"go.uber.org/zap"
)

type rawService struct {
	api *apiService
}

func NewRawService(api *apiService) *rawService {
	return &rawService{api: api}
}

func (s *rawService) AuthAttemptEvents(ctx context.Context, params api.AuthAttemptEventsParams, w http.ResponseWriter) error {
	attempt, ok := s.api.authAttempts.get(params.ID)
	if !ok {
		return &apiError{err: fmt.Errorf("auth attempt not found"), code: http.StatusNotFound}
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &apiError{err: fmt.Errorf("streaming not supported")}
	}
	ch, latest := attempt.subscribe()
	defer attempt.unsubscribe(ch)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if len(latest) > 0 {
		fmt.Fprintf(w, "data: %s\n\n", latest)
		flusher.Flush()
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case payload, ok := <-ch:
			if !ok {
				return nil
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *rawService) EventsEventsStream(ctx context.Context, params api.EventsEventsStreamParams, w http.ResponseWriter) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &apiError{err: fmt.Errorf("streaming not supported")}
	}
	eventTypes, err := events.ParseEventTypes(params.Types)
	if err != nil {
		return &apiError{err: err, code: http.StatusBadRequest}
	}
	interval := 30 * time.Second
	if v, ok := params.Interval.Get(); ok && v > 0 {
		interval = time.Duration(v) * time.Millisecond
	}
	userID := auth.User(ctx)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	eventChan := s.api.events.Subscribe(userID, eventTypes)
	defer s.api.events.Unsubscribe(userID, eventChan)
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-eventChan:
			if !ok {
				return nil
			}
			data := mapper.ToEventOutFromDTO(event)
			jsonData, err := data.MarshalJSON()
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *rawService) FilesStream(ctx context.Context, params api.FilesStreamParams, w http.ResponseWriter) error {
	user := auth.JWTUser(ctx)
	session := &jetmodel.Sessions{UserID: auth.User(ctx), TgSession: user.TgSession}
	download := false
	if v, ok := params.Download.Get(); ok && v == api.FilesStreamDownload1 {
		download = true
	}
	return s.streamFile(ctx, w, params.ID, session, params.Range.Or(""), download)
}

func (s *rawService) SharesStream(ctx context.Context, params api.SharesStreamParams, w http.ResponseWriter) error {
	if _, err := uuid.Parse(params.ID); err != nil {
		return &apiError{err: err, code: http.StatusBadRequest}
	}
	share, err := s.api.validFileShare(ctx, params.ID, params.ShareToken.Or(""))
	if err != nil {
		return err
	}
	session := &jetmodel.Sessions{UserID: share.UserID}
	download := false
	if v, ok := params.Download.Get(); ok && v == api.SharesStreamDownload1 {
		download = true
	}
	return s.streamFile(ctx, w, params.FileId, session, "", download)
}

func (s *rawService) streamFile(ctx context.Context, w http.ResponseWriter, fileID string, session *jetmodel.Sessions, rawRange string, download bool) error {
	logger := logging.Component("FILE").With(zap.String("file_id", fileID), zap.Int64("user_id", session.UserID))
	file, err := cache.Fetch(ctx, s.api.cache, cache.Key("files", fileID), 0, func() (*jetmodel.Files, error) {
		id, err := uuid.Parse(fileID)
		if err != nil {
			return nil, err
		}
		return s.api.repo.Files.GetByID(ctx, id)
	})
	if err != nil {
		return &apiError{err: err, code: http.StatusBadRequest}
	}
	w.Header().Set("Accept-Ranges", "bytes")
	contentType := defaultContentType
	if file.MimeType != "" {
		contentType = file.MimeType
	}
	if file.Size == nil || *file.Size == 0 {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", "0")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": file.Name}))
		w.WriteHeader(http.StatusOK)
		return nil
	}
	var start, end int64
	status := http.StatusOK
	if rawRange == "" {
		start = 0
		end = *file.Size - 1
	} else {
		ranges, err := http_range.Parse(rawRange, *file.Size)
		if err == http_range.ErrNoOverlap {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", *file.Size))
			return &apiError{err: http_range.ErrNoOverlap, code: http.StatusRequestedRangeNotSatisfiable}
		}
		if err != nil {
			return &apiError{err: err, code: http.StatusBadRequest}
		}
		if len(ranges) > 1 {
			return &apiError{err: fmt.Errorf("multiple ranges are not supported"), code: http.StatusRequestedRangeNotSatisfiable}
		}
		start = ranges[0].Start
		end = ranges[0].End
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, *file.Size))
		status = http.StatusPartialContent
	}
	contentLength := end - start + 1
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", md5.FromString(fileID+strconv.FormatInt(*file.Size, 10))))
	w.Header().Set("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))
	disposition := "inline"
	if download {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))
	w.WriteHeader(status)
	tokens, err := s.api.channelManager.BotTokens(ctx, session.UserID)
	if err != nil {
		logger.Error("stream.bots_fetch_failed", zap.Error(err))
		return &apiError{err: fmt.Errorf("failed to get bots")}
	}
	if limit := s.api.cnf.TG.Stream.BotsLimit; limit > 0 && len(tokens) > limit {
		tokens = tokens[:limit]
	}
	var (
		lr     io.ReadCloser
		client TelegramClient
		token  string
	)
	if len(tokens) == 0 {
		client, err = s.api.telegram.AuthClient(ctx, session.TgSession, 5)
		if err != nil {
			logger.Error("stream.auth_client_failed", zap.Error(err))
			return err
		}
	} else {
		token, _, err = s.api.telegram.SelectBotToken(ctx, TelegramOpStream, session.UserID, tokens)
		if err != nil {
			logger.Error("stream.bot_selection_failed", zap.Error(err))
			return err
		}
		client, err = s.api.telegram.BotClient(ctx, token, 5)
		if err != nil {
			logger.Error("stream.bot_client_failed", zap.Error(err))
			return err
		}
	}
	botID := strconv.FormatInt(session.UserID, 10)
	if token != "" {
		parts := strings.Split(token, ":")
		if len(parts) > 0 {
			botID = parts[0]
		}
	}
	handleStream := func() error {
		if file.ChannelID == nil {
			return fmt.Errorf("missing channel id")
		}
		parts, err := getParts(ctx, s.api.telegram, client, s.api.cache, file.ID.String(), *file.ChannelID, fileParts(file.Parts), file.Encrypted)
		if err != nil {
			return err
		}
		fileRef := &reader.FileRef{ID: file.ID.String(), ChannelID: *file.ChannelID, Encrypted: file.Encrypted}
		lr, err = reader.NewReader(ctx, client.API(), s.api.cache, fileRef, parts, start, end, &s.api.cnf.TG, botID)
		if err != nil {
			return err
		}
		if lr == nil {
			return fmt.Errorf("failed to initialise reader")
		}
		_, err = io.CopyN(w, lr, contentLength)
		if err != nil {
			_ = lr.Close()
		}
		return nil
	}
	return s.api.telegram.RunWithAuth(ctx, client, token, func(ctx context.Context) error { return handleStream() })
}
