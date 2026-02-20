package bluetooth

import (
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestExtractString(t *testing.T) {
	tests := []struct {
		name     string
		props    map[string]dbus.Variant
		key      string
		expected string
	}{
		{
			name: "valid string",
			props: map[string]dbus.Variant{
				"Name": dbus.MakeVariant("My Device"),
			},
			key:      "Name",
			expected: "My Device",
		},
		{
			name: "missing key",
			props: map[string]dbus.Variant{
				"Other": dbus.MakeVariant("value"),
			},
			key:      "Name",
			expected: "",
		},
		{
			name:     "empty map",
			props:    map[string]dbus.Variant{},
			key:      "Name",
			expected: "",
		},
		{
			name: "wrong type",
			props: map[string]dbus.Variant{
				"Name": dbus.MakeVariant(123),
			},
			key:      "Name",
			expected: "",
		},
		{
			name: "empty string",
			props: map[string]dbus.Variant{
				"Name": dbus.MakeVariant(""),
			},
			key:      "Name",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractString(tt.props, tt.key)
			if result != tt.expected {
				t.Errorf("extractString(%v, %q) = %q, want %q", tt.props, tt.key, result, tt.expected)
			}
		})
	}
}

func TestExtractBoolProp(t *testing.T) {
	tests := []struct {
		name     string
		props    map[string]dbus.Variant
		key      string
		expected bool
	}{
		{
			name: "true value",
			props: map[string]dbus.Variant{
				"Connected": dbus.MakeVariant(true),
			},
			key:      "Connected",
			expected: true,
		},
		{
			name: "false value",
			props: map[string]dbus.Variant{
				"Connected": dbus.MakeVariant(false),
			},
			key:      "Connected",
			expected: false,
		},
		{
			name: "missing key",
			props: map[string]dbus.Variant{
				"Other": dbus.MakeVariant(true),
			},
			key:      "Connected",
			expected: false,
		},
		{
			name:     "empty map",
			props:    map[string]dbus.Variant{},
			key:      "Connected",
			expected: false,
		},
		{
			name: "wrong type string",
			props: map[string]dbus.Variant{
				"Connected": dbus.MakeVariant("true"),
			},
			key:      "Connected",
			expected: false,
		},
		{
			name: "wrong type int",
			props: map[string]dbus.Variant{
				"Connected": dbus.MakeVariant(1),
			},
			key:      "Connected",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBoolProp(tt.props, tt.key)
			if result != tt.expected {
				t.Errorf("extractBoolProp(%v, %q) = %v, want %v", tt.props, tt.key, result, tt.expected)
			}
		})
	}
}

func TestExtractBool(t *testing.T) {
	tests := []struct {
		name       string
		variant    dbus.Variant
		expectedOk bool
		expected   bool
	}{
		{
			name:       "true value",
			variant:    dbus.MakeVariant(true),
			expectedOk: true,
			expected:   true,
		},
		{
			name:       "false value",
			variant:    dbus.MakeVariant(false),
			expectedOk: true,
			expected:   false,
		},
		{
			name:       "wrong type string",
			variant:    dbus.MakeVariant("true"),
			expectedOk: false,
			expected:   false,
		},
		{
			name:       "wrong type int",
			variant:    dbus.MakeVariant(1),
			expectedOk: false,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := extractBool(tt.variant)
			if ok != tt.expectedOk {
				t.Errorf("extractBool(%v) ok = %v, want %v", tt.variant, ok, tt.expectedOk)
			}
			if result != tt.expected {
				t.Errorf("extractBool(%v) = %v, want %v", tt.variant, result, tt.expected)
			}
		})
	}
}

