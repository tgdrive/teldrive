package botgateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

// ExistingChannelProvisioner ensures a bot cannot become upload-eligible until
// it has been invited to every channel currently registered for the user.
type ExistingChannelProvisioner struct {
	queries *sqlcgen.Queries
	storage telegramstore.BotInviter
}

func NewExistingChannelProvisioner(pool *pgxpool.Pool, storage telegramstore.BotInviter) (*ExistingChannelProvisioner, error) {
	if pool == nil || storage == nil {
		return nil, bots.ErrInvalidInput
	}
	return &ExistingChannelProvisioner{queries: sqlcgen.New(pool), storage: storage}, nil
}

func (p *ExistingChannelProvisioner) ProvisionBot(ctx context.Context, userID int64, identity bots.Identity) error {
	if p == nil || p.queries == nil || p.storage == nil || userID <= 0 || identity.ID <= 0 || strings.TrimSpace(identity.Username) == "" {
		return bots.ErrInvalidInput
	}
	channels, err := p.queries.ListChannels(ctx, sqlcgen.ListChannelsParams{UserID: userID, PageSize: 200})
	if err != nil {
		return fmt.Errorf("list channels for bot provisioning: %w", err)
	}
	for _, channel := range channels {
		if err := p.storage.InviteBot(ctx, userID, channel.ChannelID, identity.Username); err != nil {
			return fmt.Errorf("provision bot %d in channel %d: %w", identity.ID, channel.ChannelID, err)
		}
	}
	return nil
}

var _ bots.Provisioner = (*ExistingChannelProvisioner)(nil)
