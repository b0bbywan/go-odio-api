package mpris

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

// getProperty récupère une propriété D-Bus
func getProperty(conn *dbus.Conn, busName string, iface, prop string) (dbus.Variant, error) {
	obj := conn.Object(busName, mprisPath)
	var v dbus.Variant
	err := obj.Call(dbusPropIface+".Get", 0, iface, prop).Store(&v)
	return v, err
}

// Helper functions pour récupérer des propriétés typées
func getStringProperty(conn *dbus.Conn, busName string, iface, prop string) (string, bool) {
	v, err := getProperty(conn, busName, iface, prop)
	if err != nil {
		return "", false
	}
	val, ok := v.Value().(string)
	return val, ok
}

func getBoolProperty(conn *dbus.Conn, busName string, iface, prop string) (bool, bool) {
	v, err := getProperty(conn, busName, iface, prop)
	if err != nil {
		return false, false
	}
	val, ok := v.Value().(bool)
	return val, ok
}

func getFloat64Property(conn *dbus.Conn, busName string, iface, prop string) (float64, bool) {
	v, err := getProperty(conn, busName, iface, prop)
	if err != nil {
		return 0, false
	}
	val, ok := v.Value().(float64)
	return val, ok
}

func getInt64Property(conn *dbus.Conn, busName string, iface, prop string) (int64, bool) {
	v, err := getProperty(conn, busName, iface, prop)
	if err != nil {
		return 0, false
	}
	val, ok := v.Value().(int64)
	return val, ok
}

// extractMetadata extrait les métadonnées pertinentes
func extractMetadata(raw interface{}) map[string]string {
	metadata := make(map[string]string)

	m, ok := raw.(map[string]dbus.Variant)
	if !ok {
		return metadata
	}

	// Extraire les métadonnées courantes
	metadataKeys := []string{
		"xesam:title",
		"xesam:artist",
		"xesam:album",
		"xesam:albumArtist",
		"xesam:genre",
		"mpris:trackid",
		"mpris:artUrl",
		"mpris:length",
	}

	for _, key := range metadataKeys {
		if v, ok := m[key]; ok {
			metadata[key] = fmt.Sprintf("%v", v.Value())
		}
	}

	return metadata
}
