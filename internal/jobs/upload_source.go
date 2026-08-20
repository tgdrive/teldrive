package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"golang.org/x/sync/errgroup"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

const (
	skipExcluded     = "excluded"
	skipBelowMinSize = "below_min_size"
	skipAboveMaxSize = "above_max_size"
)

var errInvalidUploadSource = errors.New("invalid upload source")

const (
	UploadBatchKind    = "teldrive_upload_batch"
	UploadSourceKind   = "teldrive_upload_source"
	UploadQueue        = "uploads"
	uploadChunkBlock   = int64(16 * 1024 * 1024)
	minUploadChunk     = int64(64 * 1024 * 1024)
	maxUploadChunk     = int64(2000 * 1024 * 1024)
	defaultUploadChunk = int64(512 * 1024 * 1024)
)

type UploadSource struct {
	Type            string            `json:"type"`
	Path            string            `json:"path,omitempty"`
	URL             string            `json:"url,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	DestinationPath string            `json:"destination_path,omitempty"`
	Exclude         []string          `json:"exclude,omitempty"`
}

type UploadBatchArgs struct {
	BatchID         string            `json:"batch_id"`
	UserID          int64             `json:"user_id"`
	Destination     string            `json:"destination,omitempty"`
	ParentID        string            `json:"parent_id,omitempty"`
	Sources         []UploadSource    `json:"sources"`
	Headers         map[string]string `json:"headers,omitempty"`
	Exclude         []string          `json:"exclude,omitempty"`
	MinSize         string            `json:"min_size,omitempty"`
	MaxSize         string            `json:"max_size,omitempty"`
	PartConcurrency int               `json:"part_concurrency,omitempty"`
	ChunkSize       int64             `json:"chunk_size,omitempty"`
	Encryption      bool              `json:"encryption,omitempty"`
}

func (UploadBatchArgs) Kind() string { return UploadBatchKind }
func (UploadBatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: UploadQueue, MaxAttempts: 3}
}

type UploadFileSource struct {
	Type            string            `json:"type"`
	Location        string            `json:"location"`
	Headers         map[string]string `json:"headers,omitempty"`
	DestinationPath string            `json:"destination_path"`
	Size            int64             `json:"size"`
	ModTime         time.Time         `json:"mod_time"`
	HasModTime      bool              `json:"has_mod_time"`
	MIMEType        string            `json:"mime_type,omitempty"`
	Validator       string            `json:"validator,omitempty"`
}

type UploadSourceArgs struct {
	BatchID         string           `json:"batch_id" river:"unique"`
	SourceIndex     int              `json:"source_index" river:"unique"`
	UserID          int64            `json:"user_id"`
	ParentID        string           `json:"parent_id,omitempty"`
	Source          UploadFileSource `json:"source"`
	PartConcurrency int              `json:"part_concurrency"`
	ChunkSize       int64            `json:"chunk_size"`
	Encryption      bool             `json:"encryption,omitempty"`
}

func (UploadSourceArgs) Kind() string { return UploadSourceKind }
func (UploadSourceArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: UploadQueue, MaxAttempts: 5, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type UploadBatchWorker struct {
	river.WorkerDefaults[UploadBatchArgs]
	httpClient *http.Client
	catalog    *catalog.Service
}

func NewUploadBatchWorker(httpClient *http.Client, catalogService *catalog.Service) *UploadBatchWorker {
	if httpClient == nil {
		httpClient = NewUploadHTTPClient()
	}
	return &UploadBatchWorker{httpClient: httpClient, catalog: catalogService}
}

func (w *UploadBatchWorker) Timeout(*river.Job[UploadBatchArgs]) time.Duration { return time.Hour }

func (w *UploadBatchWorker) Work(ctx context.Context, job *river.Job[UploadBatchArgs]) error {
	if job.Args.UserID <= 0 || len(job.Args.Sources) == 0 {
		return errInvalidUploadSource
	}
	parentID, err := w.resolveDestination(ctx, job.Args)
	if err != nil {
		return err
	}
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return fmt.Errorf("get River client: %w", err)
	}
	filter, err := newUploadFilter(job.Args.Exclude, job.Args.MinSize, job.Args.MaxSize)
	if err != nil {
		return err
	}
	partConcurrency := job.Args.PartConcurrency
	if partConcurrency <= 0 {
		partConcurrency = 4
	}
	if partConcurrency > 16 {
		return fmt.Errorf("%w: part concurrency exceeds 16", errInvalidUploadSource)
	}
	chunkSize, err := normalizeUploadChunkSize(job.Args.ChunkSize)
	if err != nil {
		return err
	}
	batchID := job.Args.BatchID
	if _, err := uuid.Parse(batchID); err != nil {
		return fmt.Errorf("%w: invalid batch id", errInvalidUploadSource)
	}
	index := 0
	for _, source := range job.Args.Sources {
		files, err := w.expand(ctx, source, job.Args.Headers, filter)
		if err != nil {
			return err
		}
		for _, file := range files {
			args := UploadSourceArgs{BatchID: batchID, SourceIndex: index, UserID: job.Args.UserID, ParentID: parentID, Source: file, PartConcurrency: partConcurrency, ChunkSize: chunkSize, Encryption: job.Args.Encryption}
			if _, err := client.Insert(ctx, args, nil); err != nil {
				return fmt.Errorf("insert upload source job: %w", err)
			}
			index++
		}
	}
	return nil
}

func (w *UploadBatchWorker) resolveDestination(ctx context.Context, args UploadBatchArgs) (string, error) {
	// ParentID is retained for jobs queued before destination paths were resolved by the worker.
	if args.Destination == "" {
		if args.ParentID == "" {
			return "", nil
		}
		if _, err := uuid.Parse(args.ParentID); err != nil {
			return "", fmt.Errorf("%w: invalid parent id", errInvalidUploadSource)
		}
		return args.ParentID, nil
	}
	if w.catalog == nil {
		return "", errors.New("upload catalog is not configured")
	}
	destination := strings.TrimSpace(args.Destination)
	if id, err := uuid.Parse(destination); err == nil {
		file, err := w.catalog.Get(ctx, args.UserID, id)
		if err != nil {
			return "", fmt.Errorf("resolve upload destination: %w", err)
		}
		if file.Kind != sqlcgen.FileKindFolder || file.Status != sqlcgen.FileStatusActive {
			return "", fmt.Errorf("%w: destination is not an active folder", errInvalidUploadSource)
		}
		return id.String(), nil
	}
	if !strings.HasPrefix(destination, "/") {
		return "", fmt.Errorf("%w: destination path must be absolute", errInvalidUploadSource)
	}
	id, err := w.catalog.EnsureFolderPath(ctx, args.UserID, nil, destination)
	if err != nil {
		return "", fmt.Errorf("create upload destination: %w", err)
	}
	if id == nil {
		return "", nil
	}
	return id.String(), nil
}

func (w *UploadBatchWorker) expand(ctx context.Context, source UploadSource, defaults map[string]string, batchFilter uploadFilter) ([]UploadFileSource, error) {
	switch source.Type {
	case "local":
		return expandLocalSource(source, batchFilter)
	case "http":
		file, err := inspectHTTPSource(ctx, w.httpClient, source, defaults)
		if err != nil {
			return nil, err
		}
		if batchFilter.skipReason(file.DestinationPath, file.Size) != "" {
			return nil, nil
		}
		return []UploadFileSource{file}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported source type %q", errInvalidUploadSource, source.Type)
	}
}

type UploadSourceWorker struct {
	river.WorkerDefaults[UploadSourceArgs]
	pool             *pgxpool.Pool
	queries          *sqlcgen.Queries
	catalog          *catalog.Service
	uploads          *uploads.Service
	pipeline         *transfer.Pipeline
	httpClient       *http.Client
	activeKeyVersion int32
}

func NewUploadSourceWorker(pool *pgxpool.Pool, catalogService *catalog.Service, uploadService *uploads.Service, pipeline *transfer.Pipeline, httpClient *http.Client, activeKeyVersion int32) *UploadSourceWorker {
	if httpClient == nil {
		httpClient = NewUploadHTTPClient()
	}
	return &UploadSourceWorker{pool: pool, queries: sqlcgen.New(pool), catalog: catalogService, uploads: uploadService, pipeline: pipeline, httpClient: httpClient, activeKeyVersion: activeKeyVersion}
}

func (w *UploadSourceWorker) Timeout(*river.Job[UploadSourceArgs]) time.Duration {
	return 24 * time.Hour
}

func (w *UploadSourceWorker) Work(ctx context.Context, job *river.Job[UploadSourceArgs]) error {
	if w == nil || w.pool == nil || w.catalog == nil || w.uploads == nil || w.pipeline == nil || job.Args.UserID <= 0 {
		return ErrRuntimeNotConfigured
	}
	if job.Args.PartConcurrency <= 0 {
		job.Args.PartConcurrency = 4
	}
	if job.Args.PartConcurrency > 16 {
		return fmt.Errorf("%w: part concurrency exceeds 16", errInvalidUploadSource)
	}
	chunkSize, err := normalizeUploadChunkSize(job.Args.ChunkSize)
	if err != nil {
		return err
	}
	client, _ := river.ClientFromContextSafely[pgx.Tx](ctx)
	parentID, err := parseOptionalUUID(job.Args.ParentID)
	if err != nil {
		return err
	}
	directory, name := path.Split(path.Clean(strings.TrimPrefix(job.Args.Source.DestinationPath, "/")))
	if name == "" || name == "." {
		return errInvalidUploadSource
	}
	parentID, err = w.ensureFolders(ctx, job.Args.UserID, parentID, directory)
	if err != nil {
		return err
	}
	normalizedName, err := normalizedUploadName(name)
	if err != nil {
		return err
	}
	job.Args.Source.MIMEType, err = w.detectSourceMIME(ctx, job.Args.Source)
	if err != nil {
		return err
	}
	existing, err := w.queries.ResolveActiveChild(ctx, sqlcgen.ResolveActiveChildParams{UserID: job.Args.UserID, ParentID: dbtypes.OptionalUUID(parentID), NormalizedName: normalizedName})
	if err == nil {
		if existing.Kind != sqlcgen.FileKindFile {
			return uploads.ErrNameConflict
		}
		if existing.Size.Valid && existing.Size.Int64 == job.Args.Source.Size && job.Args.Source.HasModTime && modTimesEqual(existing.ModTime.Time, job.Args.Source.ModTime) && existing.MimeType.Valid && existing.MimeType.String == job.Args.Source.MIMEType {
			output := UploadSourceOutput{Path: job.Args.Source.DestinationPath, SourceType: job.Args.Source.Type, Stage: "skipped", Reason: "destination_matches", Progress: 100, TotalBytes: job.Args.Source.Size, UpdatedAt: time.Now().UTC()}
			if client != nil && job.JobRow != nil {
				return river.RecordOutput(ctx, output)
			}
			return nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("resolve upload destination: %w", err)
	}

	session, storedParts, err := w.findResumableUpload(ctx, job.Args.UserID, parentID, name, job.Args.Source, job.Args.Encryption)
	if err != nil {
		return err
	}
	if session == nil {
		input := uploads.CreateInput{UserID: job.Args.UserID, ParentID: parentID, Name: name, ExpectedSize: job.Args.Source.Size, MIMEType: optionalString(job.Args.Source.MIMEType), ModTime: job.Args.Source.ModTime, ConflictPolicy: sqlcgen.NameConflictPolicyReplace, Encryption: job.Args.Encryption, PartSize: chunkSize}
		if input.Encryption {
			if w.activeKeyVersion <= 0 {
				return transfer.ErrEncryptionKey
			}
			input.EncryptionKeyVersion = &w.activeKeyVersion
		}
		session, err = w.uploads.Create(ctx, input)
	}
	if err != nil {
		return err
	}
	if session.State == sqlcgen.UploadStateCompleted {
		return nil
	}
	uploadID, ok := dbtypes.GoogleUUID(session.ID)
	if !ok {
		return errInvalidUploadSource
	}
	partCount := int((session.ExpectedSize + session.PartSize - 1) / session.PartSize)
	var jobID int64
	if job.JobRow != nil {
		jobID = job.ID
	}
	tracker := newUploadProgressTracker(client, jobID, uploadID, job.Args.Source, session.PartSize, job.Args.PartConcurrency, partCount, storedParts)
	if err := tracker.update(ctx, "uploading", false); err != nil {
		return err
	}
	if partCount == 0 {
		file, completeErr := w.uploads.Complete(ctx, job.Args.UserID, uploadID)
		if completeErr != nil {
			_ = tracker.finish(ctx, "failed", nil)
			return completeErr
		}
		fileID, _ := dbtypes.GoogleUUID(file.ID)
		return tracker.finish(ctx, "completed", &fileID)
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(job.Args.PartConcurrency)
	progressCtx, stopProgress := context.WithCancel(ctx)
	var progressWG sync.WaitGroup
	progressWG.Add(1)
	go func() {
		defer progressWG.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-progressCtx.Done():
				return
			case <-ticker.C:
				_ = tracker.update(progressCtx, "uploading", false)
			}
		}
	}()
	for part := 0; part < partCount; part++ {
		part := part
		if _, ok := storedParts[int32(part+1)]; ok {
			continue
		}
		group.Go(func() error {
			offset := int64(part) * session.PartSize
			size := min(session.PartSize, session.ExpectedSize-offset)
			reader, closeReader, err := w.openPart(groupCtx, job.Args.Source, offset, size)
			if err != nil {
				return err
			}
			defer closeReader()
			reader = io.TeeReader(reader, tracker)
			_, err = w.pipeline.UploadPart(groupCtx, transfer.UploadPartRequest{UserID: job.Args.UserID, UploadID: uploadID, PartNo: int32(part + 1), PlainSize: size, Body: reader})
			if err == nil {
				tracker.partCompleted(groupCtx, size)
			}
			return err
		})
	}
	groupErr := group.Wait()
	stopProgress()
	progressWG.Wait()
	if groupErr != nil {
		_ = tracker.finish(ctx, "failed", nil)
		return groupErr
	}
	if job.Args.Source.Type == "local" {
		info, statErr := os.Stat(job.Args.Source.Location)
		if statErr != nil || info.Size() != job.Args.Source.Size || !info.ModTime().Equal(job.Args.Source.ModTime) {
			_ = tracker.finish(ctx, "failed", nil)
			return fmt.Errorf("%w: local source changed during upload", errInvalidUploadSource)
		}
	}
	if err := tracker.update(ctx, "completing", false); err != nil {
		return err
	}
	file, err := w.uploads.Complete(ctx, job.Args.UserID, uploadID)
	if err != nil {
		_ = tracker.finish(ctx, "failed", nil)
		return err
	}
	fileID, _ := dbtypes.GoogleUUID(file.ID)
	return tracker.finish(ctx, "completed", &fileID)
}

type UploadSourceOutput struct {
	UploadID            string    `json:"uploadId,omitempty"`
	FileID              string    `json:"fileId,omitempty"`
	Path                string    `json:"path"`
	SourceType          string    `json:"sourceType"`
	Stage               string    `json:"stage"`
	Reason              string    `json:"reason,omitempty"`
	Progress            float64   `json:"progress"`
	UploadedBytes       int64     `json:"uploadedBytes"`
	StoredBytes         int64     `json:"storedBytes"`
	TotalBytes          int64     `json:"totalBytes"`
	SpeedBytesPerSecond int64     `json:"speedBytesPerSecond"`
	CompletedParts      int32     `json:"completedParts"`
	TotalParts          int       `json:"totalParts"`
	ChunkSize           int64     `json:"chunkSize"`
	PartConcurrency     int       `json:"partConcurrency"`
	StartedAt           time.Time `json:"startedAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type uploadProgressTracker struct {
	client      *river.Client[pgx.Tx]
	jobID       int64
	output      UploadSourceOutput
	started     time.Time
	transferred atomic.Int64
	attempted   atomic.Int64
	stored      atomic.Int64
	completed   atomic.Int32
	mu          sync.Mutex
}

