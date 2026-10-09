//go:build integration

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
	"github.com/tgdrive/teldrive/v2/internal/principal"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestJobHandlersScopeUsersAndGiveAdminsGlobalAccess(t *testing.T) {
	db := testpostgres.New(t)
	runtime, err := jobs.NewRuntime(db.Pool, jobHandlerStorage{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	handler := &Handler{Jobs: runtime}
	ctx := context.Background()

	global, err := runtime.Create(ctx, jobs.CreateInput{Kind: jobs.CleanupSweepKind, Args: map[string]json.RawMessage{}})
	if err != nil {
		t.Fatalf("create global job: %v", err)
	}
	owned, err := runtime.Create(ctx, jobs.CreateInput{
		Kind: jobs.CleanupSweepKind,
		Args: map[string]json.RawMessage{"user_id": json.RawMessage(`1001`)},
	})
	if err != nil {
		t.Fatalf("create owned job: %v", err)
	}
	if _, err := runtime.Create(ctx, jobs.CreateInput{
		Kind: jobs.CleanupSweepKind,
		Args: map[string]json.RawMessage{"user_id": json.RawMessage(`2002`)},
	}); err != nil {
		t.Fatalf("create other user's job: %v", err)
	}

	adminCtx := principal.WithIdentity(ctx, principal.Identity{UserID: 1001, Roles: []string{"admin"}})
	adminPage := listJobsPage(t, handler, adminCtx)
	if len(adminPage.Tasks) != 3 {
		t.Fatalf("admin jobs = %d, want 3", len(adminPage.Tasks))
	}
	adminStats := jobStatistics(t, handler, adminCtx)
	if adminStats.Available != 3 {
		t.Fatalf("admin available jobs = %d, want 3", adminStats.Available)
	}
	if response, err := handler.GetJob(adminCtx, gen.GetJobParams{JobId: formatJobID(global.ID)}); err != nil {
		t.Fatalf("admin GetJob(global) error = %v", err)
	} else if _, ok := response.(*gen.Job); !ok {
		t.Fatalf("admin GetJob(global) response = %T, want *gen.Job", response)
	}

	ownerCtx := principal.WithIdentity(ctx, principal.Identity{UserID: 1001, Roles: []string{"owner"}})
	ownerPage := listJobsPage(t, handler, ownerCtx)
	if len(ownerPage.Tasks) != 3 {
		t.Fatalf("owner jobs = %d, want 3", len(ownerPage.Tasks))
	}

	userCtx := principal.WithIdentity(ctx, principal.Identity{UserID: 1001})
	userPage := listJobsPage(t, handler, userCtx)
	if len(userPage.Tasks) != 1 || userPage.Tasks[0].ID != formatJobID(owned.ID) {
		t.Fatalf("user jobs = %#v, want only job %d", userPage.Tasks, owned.ID)
	}
	userStats := jobStatistics(t, handler, userCtx)
	if userStats.Available != 1 {
		t.Fatalf("user available jobs = %d, want 1", userStats.Available)
	}
}

func listJobsPage(t *testing.T, handler *Handler, ctx context.Context) *gen.JobPage {
	t.Helper()
	response, err := handler.ListJobs(ctx, gen.ListJobsParams{})
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}
	page, ok := response.(*gen.JobPage)
	if !ok {
		t.Fatalf("ListJobs() response = %T, want *gen.JobPage", response)
	}
	return page
}

func jobStatistics(t *testing.T, handler *Handler, ctx context.Context) *gen.JobStatistics {
	t.Helper()
	response, err := handler.GetJobStatistics(ctx)
	if err != nil {
		t.Fatalf("GetJobStatistics() error = %v", err)
	}
	stats, ok := response.(*gen.JobStatistics)
	if !ok {
		t.Fatalf("GetJobStatistics() response = %T, want *gen.JobStatistics", response)
	}
	return stats
}

func formatJobID(id int64) string {
	return strconv.FormatInt(id, 10)
}

type jobHandlerStorage struct{}

func (jobHandlerStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}

func (jobHandlerStorage) OpenRange(context.Context, telegramstore.RangeRequest) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}

func (jobHandlerStorage) DeleteMessages(context.Context, int64, int64, []int64) error {
	return errors.New("not used")
}

func (jobHandlerStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}

func (jobHandlerStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not used")
}

func (jobHandlerStorage) DeleteChannel(context.Context, int64, int64) error {
	return errors.New("not used")
}
