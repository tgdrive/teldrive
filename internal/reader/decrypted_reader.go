package reader

import (
	"context"
	"fmt"
	"io"

	"github.com/divyam234/teldrive/internal/cache"
	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/crypt"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/pkg/schemas"
	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gotd/td/tg"
)

type DecrpytedReader struct {
	ctx         context.Context
	file        *schemas.FileOutFull
	parts       []types.Part
	ranges      []Range
	pos         int
	reader      io.ReadCloser
	remaining   int64
	config      *config.TGConfig
	worker      *tgc.StreamWorker
	client      *tg.Client
	concurrency int
	cache       cache.Cacher
}

func NewDecryptedReader(
	ctx context.Context,
	client *tg.Client,
	worker *tgc.StreamWorker,
	cache cache.Cacher,
	file *schemas.FileOutFull,
	parts []types.Part,
	start,
	end int64,
	config *config.TGConfig,
	concurrency int,
) (*DecrpytedReader, error) {

	r := &DecrpytedReader{
		ctx:         ctx,
		parts:       parts,
		file:        file,
		remaining:   end - start + 1,
		ranges:      calculatePartByteRanges(start, end, parts[0].DecryptedSize),
		config:      config,
		client:      client,
		worker:      worker,
		concurrency: concurrency,
		cache:       cache,
	}
	if err := r.initializeReader(); err != nil {
		return nil, err
	}
	return r, nil

}

func (r *DecrpytedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}

	n, err := r.reader.Read(p)
	r.remaining -= int64(n)

	if err == io.EOF && r.remaining > 0 {
		if err := r.moveToNextPart(); err != nil {
			return n, err
		}
		err = nil
	}

	return n, err
}

func (r *DecrpytedReader) Close() error {
	if r.reader != nil {
		err := r.reader.Close()
		r.reader = nil
		return err
	}
	return nil
}

func (r *DecrpytedReader) initializeReader() error {
	reader, err := r.getPartReader()
	if err != nil {
		return err
	}
	r.reader = reader
	return nil
}

func (r *DecrpytedReader) moveToNextPart() error {
	r.reader.Close()
	r.pos++
	if r.pos < len(r.ranges) {
		return r.initializeReader()
	}
	return io.EOF
}

func (r *DecrpytedReader) getPartReader() (io.ReadCloser, error) {
	currentRange := r.ranges[r.pos]
	salt := r.parts[r.ranges[r.pos].PartNo].Salt
	cipher, _ := crypt.NewCipher(r.config.Uploads.EncryptionKey, salt)
	partID := r.parts[currentRange.PartNo].ID

	chunkSrc := &chunkSource{
		channelID:   r.file.ChannelID,
		partID:      partID,
		client:      r.client,
		concurrency: r.concurrency,
		cache:       r.cache,
		key:         fmt.Sprintf("files:location:%s:%d", r.file.Id, partID),
		worker:      r.worker,
	}

	return cipher.DecryptDataSeek(r.ctx,
		func(ctx context.Context,
			underlyingOffset,
			underlyingLimit int64) (io.ReadCloser, error) {
			var end int64

			if underlyingLimit >= 0 {
				end = min(r.parts[r.ranges[r.pos].PartNo].Size-1, underlyingOffset+underlyingLimit-1)
			}

			if r.concurrency < 2 {
				return newTGReader(r.ctx, underlyingOffset, end, chunkSrc)
			}
			return newTGMultiReader(r.ctx, underlyingOffset, end, r.config, chunkSrc)

		}, currentRange.Start, currentRange.End-currentRange.Start+1)
}
