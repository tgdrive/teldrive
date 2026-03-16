package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-jet/jet/v2/pgxV5"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/riverqueue/river"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/table"
	internalduration "github.com/tgdrive/teldrive/internal/duration"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/queue"
)

type jobExecutor struct {
	api *apiService
}

func NewJobExecutor(apiSvc *apiService) queue.Executor {
	return &jobExecutor{api: apiSvc}
}

func (e *jobExecutor) Restore(ctx context.Context, userID int64, item queue.JobItem) error {
	workingCtx, err := e.workingContext(ctx, userID)
	if err != nil {
		return err
	}

	return e.api.FilesRestore(workingCtx, api.FilesRestoreParams{ID: item.ID})
}

type remoteFileMeta struct {
	size     int64
	name     string
	mimeType string
	modified time.Time
}

func probeRemoteFile(ctx context.Context, link string, headers map[string]string, proxyURL string) (*remoteFileMeta, error) {
	client, err := remoteHTTPClient(proxyURL, 60*time.Second)
	if err != nil {
		return nil, err
	}
	meta := &remoteFileMeta{}

	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, link, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		headReq.Header.Set(k, v)
	}
	if resp, err := client.Do(headReq); err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			meta.size = parseContentLength(resp.Header.Get("Content-Length"))
			meta.mimeType = parseMimeType(resp.Header.Get("Content-Type"))
			meta.name = parseFileNameFromHeadersOrURL(resp.Header.Get("Content-Disposition"), link)
			meta.modified = parseHTTPTime(resp.Header.Get("Last-Modified"))
		}
	}

	if meta.size > 0 {
		return meta, nil
	}

	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		getReq.Header.Set(k, v)
	}
	getReq.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(getReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if meta.mimeType == "" {
		meta.mimeType = parseMimeType(resp.Header.Get("Content-Type"))
	}
	if meta.name == "" {
		meta.name = parseFileNameFromHeadersOrURL(resp.Header.Get("Content-Disposition"), link)
	}
	if meta.modified.IsZero() {
		meta.modified = parseHTTPTime(resp.Header.Get("Last-Modified"))
	}
	if meta.size == 0 {
		meta.size = parseContentRangeTotal(resp.Header.Get("Content-Range"))
	}

	return meta, nil
}

func parseContentLength(v string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}

func parseContentRangeTotal(v string) int64 {
	idx := strings.LastIndex(v, "/")
	if idx < 0 || idx+1 >= len(v) {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(v[idx+1:]), 10, 64)
	return n
}

func parseMimeType(v string) string {
	if v == "" {
		return ""
	}
	parts := strings.Split(v, ";")
	return strings.TrimSpace(parts[0])
}

func parseHTTPTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func parseFileNameFromHeadersOrURL(contentDisposition, rawURL string) string {
	if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
		if name := strings.TrimSpace(params["filename"]); name != "" {
			return name
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if base == "." || base == "/" {
		return ""
	}
	return base
}

func (e *jobExecutor) resolveUploadChannel(ctx context.Context, userID int64) (int64, error) {
	channelID, err := e.api.channelManager.CurrentChannel(ctx, userID)
	if err == nil && !(e.api.cnf.TG.AutoChannelCreate && e.api.channelManager.ChannelLimitReached(channelID)) {
		return channelID, nil
	}
	if err != nil && !e.api.telegram.IsNoDefaultChannelError(err) {
		return 0, err
	}
	return e.api.channelManager.CreateNewChannel(ctx, "", userID, true)
}

func (e *jobExecutor) downloadAndUploadPart(
	ctx context.Context,
	uploadPool UploadPool,
	channelID int64,
	link string,
	headers map[string]string,
	proxyURL string,
	partName string,
	start int64,
	end int64,
	partSize int64,
) (int, error) {
	client, err := remoteHTTPClient(proxyURL, 0)
	if err != nil {
		return 0, err
	}
	var lastErr error

	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
		if err != nil {
			return 0, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("unexpected status code %d", resp.StatusCode)
			continue
		}

		tgClient := uploadPool.Default(ctx)
		partID, uploadedSize, err := e.api.telegram.UploadPart(ctx, tgClient, channelID, partName, resp.Body, partSize, e.api.cnf.TG.Uploads.Threads)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if uploadedSize != partSize {
			lastErr = fmt.Errorf("uploaded size mismatch for %s", partName)
			continue
		}

		return partID, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("failed to upload part")
	}
	return 0, lastErr
}

func remoteHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
		dialer, err := utils.Proxy.GetDial(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid remote upload proxy: %w", err)
		}
		transport.DialContext = dialer.DialContext
	}

	return &http.Client{Timeout: timeout, Transport: transport}, nil
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

type staleUploadRow struct {
	PartID    int
	ChannelID int64
	UserID    *int64
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

	rows, err := listStaleUploads(ctx, e.api, time.Now().UTC().Add(-retention))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	filtered := make([]staleUploadRow, 0, len(rows))
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
		if err := deleteStaleUploads(ctx, e.api, key.ChannelID, group.userID, group.partIDs); err != nil {
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

func listStaleUploads(ctx context.Context, apiSvc *apiService, before time.Time) ([]staleUploadRow, error) {
	stmt := table.Uploads.
		SELECT(table.Uploads.PartID, table.Uploads.ChannelID, table.Uploads.UserID).
		FROM(table.Uploads).
		WHERE(table.Uploads.CreatedAt.LT(postgres.TimestampT(before)))

	var out []staleUploadRow
	if err := pgxV5.Query(ctx, stmt, apiSvc.repo.Pool, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func groupStaleUploads(rows []staleUploadRow, sessionByUser map[int64]string) map[staleUploadGroupKey]*staleUploadGroup {
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

func deleteStaleUploads(ctx context.Context, apiSvc *apiService, channelID, userID int64, partIDs []int) error {
	for _, partID := range partIDs {
		stmt := table.Uploads.DELETE().WHERE(
			table.Uploads.ChannelID.EQ(postgres.Int64(channelID)).
				AND(table.Uploads.UserID.EQ(postgres.Int64(userID))).
				AND(table.Uploads.PartID.EQ(postgres.Int(int64(partID)))),
		)
		if _, err := pgxV5.Exec(ctx, stmt, apiSvc.repo.Pool); err != nil {
			return err
		}
	}
	return nil
}

type pendingFileRow struct {
	ID        string
	Parts     *string
	ChannelID *int64
	UserID    int64
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
	rows, err := listPendingFiles(ctx, e.api)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	filtered := make([]pendingFileRow, 0, len(rows))
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
	if err := deletePendingFiles(ctx, e.api, userID); err != nil {
		return err
	}

	return nil
}

func listPendingFiles(ctx context.Context, apiSvc *apiService) ([]pendingFileRow, error) {
	stmt := table.Files.
		SELECT(table.Files.ID, table.Files.Parts, table.Files.ChannelID, table.Files.UserID).
		FROM(table.Files).
		WHERE(table.Files.Type.EQ(postgres.String("file")).AND(table.Files.Status.EQ(postgres.String("purge_pending"))))

	var out []pendingFileRow
	if err := pgxV5.Query(ctx, stmt, apiSvc.repo.Pool, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func groupPendingFiles(rows []pendingFileRow, sessionByUser map[int64]string) map[pendingFileGroupKey]*pendingFileGroup {
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

func deletePendingFiles(ctx context.Context, apiSvc *apiService, userID int64) error {
	stmt := table.Files.DELETE().WHERE(
		table.Files.UserID.EQ(postgres.Int64(userID)).AND(table.Files.Status.EQ(postgres.String("purge_pending"))),
	)
	_, err := pgxV5.Exec(ctx, stmt, apiSvc.repo.Pool)
	return err
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
