package services

import (
	"context"
	"errors"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.uber.org/zap"
)

func getParts(ctx context.Context, client *telegram.Client, c cache.Cacher, file *models.File) ([]types.Part, error) {
	return cache.Fetch(c, cache.Key("files", "messages", file.ID), 60*time.Minute, func() ([]types.Part, error) {
		messages, err := tgc.GetMessages(ctx, client.API(), utils.Map(file.Parts, func(part api.Part) int {
			return part.ID
		}), *file.ChannelId)

		if err != nil {
			return nil, err
		}
		parts := []types.Part{}
		for i, message := range messages {
			switch item := message.(type) {
			case *tg.Message:
				media, ok := item.Media.(*tg.MessageMediaDocument)
				if !ok {
					continue
				}
				document, ok := media.Document.(*tg.Document)
				if !ok {
					continue
				}
				part := types.Part{
					ID:   int64(file.Parts[i].ID),
					Size: document.Size,
					Salt: file.Parts[i].Salt.Value,
				}
				if *file.Encrypted {
					part.DecryptedSize, _ = crypt.DecryptedSize(document.Size)
				}
				parts = append(parts, part)
			}
		}
		if len(parts) != len(file.Parts) {
			msg := "file parts mismatch"
			logging.FromContext(ctx).Error(msg, zap.String("name", file.Name),
				zap.Int("expected", len(file.Parts)), zap.Int("actual", len(parts)))
			return nil, errors.New(msg)
		}
		return parts, nil
	})
}
