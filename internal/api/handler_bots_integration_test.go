//go:build integration

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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

func TestProvisionAllBotsIncludesEveryOwnedBotAndReusesSingleTasks(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001), (2002), (3003)"); err != nil {
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
	const botCount = 205 // More than the UI's first page.
	tokens := make([]string, botCount)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("%d:secret", 777+i)
	}
	if _, err := svc.InsertPending(ctx, 1001, tokens); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InsertPending(ctx, 2002, []string{"9999:other-user"}); err != nil {
		t.Fatal(err)
	}
	// Enabled and disabled bots both need to be included in manual provisioning.
	if _, err := db.Pool.Exec(ctx, "UPDATE bots SET enabled=true WHERE user_id=1001 AND bot_id % 2 = 0"); err != nil {
		t.Fatal(err)
	}
	runtime, err := jobs.NewRuntimeWithSchemaAndBotProvision(db.Pool, manualProvisionStorage{}, "teldrive", svc, cipher, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{Bots: svc, Jobs: runtime}
	if _, err := handler.ProvisionBots(ctx, gen.ProvisionBotsParams{}); err == nil {
		t.Fatal("unauthenticated bulk provisioning accepted")
	}
	emptyCtx := principal.WithIdentity(ctx, principal.Identity{UserID: 3003})
	empty, err := handler.ProvisionBots(emptyCtx, gen.ProvisionBotsParams{})
	if err != nil || len(empty.(*gen.BotBulkProvisionResponse).JobIds) != 0 {
		t.Fatalf("empty user's bulk provisioning=%v, err=%v", empty, err)
	}
	ownerCtx := principal.WithIdentity(ctx, principal.Identity{UserID: 1001})
	single, err := handler.ProvisionBot(ownerCtx, gen.ProvisionBotParams{BotId: 777})
	if err != nil {
		t.Fatal(err)
	}
	response, err := handler.ProvisionBots(ownerCtx, gen.ProvisionBotsParams{})
	if err != nil {
		t.Fatal(err)
	}
	result := response.(*gen.BotBulkProvisionResponse)
	if len(result.JobIds) != botCount || result.JobIds[0] != single.(*gen.BotProvisionResponse).JobId {
		t.Fatalf("bulk job count=%d, single task not reused=%v", len(result.JobIds), result.JobIds)
	}
	again, err := handler.ProvisionBots(ownerCtx, gen.ProvisionBotsParams{})
	if err != nil || !reflect.DeepEqual(again.(*gen.BotBulkProvisionResponse).JobIds, result.JobIds) {
		t.Fatalf("bulk request duplicated active tasks: result=%v, err=%v", again, err)
	}
	rows, err := db.Pool.Query(ctx, "SELECT args FROM river_job ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[int64]bool{}
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			t.Fatal(err)
		}
		plain, err := riverencrypt.DecryptArgs(cipher, encoded)
		if err != nil {
			t.Fatal(err)
		}
		var args jobs.BotProvisionArgs
		if err := json.Unmarshal(plain, &args); err != nil {
			t.Fatal(err)
		}
		if args.UserID != 1001 || !args.Force || len(args.BotIDs) != 1 {
			t.Fatalf("bulk provisioning is not one owned bot per task: %+v", args)
		}
		seen[args.BotIDs[0]] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(seen) != botCount || seen[9999] {
		t.Fatalf("wrong bots queued: count=%d, includes another user=%v", len(seen), seen[9999])
	}
	if stats := jobStatistics(t, handler, ownerCtx); stats.Available != botCount {
		t.Fatalf("bulk provisioning ownership statistics=%+v", stats)
	}
}

func TestCreateBotsQueuesIndependentTasks(t *testing.T) {
	db := testpostgres.New(t)
	ctx := principal.WithIdentity(context.Background(), principal.Identity{UserID: 1001})
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
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
	runtime, err := jobs.NewRuntimeWithSchemaAndBotProvision(db.Pool, manualProvisionStorage{}, "teldrive", svc, cipher, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{Bots: svc, Jobs: runtime}
	req := &gen.BotCreateRequest{Tokens: []string{"777:secret", "778:secret", "invalid", "777:duplicate"}}
	// Reject the second task to verify a failed batch doesn't leave the first queued.
	if _, err := db.Pool.Exec(ctx, "ALTER TABLE river_job ADD CONSTRAINT reject_even_job_ids CHECK (id % 2 = 1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.CreateBots(ctx, req, gen.CreateBotsParams{}); err == nil {
		t.Fatal("expected second task insert to fail")
	}
	var queued int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("failed batch left queued tasks: count=%d, err=%v", queued, err)
	}
	if _, err := db.Pool.Exec(ctx, "ALTER TABLE river_job DROP CONSTRAINT reject_even_job_ids"); err != nil {
		t.Fatal(err)
	}
	response, err := handler.CreateBots(ctx, req, gen.CreateBotsParams{})
	if err != nil {
		t.Fatal(err)
	}
	result := response.(*gen.BotCreateResponse)
	if len(result.Bots) != 2 || len(result.JobIds) != 2 || result.JobId.Or("") != result.JobIds[0] || !reflect.DeepEqual(result.FailedIndexes, []int32{2, 3}) {
		t.Fatalf("create bots result=%+v", result)
	}
	for i, jobID := range result.JobIds {
		var encoded []byte
		if err := db.Pool.QueryRow(ctx, "SELECT args FROM river_job WHERE id=$1", jobID).Scan(&encoded); err != nil {
			t.Fatal(err)
		}
		plain, err := riverencrypt.DecryptArgs(cipher, encoded)
		if err != nil {
			t.Fatal(err)
		}
		var args jobs.BotProvisionArgs
		if err := json.Unmarshal(plain, &args); err != nil {
			t.Fatal(err)
		}
		if args.UserID != 1001 || args.Force || len(args.BotIDs) != 1 || args.BotIDs[0] != result.Bots[i].ID {
			t.Fatalf("create job does not contain one pending bot: %+v", args)
		}
	}
}
