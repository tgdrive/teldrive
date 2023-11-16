package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/models"
	"github.com/divyam234/teldrive/schemas"
	"github.com/divyam234/teldrive/types"
	"github.com/divyam234/teldrive/utils/cache"
	"github.com/divyam234/teldrive/utils/tgc"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
	"github.com/thoas/go-funk"
	"go.etcd.io/bbolt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserService struct {
	Db *gorm.DB
}

type BotInfo struct {
	Id         int64
	UserName   string
	AccessHash int64
	Token      string
}

func (us *UserService) GetProfilePhoto(c *gin.Context) {
	_, session := getUserAuth(c)

	client, err := tgc.UserLogin(session)

	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	err = tgc.RunWithAuth(c, client, "", func(ctx context.Context) error {
		self, err := client.Self(c)
		if err != nil {
			return err
		}
		peer := self.AsInputPeer()
		if self.Photo == nil {
			return nil
		}
		photo, ok := self.Photo.AsNotEmpty()
		if !ok {
			return errors.New("profile not found")
		}
		location := &tg.InputPeerPhotoFileLocation{Big: false, Peer: peer, PhotoID: photo.PhotoID}
		buff, err := iterContent(c, client, location)
		if err != nil {
			return err
		}
		content := buff.Bytes()
		c.Writer.Header().Set("Content-Type", "image/jpeg")
		c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
		c.Writer.Header().Set("Content-Length", strconv.Itoa(len(content)))
		c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", "profile.jpeg"))
		c.Writer.Write(content)
		return nil
	})
	if err != nil {
		c.AbortWithError(http.StatusNotFound, err)
		return
	}
}

func (us *UserService) Stats(c *gin.Context) (*schemas.AccountStats, *types.AppError) {
	userId, _ := getUserAuth(c)
	var res []schemas.AccountStats
	if err := us.Db.Raw("select * from teldrive.account_stats(?);", userId).Scan(&res).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to get stats"), Code: http.StatusInternalServerError}
	}
	return &res[0], nil
}

func (us *UserService) GetBots(c *gin.Context) ([]string, *types.AppError) {
	userID, _ := getUserAuth(c)
	var (
		channelId int64
		err       error
	)
	if c.Param("channelId") != "" {
		channelId, _ = strconv.ParseInt(c.Param("channelId"), 10, 64)
	} else {
		channelId, err = GetDefaultChannel(c, userID)
		if err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
		}
	}

	tokens, err := GetBotsToken(c, userID, channelId)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}
	return tokens, nil
}

func (us *UserService) UpdateChannel(c *gin.Context) (*schemas.Message, *types.AppError) {
	userId, _ := getUserAuth(c)

	var payload schemas.Channel

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid request payload"), Code: http.StatusBadRequest}
	}

	channel := &models.Channel{ChannelID: payload.ChannelID, ChannelName: payload.ChannelName, UserID: userId,
		Selected: true}

	if err := us.Db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"selected": true}),
	}).Create(channel).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to update channel"),
			Code: http.StatusInternalServerError}
	}
	us.Db.Model(&models.Channel{}).Where("channel_id != ?", payload.ChannelID).
		Where("user_id = ?", userId).Update("selected", false)

	key := fmt.Sprintf("users:channel:%d", userId)
	cache.GetCache().Set(key, payload.ChannelID, 0)
	return &schemas.Message{Status: true, Message: "channel updated"}, nil
}

func (us *UserService) ListChannels(c *gin.Context) (interface{}, *types.AppError) {
	_, session := getUserAuth(c)
	client, _ := tgc.UserLogin(session)

	channels := make(map[int64]*schemas.Channel)

	client.Run(c, func(ctx context.Context) error {

		dialogs, _ := query.GetDialogs(client.API()).BatchSize(100).Collect(ctx)

		for _, dialog := range dialogs {
			if !dialog.Deleted() {
				for _, channel := range dialog.Entities.Channels() {
					_, exists := channels[channel.ID]
					if !exists && channel.AdminRights.AddAdmins {
						channels[channel.ID] = &schemas.Channel{ChannelID: channel.ID, ChannelName: channel.Title}
					}
				}
			}
		}
		return nil

	})

	return funk.Values(channels), nil
}

