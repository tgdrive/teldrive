package recovery

import (
	"context"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-faster/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

type recovery struct {
	ctx     context.Context
	backoff backoff.BackOff
}

func New(ctx context.Context, backoff backoff.BackOff) telegram.Middleware {
	return &recovery{
		ctx:     ctx,
		backoff: backoff,
	}
}

func (r *recovery) Handle(next tg.Invoker) telegram.InvokeFunc {
	return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		return backoff.RetryNotify(func() error {
			if err := next.Invoke(ctx, input, output); err != nil {
				if r.shouldRecover(err) {
					return errors.Wrap(err, "recover")
				}

				return backoff.Permanent(err)
			}

			return nil
		}, r.backoff, nil)
	}
}

func (r *recovery) shouldRecover(err error) bool {
	select {
	case <-r.ctx.Done():
		return false
	default:
	}

	_, ok := tgerr.As(err)

	return !ok
}
