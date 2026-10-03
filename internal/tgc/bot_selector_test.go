package tgc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testBots = []string{"1:aaa", "2:bbb", "3:ccc"}

func newTestSelector(now *time.Time) *MemoryBotSelector {
	s := NewMemoryBotSelector()
	s.now = func() time.Time { return *now }
	return s
}

func nextBots(t *testing.T, s BotSelector, op BotOp, n int) []string {
	t.Helper()
	res := make([]string, n)
	for i := range res {
		token, idx, err := s.Next(context.Background(), op, 1, testBots)
		require.NoError(t, err)
		assert.Equal(t, testBots[idx], token)
		res[i] = token
	}
	return res
}

func TestMemoryBotSelectorRoundRobin(t *testing.T) {
	now := time.Now()
	s := newTestSelector(&now)

	assert.Equal(t, []string{"1:aaa", "2:bbb", "3:ccc", "1:aaa"}, nextBots(t, s, BotOpStream, 4))
}

func TestMemoryBotSelectorNoBots(t *testing.T) {
	s := NewMemoryBotSelector()
	_, _, err := s.Next(context.Background(), BotOpStream, 1, nil)
	assert.Error(t, err)
}

func TestMemoryBotSelectorSkipsStandby(t *testing.T) {
	now := time.Now()
	s := newTestSelector(&now)
	ctx := context.Background()

	require.NoError(t, s.Standby(ctx, BotOpStream, "2:bbb", 30*time.Second))

	assert.Equal(t, []string{"1:aaa", "3:ccc", "1:aaa", "3:ccc"}, nextBots(t, s, BotOpStream, 4))
}

func TestMemoryBotSelectorStandbyIsPerOperation(t *testing.T) {
	now := time.Now()
	s := newTestSelector(&now)

	require.NoError(t, s.Standby(context.Background(), BotOpStream, "1:aaa", 30*time.Second))

	assert.Equal(t, []string{"1:aaa", "2:bbb"}, nextBots(t, s, BotOpUpload, 2))
}

func TestMemoryBotSelectorStandbyExpires(t *testing.T) {
	now := time.Now()
	s := newTestSelector(&now)

	require.NoError(t, s.Standby(context.Background(), BotOpStream, "1:aaa", 30*time.Second))
	assert.Equal(t, []string{"2:bbb", "3:ccc"}, nextBots(t, s, BotOpStream, 2))

	now = now.Add(30 * time.Second)
	assert.Equal(t, []string{"1:aaa"}, nextBots(t, s, BotOpStream, 1))
}

func TestMemoryBotSelectorAllOnStandbyPicksSoonest(t *testing.T) {
	now := time.Now()
	s := newTestSelector(&now)
	ctx := context.Background()

	require.NoError(t, s.Standby(ctx, BotOpStream, "1:aaa", 60*time.Second))
	require.NoError(t, s.Standby(ctx, BotOpStream, "2:bbb", 10*time.Second))
	require.NoError(t, s.Standby(ctx, BotOpStream, "3:ccc", 30*time.Second))

	assert.Equal(t, []string{"2:bbb", "2:bbb"}, nextBots(t, s, BotOpStream, 2))
}

func TestMemoryBotSelectorBotListShrinks(t *testing.T) {
	now := time.Now()
	s := newTestSelector(&now)
	ctx := context.Background()

	nextBots(t, s, BotOpStream, 2)

	token, idx, err := s.Next(ctx, BotOpStream, 1, testBots[:1])
	require.NoError(t, err)
	assert.Equal(t, 0, idx)
	assert.Equal(t, "1:aaa", token)
}

func TestStandbyKeyUsesBotIDOnly(t *testing.T) {
	assert.Equal(t, "stream:123", standbyKey(BotOpStream, "123:secret"))
}
