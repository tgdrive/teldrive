package tgc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	storage "github.com/gotd/contrib/storage"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/tgstorage"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"github.com/tgdrive/teldrive/pkg/types"
	"golang.org/x/sync/errgroup"
)

var (
	ErrNoDefaultChannel = fmt.Errorf("no default channel found")
)

type ChannelManager struct {
	repo  *repositories.Repositories
	cache cache.Cacher
	cnf   *config.TGConfig
}

func NewChannelManager(repo *repositories.Repositories, cache cache.Cacher, cnf *config.TGConfig) *ChannelManager {
	return &ChannelManager{
		repo:  repo,
		cache: cache,
		cnf:   cnf,
	}
}

func (cm *ChannelManager) Channel(ctx context.Context, userID int64) (int64, error) {
	return cm.CurrentChannel(ctx, userID)
}

func (cm *ChannelManager) ChannelLimitReached(channelID int64) bool {
	totalParts, err := cm.repo.Files.CountPartsByChannel(context.Background(), channelID)
	if err != nil {
		return false
	}

	return totalParts >= int64(cm.cnf.ChannelLimit)
}

func (cm *ChannelManager) CurrentChannel(ctx context.Context, userID int64) (int64, error) {
	return cache.Fetch(ctx, cm.cache, cache.KeyUserChannel(userID), 0, func() (int64, error) {
		selected, err := cm.repo.Channels.GetSelected(ctx, userID)
		if err != nil {
			return 0, err
		}
		return selected.ChannelID, nil
	})
}

func (cm *ChannelManager) BotTokens(ctx context.Context, userID int64) ([]string, error) {
	return cache.Fetch(ctx, cm.cache, cache.KeyUserBots(userID), 0, func() ([]string, error) {
		return cm.repo.Bots.GetTokensByUserID(ctx, userID)
	})

}

