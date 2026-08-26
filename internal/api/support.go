package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/events"
	"github.com/tgdrive/teldrive/v2/internal/fileops"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

type Problem struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func (p *Problem) Error() string {
	if p.Cause == nil {
		return p.Message
	}
	return p.Message + ": " + p.Cause.Error()
}
func (p *Problem) Unwrap() error { return p.Cause }

func problem(status int, code, message string, cause error) error {
	return &Problem{Status: status, Code: code, Message: message, Cause: cause}
}

func mapServiceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return problem(http.StatusGatewayTimeout, "request_timeout", "request timed out", err)
	case errors.Is(err, ErrUnauthenticated), errors.Is(err, authn.ErrInvalidCredential), errors.Is(err, authn.ErrUserNotAllowed):
		return problem(http.StatusUnauthorized, "unauthorized", "authentication is required", err)
	case errors.Is(err, shares.ErrPasswordNeeded), errors.Is(err, shares.ErrInvalidPassword):
		return problem(http.StatusUnauthorized, "share_password_required", "a valid share password is required", err)
	case errors.Is(err, shares.ErrForbidden):
		return problem(http.StatusForbidden, "forbidden", "operation is not permitted", err)
	case errors.Is(err, events.ErrInvalidCursor):
		return problem(http.StatusUnprocessableEntity, "invalid_event_cursor", "event cursor is invalid", err)
	case errors.Is(err, events.ErrTooManyConnections):
		return problem(http.StatusTooManyRequests, "too_many_event_streams", "too many event streams are open", err)
	case errors.Is(err, events.ErrServiceClosed), errors.Is(err, ErrOperationUnavailable), errors.Is(err, transfer.ErrUploadNotConfigured), errors.Is(err, transfer.ErrDownloadNotConfigured):
		return problem(http.StatusServiceUnavailable, "service_unavailable", "operation is not available", err)
	case errors.Is(err, catalog.ErrNotFound), errors.Is(err, uploads.ErrNotFound), errors.Is(err, authn.ErrSessionNotFound), errors.Is(err, authn.ErrAPIKeyNotFound), errors.Is(err, bots.ErrNotFound), errors.Is(err, channels.ErrInvalidChannel), errors.Is(err, shares.ErrNotFound), errors.Is(err, fileops.ErrNotFound):
		return problem(http.StatusNotFound, "not_found", "resource was not found", err)
	case errors.Is(err, uploads.ErrExpired), errors.Is(err, authn.ErrFlowNotFound), errors.Is(err, shares.ErrExpired):
		return problem(http.StatusGone, "upload_expired", "upload session has expired", err)
	case errors.Is(err, catalog.ErrConflict), errors.Is(err, catalog.ErrCycle), errors.Is(err, uploads.ErrNameConflict), errors.Is(err, uploads.ErrInvalidState), errors.Is(err, uploads.ErrPartBusy), errors.Is(err, uploads.ErrPartConflict), errors.Is(err, channels.ErrSelectedChannel), errors.Is(err, channels.ErrChannelInUse), errors.Is(err, fileops.ErrNotTrashed):
		return problem(http.StatusConflict, "conflict", "operation conflicts with current state", err)
	case errors.Is(err, catalog.ErrPrecondition):
		return problem(http.StatusPreconditionFailed, "precondition_failed", "resource generation does not match", err)
	case errors.Is(err, transfer.ErrRangeNotSatisfiable):
		return problem(http.StatusRequestedRangeNotSatisfiable, "range_not_satisfiable", "requested byte range is not satisfiable", err)
	case errors.Is(err, uploads.ErrHashMismatch), errors.Is(err, transfer.ErrChecksumMismatch):
		return problem(http.StatusUnprocessableEntity, "hash_mismatch", "content hash does not match", err)
	case errors.Is(err, catalog.ErrInvalidName), errors.Is(err, catalog.ErrInvalidParent), errors.Is(err, catalog.ErrInvalidOwner), errors.Is(err, uploads.ErrInvalidInput), errors.Is(err, uploads.ErrInvalidParent), errors.Is(err, uploads.ErrInvalidChannel), errors.Is(err, uploads.ErrUnsupportedConflictPolicy), errors.Is(err, transfer.ErrInvalidUpload), errors.Is(err, transfer.ErrInvalidDownload), errors.Is(err, authn.ErrInvalidInput), errors.Is(err, authn.ErrCodeInvalid), errors.Is(err, authn.ErrPasswordInvalid), errors.Is(err, bots.ErrInvalidInput), errors.Is(err, bots.ErrNotBot), errors.Is(err, shares.ErrInvalidInput), errors.Is(err, fileops.ErrInvalidInput):
		return problem(http.StatusUnprocessableEntity, "invalid_request", "request is invalid", err)
	default:
		return problem(http.StatusInternalServerError, "internal_error", "request failed", err)
	}
}

