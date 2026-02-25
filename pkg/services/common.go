package services

import (
	"context"
	"fmt"
	"time"

	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.uber.org/zap"
)

func getParts(ctx context.Context, telegram TelegramService, client TelegramClient, c cache.Cacher, fileID string, channelID int64, fileParts []api.Part, encrypted bool) ([]types.Part, error) {
	return cache.Fetch(ctx, c, cache.KeyFileMessages(fileID), 60*time.Minute, func() ([]types.Part, error) {
		parts, err := telegram.GetParts(ctx, client, channelID, fileParts, encrypted)
		if err != nil {
			return nil, err
		}

		if len(parts) != len(fileParts) {
			logger := logging.Component("FILE")
			logger.Error("parts.mismatch",
				zap.String("file_id", fileID),
				zap.Int("expected", len(fileParts)),
				zap.Int("actual", len(parts)))
			return nil, fmt.Errorf("file parts mismatch")
		}
		return parts, nil
	})
}
