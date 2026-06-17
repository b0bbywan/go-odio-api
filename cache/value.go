package cache

import (
	"sync/atomic"
	"time"
)

// Value is a lock-free holder for a single value plus its last-write time.
type Value[T any] struct {
	ptr atomic.Pointer[stamped[T]]
}

type stamped[T any] struct {
	val       T
	updatedAt time.Time
}

// Load returns the stored value, or the zero value (nil for pointers) if never stored.
func (v *Value[T]) Load() T {
	if s := v.ptr.Load(); s != nil {
		return s.val
	}
	var zero T
	return zero
}

// Store sets the value and stamps the write time.
func (v *Value[T]) Store(val T) {
	v.ptr.Store(&stamped[T]{val: val, updatedAt: time.Now()})
}

// Reset clears the value; Load then reports ok=false again.
func (v *Value[T]) Reset() {
	v.ptr.Store(nil)
}

// UpdatedAt returns the last Store time, or the zero time if never stored.
func (v *Value[T]) UpdatedAt() time.Time {
	if s := v.ptr.Load(); s != nil {
		return s.updatedAt
	}
	return time.Time{}
}
