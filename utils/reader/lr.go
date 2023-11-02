package reader

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/divyam234/teldrive/types"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type linearReader struct {
	ctx       context.Context
	parts     []types.Part
	pos       int
	client    *telegram.Client
	next      func() []byte
	buffer    []byte
	bytesread int64
	chunkSize int64
	i         int64
	mu        sync.Mutex
}

func (*linearReader) Close() error {
	return nil
}

func NewLinearReader(ctx context.Context, client *telegram.Client, parts []types.Part) (io.ReadCloser, error) {

	r := &linearReader{
		ctx:       ctx,
		parts:     parts,
		client:    client,
		chunkSize: int64(1024 * 1024),
	}

	r.next = r.partStream()

	return r, nil
}

func (r *linearReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.i >= int64(len(r.buffer)) {
		r.buffer = r.next()
		if len(r.buffer) == 0 && r.pos == len(r.parts)-1 {
			return 0, io.EOF
		}
		r.i = 0
	}

	n = copy(p, r.buffer[r.i:])

	r.i += int64(n)

	r.bytesread += int64(n)

	if r.bytesread == r.parts[r.pos].Length && r.pos < len(r.parts)-1 {
		r.pos++
		r.next = r.partStream()
		r.bytesread = 0
	}
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

func (r *linearReader) partStream() func() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	start := r.parts[r.pos].Start
	end := r.parts[r.pos].End
	offset := start - (start % r.chunkSize)

	firstPartCut := start - offset

	lastPartCut := (end % r.chunkSize) + 1

	partCount := int((end - offset + r.chunkSize) / r.chunkSize)

	currentPart := 1

	readData := func() []byte {

		if currentPart > partCount {
			return make([]byte, 0)
		}

		res, _ := r.chunk(offset, r.chunkSize)

		if len(res) == 0 {
			return res
		} else if partCount == 1 {
			res = res[firstPartCut:lastPartCut]

		} else if currentPart == 1 {
			res = res[firstPartCut:]

		} else if currentPart == partCount {
			res = res[:lastPartCut]

		}

		currentPart++

		offset += r.chunkSize

		return res

	}
	return readData
}
