package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/config"
	internalduration "github.com/tgdrive/teldrive/internal/duration"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/pkg/queue"
	"github.com/tgdrive/teldrive/pkg/repositories"
)

type jobExecutor struct {
	api *apiService
}

func NewJobExecutor(apiSvc *apiService) queue.Executor {
	return &jobExecutor{api: apiSvc}
}

func (e *jobExecutor) resolveUploadChannel(ctx context.Context, userID int64) (int64, error) {
	channelID, err := e.api.channelManager.CurrentChannel(ctx, userID)
	if err == nil && (!e.api.cnf.TG.AutoChannelCreate || !e.api.channelManager.ChannelLimitReached(channelID)) {
		return channelID, nil
	}
	if err != nil && !e.api.telegram.IsNoDefaultChannelError(err) {
		return 0, err
	}
	return e.api.channelManager.CreateNewChannel(ctx, "", userID, true)
}

func writeJobProgress(ctx context.Context, done, total int, results []map[string]any) error {
	percent := 0
	if total > 0 {
		percent = int(float64(done) * 100.0 / float64(total))
	}

	return river.RecordOutput(ctx, map[string]any{
		"progress": map[string]any{
			"total":   total,
			"done":    done,
			"percent": percent,
		},
		"data": map[string]any{
			"results":     results,
			"updatedAt":   time.Now().UTC(),
			"isCompleted": done == total,
		},
	})
}

func (e *jobExecutor) CleanOldEvents(ctx context.Context) error {
	before := time.Now().UTC().Add(-5 * 24 * time.Hour)
	_, err := e.api.repo.Events.DeleteOlderThan(ctx, before)
	return err
}

func (e *jobExecutor) CleanOldEventsForUser(ctx context.Context, args queue.CleanOldEventsArgs) error {
	retention, err := parseRetentionDuration(args.Retention)
	if err != nil {
		return err
	}
	before := time.Now().UTC().Add(-retention)
	_, err = e.api.repo.Events.DeleteOlderThanForUser(ctx, args.UserID, before)
	return err
}

type staleUploadGroupKey struct {
	ChannelID int64
	UserID    int64
	Session   string
}

type staleUploadGroup struct {
	partIDs []int
	userID  int64
}