func newUploadProgressTracker(client *river.Client[pgx.Tx], jobID int64, uploadID uuid.UUID, source UploadFileSource, chunkSize int64, concurrency, totalParts int, stored map[int32]int64) *uploadProgressTracker {
	now := time.Now().UTC()
	tracker := &uploadProgressTracker{client: client, jobID: jobID, started: now, output: UploadSourceOutput{UploadID: uploadID.String(), Path: source.DestinationPath, SourceType: source.Type, TotalBytes: source.Size, TotalParts: totalParts, ChunkSize: chunkSize, PartConcurrency: concurrency, StartedAt: now}}
	for _, size := range stored {
		tracker.stored.Add(size)
		tracker.completed.Add(1)
	}
	return tracker
}

func (t *uploadProgressTracker) Write(p []byte) (int, error) {
	t.transferred.Add(int64(len(p)))
	t.attempted.Add(int64(len(p)))
	return len(p), nil
}

func (t *uploadProgressTracker) partCompleted(ctx context.Context, size int64) {
	t.transferred.Add(-size)
	t.stored.Add(size)
	t.completed.Add(1)
	_ = t.update(ctx, "uploading", false)
}

func (t *uploadProgressTracker) update(ctx context.Context, stage string, terminal bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now().UTC()
	stored, transferred := t.stored.Load(), t.transferred.Load()
	uploaded := min(t.output.TotalBytes, stored+transferred)
	t.output.Stage, t.output.StoredBytes, t.output.UploadedBytes = stage, stored, uploaded
	t.output.CompletedParts, t.output.UpdatedAt = t.completed.Load(), now
	if t.output.TotalBytes == 0 || stage == "completed" || stage == "skipped" {
		t.output.Progress = 100
	} else {
		t.output.Progress = float64(uploaded) * 100 / float64(t.output.TotalBytes)
	}
	if elapsed := now.Sub(t.started).Seconds(); elapsed > 0 {
		t.output.SpeedBytesPerSecond = int64(float64(t.attempted.Load()) / elapsed)
	}
	if terminal {
		if t.client == nil {
			return nil
		}
		return river.RecordOutput(ctx, t.output)
	}
	if t.client == nil || t.jobID <= 0 {
		return nil
	}
	_, err := t.client.JobUpdate(ctx, t.jobID, &river.JobUpdateParams{Output: t.output})
	return err
}

