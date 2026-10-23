package telegramstore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

var transientTelegramErrors = []string{
	"RPC_CALL_FAIL",
	"RPC_MCGET_FAIL",
	"WORKER_BUSY_TOO_LONG_RETRY",
	"STORAGE_CHOOSE_VOLUME_FAILED",
}

var transientTelegramMessages = []string{
	"timeout",
	"timedout",
	"no workers running",
	"memory limit exit",
	"connection dead",
	"engine was closed",
	"broken pipe",
	"connection reset by peer",
}

type retryMiddleware struct{ max int }

func (m retryMiddleware) Handle(next tg.Invoker) telegram.InvokeFunc {
	return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		for attempt := 0; ; attempt++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			err := next.Invoke(ctx, input, output)
			if err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !isTransientTelegramError(err) {
				return err
			}
			clientID, _ := ClientID(ctx)
			if attempt >= m.max {
				slog.WarnContext(ctx, "telegram.rpc_retry_exhausted",
					"client_id", clientID, "rpc_type", fmt.Sprintf("%T", input),
					"attempts", attempt+1, "max_retries", m.max, "error", err)
				return err
			}
			delay := telegramRetryDelay(attempt)
			slog.DebugContext(ctx, "telegram.rpc_retry",
				"client_id", clientID, "rpc_type", fmt.Sprintf("%T", input),
				"retry", attempt+1, "max_retries", m.max, "delay", delay, "error", err)
			// Give the transport time to reconnect before resending this RPC.
			// Retrying the entire upload would require rewinding its reader.
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
}

func telegramRetryDelay(attempt int) time.Duration {
	delay := 500 * time.Millisecond
	const maxDelay = 10 * time.Second
	for i := 0; i < attempt && delay < maxDelay; i++ {
		delay *= 2
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func isTransientTelegramError(err error) bool {
	if tgerr.Is(err, transientTelegramErrors...) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range transientTelegramMessages {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func newRetryMiddleware(max int) (telegram.Middleware, error) {
	if max < 0 {
		return nil, fmt.Errorf("Telegram retry count cannot be negative")
	}
	return retryMiddleware{max: max}, nil
}