func (e *jobExecutor) CleanStaleUploadsForUser(ctx context.Context, args queue.CleanStaleUploadsArgs) error {
	retention, err := parseRetentionDuration(args.Retention)
	if err != nil {
		return err
	}

	rows, err := e.api.repo.Uploads.ListStale(ctx, time.Now().UTC().Add(-retention))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	filtered := make([]repositories.StaleUpload, 0, len(rows))
	for _, row := range rows {
		if row.UserID != nil && *row.UserID == args.UserID {
			filtered = append(filtered, row)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	sessionByUser, err := latestSessionsByUsers(ctx, e.api, []int64{args.UserID})
	if err != nil {
		return err
	}

	groups := groupStaleUploads(filtered, sessionByUser)
	for key, group := range groups {
		if err := deleteChannelMessages(ctx, &e.api.cnf.TG, key.Session, key.ChannelID, group.partIDs); err != nil {
			return err
		}
		if err := e.api.repo.Uploads.DeleteParts(ctx, key.ChannelID, group.userID, group.partIDs); err != nil {
			return err
		}
	}

	return nil
}

func parseRetentionDuration(raw string) (time.Duration, error) {
	retention, err := internalduration.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid retention duration %q: %w", raw, err)
	}
	if retention <= 0 {
		return 0, fmt.Errorf("retention duration must be greater than zero")
	}
	return retention, nil
}

func groupStaleUploads(rows []repositories.StaleUpload, sessionByUser map[int64]string) map[staleUploadGroupKey]*staleUploadGroup {
	groups := make(map[staleUploadGroupKey]*staleUploadGroup)
	for _, row := range rows {
		if row.UserID == nil {
			continue
		}
		session := sessionByUser[*row.UserID]
		if session == "" {
			continue
		}
		key := staleUploadGroupKey{ChannelID: row.ChannelID, UserID: *row.UserID, Session: session}
		group := groups[key]
		if group == nil {
			group = &staleUploadGroup{userID: *row.UserID}
			groups[key] = group
		}
		group.partIDs = append(group.partIDs, row.PartID)
	}
	return groups
}

type pendingFilePart struct {
	ID int `json:"id"`
}

type pendingFileGroupKey struct {
	ChannelID int64
	UserID    int64
	Session   string
}

type pendingFileGroup struct {
	fileIDs []string
	partIDs []int
}

func (e *jobExecutor) CleanPendingFilesForUser(ctx context.Context, userID int64) error {
	rows, err := e.api.repo.Files.ListPendingForDeletion(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	filtered := make([]repositories.PendingFile, 0, len(rows))
	for _, row := range rows {
		if row.UserID == userID {
			filtered = append(filtered, row)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	sessionByUser, err := latestSessionsByUsers(ctx, e.api, []int64{userID})
	if err != nil {
		return err
	}

	groups := groupPendingFiles(filtered, sessionByUser)
	for key, group := range groups {
		if err := deleteChannelMessages(ctx, &e.api.cnf.TG, key.Session, key.ChannelID, group.partIDs); err != nil {
			return err
		}
	}
	if err := e.api.repo.Files.DeletePendingForDeletionByUser(ctx, userID); err != nil {
		return err
	}

	return nil
}

func groupPendingFiles(rows []repositories.PendingFile, sessionByUser map[int64]string) map[pendingFileGroupKey]*pendingFileGroup {
	groups := make(map[pendingFileGroupKey]*pendingFileGroup)
	for _, row := range rows {
		if row.ChannelID == nil {
			continue
		}
		session := sessionByUser[row.UserID]
		if session == "" {
			continue
		}
		key := pendingFileGroupKey{ChannelID: *row.ChannelID, UserID: row.UserID, Session: session}
		group := groups[key]
		if group == nil {
			group = &pendingFileGroup{}
			groups[key] = group
		}
		group.fileIDs = append(group.fileIDs, row.ID)
		if row.Parts == nil || *row.Parts == "" {
			continue
		}
		var parts []pendingFilePart
		if err := json.Unmarshal([]byte(*row.Parts), &parts); err != nil {
			continue
		}
		for _, part := range parts {
			group.partIDs = append(group.partIDs, part.ID)
		}
	}
	return groups
}

func (e *jobExecutor) workingContext(ctx context.Context, userID int64) (context.Context, error) {
	session, err := latestTGSession(ctx, e.api, userID)
	if err != nil {
		return nil, err
	}
	return auth.WithUser(ctx, userID, session), nil
}

func latestTGSession(ctx context.Context, apiSvc *apiService, userID int64) (string, error) {
	sessions, err := apiSvc.repo.Sessions.GetByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("no active telegram session found for user %d", userID)
	}
	return sessions[0].TgSession, nil
}

func latestSessionsByUsers(ctx context.Context, apiSvc *apiService, userIDs []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(userIDs))
	for _, userID := range userIDs {
		sessions, err := apiSvc.repo.Sessions.GetByUserID(ctx, userID)
		if err != nil {
			return nil, err
		}
		if len(sessions) == 0 {
			continue
		}
		out[userID] = sessions[0].TgSession
	}
	return out, nil
}

func deleteChannelMessages(ctx context.Context, tgConfig *config.TGConfig, session string, channelID int64, ids []int) error {
	if session == "" || len(ids) == 0 {
		return nil
	}
	middlewares := tgc.NewMiddleware(tgConfig, tgc.WithFloodWait(), tgc.WithRateLimit())
	client, err := tgc.AuthClient(ctx, tgConfig, session, middlewares...)
	if err != nil {
		return err
	}
	return tgc.DeleteMessages(ctx, client, channelID, ids)
}