func normalizeUploadChunkSize(value int64) (int64, error) {
	if value <= 0 {
		return defaultUploadChunk, nil
	}
	if value < minUploadChunk || value > maxUploadChunk {
		return 0, fmt.Errorf("%w: chunk size must be between 64 MiB and 2000 MiB", errInvalidUploadSource)
	}
	aligned := ((value + uploadChunkBlock/2) / uploadChunkBlock) * uploadChunkBlock
	return min(aligned, maxUploadChunk), nil
}

func (w *UploadSourceWorker) findResumableUpload(ctx context.Context, userID int64, parentID *uuid.UUID, name string, source UploadFileSource, encryption bool) (*sqlcgen.UploadSession, map[int32]int64, error) {
	state := sqlcgen.UploadStateOpen
	var afterCreatedAt *time.Time
	var afterID *uuid.UUID
	for {
		sessions, err := w.uploads.List(ctx, uploads.ListInput{UserID: userID, State: &state, AfterCreatedAt: afterCreatedAt, AfterID: afterID, Limit: 200})
		if err != nil {
			return nil, nil, err
		}
		for _, session := range sessions {
			if !uploadSessionMatches(session, parentID, name, source, encryption) {
				continue
			}
			uploadID, ok := dbtypes.GoogleUUID(session.ID)
			if !ok {
				continue
			}
			stored, compatible, err := w.storedUploadParts(ctx, userID, uploadID, session.ExpectedSize, session.PartSize)
			if err != nil {
				return nil, nil, err
			}
			if compatible {
				return session, stored, nil
			}
		}
		if len(sessions) < 200 {
			return nil, map[int32]int64{}, nil
		}
		last := sessions[len(sessions)-1]
		id, ok := dbtypes.GoogleUUID(last.ID)
		if !ok || !last.CreatedAt.Valid {
			return nil, nil, errInvalidUploadSource
		}
		createdAt := last.CreatedAt.Time
		afterCreatedAt, afterID = &createdAt, &id
	}
}

