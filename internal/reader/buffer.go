package reader

import "github.com/valyala/bytebufferpool"

type buffer struct {
	buf    *bytebufferpool.ByteBuffer
	offset int
}

func (b *buffer) isEmpty() bool {
	return b == nil || len(b.buf.B)-b.offset <= 0
}

func (b *buffer) buffer() []byte {
	return b.buf.B[b.offset:]
}

func (b *buffer) increment(n int) {
	b.offset += n
}

func (b *buffer) length() int {
	return len(b.buf.B) - b.offset
}
