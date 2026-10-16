package bots

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/cache"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

func (s *Service) ConfigureIdentityCache(c cache.Cacher) *Service {
	s.identityCache = c
	return s
}

func (s *Service) saveIdentity(ctx context.Context, userID int64, identity Identity) (*sqlcgen.Bot, error) {
	if userID <= 0 || identity.ID <= 0 || strings.TrimSpace(identity.Username) == "" {
		return nil, ErrNotBot
	}
	return s.queries.UpdateBotIdentity(ctx, sqlcgen.UpdateBotIdentityParams{
		UserID: userID, BotID: identity.ID,
		Name:     dbtypes.OptionalText(nonEmpty(identity.Name)),
		Username: dbtypes.OptionalText(nonEmpty(identity.Username)),
	})
}

// Recover labels for previously imported bots without changing their enabled state.
// A shared deadline bounds the listing delay, and failed lookups are throttled.
func (s *Service) hydrateIdentities(ctx context.Context, userID int64, rows []*sqlcgen.Bot) {
	if s.identityCache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for index, row := range rows {
		if strings.TrimSpace(row.Name.String) != "" && strings.TrimSpace(row.Username.String) != "" {
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Go(func() {
			defer func() { <-sem }()
			key := cache.Key("bot-identity-refresh", userID, row.BotID)
			var attempted bool
			if s.identityCache.Get(ctx, key, &attempted) == nil && attempted {
				return
			}
			if ctx.Err() != nil {
				return
			}
			_ = s.identityCache.Set(ctx, key, true, time.Minute)
			token, err := s.cipher.Open("bot-token", row.TokenCiphertext)
			if err != nil {
				return
			}
			identity, err := s.verifier.Verify(ctx, string(token))
			if err != nil || identity.ID != row.BotID {
				return
			}
			updated, err := s.saveIdentity(ctx, userID, identity)
			if err == nil {
				rows[index] = updated
			}
		})
	}
	wg.Wait()
}