func uploadSessionMatches(session *sqlcgen.UploadSession, parentID *uuid.UUID, name string, source UploadFileSource, encryption bool) bool {
	if session == nil || session.Name != name || session.ExpectedSize != source.Size || session.Encryption != encryption || session.ConflictPolicy != sqlcgen.NameConflictPolicyReplace || !session.ExpiresAt.Valid || !session.ExpiresAt.Time.After(time.Now()) {
		return false
	}
	sessionParent, hasParent := dbtypes.GoogleUUID(session.ParentID)
	if (parentID == nil) != !hasParent || parentID != nil && sessionParent != *parentID {
		return false
	}
	if session.MimeType.Valid != (source.MIMEType != "") || session.MimeType.Valid && session.MimeType.String != source.MIMEType {
		return false
	}
	return !source.HasModTime || session.ModTime.Valid && modTimesEqual(session.ModTime.Time, source.ModTime)
}

func (w *UploadSourceWorker) storedUploadParts(ctx context.Context, userID int64, uploadID uuid.UUID, totalSize, partSize int64) (map[int32]int64, bool, error) {
	stored := make(map[int32]int64)
	var after *int32
	for {
		parts, err := w.uploads.ListParts(ctx, uploads.ListPartsInput{UserID: userID, UploadID: uploadID, AfterPartNo: after, Limit: 200})
		if err != nil {
			return nil, false, err
		}
		for _, part := range parts {
			if part.State != sqlcgen.UploadPartStateStored {
				continue
			}
			offset := int64(part.PartNo-1) * partSize
			if offset < 0 || offset >= totalSize || part.PlainSize != min(partSize, totalSize-offset) {
				return nil, false, nil
			}
			stored[part.PartNo] = part.PlainSize
		}
		if len(parts) < 200 {
			return stored, true, nil
		}
		value := parts[len(parts)-1].PartNo
		after = &value
	}
}

