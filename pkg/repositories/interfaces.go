package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
)

var (
	ErrNotFound = errors.New("repository: not found")
	ErrConflict = errors.New("repository: conflict")
)

type FileQueryParams struct {
	UserID     int64
	Operation  string
	Status     string
	ParentID   string
	Path       string
	Name       string
	Type       string
	Category   []string
	Query      string
	SearchType string
	DeepSearch bool
	UpdatedAt  string
	Shared     bool
	Sort       string
	Order      string
	Cursor     string
	Limit      int
}

// CategoryStats represents category statistics
type CategoryStats struct {
	Category   string
	TotalFiles int64
	TotalSize  int64
}

type UploadStat struct {
	UploadDate    time.Time
	TotalUploaded int64
}

type FileUpdate struct {
	Name      *string
	Type      *string
	MimeType  *string
	Size      *int64
	Status    *string
	ParentID  *uuid.UUID
	ChannelID *int64
	Parts     *string
	Encrypted *bool
	Category  *string
	Hash      *string
	UpdatedAt *time.Time
}

type ChannelUpdate struct {
	ChannelName *string
	Selected    *bool
}

type ShareUpdate struct {
	Password  *string
	ExpiresAt *time.Time
	UpdatedAt *time.Time
}

type UserUpdate struct {
	Name      *string
	UserName  *string
	IsPremium *bool
	UpdatedAt *time.Time
}

// FileRepository defines operations for file persistence
type FileRepository interface {
	Create(ctx context.Context, file *model.Files) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Files, error)
	GetByIDAndUser(ctx context.Context, id uuid.UUID, userID int64) (*model.Files, error)
	GetByChannelID(ctx context.Context, channelID int64) ([]model.Files, error)
	GetActiveByNameAndParent(ctx context.Context, userID int64, name string, parentID *uuid.UUID) (*model.Files, error)
	Update(ctx context.Context, id uuid.UUID, update FileUpdate) error
	MoveSingle(ctx context.Context, id uuid.UUID, userID int64, parentID *uuid.UUID, name *string) error
	MoveBulk(ctx context.Context, ids []uuid.UUID, userID int64, parentID *uuid.UUID) error
	Delete(ctx context.Context, ids []uuid.UUID) error
	ResolvePathID(ctx context.Context, path string, userID int64) (*uuid.UUID, error)
	List(ctx context.Context, params FileQueryParams) ([]model.Files, error)
	GetFullPath(ctx context.Context, fileID uuid.UUID) (string, error)
	CategoryStats(ctx context.Context, userID int64) ([]CategoryStats, error)
	DeleteBulk(ctx context.Context, fileIDs []uuid.UUID, userID int64, targetStatus string) error
	CreateDirectories(ctx context.Context, userID int64, path string) (*uuid.UUID, error)
}

// SessionRepository defines operations for session persistence
type SessionRepository interface {
	Create(ctx context.Context, session *model.Sessions) error
	GetByID(ctx context.Context, id string) (*model.Sessions, error)
	GetByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (*model.Sessions, error)
	GetByUserID(ctx context.Context, userID int64) ([]model.Sessions, error)
	UpdateRefreshTokenHash(ctx context.Context, id string, refreshTokenHash string) error
	Revoke(ctx context.Context, id string) error
}

// APIKeyRepository defines operations for API key persistence
type APIKeyRepository interface {
	Create(ctx context.Context, key *model.APIKeys) error
	ListByUserID(ctx context.Context, userID int64) ([]model.APIKeys, error)
	GetActiveByTokenHash(ctx context.Context, tokenHash string, now time.Time) (*model.APIKeys, error)
	TouchLastUsed(ctx context.Context, id uuid.UUID, usedAt time.Time) error
	Revoke(ctx context.Context, userID int64, id string) error
}

// UploadRepository defines operations for upload part persistence
type UploadRepository interface {
	Create(ctx context.Context, upload *model.Uploads) error
	GetByUploadID(ctx context.Context, uploadID string) ([]model.Uploads, error)
	GetByUploadIDAndRetention(ctx context.Context, uploadID string, retention time.Duration) ([]model.Uploads, error)
	Delete(ctx context.Context, uploadID string) error
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
	StatsByDays(ctx context.Context, userID int64, days int) ([]UploadStat, error)
}

// ChannelRepository defines operations for channel persistence
type ChannelRepository interface {
	Create(ctx context.Context, channel *model.Channels) error
	GetByUserID(ctx context.Context, userID int64) ([]model.Channels, error)
	GetByChannelID(ctx context.Context, channelID int64) (*model.Channels, error)
	GetSelected(ctx context.Context, userID int64) (*model.Channels, error)
	Update(ctx context.Context, channelID int64, update ChannelUpdate) error
	Delete(ctx context.Context, channelID int64) error
	DeleteByUserID(ctx context.Context, userID int64) error
}

