package tgc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// BotOp represents the type of operation for bot selection
type BotOp string

const (
	BotOpStream BotOp = "stream"
	BotOpUpload BotOp = "upload"
)

// BotSelector selects the next bot for a user using round-robin
type BotSelector interface {
	// Next returns the next bot token for the given user and operation using round-robin.
	// Different operations (stream, upload) maintain separate counters.
	// Bots on standby for the operation are skipped. If every bot is on standby,
	// the one whose standby ends soonest is returned.
	Next(ctx context.Context, op BotOp, userID int64, bots []string) (token string, index int, err error)
	// Standby takes a bot out of rotation for the given operation for duration d.
	Standby(ctx context.Context, op BotOp, token string, d time.Duration) error
}

// selectorKey creates a unique key for user+operation combination
func selectorKey(op BotOp, userID int64) string {
	return fmt.Sprintf("%s:%d", op, userID)
}

// standbyKey creates a unique key for bot+operation combination.
// Only the bot ID (the part of the token before the colon) is used, so the secret never ends up in a key.
func standbyKey(op BotOp, token string) string {
	return fmt.Sprintf("%s:%s", op, strings.Split(token, ":")[0])
}

// pickAvailable walks the bots in round-robin order starting at start and returns the index of the
// first bot that is not on standby. If all bots are on standby, it returns the one whose standby ends soonest.
func pickAvailable(bots []string, start int, now time.Time, standbyUntil func(token string) (time.Time, bool)) int {
	best := -1
	var bestUntil time.Time
	for i := range bots {
		idx := (start + i) % len(bots)
		until, ok := standbyUntil(bots[idx])
		if !ok || !now.Before(until) {
			return idx
		}
		if best == -1 || until.Before(bestUntil) {
			best, bestUntil = idx, until
		}
	}
	return best
}

// MemoryBotSelector provides in-memory round-robin bot selection.
// This is used when Redis is not available (single instance mode).
type MemoryBotSelector struct {
	mu      sync.Mutex
	currIdx map[string]int
	standby map[string]time.Time
	now     func() time.Time
}

// NewMemoryBotSelector creates a new in-memory bot selector.
func NewMemoryBotSelector() *MemoryBotSelector {
	return &MemoryBotSelector{
		currIdx: make(map[string]int),
		standby: make(map[string]time.Time),
		now:     time.Now,
	}
}

// Next returns the next bot token using in-memory round-robin, skipping bots on standby.
func (s *MemoryBotSelector) Next(ctx context.Context, op BotOp, userID int64, bots []string) (string, int, error) {
	if len(bots) == 0 {
		return "", 0, fmt.Errorf("no bots available")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	key := selectorKey(op, userID)
	idx := pickAvailable(bots, s.currIdx[key]%len(bots), now, func(token string) (time.Time, bool) {
		k := standbyKey(op, token)
		until, ok := s.standby[k]
		if ok && !now.Before(until) {
			delete(s.standby, k)
			return time.Time{}, false
		}
		return until, ok
	})
	s.currIdx[key] = (idx + 1) % len(bots)
	return bots[idx], idx, nil
}

// Standby takes a bot out of rotation for the given operation for duration d.
func (s *MemoryBotSelector) Standby(ctx context.Context, op BotOp, token string, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.standby[standbyKey(op, token)] = s.now().Add(d)
	return nil
}

// RedisBotSelector provides Redis-backed round-robin bot selection.
// This enables coordinated bot selection across multiple TelDrive instances.
type RedisBotSelector struct {
	client *redis.Client
	now    func() time.Time
}

// NewRedisBotSelector creates a new Redis-backed bot selector.
func NewRedisBotSelector(client *redis.Client) *RedisBotSelector {
	return &RedisBotSelector{client: client, now: time.Now}
}

func redisStandbyKey(op BotOp, token string) string {
	return "teldrive:bot_standby:" + standbyKey(op, token)
}

// Next returns the next bot token using Redis atomic increment for coordination, skipping bots on standby.
func (s *RedisBotSelector) Next(ctx context.Context, op BotOp, userID int64, bots []string) (string, int, error) {
	if len(bots) == 0 {
		return "", 0, fmt.Errorf("no bots available")
	}

	key := fmt.Sprintf("teldrive:bot_idx:%s:%d", op, userID)

	// Atomic increment in Redis
	idx, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return "", 0, fmt.Errorf("redis incr failed: %w", err)
	}

	// Convert to 0-based index and wrap around
	start := int((idx - 1) % int64(len(bots)))

	// Standby entries expire on their own via TTL; the stored value is the end time in unix milliseconds,
	// used to find the bot that is available soonest when every bot is on standby.
	keys := make([]string, len(bots))
	for i, token := range bots {
		keys[i] = redisStandbyKey(op, token)
	}
	vals, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return "", 0, fmt.Errorf("redis mget failed: %w", err)
	}
	standby := make(map[string]time.Time, len(bots))
	for i, v := range vals {
		str, ok := v.(string)
		if !ok {
			continue
		}
		ms, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			continue
		}
		standby[bots[i]] = time.UnixMilli(ms)
	}

	actualIdx := pickAvailable(bots, start, s.now(), func(token string) (time.Time, bool) {
		until, ok := standby[token]
		return until, ok
	})

	return bots[actualIdx], actualIdx, nil
}

// Standby takes a bot out of rotation for the given operation for duration d.
func (s *RedisBotSelector) Standby(ctx context.Context, op BotOp, token string, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	until := s.now().Add(d).UnixMilli()
	if err := s.client.Set(ctx, redisStandbyKey(op, token), until, d).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}
	return nil
}

func NewBotSelector(redisClient *redis.Client) BotSelector {
	if redisClient != nil {
		return NewRedisBotSelector(redisClient)
	}
	return NewMemoryBotSelector()
}
