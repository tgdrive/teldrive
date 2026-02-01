package tgc

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/tgdrive/teldrive/internal/logging"
	"go.uber.org/zap"
)

func RunWithAuth(ctx context.Context, client *telegram.Client, token string, f func(ctx context.Context) error) error {
	return client.Run(ctx, func(ctx context.Context) error {
		if err := Auth(ctx, client, token); err != nil {
			return err
		}
		return f(ctx)
	})
}

func Auth(ctx context.Context, client *telegram.Client, token string) error {
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return err
	}

	logger := logging.Component("TG")

	if token == "" {
		if !status.Authorized {
			return errors.Errorf("not authorized. please login first")
		}
		logger.Info("session.user",
			zap.Int64("user_id", status.User.ID),
			zap.String("username", status.User.Username))
	} else {
		if !status.Authorized {
			_, err := client.Auth().Bot(ctx, token)
			if err != nil {
				logger.Error("auth.bot_failed", zap.Error(err))
				return err
			}
			status, err = client.Auth().Status(ctx)
			if err != nil {
				return err
			}
			logger.Info("session.bot",
				zap.Int64("bot_id", status.User.ID),
				zap.String("username", status.User.Username))
		}
	}
	return nil
}
