package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/contrib/storage"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/tgstorage"
	"github.com/tgdrive/teldrive/pkg/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	// Global mutex to prevent concurrent rollover operations per user
	rolloverMutexes     = make(map[int64]*sync.Mutex)
	rolloverMutexesLock sync.RWMutex
)

type ChannelManager struct {
	db          *gorm.DB
	cache       cache.Cacher
	tgdb        *gorm.DB
	cnf         *config.TGConfig
	middlewares []telegram.Middleware
}

func NewChannelManager(db *gorm.DB, cache cache.Cacher, tgdb *gorm.DB, cnf *config.TGConfig, middlewares []telegram.Middleware) *ChannelManager {
	return &ChannelManager{
		db:          db,
		cache:       cache,
		tgdb:        tgdb,
		cnf:         cnf,
		middlewares: middlewares,
	}
}

// getUserRolloverMutex gets or creates a mutex for a specific user to prevent concurrent rollovers
func getUserRolloverMutex(userID int64) *sync.Mutex {
	rolloverMutexesLock.RLock()
	mutex, exists := rolloverMutexes[userID]
	rolloverMutexesLock.RUnlock()

	if !exists {
		rolloverMutexesLock.Lock()
		// Double-check after acquiring write lock
		if mutex, exists = rolloverMutexes[userID]; !exists {
			mutex = &sync.Mutex{}
			rolloverMutexes[userID] = mutex
		}
		rolloverMutexesLock.Unlock()
	}

	return mutex
}

// GetChannelForUpload returns a channel ID suitable for upload
// Creates a new channel if current one is approaching message limit
func (cm *ChannelManager) GetChannelForUpload(ctx context.Context, userID int64) (int64, error) {
	logger := logging.FromContext(ctx)

	// Use per-user mutex to prevent concurrent rollover operations
	mutex := getUserRolloverMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()

	// Get user's current default channel (must be inside mutex)
	currentChannelID, err := cm.getCurrentChannel(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get current channel: %w", err)
	}

	logger.Debug("checking rollover",
		zap.Int64("userID", userID),
		zap.Int64("currentChannelID", currentChannelID))

	// Check if current channel is approaching limit using actual message count
	if cm.isChannelNearLimit(ctx, currentChannelID) {
		logger.Info("channel near limit, attempting to create new channel",
			zap.Int64("channelID", currentChannelID))
		// Create new channel and set as default
		newChannelID, err := cm.createNewChannel(ctx, userID, currentChannelID)
		if err != nil {
			logger.Error("failed to create new channel, continuing with current channel",
				zap.Error(err),
				zap.Int64("currentChannelID", currentChannelID))
			// If channel creation fails, continue with current channel
			return currentChannelID, nil
		}
		logger.Info("successfully created new channel",
			zap.Int64("newChannelID", newChannelID),
			zap.Int64("userID", userID))
		return newChannelID, nil
	}

	logger.Debug("channel within limits, using for upload",
		zap.Int64("channelID", currentChannelID))
	return currentChannelID, nil
}

// isChannelNearLimit checks if channel is approaching message limit using actual message count
func (cm *ChannelManager) isChannelNearLimit(ctx context.Context, channelID int64) bool {
	logger := logging.FromContext(ctx)

	// Get JWT user for Telegram session
	jwtUser := auth.GetJWTUser(ctx)
	if jwtUser == nil {
		logger.Error("no JWT user found in context for channel limit check")
		return false
	}

	// Create Telegram client to get actual message count
	client, err := tgc.AuthClient(ctx, cm.cnf, jwtUser.TgSession, cm.middlewares...)
	if err != nil {
		logger.Error("failed to create Telegram client for channel limit check", zap.Error(err))
		return false
	}

	var totalMessages int
	err = client.Run(ctx, func(ctx context.Context) error {
		channel, err := tgc.GetChannelById(ctx, client.API(), channelID)
		if err != nil {
			return fmt.Errorf("failed to get channel: %w", err)
		}

		q := query.NewQuery(client.API()).Messages().GetHistory(&tg.InputPeerChannel{
			ChannelID:  channelID,
			AccessHash: channel.AccessHash,
		})

		msgiter := messages.NewIterator(q, 100)
		total, err := msgiter.Total(ctx)
		if err != nil {
			return fmt.Errorf("failed to get total messages: %w", err)
		}

		totalMessages = total
		return nil
	})

	if err != nil {
		logger.Error("error checking channel message limit",
			zap.Int64("channelID", channelID),
			zap.Error(err))
		// On error, assume not near limit to avoid disruption
		return false
	}

	nearLimit := int64(totalMessages) >= cm.cnf.MessageLimit
	logger.Debug("channel limit check",
		zap.Int64("channelID", channelID),
		zap.Int("totalMessages", totalMessages),
		zap.Int64("messageLimit", cm.cnf.MessageLimit),
		zap.Bool("nearLimit", nearLimit))

	return nearLimit
}

