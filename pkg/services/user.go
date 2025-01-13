package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/types"
	"golang.org/x/sync/errgroup"

	tgbbolt "github.com/gotd/contrib/bbolt"
	"github.com/gotd/contrib/storage"
	"gorm.io/gorm/clause"
)

func (a *apiService) UsersAddBots(ctx context.Context, req *api.AddBots) error {
	userId := auth.GetUser(ctx)
	client, _ := tgc.AuthClient(ctx, &a.cnf.TG, auth.GetJWTUser(ctx).TgSession, a.middlewares...)

	if len(req.Bots) > 0 {
		channelId, err := getDefaultChannel(a.db, a.cache, userId)

		if err != nil {
			return &apiError{err: err}
		}
		err = a.addBots(ctx, client, userId, channelId, req.Bots)
		if err != nil {
			return &apiError{err: err}
		}
	}
	return nil

}

func (a *apiService) UsersListChannels(ctx context.Context) ([]api.Channel, error) {

	userId := auth.GetUser(ctx)

	channels := make(map[int64]*api.Channel)

	peerStorage := tgbbolt.NewPeerStorage(a.boltdb, []byte(fmt.Sprintf("peers:%d", userId)))

	iter, err := peerStorage.Iterate(ctx)
	if err != nil {
		return []api.Channel{}, nil
	}
	for iter.Next(ctx) {
		peer := iter.Value()
		if peer.Channel != nil && peer.Channel.AdminRights.AddAdmins {
			_, exists := channels[peer.Channel.ID]
			if !exists {
				channels[peer.Channel.ID] = &api.Channel{ChannelId: peer.Channel.ID, ChannelName: peer.Channel.Title}
			}
		}

	}

	res := []api.Channel{}
	for _, channel := range channels {
		res = append(res, *channel)

	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].ChannelName < res[j].ChannelName
	})
	return res, nil
}

func (a *apiService) UsersSyncChannels(ctx context.Context) error {
	userId := auth.GetUser(ctx)
	peerStorage := tgbbolt.NewPeerStorage(a.boltdb, []byte(fmt.Sprintf("peers:%d", userId)))
	collector := storage.CollectPeers(peerStorage)
	client, err := tgc.AuthClient(ctx, &a.cnf.TG, auth.GetJWTUser(ctx).TgSession, a.middlewares...)
	if err != nil {
		return &apiError{err: err}
	}
	err = client.Run(ctx, func(ctx context.Context) error {
		return collector.Dialogs(ctx, query.GetDialogs(client.API()).Iter())
	})
	if err != nil {
		return &apiError{err: err}
	}
	return nil
}

