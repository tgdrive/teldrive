package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/tgstorage"
	"github.com/tgdrive/teldrive/pkg/models"

	"github.com/gotd/contrib/storage"
	"gorm.io/gorm/clause"
)

func (a *apiService) UsersAddBots(ctx context.Context, req *api.AddBots) error {
	userID := auth.GetUser(ctx)

	payload := []models.Bot{}
	if len(req.Bots) > 0 {
		for _, token := range req.Bots {
			botID, _ := strconv.ParseInt(strings.Split(token, ":")[0], 10, 64)
			payload = append(payload, models.Bot{UserId: userID, Token: token, BotId: botID})
		}
		if err := a.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&payload).Error; err != nil {
			return err
		}
		var channels []int64
		if err := a.db.Model(&models.Channel{}).Where("user_id = ?", userID).Pluck("channel_id", &channels).Error; err != nil {
			return err
		}
		if len(channels) > 0 {
			for _, channel := range channels {
				_ = a.channelManager.AddBotsToChannel(ctx, userID, channel, req.Bots, false)
			}
		}
		_ = a.cache.Delete(cache.Key("users", "bots", userID))
	}
	return nil

}

func (a *apiService) UsersListChannels(ctx context.Context) ([]api.Channel, error) {

	userId := auth.GetUser(ctx)

	channels := make(map[int64]*api.Channel)

	peerStorage := tgstorage.NewPeerStorage(a.db, cache.Key("peers", userId))

	iter, err := peerStorage.Iterate(ctx)
	if err != nil {
		return []api.Channel{}, nil
	}
	defer iter.Close()
	for iter.Next(ctx) {
		peer := iter.Value()
		if peer.Channel != nil && peer.Channel.AdminRights.AddAdmins {
			_, exists := channels[peer.Channel.ID]
			if !exists {
				channels[peer.Channel.ID] = &api.Channel{ChannelId: api.NewOptInt64(peer.Channel.ID), ChannelName: peer.Channel.Title}
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

func (a *apiService) UsersCreateChannel(ctx context.Context, req *api.Channel) error {
	userID := auth.GetUser(ctx)
	_, err := a.channelManager.CreateNewChannel(ctx, req.ChannelName, userID, false)
	if err != nil {
		return &apiError{err: err}
	}
	return nil
}

func (a *apiService) UsersDeleteChannel(ctx context.Context, params api.UsersDeleteChannelParams) error {
	userId := auth.GetUser(ctx)
	client, _ := tgc.AuthClient(ctx, &a.cnf.TG, auth.GetJWTUser(ctx).TgSession, a.middlewares...)
	channelId, _ := strconv.ParseInt(params.ID, 10, 64)
	peerStorage := tgstorage.NewPeerStorage(a.db, cache.Key("peers", userId))
	var (
		channel *tg.Channel
		err     error
	)
	err = client.Run(ctx, func(ctx context.Context) error {
		channel, err = tgc.GetChannelFull(ctx, client.API(), channelId)
		if err != nil {
			return err
		}
		_, err = client.API().ChannelsDeleteChannel(ctx, channel.AsInput())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return &apiError{err: err}
	}
	a.db.Where("channel_id = ?", channelId).Delete(&models.Channel{})
	peer := storage.Peer{}
	peer.FromChat(channel)
	_ = peerStorage.Delete(ctx, storage.KeyFromPeer(peer))
	return nil
}

func (a *apiService) UsersSyncChannels(ctx context.Context) error {
	userId := auth.GetUser(ctx)
	peerStorage := tgstorage.NewPeerStorage(a.db, cache.Key("peers", userId))
	err := peerStorage.Purge(ctx)
	if err != nil {
		return &apiError{err: err}
	}
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
	return cache.Fetch(a.cache, cache.Key("users", "sessions", userId), 0, func() ([]api.UserSession, error) {
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
						s.AppName = api.NewOptString(strings.Trim(strings.ReplaceAll(a.AppName, "Telegram", ""), " "))
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

	})

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

	if err := a.db.Where("user_id = ?", userId).Delete(&models.Bot{}).Error; err != nil {
		return &apiError{err: err}
	}
	_ = a.cache.Delete(cache.Key("users", "bots", userId))

	return nil
}

func (a *apiService) UsersRemoveSession(ctx context.Context, params api.UsersRemoveSessionParams) error {
	userId := auth.GetUser(ctx)

	session := &models.Session{}

	if err := a.db.Where("user_id = ?", userId).Where("hash = ?", params.ID).First(session).Error; err != nil {
		return &apiError{err: err}
	}

	client, _ := tgc.AuthClient(ctx, &a.cnf.TG, session.Session, a.middlewares...)

	_ = client.Run(ctx, func(ctx context.Context) error {
		_, err := client.API().AuthLogOut(ctx)
		if err != nil {
			return err
		}
		return nil
	})

	a.db.Where("user_id = ?", userId).Where("hash = ?", session.Hash).Delete(&models.Session{})
	_ = a.cache.Delete(cache.Key("users", "sessions", userId))

	return nil
}

func (a *apiService) UsersStats(ctx context.Context) (*api.UserConfig, error) {
	userId := auth.GetUser(ctx)
	var (
		channelId int64
		err       error
	)

	channelId, err = a.channelManager.CurrentChannel(userId)
	if err != nil {
		channelId = 0
	}

	tokens, err := a.channelManager.BotTokens(userId)

	if err != nil {
		tokens = []string{}
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
		DoUpdates: clause.Assignments(map[string]any{"selected": true}),
	}).Create(channel).Error; err != nil {
		return &apiError{err: errors.New("failed to update channel")}
	}
	a.db.Model(&models.Channel{}).Where("channel_id != ?", channel.ChannelId).
		Where("user_id = ?", userId).Update("selected", false)

	_ = a.cache.Set(cache.Key("users", "channel", userId), channel.ChannelId, 0)
	return nil
}
