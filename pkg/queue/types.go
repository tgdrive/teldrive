package queue

import "context"

const (
	JobKindFilesCopy    = "files.copy"
	JobKindFilesMove    = "files.move"
	JobKindFilesDelete  = "files.delete"
	JobKindSyncRun      = "sync.run"
	JobKindSyncTransfer = "sync.transfer"

	JobKindCleanOldEvents   = "clean.old_events"
	JobKindCleanStaleUpload = "clean.stale_uploads"
	JobKindCleanPendingFile = "clean.pending_files"
)

type JobItem struct {
	ID              string `json:"id"`
	DestinationName string `json:"destinationName,omitempty"`
}

type SyncRunJobArgs struct {
	UserID         int64             `json:"userId" river:"unique"`
	RunID          string            `json:"runId,omitempty"`
	Source         string            `json:"source"`
	DestinationDir string            `json:"destinationDir"`
	Headers        map[string]string `json:"headers,omitempty"`
	Proxy          string            `json:"proxy,omitempty"`
	Filters        SyncFilters       `json:"filters,omitempty"`
	Options        SyncOptions       `json:"options,omitempty"`
	PollInterval   int               `json:"pollInterval,omitempty"`
}

type SyncFilters struct {
	Include          []string `json:"include,omitempty"`
	Exclude          []string `json:"exclude,omitempty"`
	MinSize          int64    `json:"minSize,omitempty"`
	MaxSize          int64    `json:"maxSize,omitempty"`
	ExcludeIfPresent []string `json:"excludeIfPresent,omitempty"`
}

type SyncOptions struct {
	PartSize int64 `json:"partSize,omitempty"`
	Sync     bool  `json:"sync,omitempty"`
}

func (SyncRunJobArgs) Kind() string { return JobKindSyncRun }

type SyncTransferJobArgs struct {
	UserID          int64             `json:"userId" river:"unique"`
	RunID           string            `json:"runId"`
	Source          string            `json:"source"`
	SourcePath      string            `json:"sourcePath,omitempty"`
	DestinationPath string            `json:"destinationPath,omitempty"`
	Name            string            `json:"name"`
	Size            int64             `json:"size,omitempty"`
	MimeType        string            `json:"mimeType,omitempty"`
	Hash            string            `json:"hash,omitempty"`
	ModifiedAtUnix  int64             `json:"modifiedAtUnix,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Proxy           string            `json:"proxy,omitempty"`
	PartSize        int64             `json:"partSize,omitempty"`
}

func (SyncTransferJobArgs) Kind() string { return JobKindSyncTransfer }

type CleanOldEventsArgs struct {
	UserID    int64  `json:"userId" river:"unique"`
	Retention string `json:"retention" river:"unique"`
}

func (CleanOldEventsArgs) Kind() string { return JobKindCleanOldEvents }

type CleanStaleUploadsArgs struct {
	UserID    int64  `json:"userId" river:"unique"`
	Retention string `json:"retention" river:"unique"`
}

func (CleanStaleUploadsArgs) Kind() string { return JobKindCleanStaleUpload }

type CleanPendingFilesArgs struct {
	UserID int64 `json:"userId" river:"unique"`
}

func (CleanPendingFilesArgs) Kind() string { return JobKindCleanPendingFile }

type Executor interface {
	SyncRun(ctx context.Context, args SyncRunJobArgs, jobID int64) error
	SyncTransfer(ctx context.Context, args SyncTransferJobArgs) error
	CleanOldEventsForUser(ctx context.Context, args CleanOldEventsArgs) error
	CleanStaleUploadsForUser(ctx context.Context, args CleanStaleUploadsArgs) error
	CleanPendingFilesForUser(ctx context.Context, userID int64) error
}
