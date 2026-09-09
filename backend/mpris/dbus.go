package mpris

import (
	"context"
	"errors"
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

// call issues a method call bounded by timeout. The deadline must be on the
// call itself: obj.Call blocks until a reply, and some players (Kodi) never
// reply to interfaces they don't implement, which used to freeze the caller
// until the player left the bus.
func call(obj dbus.BusObject, timeout time.Duration, method string, args ...interface{}) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := obj.CallWithContext(ctx, method, 0, args...)
	if errors.Is(c.Err, context.DeadlineExceeded) {
		c.Err = &dbusTimeoutError{}
	}
	return c
}

func (m *MPRISBackend) call(obj dbus.BusObject, method string, args ...interface{}) *dbus.Call {
	return call(obj, m.timeout, method, args...)
}

func (m *MPRISBackend) callMethod(busName, method string, args ...interface{}) error {
	obj := m.conn.Object(busName, MPRIS_PATH)
	return m.call(obj, method, args...).Err
}

func (m *MPRISBackend) setProperty(busName, property string, value interface{}) error {
	obj := m.conn.Object(busName, MPRIS_PATH)
	return m.call(obj, DBUS_PROP_SET, MPRIS_PLAYER_IFACE, property, dbus.MakeVariant(value)).Err
}

func (m *MPRISBackend) getProperty(busName, iface, prop string) (dbus.Variant, error) {
	obj := m.conn.Object(busName, MPRIS_PATH)
	var v dbus.Variant
	call := m.call(obj, DBUS_PROP_GET, iface, prop)
	if call.Err != nil {
		return dbus.Variant{}, call.Err
	}
	if err := call.Store(&v); err != nil {
		return dbus.Variant{}, err
	}
	return v, nil
}

// listDBusNames retrieves the list of all bus names on D-Bus
func (m *MPRISBackend) listDBusNames() ([]string, error) {
	var names []string
	call := m.call(m.conn.BusObject(), DBUS_LIST_NAMES_METHOD)
	if call.Err != nil {
		return nil, call.Err
	}
	if err := call.Store(&names); err != nil {
		return nil, err
	}
	return names, nil
}

// addMatchRule subscribes to a D-Bus signal via a match rule
func (m *MPRISBackend) addMatchRule(rule string) error {
	return m.call(m.conn.BusObject(), DBUS_ADD_MATCH_METHOD, rule).Err
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
	call := m.call(m.conn.BusObject(), DBUS_GET_NAME_OWNER, busName)
	if call.Err != nil {
		return "", call.Err
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

func (p *Player) call(method string, args ...interface{}) *dbus.Call {
	return call(p.conn.Object(p.BusName, MPRIS_PATH), p.timeout, method, args...)
}

// getAllProperties retrieves all properties of a D-Bus interface in a single call
func (p *Player) getAllProperties(iface string) (map[string]dbus.Variant, error) {
	var props map[string]dbus.Variant

	call := p.call(DBUS_PROP_GET_ALL, iface)
	if call.Err != nil {
		return nil, call.Err
	}

	err := call.Store(&props)
	return props, err
}

// getTracksMetadata retrieves metadata for the given track IDs in a single call.
// The spec guarantees results in the same order as the requested IDs.
func (p *Player) getTracksMetadata(ids []dbus.ObjectPath) ([]map[string]dbus.Variant, error) {
	var metas []map[string]dbus.Variant

	call := p.call(MPRIS_METHOD_GET_TRACKS_METADATA, ids)
	if call.Err != nil {
		return nil, call.Err
	}

	err := call.Store(&metas)
	return metas, err
}
