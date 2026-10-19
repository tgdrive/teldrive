package streamstats

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAttributionConcurrencyAndIdleSpeed(t *testing.T) {
	var registry Registry
	ctx, closeStream := registry.Start(context.Background(), "owner:file")
	ctx = WithPart(ctx, 900, 12)
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			done := BeginDownload(ctx, int64(10+i%2), int64(i*4096))
			done(4096, nil)
		}(i)
	}
	workers.Wait()
	failed := BeginDownload(ctx, 10, 409600)
	failed(0, errors.New("retry"))
	snapshot := registry.Snapshot("owner:file")
	if snapshot.Active != 1 || len(snapshot.Accounts) != 2 {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	for _, a := range snapshot.Accounts {
		if a.Bytes != 50*4096 || a.Active != 0 || a.ChannelID != 900 || a.MessageID != 12 || a.BytesPerSecond(time.Now()) <= 0 {
			t.Fatalf("attribution: %+v", a)
		}
		if a.BytesPerSecond(time.Now().Add(5*time.Second)) != 0 {
			t.Fatal("idle account retains a speed")
		}
	}
	if snapshot.Accounts[0].Requests != 51 || snapshot.Accounts[0].Errors != 1 {
		t.Fatal("retry was not counted")
	}
	closeStream()
	closeStream()
	if registry.Snapshot("owner:file").Active != 0 {
		t.Fatal("stream not closed exactly once")
	}
	if len(registry.Snapshot("another:file").Accounts) != 0 {
		t.Fatal("statistics leaked across files")
	}
}

func TestRegistryBoundAndExpiry(t *testing.T) {
	var registry Registry
	for i := 0; i < 512; i++ {
		_, done := registry.Start(context.Background(), string(rune(i)))
		done()
	}
	ctx, done := registry.Start(context.Background(), "overflow")
	BeginDownload(ctx, 1, 0)(100, nil)
	done()
	if len(registry.streams) != 512 || len(registry.Snapshot("overflow").Accounts) != 0 {
		t.Fatal("registry is unbounded")
	}
	for _, s := range registry.streams {
		s.touched = time.Now().Add(-11 * time.Minute)
	}
	_, done = registry.Start(context.Background(), "fresh")
	done()
	if len(registry.streams) != 1 {
		t.Fatal("expired entries were not reaped")
	}
}
