package mpris

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/godbus/dbus/v5"
)

// getProperty récupère une propriété D-Bus
func (p *Player) getProperty(iface, prop string) (dbus.Variant, error) {
	obj := p.conn.Object(p.BusName, mprisPath)
	var v dbus.Variant
	err := obj.Call(dbusPropIface+".Get", 0, iface, prop).Store(&v)
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

// Capability getter methods

// CanPlay retourne si le lecteur peut jouer
func (p *Player) CanPlay() bool {
	return p.capabilities.canPlay
}

// CanPause retourne si le lecteur peut mettre en pause
func (p *Player) CanPause() bool {
	return p.capabilities.canPause
}

// CanGoNext retourne si le lecteur peut passer à la piste suivante
func (p *Player) CanGoNext() bool {
	return p.capabilities.canGoNext
}

// CanGoPrevious retourne si le lecteur peut revenir à la piste précédente
func (p *Player) CanGoPrevious() bool {
	return p.capabilities.canGoPrevious
}

// CanSeek retourne si le lecteur peut chercher dans la piste
func (p *Player) CanSeek() bool {
	return p.capabilities.canSeek
}

// CanControl retourne si le lecteur peut être contrôlé
func (p *Player) CanControl() bool {
	return p.capabilities.canControl
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
	p.capabilities = p.loadCapabilities()

	return nil
}

// loadCapabilities charge les capabilities depuis D-Bus
func (p *Player) loadCapabilities() capabilities {
	var caps capabilities

	if val, ok := p.getBoolProperty(mprisPlayerIface, "CanPlay"); ok {
		caps.canPlay = val
	}
	if val, ok := p.getBoolProperty(mprisPlayerIface, "CanPause"); ok {
		caps.canPause = val
	}
	if val, ok := p.getBoolProperty(mprisPlayerIface, "CanGoNext"); ok {
		caps.canGoNext = val
	}
	if val, ok := p.getBoolProperty(mprisPlayerIface, "CanGoPrevious"); ok {
		caps.canGoPrevious = val
	}
	if val, ok := p.getBoolProperty(mprisPlayerIface, "CanSeek"); ok {
		caps.canSeek = val
	}
	if val, ok := p.getBoolProperty(mprisPlayerIface, "CanControl"); ok {
		caps.canControl = val
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
			metadata[key] = fmt.Sprintf("%v", v.Value())
		}
	}

	return metadata
}

// MarshalJSON implémente json.Marshaler pour Player
func (p *Player) MarshalJSON() ([]byte, error) {
	type Alias Player
	return json.Marshal(&struct {
		*Alias
		Capabilities struct {
			CanPlay       bool `json:"can_play"`
			CanPause      bool `json:"can_pause"`
			CanGoNext     bool `json:"can_go_next"`
			CanGoPrevious bool `json:"can_go_previous"`
			CanSeek       bool `json:"can_seek"`
			CanControl    bool `json:"can_control"`
		} `json:"capabilities"`
	}{
		Alias: (*Alias)(p),
		Capabilities: struct {
			CanPlay       bool `json:"can_play"`
			CanPause      bool `json:"can_pause"`
			CanGoNext     bool `json:"can_go_next"`
			CanGoPrevious bool `json:"can_go_previous"`
			CanSeek       bool `json:"can_seek"`
			CanControl    bool `json:"can_control"`
		}{
			CanPlay:       p.CanPlay(),
			CanPause:      p.CanPause(),
			CanGoNext:     p.CanGoNext(),
			CanGoPrevious: p.CanGoPrevious(),
			CanSeek:       p.CanSeek(),
			CanControl:    p.CanControl(),
		},
	})
}
