package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/pkg/queue"
	"github.com/tgdrive/teldrive/pkg/remotes"
	"github.com/tgdrive/teldrive/pkg/remotes/local"
	"github.com/tgdrive/teldrive/pkg/remotes/rclone"
	"github.com/tgdrive/teldrive/pkg/remotes/sftp"
	"github.com/tgdrive/teldrive/pkg/remotes/webdav"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"go.uber.org/zap"
)

type sourceFile struct {
	relPath   string
	sourceURI string
	size      int64
	name      string
	mimeType  string
	hash      string
	modified  time.Time
	fullPath  string
}

const syncTransferTaskPrefix = "transfer_"

type syncRunPlan struct {
	planned      int
	queued       int
	skipped      int
	deleted      int
	bytesPlanned int64
	depNames     []string
}

type syncWorkflowStats struct {
	total     int
	completed int
	failed    int
	files     []map[string]any
}

func (e *jobExecutor) SyncRun(ctx context.Context, args queue.SyncRunJobArgs, jobID int64) error {
	if args.UserID == 0 {
		return fmt.Errorf("missing user id")
	}
	if strings.TrimSpace(args.Source) == "" {
		return fmt.Errorf("missing source")
	}

	workingCtx, err := e.workingContext(ctx, args.UserID)
	if err != nil {
		return err
	}

	destinationRoot, err := e.resolveDestinationPath(workingCtx, args.UserID, args.DestinationDir)
	if err != nil {
		return err
	}

	files, err := e.listSourceFiles(workingCtx, args)
	if err != nil {
		return err
	}

	client := river.ClientFromContext[pgx.Tx](workingCtx)
	if client == nil {
		return fmt.Errorf("river client not available in context")
	}

	runID := strings.TrimSpace(args.RunID)
	if runID == "" {
		runID = fmt.Sprintf("sync_run_%d", jobID)
	}
	workflow := client.NewWorkflow(&river.WorkflowOpts{ID: runID, Name: "sync"})

	all, err := workflow.LoadAll(workingCtx, nil)
	if err != nil {
		return err
	}
	if all.Count() == 0 {
		plan, err := e.initializeSyncWorkflow(workingCtx, workflow, client, args, runID, destinationRoot, files)
		if err != nil {
			return err
		}
		if err := recordSyncRunPlanOutput(workingCtx, runID, args.Source, destinationRoot, plan); err != nil {
			return err
		}
	}

	fresh, err := workflow.LoadAll(workingCtx, nil)
	if err != nil {
		return err
	}
	stats, err := summarizeSyncWorkflow(fresh)
	if err != nil {
		return err
	}
	if stats.failed > 0 {
		return fmt.Errorf("sync workflow failed for %d transfer tasks", stats.failed)
	}
	if stats.total > 0 && stats.completed < stats.total {
		return river.JobSnooze(e.syncPollInterval(args.PollInterval))
	}

	return recordSyncRunSummaryOutput(workingCtx, runID, stats)
}

func (e *jobExecutor) SyncTransfer(ctx context.Context, args queue.SyncTransferJobArgs) error {
	if args.UserID == 0 {
		return fmt.Errorf("missing user id")
	}
	if strings.TrimSpace(args.Source) == "" {
		return fmt.Errorf("missing source")
	}
	if strings.TrimSpace(args.Name) == "" {
		return fmt.Errorf("missing file name")
	}

	workingCtx, err := e.workingContext(ctx, args.UserID)
	if err != nil {
		return err
	}

	reader, size, mimeType, err := e.openSourceFile(workingCtx, args)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	if size <= 0 {
		return fmt.Errorf("source size must be greater than zero")
	}
	if args.Encrypted && e.api.cnf.TG.Uploads.EncryptionKey == "" {
		return fmt.Errorf("sync encryption is not enabled")
	}

	stager, err := e.api.newUploadStager(workingCtx, args.UserID, 0)
	if err != nil {
		return err
	}
	defer stager.Close()

	uploadID, err := e.syncUploadID(workingCtx, args)
	if err != nil {
		return err
	}
	if err := e.uploadSyncTransferParts(workingCtx, stager, reader, args, uploadID, size); err != nil {
		e.cleanupUpload(workingCtx, uploadID, "upload transfer failed")
		return err
	}

	fileReq := &api.File{
		UploadId: api.NewOptString(uploadID),
		Path:     api.NewOptString(cleanPath(args.DestinationPath)),
		Name:     args.Name,
		Type:     api.FileTypeFile,
		Size:     api.NewOptInt64(size),
	}
	if strings.TrimSpace(args.MimeType) != "" {
		fileReq.MimeType = api.NewOptString(args.MimeType)
	} else if mimeType != "" {
		fileReq.MimeType = api.NewOptString(mimeType)
	}
	if args.ModifiedAtUnixNano > 0 {
		fileReq.UpdatedAt = api.NewOptDateTime(time.Unix(0, args.ModifiedAtUnixNano).UTC())
	}
	if args.Encrypted {
		fileReq.Encrypted = api.NewOptBool(true)
	}

	created, err := e.api.FilesCreate(workingCtx, fileReq)
	if err != nil {
		e.cleanupUpload(workingCtx, uploadID, "create file from upload failed")
		return err
	}

	return recordSyncTransferOutput(workingCtx, args.RunID, uploadID, created.ID.Value, created.Name, args.Source)
}

