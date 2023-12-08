package reader

import (
	"context"
	"fmt"
	"io"

	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type tgReader struct {
	ctx       context.Context
	client    *telegram.Client
	location  *tg.InputDocumentFileLocation
	start     int64
	end       int64
	next      func() ([]byte, error)
	buffer    []byte
	bytesread int64
	chunkSize int64
	i         int64
}

func calculateChunkSize(start, end int64) int64 {
	chunkSize := int64(1024 * 1024)

	for chunkSize > 1024 && chunkSize > (end-start) {
		chunkSize /= 2
	}

	return chunkSize
}

func NewTGReader(
	ctx context.Context,
	client *telegram.Client,
	part types.Part,

) (io.ReadCloser, error) {

	r := &tgReader{
		ctx:       ctx,
		location:  part.Location,
		client:    client,
		start:     part.Start,
		end:       part.End,
		chunkSize: calculateChunkSize(part.Start, part.End),
	}
	r.next = r.partStream()
	return r, nil
}

func (r *tgReader) Read(p []byte) (n int, err error) {

	if r.i >= int64(len(r.buffer)) {
		r.buffer, err = r.next()
		if err != nil {
			return 0, err
		}
		if len(r.buffer) == 0 {
			r.next = r.partStream()
			r.buffer, err = r.next()
			if err != nil {
				return 0, err
			}

		}
		r.i = 0
	}
	n = copy(p, r.buffer[r.i:])
	r.i += int64(n)
	r.bytesread += int64(n)

	if r.bytesread == r.end-r.start+1 {
		return n, io.EOF
	}
	return n, nil
}

func (*tgReader) Close() error {
	return nil
}

func (r *tgReader) chunk(offset int64, limit int64) ([]byte, error) {

	req := &tg.UploadGetFileRequest{
		Offset:   offset,
		Limit:    int(limit),
		Location: r.location,
		Precise:  true,
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

func (r *tgReader) partStream() func() ([]byte, error) {

	start := r.start
	end := r.end
	offset := start - (start % r.chunkSize)

	leftCut := start - offset
	rightCut := (end % r.chunkSize) + 1
	totalParts := int((end - offset + r.chunkSize) / r.chunkSize)
	currentPart := 1

	return func() ([]byte, error) {
		if currentPart > totalParts {
			return make([]byte, 0), nil
		}
		res, err := r.chunk(offset, r.chunkSize)
		if err != nil {
			return nil, err
		}
		if len(res) == 0 {
			return res, nil
		} else if totalParts == 1 {
			res = res[leftCut:rightCut]
		} else if currentPart == 1 {
			res = res[leftCut:]
		} else if currentPart == totalParts {
			res = res[:rightCut]
		}

		currentPart++
		offset += r.chunkSize
		return res, nil
	}
}
