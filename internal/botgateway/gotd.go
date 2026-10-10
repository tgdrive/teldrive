package botgateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

type GotdVerifier struct {
	factory *telegramstore.Factory
}

func NewGotdVerifier(factory *telegramstore.Factory) (*GotdVerifier, error) {
	if factory == nil {
		return nil, bots.ErrInvalidInput
	}
	return &GotdVerifier{factory: factory}, nil
}

func (v *GotdVerifier) Verify(ctx context.Context, token string) (bots.Identity, error) {
	memory := &session.StorageMemory{}
	client, err := v.factory.New(memory)
	if err != nil {
		return bots.Identity{}, err
	}
	var identity bots.Identity
	err = client.Run(ctx, func(runCtx context.Context) error {
		if _, err := client.Auth().Bot(runCtx, token); err != nil {
			return err
		}
		self, err := client.Self(runCtx)
		if err != nil {
			return err
		}
		if self == nil || !self.Bot {
			return bots.ErrNotBot
		}
		identity = bots.Identity{ID: self.ID, Username: self.Username}
		return nil
	})
	if err != nil {
		return bots.Identity{}, fmt.Errorf("verify Telegram bot: %w", err)
	}
	return identity, nil
}

type ChannelBotProvider struct {
	queries *sqlcgen.Queries
}

func NewChannelBotProvider(pool *pgxpool.Pool) (*ChannelBotProvider, error) {
	if pool == nil {
		return nil, bots.ErrInvalidInput
	}
	return &ChannelBotProvider{queries: sqlcgen.New(pool)}, nil
}

func (p *ChannelBotProvider) ChannelBots(ctx context.Context, userID int64, api *tg.Client) ([]tg.InputUserClass, error) {
	if p == nil || p.queries == nil || userID <= 0 || api == nil {
		return nil, bots.ErrInvalidInput
	}
	rows, err := p.queries.ListEnabledBots(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list enabled bots: %w", err)
	}
	users := make([]tg.InputUserClass, 0, len(rows))
	for _, row := range rows {
		if !row.Username.Valid || strings.TrimSpace(row.Username.String) == "" {
			continue
		}
		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: strings.TrimPrefix(row.Username.String, "@")})
		if err != nil {
			return nil, fmt.Errorf("resolve bot %d: %w", row.BotID, err)
		}
		found := false
		for _, item := range resolved.Users {
			user, ok := item.(*tg.User)
			if !ok || user.ID != row.BotID || !user.Bot {
				continue
			}
			users = append(users, user.AsInput())
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("resolve bot %d: %w", row.BotID, bots.ErrNotFound)
		}
	}
	return users, nil
}

var (
	_ bots.Verifier             = (*GotdVerifier)(nil)
	_ telegramstore.BotProvider = (*ChannelBotProvider)(nil)
)
