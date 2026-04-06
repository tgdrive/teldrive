package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/tgstorage"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"go.uber.org/zap"
)

func (a *apiService) UsersAddBots(ctx context.Context, req *api.AddBots) error {
	userID := auth.User(ctx)

	if len(req.Bots) > 0 {
		for _, token := range req.Bots {
			err := a.repo.Bots.CreateToken(ctx, userID, token)
			if err != nil && !errors.Is(err, repositories.ErrConflict) {
				return err
			}
		}
		channelRows, err := a.repo.Channels.GetByUserID(ctx, userID)
		if err != nil {
			return err
		}
		channels := make([]int64, 0, len(channelRows))
		for _, c := range channelRows {
			channels = append(channels, c.ChannelID)
		}
		if len(channels) > 0 {
			for _, channel := range channels {
				if err := a.channelManager.AddBotsToChannel(ctx, userID, channel, req.Bots, false); err != nil {
					return err
				}
			}
		}
		a.cache.Delete(ctx, cache.KeyUserBots(userID))
	}
	return nil

}

func (a *apiService) UsersListChannels(ctx context.Context) ([]api.Channel, error) {

	userId := auth.User(ctx)

	channels := make(map[int64]*api.Channel)

	peerStorage := tgstorage.NewPeerStorage(a.repo.KV, cache.KeyPeer(userId))

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
	userID := auth.User(ctx)
	_, err := a.channelManager.CreateNewChannel(ctx, req.ChannelName, userID, false)
	if err != nil {
		return &apiError{err: err}
	}
	return nil
}

func (a *apiService) UsersDeleteChannel(ctx context.Context, params api.UsersDeleteChannelParams) error {
	userId := auth.User(ctx)
	client, err := a.telegram.AuthClient(ctx, auth.JWTUser(ctx).TgSession, 5)
	if err != nil {
		return &apiError{err: err}
	}
	channelId, err := strconv.ParseInt(params.ID, 10, 64)
	if err != nil {
		return &apiError{err: fmt.Errorf("invalid channel id: %w", err), code: 400}
	}
	peerStorage := tgstorage.NewPeerStorage(a.repo.KV, cache.KeyPeer(userId))
	peerKey, err := a.telegram.DeleteChannel(ctx, client, channelId)
	if err != nil {
		return &apiError{err: err}
	}
	if err := a.repo.Channels.Delete(ctx, channelId); err != nil {
		return &apiError{err: err}
	}
	if err := peerStorage.Delete(ctx, peerKey); err != nil {
		logging.FromContext(ctx).Warn("failed to delete peer storage entry",
			zap.Int64("user_id", userId),
			zap.Int64("channel_id", channelId),
			zap.Error(err),
		)
	}
	return nil
}

func (a *apiService) UsersSyncChannels(ctx context.Context) error {
	userId := auth.User(ctx)
	peerStorage := tgstorage.NewPeerStorage(a.repo.KV, cache.KeyPeer(userId))
	err := peerStorage.Purge(ctx)
	if err != nil {
		return &apiError{err: err}
	}
	client, err := a.telegram.AuthClient(ctx, auth.JWTUser(ctx).TgSession, 5)
	if err != nil {
		return &apiError{err: err}
	}
	err = a.telegram.SyncDialogs(ctx, client, peerStorage)
	if err != nil {
		return &apiError{err: err}
	}
	return nil
}