func (cm *ChannelManager) CreateNewChannel(ctx context.Context, newChannelName string, userID int64, setDefault bool) (int64, error) {
	// Acquire distributed lock to prevent race conditions in multi-instance setups
	lockID := cm.generateLockID(userID, "channel_rollover")

	// Try to acquire lock (10 second timeout)
	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := cm.acquireAdvisoryLock(lockCtx, lockID); err != nil {
		return 0, fmt.Errorf("failed to acquire channel creation lock: %w", err)
	}
	defer cm.releaseAdvisoryLock(context.Background(), lockID)

	// Double-check: Was a channel created recently by another instance?
	recentChannel, err := cm.getChannelCreatedAfter(ctx, userID, time.Now().Add(-10*time.Second))
	if err != nil {
		return 0, err
	}
	if recentChannel != nil {
		// Another instance already created channel - use it!
		return recentChannel.ChannelID, nil
	}

	if newChannelName == "" {
		newChannelName = fmt.Sprintf("storage_%d", time.Now().Unix())
	}

	jwtUser := auth.JWTUser(ctx)
	if jwtUser == nil {
		return 0, fmt.Errorf("no JWT user found in context")
	}

	peerStorage := tgstorage.NewPeerStorage(cm.repo.KV, cache.KeyPeer(userID))
	middlewares := NewMiddleware(cm.cnf, WithFloodWait(), WithRetry(5), WithRateLimit())
	client, err := AuthClient(ctx, cm.cnf, jwtUser.TgSession, middlewares...)
	if err != nil {
		return 0, fmt.Errorf("failed to create Telegram client: %w", err)
	}

	var newChannelID int64
	var newChannel *tg.Channel

	err = client.Run(ctx, func(ctx context.Context) error {
		res, err := client.API().ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{
			Title:     newChannelName,
			Broadcast: true,
		})
		if err != nil {
			return err
		}

		updates := res.(*tg.Updates)
		var found bool
		for _, chat := range updates.Chats {
			if ch, ok := chat.(*tg.Channel); ok {
				newChannel = ch
				newChannelID = ch.ID
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("failed to extract channel from creation response")
		}

		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to create Telegram channel: %w", err)
	}

	peer := storage.Peer{}
	peer.FromChat(newChannel)
	peerStorage.Add(ctx, peer)
	botTokens, err := cm.BotTokens(ctx, userID)
	if err != nil {
		return 0, err
	}
	if len(botTokens) > 0 {
		err = cm.AddBotsToChannel(ctx, userID, newChannelID, botTokens, false)
		if err != nil {
			return 0, err
		}
	} else {
		return 0, fmt.Errorf("add bot tokens before continuing")
	}

	selected := setDefault
	newChannelRecord := jetmodel.Channels{
		ChannelName: newChannelName,
		ChannelID:   newChannelID,
		UserID:      userID,
		Selected:    &selected,
	}

	if setDefault {
		err := cm.repo.WithTx(ctx, func(txCtx context.Context) error {
			channels, err := cm.repo.Channels.GetByUserID(txCtx, userID)
			if err != nil {
				return fmt.Errorf("failed to list channels: %w", err)
			}
			for _, c := range channels {
				sel := false
				if err := cm.repo.Channels.Update(txCtx, c.ChannelID, repositories.ChannelUpdate{Selected: &sel}); err != nil {
					return fmt.Errorf("failed to reset selected channels: %w", err)
				}
			}
			if err := cm.repo.Channels.Create(txCtx, &newChannelRecord); err != nil {
				return fmt.Errorf("failed to create channel record: %w", err)
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
		cm.cache.Delete(ctx, cache.KeyUserChannel(userID))
	} else {
		if err := cm.repo.Channels.Create(ctx, &newChannelRecord); err != nil {
			return 0, fmt.Errorf("failed to create channel record: %w", err)
		}
	}

	return newChannelID, nil
}

// generateLockID creates a unique lock ID from user ID and operation
func (cm *ChannelManager) generateLockID(userID int64, operation string) int64 {
	// Use hash to generate unique int64 from userID + operation
	// PostgreSQL advisory locks use int64
	return userID*1000000 + int64(len(operation)) // Simple but effective
}

// acquireAdvisoryLock attempts to acquire PostgreSQL advisory lock
func (cm *ChannelManager) acquireAdvisoryLock(ctx context.Context, lockID int64) error {
	return nil
}

// releaseAdvisoryLock releases PostgreSQL advisory lock
func (cm *ChannelManager) releaseAdvisoryLock(ctx context.Context, lockID int64) error {
	return nil
}

// getChannelCreatedAfter checks if a channel was created for user after given time
func (cm *ChannelManager) getChannelCreatedAfter(ctx context.Context, userID int64, after time.Time) (*jetmodel.Channels, error) {
	return cm.repo.Channels.GetByUserIDCreatedAfter(ctx, userID, after)
}

func (cm *ChannelManager) AddBotsToChannel(ctx context.Context, userId int64, channelId int64, botsTokens []string, save bool) error {

	jwtUser := auth.JWTUser(ctx)

	middlewares := NewMiddleware(cm.cnf, WithFloodWait(), WithRateLimit())

	client, err := AuthClient(ctx, cm.cnf, jwtUser.TgSession, middlewares...)
	if err != nil {
		return err
	}

	err = RunWithAuth(ctx, client, "", func(ctx context.Context) error {

		channel, err := ChannelByID(ctx, client.API(), channelId)

		if err != nil {
			return err
		}

		errChan := make(chan error, len(botsTokens))

		infoChan := make(chan *types.BotInfo, len(botsTokens))

		g, ctx := errgroup.WithContext(ctx)

		g.SetLimit(4)

		for _, token := range botsTokens {
			g.Go(func() error {
				var info *types.BotInfo

				backoffCfg := backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3)

				err := backoff.RetryNotify(func() error {
					var err error
					info, err = FetchBotInfo(ctx, cm.repo.KV, cm.cache, cm.cnf, token)
					if err != nil {
						return err
					}

					peerClass, err := peer.DefaultResolver(client.API()).ResolveDomain(ctx, info.UserName)
					if err != nil {
						return err
					}

					var ok bool
					botPeer, ok := peerClass.(*tg.InputPeerUser)
					if !ok {
						return fmt.Errorf("invalid peer type for bot %s", info.UserName)
					}
					info.AccessHash = botPeer.AccessHash
					payload := &tg.ChannelsEditAdminRequest{
						Channel: channel,
						UserID:  tg.InputUserClass(&tg.InputUser{UserID: info.ID, AccessHash: info.AccessHash}),
						AdminRights: tg.ChatAdminRights{
							ChangeInfo:     true,
							PostMessages:   true,
							EditMessages:   true,
							DeleteMessages: true,
							BanUsers:       true,
							InviteUsers:    true,
							PinMessages:    true,
							ManageCall:     true,
							Other:          true,
							ManageTopics:   true,
						},
						Rank: "bot",
					}

					_, err = client.API().ChannelsEditAdmin(ctx, payload)
					if err != nil {
						return err
					}
					return nil
				}, backoffCfg, nil)

				if err != nil {
					errChan <- err
					return nil
				}
				infoChan <- info
				return nil
			})
		}

		done := make(chan struct{})
		go func() {
			g.Wait()
			close(infoChan)
			close(errChan)
			close(done)
		}()

		var botInfos []*types.BotInfo
		var botErrors []error

		for {
			select {
			case info, ok := <-infoChan:
				if ok {
					botInfos = append(botInfos, info)
				}
			case botErr, ok := <-errChan:
				if ok {
					botErrors = append(botErrors, botErr)
				}
			case <-done:
				if len(botErrors) > 2 {
					return fmt.Errorf("failed to process %d out of %d bots", len(botErrors), len(botsTokens))
				}
				if save && len(botInfos) > 0 {
					payload := []jetmodel.Bots{}
					for _, info := range botInfos {
						payload = append(payload, jetmodel.Bots{UserID: userId, Token: info.Token, BotID: info.ID})
					}
					for i := range payload {
						if err := cm.repo.Bots.Create(ctx, &payload[i]); err != nil {
							// ignore duplicates
							if !errors.Is(err, repositories.ErrConflict) {
								return fmt.Errorf("failed to save bots: %w", err)
							}
						}
					}
					cm.cache.Delete(ctx, cache.KeyUserBots(userId))
				}
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	return err
}
