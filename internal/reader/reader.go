package reader

import (
	"context"
	"io"

	"github.com/divyam234/teldrive/internal/cache"
	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/pkg/types"
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
	parts       []types.Part
	ranges      []types.Range
	pos         int
	reader      io.ReadCloser
	limit       int64
	config      *config.TGConfig
	channelID   int64
	worker      *tgc.StreamWorker
	client      *tgc.Client
	fileID      string
	concurrency int
	cache       cache.Cacher
}

func NewLinearReader(ctx context.Context, fileID string, parts []types.Part, start, end int64,
	channelID int64, config *config.TGConfig, concurrency int, client *tgc.Client,
	worker *tgc.StreamWorker, cache cache.Cacher) (io.ReadCloser, error) {

	r := &LinearReader{
		ctx:         ctx,
		parts:       parts,
		limit:       end - start + 1,
		ranges:      calculatePartByteRanges(start, end, parts[0].Size),
		config:      config,
		client:      client,
		worker:      worker,
		channelID:   channelID,
		fileID:      fileID,
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

	chunkSrc := &chunkSource{
		channelID:   r.channelID,
		worker:      r.worker,
		fileID:      r.fileID,
		partID:      r.parts[r.ranges[r.pos].PartNo].ID,
		client:      r.client,
		concurrency: r.concurrency,
		cache:       r.cache,
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
