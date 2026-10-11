package querytrace

import (
	"context"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

type Counter struct {
	mu     sync.Mutex
	counts map[string]int
}

func (c *Counter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	name := queryName(data.SQL)
	if name == "" {
		return ctx
	}
	c.mu.Lock()
	if c.counts == nil {
		c.counts = make(map[string]int)
	}
	c.counts[name]++
	c.mu.Unlock()
	return ctx
}

func (*Counter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *Counter) Count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[name]
}

func queryName(sql string) string {
	const prefix = "-- name: "
	if !strings.HasPrefix(sql, prefix) {
		return ""
	}
	name := strings.TrimPrefix(sql, prefix)
	if index := strings.IndexByte(name, ' '); index >= 0 {
		name = name[:index]
	}
	if index := strings.IndexByte(name, '\n'); index >= 0 {
		name = name[:index]
	}
	return name
}
