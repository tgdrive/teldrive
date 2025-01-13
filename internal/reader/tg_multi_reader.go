package reader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/tgc"
	"golang.org/x/sync/errgroup"
)

var (
	ErrStreamAbandoned = errors.New("stream abandoned")
	ErrChunkTimeout    = errors.New("chunk fetch timed out")
)

type ChunkSource interface {
	Chunk(ctx context.Context, offset int64, limit int64) ([]byte, error)
	ChunkSize(start, end int64) int64
}

type chunkSource struct {
	channelId   int64
	partId      int64
	concurrency int
	client      *tg.Client
	key         string
	cache       cache.Cacher
}

func (c *chunkSource) ChunkSize(start, end int64) int64 {
	return tgc.CalculateChunkSize(start, end)
}

func (c *chunkSource) Chunk(ctx context.Context, offset int64, limit int64) ([]byte, error) {
	var (
		location *tg.InputDocumentFileLocation
		err      error
	)

	err = c.cache.Get(c.key, location)

	if err != nil {
		location, err = tgc.GetLocation(ctx, c.client, c.channelId, c.partId)
		if err != nil {
			return nil, err
		}
		c.cache.Set(c.key, location, 30*time.Minute)
	}

	return tgc.GetChunk(ctx, c.client, location, offset, limit)

}

type tgMultiReader struct {
	ctx         context.Context
	cancel      context.CancelFunc
	offset      int64
	limit       int64
	chunkSize   int64
	bufferChan  chan *buffer
	cur         *buffer
	concurrency int
	leftCut     int64
	rightCut    int64
	totalParts  int
	currentPart int
	chunkSrc    ChunkSource
	timeout     time.Duration
}

func newTGMultiReader(
	ctx context.Context,
	start int64,
	end int64,
	config *config.TGConfig,
	chunkSrc ChunkSource,
) (*tgMultiReader, error) {
	chunkSize := chunkSrc.ChunkSize(start, end)
	offset := start - (start % chunkSize)

	ctx, cancel := context.WithCancel(ctx)

	r := &tgMultiReader{
		ctx:         ctx,
		cancel:      cancel,
		limit:       end - start + 1,
		bufferChan:  make(chan *buffer, config.Stream.Buffers),
		concurrency: config.Stream.MultiThreads,
		leftCut:     start - offset,
		rightCut:    (end % chunkSize) + 1,
		totalParts:  int((end - offset + chunkSize) / chunkSize),
		offset:      offset,
		chunkSize:   chunkSize,
		chunkSrc:    chunkSrc,
		timeout:     config.Stream.ChunkTimeout,
	}

	go r.fillBufferConcurrently()
	return r, nil
}

func (r *tgMultiReader) Close() error {
	r.cancel()
	return nil
}

func (r *tgMultiReader) Read(p []byte) (int, error) {
	if r.limit <= 0 {
		return 0, io.EOF
	}

	if r.cur == nil || r.cur.isEmpty() {
		select {
		case cur, ok := <-r.bufferChan:
			if !ok {
				return 0, ErrStreamAbandoned
			}
			r.cur = cur
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
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

func (r *tgMultiReader) fillBufferConcurrently() {
	defer close(r.bufferChan)

	for r.currentPart < r.totalParts {
		if err := r.fillBatch(); err != nil {
			r.cancel()
			return
		}
	}
}

func (r *tgMultiReader) fillBatch() error {
	g, ctx := errgroup.WithContext(r.ctx)
	g.SetLimit(r.concurrency)

	buffers := make([]*buffer, r.concurrency)

	for i := 0; i < r.concurrency && r.currentPart+i < r.totalParts; i++ {
		g.Go(func() error {
			chunkCtx, cancel := context.WithTimeout(ctx, r.timeout)
			defer cancel()

			chunk, err := r.fetchChunkWithTimeout(chunkCtx, int64(i))
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					return fmt.Errorf("chunk %d: %w", r.currentPart+i, ErrChunkTimeout)
				}
				return fmt.Errorf("chunk %d: %w", r.currentPart+i, err)
			}

			if r.totalParts == 1 {
				chunk = chunk[r.leftCut:r.rightCut]
			} else if r.currentPart+i == 0 {
				chunk = chunk[r.leftCut:]
			} else if r.currentPart+i+1 == r.totalParts {
				chunk = chunk[:r.rightCut]
			}

			buffers[i] = &buffer{buf: chunk}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	for _, buf := range buffers {
		if buf == nil {
			break
		}
		select {
		case r.bufferChan <- buf:
		case <-r.ctx.Done():
			return r.ctx.Err()
		}
	}

	r.currentPart += r.concurrency
	r.offset += r.chunkSize * int64(r.concurrency)

	return nil
}

func (r *tgMultiReader) fetchChunkWithTimeout(ctx context.Context, i int64) ([]byte, error) {
	chunkChan := make(chan []byte, 1)
	errChan := make(chan error, 1)

	go func() {
		chunk, err := r.chunkSrc.Chunk(ctx, r.offset+i*r.chunkSize, r.chunkSize)
		if err != nil {
			errChan <- err
		} else {
			chunkChan <- chunk
		}
	}()

	select {
	case chunk := <-chunkChan:
		return chunk, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type buffer struct {
	buf    []byte
	offset int
}

func (b *buffer) isEmpty() bool {
	return b == nil || len(b.buf)-b.offset <= 0
}

func (b *buffer) buffer() []byte {
	return b.buf[b.offset:]
}

func (b *buffer) increment(n int) {
	b.offset += n
}