// getCurrentChannel gets user's current default channel
func (cm *ChannelManager) getCurrentChannel(userID int64) (int64, error) {
	var channelIds []int64
	err := cm.db.Model(&models.Channel{}).Where("user_id = ?", userID).Where("selected = ?", true).
		Pluck("channel_id", &channelIds).Error
	if err != nil {
		return 0, err
	}
	if len(channelIds) == 0 {
		return 0, fmt.Errorf("no default channel found for user %d", userID)
	}
	return channelIds[0], nil
}

// createNewChannel creates a new Telegram channel and sets it as user's default
func (cm *ChannelManager) createNewChannel(ctx context.Context, userID, currentChannelID int64) (int64, error) {
	logger := logging.FromContext(ctx)

	// Get current channel name to create a rollover name
	var currentChannel models.Channel
	err := cm.db.Where("channel_id = ? AND user_id = ?", currentChannelID, userID).First(&currentChannel).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get current channel info: %w", err)
	}

	// Generate rollover channel name
	newChannelName := fmt.Sprintf("%s_rollover_%d", currentChannel.ChannelName, time.Now().Unix())

	// Get JWT user for Telegram session
	jwtUser := auth.GetJWTUser(ctx)
	if jwtUser == nil {
		return 0, fmt.Errorf("no JWT user found in context")
	}

	// Create Telegram client
	peerStorage := tgstorage.NewPeerStorage(cm.tgdb, cache.Key("peers", userID))
	client, err := tgc.AuthClient(ctx, cm.cnf, jwtUser.TgSession, cm.middlewares...)
	if err != nil {
		return 0, fmt.Errorf("failed to create Telegram client: %w", err)
	}

	var newChannelID int64
	var newChannel *tg.Channel

	// Create the new channel
	err = client.Run(ctx, func(ctx context.Context) error {
		res, err := client.API().ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{
			Title:     newChannelName,
			Broadcast: true,
		})
		if err != nil {
			return err
		}

		// Extract channel from response
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

	// Store channel in peer storage
	peer := storage.Peer{}
	peer.FromChat(newChannel)
	peerStorage.Add(ctx, peer)

	// Get bots from current channel to copy to new channel
	var currentBots []models.Bot
	err = cm.db.Where("channel_id = ? AND user_id = ?", currentChannelID, userID).Find(&currentBots).Error
	if err != nil {
		logger.Warn("failed to get bots from current channel",
			zap.Int64("currentChannelID", currentChannelID),
			zap.Error(err))
	} else if len(currentBots) > 0 {
		logger.Info("found bots in current channel, copying to new channel",
			zap.Int("botCount", len(currentBots)),
			zap.Int64("currentChannelID", currentChannelID),
			zap.Int64("newChannelID", newChannelID))

		// Extract bot tokens
		botTokens := make([]string, len(currentBots))
		for i, bot := range currentBots {
			botTokens[i] = bot.Token
		}

		// Copy bots to new channel
		err = cm.addBotsToChannel(ctx, userID, newChannelID, botTokens)
		if err != nil {
			logger.Warn("failed to copy bots to new channel",
				zap.Int64("newChannelID", newChannelID),
				zap.Error(err))
			// Don't fail the whole operation
		}
	} else {
		logger.Debug("no bots found in current channel",
			zap.Int64("currentChannelID", currentChannelID))
	}

	// Add new channel to database
	newChannelRecord := models.Channel{
		ChannelId:   newChannelID,
		ChannelName: newChannelName,
		UserId:      userID,
		Selected:    true, // Make this the new default channel
	}

	// Transaction to update channel selection
	err = cm.db.Transaction(func(tx *gorm.DB) error {
		// Unset current default channel
		err := tx.Model(&models.Channel{}).Where("user_id = ? AND selected = ?", userID, true).
			Update("selected", false).Error
		if err != nil {
			return err
		}

		// Add new channel as default
		return tx.Create(&newChannelRecord).Error
	})

	if err != nil {
		return 0, fmt.Errorf("failed to update channel database: %w", err)
	}

	// Clear cache for user's bots and channels
	cm.cache.Delete(cache.Key("users", "channel", userID))
	cm.cache.Delete(cache.Key("users", "bots", userID, currentChannelID))

	logger.Info("successfully created rollover channel",
		zap.Int64("newChannelID", newChannelID),
		zap.String("newChannelName", newChannelName),
		zap.Int64("userID", userID))
	return newChannelID, nil
}

// addBotsToChannel adds bots to both the Telegram channel and database
// Uses the same logic as the UI's UsersAddBots function
func (cm *ChannelManager) addBotsToChannel(ctx context.Context, userID, channelID int64, botTokens []string) error {
	// Get JWT user for creating a fresh Telegram client (same as UI)
	jwtUser := auth.GetJWTUser(ctx)
	if jwtUser == nil {
		return fmt.Errorf("no JWT user found in context")
	}

	// Create a fresh Telegram client specifically for bot operations (same as UI)
	client, err := tgc.AuthClient(ctx, cm.cnf, jwtUser.TgSession, cm.middlewares...)
	if err != nil {
		return fmt.Errorf("failed to create Telegram client for bot operations: %w", err)
	}

	// Use the shared addBotsToChannel function
	return addBotsToChannel(ctx, cm.db, cm.tgdb, cm.cache, cm.cnf, client, userID, channelID, botTokens)
}
