package realtime

import (
	"sync"
	"sync/atomic"
	"time"
)

type POSEvent struct {
	ID        uint64    `json:"id"`
	Type      string    `json:"type"`
	Action    string    `json:"action,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type posHub struct {
	mu          sync.RWMutex
	subscribers map[uint]map[chan POSEvent]struct{}
}

var (
	hub             = &posHub{subscribers: make(map[uint]map[chan POSEvent]struct{})}
	posEventCounter atomic.Uint64
)

func NewPOSEvent(action string) POSEvent {
	return POSEvent{
		ID:        posEventCounter.Add(1),
		Type:      "pos-sync",
		Action:    action,
		CreatedAt: time.Now(),
	}
}

func SubscribePOS(divisionID uint) (<-chan POSEvent, func()) {
	events := make(chan POSEvent, 8)

	hub.mu.Lock()
	if hub.subscribers[divisionID] == nil {
		hub.subscribers[divisionID] = make(map[chan POSEvent]struct{})
	}
	hub.subscribers[divisionID][events] = struct{}{}
	hub.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			hub.mu.Lock()
			delete(hub.subscribers[divisionID], events)
			if len(hub.subscribers[divisionID]) == 0 {
				delete(hub.subscribers, divisionID)
			}
			close(events)
			hub.mu.Unlock()
		})
	}

	return events, unsubscribe
}

func PublishPOS(divisionID uint, action string) {
	event := NewPOSEvent(action)

	hub.mu.RLock()
	defer hub.mu.RUnlock()

	for subscriber := range hub.subscribers[divisionID] {
		select {
		case subscriber <- event:
		default:
		}
	}
}
