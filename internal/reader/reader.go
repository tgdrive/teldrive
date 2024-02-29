package reader

import (
	"context"
	"io"

	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gotd/td/telegram"
)

type linearReader struct {
	ctx    context.Context
	chunks []types.Part
	client *telegram.Client
	limit  int64
	count  int64
	pos    int
	reader io.ReadCloser
	err    error
}

func NewLinearReader(ctx context.Context, client *telegram.Client, offset, limit int64, chunks []types.Part) (io.ReadCloser, error) {
	r := &linearReader{
		ctx:    ctx,
		chunks: chunks,
		limit:  limit,
		client: client,
	}

	err := io.EOF
	for offset >= 0 && err != nil {
		offset, err = r.nextChunk(offset)
	}
	if err == nil || err == io.EOF {
		r.err = err
		return r, nil
	}
	return nil, err
}

func (r *linearReader) nextChunk(offset int64) (int64, error) {
	if r.err != nil {
		return -1, r.err
	}
	if r.pos >= len(r.chunks) || r.limit <= 0 || offset < 0 {
		return -1, io.EOF
	}

	chunk := r.chunks[r.pos]
	count := chunk.Size
	r.pos++

	if offset >= count {
		return offset - count, io.EOF
	}
	count -= offset
	if r.limit < count {
		count = r.limit
	}

	if err := r.Close(); err != nil {
		return -1, err
	}

	reader, err := newTGReader(r.ctx, r.client, r.chunks[r.pos-1].Location, offset, offset+count-1)
	if err != nil {
		return -1, err
	}

	r.reader = reader
	r.count = count
	return offset, nil
}

func (r *linearReader) Read(p []byte) (n int, err error) {
	if r.err != nil {
		return 0, r.err
	}
	if r.limit <= 0 {
		r.err = io.EOF
		return 0, io.EOF
	}

	for r.count <= 0 {
		off, err := r.nextChunk(0)
		if off < 0 {
			r.err = err
			return 0, err
		}
	}

	n, err = r.reader.Read(p)
	if err == nil || err == io.EOF {
		r.count -= int64(n)
		r.limit -= int64(n)
		if r.limit > 0 {
			err = nil
		}
	}
	r.err = err
	return
}

func (r *linearReader) Close() (err error) {
	if r.reader != nil {
		err = r.reader.Close()
		r.reader = nil
	}
	return
}
