package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	return river.InsertOpts{Queue: CleanupQueue, MaxAttempts: 10, Priority: 2}
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
	for _, botID := range botIDs {
		row, verifyErr := w.bots.VerifyPending(ctx, job.Args.UserID, botID)
		if verifyErr != nil {
			_ = w.bots.MarkProvisionFailure(ctx, job.Args.UserID, botID, verifyErr)
			return fmt.Errorf("verify pending bot %d: %w", botID, verifyErr)
		}
		username := strings.TrimSpace(row.Username.String)
		for _, channel := range channels {
			if inviteErr := w.inviter.InviteBot(ctx, job.Args.UserID, channel.ChannelID, username); inviteErr != nil {
				_ = w.bots.MarkProvisionFailure(ctx, job.Args.UserID, botID, inviteErr)
				return fmt.Errorf("provision bot %d in channel %d: %w", botID, channel.ChannelID, inviteErr)
			}
		}
		slog.InfoContext(ctx, "Telegram bot provisioned", "job_id", job.ID, "user_id", job.Args.UserID, "bot_id", botID, "bot_username", username, "channel_count", len(channels))
	}
	return nil
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
