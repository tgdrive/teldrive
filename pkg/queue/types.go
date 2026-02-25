package queue

import "context"

const (
	JobKindFilesCopy    = "files.copy"
	JobKindFilesMove    = "files.move"
	JobKindFilesDelete  = "files.delete"
	JobKindFilesRestore = "files.restore"
	JobKindSyncRun      = "sync.run"
	JobKindSyncTransfer = "sync.transfer"
	JobKindSyncFinalize = "sync.finalize"

	JobKindCleanOldEvents   = "clean.old_events"
	JobKindCleanStaleUpload = "clean.stale_uploads"
	JobKindCleanPendingFile = "clean.pending_files"
)

type JobItem struct {
	ID              string `json:"id"`
	DestinationName string `json:"destinationName,omitempty"`
}

type FilesCopyJobArgs struct {
	UserID              int64     `json:"userId" river:"unique"`
	SessionID           string    `json:"sessionId" river:"unique"`
	DestinationParentID string    `json:"destinationParentId"`
	Items               []JobItem `json:"items"`
}

func (FilesCopyJobArgs) Kind() string { return JobKindFilesCopy }

type FilesMoveJobArgs struct {
	UserID              int64     `json:"userId" river:"unique"`
	SessionID           string    `json:"sessionId" river:"unique"`
	DestinationParentID string    `json:"destinationParentId"`
	Items               []JobItem `json:"items"`
}

func (FilesMoveJobArgs) Kind() string { return JobKindFilesMove }

type FilesDeleteJobArgs struct {
	UserID    int64     `json:"userId" river:"unique"`
	SessionID string    `json:"sessionId" river:"unique"`
	Items     []JobItem `json:"items"`
}

func (FilesDeleteJobArgs) Kind() string { return JobKindFilesDelete }

type FilesRestoreJobArgs struct {
	UserID    int64     `json:"userId" river:"unique"`
	SessionID string    `json:"sessionId" river:"unique"`
	Items     []JobItem `json:"items"`
}

func (FilesRestoreJobArgs) Kind() string { return JobKindFilesRestore }

type SyncRunJobArgs struct {
	UserID         int64             `json:"userId" river:"unique"`
	RunID          string            `json:"runId,omitempty"`
	Source         string            `json:"source"`
	SourceDir      string            `json:"sourceDir,omitempty"`
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
	PartSize    int64 `json:"partSize,omitempty"`
	Concurrency int   `json:"concurrency,omitempty"`
	Sync        bool  `json:"sync,omitempty"`
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

type SyncFinalizeJobArgs struct {
	UserID int64  `json:"userId" river:"unique"`
	RunID  string `json:"runId"`
}

func (SyncFinalizeJobArgs) Kind() string { return JobKindSyncFinalize }

type CleanOldEventsArgs struct{}

func (CleanOldEventsArgs) Kind() string { return JobKindCleanOldEvents }

type CleanOldEventsUserArgs struct {
	UserID    int64  `json:"userId" river:"unique"`
	Retention string `json:"retention" river:"unique"`
}

func (CleanOldEventsUserArgs) Kind() string { return JobKindCleanOldEvents + ".user" }

type CleanStaleUploadsArgs struct{}

func (CleanStaleUploadsArgs) Kind() string { return JobKindCleanStaleUpload }

type CleanStaleUploadsUserArgs struct {
	UserID    int64  `json:"userId" river:"unique"`
	Retention string `json:"retention" river:"unique"`
}

func (CleanStaleUploadsUserArgs) Kind() string { return JobKindCleanStaleUpload + ".user" }

type CleanPendingFilesArgs struct{}

func (CleanPendingFilesArgs) Kind() string { return JobKindCleanPendingFile }

type CleanPendingFilesUserArgs struct {
	UserID int64 `json:"userId" river:"unique"`
}

func (CleanPendingFilesUserArgs) Kind() string { return JobKindCleanPendingFile + ".user" }

type Executor interface {
	Restore(ctx context.Context, userID int64, item JobItem) error
	SyncRun(ctx context.Context, args SyncRunJobArgs, jobID int64) error
	SyncTransfer(ctx context.Context, args SyncTransferJobArgs) error
	SyncFinalize(ctx context.Context, args SyncFinalizeJobArgs) error

	CleanOldEventsForUser(ctx context.Context, args CleanOldEventsUserArgs) error
	UserIDs(ctx context.Context) ([]int64, error)
	CleanStaleUploadsForUser(ctx context.Context, args CleanStaleUploadsUserArgs) error
	CleanPendingFilesForUser(ctx context.Context, userID int64) error
}
