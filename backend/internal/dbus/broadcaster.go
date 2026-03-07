package dbus

import (
	"context"
	"sync"
)

// Broadcaster fans out items from a single upstream channel to all subscribers.
type Broadcaster[T any] struct {
	mu      sync.RWMutex
	clients map[chan T]func(T) bool
}

// NewBroadcaster starts a broadcaster that reads from upstream and fans out to
// all subscribers. It stops when ctx is cancelled or upstream is closed.
func NewBroadcaster[T any](ctx context.Context, upstream <-chan T) *Broadcaster[T] {
	b := &Broadcaster[T]{
		clients: make(map[chan T]func(T) bool),
	}
	go b.run(ctx, upstream)
	return b
}

// Subscribe registers a new subscriber (no filter — all items pass) and returns
// its dedicated channel (buffered, size 32).
func (b *Broadcaster[T]) Subscribe() chan T {
	return b.SubscribeFunc(nil)
}

// SubscribeFunc registers a new subscriber with an optional filter function.
// Only items for which filter returns true are delivered to the channel.
// A nil filter passes all items.
func (b *Broadcaster[T]) SubscribeFunc(filter func(T) bool) chan T {
	ch := make(chan T, 32)
	b.mu.Lock()
	b.clients[ch] = filter
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *Broadcaster[T]) Unsubscribe(ch chan T) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *Broadcaster[T]) broadcast(v T) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch, filter := range b.clients {
		if filter != nil && !filter(v) {
			continue
		}
		select {
		case ch <- v:
		default:
		}
	}
}

func (b *Broadcaster[T]) run(ctx context.Context, upstream <-chan T) {
	for {
		select {
		case <-ctx.Done():
			return
		case v, ok := <-upstream:
			if !ok {
				return
			}
			b.broadcast(v)
		}
	}
}

// FanIn merges multiple channels into one.
// Nil sources are skipped. The merged channel is closed when all sources exit
// or ctx is cancelled.
func FanIn[T any](ctx context.Context, sources ...<-chan T) <-chan T {
	merged := make(chan T, 64)
	var wg sync.WaitGroup

	for _, src := range sources {
		if src == nil {
			continue
		}
		wg.Add(1)
		go func(ch <-chan T) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case v, ok := <-ch:
					if !ok {
						return
					}
					select {
					case merged <- v:
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
