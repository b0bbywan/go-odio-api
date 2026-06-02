package bluetooth

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestDevicePath(t *testing.T) {
	tests := []struct {
		address string
		want    dbus.ObjectPath
	}{
		{"40:C1:F6:D4:67:88", "/org/bluez/hci0/dev_40_C1_F6_D4_67_88"},
		{"aa:bb:cc:dd:ee:ff", "/org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF"},
	}
	for _, tt := range tests {
		if got := devicePath(tt.address); got != tt.want {
			t.Errorf("devicePath(%q) = %q, want %q", tt.address, got, tt.want)
		}
	}
}

func TestAddressFromPath(t *testing.T) {
	tests := []struct {
		path dbus.ObjectPath
		want string
	}{
		{"/org/bluez/hci0/dev_40_C1_F6_D4_67_88", "40:C1:F6:D4:67:88"},
		{"/org/bluez/hci0", ""},
		{"/org/bluez/hci0/dev_40_C1_F6_D4_67_88/fd0", "40:C1:F6:D4:67:88/fd0"},
	}
	for _, tt := range tests {
		if got := addressFromPath(tt.path); got != tt.want {
			t.Errorf("addressFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// devicePath and addressFromPath round-trip for a canonical (upper-case) address.
func TestDevicePathRoundTrip(t *testing.T) {
	const addr = "40:C1:F6:D4:67:88"
	if got := addressFromPath(devicePath(addr)); got != addr {
		t.Errorf("round-trip = %q, want %q", got, addr)
	}
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		address string
		wantErr bool
	}{
		{"40:C1:F6:D4:67:88", false},
		{"aa:bb:cc:dd:ee:ff", false},
		{"40-C1-F6-D4-67-88", true},
		{"40:C1:F6:D4:67", true},
		{"", true},
		{"not-a-mac", true},
	}
	for _, tt := range tests {
		err := validateAddress(tt.address)
		if tt.wantErr {
			if err == nil {
				t.Errorf("validateAddress(%q) = nil, want error", tt.address)
			} else if !errors.Is(err, ErrInvalidAddress) {
				t.Errorf("validateAddress(%q) error = %v, want ErrInvalidAddress", tt.address, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("validateAddress(%q) = %v, want nil", tt.address, err)
		}
	}
}

func TestParseInterfacesAdded(t *testing.T) {
	devProps := map[string]dbus.Variant{
		"Address": dbus.MakeVariant("40:C1:F6:D4:67:88"),
	}

	t.Run("device interface", func(t *testing.T) {
		sig := &dbus.Signal{Body: []interface{}{
			dbus.ObjectPath("/org/bluez/hci0/dev_40_C1_F6_D4_67_88"),
			map[string]map[string]dbus.Variant{BLUETOOTH_DEVICE: devProps},
		}}
		path, props, ok := parseInterfacesAdded(sig)
		if !ok {
			t.Fatal("expected ok=true for Device1 interface")
		}
		if path != "/org/bluez/hci0/dev_40_C1_F6_D4_67_88" {
			t.Errorf("path = %q", path)
		}
		if extractString(props, BT_PROP_ADDRESS) != "40:C1:F6:D4:67:88" {
			t.Errorf("address not parsed: %v", props)
		}
	})

	t.Run("non-device interface", func(t *testing.T) {
		sig := &dbus.Signal{Body: []interface{}{
			dbus.ObjectPath("/org/bluez/hci0"),
			map[string]map[string]dbus.Variant{BLUETOOTH_ADAPTER: {}},
		}}
		if _, _, ok := parseInterfacesAdded(sig); ok {
			t.Error("expected ok=false for non-Device1 interface")
		}
	})

	t.Run("malformed body", func(t *testing.T) {
		if _, _, ok := parseInterfacesAdded(&dbus.Signal{Body: []interface{}{"x"}}); ok {
			t.Error("expected ok=false for short body")
		}
		if _, _, ok := parseInterfacesAdded(nil); ok {
			t.Error("expected ok=false for nil signal")
		}
	})
}