func (a *apiService) UsersListSessions(ctx context.Context) ([]api.UserSession, error) {
	userId := auth.GetUser(ctx)

	userSession := auth.GetJWTUser(ctx).TgSession

	client, _ := tgc.AuthClient(ctx, &a.cnf.TG, userSession, a.middlewares...)

	var (
		auth *tg.AccountAuthorizations
		err  error
	)

	err = client.Run(ctx, func(ctx context.Context) error {
		auth, err = client.API().AccountGetAuthorizations(ctx)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil && !tgerr.Is(err, "AUTH_KEY_UNREGISTERED") {
		return nil, err
	}

	dbSessions := []models.Session{}

	if err = a.db.Where("user_id = ?", userId).Order("created_at DESC").Find(&dbSessions).Error; err != nil {
		return nil, err
	}

	sessionsOut := []api.UserSession{}

	for _, session := range dbSessions {

		s := api.UserSession{Hash: session.Hash,
			CreatedAt: session.CreatedAt.UTC(),
			Current:   session.Session == userSession}

		if auth != nil {
			for _, a := range auth.Authorizations {
				if session.SessionDate == a.DateCreated {
					s.AppName = api.NewOptString(strings.Trim(strings.Replace(a.AppName, "Telegram", "", -1), " "))
					s.Location = api.NewOptString(a.Country)
					s.OfficialApp = api.NewOptBool(a.OfficialApp)
					s.Valid = true
					break
				}
			}
		}

		sessionsOut = append(sessionsOut, s)
	}

	return sessionsOut, nil
}

func (a *apiService) UsersProfileImage(ctx context.Context, params api.UsersProfileImageParams) (*api.UsersProfileImageOKHeaders, error) {

	client, err := tgc.AuthClient(ctx, &a.cnf.TG, auth.GetJWTUser(ctx).TgSession, a.middlewares...)

	if err != nil {
		return nil, &apiError{err: err}
	}

	res := &api.UsersProfileImageOKHeaders{}

	err = tgc.RunWithAuth(ctx, client, "", func(ctx context.Context) error {
		self, err := client.Self(ctx)
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
		photo.GetPersonal()
		location := &tg.InputPeerPhotoFileLocation{Big: false, Peer: peer, PhotoID: photo.PhotoID}
		buff, err := tgc.GetMediaContent(ctx, client.API(), location)
		if err != nil {
			return err
		}
		content := buff.Bytes()
		res.SetCacheControl("public, max-age=86400, must-revalidate")
		res.SetContentLength(int64(len(content)))
		res.SetEtag(fmt.Sprintf("\"%v\"", photo.PhotoID))
		res.SetContentDisposition(fmt.Sprintf("inline; filename=\"%s\"", "profile.jpeg"))
		res.Response = api.UsersProfileImageOK{Data: bytes.NewReader(content)}
		return nil
	})
	if err != nil {
		return nil, &apiError{err: err}
	}
	return res, nil
}

func (a *apiService) UsersRemoveBots(ctx context.Context) error {
	userId := auth.GetUser(ctx)

	channelId, err := getDefaultChannel(a.db, a.cache, userId)
	if err != nil {
		return &apiError{err: err}
	}

	if err := a.db.Where("user_id = ?", userId).Where("channel_id = ?", channelId).
		Delete(&models.Bot{}).Error; err != nil {
		return &apiError{err: err}
	}

	a.cache.Delete(cache.Key("users", "bots", userId, channelId))

	return nil
}

func (a *apiService) UsersRemoveSession(ctx context.Context, params api.UsersRemoveSessionParams) error {
	userId := auth.GetUser(ctx)

	session := &models.Session{}

	if err := a.db.Where("user_id = ?", userId).Where("hash = ?", params.ID).First(session).Error; err != nil {
		return &apiError{err: err}
	}

	client, _ := tgc.AuthClient(ctx, &a.cnf.TG, session.Session, a.middlewares...)

	client.Run(ctx, func(ctx context.Context) error {
		_, err := client.API().AuthLogOut(ctx)
		if err != nil {
			return err
		}
		return nil
	})

	a.db.Where("user_id = ?", userId).Where("hash = ?", session.Hash).Delete(&models.Session{})

	return nil
}

func (a *apiService) UsersStats(ctx context.Context) (*api.UserConfig, error) {
	userId := auth.GetUser(ctx)
	var (
		channelId int64
		err       error
	)

	channelId, err = getDefaultChannel(a.db, a.cache, userId)
	if err != nil {
		return nil, &apiError{err: err}
	}

	tokens, err := getBotsToken(a.db, a.cache, userId, channelId)

	if err != nil {
		return nil, &apiError{err: err}
	}
	return &api.UserConfig{Bots: tokens, ChannelId: channelId}, nil
}

func (a *apiService) UsersUpdateChannel(ctx context.Context, req *api.ChannelUpdate) error {
	userId := auth.GetUser(ctx)

	channel := &models.Channel{UserId: userId, Selected: true}

	if req.ChannelId.Value != 0 {
		channel.ChannelId = req.ChannelId.Value
	}
	if req.ChannelName.Value != "" {
		channel.ChannelName = req.ChannelName.Value
	}

	if err := a.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"selected": true}),
	}).Create(channel).Error; err != nil {
		return &apiError{err: errors.New("failed to update channel")}
	}
	a.db.Model(&models.Channel{}).Where("channel_id != ?", channel.ChannelId).
		Where("user_id = ?", userId).Update("selected", false)

	a.cache.Set(cache.Key("users", "channel", userId), channel.ChannelId, 0)
	return nil
}

func (a *apiService) addBots(c context.Context, client *telegram.Client, userId int64, channelId int64, botsTokens []string) error {

	botInfoMap := make(map[string]*types.BotInfo)

	err := tgc.RunWithAuth(c, client, "", func(ctx context.Context) error {

		channel, err := tgc.GetChannelById(ctx, client.API(), channelId)

		if err != nil {
			return err
		}

		g, _ := errgroup.WithContext(ctx)

		g.SetLimit(8)

		mapMu := sync.Mutex{}

		for _, token := range botsTokens {
			g.Go(func() error {
				info, err := tgc.GetBotInfo(c, a.boltdb, &a.cnf.TG, token)
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

	payload := []models.Bot{}

	for _, info := range botInfoMap {
		payload = append(payload, models.Bot{UserId: userId, Token: info.Token, BotId: info.Id,
			BotUserName: info.UserName, ChannelId: channelId,
		})
	}

	a.cache.Delete(cache.Key("users", "bots", userId, channelId))

	if err := a.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&payload).Error; err != nil {
		return err
	}
	return nil

}
