package channels

import (
	"context"
	"errors"

	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

// TelegramCreator adapts the shared Telegram storage boundary to channel
// rollover. Bot provisioning and authentication remain inside the storage
// implementation's Runner.
type TelegramCreator struct {
	Storage telegramstore.Storage
}

func (c TelegramCreator) Create(ctx context.Context, userID int64, name string) (RemoteChannel, error) {
	if c.Storage == nil {
		return RemoteChannel{}, errors.New("Telegram storage is not configured")
	}
	channel, err := c.Storage.CreateChannel(ctx, userID, name)
	if err != nil {
		return RemoteChannel{}, err
	}
	return RemoteChannel{ID: channel.ID, Name: channel.Name}, nil
}

func (c TelegramCreator) Delete(ctx context.Context, userID, channelID int64) error {
	if c.Storage == nil {
		return errors.New("Telegram storage is not configured")
	}
	return c.Storage.DeleteChannel(ctx, userID, channelID)
}
