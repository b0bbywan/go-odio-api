package backend

import (
	"context"
	"sync"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// Broadcaster fans out events from a single upstream channel to all subscribers.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[chan events.Event]struct{}
}

// NewBroadcaster starts a broadcaster that reads from upstream and fans out to
// all subscribers. It stops when ctx is cancelled or upstream is closed.
func NewBroadcaster(ctx context.Context, upstream <-chan events.Event) *Broadcaster {
	b := &Broadcaster{
		clients: make(map[chan events.Event]struct{}),
	}
	go b.run(ctx, upstream)
	return b
}

// Subscribe registers a new subscriber and returns its dedicated channel (buffered, size 32).
func (b *Broadcaster) Subscribe() chan events.Event {
	ch := make(chan events.Event, 32)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *Broadcaster) Unsubscribe(ch chan events.Event) {
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

// newBroadcasterFromBackend wires all enabled sub-backend event channels into
// a single Broadcaster. Called once by Backend.New().
func newBroadcasterFromBackend(ctx context.Context, b *Backend) *Broadcaster {
	var srcs []<-chan events.Event
	if b.MPRIS != nil {
		srcs = append(srcs, b.MPRIS.Events())
	}
	if b.Pulse != nil {
		srcs = append(srcs, b.Pulse.Events())
	}
	if b.Systemd != nil {
		srcs = append(srcs, b.Systemd.Events())
	}
	return NewBroadcaster(ctx, fanIn(ctx, srcs...))
}

// fanIn merges multiple event channels into one.
// Nil sources are skipped. The merged channel is closed when all sources exit
// or ctx is cancelled.
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
