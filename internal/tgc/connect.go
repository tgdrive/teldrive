package tgc

import (
	"context"
	"errors"

	"github.com/gotd/td/telegram"
)

type StopFunc func() error

type connectOptions struct {
	ctx   context.Context
	token string
}

type Option interface {
	apply(o *connectOptions)
}

type fnOption func(o *connectOptions)

func (f fnOption) apply(o *connectOptions) {
	f(o)
}

func WithContext(ctx context.Context) Option {
	return fnOption(func(o *connectOptions) {
		o.ctx = ctx
	})
}

func WithBotToken(token string) Option {
	return fnOption(func(o *connectOptions) {
		o.token = token
	})
}

func Connect(client *telegram.Client, options ...Option) (StopFunc, error) {
	opt := &connectOptions{
		ctx: context.Background(),
	}
	for _, o := range options {
		o.apply(opt)
	}

	ctx, cancel := context.WithCancel(opt.ctx)

	errC := make(chan error, 1)
	initDone := make(chan struct{})
	go func() {
		defer close(errC)
		errC <- RunWithAuth(ctx, client, opt.token, func(ctx context.Context) error {
			close(initDone)
			<-ctx.Done()
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		})
	}()

	select {
	case <-ctx.Done(): // context canceled
		cancel()
		return func() error { return nil }, ctx.Err()
	case err := <-errC: // startup timeout
		cancel()
		return func() error { return nil }, err
	case <-initDone: // init done
	}

	stopFn := func() error {
		cancel()
		return <-errC
	}
	return stopFn, nil
}
