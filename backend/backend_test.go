package backend

import (
	"context"
	"net"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/login1"
	"github.com/b0bbywan/go-odio-api/config"
)

// TestBackendDisabled verifies that backends are nil when disabled in config
func TestBackendDisabled(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		bluetoothEnabled bool
		login1Enabled    bool
		mprisEnabled     bool
		pulseEnabled     bool
		systemdEnabled   bool
		zeroconfEnabled  bool
	}{
		{
			name:             "all backends disabled",
			bluetoothEnabled: false,
			login1Enabled:    false,
			mprisEnabled:     false,
			pulseEnabled:     false,
			systemdEnabled:   false,
			zeroconfEnabled:  false,
		},
		{
			name:             "only bluetooth enabled",
			bluetoothEnabled: true,
			login1Enabled:    false,
			mprisEnabled:     false,
			pulseEnabled:     false,
			systemdEnabled:   false,
			zeroconfEnabled:  false,
		},
		{
			name:             "only systemd enabled",
			bluetoothEnabled: false,
			login1Enabled:    false,
			mprisEnabled:     false,
			pulseEnabled:     false,
			systemdEnabled:   true,
			zeroconfEnabled:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			login1Cfg := &config.Login1Config{Enabled: tt.login1Enabled}
			mprisCfg := &config.MPRISConfig{Enabled: tt.mprisEnabled}
			pulseCfg := &config.PulseAudioConfig{Enabled: tt.pulseEnabled}
			// Add empty services for systemd to ensure it returns nil when enabled without services
			systemdCfg := &config.SystemdConfig{
				Enabled:        tt.systemdEnabled,
				SystemServices: []string{},
				UserServices:   []string{},
			}
			zeroconfCfg := &config.ZeroConfig{Enabled: tt.zeroconfEnabled}

			backend, err := New(ctx, login1Cfg, mprisCfg, pulseCfg, systemdCfg, zeroconfCfg)

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
		&config.Login1Config{Enabled: false},
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
		&config.Login1Config{Enabled: false},
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

// --- Tests Login1 ---

// TestLogin1DisabledInBackend verifies that Login1 field stays nil when disabled
func TestLogin1DisabledInBackend(t *testing.T) {
	ctx := context.Background()

	login1Cfg := &config.Login1Config{Enabled: false}

	backend, err := New(
		ctx,
		login1Cfg,
		&config.MPRISConfig{Enabled: false},
		&config.PulseAudioConfig{Enabled: false},
		&config.SystemdConfig{Enabled: false},
		&config.ZeroConfig{Enabled: false},
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	// Login1 is not initialised by backend.New(), the field must remain nil
	if backend.Login1 != nil {
		t.Error("Login1 should be nil when disabled")
	}
}

// TestLogin1DisabledWithCapabilities verifies Login1 stays nil even when capabilities are set but backend is disabled
func TestLogin1DisabledWithCapabilities(t *testing.T) {
	ctx := context.Background()

	login1Cfg := &config.Login1Config{
		Enabled: false,
		Capabilities: &config.Login1Capabilities{
			CanReboot:   true,
			CanPoweroff: true,
		},
	}

	backend, err := New(
		ctx,
		login1Cfg,
		&config.MPRISConfig{Enabled: false},
		&config.PulseAudioConfig{Enabled: false},
		&config.SystemdConfig{Enabled: false},
		&config.ZeroConfig{Enabled: false},
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	if backend.Login1 != nil {
		t.Error("Login1 should be nil even with capabilities when disabled")
	}
}

// TestBackendClose_WithLogin1Nil verifies Close() doesn't panic when Login1 is nil
func TestBackendClose_WithLogin1Nil(t *testing.T) {
	backend := &Backend{
		Login1:   nil,
		MPRIS:    nil,
		Pulse:    nil,
		Systemd:  nil,
		Zeroconf: nil,
	}

	// Should not panic
	backend.Close()
}

// TestBackendNew_Login1FieldInitialisedToNil verifies the Login1 field is nil after New() with disabled config
func TestBackendNew_Login1FieldInitialisedToNil(t *testing.T) {
	ctx := context.Background()

	backend, err := New(
		ctx,
		&config.Login1Config{Enabled: false},
		&config.MPRISConfig{Enabled: false},
		&config.PulseAudioConfig{Enabled: false},
		&config.SystemdConfig{Enabled: false, SystemServices: []string{}, UserServices: []string{}},
		&config.ZeroConfig{Enabled: false},
	)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if backend == nil {
		t.Fatal("New() should return a non-nil Backend")
	}
	if backend.Login1 != nil {
		t.Error("Backend.Login1 should be nil when Login1 is not initialised by New()")
	}
}

// TestGetServerDeviceInfo_PowerField tests that the Power flag in Backends reflects Login1 presence
func TestGetServerDeviceInfo_PowerField(t *testing.T) {
	tests := []struct {
		name      string
		login1    *login1.Login1Backend
		wantPower bool
	}{
		{
			name:      "Login1 nil → Power false",
			login1:    nil,
			wantPower: false,
		},
		{
			name:      "Login1 set (reboot only) → Power true",
			login1:    &login1.Login1Backend{CanReboot: true},
			wantPower: true,
		},
		{
			name:      "Login1 set (poweroff only) → Power true",
			login1:    &login1.Login1Backend{CanPoweroff: true},
			wantPower: true,
		},
		{
			name:      "Login1 set (both) → Power true",
			login1:    &login1.Login1Backend{CanReboot: true, CanPoweroff: true},
			wantPower: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backend{Login1: tt.login1}
			info, err := b.GetServerDeviceInfo()
			if err != nil {
				t.Fatalf("GetServerDeviceInfo() returned error: %v", err)
			}
			if info.Backends.Power != tt.wantPower {
				t.Errorf("Backends.Power = %v, want %v", info.Backends.Power, tt.wantPower)
			}
		})
	}
}

// TestNew_Login1NoCapabilityEnabled_RequiresDbus documents that New() returns nil when
// all capabilities are disabled, even if the backend is enabled (requires D-Bus to reach that path).
func TestNew_Login1NoCapabilityEnabled_RequiresDbus(t *testing.T) {
	t.Skip("reaching the 'no capability enabled' early-return requires a live D-Bus system connection; tested via integration tests")
}
