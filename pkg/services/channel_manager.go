package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gotd/contrib/storage"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/tgstorage"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/types"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	// Use per-user mutex to prevent concurrent rollover operations
	mutex := getUserRolloverMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()

	// Get user's current default channel (must be inside mutex)
	currentChannelID, err := cm.getCurrentChannel(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get current channel: %w", err)
	}

	log.Printf("Checking rollover for user %d, current channel %d", userID, currentChannelID)

	// Check if current channel is approaching limit using Part IDs
	if cm.isChannelNearLimit(currentChannelID) {
		log.Printf("Channel %d is near limit, attempting to create new channel", currentChannelID)
		// Create new channel and set as default
		newChannelID, err := cm.createNewChannel(ctx, userID, currentChannelID)
		if err != nil {
			log.Printf("Failed to create new channel: %v, continuing with current channel %d", err, currentChannelID)
			// If channel creation fails, continue with current channel
			return currentChannelID, nil
		}
		log.Printf("Successfully created new channel %d for user %d", newChannelID, userID)
		return newChannelID, nil
	}

	log.Printf("Channel %d is within limits, using for upload", currentChannelID)
	return currentChannelID, nil
}

// isChannelNearLimit checks if channel is approaching message limit using Part IDs
func (cm *ChannelManager) isChannelNearLimit(channelID int64) bool {
	var maxPartID int64

	// Query the highest Part ID (which IS the Telegram message ID) for this channel
	err := cm.db.Raw(`
		SELECT COALESCE(MAX((part_data->>'id')::bigint), 0) as max_id
		FROM (
			SELECT jsonb_array_elements(parts) as part_data
			FROM teldrive.files 
			WHERE channel_id = ?
		) parts_expanded
	`, channelID).Scan(&maxPartID).Error

	if err != nil {
		log.Printf("Error checking channel limit for channel %d: %v", channelID, err)
		// On error, assume not near limit to avoid disruption
		return false
	}

	log.Printf("Channel %d: highest message ID = %d, limit = %d, near limit = %t",
		channelID, maxPartID, cm.cnf.MessageLimit, maxPartID >= cm.cnf.MessageLimit)

	return maxPartID >= cm.cnf.MessageLimit
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
		log.Printf("Warning: failed to get bots from current channel %d: %v", currentChannelID, err)
	} else if len(currentBots) > 0 {
		log.Printf("Found %d bots in current channel %d, copying to new channel %d", len(currentBots), currentChannelID, newChannelID)

		// Extract bot tokens
		botTokens := make([]string, len(currentBots))
		for i, bot := range currentBots {
			botTokens[i] = bot.Token
		}

		// Copy bots to new channel
		err = cm.addBotsToChannel(ctx, userID, newChannelID, botTokens)
		if err != nil {
			log.Printf("Warning: failed to copy bots to new channel %d: %v", newChannelID, err)
			// Don't fail the whole operation
		}
	} else {
		log.Printf("No bots found in current channel %d", currentChannelID)
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

	log.Printf("Successfully created rollover channel %d (%s) for user %d", newChannelID, newChannelName, userID)
	return newChannelID, nil
}

// addBotsToChannel adds bots to both the Telegram channel and database
// Uses the same logic as the UI's UsersAddBots function
func (cm *ChannelManager) addBotsToChannel(ctx context.Context, userID, channelID int64, botTokens []string) error {
	if len(botTokens) == 0 {
		return nil
	}

	log.Printf("Adding %d bots to channel %d", len(botTokens), channelID)

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

	// Use the exact same logic as the UI's addBots function
	botInfoMap := make(map[string]*types.BotInfo)

	err = tgc.RunWithAuth(ctx, client, "", func(botCtx context.Context) error {
		channel, err := tgc.GetChannelById(botCtx, client.API(), channelID)
		if err != nil {
			return fmt.Errorf("failed to get channel: %w", err)
		}

		g, _ := errgroup.WithContext(botCtx)
		g.SetLimit(8)
		mapMu := sync.Mutex{}

		// Fetch bot info in parallel (same as UI)
		for _, token := range botTokens {
			token := token // capture loop variable
			g.Go(func() error {
				info, err := tgc.GetBotInfo(ctx, cm.tgdb, cm.cnf, token)
				if err != nil {
					log.Printf("Warning: failed to get bot info for token %s: %v", token, err)
					return err
				}

				// Resolve bot domain to get access hash (same as UI)
				botPeerClass, err := peer.DefaultResolver(client.API()).ResolveDomain(botCtx, info.UserName)
				if err != nil {
					log.Printf("Warning: failed to resolve bot domain for %s: %v", info.UserName, err)
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

		// Only proceed if we got info for all bots (same validation as UI)
		if len(botTokens) == len(botInfoMap) {
			users := []tg.InputUser{}
			for _, info := range botInfoMap {
				users = append(users, tg.InputUser{UserID: info.Id, AccessHash: info.AccessHash})
			}

			// Add each bot as admin to the channel (same as UI)
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
					log.Printf("Warning: failed to add bot as admin to channel %d: %v", channelID, err)
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

	// Save bots to database (same as UI)
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

	// Clear bot cache for this channel (same as UI)
	cm.cache.Delete(cache.Key("users", "bots", userID, channelID))

	// Insert bots with conflict handling (same as UI)
	if err := cm.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&payload).Error; err != nil {
		log.Printf("Warning: failed to save bots to database: %v", err)
		return fmt.Errorf("failed to save bots to database: %w", err)
	}

	log.Printf("Successfully added %d bots to channel %d", len(botInfoMap), channelID)
	return nil
}
