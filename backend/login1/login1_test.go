package login1

import (
	"context"
	"errors"
	"testing"

	"github.com/b0bbywan/go-odio-api/config"
)

// --- Tests pour New() ---

func TestNew_NilConfig(t *testing.T) {
	b, err := New(context.Background(), nil)
	if err != nil {
		t.Errorf("New(nil) should return nil error, got: %v", err)
	}
	if b != nil {
		t.Errorf("New(nil) should return nil backend, got: %v", b)
	}
}

func TestNew_DisabledConfig(t *testing.T) {
	cfg := &config.Login1Config{Enabled: false}
	b, err := New(context.Background(), cfg)
	if err != nil {
		t.Errorf("New(disabled) should return nil error, got: %v", err)
	}
	if b != nil {
		t.Errorf("New(disabled) should return nil backend, got: %v", b)
	}
}

func TestNew_DisabledConfig_WithCapacities(t *testing.T) {
	cfg := &config.Login1Config{
		Enabled: false,
		Capacities: &config.Login1Capacities{
			CanReboot:   true,
			CanPoweroff: true,
		},
	}
	b, err := New(context.Background(), cfg)
	if err != nil {
		t.Errorf("New(disabled with capacities) should return nil error, got: %v", err)
	}
	if b != nil {
		t.Errorf("New(disabled with capacities) should return nil backend, got: %v", b)
	}
}

// --- Tests pour Close() ---

func TestClose_NilConn(t *testing.T) {
	b := &Login1Backend{conn: nil}
	// Should not panic
	b.Close()
}

func TestClose_IdempotentAfterClose(t *testing.T) {
	b := &Login1Backend{conn: nil}
	// Multiple calls should not panic
	b.Close()
	b.Close()
}

// --- Tests pour Reboot() et PowerOff() avec capacités désactivées ---

func TestReboot_CapabilityDisabled(t *testing.T) {
	b := &Login1Backend{CanReboot: false}
	err := b.Reboot()
	if err == nil {
		t.Fatal("Reboot() with CanReboot=false should return an error")
	}
	var capErr *CapabilityError
	if !errors.As(err, &capErr) {
		t.Errorf("Reboot() should return CapabilityError, got: %T: %v", err, err)
	}
}

func TestPowerOff_CapabilityDisabled(t *testing.T) {
	b := &Login1Backend{CanPoweroff: false}
	err := b.PowerOff()
	if err == nil {
		t.Fatal("PowerOff() with CanPoweroff=false should return an error")
	}
	var capErr *CapabilityError
	if !errors.As(err, &capErr) {
		t.Errorf("PowerOff() should return CapabilityError, got: %T: %v", err, err)
	}
}

func TestReboot_CapabilityDisabled_ErrorMessage(t *testing.T) {
	b := &Login1Backend{CanReboot: false}
	err := b.Reboot()
	if err == nil {
		t.Fatal("Reboot() with CanReboot=false should return an error")
	}
	msg := err.Error()
	if msg == "" {
		t.Error("Reboot error message should not be empty")
	}
}

func TestPowerOff_CapabilityDisabled_ErrorMessage(t *testing.T) {
	b := &Login1Backend{CanPoweroff: false}
	err := b.PowerOff()
	if err == nil {
		t.Fatal("PowerOff() with CanPoweroff=false should return an error")
	}
	msg := err.Error()
	if msg == "" {
		t.Error("PowerOff error message should not be empty")
	}
}

// --- Tests pour validateCapabilities() ---

func TestValidateCapabilities_AllDisabled(t *testing.T) {
	b := &Login1Backend{}
	caps := config.Login1Capacities{
		CanReboot:   false,
		CanPoweroff: false,
	}
	err := b.validateCapabilities(caps)
	if err != nil {
		t.Errorf("validateCapabilities(all false) should return nil, got: %v", err)
	}
	if b.CanReboot {
		t.Error("CanReboot should remain false when capacity is disabled")
	}
	if b.CanPoweroff {
		t.Error("CanPoweroff should remain false when capacity is disabled")
	}
}

func TestValidateCapabilities_RebootOnly_RequiresDbus(t *testing.T) {
	// validateCapabilities with CanReboot=true requires a live D-Bus connection.
	// This test documents the expected behaviour and is skipped when D-Bus is unavailable.
	t.Skip("requires a live D-Bus system connection; tested via integration tests")
}

func TestValidateCapabilities_PoweroffOnly_RequiresDbus(t *testing.T) {
	// validateCapabilities with CanPoweroff=true requires a live D-Bus connection.
	// This test documents the expected behaviour and is skipped when D-Bus is unavailable.
	t.Skip("requires a live D-Bus system connection; tested via integration tests")
}

// --- Tests pour les types d'erreurs ---

