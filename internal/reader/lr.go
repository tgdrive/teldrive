package reader

import (
	"context"
	"fmt"
	"io"

	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type linearReader struct {
	ctx           context.Context
	parts         []types.Part
	pos           int
	client        *telegram.Client
	next          func() ([]byte, error)
	buffer        []byte
	bytesread     int64
	chunkSize     int64
	i             int64
	contentLength int64
}

func (*linearReader) Close() error {
	return nil
}

func NewLinearReader(ctx context.Context, client *telegram.Client, parts []types.Part, contentLength int64) (io.ReadCloser, error) {

	r := &linearReader{
		ctx:           ctx,
		parts:         parts,
		client:        client,
		chunkSize:     int64(1024 * 1024),
		contentLength: contentLength,
	}

	r.next = r.partStream()

	return r, nil
}

func (r *linearReader) Read(p []byte) (n int, err error) {

	if r.bytesread == r.contentLength {
		return 0, io.EOF
	}

	if r.i >= int64(len(r.buffer)) {
		r.buffer, err = r.next()
		if err != nil {
			return 0, err
		}
		if len(r.buffer) == 0 {
			r.pos++
			if r.pos == len(r.parts) {
				return 0, io.EOF
			} else {
				r.next = r.partStream()
				r.buffer, err = r.next()
				if err != nil {
					return 0, err
				}
			}

		}
		r.i = 0
	}

	n = copy(p, r.buffer[r.i:])

	r.i += int64(n)

	r.bytesread += int64(n)

	return n, nil
}

func (r *linearReader) chunk(offset int64, limit int64) ([]byte, error) {

	req := &tg.UploadGetFileRequest{
		Offset:   offset,
		Limit:    int(limit),
		Location: r.parts[r.pos].Location,
	}

	res, err := r.client.API().UploadGetFile(r.ctx, req)

	if err != nil {
		return nil, err
	}

	switch result := res.(type) {
	case *tg.UploadFile:
		return result.Bytes, nil
	default:
		return nil, fmt.Errorf("unexpected type %T", r)
	}
}

func (r *linearReader) partStream() func() ([]byte, error) {

	start := r.parts[r.pos].Start
	end := r.parts[r.pos].End
	offset := start - (start % r.chunkSize)

	firstPartCut := start - offset

	lastPartCut := (end % r.chunkSize) + 1

	partCount := int((end - offset + r.chunkSize) / r.chunkSize)

	currentPart := 1

	readData := func() ([]byte, error) {

		if currentPart > partCount {
			return make([]byte, 0), nil
		}

		res, err := r.chunk(offset, r.chunkSize)

		if err != nil {
			return nil, err
		}

		if len(res) == 0 {
			return res, nil
		} else if partCount == 1 {
			res = res[firstPartCut:lastPartCut]

		} else if currentPart == 1 {
			res = res[firstPartCut:]

		} else if currentPart == partCount {
			res = res[:lastPartCut]

		}

		currentPart++

		offset += r.chunkSize

		return res, nil

	}
	return readData
}
