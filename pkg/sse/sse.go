package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type EventType string

const (
	EventRequestLog  EventType = "request_log"
	EventResponseLog EventType = "response_log"
	EventIntercepted EventType = "intercepted"
)

type Event struct {
	Type EventType `json:"type"`
}

// Broadcaster fans out events to all registered SSE clients.
type Broadcaster struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subs: make(map[chan Event]struct{}),
	}
}

func (b *Broadcaster) subscribe() (ch chan Event, unsubscribe func()) {
	c := make(chan Event, 8)
	b.mu.Lock()
	b.subs[c] = struct{}{}
	b.mu.Unlock()
	return c, func() {
		b.mu.Lock()
		delete(b.subs, c)
		b.mu.Unlock()
	}
}

func (b *Broadcaster) Broadcast(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for c := range b.subs {
		// Non-blocking: drop if the subscriber channel is full.
		select {
		case c <- e:
		default:
		}
	}
}

// Handler returns an HTTP handler that streams events to the client.
func Handler(b *Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher.Flush()

		ch, unsub := b.subscribe()
		defer unsub()

		for {
			select {
			case evt := <-ch:
				data, _ := json.Marshal(evt)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}