func ErrorHandler(ctx context.Context, w http.ResponseWriter, _ *http.Request, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	requestID := middleware.GetReqID(ctx)
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "request failed"
	var securityErr *ogenerrors.SecurityError
	var decodeRequestErr *ogenerrors.DecodeRequestError
	var decodeParamsErr *ogenerrors.DecodeParamsError
	switch {
	case errors.As(err, &securityErr), errors.Is(err, ErrUnauthenticated):
		status = http.StatusUnauthorized
		code = "unauthorized"
		message = "authentication is required"
	case errors.As(err, &decodeRequestErr), errors.As(err, &decodeParamsErr):
		status = http.StatusBadRequest
		code = "invalid_request"
		message = "request could not be decoded"
	case errors.Is(err, ErrOperationUnavailable):
		status = http.StatusServiceUnavailable
		code = "service_unavailable"
		message = "operation is not available"
	}
	var p *Problem
	if errors.As(err, &p) {
		if p.Status >= 400 && p.Status <= 599 {
			status = p.Status
		}
		if p.Code != "" {
			code = p.Code
		}
		if p.Message != "" {
			message = p.Message
		}
	}
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(ctx, "api.request_failed", "status", status, "code", code, "request_id", requestID, "error", err)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code": code, "message": message,
		},
	})
}

func NewServer(handler *Handler, security *Security, opts ...gen.ServerOption) (*gen.Server, error) {
	opts = append([]gen.ServerOption{gen.WithErrorHandler(ErrorHandler)}, opts...)
	return gen.NewServer(handler, NewRawHandler(handler), security, opts...)
}

func generationETag(generation int64) gen.ETag {
	return gen.ETag(fmt.Sprintf(`"%d"`, generation))
}

func parseGenerationETag(value gen.OptETag) (*int64, error) {
	etag, ok := value.Get()
	if !ok {
		return nil, nil
	}
	raw := strings.TrimSpace(string(etag))
	raw = strings.TrimPrefix(raw, "W/")
	raw = strings.Trim(raw, `"`)
	generation, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || generation < 0 {
		return nil, errors.New("invalid generation ETag")
	}
	return &generation, nil
}

func googleUUID(value gen.UUID) uuid.UUID { return uuid.UUID(value) }
func apiUUID(value uuid.UUID) gen.UUID    { return gen.UUID(value) }

func optionalAPIUUID(value pgtype.UUID) gen.OptUUID {
	id, ok := dbtypes.GoogleUUID(value)
	if !ok {
		return gen.OptUUID{}
	}
	return gen.NewOptUUID(apiUUID(id))
}

func optionalGoogleUUID(value gen.OptUUID) *uuid.UUID {
	id, ok := value.Get()
	if !ok {
		return nil
	}
	converted := googleUUID(id)
	return &converted
}

func fileEntry(file *sqlcgen.File) (gen.FileEntry, error) {
	id, ok := dbtypes.GoogleUUID(file.ID)
	if !ok {
		return gen.FileEntry{}, errors.New("file has invalid id")
	}
	entry := gen.FileEntry{
		ID: apiUUID(id), ParentId: optionalAPIUUID(file.ParentID), Name: file.Name,
		Kind: gen.FileKind(file.Kind), Encryption: file.Encryption,
		Status: gen.FileStatus(file.Status), ModTime: file.ModTime.Time, Generation: file.Generation,
		CreatedAt: file.CreatedAt.Time, UpdatedAt: file.UpdatedAt.Time,
	}
	if file.MimeType.Valid {
		entry.MimeType = gen.NewOptString(file.MimeType.String)
	}
	if file.Size.Valid {
		entry.Size = gen.NewOptInt64(file.Size.Int64)
	}
	if file.HashAlgorithm.Valid && file.HashValue.Valid {
		entry.Hash = gen.NewOptFileHash(gen.FileHash{
			Algorithm: gen.HashAlgorithm(file.HashAlgorithm.String), Value: gen.Checksum(file.HashValue.String),
		})
	}
	return entry, nil
}

