package cache

import (
	"sync"
	"time"
)

type Entry[T any] struct {
	Value     T
	ExpiresAt time.Time
}

func (e Entry[T]) IsExpired() bool {
	// Si ExpiresAt est zero value, le cache n'expire jamais
	if e.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(e.ExpiresAt)
}

type Cache[T any] struct {
	mu      sync.RWMutex
	entries map[string]Entry[T]
	ttl     time.Duration
}

func New[T any](ttl time.Duration) *Cache[T] {
	return &Cache[T]{
		entries: make(map[string]Entry[T]),
		ttl:     ttl,
	}
}

func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		var zero T
		return zero, false
	}

	if entry.IsExpired() {
		var zero T
		return zero, false
	}

	return entry.Value, true
}

func (c *Cache[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiresAt time.Time
	if c.ttl > 0 {
		expiresAt = time.Now().Add(c.ttl)
	}
	// Si ttl == 0, expiresAt reste à zero value = pas d'expiration

	c.entries[key] = Entry[T]{
		Value:     value,
		ExpiresAt: expiresAt,
	}

}

func (c *Cache[T]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

func (c *Cache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]Entry[T])
}

func (c *Cache[T]) CleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, key)
		}
	}
}
