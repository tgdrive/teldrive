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
	reader    io.ReadCloser
	client    *telegram.Client
	bytesread int64
	chunkSize int64
	sync.Mutex
}

func NewLinearReader(ctx context.Context, client *telegram.Client, parts []types.Part) (io.ReadCloser, error) {

	r := &linearReader{
		ctx:       ctx,
		parts:     parts,
		client:    client,
		chunkSize: int64(1024 * 1024),
	}

	res, err := r.nextPart()

	if err != nil {
		return nil, err
	}

	r.reader = res

	return r, nil
}

func (r *linearReader) Read(p []byte) (n int, err error) {
	r.Lock()
	defer r.Unlock()
	n, err = r.reader.Read(p)
	if err != nil {
		return 0, err
	}

	r.bytesread += int64(n)

	if r.bytesread == r.parts[r.pos].Length && r.pos < len(r.parts)-1 {
		r.pos++
		r.reader, err = r.nextPart()

		if err != nil {
			return 0, err
		}
		r.bytesread = 0
	}
	return n, nil
}

func (r *linearReader) Close() (err error) {
	if r.reader != nil {
		err = r.reader.Close()
		r.reader = nil
	}
	return
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

func (r *linearReader) nextPart() (io.ReadCloser, error) {
	stream := r.tgRangeStream()
	ir, iw := io.Pipe()

	go func() {
		defer iw.Close()

		for {

			data, more := <-stream
			if !more {
				return
			}

			_, err := iw.Write(data)
			if err != nil {
				return
			}
		}
	}()

	return ir, nil
}

func (r *linearReader) tgRangeStream() chan []byte {

	start := r.parts[r.pos].Start
	end := r.parts[r.pos].End
	offset := start - (start % r.chunkSize)

	firstPartCut := start - offset

	lastPartCut := (end % r.chunkSize) + 1

	partCount := int((end - offset + r.chunkSize) / r.chunkSize)

	currentPart := 1

	stream := make(chan []byte)

	go func() {

		defer close(stream)

		for {

			res, _ := r.chunk(offset, r.chunkSize)

			if len(res) == 0 {
				return
			} else if partCount == 1 {
				res = res[firstPartCut:lastPartCut]

			} else if currentPart == 1 {
				res = res[firstPartCut:]

			} else if currentPart == partCount {
				res = res[:lastPartCut]

			}

			stream <- res

			currentPart++

			offset += r.chunkSize

			if currentPart > partCount {
				return
			}

		}
	}()

	return stream
}
