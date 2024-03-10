package reader

import (
	"context"
	"io"

	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gotd/td/telegram"
)

type linearReader struct {
	ctx    context.Context
	parts  []types.Part
	pos    int
	client *telegram.Client
	reader io.ReadCloser
	limit  int64
	err    error
}

func NewLinearReader(ctx context.Context,
	client *telegram.Client,
	parts []types.Part,
	limit int64,
) (reader io.ReadCloser, err error) {

	r := &linearReader{
		ctx:    ctx,
		parts:  parts,
		client: client,
		limit:  limit,
	}

	reader, err = newTGReader(r.ctx, r.client, r.parts[r.pos])

	if err != nil {
		return nil, err
	}
	r.reader = reader

	return r, nil
}

func (r *linearReader) Read(p []byte) (n int, err error) {

	if r.err != nil {
		return 0, r.err
	}

	if r.limit <= 0 {
		return 0, io.EOF
	}

	n, err = r.reader.Read(p)

	if err == nil {
		r.limit -= int64(n)
	}

	if err == io.EOF {
		if r.limit > 0 {
			err = nil
		}
		r.pos++
		if r.pos < len(r.parts) {
			r.reader, err = newTGReader(r.ctx, r.client, r.parts[r.pos])
		}
	}
	r.err = err
	return
}

func (r *linearReader) Close() (err error) {
	if r.reader != nil {
		err = r.reader.Close()
		r.reader = nil
		return err
	}
	return nil
}
