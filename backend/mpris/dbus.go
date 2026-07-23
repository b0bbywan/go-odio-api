package mpris

import (
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// validateBusName validates that a busName is MPRIS-compliant
func validateBusName(busName string) error {
	if busName == "" {
		return &InvalidBusNameError{BusName: busName, Reason: "empty bus name"}
	}
	if !strings.HasPrefix(busName, MPRIS_PREFIX+".") {
		return &InvalidBusNameError{BusName: busName, Reason: "must start with org.mpris.MediaPlayer2."}
	}
	// Check that it doesn't contain dangerous characters
	if strings.Contains(busName, "..") || strings.Contains(busName, "/") || strings.ContainsAny(busName, "\x00\r\n") {
		return &InvalidBusNameError{BusName: busName, Reason: "contains illegal characters"}
	}
	return nil
}

// callWithTimeout executes a D-Bus call with timeout
func callWithTimeout(call *dbus.Call, timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		done <- call.Err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return &dbusTimeoutError{}
	}
}

// callWithTimeout receiver method for MPRISBackend
func (m *MPRISBackend) callWithTimeout(call *dbus.Call) error {
	return callWithTimeout(call, m.timeout)
}

// callMethod calls an MPRIS method on a player with timeout
func (m *MPRISBackend) callMethod(busName, method string, args ...interface{}) error {
	obj := m.conn.Object(busName, MPRIS_PATH)
	return m.callWithTimeout(obj.Call(method, 0, args...))
}

// setProperty sets a property on a player
func (m *MPRISBackend) setProperty(busName, property string, value interface{}) error {
	obj := m.conn.Object(busName, MPRIS_PATH)
	return m.callWithTimeout(obj.Call(DBUS_PROP_SET, 0, MPRIS_PLAYER_IFACE, property, dbus.MakeVariant(value)))
}

// getProperty retrieves a property from D-Bus for a given busName
func (m *MPRISBackend) getProperty(busName, iface, prop string) (dbus.Variant, error) {
	obj := m.conn.Object(busName, MPRIS_PATH)
	var v dbus.Variant
	call := obj.Call(DBUS_PROP_GET, 0, iface, prop)
	if err := m.callWithTimeout(call); err != nil {
		return dbus.Variant{}, err
	}
	if err := call.Store(&v); err != nil {
		return dbus.Variant{}, err
	}
	return v, nil
}

// listDBusNames retrieves the list of all bus names on D-Bus
func (m *MPRISBackend) listDBusNames() ([]string, error) {
	var names []string
	call := m.conn.BusObject().Call(DBUS_LIST_NAMES_METHOD, 0)
	if err := m.callWithTimeout(call); err != nil {
		return nil, err
	}
	if err := call.Store(&names); err != nil {
		return nil, err
	}
	return names, nil
}

// addMatchRule subscribes to a D-Bus signal via a match rule
func (m *MPRISBackend) addMatchRule(rule string) error {
	call := m.conn.BusObject().Call(DBUS_ADD_MATCH_METHOD, 0, rule)
	return m.callWithTimeout(call)
}

// addListenMatchRules subscribes to the necessary D-Bus signals for the listener.
// Subscribes to PropertiesChanged (player state changes),
// NameOwnerChanged (player appearance/disappearance) and TrackList signals.
func (m *MPRISBackend) addListenMatchRules() error {
	matchRule := "type='signal',interface='" + DBUS_PROP_IFACE + "',member='PropertiesChanged',arg0namespace='" + MPRIS_PREFIX + "'"
	if err := m.addMatchRule(matchRule); err != nil {
		return err
	}

	ownerMatchRule := "type='signal',interface='" + DBUS_INTERFACE + "',member='NameOwnerChanged',arg0namespace='" + MPRIS_PREFIX + "'"
	if err := m.addMatchRule(ownerMatchRule); err != nil {
		return err
	}

	// No arg0namespace here: TrackListReplaced's arg0 is `ao`, not a string.
	// Unknown senders at this path are dropped by findPlayerByUniqueName.
	tracklistMatchRule := "type='signal',interface='" + MPRIS_TRACKLIST_IFACE + "',path='" + MPRIS_PATH + "'"
	if err := m.addMatchRule(tracklistMatchRule); err != nil {
		return err
	}

	return nil
}

func (m *MPRISBackend) getNameOwner(busName string) (string, error) {
	var owner string
	call := m.conn.BusObject().Call(DBUS_GET_NAME_OWNER, 0, busName)
	if err := m.callWithTimeout(call); err != nil {
		return "", err
	}
	if err := call.Store(&owner); err != nil {
		return "", err
	}
	return owner, nil
}

// arg extracts sig.Body[i] as T, false if absent or mistyped.
func arg[T any](sig *dbus.Signal, i int) (T, bool) {
	if i >= len(sig.Body) {
		var zero T
		return zero, false
	}
	v, ok := sig.Body[i].(T)
	return v, ok
}

// extract returns the variant's value as T, false if it holds another type.
// Lets signal handlers read values without additional D-Bus calls.
func extract[T any](v dbus.Variant) (T, bool) {
	val, ok := v.Value().(T)
	return val, ok
}

// callWithTimeout receiver method for Player
func (p *Player) callWithTimeout(call *dbus.Call) error {
	return callWithTimeout(call, p.timeout)
}

// getAllProperties retrieves all properties of a D-Bus interface in a single call
func (p *Player) getAllProperties(iface string) (map[string]dbus.Variant, error) {
	obj := p.conn.Object(p.BusName, MPRIS_PATH)
	var props map[string]dbus.Variant

	call := obj.Call(DBUS_PROP_GET_ALL, 0, iface)
	if err := p.callWithTimeout(call); err != nil {
		return nil, err
	}

	err := call.Store(&props)
	return props, err
}

// getTracksMetadata retrieves metadata for the given track IDs in a single call.
// The spec guarantees results in the same order as the requested IDs.
func (p *Player) getTracksMetadata(ids []dbus.ObjectPath) ([]map[string]dbus.Variant, error) {
	obj := p.conn.Object(p.BusName, MPRIS_PATH)
	var metas []map[string]dbus.Variant

	call := obj.Call(MPRIS_METHOD_GET_TRACKS_METADATA, 0, ids)
	if err := p.callWithTimeout(call); err != nil {
		return nil, err
	}

	err := call.Store(&metas)
	return metas, err
}
