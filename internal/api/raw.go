package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"syscall"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
)

// RawHandler implements ogen raw response operations. The generated router owns
// routing, parameter decoding, security, and error serialization; this type only
// writes successful streaming responses directly to the ResponseWriter.
type RawHandler struct {
	handler *Handler
}

func NewRawHandler(handler *Handler) *RawHandler {
	return &RawHandler{handler: handler}
}

func (h *RawHandler) DownloadFile(ctx context.Context, params gen.DownloadFileParams, w http.ResponseWriter) error {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return mapServiceError(err)
	}
	if h.handler == nil || h.handler.Catalog == nil || h.handler.Downloader == nil {
		return mapServiceError(ErrOperationUnavailable)
	}
	fileID := googleUUID(params.FileId)
	file, err := h.handler.Catalog.Get(ctx, userID, fileID)
	if err != nil {
		return mapServiceError(err)
	}
	return h.streamFile(ctx, w, userID, fileID, file, params.Range, params.IfNoneMatch)
}

func (h *RawHandler) DownloadPublicShare(ctx context.Context, params gen.DownloadPublicShareParams, w http.ResponseWriter) error {
	if h.handler == nil || h.handler.Shares == nil || h.handler.Downloader == nil {
		return mapServiceError(ErrOperationUnavailable)
	}
	password := params.XSharePassword.Or("")
	resolved, err := h.handler.Shares.Resolve(ctx, params.Token, password)
	if err != nil {
		return mapServiceError(err)
	}
	file := resolved.File
	etag := contentETag(file)
	if !params.Range.IsSet() && ifNoneMatch(params.IfNoneMatch) == string(etag) {
		w.Header().Set("ETag", string(etag))
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
	resolved, err = h.handler.Shares.ReserveDownload(ctx, params.Token, password)
	if err != nil {
		return mapServiceError(err)
	}
	file = resolved.File
	fileID, ok := dbtypes.GoogleUUID(file.ID)
	if !ok {
		return mapServiceError(transfer.ErrInvalidDownload)
	}
	return h.streamFile(ctx, w, resolved.Share.OwnerID, fileID, file, params.Range, params.IfNoneMatch)
}

func (h *RawHandler) streamFile(ctx context.Context, w http.ResponseWriter, userID int64, fileID uuid.UUID, file *sqlcgen.File, rangeValue gen.OptString, noneMatch gen.OptETag) error {
	if file.Kind != sqlcgen.FileKindFile || file.Status != sqlcgen.FileStatusActive || !file.Size.Valid || file.Size.Int64 < 0 {
		return mapServiceError(transfer.ErrInvalidDownload)
	}
	rangeSpec, partial, err := parseRange(rangeValue, file.Size.Int64)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", file.Size.Int64))
		return mapServiceError(err)
	}
	etag := contentETag(file)
	if !partial && ifNoneMatch(noneMatch) == string(etag) {
		w.Header().Set("ETag", string(etag))
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	download, err := h.handler.Downloader.Open(ctx, transfer.DownloadRequest{
		UserID: userID, FileID: fileID, Offset: rangeSpec.Offset, Length: rangeSpec.Length,
	})
	if err != nil {
		return mapServiceError(err)
	}
	defer download.Reader.Close()

	status := http.StatusOK
	if partial {
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rangeSpec.Offset, rangeSpec.Offset+rangeSpec.Length-1, download.TotalSize))
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Disposition", contentDisposition(file.Name))
	w.Header().Set("Content-Length", strconv.FormatInt(rangeSpec.Length, 10))
	w.Header().Set("Content-Type", download.ContentType)
	w.Header().Set("ETag", string(etag))
	w.Header().Set("Last-Modified", file.ModTime.Time.UTC().Format(http.TimeFormat))
	w.WriteHeader(status)
	_, err = io.CopyN(w, download.Reader, rangeSpec.Length)
	if err != nil && !isExpectedStreamEnd(ctx, err) {
		// Headers are already committed, so a JSON error response cannot replace
		// this stream. The Content-Length mismatch tells the client it was truncated.
		slog.ErrorContext(ctx, "api.stream_failed", "file_id", fileID, "offset", rangeSpec.Offset, "length", rangeSpec.Length, "error", err)
	}
	return nil
}

func isExpectedStreamEnd(ctx context.Context, err error) bool {
	return ctx.Err() != nil || isClientDisconnect(err)
}

func isClientDisconnect(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE)
}

func ifNoneMatch(value gen.OptETag) string {
	if value, ok := value.Get(); ok {
		return strings.TrimSpace(string(value))
	}
	return ""
}
