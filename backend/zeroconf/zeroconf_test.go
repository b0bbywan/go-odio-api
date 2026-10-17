// backend/zeroconf/zeroconf_test.go
package zeroconf

import (
	"context"
	"errors"
	"math"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/config"
)

func TestNew_Disabled(t *testing.T) {
	cfg := &config.ZeroConfig{Enabled: false}
	backend, err := New(context.Background(), cfg)

	if err != nil {
		t.Errorf("New() with disabled config returned error: %v", err)
	}
	if backend != nil {
		t.Error("New() with disabled config should return nil backend")
	}
}

func TestNew_NoInterfaces(t *testing.T) {
	cfg := &config.ZeroConfig{
		Enabled: true,
		Listen:  []net.Interface{}, // empty
	}
	backend, err := New(context.Background(), cfg)

	if err != nil {
		t.Errorf("New() with no interfaces returned error: %v", err)
	}
	if backend != nil {
		t.Error("New() with no interfaces should return nil backend")
	}
}

func TestNew_WithInterfaces(t *testing.T) {
	// Get a real interface for testing
	ifaces, err := net.Interfaces()
	if err != nil || len(ifaces) == 0 {
		t.Skip("No network interfaces available for testing")
	}

	cfg := &config.ZeroConfig{
		Enabled:      true,
		InstanceName: "test-instance",
		ServiceType:  "_http._tcp",
		Domain:       "local.",
		Port:         8018,
		TxtRecords:   []string{"version=test"},
		Listen:       []net.Interface{ifaces[0]},
	}

	ctx := context.Background()
	backend, err := New(ctx, cfg)

	if err != nil {
		t.Fatalf("New() with valid config returned error: %v", err)
	}
	if backend == nil {
		t.Fatal("New() with valid config should return non-nil backend")
	}

	// Verify config was stored
	if backend.Config != cfg {
		t.Error("backend.Config should match provided config")
	}

	// Verify context was stored
	if backend.ctx == nil {
		t.Error("backend.ctx should not be nil")
	}

	// Verify cancel func exists
	if backend.cancel == nil {
		t.Error("backend.cancel should not be nil")
	}

	// Clean up
	backend.Close()
}

func TestClose_NilServer(t *testing.T) {
	z := &ZeroConfBackend{}
	// Should not panic
	z.Close()
}

func TestClose_NilCancel(t *testing.T) {
	z := &ZeroConfBackend{
		cancel: nil,
	}
	// Should not panic
	z.Close()
}

func TestClose_Idempotent(t *testing.T) {
	z := &ZeroConfBackend{}

	// Multiple calls should not panic
	z.Close()
	z.Close()
	z.Close()
}

type fakeServer struct{ shutdowns atomic.Int32 }

func (s *fakeServer) Shutdown() { s.shutdowns.Add(1) }

// newFake returns a backend whose register fails `failures` times, then succeeds.
func newFake(t *testing.T, failures int32) (*ZeroConfBackend, *atomic.Int32, *fakeServer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var calls atomic.Int32
	srv := &fakeServer{}
	z := &ZeroConfBackend{
		Config: &config.ZeroConfig{InstanceName: "test"},
		ctx:    ctx,
		cancel: cancel,
		register: func(*config.ZeroConfig) (server, error) {
			if calls.Add(1) <= failures {
				return nil, errors.New("no IP yet")
			}
			return srv, nil
		},
		retryDelay: time.Millisecond,
		retryMax:   4 * time.Millisecond,
	}
	return z, &calls, srv
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before deadline")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestStart_RetriesUntilPublished(t *testing.T) {
	z, calls, srv := newFake(t, 3)

	if err := z.Start(); err != nil {
		t.Fatalf("Start() should not fail when publishing fails, got %v", err)
	}

	waitFor(t, func() bool {
		z.mu.Lock()
		defer z.mu.Unlock()
		return z.server == srv
	})
	if got := calls.Load(); got != 4 {
		t.Errorf("register calls = %d, want 4 (3 failures + 1 success)", got)
	}

	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 4 {
		t.Errorf("register called again after success: %d calls", got)
	}
}

func TestClose_StopsRetry(t *testing.T) {
	z, calls, srv := newFake(t, math.MaxInt32)

	if err := z.Start(); err != nil {
		t.Fatalf("Start() returned %v", err)
	}
	waitFor(t, func() bool { return calls.Load() >= 2 })

	z.Close()
	time.Sleep(10 * time.Millisecond) // let an in-flight attempt finish
	stopped := calls.Load()
	time.Sleep(20 * time.Millisecond)

	if got := calls.Load(); got != stopped {
		t.Errorf("register still called after Close: %d -> %d", stopped, got)
	}
	if srv.shutdowns.Load() != 0 {
		t.Error("server should never have been published")
	}
}

func TestClose_ShutsDownPublishedServer(t *testing.T) {
	z, _, srv := newFake(t, 0)

	if err := z.Start(); err != nil {
		t.Fatalf("Start() returned %v", err)
	}
	z.Close()

	if got := srv.shutdowns.Load(); got != 1 {
		t.Errorf("Shutdown calls = %d, want 1", got)
	}
}
