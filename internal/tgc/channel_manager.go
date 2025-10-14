package tgc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/contrib/storage"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/tgstorage"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/types"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	rolloverMutexes     = make(map[int64]*sync.Mutex)
	rolloverMutexesLock sync.RWMutex
	ErrNoDefaultChannel = errors.New("no default channel found")
)

type ChannelManager struct {
	db          *gorm.DB
	cache       cache.Cacher
	cnf         *config.TGConfig
	middlewares []telegram.Middleware
}

func NewChannelManager(db *gorm.DB, cache cache.Cacher, cnf *config.TGConfig, middlewares []telegram.Middleware) *ChannelManager {
	return &ChannelManager{
		db:          db,
		cache:       cache,
		cnf:         cnf,
		middlewares: middlewares,
	}
}

func getUserRolloverMutex(userID int64) *sync.Mutex {
	rolloverMutexesLock.RLock()
	mutex, exists := rolloverMutexes[userID]
	rolloverMutexesLock.RUnlock()

	if !exists {
		rolloverMutexesLock.Lock()
		if mutex, exists = rolloverMutexes[userID]; !exists {
			mutex = &sync.Mutex{}
			rolloverMutexes[userID] = mutex
		}
		rolloverMutexesLock.Unlock()
	}

	return mutex
}

func (cm *ChannelManager) GetChannelForUpload(ctx context.Context, userID int64) (int64, error) {

	mutex := getUserRolloverMutex(userID)
	mutex.Lock()
	defer mutex.Unlock()

	currentChannelID, err := cm.CurrentChannel(userID)
	if err != nil && err != ErrNoDefaultChannel {
		return 0, err
	}
	if err == ErrNoDefaultChannel || (cm.isChannelNearLimit(currentChannelID) && cm.cnf.AutoChannelCreate) {
		newChannelID, err := cm.CreateNewChannel(ctx, "", userID, true)
		if err != nil {
			return 0, err
		}
		return newChannelID, nil
	}
	return currentChannelID, nil
}

func (cm *ChannelManager) isChannelNearLimit(channelID int64) bool {
	var totalParts int64

	err := cm.db.Model(&models.File{}).
		Where("channel_id = ?", channelID).
		Select("COALESCE(SUM(CASE WHEN jsonb_typeof(parts) = 'array' THEN jsonb_array_length(parts) ELSE 0 END), 0) as total_parts").
		Scan(&totalParts).Error

	if err != nil {
		return false
	}

	return totalParts >= cm.cnf.ChannelLimit
}

func (cm *ChannelManager) CurrentChannel(userID int64) (int64, error) {
	return cache.Fetch(cm.cache, cache.Key("users", "channel", userID), 0, func() (int64, error) {
		var channelIds []int64
		if err := cm.db.Model(&models.Channel{}).Where("user_id = ?", userID).Where("selected = ?", true).
			Pluck("channel_id", &channelIds).Error; err != nil {
			return 0, err
		}
		if len(channelIds) == 0 {
			return 0, ErrNoDefaultChannel
		}
		return channelIds[0], nil
	})
}

func (cm *ChannelManager) BotTokens(userID int64) ([]string, error) {
	return cache.Fetch(cm.cache, cache.Key("users", "bots", userID), 0, func() ([]string, error) {
		var bots []string
		if err := cm.db.Model(&models.Bot{}).Where("user_id = ?", userID).Pluck("token", &bots).Error; err != nil {
			return nil, err
		}
		return bots, nil
	})

}

func (cm *ChannelManager) CreateNewChannel(ctx context.Context, newChannelName string, userID int64, setDefault bool) (int64, error) {

	if newChannelName == "" {
		newChannelName = fmt.Sprintf("storage_%d", time.Now().Unix())
	}

	jwtUser := auth.GetJWTUser(ctx)
	if jwtUser == nil {
		return 0, fmt.Errorf("no JWT user found in context")
	}

	peerStorage := tgstorage.NewPeerStorage(cm.db, cache.Key("peers", userID))
	client, err := AuthClient(ctx, cm.cnf, jwtUser.TgSession, cm.middlewares...)
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
	botTokens, err := cm.BotTokens(userID)
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

	if setDefault {
		newChannelRecord := models.Channel{
			ChannelId:   newChannelID,
			ChannelName: newChannelName,
			UserId:      userID,
			Selected:    true,
		}

		err = cm.db.Transaction(func(tx *gorm.DB) error {
			err := tx.Model(&models.Channel{}).Where("user_id = ? AND selected = ?", userID, true).
				Update("selected", false).Error
			if err != nil {
				return err
			}
			return tx.Create(&newChannelRecord).Error
		})

		if err != nil {
			return 0, fmt.Errorf("failed to update channel database: %w", err)
		}
		cm.cache.Delete(cache.Key("users", "channel", userID))
	}

	return newChannelID, nil
}

func (cm *ChannelManager) AddBotsToChannel(ctx context.Context, userId int64, channelId int64, botsTokens []string, save bool) error {

	jwtUser := auth.GetJWTUser(ctx)

	client, err := AuthClient(ctx, cm.cnf, jwtUser.TgSession, cm.middlewares...)
	if err != nil {
		return err
	}
	botInfoMap := make(map[string]*types.BotInfo)

	err = RunWithAuth(ctx, client, "", func(ctx context.Context) error {

		channel, err := GetChannelById(ctx, client.API(), channelId)

		if err != nil {
			return err
		}

		g, _ := errgroup.WithContext(ctx)

		g.SetLimit(8)

		mapMu := sync.Mutex{}

		for _, token := range botsTokens {
			g.Go(func() error {
				info, err := GetBotInfo(ctx, cm.db, cm.cache, cm.cnf, token)
				if err != nil {
					return err
				}
				botPeerClass, err := peer.DefaultResolver(client.API()).ResolveDomain(ctx, info.UserName)
				if err != nil {
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
		if len(botsTokens) == len(botInfoMap) {
			users := []tg.InputUser{}
			for _, info := range botInfoMap {
				users = append(users, tg.InputUser{UserID: info.Id, AccessHash: info.AccessHash})
			}
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
				_, err := client.API().ChannelsEditAdmin(ctx, payload)
				if err != nil {
					return err
				}
			}
		} else {
			return errors.New("failed to fetch bots")
		}
		return nil
	})

	if err != nil {
		return err
	}

	if save {
		payload := []models.Bot{}

		for _, info := range botInfoMap {
			payload = append(payload, models.Bot{UserId: userId, Token: info.Token, BotId: info.Id})
		}

		if err := cm.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&payload).Error; err != nil {
			return err
		}
	}
	return nil

}
