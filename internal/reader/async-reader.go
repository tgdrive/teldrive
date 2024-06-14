// taken from rclone async reader implmentation
package reader

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/rclone/rclone/lib/pool"
)

const (
	BufferSize           = 1024 * 1024
	softStartInitial     = 4 * 1024
	bufferCacheSize      = 64
	bufferCacheFlushTime = 5 * time.Second
)

var ErrorStreamAbandoned = errors.New("stream abandoned")

type AsyncReader struct {
	in      io.ReadCloser
	ready   chan *buffer
	token   chan struct{}
	exit    chan struct{}
	buffers int
	err     error
	cur     *buffer
	exited  chan struct{}
	closed  bool
	mu      sync.Mutex
}

func NewAsyncReader(ctx context.Context, rd io.ReadCloser, buffers int) (*AsyncReader, error) {
	if buffers <= 0 {
		return nil, errors.New("number of buffers too small")
	}
	if rd == nil {
		return nil, errors.New("nil reader supplied")
	}
	a := &AsyncReader{}
	a.init(rd, buffers)
	return a, nil
}

func (a *AsyncReader) init(rd io.ReadCloser, buffers int) {
	a.in = rd
	a.ready = make(chan *buffer, buffers)
	a.token = make(chan struct{}, buffers)
	a.exit = make(chan struct{})
	a.exited = make(chan struct{})
	a.buffers = buffers
	a.cur = nil

	for i := 0; i < buffers; i++ {
		a.token <- struct{}{}
	}

	go func() {
		defer close(a.exited)
		defer close(a.ready)
		for {
			select {
			case <-a.token:
				b := a.getBuffer()
				err := b.read(a.in)
				a.ready <- b
				if err != nil {
					return
				}
			case <-a.exit:
				return
			}
		}
	}()
}

var bufferPool *pool.Pool
var bufferPoolOnce sync.Once

func (a *AsyncReader) putBuffer(b *buffer) {
	bufferPool.Put(b.buf)
	b.buf = nil
}

func (a *AsyncReader) getBuffer() *buffer {
	bufferPoolOnce.Do(func() {
		bufferPool = pool.New(bufferCacheFlushTime, BufferSize, bufferCacheSize, false)
	})
	return &buffer{
		buf: bufferPool.Get(),
	}
}

func (a *AsyncReader) fill() (err error) {
	if a.cur.isEmpty() {
		if a.cur != nil {
			a.putBuffer(a.cur)
			a.token <- struct{}{}
			a.cur = nil
		}
		b, ok := <-a.ready
		if !ok {
			if a.err == nil {
				return ErrorStreamAbandoned
			}
			return a.err
		}
		a.cur = b
	}
	return nil
}

func (a *AsyncReader) Read(p []byte) (n int, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	err = a.fill()
	if err != nil {
		return 0, err
	}

	n = copy(p, a.cur.buffer())
	a.cur.increment(n)

	if a.cur.isEmpty() {
		a.err = a.cur.err
		return n, a.err
	}
	return n, nil
}

func (a *AsyncReader) StopBuffering() {
	select {
	case <-a.exit:
		return
	default:
	}
	close(a.exit)
	<-a.exited
}

func (a *AsyncReader) Abandon() {
	a.StopBuffering()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cur != nil {
		a.putBuffer(a.cur)
		a.cur = nil
	}
	for b := range a.ready {
		a.putBuffer(b)
	}
}

func (a *AsyncReader) Close() (err error) {
	a.Abandon()
	if a.closed {
		return nil
	}
	a.closed = true
	return a.in.Close()
}

type buffer struct {
	buf    []byte
	err    error
	offset int
}

func (b *buffer) isEmpty() bool {
	if b == nil {
		return true
	}
	if len(b.buf)-b.offset <= 0 {
		return true
	}
	return false
}

func (b *buffer) readFill(r io.Reader, buf []byte) (n int, err error) {
	var nn int
	for n < len(buf) && err == nil {
		nn, err = r.Read(buf[n:])
		n += nn
	}
	return n, err
}

func (b *buffer) read(rd io.Reader) error {
	var n int
	n, b.err = b.readFill(rd, b.buf)
	b.buf = b.buf[0:n]
	b.offset = 0
	return b.err
}

func (b *buffer) buffer() []byte {
	return b.buf[b.offset:]
}

func (b *buffer) increment(n int) {
	b.offset += n
}
