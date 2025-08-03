package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func getParts(ctx context.Context, client *telegram.Client, c cache.Cacher, file *models.File) ([]types.Part, error) {
	return cache.Fetch(c, cache.Key("files", "messages", file.ID), 60*time.Minute, func() ([]types.Part, error) {
		messages, err := tgc.GetMessages(ctx, client.API(), utils.Map(file.Parts, func(part api.Part) int {
			return part.ID
		}), *file.ChannelId)

		if err != nil {
			return nil, err
		}
		parts := []types.Part{}
		for i, message := range messages {
			switch item := message.(type) {
			case *tg.Message:
				media, ok := item.Media.(*tg.MessageMediaDocument)
				if !ok {
					continue
				}
				document, ok := media.Document.(*tg.Document)
				if !ok {
					continue
				}
				part := types.Part{
					ID:   int64(file.Parts[i].ID),
					Size: document.Size,
					Salt: file.Parts[i].Salt.Value,
				}
				if *file.Encrypted {
					part.DecryptedSize, _ = crypt.DecryptedSize(document.Size)
				}
				parts = append(parts, part)
			}
		}
		if len(parts) != len(file.Parts) {
			msg := "file parts mismatch"
			logging.FromContext(ctx).Error(msg, zap.String("name", file.Name),
				zap.Int("expected", len(file.Parts)), zap.Int("actual", len(parts)))
			return nil, errors.New(msg)
		}
		return parts, nil
	})
}

func getDefaultChannel(db *gorm.DB, c cache.Cacher, userId int64) (int64, error) {
	return cache.Fetch(c, cache.Key("users", "channel", userId), 0, func() (int64, error) {
		var channelIds []int64
		if err := db.Model(&models.Channel{}).Where("user_id = ?", userId).Where("selected = ?", true).
			Pluck("channel_id", &channelIds).Error; err != nil {
			return 0, err
		}
		if len(channelIds) == 0 {
			return 0, fmt.Errorf("no default channel found for user %d", userId)
		}
		return channelIds[0], nil
	})
}

func getBotsToken(db *gorm.DB, c cache.Cacher, userId, channelId int64) ([]string, error) {
	return cache.Fetch(c, cache.Key("users", "bots", userId, channelId), 0, func() ([]string, error) {
		var bots []string
		if err := db.Model(&models.Bot{}).Where("user_id = ?", userId).
			Where("channel_id = ?", channelId).Pluck("token", &bots).Error; err != nil {
			return nil, err
		}
		return bots, nil
	})
}

// addBotsToChannel adds bots to both the Telegram channel and database
// This is a common function used by both the UI and rollover system
func addBotsToChannel(ctx context.Context, db *gorm.DB, tgdb *gorm.DB, cacher cache.Cacher, cnf *config.TGConfig,
	client *telegram.Client, userID, channelID int64, botTokens []string) error {

	logger := logging.FromContext(ctx)

	if len(botTokens) == 0 {
		return nil
	}

	logger.Debug("adding bots to channel",
		zap.Int("botCount", len(botTokens)),
		zap.Int64("channelID", channelID))

	botInfoMap := make(map[string]*types.BotInfo)

	err := tgc.RunWithAuth(ctx, client, "", func(botCtx context.Context) error {
		channel, err := tgc.GetChannelById(botCtx, client.API(), channelID)
		if err != nil {
			return fmt.Errorf("failed to get channel: %w", err)
		}

		g, _ := errgroup.WithContext(botCtx)
		g.SetLimit(8)
		mapMu := sync.Mutex{}

		// Fetch bot info in parallel
		for _, token := range botTokens {
			token := token // capture loop variable
			g.Go(func() error {
				info, err := tgc.GetBotInfo(ctx, tgdb, cnf, token)
				if err != nil {
					logger.Warn("failed to get bot info",
						zap.String("token", token),
						zap.Error(err))
					return err
				}

				// Resolve bot domain to get access hash
				botPeerClass, err := peer.DefaultResolver(client.API()).ResolveDomain(botCtx, info.UserName)
				if err != nil {
					logger.Warn("failed to resolve bot domain",
						zap.String("userName", info.UserName),
						zap.Error(err))
					return err
				}

				botPeer := botPeerClass.(*tg.InputPeerUser)
				info.AccessHash = botPeer.AccessHash

				mapMu.Lock()
				botInfoMap[token] = info
				mapMu.Unlock()
				return nil
			})
		}

		if err = g.Wait(); err != nil {
			return err
		}

		// Only proceed if we got info for all bots
		if len(botTokens) == len(botInfoMap) {
			users := []tg.InputUser{}
			for _, info := range botInfoMap {
				users = append(users, tg.InputUser{UserID: info.Id, AccessHash: info.AccessHash})
			}

			// Add each bot as admin to the channel
			for _, user := range users {
				payload := &tg.ChannelsEditAdminRequest{
					Channel: channel,
					UserID:  tg.InputUserClass(&user),
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
				_, err := client.API().ChannelsEditAdmin(botCtx, payload)
				if err != nil {
					logger.Warn("failed to add bot as admin to channel",
						zap.Int64("channelID", channelID),
						zap.Error(err))
					return err
				}
			}
		} else {
			return fmt.Errorf("failed to fetch info for all bots: got %d out of %d", len(botInfoMap), len(botTokens))
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to add bots to Telegram channel: %w", err)
	}

	// Save bots to database
	payload := []models.Bot{}
	for _, info := range botInfoMap {
		payload = append(payload, models.Bot{
			UserId:      userID,
			Token:       info.Token,
			BotId:       info.Id,
			BotUserName: info.UserName,
			ChannelId:   channelID,
		})
	}

	// Clear bot cache for this channel
	cacher.Delete(cache.Key("users", "bots", userID, channelID))

	// Insert bots with conflict handling
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&payload).Error; err != nil {
		logger.Warn("failed to save bots to database", zap.Error(err))
		return fmt.Errorf("failed to save bots to database: %w", err)
	}

	logger.Info("successfully added bots to channel",
		zap.Int("botCount", len(botInfoMap)),
		zap.Int64("channelID", channelID))
	return nil
}
