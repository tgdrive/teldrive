package reader

import (
	"context"
	"io"

	"github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/crypt"
	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gotd/td/telegram"
)

type decrpytedReader struct {
	ctx           context.Context
	parts         []types.Part
	pos           int
	client        *telegram.Client
	reader        io.ReadCloser
	bytesread     int64
	contentLength int64
	config        *config.Config
}

func NewDecryptedReader(
	ctx context.Context,
	client *telegram.Client,
	parts []types.Part,
	contentLength int64) (io.ReadCloser, error) {

	r := &decrpytedReader{
		ctx:           ctx,
		parts:         parts,
		client:        client,
		contentLength: contentLength,
		config:        config.GetConfig(),
	}
	res, err := r.nextPart()

	if err != nil {
		return nil, err
	}

	r.reader = res

	return r, nil

}

func (r *decrpytedReader) Read(p []byte) (n int, err error) {

	n, err = r.reader.Read(p)

	if err == io.EOF || n == 0 {
		r.pos++
		if r.pos < len(r.parts) {
			r.reader, err = r.nextPart()
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

func (r *decrpytedReader) Close() (err error) {
	if r.reader != nil {
		err = r.reader.Close()
		r.reader = nil
		return err
	}
	return nil
}

func (r *decrpytedReader) nextPart() (io.ReadCloser, error) {

	cipher, _ := crypt.NewCipher(r.config.EncryptionKey, r.parts[r.pos].Salt)

	return cipher.DecryptDataSeek(r.ctx,
		func(ctx context.Context,
			underlyingOffset,
			underlyingLimit int64) (io.ReadCloser, error) {

			var end int64

			if underlyingLimit >= 0 {
				end = min(r.parts[r.pos].Size-1, underlyingOffset+underlyingLimit-1)
			}

			return NewTGReader(r.ctx, r.client, types.Part{
				Start:    underlyingOffset,
				End:      end,
				Location: r.parts[r.pos].Location,
			})
		}, r.parts[r.pos].Start, r.parts[r.pos].End-r.parts[r.pos].Start+1)

}
