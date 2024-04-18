package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/divyam234/teldrive/internal/cache"
	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/kv"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/pkg/logging"
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/schemas"
	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
	"github.com/thoas/go-funk"
	"go.uber.org/zap"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserService struct {
	db  *gorm.DB
	cnf *config.Config
	kv  kv.KV
}

func NewUserService(db *gorm.DB, cnf *config.Config, kv kv.KV) *UserService {
	return &UserService{db: db, cnf: cnf, kv: kv}
}
func (us *UserService) GetProfilePhoto(c *gin.Context) {
	_, session := GetUserAuth(c)

	client, err := tgc.AuthClient(c, &us.cnf.TG, session)

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

func (us *UserService) GetStats(c *gin.Context) (*schemas.AccountStats, *types.AppError) {
	userID, _ := GetUserAuth(c)
	var (
		channelId int64
		err       error
	)
	if c.Param("channelId") != "" {
		channelId, _ = strconv.ParseInt(c.Param("channelId"), 10, 64)
	} else {
		channelId, err = GetDefaultChannel(c, us.db, userID)
		if err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
		}
	}

	tokens, err := getBotsToken(c, us.db, userID, channelId)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}
	return &schemas.AccountStats{Bots: tokens, ChannelID: channelId}, nil
}

func (us *UserService) UpdateChannel(c *gin.Context) (*schemas.Message, *types.AppError) {

	cache := cache.FromContext(c)

	userId, _ := GetUserAuth(c)

	var payload schemas.Channel

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusBadRequest}
	}

	channel := &models.Channel{ChannelID: payload.ChannelID, ChannelName: payload.ChannelName, UserID: userId,
		Selected: true}

	if err := us.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"selected": true}),
	}).Create(channel).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to update channel"),
			Code: http.StatusInternalServerError}
	}
	us.db.Model(&models.Channel{}).Where("channel_id != ?", payload.ChannelID).
		Where("user_id = ?", userId).Update("selected", false)

	key := fmt.Sprintf("users:channel:%d", userId)
	cache.Set(key, payload.ChannelID, 0)
	return &schemas.Message{Message: "channel updated"}, nil
}

func (us *UserService) ListChannels(c *gin.Context) (interface{}, *types.AppError) {
	_, session := GetUserAuth(c)
	client, _ := tgc.AuthClient(c, &us.cnf.TG, session)

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
	userId, session := GetUserAuth(c)
	client, _ := tgc.AuthClient(c, &us.cnf.TG, session)

	var botsTokens []string

	if err := c.ShouldBindJSON(&botsTokens); err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusBadRequest}
	}

	if len(botsTokens) == 0 {
		return &schemas.Message{Message: "no bots to add"}, nil
	}

	channelId, err := GetDefaultChannel(c, us.db, userId)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	return us.addBots(c, client, userId, channelId, botsTokens)

}

func (us *UserService) RemoveBots(c *gin.Context) (*schemas.Message, *types.AppError) {

	cache := cache.FromContext(c)

	userID, _ := GetUserAuth(c)

	channelId, err := GetDefaultChannel(c, us.db, userID)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	if err := us.db.Where("user_id = ?", userID).Where("channel_id = ?", channelId).
		Delete(&models.Bot{}).Error; err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	cache.Delete(fmt.Sprintf("users:bots:%d:%d", userID, channelId))

	return &schemas.Message{Message: "bots deleted"}, nil

}

func (us *UserService) addBots(c context.Context, client *telegram.Client, userId int64, channelId int64, botsTokens []string) (*schemas.Message, *types.AppError) {

	botInfo := []types.BotInfo{}

	var wg sync.WaitGroup

	logger := logging.FromContext(c)

	cache := cache.FromContext(c)

	err := tgc.RunWithAuth(c, client, "", func(ctx context.Context) error {

		channel, err := GetChannelById(ctx, client, channelId, strconv.FormatInt(userId, 10))

		if err != nil {
			logger.Error("error", zap.Error(err))
			return err
		}

		botInfoChannel := make(chan *types.BotInfo, len(botsTokens))

		waitChan := make(chan struct{}, 6)

		for _, token := range botsTokens {
			waitChan <- struct{}{}
			wg.Add(1)
			go func(t string) {
				info, err := getBotInfo(c, us.kv, &us.cnf.TG, t)
				if err != nil {
					return
				}
				botPeerClass, err := peer.DefaultResolver(client.API()).ResolveDomain(ctx, info.UserName)
				if err != nil {
					logger.Error("error", zap.Error(err))
					return
				}
				botPeer := botPeerClass.(*tg.InputPeerUser)
				info.AccessHash = botPeer.AccessHash
				defer func() {
					<-waitChan
					wg.Done()
				}()

				botInfoChannel <- info

			}(token)
		}

		wg.Wait()
		close(botInfoChannel)
		for result := range botInfoChannel {
			botInfo = append(botInfo, *result)
		}

		if len(botsTokens) == len(botInfo) {
			users := funk.Map(botInfo, func(info types.BotInfo) tg.InputUser {
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
				_, err := client.API().ChannelsEditAdmin(ctx, payload)
				if err != nil {
					logger.Error("error", zap.Error(err))
					return err
				}

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

	cache.Delete(fmt.Sprintf("users:bots:%d:%d", userId, channelId))

	if err := us.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&payload).Error; err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	return &schemas.Message{Message: "bots added"}, nil

}