func (e *jobExecutor) cleanupUpload(ctx context.Context, uploadID string, reason string) {
	if err := e.api.repo.Uploads.Delete(ctx, uploadID); err != nil {
		logging.FromContext(ctx).Warn("failed to cleanup upload state",
			zap.String("upload_id", uploadID),
			zap.String("reason", reason),
			zap.Error(err),
		)
	}
}

type syncTransferOutput struct {
	Data struct {
		RunID    string `json:"runId"`
		UploadID string `json:"uploadId"`
		File     struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"file"`
	} `json:"data"`
}

func (e *jobExecutor) listSourceFiles(ctx context.Context, args queue.SyncRunJobArgs) ([]sourceFile, error) {
	fs, err := remoteFSForSource(args.Source)
	if err != nil {
		return nil, err
	}
	entries, err := fs.List(ctx, "", args.Headers, args.Proxy)
	if err != nil {
		return nil, err
	}
	entries = filterSourceEntries(entries, args.Filters)
	out := make([]sourceFile, 0, len(entries))
	for _, entry := range entries {
		out = append(out, sourceFile{
			relPath:   entry.RelPath,
			sourceURI: args.Source,
			size:      entry.Size,
			name:      entry.Name,
			mimeType:  entry.MimeType,
			hash:      entry.Hash,
			modified:  entry.ModifiedAt,
			fullPath:  entry.SourcePath,
		})
	}
	return out, nil
}

func (e *jobExecutor) openSourceFile(ctx context.Context, args queue.SyncTransferJobArgs) (io.ReadCloser, int64, string, error) {
	fs, err := remoteFSForSource(args.Source)
	if err != nil {
		return nil, 0, "", err
	}
	return fs.Open(ctx, args.SourcePath, args.Headers, args.Proxy, args.Size)
}

func filterSourceEntries(entries []remotes.Entry, filters queue.SyncFilters) []remotes.Entry {
	if len(entries) == 0 {
		return entries
	}
	markers := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		markers[e.RelPath] = struct{}{}
	}
	out := make([]remotes.Entry, 0, len(entries))
	for _, e := range entries {
		if filters.MinSize > 0 && e.Size < filters.MinSize {
			continue
		}
		if filters.MaxSize > 0 && e.Size > filters.MaxSize {
			continue
		}
		if matchesAnyPattern(e.RelPath, filters.Exclude) {
			continue
		}
		if len(filters.Include) > 0 && !matchesAnyPattern(e.RelPath, filters.Include) {
			continue
		}
		if excludedByMarker(e.RelPath, filters.ExcludeIfPresent, markers) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func excludedByMarker(relPath string, markers []string, files map[string]struct{}) bool {
	if len(markers) == 0 {
		return false
	}
	dir := path.Dir(relPath)
	for {
		prefix := ""
		if dir != "." && dir != "/" {
			prefix = strings.TrimPrefix(dir, "./") + "/"
		}
		for _, marker := range markers {
			candidate := prefix + strings.TrimPrefix(marker, "/")
			if _, ok := files[candidate]; ok {
				return true
			}
		}
		if dir == "." || dir == "/" || dir == "" {
			break
		}
		dir = path.Dir(dir)
	}
	return false
}

func matchesAnyPattern(relPath string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if ok, _ := path.Match(p, relPath); ok {
			return true
		}
		if strings.Contains(p, "**") {
			prefix := strings.TrimSuffix(p, "**")
			if strings.HasPrefix(relPath, strings.TrimSuffix(prefix, "/")) {
				return true
			}
		}
	}
	return false
}

func (e *jobExecutor) resolveDestinationPath(ctx context.Context, userID int64, destination string) (string, error) {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return "/", nil
	}
	if isUUID(destination) {
		id, err := uuid.Parse(destination)
		if err != nil {
			return "", err
		}
		if _, err := e.api.repo.Files.GetByIDAndUser(ctx, id, userID); err != nil {
			return "", err
		}
		p, err := e.api.repo.Files.GetFullPath(ctx, id)
		if err != nil {
			return "", err
		}
		return cleanPath(p), nil
	}
	return cleanPath(destination), nil
}

func (e *jobExecutor) compareDestinationFingerprint(ctx context.Context, userID int64, destinationPath, name string, src sourceFile) (bool, bool, error) {
	parentID, err := e.api.repo.Files.ResolvePathID(ctx, destinationPath, userID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return false, false, nil
		}
		return false, false, err
	}
	existing, err := e.api.repo.Files.GetActiveByNameAndParent(ctx, userID, name, parentID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return false, false, nil
		}
		return false, false, err
	}

	if src.hash != "" && existing.Hash != nil && *existing.Hash != "" {
		return true, src.hash == *existing.Hash, nil
	}
	if existing.Size == nil || *existing.Size != src.size {
		return true, false, nil
	}
	if src.modified.IsZero() {
		return true, false, nil
	}
	return true, existing.UpdatedAt.UTC().Unix() == src.modified.UTC().Unix(), nil
}

func (e *jobExecutor) extraDestinationFileIDs(ctx context.Context, destinationRoot string, sourceRel map[string]struct{}) ([]string, error) {
	cursor := ""
	ids := make([]string, 0)
	for {
		params := api.FilesListParams{
			Path:       api.NewOptString(destinationRoot),
			Operation:  api.NewOptFileQueryOperation(api.FileQueryOperationFind),
			DeepSearch: api.NewOptBool(true),
			Limit:      api.NewOptInt(1000),
		}
		if cursor != "" {
			params.Cursor = api.NewOptString(cursor)
		}
		res, err := e.api.FilesList(ctx, params)
		if err != nil {
			return nil, err
		}
		for _, item := range res.Items {
			if item.Type != api.FileTypeFile || !item.Path.IsSet() || !item.ID.IsSet() {
				continue
			}
			rel := strings.TrimPrefix(item.Path.Value, strings.TrimSuffix(destinationRoot, "/")+"/")
			if rel == item.Path.Value || rel == "" {
				continue
			}
			if _, ok := sourceRel[rel]; !ok {
				ids = append(ids, uuid.UUID(item.ID.Value).String())
			}
		}
		if !res.Meta.NextCursor.IsSet() || res.Meta.NextCursor.Value == "" {
			break
		}
		cursor = res.Meta.NextCursor.Value
	}
	return ids, nil
}

func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

func remoteFSForSource(source string) (remotes.FS, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(u.Scheme) {
	case "local":
		return local.New(source)
	case "dav":
		return webdav.New(source)
	case "sftp":
		return sftp.New(source)
	case "rclone", "rclones":
		return rclone.New(source)
	default:
		return nil, fmt.Errorf("unsupported source scheme %q", u.Scheme)
	}
}

func (e *jobExecutor) initializeSyncWorkflow(ctx context.Context, workflow *river.WorkflowT[pgx.Tx], client *river.Client[pgx.Tx], args queue.SyncRunJobArgs, runID, destinationRoot string, files []sourceFile) (*syncRunPlan, error) {
	plan := &syncRunPlan{depNames: make([]string, 0, len(files))}
	sourceSet := make(map[string]struct{}, len(files))
	for _, src := range files {
		sourceSet[src.relPath] = struct{}{}
	}
	if args.Options.Sync {
		ids, err := e.extraDestinationFileIDs(ctx, destinationRoot, sourceSet)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if err := e.api.FilesDeleteById(ctx, api.FilesDeleteByIdParams{ID: api.UUID(uuid.MustParse(id))}); err != nil {
				return nil, err
			}
			plan.deleted++
		}
	}

	for _, src := range files {
		plan.planned++
		destinationPath := cleanPath(path.Join(destinationRoot, normalizeSourceDir(src.relPath)))
		exists, sameFingerprint, err := e.compareDestinationFingerprint(ctx, args.UserID, destinationPath, src.name, src)
		if err != nil {
			return nil, err
		}
		if sameFingerprint || (exists && !sameFingerprint && !args.Options.Sync) {
			plan.skipped++
			continue
		}

		taskName := syncTransferTaskName(plan.queued + 1)
		workflow.Add(taskName, newSyncTransferJobArgs(args, runID, src, destinationPath), &river.InsertOpts{MaxAttempts: 1, Queue: queue.QueueUploads}, nil)
		plan.depNames = append(plan.depNames, taskName)
		plan.queued++
		plan.bytesPlanned += src.size
	}

	prepared, err := workflow.Prepare(ctx)
	if err != nil {
		return nil, err
	}
	if len(prepared.Jobs) > 0 {
		if _, err := client.InsertMany(ctx, prepared.Jobs); err != nil {
			return nil, err
		}
	}

	return plan, nil
}

func summarizeSyncWorkflow(tasks *river.WorkflowTasks) (*syncWorkflowStats, error) {
	stats := &syncWorkflowStats{files: make([]map[string]any, 0)}
	for _, taskName := range tasks.Names() {
		if !strings.HasPrefix(taskName, syncTransferTaskPrefix) {
			continue
		}
		task := tasks.Get(taskName)
		if task == nil || task.Job == nil {
			continue
		}
		stats.total++
		switch task.Job.State {
		case rivertype.JobStateCompleted:
			stats.completed++
			var out syncTransferOutput
			if err := task.Output(&out); err == nil {
				stats.files = append(stats.files, map[string]any{"task": taskName, "id": out.Data.File.ID, "name": out.Data.File.Name})
			}
		case rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
			stats.failed++
		}
	}

	return stats, nil
}

func (e *jobExecutor) uploadSyncTransferParts(ctx context.Context, stager *uploadStager, reader io.Reader, args queue.SyncTransferJobArgs, uploadID string, size int64) error {
	partSize := args.PartSize
	if partSize <= 0 {
		partSize = 100 * 1024 * 1024
	}
	totalParts := int((size + partSize - 1) / partSize)
	var uploaded int64

	return stager.Run(ctx, func(ctx context.Context) error {
		for i := 0; i < totalParts; i++ {
			partNo := i + 1
			chunkSize := minInt64(partSize, size-uploaded)

			if _, err := stager.StagePart(ctx, uploadStagePartRequest{
				UploadID:  uploadID,
				FileName:  args.Name,
				PartNo:    partNo,
				Reader:    io.LimitReader(reader, chunkSize),
				Size:      chunkSize,
				Encrypted: args.Encrypted,
				Hashing:   true,
				Threads:   e.api.cnf.TG.Uploads.Threads,
			}, logging.FromContext(ctx)); err != nil {
				return err
			}

			uploaded += chunkSize
			if err := writeJobProgress(ctx, partNo, totalParts, []map[string]any{{"partNo": partNo, "success": true}}); err != nil {
				return err
			}
		}

		return nil
	})
}

func (e *jobExecutor) syncUploadID(ctx context.Context, args queue.SyncTransferJobArgs) (string, error) {
	modTimeUnixNano := int64(0)
	if args.ModifiedAtUnixNano > 0 {
		modTimeUnixNano = args.ModifiedAtUnixNano
	}

	return md5Hex(fmt.Sprintf("%s:%s:%d:%d:%d", cleanPath(args.DestinationPath), args.Name, args.Size, modTimeUnixNano, args.UserID)), nil
}

func md5Hex(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func recordSyncRunPlanOutput(ctx context.Context, runID, source, destination string, plan *syncRunPlan) error {
	return river.RecordOutput(ctx, map[string]any{
		"progress": map[string]any{"total": plan.planned, "done": 0, "percent": 0},
		"data":     map[string]any{"runId": runID, "source": source, "destination": destination, "planned": plan.planned, "queued": plan.queued, "skipped": plan.skipped, "deleted": plan.deleted, "bytesPlanned": plan.bytesPlanned},
	})
}

func recordSyncRunSummaryOutput(ctx context.Context, runID string, stats *syncWorkflowStats) error {
	return river.RecordOutput(ctx, map[string]any{
		"progress": map[string]any{"total": stats.total, "done": stats.total, "percent": 100},
		"data":     map[string]any{"runId": runID, "completed": stats.completed, "failed": stats.failed, "files": stats.files},
	})
}

func recordSyncTransferOutput(ctx context.Context, runID, uploadID string, fileID any, name, source string) error {
	return river.RecordOutput(ctx, map[string]any{
		"progress": map[string]any{"total": 1, "done": 1, "percent": 100},
		"data":     map[string]any{"runId": runID, "uploadId": uploadID, "file": map[string]any{"id": fileID, "name": name}, "source": source},
	})
}

func syncTransferTaskName(n int) string { return fmt.Sprintf("%s%06d", syncTransferTaskPrefix, n) }

func normalizeSourceDir(relPath string) string {
	dir := path.Dir(relPath)
	if dir == "." {
		return ""
	}
	return dir
}

func newSyncTransferJobArgs(args queue.SyncRunJobArgs, runID string, src sourceFile, destinationPath string) queue.SyncTransferJobArgs {
	child := queue.SyncTransferJobArgs{UserID: args.UserID, RunID: runID, Source: args.Source, SourcePath: src.fullPath, DestinationPath: destinationPath, Name: src.name, Size: src.size, MimeType: src.mimeType, Hash: src.hash, Headers: args.Headers, Proxy: args.Proxy, PartSize: args.Options.PartSize, Encrypted: args.Options.Encrypted}
	if !src.modified.IsZero() {
		child.ModifiedAtUnixNano = src.modified.UTC().UnixNano()
	}
	return child
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (e *jobExecutor) syncPollInterval(secs int) time.Duration {
	if secs <= 0 {
		return 5 * time.Second
	}
	if secs > 300 {
		secs = 300
	}
	return time.Duration(secs) * time.Second
}