func uploadSession(session *sqlcgen.UploadSession) (gen.UploadSession, error) {
	id, ok := dbtypes.GoogleUUID(session.ID)
	if !ok {
		return gen.UploadSession{}, errors.New("upload has invalid id")
	}
	result := gen.UploadSession{
		ID: apiUUID(id), ParentId: optionalAPIUUID(session.ParentID), Name: session.Name,
		ExpectedSize: session.ExpectedSize, ModTime: session.ModTime.Time,
		Encryption: session.Encryption, ConflictPolicy: gen.NameConflictPolicy(session.ConflictPolicy),
		PartSize: session.PartSize, State: gen.UploadState(session.State), ExpiresAt: session.ExpiresAt.Time,
		CreatedAt: session.CreatedAt.Time, FileId: optionalAPIUUID(session.FileID),
	}
	if session.ExpectedHashAlgorithm.Valid && session.ExpectedHashValue.Valid {
		result.ExpectedHash = gen.NewOptFileHash(gen.FileHash{
			Algorithm: gen.HashAlgorithm(session.ExpectedHashAlgorithm.String), Value: gen.Checksum(session.ExpectedHashValue.String),
		})
	}
	if session.MimeType.Valid {
		result.MimeType = gen.NewOptString(session.MimeType.String)
	}
	if session.CompletedAt.Valid {
		result.CompletedAt = gen.NewOptDateTime(session.CompletedAt.Time)
	}
	return result, nil
}

func uploadPart(part *sqlcgen.UploadPart) (gen.UploadPart, error) {
	uploadID, ok := dbtypes.GoogleUUID(part.UploadID)
	if !ok {
		return gen.UploadPart{}, errors.New("upload part has invalid upload id")
	}
	result := gen.UploadPart{
		UploadId: apiUUID(uploadID), PartNo: part.PartNo, State: gen.UploadPartState(part.State),
		PlainSize: part.PlainSize, CreatedAt: part.CreatedAt.Time, UpdatedAt: part.UpdatedAt.Time,
	}
	if part.StoredSize.Valid {
		result.StoredSize = gen.NewOptInt64(part.StoredSize.Int64)
	}
	if part.Checksum.Valid {
		result.Checksum = gen.NewOptChecksum(gen.Checksum(part.Checksum.String))
	}
	return result, nil
}

func encodeCursor(value any) gen.OptCursor {
	data, err := json.Marshal(value)
	if err != nil {
		return gen.OptCursor{}
	}
	return gen.NewOptCursor(gen.Cursor(base64.RawURLEncoding.EncodeToString(data)))
}

func decodeCursor(value gen.OptCursor, target any) error {
	cursor, ok := value.Get()
	if !ok {
		return nil
	}
	data, err := base64.RawURLEncoding.DecodeString(string(cursor))
	if err != nil {
		return errors.New("invalid cursor")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return errors.New("invalid cursor")
	}
	return nil
}

type fileCursor struct {
	Name  string    `json:"name,omitempty"`
	Sort  string    `json:"sort,omitempty"`
	Order string    `json:"order,omitempty"`
	Value string    `json:"value,omitempty"`
	ID    uuid.UUID `json:"id"`
}
type uploadCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}
type partCursor struct {
	PartNo int32 `json:"part_no"`
}

type byteRange struct{ Offset, Length int64 }

func parseRange(value gen.OptString, size int64) (byteRange, bool, error) {
	if size < 0 {
		return byteRange{}, value.IsSet(), transfer.ErrRangeNotSatisfiable
	}
	raw, ok := value.Get()
	if !ok || strings.TrimSpace(raw) == "" {
		return byteRange{Offset: 0, Length: size}, false, nil
	}
	if !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		return byteRange{}, true, transfer.ErrRangeNotSatisfiable
	}
	parts := strings.SplitN(strings.TrimPrefix(raw, "bytes="), "-", 2)
	if len(parts) != 2 {
		return byteRange{}, true, transfer.ErrRangeNotSatisfiable
	}
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 || size == 0 {
			return byteRange{}, true, transfer.ErrRangeNotSatisfiable
		}
		if suffix > size {
			suffix = size
		}
		return byteRange{Offset: size - suffix, Length: suffix}, true, nil
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return byteRange{}, true, transfer.ErrRangeNotSatisfiable
	}
	if parts[1] == "" {
		return byteRange{Offset: start, Length: size - start}, true, nil
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || end < start {
		return byteRange{}, true, transfer.ErrRangeNotSatisfiable
	}
	if end >= size {
		end = size - 1
	}
	return byteRange{Offset: start, Length: end - start + 1}, true, nil
}

func contentDisposition(name string, attachment bool) string {
	disposition := "inline"
	if attachment {
		disposition = "attachment"
	}
	value := mime.FormatMediaType(disposition, map[string]string{"filename": name})
	if value == "" {
		return disposition
	}
	return value
}