// BotRepository defines operations for bot persistence
type BotRepository interface {
	Create(ctx context.Context, bot *model.Bots) error
	GetByUserID(ctx context.Context, userID int64) ([]model.Bots, error)
	GetTokensByUserID(ctx context.Context, userID int64) ([]string, error)
	DeleteByUserID(ctx context.Context, userID int64) error
}

// UserRepository defines operations for user persistence
type UserRepository interface {
	Create(ctx context.Context, user *model.Users) error
	GetByID(ctx context.Context, userID int64) (*model.Users, error)
	Update(ctx context.Context, userID int64, update UserUpdate) error
}

// ShareRepository defines operations for file share persistence
type ShareRepository interface {
	Create(ctx context.Context, share *model.FileShares) error
	GetByFileID(ctx context.Context, fileID uuid.UUID) ([]model.FileShares, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.FileShares, error)
	Update(ctx context.Context, id uuid.UUID, update ShareUpdate) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// EventRepository defines operations for event persistence
type EventRepository interface {
	Create(ctx context.Context, event *model.Events) error
	GetRecent(ctx context.Context, userID int64, since time.Time, limit int) ([]model.Events, error)
	GetSince(ctx context.Context, since time.Time, limit int) ([]model.Events, error)
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
	DeleteOlderThanForUser(ctx context.Context, userID int64, before time.Time) (int64, error)
}

type PeriodicJobRepository interface {
	Create(ctx context.Context, job *PeriodicJob) error
	ListByUserID(ctx context.Context, userID int64) ([]PeriodicJob, error)
	ListEnabled(ctx context.Context) ([]PeriodicJob, error)
	GetByIDAndUserID(ctx context.Context, id uuid.UUID, userID int64) (*PeriodicJob, error)
	GetByNameAndUserID(ctx context.Context, userID int64, name string) (*PeriodicJob, error)
	Update(ctx context.Context, id uuid.UUID, userID int64, job PeriodicJob) error
	Delete(ctx context.Context, id uuid.UUID, userID int64) error
	SetEnabled(ctx context.Context, id uuid.UUID, userID int64, enabled bool, updatedAt time.Time) error
}

type PeriodicJob struct {
	ID             uuid.UUID
	UserID         int64
	Name           string
	Kind           string
	Args           PeriodicJobArgs
	CronExpression string
	Enabled        bool
	System         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type PeriodicJobArgs interface {
	periodicJobArgs()
}

type SyncRunPeriodicArgs struct {
	Source         string            `json:"source"`
	DestinationDir string            `json:"destinationDir"`
	Headers        map[string]string `json:"headers,omitempty"`
	Proxy          *string           `json:"proxy,omitempty"`
	Filters        *SyncFiltersArgs  `json:"filters,omitempty"`
	Options        *SyncOptionsArgs  `json:"options,omitempty"`
}

func (SyncRunPeriodicArgs) periodicJobArgs() {}

type SyncFiltersArgs struct {
	Include          []string `json:"include,omitempty"`
	Exclude          []string `json:"exclude,omitempty"`
	ExcludeIfPresent []string `json:"excludeIfPresent,omitempty"`
	MinSize          *int64   `json:"minSize,omitempty"`
	MaxSize          *int64   `json:"maxSize,omitempty"`
}

type SyncOptionsArgs struct {
	PartSize    *int64 `json:"partSize,omitempty"`
	Concurrency *int   `json:"concurrency,omitempty"`
	Sync        *bool  `json:"sync,omitempty"`
}

type CleanOldEventsPeriodicArgs struct {
	Retention string `json:"retention,omitempty"`
}

func (CleanOldEventsPeriodicArgs) periodicJobArgs() {}

type CleanStaleUploadsPeriodicArgs struct {
	Retention string `json:"retention,omitempty"`
}

func (CleanStaleUploadsPeriodicArgs) periodicJobArgs() {}

type CleanPendingFilesPeriodicArgs struct{}

func (CleanPendingFilesPeriodicArgs) periodicJobArgs() {}

// KVRepository defines operations for key-value storage
type KVRepository interface {
	Set(ctx context.Context, item *model.Kv) error
	Get(ctx context.Context, key string) (*model.Kv, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
	Iterate(ctx context.Context, prefix string, fn func(key string, value []byte) error) error
}

// Repositories holds all repository instances
type Repositories struct {
	Pool         *pgxpool.Pool
	Files        FileRepository
	Sessions     SessionRepository
	APIKeys      APIKeyRepository
	Uploads      UploadRepository
	Channels     ChannelRepository
	Bots         BotRepository
	Users        UserRepository
	Shares       ShareRepository
	Events       EventRepository
	PeriodicJobs PeriodicJobRepository
	KV           KVRepository
}
