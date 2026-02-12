// backend/zeroconf/zeroconf_test.go
package zeroconf

import (
	"context"
	"net"
	"testing"

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
