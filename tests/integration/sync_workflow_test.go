package integration_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/tg"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/tgdrive/teldrive/internal/config"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/pkg/queue"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"github.com/tgdrive/teldrive/pkg/services"
)

func ptrInt64(v int64) *int64 { return &v }

func TestSyncRunWorkflow_LocalSource_Completes(t *testing.T) {
	s := newSuite(t)
	resetRiverTables(t, s)

	const userID int64 = 91001
	_ = s.authTokenForUser(userID, "sync-workflow-session")

	selected := true
	now := time.Now().UTC()
	if err := s.repos.Channels.Create(s.ctx, &jetmodel.Channels{
		ChannelID:   9100101,
		ChannelName: "sync-test",
		UserID:      userID,
		Selected:    &selected,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create selected channel: %v", err)
	}

	s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, _ int64, _ string, fileStream io.Reader, fileSize int64, _ int) (int, int64, error) {
		n, err := io.Copy(io.Discard, fileStream)
		if err != nil {
			return 0, 0, err
		}
		return 1, n, nil
	}

	channelManager := tgc.NewChannelManager(s.repos, s.cache, &s.cfg.TG)
	apiSvc := services.NewApiService(s.repos, channelManager, s.cfg, s.cache, s.tgMock, s.events, nil)
	riverClient, err := queue.NewClient(s.pool, services.NewJobExecutor(apiSvc), config.QueueConfig{}, config.JobsConfig{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}
	apiSvc.SetJobClient(riverClient)

	if err := riverClient.Start(s.ctx); err != nil {
		t.Fatalf("start river client: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = riverClient.Stop(stopCtx)
	})

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "payload.txt")
	if err := os.WriteFile(srcFile, []byte("sync integration payload"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	inserted, err := riverClient.Insert(s.ctx, queue.SyncRunJobArgs{
		UserID:         userID,
		Source:         "local://" + srcDir,
		DestinationDir: "/sync-int/nested/path",
		Options:        queue.SyncOptions{Sync: true},
		PollInterval:   1,
	}, &river.InsertOpts{MaxAttempts: 2})
	if err != nil {
		t.Fatalf("insert sync.run: %v", err)
	}

	deadline := time.Now().Add(45 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for sync.run completion")
		}
		job, err := riverClient.JobGet(s.ctx, inserted.Job.ID)
		if err != nil {
			t.Fatalf("job get: %v", err)
		}
		switch job.State {
		case rivertype.JobStateCompleted:
			goto Verify
		case rivertype.JobStateDiscarded, rivertype.JobStateCancelled:
			t.Fatalf("sync.run finished in bad state: %s", job.State)
		}
		time.Sleep(300 * time.Millisecond)
	}

Verify:
	parentID, err := s.repos.Files.ResolvePathID(s.ctx, "/sync-int/nested/path", userID)
	if err != nil {
		t.Fatalf("resolve destination path: %v", err)
	}
	var createdCount int
	err = s.pool.QueryRow(s.ctx, `
		SELECT COUNT(1)
		FROM teldrive.files
		WHERE user_id = $1 AND parent_id = $2 AND name = $3 AND type = 'file' AND status = 'active'
	`, userID, *parentID, "payload.txt").Scan(&createdCount)
	if err != nil {
		t.Fatalf("expected synced file in destination: %v", err)
	}
	if createdCount != 1 {
		t.Fatalf("expected exactly one synced file, got %d", createdCount)
	}
}

func TestSyncRunWorkflow_LocalSource_PrunesExtraDestinationFolders(t *testing.T) {
	s := newSuite(t)
	resetRiverTables(t, s)

	const userID int64 = 91005
	_ = s.authTokenForUser(userID, "sync-workflow-prune-session")

	selected := true
	now := time.Now().UTC()
	if err := s.repos.Channels.Create(s.ctx, &jetmodel.Channels{
		ChannelID:   9100501,
		ChannelName: "sync-test-prune",
		UserID:      userID,
		Selected:    &selected,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create selected channel: %v", err)
	}

	s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, _ int64, _ string, fileStream io.Reader, fileSize int64, _ int) (int, int64, error) {
		n, err := io.Copy(io.Discard, fileStream)
		if err != nil {
			return 0, 0, err
		}
		return 1, n, nil
	}

	channelManager := tgc.NewChannelManager(s.repos, s.cache, &s.cfg.TG)
	apiSvc := services.NewApiService(s.repos, channelManager, s.cfg, s.cache, s.tgMock, s.events, nil)
	riverClient, err := queue.NewClient(s.pool, services.NewJobExecutor(apiSvc), config.QueueConfig{}, config.JobsConfig{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}
	apiSvc.SetJobClient(riverClient)

	if err := riverClient.Start(s.ctx); err != nil {
		t.Fatalf("start river client: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = riverClient.Stop(stopCtx)
	})

	rootID, err := s.repos.Files.CreateDirectories(s.ctx, userID, "/sync-prune/extra/sub")
	if err != nil {
		t.Fatalf("create extra destination folders: %v", err)
	}
	status := "active"
	extraFile := &jetmodel.Files{ID: uuid.New(), Name: "old.txt", Type: "file", MimeType: "text/plain", UserID: userID, Status: &status, ParentID: rootID, Size: ptrInt64(4), ChannelID: ptrInt64(9100501), CreatedAt: now, UpdatedAt: now}
	if err := s.repos.Files.Create(s.ctx, extraFile); err != nil {
		t.Fatalf("create extra destination file: %v", err)
	}

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "payload.txt")
	if err := os.WriteFile(srcFile, []byte("sync integration payload"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	inserted, err := riverClient.Insert(s.ctx, queue.SyncRunJobArgs{
		UserID:         userID,
		Source:         "local://" + srcDir,
		DestinationDir: "/sync-prune",
		Options:        queue.SyncOptions{Sync: true},
		PollInterval:   1,
	}, &river.InsertOpts{MaxAttempts: 2})
	if err != nil {
		t.Fatalf("insert sync.run: %v", err)
	}

	deadline := time.Now().Add(45 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for sync.run completion")
		}
		job, err := riverClient.JobGet(s.ctx, inserted.Job.ID)
		if err != nil {
			t.Fatalf("job get: %v", err)
		}
		switch job.State {
		case rivertype.JobStateCompleted:
			goto VerifyPrune
		case rivertype.JobStateDiscarded, rivertype.JobStateCancelled:
			t.Fatalf("sync.run finished in bad state: %s", job.State)
		}
		time.Sleep(300 * time.Millisecond)
	}

VerifyPrune:
	if _, err := s.repos.Files.ResolvePathID(s.ctx, "/sync-prune/extra", userID); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("expected extra folder to be pruned, got err=%v", err)
	}
}

func TestSyncRunWorkflow_LocalSource_RetriesAndDiscardsOnFailure(t *testing.T) {
	s := newSuite(t)
	resetRiverTables(t, s)

	const userID int64 = 91002
	_ = s.authTokenForUser(userID, "sync-workflow-failure-session")

	selected := true
	now := time.Now().UTC()
	if err := s.repos.Channels.Create(s.ctx, &jetmodel.Channels{
		ChannelID:   9100201,
		ChannelName: "sync-test-failure",
		UserID:      userID,
		Selected:    &selected,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create selected channel: %v", err)
	}

	s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, _ int64, _ string, _ io.Reader, _ int64, _ int) (int, int64, error) {
		return 0, 0, errors.New("forced upload failure")
	}

	channelManager := tgc.NewChannelManager(s.repos, s.cache, &s.cfg.TG)
	apiSvc := services.NewApiService(s.repos, channelManager, s.cfg, s.cache, s.tgMock, s.events, nil)
	riverClient, err := queue.NewClient(s.pool, services.NewJobExecutor(apiSvc), config.QueueConfig{}, config.JobsConfig{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}
	apiSvc.SetJobClient(riverClient)

	if err := riverClient.Start(s.ctx); err != nil {
		t.Fatalf("start river client: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = riverClient.Stop(stopCtx)
	})

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "payload.txt")
	if err := os.WriteFile(srcFile, []byte("sync failure payload"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	if _, err := s.repos.Files.CreateDirectories(s.ctx, userID, "/sync-int-failure"); err != nil {
		t.Fatalf("create destination path: %v", err)
	}

	inserted, err := riverClient.Insert(s.ctx, queue.SyncRunJobArgs{
		UserID:         userID,
		Source:         "local://" + srcDir,
		DestinationDir: "/sync-int-failure",
		Options:        queue.SyncOptions{Sync: true},
		PollInterval:   1,
	}, &river.InsertOpts{MaxAttempts: 2})
	if err != nil {
		t.Fatalf("insert sync.run: %v", err)
	}

	deadline := time.Now().Add(120 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for sync.run discard")
		}
		job, err := riverClient.JobGet(s.ctx, inserted.Job.ID)
		if err != nil {
			t.Fatalf("job get: %v", err)
		}
		switch job.State {
		case rivertype.JobStateDiscarded:
			if job.Attempt < 2 {
				t.Fatalf("expected retry attempts >=2 before discard, got %d", job.Attempt)
			}
			return
		case rivertype.JobStateCompleted:
			t.Fatalf("expected sync.run to fail and discard, got completed")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func TestSyncRunWorkflow_LocalSource_ResumesUploadedPartsOnRetry(t *testing.T) {
	s := newSuite(t)
	resetRiverTables(t, s)

	const userID int64 = 91003
	_ = s.authTokenForUser(userID, "sync-workflow-resume-session")
	s.cfg.TG.Uploads.ChunkNaming = "deterministic"

	selected := true
	now := time.Now().UTC()
	if err := s.repos.Channels.Create(s.ctx, &jetmodel.Channels{
		ChannelID:   9100301,
		ChannelName: "sync-test-resume",
		UserID:      userID,
		Selected:    &selected,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create selected channel: %v", err)
	}

	var (
		mu           sync.Mutex
		attempts     = map[string]int{}
		totalUploads atomic.Int32
	)
	s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, _ int64, partName string, fileStream io.Reader, fileSize int64, _ int) (int, int64, error) {
		totalUploads.Add(1)
		if _, err := io.Copy(io.Discard, fileStream); err != nil {
			return 0, 0, err
		}
		mu.Lock()
		defer mu.Unlock()
		attempts[partName]++
		if strings.Contains(partName, ".part.002") && attempts[partName] == 1 {
			return 0, 0, errors.New("forced second part failure")
		}
		return int(totalUploads.Load()), fileSize, nil
	}

	channelManager := tgc.NewChannelManager(s.repos, s.cache, &s.cfg.TG)
	apiSvc := services.NewApiService(s.repos, channelManager, s.cfg, s.cache, s.tgMock, s.events, nil)
	riverClient, err := queue.NewClient(s.pool, services.NewJobExecutor(apiSvc), config.QueueConfig{}, config.JobsConfig{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}
	apiSvc.SetJobClient(riverClient)

	if err := riverClient.Start(s.ctx); err != nil {
		t.Fatalf("start river client: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = riverClient.Stop(stopCtx)
	})

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "payload.bin")
	buf := make([]byte, 70*1024*1024)
	for i := range buf {
		buf[i] = byte(i % 251)
	}
	if err := os.WriteFile(srcFile, buf, 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	inserted, err := riverClient.Insert(s.ctx, queue.SyncRunJobArgs{
		UserID:         userID,
		Source:         "local://" + srcDir,
		DestinationDir: "/sync-int-resume",
		Options:        queue.SyncOptions{Sync: true, PartSize: 64 * 1024 * 1024},
		PollInterval:   1,
	}, &river.InsertOpts{MaxAttempts: 2})
	if err != nil {
		t.Fatalf("insert sync.run: %v", err)
	}

	deadline := time.Now().Add(90 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for resumed sync.run completion")
		}
		job, err := riverClient.JobGet(s.ctx, inserted.Job.ID)
		if err != nil {
			t.Fatalf("job get: %v", err)
		}
		switch job.State {
		case rivertype.JobStateCompleted:
			goto VerifyResume
		case rivertype.JobStateDiscarded, rivertype.JobStateCancelled:
			t.Fatalf("sync.run finished in bad state: %s", job.State)
		}
		time.Sleep(500 * time.Millisecond)
	}

VerifyResume:
	if got := totalUploads.Load(); got != 3 {
		t.Fatalf("expected resumed upload to call telegram upload 3 times, got %d", got)
	}
	var uploadRows int
	if err := s.pool.QueryRow(s.ctx, `SELECT COUNT(1) FROM teldrive.uploads`).Scan(&uploadRows); err != nil {
		t.Fatalf("count uploads: %v", err)
	}
	if uploadRows != 0 {
		t.Fatalf("expected uploads table to be cleared after finalization, got %d rows", uploadRows)
	}
}

func TestSyncTransfer_LiveProgressVisibleOnRunningJob(t *testing.T) {
	s := newSuite(t)
	resetRiverTables(t, s)

	const userID int64 = 91004
	_ = s.authTokenForUser(userID, "sync-workflow-progress-session")
	s.cfg.TG.Uploads.ChunkNaming = "deterministic"

	selected := true
	now := time.Now().UTC()
	if err := s.repos.Channels.Create(s.ctx, &jetmodel.Channels{
		ChannelID:   9100401,
		ChannelName: "sync-test-progress",
		UserID:      userID,
		Selected:    &selected,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create selected channel: %v", err)
	}

	releaseUpload := make(chan struct{})
	released := false
	release := func() {
		if !released {
			close(releaseUpload)
			released = true
		}
	}
	startedUpload := make(chan struct{}, 1)
	s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, _ int64, _ string, fileStream io.Reader, fileSize int64, _ int) (int, int64, error) {
		select {
		case startedUpload <- struct{}{}:
		default:
		}
		n, err := io.Copy(io.Discard, fileStream)
		if err != nil {
			return 0, 0, err
		}
		<-releaseUpload
		return 1, n, nil
	}

	channelManager := tgc.NewChannelManager(s.repos, s.cache, &s.cfg.TG)
	apiSvc := services.NewApiService(s.repos, channelManager, s.cfg, s.cache, s.tgMock, s.events, nil)
	riverClient, err := queue.NewClient(s.pool, services.NewJobExecutor(apiSvc), config.QueueConfig{}, config.JobsConfig{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}
	apiSvc.SetJobClient(riverClient)

	if err := riverClient.Start(s.ctx); err != nil {
		t.Fatalf("start river client: %v", err)
	}
	t.Cleanup(func() {
		release()
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = riverClient.Stop(stopCtx)
	})

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "payload.bin")
	buf := make([]byte, 70*1024*1024)
	for i := range buf {
		buf[i] = byte(i % 251)
	}
	if err := os.WriteFile(srcFile, buf, 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	inserted, err := riverClient.Insert(s.ctx, queue.SyncRunJobArgs{
		UserID:         userID,
		Source:         "local://" + srcDir,
		DestinationDir: "/sync-int-progress",
		Options:        queue.SyncOptions{Sync: true, PartSize: 64 * 1024 * 1024},
		PollInterval:   1,
	}, &river.InsertOpts{MaxAttempts: 2})
	if err != nil {
		t.Fatalf("insert sync.run: %v", err)
	}

	select {
	case <-startedUpload:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for upload to start")
	}

	var transferID int64
	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for sync.transfer job")
		}
		err := s.pool.QueryRow(s.ctx, `SELECT id FROM teldrive.river_job WHERE kind = 'sync.transfer' ORDER BY id DESC LIMIT 1`).Scan(&transferID)
		if err == nil && transferID > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	progressVisible := false
	listProgressVisible := false
	deadline = time.Now().Add(5 * time.Second)
	for !progressVisible && time.Now().Before(deadline) {
		job, err := riverClient.JobGet(s.ctx, transferID)
		if err != nil {
			t.Fatalf("get transfer job: %v", err)
		}
		if len(job.Output()) > 0 {
			progressVisible = true
		}
		res, err := riverClient.JobList(s.ctx, river.NewJobListParams().States(rivertype.JobStateRunning, rivertype.JobStateAvailable, rivertype.JobStateRetryable, rivertype.JobStateScheduled).OrderBy(river.JobListOrderByID, river.SortOrderDesc).First(20))
		if err != nil {
			t.Fatalf("job list: %v", err)
		}
		for _, row := range res.Jobs {
			if row.ID == transferID && len(row.Output()) > 0 {
				listProgressVisible = true
				break
			}
		}
		if progressVisible && listProgressVisible {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	release()

	if !progressVisible {
		parentJob, err := riverClient.JobGet(s.ctx, inserted.Job.ID)
		if err != nil {
			t.Fatalf("get parent job: %v", err)
		}
		transferJob, err := riverClient.JobGet(s.ctx, transferID)
		if err != nil {
			t.Fatalf("get transfer job final: %v", err)
		}
		t.Fatalf("expected live progress on running sync.transfer; parent state=%s transfer state=%s transfer output=%s", parentJob.State, transferJob.State, string(transferJob.Output()))
	}
	if !listProgressVisible {
		t.Fatalf("expected live progress on running sync.transfer in job list")
	}
}

func TestSyncRunWorkflow_CancelRunningSyncTransfer(t *testing.T) {
	s := newSuite(t)
	resetRiverTables(t, s)

	const userID int64 = 91004
	_ = s.authTokenForUser(userID, "sync-workflow-cancel-session")

	selected := true
	now := time.Now().UTC()
	if err := s.repos.Channels.Create(s.ctx, &jetmodel.Channels{
		ChannelID:   9100401,
		ChannelName: "sync-test-cancel",
		UserID:      userID,
		Selected:    &selected,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create selected channel: %v", err)
	}

	uploadStarted := make(chan struct{}, 1)
	s.tgMock.uploadPartFn = func(ctx context.Context, _ *tg.Client, _ int64, _ string, _ io.Reader, _ int64, _ int) (int, int64, error) {
		select {
		case uploadStarted <- struct{}{}:
		default:
		}

		<-ctx.Done()
		return 0, 0, ctx.Err()
	}

	channelManager := tgc.NewChannelManager(s.repos, s.cache, &s.cfg.TG)
	apiSvc := services.NewApiService(s.repos, channelManager, s.cfg, s.cache, s.tgMock, s.events, nil)
	riverClient, err := queue.NewClient(s.pool, services.NewJobExecutor(apiSvc), config.QueueConfig{}, config.JobsConfig{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}
	apiSvc.SetJobClient(riverClient)

	if err := riverClient.Start(s.ctx); err != nil {
		t.Fatalf("start river client: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = riverClient.Stop(stopCtx)
	})

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "payload.txt")
	if err := os.WriteFile(srcFile, []byte(strings.Repeat("sync cancel payload", 4096)), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	if _, err := riverClient.Insert(s.ctx, queue.SyncRunJobArgs{
		UserID:         userID,
		Source:         "local://" + srcDir,
		DestinationDir: "/sync-int-cancel",
		Options:        queue.SyncOptions{Sync: true},
		PollInterval:   1,
	}, &river.InsertOpts{MaxAttempts: 2}); err != nil {
		t.Fatalf("insert sync.run: %v", err)
	}

	select {
	case <-uploadStarted:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for sync.transfer upload to start")
	}

	var transferID int64
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for sync.transfer to reach running state")
		}

		err := s.pool.QueryRow(s.ctx, `
			SELECT id
			FROM teldrive.river_job
			WHERE kind = 'sync.transfer' AND state = 'running'
			ORDER BY id DESC
			LIMIT 1
		`).Scan(&transferID)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if _, err := riverClient.JobCancel(s.ctx, transferID); err != nil {
		t.Fatalf("cancel sync.transfer: %v", err)
	}

	deadline = time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for sync.transfer cancellation")
		}

		job, err := riverClient.JobGet(s.ctx, transferID)
		if err != nil {
			t.Fatalf("get sync.transfer: %v", err)
		}
		if job.State == rivertype.JobStateCancelled {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func resetRiverTables(t *testing.T, s *suite) {
	t.Helper()
	if _, err := s.pool.Exec(s.ctx, `
		DO $$
		DECLARE
			r RECORD;
		BEGIN
			FOR r IN
				SELECT tablename
				FROM pg_tables
				WHERE schemaname = 'teldrive' AND tablename LIKE 'river_%' AND tablename <> 'river_migration'
			LOOP
				EXECUTE format('TRUNCATE TABLE teldrive.%I RESTART IDENTITY CASCADE', r.tablename);
			END LOOP;
		END $$;
	`); err != nil {
		t.Fatalf("reset river tables: %v", err)
	}
}
