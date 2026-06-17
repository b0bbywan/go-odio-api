package cache

import (
	"sync"
	"testing"
	"time"
)

func TestValueLoadBeforeStore(t *testing.T) {
	var v Value[*string]
	if got := v.Load(); got != nil {
		t.Fatalf("Load before Store: value = %v, want nil", got)
	}
	if !v.UpdatedAt().IsZero() {
		t.Fatalf("UpdatedAt before Store = %v, want zero", v.UpdatedAt())
	}
}

func TestValueStoreLoad(t *testing.T) {
	var v Value[int]
	v.Store(42)
	if got := v.Load(); got != 42 {
		t.Fatalf("Load after Store = %d, want 42", got)
	}
	if v.UpdatedAt().IsZero() {
		t.Fatal("UpdatedAt after Store is zero, want a timestamp")
	}
}

func TestValueReset(t *testing.T) {
	var v Value[int]
	v.Store(7)
	v.Reset()
	if got := v.Load(); got != 0 {
		t.Fatalf("Load after Reset = %d, want 0", got)
	}
	if !v.UpdatedAt().IsZero() {
		t.Fatalf("UpdatedAt after Reset = %v, want zero", v.UpdatedAt())
	}
}

func TestValueUpdatedAtAdvances(t *testing.T) {
	var v Value[int]
	v.Store(1)
	first := v.UpdatedAt()
	time.Sleep(time.Millisecond)
	v.Store(2)
	if !v.UpdatedAt().After(first) {
		t.Fatalf("UpdatedAt did not advance on second Store: %v then %v", first, v.UpdatedAt())
	}
}

// Exercises the lock-free path under -race: concurrent stores and loads must
// not race and Load must always observe a consistent value.
func TestValueConcurrent(t *testing.T) {
	var v Value[int]
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(n int) { defer wg.Done(); v.Store(n) }(i)
		go func() { defer wg.Done(); v.Load() }()
	}
	wg.Wait()
	if !v.UpdatedAt().After(time.Time{}) {
		t.Fatal("UpdatedAt after concurrent stores is zero, want a timestamp")
	}
}