func TestCapabilityError_ErrorMessage(t *testing.T) {
	tests := []struct {
		required string
		want     string
	}{
		{"reboot capability disabled", "action not allowed (requires reboot capability disabled)"},
		{"poweroff capability disabled", "action not allowed (requires poweroff capability disabled)"},
		{"CanReboot not available", "action not allowed (requires CanReboot not available)"},
	}

	for _, tt := range tests {
		t.Run(tt.required, func(t *testing.T) {
			err := &CapabilityError{Required: tt.required}
			if err.Error() != tt.want {
				t.Errorf("CapabilityError.Error() = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestInvalidBusNameError_ErrorMessage(t *testing.T) {
	err := &InvalidBusNameError{
		BusName: "invalid.bus",
		Reason:  "not a valid D-Bus name",
	}
	msg := err.Error()
	if msg == "" {
		t.Error("InvalidBusNameError.Error() should not be empty")
	}
	expected := "invalid bus name: not a valid D-Bus name"
	if msg != expected {
		t.Errorf("InvalidBusNameError.Error() = %q, want %q", msg, expected)
	}
}

func TestDbusTimeoutError_ErrorMessage(t *testing.T) {
	err := &dbusTimeoutError{}
	msg := err.Error()
	if msg == "" {
		t.Error("dbusTimeoutError.Error() should not be empty")
	}
	expected := "D-Bus call timeout"
	if msg != expected {
		t.Errorf("dbusTimeoutError.Error() = %q, want %q", msg, expected)
	}
}

// Compile-time assertions: these lines fail to compile if the types no longer
// implement the error interface, making the intent explicit without triggering SA4023.
var (
	_ error = (*CapabilityError)(nil)
	_ error = (*InvalidBusNameError)(nil)
	_ error = (*dbusTimeoutError)(nil)
)

// --- Tests pour les constantes ---

func TestConstants_Login1Prefix(t *testing.T) {
	if LOGIN1_PREFIX != "org.freedesktop.login1" {
		t.Errorf("LOGIN1_PREFIX = %q, want %q", LOGIN1_PREFIX, "org.freedesktop.login1")
	}
}

func TestConstants_Login1Path(t *testing.T) {
	if LOGIN1_PATH != "/org/freedesktop/login1" {
		t.Errorf("LOGIN1_PATH = %q, want %q", LOGIN1_PATH, "/org/freedesktop/login1")
	}
}

func TestConstants_Login1Interface(t *testing.T) {
	expected := "org.freedesktop.login1.Manager"
	if LOGIN1_INTERFACE != expected {
		t.Errorf("LOGIN1_INTERFACE = %q, want %q", LOGIN1_INTERFACE, expected)
	}
}

func TestConstants_Login1Methods(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"PowerOff method", LOGIN1_METHOD_POWEROFF, "org.freedesktop.login1.Manager.PowerOff"},
		{"Reboot method", LOGIN1_METHOD_REBOOT, "org.freedesktop.login1.Manager.Reboot"},
		{"CanReboot capability", LOGIN1_CAPABILITY_REBOOT, "org.freedesktop.login1.Manager.CanReboot"},
		{"CanPowerOff capability", LOGIN1_CAPABILITY_POWEROFF, "org.freedesktop.login1.Manager.CanPowerOff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// --- Tests pour la structure Login1Backend ---

func TestLogin1Backend_DefaultValues(t *testing.T) {
	b := &Login1Backend{}
	if b.CanReboot {
		t.Error("CanReboot should be false by default")
	}
	if b.CanPoweroff {
		t.Error("CanPoweroff should be false by default")
	}
}

func TestLogin1Backend_BothCapabilitiesDisabled_BothMethodsFail(t *testing.T) {
	b := &Login1Backend{
		CanReboot:   false,
		CanPoweroff: false,
	}

	rebootErr := b.Reboot()
	if rebootErr == nil {
		t.Error("Reboot() should fail when CanReboot=false")
	}

	poweroffErr := b.PowerOff()
	if poweroffErr == nil {
		t.Error("PowerOff() should fail when CanPoweroff=false")
	}
}

func TestLogin1Backend_RebootError_IsCapabilityError(t *testing.T) {
	b := &Login1Backend{CanReboot: false}
	err := b.Reboot()

	var capErr *CapabilityError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *CapabilityError, got %T", err)
	}
	if capErr.Required != "reboot capability disabled" {
		t.Errorf("CapabilityError.Required = %q, want %q", capErr.Required, "reboot capability disabled")
	}
}

func TestLogin1Backend_PowerOffError_IsCapabilityError(t *testing.T) {
	b := &Login1Backend{CanPoweroff: false}
	err := b.PowerOff()

	var capErr *CapabilityError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *CapabilityError, got %T", err)
	}
	if capErr.Required != "poweroff capability disabled" {
		t.Errorf("CapabilityError.Required = %q, want %q", capErr.Required, "poweroff capability disabled")
	}
}
