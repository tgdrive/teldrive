package reader

import (
	"context"
	"fmt"
	"io"

	"github.com/divyam234/teldrive/internal/cache"
	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/pkg/schemas"
	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gotd/td/tg"
)

func calculatePartByteRanges(startByte, endByte, partSize int64) []types.Range {
	partByteRanges := []types.Range{}
	startPart := startByte / partSize
	endPart := endByte / partSize
	startOffset := startByte % partSize

	for part := startPart; part <= endPart; part++ {
		partStartByte := int64(0)
		partEndByte := partSize - 1

		if part == startPart {
			partStartByte = startOffset
		}
		if part == endPart {
			partEndByte = endByte % partSize
		}

		partByteRanges = append(partByteRanges, types.Range{
			Start:  partStartByte,
			End:    partEndByte,
			PartNo: part,
		})

		startOffset = 0
	}

	return partByteRanges
}

type LinearReader struct {
	ctx         context.Context
	file        *schemas.FileOutFull
	parts       []types.Part
	ranges      []types.Range
	pos         int
	reader      io.ReadCloser
	limit       int64
	config      *config.TGConfig
	worker      *tgc.StreamWorker
	client      *tg.Client
	concurrency int
	cache       cache.Cacher
}

func NewLinearReader(ctx context.Context,
	client *tg.Client,
	worker *tgc.StreamWorker,
	cache cache.Cacher,
	file *schemas.FileOutFull,
	parts []types.Part,
	start,
	end int64,
	config *config.TGConfig,
	concurrency int,
) (io.ReadCloser, error) {

	r := &LinearReader{
		ctx:         ctx,
		parts:       parts,
		file:        file,
		limit:       end - start + 1,
		ranges:      calculatePartByteRanges(start, end, parts[0].Size),
		config:      config,
		client:      client,
		worker:      worker,
		concurrency: concurrency,
		cache:       cache,
	}

	var err error
	r.reader, err = r.nextPart()
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (r *LinearReader) Read(p []byte) (int, error) {
	if r.limit <= 0 {
		return 0, io.EOF
	}

	n, err := r.reader.Read(p)

	if err == io.EOF && r.limit > 0 {
		err = nil
		if r.reader != nil {
			r.reader.Close()
		}
		r.pos++
		if r.pos < len(r.ranges) {
			r.reader, err = r.nextPart()
		}
	}

	r.limit -= int64(n)
	return n, err
}

func (r *LinearReader) nextPart() (io.ReadCloser, error) {
	start := r.ranges[r.pos].Start
	end := r.ranges[r.pos].End

	partID := r.parts[r.ranges[r.pos].PartNo].ID

	chunkSrc := &chunkSource{
		channelID:   r.file.ChannelID,
		partID:      partID,
		client:      r.client,
		concurrency: r.concurrency,
		cache:       r.cache,
		key:         fmt.Sprintf("files:location:%s:%d", r.file.Id, partID),
		worker:      r.worker,
	}

	if r.concurrency < 2 {
		return newTGReader(r.ctx, start, end, chunkSrc)
	}
	return newTGMultiReader(r.ctx, start, end, r.config, chunkSrc)
}

func (r *LinearReader) Close() error {
	if r.reader != nil {
		err := r.reader.Close()
		r.reader = nil
		return err
	}
	return nil
}
