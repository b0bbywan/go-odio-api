package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// Broadcaster fans out events from a single upstream channel to all connected SSE clients.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[chan events.Event]struct{}
}

func newBroadcaster(ctx context.Context, upstream <-chan events.Event) *Broadcaster {
	b := &Broadcaster{
		clients: make(map[chan events.Event]struct{}),
	}
	go b.run(ctx, upstream)
	return b
}

func (b *Broadcaster) subscribe() chan events.Event {
	ch := make(chan events.Event, 32)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broadcaster) unsubscribe(ch chan events.Event) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *Broadcaster) broadcast(e events.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- e:
		default:
			logger.Warn("[sse] client channel full, dropping %s event", e.Type)
		}
	}
}

func (b *Broadcaster) run(ctx context.Context, upstream <-chan events.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-upstream:
			if !ok {
				return
			}
			b.broadcast(e)
		}
	}
}

// fanIn merges multiple event channels into a single channel.
// Nil sources are skipped. The merged channel is closed when all sources are
// exhausted or the context is done.
func fanIn(ctx context.Context, sources ...<-chan events.Event) <-chan events.Event {
	merged := make(chan events.Event, 64)
	var wg sync.WaitGroup

	for _, src := range sources {
		if src == nil {
			continue
		}
		wg.Add(1)
		go func(ch <-chan events.Event) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case e, ok := <-ch:
					if !ok {
						return
					}
					select {
					case merged <- e:
					case <-ctx.Done():
						return
					}
				}
			}
		}(src)
	}

	go func() {
		wg.Wait()
		close(merged)
	}()

	return merged
}

// sseHandler returns an http.HandlerFunc that streams SSE events to clients.
func sseHandler(b *Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Send initial keep-alive comment so the client knows the connection is live.
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		ch := b.subscribe()
		defer b.unsubscribe(ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				data, err := json.Marshal(e.Data)
				if err != nil {
					logger.Warn("[sse] failed to marshal event data: %v", err)
					continue
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
				flusher.Flush()
			}
		}
	}
}
