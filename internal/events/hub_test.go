package events

import (
	"errors"
	"testing"
	"time"
)

func TestHubCoalescesAndIsolatesUsers(t *testing.T) {
	t.Parallel()
	hub := NewHub(2)
	userOne, unsubscribeOne, err := hub.Subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribeOne()
	userTwo, unsubscribeTwo, err := hub.Subscribe(2)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribeTwo()

	hub.Notify(1)
	hub.Notify(1)
	select {
	case <-userOne:
	case <-time.After(time.Second):
		t.Fatal("user one was not notified")
	}
	select {
	case <-userOne:
		t.Fatal("duplicate wake-up was not coalesced")
	default:
	}
	select {
	case <-userTwo:
		t.Fatal("notification leaked to another user")
	default:
	}

	hub.Close()
	select {
	case _, ok := <-userOne:
		if ok {
			t.Fatal("closed hub left subscription open")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription was not closed")
	}
}

func TestReconnectDelayBounds(t *testing.T) {
	t.Parallel()
	minimum := 100 * time.Millisecond
	maximum := 2 * time.Second
	if got := reconnectDelay(minimum, maximum, 0); got != minimum {
		t.Fatalf("attempt zero delay = %s", got)
	}
	if got := reconnectDelay(minimum, maximum, 100); got != maximum {
		t.Fatalf("bounded delay = %s", got)
	}
}

func TestHubEnforcesPerUserConnectionLimit(t *testing.T) {
	t.Parallel()
	hub := NewHub(1)
	_, unsubscribe, err := hub.Subscribe(7)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := hub.Subscribe(7); !errors.Is(err, ErrTooManyConnections) {
		t.Fatalf("second subscription error = %v", err)
	}
	if _, otherUnsubscribe, err := hub.Subscribe(8); err != nil {
		t.Fatalf("another user was limited: %v", err)
	} else {
		otherUnsubscribe()
	}
	unsubscribe()
	if _, replacementUnsubscribe, err := hub.Subscribe(7); err != nil {
		t.Fatalf("subscription slot was not released: %v", err)
	} else {
		replacementUnsubscribe()
	}
}

func TestJitterDelayBounds(t *testing.T) {
	t.Parallel()
	base := time.Second
	for range 100 {
		got := jitterDelay(base)
		if got < 800*time.Millisecond || got > 1200*time.Millisecond {
			t.Fatalf("jittered delay = %s", got)
		}
	}
}