func (t *uploadProgressTracker) finish(ctx context.Context, stage string, fileID *uuid.UUID) error {
	if fileID != nil {
		t.output.FileID = fileID.String()
	}
	if stage == "completed" {
		t.stored.Store(t.output.TotalBytes)
		t.transferred.Store(0)
		t.completed.Store(int32(t.output.TotalParts))
	}
	return t.update(ctx, stage, true)
}

func (w *UploadSourceWorker) ensureFolders(ctx context.Context, userID int64, parentID *uuid.UUID, directory string) (*uuid.UUID, error) {
	for _, name := range strings.Split(strings.Trim(directory, "/"), "/") {
		if name == "" || name == "." {
			continue
		}
		normalized, err := normalizedUploadName(name)
		if err != nil {
			return nil, err
		}
		id, err := w.queries.ResolveActiveChildFolder(ctx, sqlcgen.ResolveActiveChildFolderParams{UserID: userID, ParentID: dbtypes.OptionalUUID(parentID), NormalizedName: normalized})
		if errors.Is(err, pgx.ErrNoRows) {
			folder, createErr := w.catalog.CreateFolder(ctx, catalog.CreateFolderInput{UserID: userID, ParentID: parentID, Name: name})
			if errors.Is(createErr, catalog.ErrConflict) {
				id, err = w.queries.ResolveActiveChildFolder(ctx, sqlcgen.ResolveActiveChildFolderParams{UserID: userID, ParentID: dbtypes.OptionalUUID(parentID), NormalizedName: normalized})
			} else if createErr != nil {
				return nil, createErr
			} else {
				id = folder.ID
				err = nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("resolve destination folder: %w", err)
		}
		value, ok := dbtypes.GoogleUUID(id)
		if !ok {
			return nil, errInvalidUploadSource
		}
		parentID = &value
	}
	return parentID, nil
}

func (w *UploadSourceWorker) detectSourceMIME(ctx context.Context, source UploadFileSource) (string, error) {
	declared := strings.TrimSpace(source.MIMEType)
	if parsed, _, err := mime.ParseMediaType(declared); err == nil {
		declared = parsed
	}
	if declared != "" && declared != "application/octet-stream" {
		return declared, nil
	}

	var reader io.ReadCloser
	switch source.Type {
	case "local":
		file, err := os.Open(source.Location)
		if err != nil {
			return "", fmt.Errorf("open source for MIME detection: %w", err)
		}
		reader = file
	case "http":
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.Location, nil)
		if err != nil {
			return "", fmt.Errorf("create MIME detection request: %w", err)
		}
		applyUploadHeaders(request.Header, source.Headers)
		request.Header.Set("Range", "bytes=0-511")
		response, err := w.httpClient.Do(request)
		if err != nil {
			return "", errors.New("read HTTP source for MIME detection failed")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			_ = response.Body.Close()
			return "", fmt.Errorf("read HTTP source for MIME detection: %s", response.Status)
		}
		reader = response.Body
	default:
		return "", fmt.Errorf("%w: unsupported source type %q", errInvalidUploadSource, source.Type)
	}
	defer reader.Close()

	buffer := make([]byte, 512)
	read, err := io.ReadFull(reader, buffer)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", fmt.Errorf("read source for MIME detection: %w", err)
	}
	if read > 0 {
		detected := http.DetectContentType(buffer[:read])
		if detected != "application/octet-stream" {
			if parsed, _, err := mime.ParseMediaType(detected); err == nil {
				return parsed, nil
			}
			return detected, nil
		}
	}
	if inferred := mime.TypeByExtension(path.Ext(source.DestinationPath)); inferred != "" {
		if parsed, _, err := mime.ParseMediaType(inferred); err == nil {
			return parsed, nil
		}
		return inferred, nil
	}
	return "application/octet-stream", nil
}

