package telegramstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
)

const maxProfilePhotoBytes = 10 * 1024 * 1024

type DiscoveredChannel struct {
	ID   int64
	Name string
}

type ProfilePhoto struct {
	Content []byte
	PhotoID int64
}

type Account interface {
	DiscoverChannels(ctx context.Context, userID int64) ([]DiscoveredChannel, error)
	ProfilePhoto(ctx context.Context, userID int64) (ProfilePhoto, bool, error)
}

type GotdAccount struct {
	runner Runner
}

func NewGotdAccount(runner Runner) (*GotdAccount, error) {
	if runner == nil {
		return nil, ErrClientUnavailable
	}
	return &GotdAccount{runner: runner}, nil
}

func (a *GotdAccount) DiscoverChannels(ctx context.Context, userID int64) ([]DiscoveredChannel, error) {
	if a == nil || a.runner == nil || userID <= 0 {
		return nil, ErrInvalidRequest
	}
	channels := make(map[int64]DiscoveredChannel)
	err := a.runner.Run(ctx, userID, OperationManage, func(runCtx context.Context, api *tg.Client) error {
		iterator := query.GetDialogs(api).BatchSize(100).Iter()
		for iterator.Next(runCtx) {
			element := iterator.Value()
			peer, ok := element.Peer.(*tg.InputPeerChannel)
			if !ok {
				continue
			}
			channel, ok := element.Entities.Channel(peer.ChannelID)
			if !ok || channel == nil || channel.ID == 0 || channel.Left {
				continue
			}
			rights, hasRights := channel.GetAdminRights()
			if !channel.Creator && (!hasRights || !rights.AddAdmins) {
				continue
			}
			channels[channel.ID] = DiscoveredChannel{ID: channel.ID, Name: channel.Title}
		}
		if err := iterator.Err(); err != nil {
			return fmt.Errorf("iterate Telegram dialogs: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]DiscoveredChannel, 0, len(channels))
	for _, channel := range channels {
		result = append(result, channel)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (a *GotdAccount) ProfilePhoto(ctx context.Context, userID int64) (ProfilePhoto, bool, error) {
	if a == nil || a.runner == nil || userID <= 0 {
		return ProfilePhoto{}, false, ErrInvalidRequest
	}
	var result ProfilePhoto
	var found bool
	err := a.runner.Run(ctx, userID, OperationDownload, func(runCtx context.Context, api *tg.Client) error {
		users, err := api.UsersGetUsers(runCtx, []tg.InputUserClass{&tg.InputUserSelf{}})
		if err != nil {
			return fmt.Errorf("get Telegram self: %w", err)
		}
		if len(users) == 0 {
			return nil
		}
		user, ok := users[0].AsNotEmpty()
		if !ok || user.Photo == nil {
			return nil
		}
		photo, ok := user.Photo.AsNotEmpty()
		if !ok || photo.PhotoID == 0 {
			return nil
		}
		location := &tg.InputPeerPhotoFileLocation{
			Big: false, Peer: user.AsInputPeer(), PhotoID: photo.PhotoID,
		}
		var buffer bytes.Buffer
		writer := &boundedWriter{writer: &buffer, remaining: maxProfilePhotoBytes}
		if _, err := downloader.NewDownloader().Download(api, location).Stream(runCtx, writer); err != nil {
			return fmt.Errorf("download Telegram profile photo: %w", err)
		}
		result = ProfilePhoto{Content: append([]byte(nil), buffer.Bytes()...), PhotoID: photo.PhotoID}
		found = true
		return nil
	})
	if err != nil {
		return ProfilePhoto{}, false, err
	}
	return result, found, nil
}

type boundedWriter struct {
	writer    *bytes.Buffer
	remaining int64
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("Telegram profile photo exceeds size limit")
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}

var _ Account = (*GotdAccount)(nil)
