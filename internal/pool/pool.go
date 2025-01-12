// implementation taken from iyear/tdl
package pool

import (
	"context"
	"sync"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/logging"
	"go.uber.org/zap"
)

type Pool interface {
	Client(ctx context.Context, dc int) *tg.Client
	Default(ctx context.Context) *tg.Client
	Close() error
}

type pool struct {
	api         *telegram.Client
	size        int64
	mu          *sync.Mutex
	middlewares []telegram.Middleware
	invoke      tg.Invoker
	close       func() error
}

func chainMiddlewares(invoker tg.Invoker, chain ...telegram.Middleware) tg.Invoker {
	if len(chain) == 0 {
		return invoker
	}
	for i := len(chain) - 1; i >= 0; i-- {
		invoker = chain[i].Handle(invoker)
	}

	return invoker
}

func NewPool(c *telegram.Client, size int64, middlewares ...telegram.Middleware) Pool {
	return &pool{
		api:         c,
		size:        size,
		mu:          &sync.Mutex{},
		middlewares: middlewares,
	}
}

func (p *pool) current() int {
	return p.api.Config().ThisDC
}

func (p *pool) Client(ctx context.Context, dc int) *tg.Client {
	return tg.NewClient(p.invoker(ctx, dc))
}

func (p *pool) invoker(ctx context.Context, dc int) tg.Invoker {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.invoke != nil {
		return p.invoke
	}

	var (
		invoker telegram.CloseInvoker
		err     error
	)
	if dc == p.current() {
		invoker, err = p.api.Pool(p.size)
	} else {
		invoker, err = p.api.DC(ctx, dc, p.size)
	}

	if err != nil {
		logging.FromContext(ctx).Error("create invoker", zap.Error(err))
		return p.api
	}

	p.close = invoker.Close
	p.invoke = chainMiddlewares(invoker, p.middlewares...)

	return p.invoke
}

func (p *pool) Default(ctx context.Context) *tg.Client {
	return p.Client(ctx, p.current())
}

func (p *pool) Close() error {

	if p.close != nil {
		return p.close()
	}

	return nil
}
