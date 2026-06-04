package bluetooth

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/events"
)

// newTestBackend builds a backend with the in-memory state a handler touches
// (status cache + events channel), without a D-Bus connection.
func newTestBackend() *BluetoothBackend {
	return &BluetoothBackend{
		statusCache: cache.New[BluetoothStatus](0),
		events:      make(chan events.Event, 16),
	}
}

func (b *BluetoothBackend) seedStatus(s BluetoothStatus) {
	b.statusCache.Set("current", s)
}

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
		key      BluetoothState
		expected bool
	}{
		{
			name: "true value",
			props: map[string]dbus.Variant{
				"Connected": dbus.MakeVariant(true),
			},
			key:      BT_STATE_CONNECTED,
			expected: true,
		},
		{
			name: "false value",
			props: map[string]dbus.Variant{
				"Connected": dbus.MakeVariant(false),
			},
			key:      BT_STATE_CONNECTED,
			expected: false,
		},
		{
			name: "missing key",
			props: map[string]dbus.Variant{
				"Other": dbus.MakeVariant(true),
			},
			key:      BT_STATE_CONNECTED,
			expected: false,
		},
		{
			name:     "empty map",
			props:    map[string]dbus.Variant{},
			key:      BT_STATE_CONNECTED,
			expected: false,
		},
		{
			name: "wrong type string",
			props: map[string]dbus.Variant{
				"Connected": dbus.MakeVariant("true"),
			},
			key:      BT_STATE_CONNECTED,
			expected: false,
		},
		{
			name: "wrong type int",
			props: map[string]dbus.Variant{
				"Connected": dbus.MakeVariant(1),
			},
			key:      BT_STATE_CONNECTED,
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
			result := tt.state.String()
			if result != tt.expected {
				t.Errorf("toString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// onSignal tests — signal parsing paths that don't touch D-Bus
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
			result := b.onSignal(tt.signal)
			if result != tt.expected {
				t.Errorf("onSignal() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// onSignal Connected signal parsing tests
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
			result := b.onSignal(tt.signal)
			if result != tt.expected {
				t.Errorf("onSignal() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// onSignal always returns false for valid signals (never stops the listener)
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

	result := b.onSignal(sig)
	if result != false {
		t.Errorf("onSignal(connected=true) = %v, want false", result)
	}
}

func TestCancelIdleTimer(t *testing.T) {
	t.Run("connected signal cancels idle timer", func(t *testing.T) {
		b := &BluetoothBackend{}
		b.idleTimer.Start(time.Hour, func() {
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

		b.onSignal(sig)

		if b.idleTimer.timer != nil {
			t.Error("idleTimer should be nil after connected=true signal")
		}
	})
}

func TestManagedTimer(t *testing.T) {
	t.Run("Start arms when duration is non-zero", func(t *testing.T) {
		var mt managedTimer
		if !mt.Start(time.Hour, func() {}) {
			t.Error("Start should report it armed the timer")
		}
		if mt.timer == nil {
			t.Error("timer should be set")
		}
		mt.Cancel()
	})

	t.Run("Start is a no-op for zero duration", func(t *testing.T) {
		var mt managedTimer
		if mt.Start(0, func() { t.Error("zero-duration timer should not run") }) {
			t.Error("Start should report nothing armed")
		}
		if mt.timer != nil {
			t.Error("timer should stay nil for zero duration")
		}
	})

	t.Run("Start does not reset an armed timer", func(t *testing.T) {
		var mt managedTimer
		mt.Start(time.Hour, func() {})
		first := mt.timer
		if mt.Start(time.Hour, func() {}) {
			t.Error("Start should report no-op when already armed")
		}
		if mt.timer != first {
			t.Error("already-armed timer should be left untouched")
		}
		mt.Cancel()
	})

	t.Run("Cancel stops and clears", func(t *testing.T) {
		var mt managedTimer
		mt.timer = time.AfterFunc(time.Hour, func() {
			t.Error("timer should have been cancelled")
		})
		if !mt.Cancel() {
			t.Error("Cancel should report it cancelled a timer")
		}
		if mt.timer != nil {
			t.Error("timer should be nil after Cancel")
		}
	})

	t.Run("Cancel is a no-op when not armed", func(t *testing.T) {
		var mt managedTimer
		if mt.Cancel() {
			t.Error("Cancel should report nothing to cancel")
		}
	})
}

// adapterSignal builds a PropertiesChanged signal on the adapter interface.
func adapterSignal(changed map[string]dbus.Variant) *dbus.Signal {
	return &dbus.Signal{
		Name: DBUS_PROP_IFACE + ".PropertiesChanged",
		Path: BLUETOOTH_PATH,
		Body: []interface{}{BLUETOOTH_ADAPTER, changed},
	}
}

// TestOnSignalAdapterPoweredOff: an external power-off (adapter Powered=false)
// resets the published status, not just the Powered flag.
func TestOnSignalAdapterPoweredOff(t *testing.T) {
	b := newTestBackend()
	b.seedStatus(BluetoothStatus{
		Powered:      true,
		Scanning:     true,
		KnownDevices: []BluetoothDevice{{Address: "AA:BB:CC:DD:EE:FF"}},
	})

	b.onSignal(adapterSignal(map[string]dbus.Variant{
		"Powered": dbus.MakeVariant(false),
	}))

	got := b.GetStatus()
	if got.Powered {
		t.Error("Powered should be false after power-off")
	}
	if got.Scanning {
		t.Error("Scanning should be cleared after power-off")
	}
	if got.KnownDevices != nil {
		t.Errorf("KnownDevices should be cleared after power-off, got %v", got.KnownDevices)
	}
}

// TestCheckAndStartIdleTimerNotPowered: the idle timer never arms while powered
// off (the guard also keeps it from reaching the D-Bus connected-devices check).
func TestCheckAndStartIdleTimerNotPowered(t *testing.T) {
	b := newTestBackend()
	b.idleTimeout = time.Hour
	b.seedStatus(BluetoothStatus{Powered: false})

	b.checkAndStartIdleTimer()

	if b.idleTimer.timer != nil {
		t.Error("idle timer must not arm when powered off")
	}
}

// interfacesAddedSignal builds an InterfacesAdded signal for a device.
func interfacesAddedSignal(path string, dev map[string]dbus.Variant) *dbus.Signal {
	return &dbus.Signal{
		Name: DBUS_OBJ_MANAGER + ".InterfacesAdded",
		Path: "/",
		Body: []interface{}{
			dbus.ObjectPath(path),
			map[string]map[string]dbus.Variant{BLUETOOTH_DEVICE: dev},
		},
	}
}

func TestOnSignalDiscovered(t *testing.T) {
	dev := map[string]dbus.Variant{
		"Address": dbus.MakeVariant("AA:BB:CC:DD:EE:FF"),
		"Name":    dbus.MakeVariant("Speaker"),
	}

	t.Run("merges device and emits event while scanning", func(t *testing.T) {
		b := newTestBackend()
		b.seedStatus(BluetoothStatus{Powered: true, Scanning: true})

		b.onSignal(interfacesAddedSignal("/org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF", dev))

		known := b.GetStatus().KnownDevices
		if len(known) != 1 || known[0].Address != "AA:BB:CC:DD:EE:FF" {
			t.Fatalf("discovered device should be merged, got %v", known)
		}
		if !drainHasEvent(b, events.TypeBluetoothDiscovered) {
			t.Errorf("expected a %s event", events.TypeBluetoothDiscovered)
		}
	})

	t.Run("ignores discovery when not scanning", func(t *testing.T) {
		b := newTestBackend()
		b.seedStatus(BluetoothStatus{Powered: true, Scanning: false})

		b.onSignal(interfacesAddedSignal("/org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF", dev))

		if known := b.GetStatus().KnownDevices; known != nil {
			t.Errorf("no device should be merged when not scanning, got %v", known)
		}
		if drainHasEvent(b, events.TypeBluetoothDiscovered) {
			t.Error("no discovered event expected when not scanning")
		}
	})
}

// drainHasEvent reports whether any buffered event has the given type.
func drainHasEvent(b *BluetoothBackend, eventType string) bool {
	for {
		select {
		case ev := <-b.events:
			if ev.Type == eventType {
				return true
			}
		default:
			return false
		}
	}
}
