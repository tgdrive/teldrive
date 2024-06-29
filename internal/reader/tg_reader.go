package reader

import (
	"context"
	"io"
)

type tgReader struct {
	ctx         context.Context
	cur         *buffer
	offset      int64
	limit       int64
	chunkSize   int64
	leftCut     int64
	rightCut    int64
	totalParts  int
	currentPart int
	chunkSrc    ChunkSource
	err         error
}

func newTGReader(
	ctx context.Context,
	start int64,
	end int64,
	chunkSrc ChunkSource,

) (io.ReadCloser, error) {

	chunkSize := chunkSrc.ChunkSize(start, end)

	offset := start - (start % chunkSize)

	r := &tgReader{
		ctx:         ctx,
		leftCut:     start - offset,
		rightCut:    (end % chunkSize) + 1,
		totalParts:  int((end - offset + chunkSize) / chunkSize),
		offset:      offset,
		limit:       end - start + 1,
		chunkSize:   chunkSize,
		chunkSrc:    chunkSrc,
		currentPart: 1,
	}
	return r, nil
}

func (r *tgReader) Read(p []byte) (int, error) {

	if r.limit <= 0 {
		return 0, io.EOF
	}

	if r.cur.isEmpty() {
		r.cur, r.err = r.next()
		if r.err != nil {
			return 0, r.err
		}
	}

	n := copy(p, r.cur.buffer())
	r.cur.increment(n)
	r.limit -= int64(n)

	if r.limit <= 0 {
		return n, io.EOF
	}

	return n, nil
}

func (*tgReader) Close() error {
	return nil
}

func (r *tgReader) next() (*buffer, error) {

	if r.currentPart > r.totalParts {
		return nil, io.EOF
	}
	chunk, err := r.chunkSrc.Chunk(r.ctx, r.offset, r.chunkSize)
	if err != nil {
		return nil, err
	}
	if r.totalParts == 1 {
		chunk = chunk[r.leftCut:r.rightCut]
	} else if r.currentPart == 1 {
		chunk = chunk[r.leftCut:]
	} else if r.currentPart == r.totalParts {
		chunk = chunk[:r.rightCut]
	}

	r.currentPart++
	r.offset += r.chunkSize
	return &buffer{buf: chunk}, nil

}
