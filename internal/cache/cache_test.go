package cache

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryCacheStoresEntryLargerThanFreeCacheLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := NewMemoryCache(5 * 1024 * 1024)
	defer c.Close()

	want := bytes.Repeat([]byte("x"), 64*1024)
	if err := c.Set(ctx, "large", want, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var got []byte
	if err := c.Get(ctx, "large", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("cached value mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

func TestMemoryCacheDeleteAndTTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := NewMemoryCache(512 * 1024)
	defer c.Close()

	if err := c.Set(ctx, "delete", "value", time.Minute); err != nil {
		t.Fatalf("Set(delete): %v", err)
	}
	if err := c.Delete(ctx, "delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var value string
	if err := c.Get(ctx, "delete", &value); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}

	if err := c.Set(ctx, "ttl", "value", 5*time.Millisecond); err != nil {
		t.Fatalf("Set(ttl): %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := c.Get(ctx, "ttl", &value); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after TTL = %v, want ErrNotFound", err)
	}
}
