package integration_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/tgdrive/teldrive/internal/config"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/pkg/queue"
	"github.com/tgdrive/teldrive/pkg/services"
)

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
	riverClient, err := queue.NewClient(s.pool, services.NewJobExecutor(apiSvc), config.QueueConfig{})
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

	if _, err := s.repos.Files.CreateDirectories(s.ctx, userID, "/sync-int"); err != nil {
		t.Fatalf("create destination path: %v", err)
	}

	inserted, err := riverClient.Insert(s.ctx, queue.SyncRunJobArgs{
		UserID:         userID,
		Source:         "local://" + srcDir,
		DestinationDir: "/sync-int",
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
	parentID, err := s.repos.Files.ResolvePathID(s.ctx, "/sync-int", userID)
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
	riverClient, err := queue.NewClient(s.pool, services.NewJobExecutor(apiSvc), config.QueueConfig{})
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
