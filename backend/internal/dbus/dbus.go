package dbus

import (
	"time"

	"github.com/godbus/dbus/v5"
)

// DefaultTimeout is the timeout used for all D-Bus calls.
var DefaultTimeout = 5 * time.Second

// CallWithTimeout executes a D-Bus call with the default timeout.
func CallWithTimeout(call *dbus.Call) error {
	done := make(chan error, 1)
	go func() { done <- call.Err }()
	select {
	case err := <-done:
		return err
	case <-time.After(DefaultTimeout):
		return &TimeoutError{}
	}
}

// GetProperty retrieves a single property from a D-Bus object.
func GetProperty(obj dbus.BusObject, iface, prop string) (dbus.Variant, error) {
	var v dbus.Variant
	call := obj.Call(PROP_GET, 0, iface, prop)
	if err := CallWithTimeout(call); err != nil {
		return dbus.Variant{}, err
	}
	if err := call.Store(&v); err != nil {
		return dbus.Variant{}, err
	}
	return v, nil
}

// SetProperty sets a single property on a D-Bus object.
func SetProperty(obj dbus.BusObject, iface, prop string, value interface{}) error {
	return CallWithTimeout(obj.Call(PROP_SET, 0, iface, prop, dbus.MakeVariant(value)))
}

// GetAllProperties retrieves all properties of a D-Bus interface in a single call.
func GetAllProperties(obj dbus.BusObject, iface string) (map[string]dbus.Variant, error) {
	var props map[string]dbus.Variant
	call := obj.Call(PROP_GET_ALL, 0, iface)
	if err := CallWithTimeout(call); err != nil {
		return nil, err
	}
	return props, call.Store(&props)
}

// CallMethod calls a method on a D-Bus object with the default timeout.
func CallMethod(obj dbus.BusObject, method string, args ...interface{}) error {
	return CallWithTimeout(obj.Call(method, 0, args...))
}

// GetObject returns a D-Bus object for the given service and object path.
func GetObject(conn *dbus.Conn, service, path string) dbus.BusObject {
	return conn.Object(service, dbus.ObjectPath(path))
}

// AddMatchRule subscribes to a D-Bus signal via a match rule.
func AddMatchRule(conn *dbus.Conn, rule string) error {
	return conn.BusObject().Call(BUS_ADD_MATCH, 0, rule).Err
}

// RemoveMatchRule unsubscribes from a D-Bus signal match rule.
func RemoveMatchRule(conn *dbus.Conn, rule string) error {
	return conn.BusObject().Call(BUS_REMOVE_MATCH, 0, rule).Err
}

// FilterSignal parses a PropertiesChanged D-Bus signal body.
// Returns changed properties map and interface name, or an error if malformed.
func FilterSignal(sig *dbus.Signal) (map[string]dbus.Variant, string, error) {
	if sig == nil {
		return nil, "", &SignalError{Reason: "channel closed"}
	}
	if len(sig.Body) < 2 {
		return nil, "", &SignalError{Reason: "body too short"}
	}
	iface, ok := sig.Body[0].(string)
	if !ok {
		return nil, "", &SignalError{Reason: "failed to parse interface name"}
	}
	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return nil, "", &SignalError{Reason: "body[1] is not map[string]Variant"}
	}
	return changed, iface, nil
}

// --- Variant extraction helpers ---

// ExtractString extracts a string from a dbus.Variant.
func ExtractString(v dbus.Variant) (string, bool) {
	val, ok := v.Value().(string)
	return val, ok
}

// ExtractBool extracts a bool from a dbus.Variant.
func ExtractBool(v dbus.Variant) (bool, bool) {
	val, ok := v.Value().(bool)
	return val, ok
}

// ExtractInt64 extracts an int64 from a dbus.Variant.
func ExtractInt64(v dbus.Variant) (int64, bool) {
	val, ok := v.Value().(int64)
	return val, ok
}

// ExtractFloat64 extracts a float64 from a dbus.Variant.
func ExtractFloat64(v dbus.Variant) (float64, bool) {
	val, ok := v.Value().(float64)
	return val, ok
}

// ExtractVariantMap extracts a map[string]dbus.Variant from a dbus.Variant.
func ExtractVariantMap(v dbus.Variant) (map[string]dbus.Variant, bool) {
	val, ok := v.Value().(map[string]dbus.Variant)
	return val, ok
}

// --- Map helpers (props map[string]dbus.Variant) ---

// MapString extracts a string from a props map by key.
func MapString(props map[string]dbus.Variant, key string) string {
	if v, ok := props[key]; ok {
		s, _ := ExtractString(v)
		return s
	}
	return ""
}

// MapBool extracts a bool from a props map by key.
func MapBool(props map[string]dbus.Variant, key string) bool {
	if v, ok := props[key]; ok {
		b, _ := ExtractBool(v)
		return b
	}
	return false
}

// MapBoolOK extracts a bool from a props map by key, with existence check.
func MapBoolOK(props map[string]dbus.Variant, key string) (bool, bool) {
	if v, ok := props[key]; ok {
		return ExtractBool(v)
	}
	return false, false
}

// Keys returns the keys of a props map (useful for debug logging).
func Keys(props map[string]dbus.Variant) []string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	return keys
}