func (a *apiService) UsersListSessions(ctx context.Context) ([]api.UserSession, error) {
	userId := auth.User(ctx)
	return cache.Fetch(ctx, a.cache, cache.KeyUserSessions(userId), 0, func() ([]api.UserSession, error) {
		userSession := auth.JWTUser(ctx).TgSession
		client, err := a.telegram.AuthClient(ctx, userSession, 5)
		if err != nil {
			return nil, err
		}
		auths, err := a.telegram.ListAuthorizations(ctx, client)

		if err != nil && !a.telegram.IsAuthKeyUnregistered(err) {
			return nil, err
		}

		dbSessions, err := a.repo.Sessions.GetByUserID(ctx, userId)
		if err != nil {
			return nil, err
		}

		sessionsOut := []api.UserSession{}

		for _, session := range dbSessions {

			s := api.UserSession{SessionId: api.UUID(session.ID),
				CreatedAt: session.CreatedAt.UTC(),
				Current:   session.TgSession == userSession}

			for _, authorization := range auths {
				if session.SessionDate != nil && *session.SessionDate == authorization.DateCreated {
					s.AppName = api.NewOptString(strings.Trim(strings.ReplaceAll(authorization.AppName, "Telegram", ""), " "))
					s.Location = api.NewOptString(authorization.Country)
					s.OfficialApp = api.NewOptBool(authorization.OfficialApp)
					s.Valid = true
					break
				}
			}

			sessionsOut = append(sessionsOut, s)
		}

		return sessionsOut, nil

	})

}

func (a *apiService) UsersProfileImage(ctx context.Context) (*api.UsersProfileImageOKHeaders, error) {

	client, err := a.telegram.AuthClient(ctx, auth.JWTUser(ctx).TgSession, 5)

	if err != nil {
		return nil, &apiError{err: err}
	}

	res := &api.UsersProfileImageOKHeaders{}

	content, photoID, found, err := a.telegram.GetProfilePhoto(ctx, client)
	if err != nil {
		return nil, &apiError{err: err}
	}
	if found {
		res.SetCacheControl("public, max-age=86400, must-revalidate")
		res.SetContentLength(int64(len(content)))
		res.SetEtag(fmt.Sprintf("\"%v\"", photoID))
		res.SetContentDisposition(fmt.Sprintf("inline; filename=\"%s\"", "profile.jpeg"))
		res.Response = api.UsersProfileImageOK{Data: bytes.NewReader(content)}
	}
	return res, nil
}

func (a *apiService) UsersRemoveBots(ctx context.Context) error {
	userId := auth.User(ctx)

	if err := a.repo.Bots.DeleteByUserID(ctx, userId); err != nil {
		return &apiError{err: err}
	}
	a.cache.Delete(ctx, cache.KeyUserBots(userId))

	return nil
}

func (a *apiService) UsersListApiKeys(ctx context.Context) ([]api.UserApiKey, error) {
	userID := auth.User(ctx)
	keys, err := a.repo.APIKeys.ListByUserID(ctx, userID)
	if err != nil {
		return nil, &apiError{err: err}
	}

	out := make([]api.UserApiKey, 0, len(keys))
	for _, key := range keys {
		item := api.UserApiKey{
			ID:        api.UUID(key.ID),
			Name:      key.Name,
			CreatedAt: key.CreatedAt.UTC(),
		}
		if key.ExpiresAt != nil {
			item.ExpiresAt = api.NewOptDateTime(key.ExpiresAt.UTC())
		}
		if key.LastUsedAt != nil {
			item.LastUsedAt = api.NewOptDateTime(key.LastUsedAt.UTC())
		}
		out = append(out, item)
	}

	return out, nil
}

func (a *apiService) UsersCreateApiKey(ctx context.Context, req *api.UserApiKeyCreate) (*api.UserApiKeyCreateResult, error) {
	userID := auth.User(ctx)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, &apiError{err: errors.New("name is required"), code: 400}
	}

	var expiresAt *time.Time
	if req.ExpiresAt.Set {
		expires := req.ExpiresAt.Value.UTC()
		if !expires.After(time.Now().UTC()) {
			return nil, &apiError{err: errors.New("expiresAt must be in the future"), code: 400}
		}
		expiresAt = &expires
	}

	raw, err := generateToken(32)
	if err != nil {
		return nil, &apiError{err: err}
	}
	keyValue := "tdk_" + raw

	row := jetmodel.APIKeys{
		UserID:    userID,
		Name:      name,
		TokenHash: hashToken(keyValue),
		ExpiresAt: expiresAt,
	}
	if err := a.repo.APIKeys.Create(ctx, &row); err != nil {
		return nil, &apiError{err: err}
	}

	res := &api.UserApiKeyCreateResult{}
	res.ID = api.UUID(row.ID)
	res.Name = row.Name
	res.Key = keyValue
	res.CreatedAt = row.CreatedAt.UTC()
	if row.ExpiresAt != nil {
		res.ExpiresAt = api.NewOptDateTime(row.ExpiresAt.UTC())
	}

	return res, nil
}