func (w *UploadSourceWorker) openPart(ctx context.Context, source UploadFileSource, offset, size int64) (io.Reader, func(), error) {
	if source.Type == "local" {
		file, err := os.Open(source.Location)
		if err != nil {
			return nil, func() {}, fmt.Errorf("open local upload source: %w", err)
		}
		return io.NewSectionReader(file, offset, size), func() { _ = file.Close() }, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.Location, nil)
	if err != nil {
		return nil, func() {}, err
	}
	applyUploadHeaders(request.Header, source.Headers)
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+size-1))
	if source.Validator != "" {
		request.Header.Set("If-Range", source.Validator)
	}
	response, err := w.httpClient.Do(request)
	if err != nil {
		return nil, func() {}, errors.New("open HTTP upload part request failed")
	}
	if response.StatusCode != http.StatusPartialContent && !(offset == 0 && size == source.Size && response.StatusCode == http.StatusOK) {
		_ = response.Body.Close()
		return nil, func() {}, fmt.Errorf("HTTP upload source does not support byte ranges: %s", response.Status)
	}
	if response.StatusCode == http.StatusPartialContent && !validContentRange(response.Header.Get("Content-Range"), offset, size, source.Size) {
		_ = response.Body.Close()
		return nil, func() {}, fmt.Errorf("HTTP upload source returned an invalid content range")
	}
	return response.Body, func() { _ = response.Body.Close() }, nil
}

type uploadFilter struct {
	exclude []*regexp.Regexp
	minSize *int64
	maxSize *int64
}

func newUploadFilter(patterns []string, minSize, maxSize string) (uploadFilter, error) {
	filter := uploadFilter{}
	for _, pattern := range patterns {
		compiled, err := compileUploadGlob(pattern)
		if err != nil {
			return uploadFilter{}, err
		}
		filter.exclude = append(filter.exclude, compiled)
	}
	for _, item := range []struct {
		value  string
		target **int64
	}{{minSize, &filter.minSize}, {maxSize, &filter.maxSize}} {
		value, target := item.value, item.target
		if strings.TrimSpace(value) == "" {
			continue
		}
		size, err := parseUploadSize(value)
		if err != nil {
			return uploadFilter{}, err
		}
		*target = &size
	}
	if filter.minSize != nil && filter.maxSize != nil && *filter.minSize > *filter.maxSize {
		return uploadFilter{}, fmt.Errorf("%w: minimum size exceeds maximum size", errInvalidUploadSource)
	}
	return filter, nil
}

func (f uploadFilter) skipReason(name string, size int64) string {
	normalized := strings.TrimPrefix(path.Clean(strings.ReplaceAll(name, "\\", "/")), "./")
	for _, pattern := range f.exclude {
		if pattern.MatchString(normalized) {
			return skipExcluded
		}
	}
	if f.minSize != nil && size < *f.minSize {
		return skipBelowMinSize
	}
	if f.maxSize != nil && size > *f.maxSize {
		return skipAboveMaxSize
	}
	return ""
}