func (us *UserService) AddBots(c *gin.Context) (*schemas.Message, *types.AppError) {
	userId, session := getUserAuth(c)
	client, _ := tgc.UserLogin(session)

	var botsTokens []string

	if err := c.ShouldBindJSON(&botsTokens); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid request payload"), Code: http.StatusBadRequest}
	}

	if len(botsTokens) == 0 {
		return &schemas.Message{Status: true, Message: "no bots to add"}, nil
	}

	channelId, err := GetDefaultChannel(c, userId)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	return us.addBots(c, client, userId, channelId, botsTokens)

}

func (us *UserService) RemoveBots(c *gin.Context) (*schemas.Message, *types.AppError) {
	userID, _ := getUserAuth(c)

	channelId, err := GetDefaultChannel(c, userID)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	if err := us.Db.Where("user_id = ?", userID).Where("channel_id = ?", channelId).
		Delete(&models.Bot{}).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to delete bots"), Code: http.StatusInternalServerError}
	}

	cache.GetCache().Delete(fmt.Sprintf("users:bots:%d:%d", userID, channelId))

	return &schemas.Message{Status: true, Message: "bots deleted"}, nil

}

func (us *UserService) RevokeBotSession(c *gin.Context) (*schemas.Message, *types.AppError) {

	pattern := []byte("botsession:")

	err := database.BoltDB.Update(func(tx *bbolt.Tx) error {

		bucket := tx.Bucket([]byte("teldrive"))
		if bucket == nil {
			return errors.New("bucket not found")
		}

		c := bucket.Cursor()

		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			if bytes.HasPrefix(key, pattern) {
				if err := c.Delete(); err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, &types.AppError{Error: errors.New("failed to revoke session"),
			Code: http.StatusInternalServerError}
	}

	return &schemas.Message{Status: true, Message: "session revoked"}, nil

}

func (us *UserService) addBots(c context.Context, client *telegram.Client, userId int64, channelId int64, botsTokens []string) (*schemas.Message, *types.AppError) {

	botInfo := []BotInfo{}

	var wg sync.WaitGroup

	err := tgc.RunWithAuth(c, client, "", func(ctx context.Context) error {

		channel, err := GetChannelById(ctx, client, channelId, strconv.FormatInt(userId, 10))
		if err != nil {
			return err
		}

		if err != nil {
			return err

		}
		botInfoChannel := make(chan *BotInfo, len(botsTokens))

		waitChan := make(chan struct{}, 6)

		for _, token := range botsTokens {
			waitChan <- struct{}{}
			wg.Add(1)
			go func(t string) {
				info, err := getBotInfo(c, t)
				if err != nil {
					return
				}
				botPeerClass, err := peer.DefaultResolver(client.API()).ResolveDomain(ctx, info.UserName)
				if err != nil {
					return
				}
				botPeer := botPeerClass.(*tg.InputPeerUser)
				info.AccessHash = botPeer.AccessHash
				defer func() {
					<-waitChan
					wg.Done()
				}()
				if err == nil {
					botInfoChannel <- info
				}
			}(token)
		}

		wg.Wait()
		close(botInfoChannel)
		for result := range botInfoChannel {
			botInfo = append(botInfo, *result)
		}

		if len(botsTokens) == len(botInfo) {
			users := funk.Map(botInfo, func(info BotInfo) tg.InputUser {
				return tg.InputUser{UserID: info.Id, AccessHash: info.AccessHash}
			})
			botsToAdd := users.([]tg.InputUser)
			for _, user := range botsToAdd {
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
				client.API().ChannelsEditAdmin(ctx, payload)
			}
		} else {
			return errors.New("failed to fetch bots")
		}
		return nil
	})

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	payload := []models.Bot{}

	for _, info := range botInfo {
		payload = append(payload, models.Bot{UserID: userId, Token: info.Token, BotID: info.Id,
			BotUserName: info.UserName, ChannelID: channelId,
		})
	}

	cache.GetCache().Delete(fmt.Sprintf("users:bots:%d:%d", userId, channelId))

	if err := us.Db.Clauses(clause.OnConflict{DoNothing: true}).Create(&payload).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to add bots"), Code: http.StatusInternalServerError}
	}

	return &schemas.Message{Status: true, Message: "bots added"}, nil

}
