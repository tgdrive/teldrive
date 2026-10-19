// Package streamstats records actual Telegram download requests without credentials.
package streamstats

import (
	"context"
	"sort"
	"sync"
	"time"
)

type key struct{}
type bucket struct {
	second int64
	bytes  int64
}
type Account struct {
	ID                           int64
	Bytes, Requests, Errors      int64
	Active                       int
	LastSeen                     time.Time
	LatencyMS                    float64
	ChannelID, MessageID, Offset int64
	rate                         [4]bucket
}
type Snapshot struct {
	Started  time.Time
	Active   int
	Accounts []Account
}
type Stream struct {
	mu               sync.Mutex
	started, touched time.Time
	active           int
	accounts         map[int64]*Account
}
type Registry struct {
	mu      sync.Mutex
	streams map[string]*Stream
}

func (r *Registry) Start(ctx context.Context, file string) (context.Context, func()) {
	r.mu.Lock()
	if r.streams == nil {
		r.streams = make(map[string]*Stream)
	}
	now := time.Now()
	for k, s := range r.streams {
		s.mu.Lock()
		expired := s.active == 0 && now.Sub(s.touched) > 10*time.Minute
		s.mu.Unlock()
		if expired {
			delete(r.streams, k)
		}
	}
	s := r.streams[file]
	if s == nil && len(r.streams) >= 512 {
		r.mu.Unlock()
		return ctx, func() {}
	}
	if s == nil {
		s = &Stream{started: now, accounts: make(map[int64]*Account)}
		r.streams[file] = s
	}
	s.mu.Lock()
	s.active++
	s.touched = now
	s.mu.Unlock()
	r.mu.Unlock()
	var once sync.Once
	return context.WithValue(ctx, key{}, s), func() { once.Do(func() { s.mu.Lock(); s.active--; s.touched = time.Now(); s.mu.Unlock() }) }
}

// Begin is called immediately before each real Telegram RPC, including retries.
func Begin(ctx context.Context, id, channel, message, offset int64) func(int, error) {
	s, _ := ctx.Value(key{}).(*Stream)
	if s == nil || id <= 0 {
		return func(int, error) {}
	}
	start := time.Now()
	s.mu.Lock()
	a := s.accounts[id]
	if a == nil {
		if len(s.accounts) >= 128 {
			s.mu.Unlock()
			return func(int, error) {}
		}
		a = &Account{ID: id}
		s.accounts[id] = a
	}
	a.Active++
	a.Requests++
	a.ChannelID = channel
	a.MessageID = message
	a.Offset = offset
	s.mu.Unlock()
	return func(n int, err error) {
		now := time.Now()
		s.mu.Lock()
		defer s.mu.Unlock()
		a.Active--
		a.LastSeen = now
		a.LatencyMS = float64(now.Sub(start).Microseconds()) / 1000
		if err != nil {
			a.Errors++
		}
		a.Bytes += int64(n)
		second := now.Unix()
		b := &a.rate[second%4]
		if b.second != second {
			*b = bucket{second: second}
		}
		b.bytes += int64(n)
		s.touched = now
	}
}
func (a Account) BytesPerSecond(now time.Time) float64 {
	var n int64
	for _, b := range a.rate {
		if age := now.Unix() - b.second; age >= 0 && age < 2 {
			n += b.bytes
		}
	}
	return float64(n) / 2
}
func (r *Registry) Snapshot(file string) Snapshot {
	r.mu.Lock()
	s := r.streams[file]
	r.mu.Unlock()
	out := Snapshot{Accounts: []Account{}}
	if s == nil {
		return out
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out.Started = s.started
	out.Active = s.active
	for _, a := range s.accounts {
		out.Accounts = append(out.Accounts, *a)
	}
	sort.Slice(out.Accounts, func(i, j int) bool { return out.Accounts[i].ID < out.Accounts[j].ID })
	return out
}

type partKey struct{}
type part struct{ channel, message int64 }

func WithPart(ctx context.Context, channel, message int64) context.Context {
	return context.WithValue(ctx, partKey{}, part{channel, message})
}
func BeginDownload(ctx context.Context, id, offset int64) func(int, error) {
	p, _ := ctx.Value(partKey{}).(part)
	return Begin(ctx, id, p.channel, p.message, offset)
}
