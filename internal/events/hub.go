package events

import (
	"errors"
	"sync"
)

var (
	ErrTooManyConnections = errors.New("too many event stream connections")
	ErrServiceClosed      = errors.New("event service is closed")
)

// Hub coalesces PostgreSQL wake-ups for local SSE subscribers. It never carries
// event payloads; subscribers always read durable rows from PostgreSQL.
type Hub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[int64]map[uint64]chan struct{}
	maxPerUser  int
	closed      bool
}

func NewHub(maxPerUser int) *Hub {
	return &Hub{subscribers: make(map[int64]map[uint64]chan struct{}), maxPerUser: maxPerUser}
}

func (h *Hub) Subscribe(userID int64) (<-chan struct{}, func(), error) {
	if h == nil || userID <= 0 {
		return nil, nil, errors.New("invalid event subscriber")
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, nil, ErrServiceClosed
	}
	if h.maxPerUser > 0 && len(h.subscribers[userID]) >= h.maxPerUser {
		h.mu.Unlock()
		return nil, nil, ErrTooManyConnections
	}
	h.nextID++
	id := h.nextID
	wake := make(chan struct{}, 1)
	if h.subscribers[userID] == nil {
		h.subscribers[userID] = make(map[uint64]chan struct{})
	}
	h.subscribers[userID][id] = wake
	h.mu.Unlock()

	var once sync.Once
	return wake, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			userSubscribers := h.subscribers[userID]
			if userSubscribers == nil {
				return
			}
			delete(userSubscribers, id)
			if len(userSubscribers) == 0 {
				delete(h.subscribers, userID)
			}
		})
	}, nil
}

func (h *Hub) Notify(userID int64) {
	if h == nil || userID <= 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for _, wake := range h.subscribers[userID] {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func (h *Hub) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for _, userSubscribers := range h.subscribers {
		for _, wake := range userSubscribers {
			close(wake)
		}
	}
	h.subscribers = nil
}
