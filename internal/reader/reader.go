package reader

import (
	"context"
	"io"

	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gotd/td/telegram"
)

type linearReader struct {
	ctx           context.Context
	parts         []types.Part
	pos           int
	client        *telegram.Client
	reader        io.ReadCloser
	bytesread     int64
	contentLength int64
}

func NewLinearReader(ctx context.Context,
	client *telegram.Client,
	parts []types.Part,
	contentLength int64,
) (reader io.ReadCloser, err error) {

	r := &linearReader{
		ctx:           ctx,
		parts:         parts,
		client:        client,
		contentLength: contentLength,
	}

	reader, err = NewTGReader(r.ctx, r.client, r.parts[r.pos])

	if err != nil {
		return nil, err
	}
	r.reader = reader

	return r, nil
}

func (r *linearReader) Read(p []byte) (n int, err error) {

	n, err = r.reader.Read(p)

	if err == io.EOF || n == 0 {
		r.pos++
		if r.pos < len(r.parts) {
			r.reader, err = NewTGReader(r.ctx, r.client, r.parts[r.pos])
			if err != nil {
				return 0, err
			}
		}
	}
	r.bytesread += int64(n)

	if r.bytesread == r.contentLength {
		return n, io.EOF
	}

	return n, nil
}

func (r *linearReader) Close() (err error) {
	if r.reader != nil {
		err = r.reader.Close()
		r.reader = nil
		return err
	}
	return nil
}
