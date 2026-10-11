package telegramstore

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrInvalidRequest   = errors.New("invalid Telegram storage request")
	ErrInvalidChannel   = errors.New("invalid Telegram channel")
	ErrMessageNotFound  = errors.New("Telegram message not found")
	ErrDocumentNotFound = errors.New("Telegram document not found")
	ErrSizeMismatch     = errors.New("Telegram stored size mismatch")
)

type Operation string

const (
	OperationUpload   Operation = "upload"
	OperationDownload Operation = "download"
	OperationManage   Operation = "manage"
)

type UploadRequest struct {
	UserID    int64
	ChannelID int64
	Name      string
	Reader    io.Reader
	Size      int64
	Threads   int
}

type StoredPart struct {
	ChannelID int64
	MessageID int64
	Size      int64
}

type DocumentMessage struct {
	ID        int64
	CreatedAt time.Time
}

type ListDocumentMessagesRequest struct {
	UserID    int64
	ChannelID int64
	BeforeID  int64
	Limit     int
}

type DocumentMessagePage struct {
	Messages  []DocumentMessage
	BeforeID  int64
	Exhausted bool
}

// DocumentMessageLister is an optional maintenance capability implemented by
// production storage without expanding the core upload/download boundary.
type DocumentMessageLister interface {
	ListDocumentMessages(context.Context, ListDocumentMessagesRequest) (DocumentMessagePage, error)
}

type MetadataRequest struct {
	UserID    int64
	ChannelID int64
	MessageID int64
}

// MetadataReader resolves Telegram document metadata without opening its body.
// Production storage implements it; legacy-size resolution uses it lazily.
type MetadataReader interface {
	Metadata(ctx context.Context, request MetadataRequest) (StoredPart, error)
}

type RangeRequest struct {
	UserID    int64
	ChannelID int64
	MessageID int64
	Offset    int64
	Length    int64 // -1 reads through the end of the document.
}

// DownloadSession reuses one authenticated Telegram client for all metadata and
// range operations belonging to a single caller request.
type DownloadSession interface {
	Metadata(context.Context, MetadataRequest) (StoredPart, error)
	OpenRange(context.Context, RangeRequest) (io.ReadCloser, error)
	Close() error
}

// DownloadSessionOpener is implemented by production storage. Storage fakes and
// local implementations can continue using the base methods through an adapter.
type DownloadSessionOpener interface {
	OpenDownloadSession(context.Context, int64) (DownloadSession, error)
}

type Channel struct {
	ID   int64
	Name string
}

// Storage is the Telegram object boundary used by upload, download, cleanup,
// and rollover services. Tests use deterministic in-memory implementations;
// production uses GotdStorage.
type Storage interface {
	Upload(ctx context.Context, request UploadRequest) (StoredPart, error)
	OpenRange(ctx context.Context, request RangeRequest) (io.ReadCloser, error)
	DeleteMessages(ctx context.Context, userID, channelID int64, messageIDs []int64) error
	CopyPart(ctx context.Context, userID, sourceChannelID, sourceMessageID, destinationChannelID int64) (StoredPart, error)
	CreateChannel(ctx context.Context, userID int64, name string) (Channel, error)
	DeleteChannel(ctx context.Context, userID, channelID int64) error
}

// BotInviter is an optional Telegram administration capability. Production
// GotdStorage implements it; deterministic content-storage fakes do not need to.
type BotInviter interface {
	InviteBot(ctx context.Context, userID, channelID int64, username string) error
}
