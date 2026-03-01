package mpris

import (
	"strings"

	"github.com/godbus/dbus/v5"

	idbus "github.com/b0bbywan/go-odio-api/backend/internal/dbus"
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

func (m *MPRISBackend) getObj(busName string) dbus.BusObject {
	return idbus.GetObject(m.conn, busName, MPRIS_PATH)
}

// callMethod calls an MPRIS method on a player
func (m *MPRISBackend) callMethod(busName, method string, args ...interface{}) error {
	return idbus.CallMethod(m.getObj(busName), method, args...)
}

// setProperty sets a property on a player
func (m *MPRISBackend) setProperty(busName, property string, value interface{}) error {
	return idbus.SetProperty(m.getObj(busName), MPRIS_PLAYER_IFACE, property, value)
}

// getProperty retrieves a property from D-Bus for a given busName
func (m *MPRISBackend) getProperty(busName, iface, prop string) (dbus.Variant, error) {
	return idbus.GetProperty(m.getObj(busName), iface, prop)
}

// listDBusNames retrieves the list of all bus names on D-Bus
func (m *MPRISBackend) listDBusNames() ([]string, error) {
	var names []string
	call := m.conn.BusObject().Call(idbus.BUS_LIST_NAMES, 0)
	if err := idbus.CallWithTimeout(call); err != nil {
		return nil, err
	}
	if err := call.Store(&names); err != nil {
		return nil, err
	}
	return names, nil
}

// addMatchRule subscribes to a D-Bus signal via a match rule
func (m *MPRISBackend) addMatchRule(rule string) error {
	return idbus.AddMatchRule(m.conn, rule)
}

// addListenMatchRules subscribes to the necessary D-Bus signals for the listener.
// Subscribes to PropertiesChanged (player state changes) and
// NameOwnerChanged (player appearance/disappearance).
func (m *MPRISBackend) addListenMatchRules() error {
	matchRule := "type='signal',interface='" + DBUS_PROP_IFACE + "',member='PropertiesChanged',arg0namespace='" + MPRIS_PREFIX + "'"
	if err := m.addMatchRule(matchRule); err != nil {
		return err
	}

	ownerMatchRule := "type='signal',interface='" + DBUS_INTERFACE + "',member='NameOwnerChanged',arg0namespace='" + MPRIS_PREFIX + "'"
	if err := m.addMatchRule(ownerMatchRule); err != nil {
		return err
	}

	return nil
}

func (m *MPRISBackend) getNameOwner(busName string) (string, error) {
	var owner string
	call := m.conn.BusObject().Call(idbus.BUS_GET_NAME_OWNER, 0, busName)
	if err := idbus.CallWithTimeout(call); err != nil {
		return "", err
	}
	if err := call.Store(&owner); err != nil {
		return "", err
	}
	return owner, nil
}

func (p *Player) getObj() dbus.BusObject {
	return idbus.GetObject(p.conn, p.BusName, MPRIS_PATH)
}

// getAllProperties retrieves all properties of a D-Bus interface in a single call
func (p *Player) getAllProperties(iface string) (map[string]dbus.Variant, error) {
	return idbus.GetAllProperties(p.getObj(), iface)
}
