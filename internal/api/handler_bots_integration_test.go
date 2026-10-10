//go:build integration

package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/divyam234/riverpro/riverencrypt"
	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
	"github.com/tgdrive/teldrive/v2/internal/principal"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

type manualProvisionStorage struct{ jobHandlerStorage }

func (manualProvisionStorage) InviteBot(context.Context, int64, int64, string) error {
	return nil
}

type manualProvisionVerifier struct{}

func (manualProvisionVerifier) Verify(context.Context, string) (bots.Identity, error) {
	return bots.Identity{ID: 777, Username: "storage_bot"}, nil
}

func TestProvisionBotRequiresOwnershipAndQueuesManualJob(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001), (2002)"); err != nil {
		t.Fatal(err)
	}
	cipher, err := secureblob.New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := bots.NewService(db.Pool, cipher, manualProvisionVerifier{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InsertPending(ctx, 1001, []string{"777:secret"}); err != nil {
		t.Fatal(err)
	}
	runtime, err := jobs.NewRuntimeWithSchemaAndBotProvision(db.Pool, manualProvisionStorage{}, "teldrive", svc, cipher, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{Bots: svc, Jobs: runtime}
	params := gen.ProvisionBotParams{BotId: 777}
	if _, err := handler.ProvisionBot(ctx, params); err == nil {
		t.Fatal("unauthenticated provisioning accepted")
	}
	otherCtx := principal.WithIdentity(ctx, principal.Identity{UserID: 2002})
	if _, err := handler.ProvisionBot(otherCtx, params); err == nil {
		t.Fatal("another user's bot accepted")
	}
	var count int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&count); err != nil || count != 0 {
		t.Fatalf("unauthorized requests queued jobs: count=%d, err=%v", count, err)
	}
	ownerCtx := principal.WithIdentity(ctx, principal.Identity{UserID: 1001})
	response, err := handler.ProvisionBot(ownerCtx, params)
	if err != nil {
		t.Fatal(err)
	}
	result := response.(*gen.BotProvisionResponse)
	if result.JobId == "" {
		t.Fatal("missing provisioning job ID")
	}
	// A repeated request should reuse the active job rather than queue a duplicate.
	again, err := handler.ProvisionBot(ownerCtx, params)
	if err != nil || again.(*gen.BotProvisionResponse).JobId != result.JobId {
		t.Fatalf("duplicate provisioning result=%v, err=%v", again, err)
	}
	var kind string
	var args []byte
	if err := db.Pool.QueryRow(ctx, "SELECT kind, args FROM river_job WHERE id=$1", result.JobId).Scan(&kind, &args); err != nil {
		t.Fatal(err)
	}
	if kind != jobs.BotProvisionKind {
		t.Fatalf("queued kind=%q", kind)
	}
	plain, err := riverencrypt.DecryptArgs(cipher, args)
	if err != nil {
		t.Fatal(err)
	}
	var provisionArgs jobs.BotProvisionArgs
	if err := json.Unmarshal(plain, &provisionArgs); err != nil {
		t.Fatal(err)
	}
	if !provisionArgs.Force || provisionArgs.UserID != 1001 || len(provisionArgs.BotIDs) != 1 || provisionArgs.BotIDs[0] != 777 {
		t.Fatalf("manual job arguments=%+v", provisionArgs)
	}
	page := listJobsPage(t, handler, ownerCtx)
	if len(page.Tasks) != 1 || page.Tasks[0].ID != result.JobId {
		t.Fatalf("owner cannot track provisioning: %+v", page.Tasks)
	}
	if page := listJobsPage(t, handler, otherCtx); len(page.Tasks) != 0 {
		t.Fatalf("provisioning leaked to another user: %+v", page.Tasks)
	}
	if _, err := handler.GetJob(ownerCtx, gen.GetJobParams{JobId: result.JobId}); err != nil {
		t.Fatalf("owner cannot read provisioning task: %v", err)
	}
	if _, err := handler.GetJob(otherCtx, gen.GetJobParams{JobId: result.JobId}); err == nil {
		t.Fatal("another user can read provisioning task")
	}
	if stats := jobStatistics(t, handler, ownerCtx); stats.Available != 1 {
		t.Fatalf("provisioning statistics=%+v", stats)
	}
}
