package services

import (
	"context"
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
	jetmodel "github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/pkg/queue"
	"github.com/tgdrive/teldrive/pkg/remotes"
	"github.com/tgdrive/teldrive/pkg/remotes/local"
	"github.com/tgdrive/teldrive/pkg/remotes/rclone"
	"github.com/tgdrive/teldrive/pkg/remotes/sftp"
	"github.com/tgdrive/teldrive/pkg/remotes/webdav"
	"github.com/tgdrive/teldrive/pkg/repositories"
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
		planned := 0
		skipped := 0
		queued := 0
		deleted := 0
		var bytesPlanned int64
		depNames := make([]string, 0, len(files))
		sourceSet := make(map[string]struct{}, len(files))
		for _, src := range files {
			sourceSet[src.relPath] = struct{}{}
		}
		if args.Options.Sync {
			ids, err := e.extraDestinationFileIDs(workingCtx, destinationRoot, sourceSet)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if err := e.api.FilesDeleteById(workingCtx, api.FilesDeleteByIdParams{ID: id}); err != nil {
					return err
				}
				deleted++
			}
		}

		for _, src := range files {
			planned++
			dir := path.Dir(src.relPath)
			if dir == "." {
				dir = ""
			}
			destinationPath := cleanPath(path.Join(destinationRoot, dir))

			exists, sameFingerprint, err := e.compareDestinationFingerprint(workingCtx, args.UserID, destinationPath, src.name, src)
			if err != nil {
				return err
			}
			if sameFingerprint {
				skipped++
				continue
			}
			if exists && !sameFingerprint && !args.Options.Sync {
				skipped++
				continue
			}

			child := queue.SyncTransferJobArgs{
				UserID:          args.UserID,
				RunID:           runID,
				Source:          args.Source,
				SourcePath:      src.fullPath,
				DestinationPath: destinationPath,
				Name:            src.name,
				Size:            src.size,
				MimeType:        src.mimeType,
				Hash:            src.hash,
				Headers:         args.Headers,
				Proxy:           args.Proxy,
				PartSize:        args.Options.PartSize,
			}
			if !src.modified.IsZero() {
				child.ModifiedAtUnix = src.modified.UTC().Unix()
			}

			opts := &river.InsertOpts{MaxAttempts: 1, Queue: queue.QueueUploads}

			taskName := fmt.Sprintf("transfer_%06d", queued+1)
			workflow.Add(taskName, child, opts, nil)
			depNames = append(depNames, taskName)
			queued++
			bytesPlanned += src.size
		}

		workflow.Add("finalize", queue.SyncFinalizeJobArgs{UserID: args.UserID, RunID: runID}, nil, &river.WorkflowTaskOpts{
			Deps:                depNames,
			IgnoreCancelledDeps: true,
			IgnoreDiscardedDeps: true,
			IgnoreDeletedDeps:   true,
		})
		prepared, err := workflow.Prepare(workingCtx)
		if err != nil {
			return err
		}
		if len(prepared.Jobs) > 0 {
			if _, err := client.InsertMany(workingCtx, prepared.Jobs); err != nil {
				return err
			}
		}

		if err := river.RecordOutput(workingCtx, map[string]any{
			"progress": map[string]any{"total": planned, "done": 0, "percent": 0},
			"data": map[string]any{
				"runId":        runID,
				"source":       args.Source,
				"destination":  destinationRoot,
				"planned":      planned,
				"queued":       queued,
				"skipped":      skipped,
				"deleted":      deleted,
				"bytesPlanned": bytesPlanned,
			},
		}); err != nil {
			return err
		}
	}

	fresh, err := workflow.LoadAll(workingCtx, nil)
	if err != nil {
		return err
	}

	transferTotal := 0
	transferCompleted := 0
	transferFailed := 0
	for _, taskName := range fresh.Names() {
		if !strings.HasPrefix(taskName, "transfer_") {
			continue
		}
		task := fresh.Get(taskName)
		if task == nil || task.Job == nil {
			continue
		}
		transferTotal++
		switch task.Job.State {
		case rivertype.JobStateCompleted:
			transferCompleted++
		case rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
			transferFailed++
		}
	}

	if transferFailed > 0 {
		return fmt.Errorf("sync workflow failed for %d transfer tasks", transferFailed)
	}
	if transferTotal > 0 && transferCompleted < transferTotal {
		return river.JobSnooze(e.syncPollInterval(args.PollInterval))
	}

	finalTask := fresh.Get("finalize")
	if finalTask == nil || finalTask.Job == nil {
		return river.JobSnooze(e.syncPollInterval(args.PollInterval))
	}

	switch finalTask.Job.State {
	case rivertype.JobStateCompleted:
		var summary syncFinalizeSummary
		if err := workflow.LoadOutput(workingCtx, "finalize", &summary); err != nil {
			return err
		}
		if summary.Data.Failed > 0 {
			return fmt.Errorf("sync workflow failed for %d transfer tasks", summary.Data.Failed)
		}
		return river.RecordOutput(workingCtx, map[string]any{
			"progress": map[string]any{"total": summary.Data.Completed, "done": summary.Data.Completed, "percent": 100},
			"data":     summary.Data,
		})
	case rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
		return fmt.Errorf("sync workflow finalize task ended in state %s", finalTask.Job.State)
	default:
		return river.JobSnooze(e.syncPollInterval(args.PollInterval))
	}
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

	channelID, err := e.resolveUploadChannel(workingCtx, args.UserID)
	if err != nil {
		return err
	}

	client, _, _, _, err := e.api.getUploadClient(workingCtx, args.UserID)
	if err != nil {
		return err
	}

	uploadPool, err := e.api.telegram.NewUploadPool(workingCtx, client, int64(e.api.cnf.TG.PoolSize), e.api.cnf.TG.Uploads.MaxRetries)
	if err != nil {
		return err
	}
	defer uploadPool.Close()

	partSize := args.PartSize
	if partSize <= 0 {
		partSize = 100 * 1024 * 1024
	}
	totalParts := int((size + partSize - 1) / partSize)
	uploadID := uuid.NewString()

	var uploaded int64
	for i := 0; i < totalParts; i++ {
		partNo := i + 1
		remaining := size - uploaded
		chunkSize := partSize
		if remaining < chunkSize {
			chunkSize = remaining
		}

		partName := fmt.Sprintf("%s.part%03d", args.Name, partNo)
		partReader := io.LimitReader(reader, chunkSize)
		tgClient := uploadPool.Default(workingCtx)
		partID, uploadedSize, err := e.api.telegram.UploadPart(workingCtx, tgClient, channelID, partName, partReader, chunkSize, e.api.cnf.TG.Uploads.Threads)
		if err != nil {
			_ = e.api.repo.Uploads.Delete(workingCtx, uploadID)
			return err
		}
		if uploadedSize != chunkSize {
			_ = e.api.repo.Uploads.Delete(workingCtx, uploadID)
			return fmt.Errorf("uploaded size mismatch for %s", partName)
		}

		now := time.Now().UTC()
		if err := e.api.repo.Uploads.Create(workingCtx, &jetmodel.Uploads{
			Name:      partName,
			UploadID:  uploadID,
			PartID:    int32(partID),
			ChannelID: channelID,
			Size:      chunkSize,
			PartNo:    int32(partNo),
			UserID:    &args.UserID,
			CreatedAt: &now,
		}); err != nil {
			_ = e.api.repo.Uploads.Delete(workingCtx, uploadID)
			return err
		}

		uploaded += chunkSize
		if err := writeJobProgress(workingCtx, partNo, totalParts, []map[string]any{{"partNo": partNo, "success": true}}); err != nil {
			return err
		}
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
	if args.ModifiedAtUnix > 0 {
		fileReq.UpdatedAt = api.NewOptDateTime(time.Unix(args.ModifiedAtUnix, 0).UTC())
	}

	created, err := e.api.FilesCreate(workingCtx, fileReq)
	if err != nil {
		_ = e.api.repo.Uploads.Delete(workingCtx, uploadID)
		return err
	}

	return river.RecordOutput(workingCtx, map[string]any{
		"progress": map[string]any{"total": 1, "done": 1, "percent": 100},
		"data": map[string]any{
			"runId":    args.RunID,
			"uploadId": uploadID,
			"file":     map[string]any{"id": created.ID.Value, "name": created.Name},
			"source":   args.Source,
		},
	})
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

type syncFinalizeSummary struct {
	Data struct {
		RunID     string           `json:"runId"`
		Completed int              `json:"completed"`
		Failed    int              `json:"failed"`
		Files     []map[string]any `json:"files"`
	} `json:"data"`
}

func (e *jobExecutor) SyncFinalize(ctx context.Context, args queue.SyncFinalizeJobArgs) error {
	if args.UserID == 0 {
		return fmt.Errorf("missing user id")
	}
	if strings.TrimSpace(args.RunID) == "" {
		return fmt.Errorf("missing run id")
	}

	workingCtx, err := e.workingContext(ctx, args.UserID)
	if err != nil {
		return err
	}

	client := river.ClientFromContext[pgx.Tx](workingCtx)
	if client == nil {
		return fmt.Errorf("river client not available in context")
	}

	workflow := client.NewWorkflow(&river.WorkflowOpts{ID: args.RunID})
	deps, err := workflow.LoadDeps(workingCtx, "finalize", &river.WorkflowLoadDepsOpts{Recursive: false})
	if err != nil {
		return err
	}

	completed := 0
	failed := 0
	files := make([]map[string]any, 0)
	for _, taskName := range deps.Names() {
		if taskName == "finalize" {
			continue
		}
		task := deps.Get(taskName)
		if task == nil || task.Job == nil {
			failed++
			continue
		}
		if task.Job.State != rivertype.JobStateCompleted {
			failed++
			continue
		}
		var out syncTransferOutput
		if err := deps.Output(taskName, &out); err != nil {
			failed++
			continue
		}
		completed++
		files = append(files, map[string]any{"task": taskName, "id": out.Data.File.ID, "name": out.Data.File.Name})
	}

	return river.RecordOutput(workingCtx, map[string]any{
		"progress": map[string]any{"total": completed + failed, "done": completed + failed, "percent": 100},
		"data": map[string]any{
			"runId":     args.RunID,
			"completed": completed,
			"failed":    failed,
			"files":     files,
		},
	})
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
				ids = append(ids, item.ID.Value)
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

func (e *jobExecutor) syncPollInterval(secs int) time.Duration {
	if secs <= 0 {
		return 5 * time.Second
	}
	if secs > 300 {
		secs = 300
	}
	return time.Duration(secs) * time.Second
}
