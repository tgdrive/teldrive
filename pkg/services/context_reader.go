package services

import (
	"context"
	"io"
)

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func newContextReader(ctx context.Context, reader io.Reader) io.Reader {
	if reader == nil {
		return nil
	}

	return &contextReader{ctx: ctx, reader: reader}
}

func (r *contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, context.Cause(r.ctx)
	default:
	}

	n, err := r.reader.Read(p)
	if err != nil {
		return n, err
	}

	select {
	case <-r.ctx.Done():
		if n > 0 {
			return n, nil
		}
		return 0, context.Cause(r.ctx)
	default:
		return n, nil
	}
}
