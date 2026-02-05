package mpris

import (
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// validateBusName valide qu'un busName est conforme à MPRIS
func validateBusName(busName string) error {
	if busName == "" {
		return &InvalidBusNameError{BusName: busName, Reason: "empty bus name"}
	}
	if !strings.HasPrefix(busName, MPRIS_PREFIX+".") {
		return &InvalidBusNameError{BusName: busName, Reason: "must start with org.mpris.MediaPlayer2."}
	}
	// Vérifier qu'il ne contient pas de caractères dangereux
	if strings.Contains(busName, "..") || strings.Contains(busName, "/") || strings.ContainsAny(busName, "\x00\r\n") {
		return &InvalidBusNameError{BusName: busName, Reason: "contains illegal characters"}
	}
	return nil
}

// callWithTimeout exécute un appel D-Bus avec timeout
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

// callWithTimeout méthode receiver pour MPRISBackend
func (m *MPRISBackend) callWithTimeout(call *dbus.Call) error {
	return callWithTimeout(call, m.timeout)
}

// callMethod appelle une méthode MPRIS sur un player avec timeout
func (m *MPRISBackend) callMethod(busName, method string, args ...interface{}) error {
	obj := m.conn.Object(busName, MPRIS_PATH)
	return m.callWithTimeout(obj.Call(method, 0, args...))
}

// setProperty définit une propriété sur un player
func (m *MPRISBackend) setProperty(busName, property string, value interface{}) error {
	obj := m.conn.Object(busName, MPRIS_PATH)
	return m.callWithTimeout(obj.Call(DBUS_PROP_SET, 0, MPRIS_PLAYER_IFACE, property, dbus.MakeVariant(value)))
}

// getProperty récupère une propriété depuis D-Bus pour un busName donné
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

// listDBusNames récupère la liste de tous les bus names sur D-Bus
func (m *MPRISBackend) listDBusNames() ([]string, error) {
	var names []string
	if err := m.conn.BusObject().Call(DBUS_LIST_NAMES_METHOD, 0).Store(&names); err != nil {
		return nil, err
	}
	return names, nil
}

// addMatchRule s'abonne à un signal D-Bus via une match rule
func (m *MPRISBackend) addMatchRule(rule string) error {
	return m.conn.BusObject().Call(DBUS_ADD_MATCH_METHOD, 0, rule).Err
}

// addListenMatchRules s'abonne aux signaux D-Bus nécessaires pour le listener.
// S'abonne à PropertiesChanged (changements d'état des players) et
// NameOwnerChanged (apparition/disparition des players).
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
	if err := m.conn.BusObject().Call(DBUS_GET_NAME_OWNER, 0, busName).Store(&owner); err != nil {
		return "", err
	}
	return owner, nil
}

// Helpers d'extraction de valeurs depuis dbus.Variant
// Ces helpers sont utilisés pour extraire les valeurs des variants reçus
// dans les signaux D-Bus sans faire d'appels D-Bus supplémentaires.

// extractString extrait une string d'un dbus.Variant
func extractString(v dbus.Variant) (string, bool) {
	val, ok := v.Value().(string)
	return val, ok
}

// extractBool extrait un bool d'un dbus.Variant
func extractBool(v dbus.Variant) (bool, bool) {
	val, ok := v.Value().(bool)
	return val, ok
}

// extractInt64 extrait un int64 d'un dbus.Variant
func extractInt64(v dbus.Variant) (int64, bool) {
	val, ok := v.Value().(int64)
	return val, ok
}

// extractFloat64 extrait un float64 d'un dbus.Variant
func extractFloat64(v dbus.Variant) (float64, bool) {
	val, ok := v.Value().(float64)
	return val, ok
}

// extractMetadataMap extrait une map de metadata d'un dbus.Variant
func extractMetadataMap(v dbus.Variant) (map[string]dbus.Variant, bool) {
	val, ok := v.Value().(map[string]dbus.Variant)
	return val, ok
}

// callWithTimeout méthode receiver pour Player
func (p *Player) callWithTimeout(call *dbus.Call) error {
	return callWithTimeout(call, p.timeout)
}

// getAllProperties récupère toutes les propriétés d'une interface D-Bus en un seul appel
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
