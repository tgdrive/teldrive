package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/tgerr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

const BotProvisionKind = "teldrive_provision_bots"

var ErrBotProvisionNotConfigured = errors.New("bot provisioning worker is not configured")

type BotProvisionArgs struct {
	UserID int64   `json:"user_id"`
	BotIDs []int64 `json:"bot_ids"`
}

func (BotProvisionArgs) Kind() string { return BotProvisionKind }

func (BotProvisionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: CleanupQueue, MaxAttempts: 3, Priority: 2, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type BotProvisionWorker struct {
	river.WorkerDefaults[BotProvisionArgs]
	queries *sqlcgen.Queries
	bots    *bots.Service
	inviter telegramstore.BotInviter
}

func NewBotProvisionWorker(pool *pgxpool.Pool, botService *bots.Service, inviter telegramstore.BotInviter) *BotProvisionWorker {
	return &BotProvisionWorker{queries: sqlcgen.New(pool), bots: botService, inviter: inviter}
}

func (w *BotProvisionWorker) Timeout(*river.Job[BotProvisionArgs]) time.Duration {
	return 30 * time.Minute
}

func (w *BotProvisionWorker) Work(ctx context.Context, job *river.Job[BotProvisionArgs]) error {
	if w == nil || w.queries == nil || w.bots == nil || w.inviter == nil || job.Args.UserID <= 0 {
		return ErrBotProvisionNotConfigured
	}
	botIDs := normalizedBotIDs(job.Args.BotIDs)
	if len(botIDs) == 0 {
		return nil
	}
	slog.InfoContext(ctx, "Starting bot provisioning job", "job_id", job.ID, "user_id", job.Args.UserID, "bot_count", len(botIDs))
	channels, err := w.queries.ListChannels(ctx, sqlcgen.ListChannelsParams{UserID: job.Args.UserID, PageSize: 200})
	if err != nil {
		return fmt.Errorf("list channels for bot provisioning: %w", err)
	}
	var failures []error
	waiting := false
	for _, botID := range botIDs {
		row, verifyErr := w.bots.VerifyForProvision(ctx, job.Args.UserID, botID)
		if verifyErr != nil {
			failures = append(failures, fmt.Errorf("verify pending bot %d: %w", botID, verifyErr), w.bots.MarkProvisionFailure(ctx, job.Args.UserID, botID, verifyErr))
			continue
		}
		if row.Enabled {
			continue
		}
		username := strings.TrimSpace(row.Username.String)
		var wg sync.WaitGroup
		var inviteErr error
		freshAdminRestriction := false
		var inviteMu sync.Mutex
		sem := make(chan struct{}, 3)
		for _, channel := range channels {
			wg.Go(func() {
				sem <- struct{}{}
				defer func() { <-sem }()

				if err := w.inviter.InviteBot(ctx, job.Args.UserID, channel.ChannelID, username); err != nil {
					inviteMu.Lock()
					freshAdminRestriction = freshAdminRestriction || tgerr.Is(err, "FRESH_CHANGE_ADMINS_FORBIDDEN")
					inviteErr = errors.Join(inviteErr, err)
					inviteMu.Unlock()
				}
			})
		}
		wg.Wait()
		if inviteErr != nil {
			if freshAdminRestriction && time.Since(job.CreatedAt) < 48*time.Hour {
				waiting = true
				continue
			}
			if freshAdminRestriction {
				inviteErr = fmt.Errorf("Telegram admin restriction persisted for 48 hours; add the bot as a channel admin manually: %w", inviteErr)
			}
			failures = append(failures, fmt.Errorf("provision bot %d: %w", botID, inviteErr), w.bots.MarkProvisionFailure(ctx, job.Args.UserID, botID, inviteErr))
			continue
		}
		if _, err := w.bots.Activate(ctx, job.Args.UserID, botID, username); err != nil {
			failures = append(failures, err)
			continue
		}
		slog.InfoContext(ctx, "Telegram bot provisioned", "job_id", job.ID, "user_id", job.Args.UserID, "bot_id", botID, "bot_username", username, "channel_count", len(channels))
	}
	if waiting {
		return river.JobSnooze(min(3*time.Hour, max(time.Until(job.CreatedAt.Add(48*time.Hour)), time.Second)))
	}
	return errors.Join(failures...)
}

func normalizedBotIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
