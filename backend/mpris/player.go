package mpris

import (
	"fmt"
	"reflect"

	"github.com/godbus/dbus/v5"
)

// newPlayer creates a new Player with backend connection
func newPlayer(backend *MPRISBackend, busName string) *Player {
	return &Player{
		backend: backend,
		conn:    backend.conn,
		timeout: backend.timeout,
		BusName: busName,
	}
}

// Capability getter methods (shortcuts for shorter access)

// CanPlay returns whether the player can play
func (p *Player) CanPlay() bool {
	return p.Capabilities.CanPlay
}

// CanPause returns whether the player can pause
func (p *Player) CanPause() bool {
	return p.Capabilities.CanPause
}

// CanGoNext returns whether the player can skip to the next track
func (p *Player) CanGoNext() bool {
	return p.Capabilities.CanGoNext
}

// CanGoPrevious returns whether the player can go back to the previous track
func (p *Player) CanGoPrevious() bool {
	return p.Capabilities.CanGoPrevious
}

// CanSeek returns whether the player can seek within the track
func (p *Player) CanSeek() bool {
	return p.Capabilities.CanSeek
}

// CanControl returns whether the player can be controlled
func (p *Player) CanControl() bool {
	return p.Capabilities.CanControl
}

// loadFromDBus loads all player properties from D-Bus.
// This private function performs the necessary D-Bus calls to fill all Player fields
// using GetAll (2 calls) instead of individual Get calls (~15 calls).
// Property mapping to struct fields is done via reflection using
// the `dbus` and `iface` tags.
func (p *Player) loadFromDBus() error {
	// Retrieve the unique name via GetNameOwner
	owner, err := p.backend.getNameOwner(p.BusName)
	if err != nil {
		return err
	}
	p.uniqueName = owner

	// Retrieve all properties from both interfaces in 2 calls instead of ~15
	propsMediaPlayer2, err := p.getAllProperties(MPRIS_INTERFACE)
	if err != nil {
		return err
	}

	propsPlayer, err := p.getAllProperties(MPRIS_PLAYER_IFACE)
	if err != nil {
		return err
	}

	// Combine both maps
	allProps := make(map[string]map[string]dbus.Variant)
	allProps[MPRIS_INTERFACE] = propsMediaPlayer2
	allProps[MPRIS_PLAYER_IFACE] = propsPlayer

	// Map properties to struct fields
	val := reflect.ValueOf(p).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Retrieve tags
		dbusTag := fieldType.Tag.Get("dbus")
		ifaceTag := fieldType.Tag.Get("iface")

		// Ignore fields without dbus tag (conn, BusName, Capabilities)
		if dbusTag == "" {
			continue
		}

		// Retrieve property from cache
		props, ok := allProps[ifaceTag]
		if !ok {
			continue
		}

		variant, ok := props[dbusTag]
		if !ok {
			continue
		}

		// Handle according to field type
		switch field.Kind() {
		case reflect.String:
			if val, ok := extractString(variant); ok {
				field.SetString(val)
			}

		case reflect.Bool:
			if val, ok := extractBool(variant); ok {
				field.SetBool(val)
			}

		case reflect.Float64:
			if val, ok := extractFloat64(variant); ok {
				field.SetFloat(val)
			}

		case reflect.Int64:
			if val, ok := extractInt64(variant); ok {
				field.SetInt(val)
			}

		case reflect.Map:
			// Special case for Metadata
			if dbusTag == "Metadata" {
				field.Set(reflect.ValueOf(extractMetadata(variant.Value())))
			}
		}
	}

	// Load capabilities from already retrieved properties
	p.Capabilities = p.loadCapabilitiesFromProps(propsPlayer)

	return nil
}

// loadCapabilitiesFromProps loads capabilities from already retrieved properties.
// Used by loadFromDBus() to avoid additional D-Bus calls.
// Maps D-Bus properties (CanPlay, CanPause, etc.) to the Capabilities struct
// using reflection and `dbus` tags.
func (p *Player) loadCapabilitiesFromProps(props map[string]dbus.Variant) Capabilities {
	var caps Capabilities

	val := reflect.ValueOf(&caps).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Retrieve dbus tag
		dbusTag := fieldType.Tag.Get("dbus")
		if dbusTag == "" {
			continue
		}

		// Retrieve property from props
		if variant, ok := props[dbusTag]; ok {
			if boolVal, ok := extractBool(variant); ok {
				field.SetBool(boolVal)
			}
		}
	}

	return caps
}

// extractMetadata extracts relevant metadata
func extractMetadata(raw interface{}) map[string]string {
	metadata := make(map[string]string)

	m, ok := raw.(map[string]dbus.Variant)
	if !ok {
		return metadata
	}

	// Extract common metadata
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

// formatMetadataValue formats a metadata value as string
func formatMetadataValue(value interface{}) string {
	switch v := value.(type) {
	case []string:
		// Join string arrays with commas
		result := ""
		for i, s := range v {
			if i > 0 {
				result += ", "
			}
			result += s
		}
		return result
	case []interface{}:
		// Handle generic slices
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
