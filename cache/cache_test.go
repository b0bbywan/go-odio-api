package cache

import (
	"testing"
	"time"
)

func TestCacheSet(t *testing.T) {
	c := New[string](0)
	c.Set("key1", "value1")

	val, exists := c.Get("key1")
	if !exists {
		t.Fatal("key1 should exist")
	}
	if val != "value1" {
		t.Fatalf("expected 'value1', got '%s'", val)
	}
}

func TestCacheGetMissing(t *testing.T) {
	c := New[string](0)

	_, exists := c.Get("missing")
	if exists {
		t.Fatal("missing key should not exist")
	}
}

func TestCacheTTL(t *testing.T) {
	c := New[string](100 * time.Millisecond)
	c.Set("key1", "value1")

	// Should exist immediately
	_, exists := c.Get("key1")
	if !exists {
		t.Fatal("key1 should exist immediately after set")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	_, exists = c.Get("key1")
	if exists {
		t.Fatal("key1 should be expired after TTL")
	}
}

func TestCacheZeroTTL(t *testing.T) {
	c := New[string](0) // TTL=0 means never expire
	c.Set("key1", "value1")

	// Wait and check it's still there
	time.Sleep(100 * time.Millisecond)

	val, exists := c.Get("key1")
	if !exists {
		t.Fatal("key1 should never expire with TTL=0")
	}
	if val != "value1" {
		t.Fatalf("expected 'value1', got '%s'", val)
	}
}

func TestCacheDelete(t *testing.T) {
	c := New[string](0)
	c.Set("key1", "value1")

	c.Delete("key1")

	_, exists := c.Get("key1")
	if exists {
		t.Fatal("key1 should be deleted")
	}
}

func TestCacheClear(t *testing.T) {
	c := New[string](0)
	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Set("key3", "value3")

	c.Clear()

	_, exists1 := c.Get("key1")
	_, exists2 := c.Get("key2")
	_, exists3 := c.Get("key3")

	if exists1 || exists2 || exists3 {
		t.Fatal("all keys should be cleared")
	}
}

func TestCacheCleanExpired(t *testing.T) {
	c := New[string](100 * time.Millisecond)
	c.Set("key1", "value1")
	c.Set("key2", "value2")

	time.Sleep(150 * time.Millisecond)

	c.Set("key3", "value3") // This one should not expire

	c.CleanExpired()

	_, exists1 := c.Get("key1")
	_, exists2 := c.Get("key2")
	_, exists3 := c.Get("key3")

	if exists1 || exists2 {
		t.Fatal("expired keys should be cleaned")
	}
	if !exists3 {
		t.Fatal("non-expired key3 should still exist")
	}
}

func TestCacheThreadSafety(t *testing.T) {
	c := New[int](0)
	done := make(chan bool, 10)

	// 5 goroutines writing
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				c.Set("key", id*100+j)
			}
			done <- true
		}(i)
	}

	// 5 goroutines reading
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.Get("key")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic from race conditions
	_, _ = c.Get("key")
}