func TestBluetoothStateToString(t *testing.T) {
	tests := []struct {
		name     string
		state    BluetoothState
		expected string
	}{
		{
			name:     "powered",
			state:    BT_STATE_POWERED,
			expected: "Powered",
		},
		{
			name:     "discoverable",
			state:    BT_STATE_DISCOVERABLE,
			expected: "Discoverable",
		},
		{
			name:     "pairable",
			state:    BT_STATE_PAIRABLE,
			expected: "Pairable",
		},
		{
			name:     "trusted",
			state:    BT_STATE_TRUSTED,
			expected: "Trusted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.state.toString()
			if result != tt.expected {
				t.Errorf("toString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// onPropertiesChanged tests — signal parsing paths that don't touch D-Bus
func TestOnPropertiesChangedPaired(t *testing.T) {
	b := &BluetoothBackend{}

	tests := []struct {
		name     string
		signal   *dbus.Signal
		expected bool
	}{
		{
			name:     "nil signal",
			signal:   nil,
			expected: true,
		},
		{
			name:     "empty body",
			signal:   &dbus.Signal{Path: "/dev/1", Body: []interface{}{}},
			expected: false,
		},
		{
			name:     "body too short",
			signal:   &dbus.Signal{Path: "/dev/1", Body: []interface{}{"org.bluez.Device1"}},
			expected: false,
		},
		{
			name: "body[1] wrong type",
			signal: &dbus.Signal{
				Path: "/dev/1",
				Body: []interface{}{"org.bluez.Device1", "not a map"},
			},
			expected: false,
		},
		{
			name: "no Paired key",
			signal: &dbus.Signal{
				Path: "/dev/1",
				Body: []interface{}{
					"org.bluez.Device1",
					map[string]dbus.Variant{
						"Name": dbus.MakeVariant("Speaker"),
					},
				},
			},
			expected: false,
		},
		{
			name: "Paired wrong type",
			signal: &dbus.Signal{
				Path: "/dev/1",
				Body: []interface{}{
					"org.bluez.Device1",
					map[string]dbus.Variant{
						"Paired": dbus.MakeVariant("yes"),
					},
				},
			},
			expected: false,
		},
		{
			name: "Paired false",
			signal: &dbus.Signal{
				Path: "/dev/1",
				Body: []interface{}{
					"org.bluez.Device1",
					map[string]dbus.Variant{
						"Paired": dbus.MakeVariant(false),
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := b.onPropertiesChanged(tt.signal)
			if result != tt.expected {
				t.Errorf("onPropertiesChanged() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// onPropertiesChanged Connected signal parsing tests
func TestOnPropertiesChangedConnected(t *testing.T) {
	b := &BluetoothBackend{}

	tests := []struct {
		name     string
		signal   *dbus.Signal
		expected bool
	}{
		{
			name:     "empty body",
			signal:   &dbus.Signal{Path: "/dev/1", Body: []interface{}{}},
			expected: false,
		},
		{
			name:     "body too short",
			signal:   &dbus.Signal{Path: "/dev/1", Body: []interface{}{"org.bluez.Device1"}},
			expected: false,
		},
		{
			name: "body[1] wrong type",
			signal: &dbus.Signal{
				Path: "/dev/1",
				Body: []interface{}{"org.bluez.Device1", 42},
			},
			expected: false,
		},
		{
			name: "no Connected key",
			signal: &dbus.Signal{
				Path: "/dev/1",
				Body: []interface{}{
					"org.bluez.Device1",
					map[string]dbus.Variant{
						"Name": dbus.MakeVariant("Speaker"),
					},
				},
			},
			expected: false,
		},
		{
			name: "Connected wrong type",
			signal: &dbus.Signal{
				Path: "/dev/1",
				Body: []interface{}{
					"org.bluez.Device1",
					map[string]dbus.Variant{
						"Connected": dbus.MakeVariant("yes"),
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := b.onPropertiesChanged(tt.signal)
			if result != tt.expected {
				t.Errorf("onPropertiesChanged() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// onPropertiesChanged always returns false for valid signals (never stops the listener)
func TestOnPropertiesChangedNeverStops(t *testing.T) {
	b := &BluetoothBackend{}

	sig := &dbus.Signal{
		Path: "/dev/1",
		Body: []interface{}{
			"org.bluez.Device1",
			map[string]dbus.Variant{
				"Connected": dbus.MakeVariant(true),
			},
		},
	}

	result := b.onPropertiesChanged(sig)
	if result != false {
		t.Errorf("onPropertiesChanged(connected=true) = %v, want false", result)
	}
}

func TestCancelIdleTimer(t *testing.T) {
	t.Run("cancels running timer", func(t *testing.T) {
		b := &BluetoothBackend{}
		b.idleTimer = time.AfterFunc(time.Hour, func() {
			t.Error("timer should have been cancelled")
		})

		b.cancelIdleTimer()

		if b.idleTimer != nil {
			t.Error("idleTimer should be nil after cancel")
		}
	})

	t.Run("noop when no timer", func(t *testing.T) {
		b := &BluetoothBackend{}
		b.cancelIdleTimer() // should not panic
		if b.idleTimer != nil {
			t.Error("idleTimer should remain nil")
		}
	})

	t.Run("connected signal cancels idle timer", func(t *testing.T) {
		b := &BluetoothBackend{
			idleTimerMu: sync.Mutex{},
		}
		b.idleTimer = time.AfterFunc(time.Hour, func() {
			t.Error("timer should have been cancelled by connected signal")
		})

		sig := &dbus.Signal{
			Path: "/dev/1",
			Body: []interface{}{
				"org.bluez.Device1",
				map[string]dbus.Variant{
					"Connected": dbus.MakeVariant(true),
				},
			},
		}

		b.onPropertiesChanged(sig)

		if b.idleTimer != nil {
			t.Error("idleTimer should be nil after connected=true signal")
		}
	})
}
