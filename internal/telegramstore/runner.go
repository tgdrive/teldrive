package telegramstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

var ErrClientUnavailable = errors.New("Telegram client is unavailable")

type clientIDContextKey struct{}

func WithClientID(ctx context.Context, clientID int64) context.Context {
	if clientID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, clientIDContextKey{}, clientID)
}

func ClientID(ctx context.Context) (int64, bool) {
	clientID, ok := ctx.Value(clientIDContextKey{}).(int64)
	return clientID, ok && clientID > 0
}

// ClientProvider owns user/bot session lookup, persistent session storage, and
// gotd middleware construction. Returning a new or safely reusable client is an
// infrastructure concern; storage business logic only needs the running API.
type ClientProvider interface {
	Client(ctx context.Context, userID int64, operation Operation) (*telegram.Client, error)
}

type ClientProviderFunc func(context.Context, int64, Operation) (*telegram.Client, error)

func (f ClientProviderFunc) Client(ctx context.Context, userID int64, operation Operation) (*telegram.Client, error) {
	return f(ctx, userID, operation)
}

// ClientRunner is the production Runner for gotd. It guarantees that every raw
// tg.Client callback executes inside telegram.Client.Run, so authentication,
// updates, reconnects, flood waits, and connection cleanup share one lifetime.
type ClientRunner struct {
	Provider ClientProvider
	Factory  *Factory
}

type PooledRunner interface {
	RunPooled(ctx context.Context, userID int64, operation Operation, connections int, fn func(context.Context, *tg.Client) error) error
}

func (r ClientRunner) Run(ctx context.Context, userID int64, operation Operation, fn func(context.Context, *tg.Client) error) error {
	return r.run(ctx, userID, operation, 1, fn)
}

func (r ClientRunner) RunPooled(ctx context.Context, userID int64, operation Operation, connections int, fn func(context.Context, *tg.Client) error) error {
	return r.run(ctx, userID, operation, connections, fn)
}

func (r ClientRunner) run(ctx context.Context, userID int64, operation Operation, connections int, fn func(context.Context, *tg.Client) error) error {
	if r.Provider == nil || userID <= 0 || connections < 1 || fn == nil {
		return ErrInvalidRequest
	}
	client, err := r.Provider.Client(ctx, userID, operation)
	if err != nil {
		return fmt.Errorf("get Telegram client for %s: %w", operation, err)
	}
	if client == nil {
		return ErrClientUnavailable
	}
	if err := client.Run(ctx, func(runCtx context.Context) error {
		status, err := client.Auth().Status(runCtx)
		if err != nil {
			return fmt.Errorf("get Telegram authorization status: %w", err)
		}
		if !status.Authorized || status.User == nil {
			return fmt.Errorf("Telegram session is not authorized")
		}
		if operation == OperationManage && status.User.Bot {
			return fmt.Errorf("Telegram manage operation requires a user session, got bot account %d", status.User.ID)
		}
		slog.DebugContext(runCtx, "Telegram client authenticated",
			"user_id", userID,
			"operation", operation,
			"telegram_id", status.User.ID,
			"telegram_username", status.User.Username,
			"is_bot", status.User.Bot,
		)

		api := client.API()
		if connections > 1 {
			if r.Factory == nil {
				return ErrTelegramConfiguration
			}
			pooled, closePool, err := r.Factory.PooledAPI(client, connections)
			if err != nil {
				return err
			}
			defer closePool()
			api = pooled
		}
		return fn(WithClientID(runCtx, status.User.ID), api)
	}); err != nil {
		return fmt.Errorf("run Telegram client for %s: %w", operation, err)
	}
	return nil
}

func runWithConnections(ctx context.Context, runner Runner, userID int64, operation Operation, connections int, fn func(context.Context, *tg.Client) error) error {
	if pooled, ok := runner.(PooledRunner); ok && connections > 1 {
		return pooled.RunPooled(ctx, userID, operation, connections, fn)
	}
	return runner.Run(ctx, userID, operation, fn)
}

var _ Runner = ClientRunner{}
var _ PooledRunner = ClientRunner{}