func parseUploadSize(value string) (int64, error) {
	normalized := strings.TrimSpace(value)
	match := regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*([kmgtpe]?i?b?)?$`).FindStringSubmatch(normalized)
	if match == nil {
		return 0, fmt.Errorf("%w: invalid size %q", errInvalidUploadSource, value)
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("%w: invalid size %q", errInvalidUploadSource, value)
	}
	unit := strings.ToUpper(match[2])
	unit = strings.TrimSuffix(unit, "B")
	base := float64(1000)
	if strings.HasSuffix(unit, "I") {
		base = 1024
		unit = strings.TrimSuffix(unit, "I")
	}
	power := strings.Index(" KMGTPE", unit)
	if power < 0 {
		return 0, fmt.Errorf("%w: invalid size %q", errInvalidUploadSource, value)
	}
	result := number * math.Pow(base, float64(power))
	if result > math.MaxInt64 || math.Trunc(result) != result {
		return 0, fmt.Errorf("%w: invalid size %q", errInvalidUploadSource, value)
	}
	return int64(result), nil
}

func compileUploadGlob(pattern string) (*regexp.Regexp, error) {
	pattern = strings.TrimPrefix(path.Clean(strings.ReplaceAll(strings.TrimSpace(pattern), "\\", "/")), "./")
	if pattern == "" || pattern == "." {
		return nil, fmt.Errorf("%w: empty exclusion", errInvalidUploadSource)
	}
	var expression strings.Builder
	expression.WriteString("^(?:.*/)?")
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				if index+2 < len(pattern) && pattern[index+2] == '/' {
					expression.WriteString("(?:.*/)?")
					index += 2
				} else {
					expression.WriteString(".*")
					index++
				}
			} else {
				expression.WriteString("[^/]*")
			}
		case '?':
			expression.WriteString("[^/]")
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[index])))
		}
	}
	expression.WriteString("$")
	compiled, err := regexp.Compile(expression.String())
	if err != nil {
		return nil, fmt.Errorf("%w: exclusion %q: %v", errInvalidUploadSource, pattern, err)
	}
	return compiled, nil
}

func expandLocalSource(source UploadSource, batchFilter uploadFilter) ([]UploadFileSource, error) {
	location := filepath.Clean(strings.TrimSpace(source.Path))
	if !filepath.IsAbs(location) {
		return nil, fmt.Errorf("%w: local path must be absolute", errInvalidUploadSource)
	}
	info, err := os.Lstat(location)
	if err != nil {
		return nil, fmt.Errorf("inspect local upload source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: symlink sources are not supported", errInvalidUploadSource)
	}
	filter, err := newUploadFilter(source.Exclude, "", "")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: local source is not a regular file", errInvalidUploadSource)
		}
		name := source.DestinationPath
		if name == "" {
			name = info.Name()
		}
		destination, err := validateDestinationPath(name)
		if err != nil {
			return nil, err
		}
		if uploadSkipReason(batchFilter, filter, info.Name(), info.Size()) != "" {
			return nil, nil
		}
		return []UploadFileSource{{Type: "local", Location: location, DestinationPath: destination, Size: info.Size(), ModTime: info.ModTime().UTC(), HasModTime: true, MIMEType: mime.TypeByExtension(filepath.Ext(info.Name()))}}, nil
	}
	baseDestination, err := validateDestinationPath(source.DestinationPath)
	if err != nil {
		return nil, err
	}
	files := make([]UploadFileSource, 0)
	err = filepath.WalkDir(location, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(location, filePath)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if filter.skipReason(relative, 0) == skipExcluded || batchFilter.skipReason(relative, 0) == skipExcluded {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || uploadSkipReason(batchFilter, filter, relative, info.Size()) != "" {
			return nil
		}
		files = append(files, UploadFileSource{Type: "local", Location: filePath, DestinationPath: cleanDestinationPath(path.Join(baseDestination, relative)), Size: info.Size(), ModTime: info.ModTime().UTC(), HasModTime: true, MIMEType: mime.TypeByExtension(filepath.Ext(info.Name()))})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk local upload source: %w", err)
	}
	return files, nil
}

func inspectHTTPSource(ctx context.Context, client *http.Client, source UploadSource, defaults map[string]string) (UploadFileSource, error) {
	if client == nil {
		client = http.DefaultClient
	}
	parsed, err := url.Parse(strings.TrimSpace(source.URL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return UploadFileSource{}, fmt.Errorf("%w: invalid HTTP URL", errInvalidUploadSource)
	}
	headers := mergeUploadHeaders(defaults, source.Headers)
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, parsed.String(), nil)
	if err != nil {
		return UploadFileSource{}, err
	}
	applyUploadHeaders(request.Header, headers)
	response, err := client.Do(request)
	if err != nil {
		return UploadFileSource{}, errors.New("inspect HTTP upload source request failed")
	}
	_ = response.Body.Close()
	if response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented {
		request, err = http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
		if err != nil {
			return UploadFileSource{}, err
		}
		applyUploadHeaders(request.Header, headers)
		request.Header.Set("Range", "bytes=0-0")
		response, err = client.Do(request)
		if err != nil {
			return UploadFileSource{}, errors.New("inspect HTTP upload source request failed")
		}
		_ = response.Body.Close()
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return UploadFileSource{}, fmt.Errorf("inspect HTTP upload source: %s", response.Status)
	}
	size := response.ContentLength
	if contentRange := response.Header.Get("Content-Range"); contentRange != "" {
		if slash := strings.LastIndex(contentRange, "/"); slash >= 0 {
			size, _ = strconv.ParseInt(contentRange[slash+1:], 10, 64)
		}
	}
	if size < 0 {
		return UploadFileSource{}, fmt.Errorf("%w: HTTP source has no content length", errInvalidUploadSource)
	}
	name := source.DestinationPath
	if name == "" {
		if _, params, parseErr := mime.ParseMediaType(response.Header.Get("Content-Disposition")); parseErr == nil {
			name = params["filename"]
		}
	}
	if name == "" {
		name = path.Base(parsed.Path)
	}
	if name == "" || name == "." || name == "/" {
		return UploadFileSource{}, fmt.Errorf("%w: HTTP destination name is required", errInvalidUploadSource)
	}
	modTime, timeErr := http.ParseTime(response.Header.Get("Last-Modified"))
	validator := strings.TrimSpace(response.Header.Get("ETag"))
	if validator == "" && timeErr == nil {
		validator = modTime.Format(http.TimeFormat)
	}
	destination, err := validateDestinationPath(name)
	if err != nil {
		return UploadFileSource{}, err
	}
	return UploadFileSource{Type: "http", Location: parsed.String(), Headers: headers, DestinationPath: destination, Size: size, ModTime: modTime.UTC(), HasModTime: timeErr == nil, MIMEType: strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]), Validator: validator}, nil
}

func mergeUploadHeaders(defaults, overrides map[string]string) map[string]string {
	result := make(map[string]string, len(defaults)+len(overrides))
	for key, value := range defaults {
		result[http.CanonicalHeaderKey(key)] = value
	}
	for key, value := range overrides {
		result[http.CanonicalHeaderKey(key)] = value
	}
	return result
}

func applyUploadHeaders(target http.Header, values map[string]string) {
	for key, value := range values {
		switch http.CanonicalHeaderKey(key) {
		case "Host", "Content-Length", "Range", "Connection", "Transfer-Encoding", "Proxy-Connection":
			continue
		}
		target.Set(key, value)
	}
}

func cleanDestinationPath(value string) string {
	cleaned := path.Clean(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")))
	if cleaned == "." || cleaned == "/" {
		return ""
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

func validateDestinationPath(value string) (string, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if normalized == "" {
		return "", nil
	}
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("%w: destination path must be relative", errInvalidUploadSource)
	}
	cleaned := cleanDestinationPath(normalized)
	if cleaned == "" {
		return "", fmt.Errorf("%w: invalid destination path", errInvalidUploadSource)
	}
	return cleaned, nil
}

func normalizedUploadName(value string) (string, error) {
	_, normalized, err := catalog.NormalizeName(value)
	return normalized, err
}

func parseOptionalUUID(value string) (*uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid parent id", errInvalidUploadSource)
	}
	return &parsed, nil
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func modTimesEqual(left, right time.Time) bool {
	difference := left.Sub(right)
	return difference >= -time.Second && difference <= time.Second
}

func uploadSkipReason(batch, source uploadFilter, name string, size int64) string {
	if reason := batch.skipReason(name, size); reason != "" {
		return reason
	}
	return source.skipReason(name, size)
}

func validContentRange(value string, offset, size, total int64) bool {
	var start, end, reportedTotal int64
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "bytes %d-%d/%d", &start, &end, &reportedTotal); err != nil {
		return false
	}
	return start == offset && end == offset+size-1 && reportedTotal == total
}

func NewUploadHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			if !safeUploadAddress(address) {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		}
		return nil, fmt.Errorf("%w: HTTP target has no public address", errInvalidUploadSource)
	}
	client := &http.Client{Transport: transport}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "http" && request.URL.Scheme != "https" {
			return fmt.Errorf("%w: unsupported redirect scheme", errInvalidUploadSource)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if len(via) > 0 && !strings.EqualFold(request.URL.Host, via[len(via)-1].URL.Host) {
			request.Header.Del("Authorization")
			request.Header.Del("Cookie")
			request.Header.Del("Proxy-Authorization")
		}
		return nil
	}
	return client
}

func safeUploadAddress(address netip.Addr) bool {
	return address.IsValid() && !address.IsUnspecified() && !address.IsLoopback() && !address.IsPrivate() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast() && !address.IsMulticast()
}
