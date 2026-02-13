package backend

import (
	"context"
	"net"
	"testing"

	"github.com/b0bbywan/go-odio-api/config"
)

// TestBackendDisabled verifies that backends are nil when disabled in config
func TestBackendDisabled(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		bluetoothEnabled bool
		mprisEnabled     bool
		pulseEnabled     bool
		systemdEnabled   bool
		zeroconfEnabled  bool
	}{
		{
			name:             "all backends disabled",
			bluetoothEnabled: false,
			mprisEnabled:     false,
			pulseEnabled:     false,
			systemdEnabled:   false,
			zeroconfEnabled:  false,
		},
		{
			name:             "only bluetooth enabled",
			bluetoothEnabled: true,
			mprisEnabled:     false,
			pulseEnabled:     false,
			systemdEnabled:   false,
			zeroconfEnabled:  false,
		},
		{
			name:             "only systemd enabled",
			bluetoothEnabled: false,
			mprisEnabled:     false,
			pulseEnabled:     false,
			systemdEnabled:   true,
			zeroconfEnabled:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mprisCfg := &config.MPRISConfig{Enabled: tt.mprisEnabled}
			pulseCfg := &config.PulseAudioConfig{Enabled: tt.pulseEnabled}
			// Add empty services for systemd to ensure it returns nil when enabled without services
			systemdCfg := &config.SystemdConfig{
				Enabled:        tt.systemdEnabled,
				SystemServices: []string{},
				UserServices:   []string{},
			}
			zeroconfCfg := &config.ZeroConfig{Enabled: tt.zeroconfEnabled}

			backend, err := New(ctx, mprisCfg, pulseCfg, systemdCfg, zeroconfCfg)

			// Bluetooth and other D-Bus backends may fail in test environment
			// This is expected and we should skip the test
			if err != nil && tt.bluetoothEnabled {
				t.Skipf("Skipping test - D-Bus connection failed (expected in test env): %v", err)
			}

			if err != nil && !tt.bluetoothEnabled {
				t.Fatalf("New() unexpected error when bluetooth disabled: %v", err)
			}

			if backend == nil {
				t.Fatal("New() should return a non-nil Backend struct")
			}

			// Check MPRIS
			if !tt.mprisEnabled && backend.MPRIS != nil {
				t.Error("MPRIS should be nil when disabled")
			}

			// Check PulseAudio
			if !tt.pulseEnabled && backend.Pulse != nil {
				t.Error("PulseAudio should be nil when disabled")
			}

			// Check Systemd - with empty services list, should be nil even when enabled
			if !tt.systemdEnabled && backend.Systemd != nil {
				t.Error("Systemd should be nil when disabled")
			}
			// Note: With empty services, systemd will be nil even when enabled

			// Check Zeroconf
			if !tt.zeroconfEnabled && backend.Zeroconf != nil {
				t.Error("Zeroconf should be nil when disabled")
			}
		})
	}
}

// TestSystemdWithEmptyConfig verifies systemd backend is nil with no services configured
func TestSystemdWithEmptyConfig(t *testing.T) {
	ctx := context.Background()

	systemdCfg := &config.SystemdConfig{
		Enabled:        true,
		SystemServices: []string{}, // No services
		UserServices:   []string{}, // No services
	}

	backend, err := New(
		ctx,
		&config.MPRISConfig{Enabled: false},
		&config.PulseAudioConfig{Enabled: false},
		systemdCfg,
		&config.ZeroConfig{Enabled: false},
	)

	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	if backend.Systemd != nil {
		t.Error("Systemd should be nil when enabled but no services configured")
	}
}

// TestZeroconfWithLocalhostBind verifies zeroconf is disabled for localhost
func TestZeroconfWithLocalhostBind(t *testing.T) {
	ctx := context.Background()

	zeroconfCfg := &config.ZeroConfig{
		Enabled: true,
		Listen:  []net.Interface{}, // Empty interfaces list (like when bind=127.0.0.1)
	}

	backend, err := New(
		ctx,
		&config.MPRISConfig{Enabled: false},
		&config.PulseAudioConfig{Enabled: false},
		&config.SystemdConfig{Enabled: false},
		zeroconfCfg,
	)

	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	if backend.Zeroconf != nil {
		t.Error("Zeroconf should be nil when no interfaces are configured")
	}
}

// TestBackendStartWithNilBackends verifies Start() doesn't panic with nil backends
func TestBackendStartWithNilBackends(t *testing.T) {
	backend := &Backend{
		MPRIS:    nil,
		Pulse:    nil,
		Systemd:  nil,
		Zeroconf: nil,
	}

	// Should not panic
	err := backend.Start()
	if err != nil {
		t.Errorf("Start() should not return error with all backends nil: %v", err)
	}
}

// TestBackendCloseWithNilBackends verifies Close() doesn't panic with nil backends
func TestBackendCloseWithNilBackends(t *testing.T) {
	backend := &Backend{
		MPRIS:    nil,
		Pulse:    nil,
		Systemd:  nil,
		Zeroconf: nil,
	}

	// Should not panic
	backend.Close()
}
