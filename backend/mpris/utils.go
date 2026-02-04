package mpris

import (
	"fmt"
	"reflect"

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

// loadCapabilities charge les capabilities depuis D-Bus en utilisant reflection et les tags `dbus`
func loadCapabilities(conn *dbus.Conn, busName string) Capabilities {
	var caps Capabilities

	val := reflect.ValueOf(&caps).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Récupérer le tag dbus
		dbusTag := fieldType.Tag.Get("dbus")
		if dbusTag == "" {
			continue
		}

		// Récupérer la propriété D-Bus
		if propVal, ok := getBoolProperty(conn, busName, mprisPlayerIface, dbusTag); ok {
			field.SetBool(propVal)
		}
	}

	return caps
}

// loadPlayer charge un Player depuis D-Bus en utilisant reflection et les tags `dbus` et `iface`
func loadPlayer(conn *dbus.Conn, busName string) Player {
	player := Player{
		BusName: busName,
	}

	val := reflect.ValueOf(&player).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Récupérer les tags
		dbusTag := fieldType.Tag.Get("dbus")
		ifaceTag := fieldType.Tag.Get("iface")

		// Ignorer les champs sans tag dbus (BusName, Capabilities)
		if dbusTag == "" {
			continue
		}

		// Gérer selon le type de champ
		switch field.Kind() {
		case reflect.String:
			if propVal, ok := getStringProperty(conn, busName, ifaceTag, dbusTag); ok {
				field.SetString(propVal)
			}

		case reflect.Bool:
			if propVal, ok := getBoolProperty(conn, busName, ifaceTag, dbusTag); ok {
				field.SetBool(propVal)
			}

		case reflect.Float64:
			if propVal, ok := getFloat64Property(conn, busName, ifaceTag, dbusTag); ok {
				field.SetFloat(propVal)
			}

		case reflect.Int64:
			if propVal, ok := getInt64Property(conn, busName, ifaceTag, dbusTag); ok {
				field.SetInt(propVal)
			}

		case reflect.Map:
			// Cas spécial pour Metadata
			if dbusTag == "Metadata" {
				if metadata, err := getProperty(conn, busName, ifaceTag, dbusTag); err == nil {
					field.Set(reflect.ValueOf(extractMetadata(metadata.Value())))
				}
			}
		}
	}

	// Charger les capabilities séparément (déjà géré par loadCapabilities)
	player.Capabilities = loadCapabilities(conn, busName)

	return player
}

// CheckCapabilities vérifie si un player a les capabilities requises
// Supporte la logique OR : si plusieurs noms sont donnés, au moins un doit être true
// Retourne (hasCapability, errorMessage)
func CheckCapabilities(player *Player, capFieldNames ...string) (bool, string) {
	if len(capFieldNames) == 0 {
		return true, ""
	}

	capsVal := reflect.ValueOf(player.Capabilities)
	capsType := capsVal.Type()

	var dbusNames []string
	hasAny := false

	for _, fieldName := range capFieldNames {
		field := capsVal.FieldByName(fieldName)
		if !field.IsValid() {
			continue
		}

		// Récupérer le tag dbus pour le message d'erreur
		if structField, ok := capsType.FieldByName(fieldName); ok {
			dbusTag := structField.Tag.Get("dbus")
			if dbusTag != "" {
				dbusNames = append(dbusNames, dbusTag)
			}
		}

		// Vérifier si la capability est true
		if field.Kind() == reflect.Bool && field.Bool() {
			hasAny = true
		}
	}

	// Construire le message d'erreur
	var errorMsg string
	if len(dbusNames) == 1 {
		errorMsg = dbusNames[0]
	} else if len(dbusNames) > 1 {
		errorMsg = dbusNames[0]
		for i := 1; i < len(dbusNames); i++ {
			errorMsg += " or " + dbusNames[i]
		}
	}

	return hasAny, errorMsg
}
