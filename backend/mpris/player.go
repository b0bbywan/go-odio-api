package mpris

import (
	"fmt"
	"reflect"

	"github.com/godbus/dbus/v5"
)

// getProperty récupère une propriété D-Bus
func (p *Player) getProperty(iface, prop string) (dbus.Variant, error) {
	obj := p.conn.Object(p.BusName, mprisPath)
	var v dbus.Variant

	call := obj.Call(dbusPropGet, 0, iface, prop)
	if err := callWithTimeout(call); err != nil {
		return dbus.Variant{}, err
	}

	err := call.Store(&v)
	return v, err
}

// getStringProperty récupère une propriété string
func (p *Player) getStringProperty(iface, prop string) (string, bool) {
	v, err := p.getProperty(iface, prop)
	if err != nil {
		return "", false
	}
	val, ok := v.Value().(string)
	return val, ok
}

// getBoolProperty récupère une propriété bool
func (p *Player) getBoolProperty(iface, prop string) (bool, bool) {
	v, err := p.getProperty(iface, prop)
	if err != nil {
		return false, false
	}
	val, ok := v.Value().(bool)
	return val, ok
}

// getFloat64Property récupère une propriété float64
func (p *Player) getFloat64Property(iface, prop string) (float64, bool) {
	v, err := p.getProperty(iface, prop)
	if err != nil {
		return 0, false
	}
	val, ok := v.Value().(float64)
	return val, ok
}

// getInt64Property récupère une propriété int64
func (p *Player) getInt64Property(iface, prop string) (int64, bool) {
	v, err := p.getProperty(iface, prop)
	if err != nil {
		return 0, false
	}
	val, ok := v.Value().(int64)
	return val, ok
}

// Capability getter methods (raccourcis pour un accès plus court)

// CanPlay retourne si le lecteur peut jouer
func (p *Player) CanPlay() bool {
	return p.Capabilities.CanPlay
}

// CanPause retourne si le lecteur peut mettre en pause
func (p *Player) CanPause() bool {
	return p.Capabilities.CanPause
}

// CanGoNext retourne si le lecteur peut passer à la piste suivante
func (p *Player) CanGoNext() bool {
	return p.Capabilities.CanGoNext
}

// CanGoPrevious retourne si le lecteur peut revenir à la piste précédente
func (p *Player) CanGoPrevious() bool {
	return p.Capabilities.CanGoPrevious
}

// CanSeek retourne si le lecteur peut chercher dans la piste
func (p *Player) CanSeek() bool {
	return p.Capabilities.CanSeek
}

// CanControl retourne si le lecteur peut être contrôlé
func (p *Player) CanControl() bool {
	return p.Capabilities.CanControl
}

// Load charge toutes les propriétés du player depuis D-Bus
func (p *Player) Load() error {
	val := reflect.ValueOf(p).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Récupérer les tags
		dbusTag := fieldType.Tag.Get("dbus")
		ifaceTag := fieldType.Tag.Get("iface")

		// Ignorer les champs sans tag dbus (conn, BusName, Capabilities)
		if dbusTag == "" {
			continue
		}

		// Gérer selon le type de champ
		switch field.Kind() {
		case reflect.String:
			if propVal, ok := p.getStringProperty(ifaceTag, dbusTag); ok {
				field.SetString(propVal)
			}

		case reflect.Bool:
			if propVal, ok := p.getBoolProperty(ifaceTag, dbusTag); ok {
				field.SetBool(propVal)
			}

		case reflect.Float64:
			if propVal, ok := p.getFloat64Property(ifaceTag, dbusTag); ok {
				field.SetFloat(propVal)
			}

		case reflect.Int64:
			if propVal, ok := p.getInt64Property(ifaceTag, dbusTag); ok {
				field.SetInt(propVal)
			}

		case reflect.Map:
			// Cas spécial pour Metadata
			if dbusTag == "Metadata" {
				if metadata, err := p.getProperty(ifaceTag, dbusTag); err == nil {
					field.Set(reflect.ValueOf(extractMetadata(metadata.Value())))
				}
			}
		}
	}

	// Charger les capabilities
	p.Capabilities = p.loadCapabilities()

	return nil
}

// loadCapabilities charge les capabilities depuis D-Bus
func (p *Player) loadCapabilities() Capabilities {
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
		if propVal, ok := p.getBoolProperty(mprisPlayerIface, dbusTag); ok {
			field.SetBool(propVal)
		}
	}

	return caps
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
			metadata[key] = formatMetadataValue(v.Value())
		}
	}

	return metadata
}

// formatMetadataValue formate une valeur de métadonnée en string
func formatMetadataValue(value interface{}) string {
	switch v := value.(type) {
	case []string:
		// Joindre les tableaux de strings avec des virgules
		result := ""
		for i, s := range v {
			if i > 0 {
				result += ", "
			}
			result += s
		}
		return result
	case []interface{}:
		// Gérer les slices génériques
		result := ""
		for i, item := range v {
			if i > 0 {
				result += ", "
			}
			result += fmt.Sprintf("%v", item)
		}
		return result
	default:
		return fmt.Sprintf("%v", v)
	}
}

// newPlayer crée un nouveau Player avec connexion D-Bus
func newPlayer(conn *dbus.Conn, busName string) *Player {
	return &Player{
		conn:    conn,
		BusName: busName,
	}
}
