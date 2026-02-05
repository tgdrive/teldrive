package tgc

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tgdrive/teldrive/internal/config"
)

func TestClientPoolCreation(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)
	assert.NotNil(t, pool)
	assert.Equal(t, 0, pool.Len())
	assert.Equal(t, LeastConnections, pool.Strategy())
}

func TestClientPoolWithStrategy(t *testing.T) {
	pool := NewClientPool(nil, nil, &config.TGConfig{PoolRoutingStrategy: "round_robin"})
	assert.Equal(t, RoundRobin, pool.Strategy())
}

func TestSelectRoundRobin(t *testing.T) {
	pool := NewClientPool(nil, nil, &config.TGConfig{PoolRoutingStrategy: "round_robin"})

	clients := []*PooledClient{
		{Key: "c1"},
		{Key: "c2"},
		{Key: "c3"},
	}

	assert.Equal(t, "c1", pool.selectClient(clients).Key)
	assert.Equal(t, "c2", pool.selectClient(clients).Key)
	assert.Equal(t, "c3", pool.selectClient(clients).Key)
	assert.Equal(t, "c1", pool.selectClient(clients).Key)
}

func TestSelectLeastConnections(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)

	clients := []*PooledClient{
		{Key: "c1", Connections: 10},
		{Key: "c2", Connections: 5},
		{Key: "c3", Connections: 8},
	}

	selected := pool.selectClient(clients)
	assert.Equal(t, "c2", selected.Key)
}

func TestAcquireRelease(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)

	pool.clients.Store("client1", &PooledClient{})

	pool.Acquire("client1")
	assert.Equal(t, int64(1), pool.TotalConnections())

	pool.Acquire("client1")
	assert.Equal(t, int64(2), pool.TotalConnections())

	pool.Release("client1")
	assert.Equal(t, int64(1), pool.TotalConnections())

	pool.Release("client1")
	assert.Equal(t, int64(0), pool.TotalConnections())
}

func TestPoolStats(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)

	stats := pool.Stats()
	assert.Equal(t, 0, stats.TotalClients)
	assert.Equal(t, int64(0), stats.TotalConns)

	pool.clients.Store("test:bot", &PooledClient{Key: "test:bot", Connections: 5})

	// Acquire to increment both totalConns and pc.Connections
	pool.Acquire("test:bot")

	stats = pool.Stats()
	assert.Equal(t, 1, stats.TotalClients)
	assert.Equal(t, int64(1), stats.TotalConns)
	// Connections was 5, then Acquire added 1 atomically = 6
	assert.Equal(t, int64(6), stats.ClientStats["test:bot"])
}

func TestCloseEmptyPool(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)
	err := pool.Close()
	assert.NoError(t, err)
	assert.Equal(t, 0, pool.Len())
}

func TestClosePoolWithClients(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)

	pool.clients.Store("test", &PooledClient{
		stop: func() error { return nil },
	})

	err := pool.Close()
	assert.NoError(t, err)
	assert.Equal(t, 0, pool.Len())
}

func TestPoolIdleTimeout(t *testing.T) {
	pool := NewClientPool(nil, nil, &config.TGConfig{
		PoolIdleTimeout: 1 * time.Minute,
	})
	assert.Equal(t, 1*time.Minute, pool.idleTimeout)

	pool2 := NewClientPool(nil, nil, nil)
	assert.Equal(t, 30*time.Minute, pool2.idleTimeout)
}

func TestClientPool_UserIsolation(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)

	userIDs := []int64{1, 2, 3}
	bots := []string{"sharedBot"}

	for _, userID := range userIDs {
		key := fmt.Sprintf("user:%d:bot:%s", userID, bots[0])
		pool.clients.Store(key, &PooledClient{
			Key: key,
		})
	}

	assert.Equal(t, 3, pool.Len())

	keys := make([]string, 0, 3)
	pool.clients.Range(func(k, v any) bool {
		pc := v.(*PooledClient)
		keys = append(keys, pc.Key)
		return true
	})

	assert.Equal(t, 3, len(keys))
}

func TestPooledClient_Struct(t *testing.T) {
	pc := &PooledClient{
		Key:         "test",
		Connections: 5,
	}

	assert.Equal(t, "test", pc.Key)
	assert.Equal(t, int64(5), pc.Connections)
	assert.Nil(t, pc.Client)
	assert.Nil(t, pc.stop)
}

func TestRoutingStrategy_Values(t *testing.T) {
	assert.Equal(t, RoutingStrategy("round_robin"), RoundRobin)
	assert.Equal(t, RoutingStrategy("least_connections"), LeastConnections)
}

func TestClientPoolLen(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)
	assert.Equal(t, 0, pool.Len())

	pool.clients.Store("a", &PooledClient{})
	pool.clients.Store("b", &PooledClient{})

	assert.Equal(t, 2, pool.Len())
}

func TestClientPoolTotalConnections(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)

	assert.Equal(t, int64(0), pool.TotalConnections())

	pool.clients.Store("a", &PooledClient{Connections: 3})
	pool.clients.Store("b", &PooledClient{Connections: 7})

	// TotalConnections tracks Acquire/Release, not static values
	pool.Acquire("a")
	pool.Acquire("b")

	assert.Equal(t, int64(2), pool.TotalConnections())
}

func TestClientPoolConcurrentAccess(t *testing.T) {
	pool := NewClientPool(nil, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.clients.Store("test", &PooledClient{Key: "test"})

			pool.Acquire("test")
			pool.Release("test")
		}()
	}

	wg.Wait()
	assert.Equal(t, 1, pool.Len())
}