func (a *apiService) UsersRemoveApiKey(ctx context.Context, params api.UsersRemoveApiKeyParams) error {
	userID := auth.User(ctx)
	if err := a.repo.APIKeys.Revoke(ctx, userID, uuid.UUID(params.ID)); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return &apiError{err: errors.New("api key not found"), code: 404}
		}
		return &apiError{err: err}
	}
	_ = a.cache.DeletePattern(ctx, cache.KeyAPIKeyAuthPattern())

	return nil
}

func (a *apiService) UsersRemoveSession(ctx context.Context, params api.UsersRemoveSessionParams) error {
	userId := auth.User(ctx)

	session, err := a.repo.Sessions.GetByID(ctx, uuid.UUID(params.ID))
	if err != nil {
		return &apiError{err: err}
	}
	if session.UserID != userId {
		return &apiError{err: errors.New("session not found"), code: 404}
	}
	if err := a.repo.Sessions.Revoke(ctx, session.ID); err != nil {
		return &apiError{err: err}
	}
	a.cache.Delete(ctx, cache.KeySessionID(session.ID.String()), cache.KeyUserSessions(userId))
	_ = a.cache.DeletePattern(ctx, cache.KeyAPIKeyAuthPattern())
	client, err := a.telegram.AuthClient(ctx, session.TgSession, 5)
	if err != nil {
		logging.FromContext(ctx).Warn("session revoked but telegram client init failed",
			zap.Int64("user_id", userId),
			zap.String("session_id", session.ID.String()),
			zap.Error(err),
		)
		return nil
	}

	if err := a.telegram.LogOut(ctx, client); err != nil {
		logging.FromContext(ctx).Warn("session revoked but telegram logout failed",
			zap.Int64("user_id", userId),
			zap.String("session_id", session.ID.String()),
			zap.Error(err),
		)
		return nil
	}

	return nil
}

func (a *apiService) UsersStats(ctx context.Context) (*api.UserConfig, error) {
	userId := auth.User(ctx)
	var (
		channelId int64
		err       error
	)

	channelId, err = a.channelManager.CurrentChannel(ctx, userId)
	if err != nil {
		channelId = 0
	}

	tokens, err := a.channelManager.BotTokens(ctx, userId)

	if err != nil {
		tokens = []string{}
	}
	return &api.UserConfig{Bots: tokens, ChannelId: channelId}, nil
}

func (a *apiService) UsersUpdateChannel(ctx context.Context, req *api.ChannelUpdate) error {
	userId := auth.User(ctx)

	channelID := req.ChannelId.Value
	if channelID == 0 {
		return &apiError{err: errors.New("channel id is required"), code: 400}
	}

	err := a.repo.WithTx(ctx, func(txCtx context.Context) error {
		_, err := a.repo.Channels.GetByChannelID(txCtx, channelID)
		if err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				name := req.ChannelName.Value
				if name == "" {
					name = strconv.FormatInt(channelID, 10)
				}
				selected := true
				if err := a.repo.Channels.Create(txCtx, &jetmodel.Channels{UserID: userId, ChannelID: channelID, ChannelName: name, Selected: &selected}); err != nil {
					return errors.New("failed to update channel")
				}
			} else {
				return errors.New("failed to update channel")
			}
		}

		allChannels, err := a.repo.Channels.GetByUserID(txCtx, userId)
		if err == nil {
			for _, c := range allChannels {
				selected := c.ChannelID == channelID
				if err := a.repo.Channels.Update(txCtx, c.ChannelID, repositories.ChannelUpdate{Selected: &selected}); err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		return &apiError{err: err}
	}

	a.cache.Set(ctx, cache.KeyUserChannel(userId), channelID, 0)
	return nil
}
