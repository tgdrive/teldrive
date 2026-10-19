//go:build integration

package jobs_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gotd/td/tgerr"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

type provisionVerifier struct{}

func (provisionVerifier) Verify(_ context.Context, token string) (bots.Identity, error) {
	id, err := bots.TokenBotID(token)
	return bots.Identity{ID: id, Username: fmt.Sprintf("bot%d", id)}, err
}

type provisionInviter struct {
	err   error
	calls map[string]int
}

func (i *provisionInviter) InviteBot(_ context.Context, _, _ int64, username string) error {
	i.calls[username]++
	if username == "bot777" {
		return i.err
	}
	return nil
}

func TestBotProvisionFailuresAndRetries(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		t.Run(fmt.Sprintf("fresh=%v", fresh), func(t *testing.T) {
			db := testpostgres.New(t)
			ctx := context.Background()
			if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001); INSERT INTO channels (user_id, channel_id, name) VALUES (1001, 9001, 'storage')"); err != nil {
				t.Fatal(err)
			}
			cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{2}, 32), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			svc, err := bots.NewService(db.Pool, cipher, provisionVerifier{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.InsertPending(ctx, 1001, []string{"777:secret", "778:secret"}); err != nil {
				t.Fatal(err)
			}
			inviter := &provisionInviter{err: errors.New("invite failed"), calls: map[string]int{}}
			if fresh {
				inviter.err = &tgerr.Error{Code: 406, Type: "FRESH_CHANGE_ADMINS_FORBIDDEN"}
			}
			worker := jobs.NewBotProvisionWorker(db.Pool, svc, inviter)
			job := &river.Job[jobs.BotProvisionArgs]{JobRow: &rivertype.JobRow{CreatedAt: time.Now()}, Args: jobs.BotProvisionArgs{UserID: 1001, BotIDs: []int64{777, 778}}}
			err = worker.Work(ctx, job)
			var snooze *river.JobSnoozeError
			if err == nil || errors.As(err, &snooze) != fresh {
				t.Fatalf("Work() = %v", err)
			}
			if fresh && snooze.Duration != time.Hour {
				t.Fatalf("fresh admin retry delay = %v, want one hour", snooze.Duration)
			}
			if inviter.calls["bot778"] != 1 {
				t.Fatal("later bot was not attempted")
			}
			var enabled bool
			if err := db.Pool.QueryRow(ctx, "SELECT enabled FROM bots WHERE bot_id=777").Scan(&enabled); err != nil || enabled {
				t.Fatalf("failed bot enabled=%v, err=%v", enabled, err)
			}
			if fresh {
				job.CreatedAt = time.Now().Add(-47*time.Hour - 30*time.Minute)
				err = worker.Work(ctx, job)
				if !errors.As(err, &snooze) || snooze.Duration > 30*time.Minute || snooze.Duration < 29*time.Minute {
					t.Fatalf("retry exceeds remaining restriction window: %v", err)
				}
				job.CreatedAt = time.Now().Add(-49 * time.Hour)
				err = worker.Work(ctx, job)
				if err == nil || errors.As(err, &snooze) {
					t.Fatalf("expired restriction: %v", err)
				}
			}
			rows, err := svc.InsertPending(ctx, 1001, []string{"777:replacement", "778:secret"})
			if err != nil || len(rows) != 1 || rows[0].BotID != 777 {
				t.Fatalf("requeue rows=%v, err=%v", rows, err)
			}
			inviter.err = nil
			if err := worker.Work(ctx, job); err != nil {
				t.Fatal(err)
			}
			if inviter.calls["bot778"] != 1 {
				t.Fatal("retry invited already enabled bot")
			}
			if err := db.Pool.QueryRow(ctx, "SELECT enabled FROM bots WHERE bot_id=777").Scan(&enabled); err != nil || !enabled {
				t.Fatalf("recovered bot enabled=%v, err=%v", enabled, err)
			}
		})
	}
}

func TestBotReprovisionRepairsEnabledLegacyBot(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001); INSERT INTO channels (user_id, channel_id, name) VALUES (1001, 9001, 'storage')"); err != nil {
		t.Fatal(err)
	}
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{2}, 32), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := bots.NewService(db.Pool, cipher, provisionVerifier{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InsertPending(ctx, 1001, []string{"777:secret"}); err != nil {
		t.Fatal(err)
	}
	// Legacy migration imports enabled bots without usernames.
	if _, err := db.Pool.Exec(ctx, "UPDATE bots SET enabled=true, username=NULL WHERE bot_id=777"); err != nil {
		t.Fatal(err)
	}
	inviter := &provisionInviter{calls: map[string]int{}}
	worker := jobs.NewBotProvisionWorker(db.Pool, svc, inviter)
	job := &river.Job[jobs.BotProvisionArgs]{JobRow: &rivertype.JobRow{CreatedAt: time.Now()}, Args: jobs.BotProvisionArgs{UserID: 1001, BotIDs: []int64{777}}}
	if err := worker.Work(ctx, job); err != nil {
		t.Fatal(err)
	}
	if len(inviter.calls) != 0 {
		t.Fatal("ordinary provisioning should skip an enabled bot")
	}
	job.Args.Force = true
	if err := worker.Work(ctx, job); err != nil {
		t.Fatal(err)
	}
	if inviter.calls["bot777"] != 1 {
		t.Fatalf("manual provisioning invitations = %v", inviter.calls)
	}
	row, err := svc.Get(ctx, 1001, 777)
	if err != nil || !row.Enabled || row.Username.String != "bot777" {
		t.Fatalf("repaired bot=%v, err=%v", row, err)
	}
	job.Args.UserID = 2002
	if err := worker.Work(ctx, job); err == nil {
		t.Fatal("another user's bot must not be provisioned")
	}
	if inviter.calls["bot777"] != 1 {
		t.Fatal("manual provisioning crossed user ownership boundary")
	}
}
