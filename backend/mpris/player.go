package mpris

import (
	"fmt"
	"reflect"

	"github.com/godbus/dbus/v5"
)

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

// getProperty récupère une propriété D-Bus via le backend
func (p *Player) getProperty(iface, prop string) (dbus.Variant, error) {
	return p.backend.getProperty(p.BusName, iface, prop)
}

// getStringProperty récupère une propriété string
func (p *Player) getStringProperty(iface, prop string) (string, bool) {
	v, err := p.getProperty(iface, prop)
	if err != nil {
		return "", false
	}
	return extractString(v)
}

// getBoolProperty récupère une propriété bool
func (p *Player) getBoolProperty(iface, prop string) (bool, bool) {
	v, err := p.getProperty(iface, prop)
	if err != nil {
		return false, false
	}
	return extractBool(v)
}

// getFloat64Property récupère une propriété float64
func (p *Player) getFloat64Property(iface, prop string) (float64, bool) {
	v, err := p.getProperty(iface, prop)
	if err != nil {
		return 0, false
	}
	return extractFloat64(v)
}

// getInt64Property récupère une propriété int64
func (p *Player) getInt64Property(iface, prop string) (int64, bool) {
	v, err := p.getProperty(iface, prop)
	if err != nil {
		return 0, false
	}
	return extractInt64(v)
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

// loadFromDBus charge toutes les propriétés du player depuis D-Bus.
// Cette fonction privée effectue les appels D-Bus nécessaires pour remplir tous les champs
// du Player en utilisant GetAll (2 appels) au lieu d'appels individuels Get (~15 appels).
// Le mapping des propriétés vers les champs struct se fait via reflection en utilisant
// les tags `dbus` et `iface`.
func (p *Player) loadFromDBus() error {
	// Récupérer le unique name via GetNameOwner
	var owner string
	if err := p.conn.BusObject().Call(DBUS_GET_NAME_OWNER, 0, p.BusName).Store(&owner); err != nil {
		return err
	}
	p.uniqueName = owner

	// Récupérer toutes les propriétés des deux interfaces en 2 appels au lieu de ~15
	propsMediaPlayer2, err := p.getAllProperties(MPRIS_INTERFACE)
	if err != nil {
		return err
	}

	propsPlayer, err := p.getAllProperties(MPRIS_PLAYER_IFACE)
	if err != nil {
		return err
	}

	// Combiner les deux maps
	allProps := make(map[string]map[string]dbus.Variant)
	allProps[MPRIS_INTERFACE] = propsMediaPlayer2
	allProps[MPRIS_PLAYER_IFACE] = propsPlayer

	// Mapper les propriétés aux champs du struct
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

		// Récupérer la propriété depuis le cache
		props, ok := allProps[ifaceTag]
		if !ok {
			continue
		}

		variant, ok := props[dbusTag]
		if !ok {
			continue
		}

		// Gérer selon le type de champ
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
			// Cas spécial pour Metadata
			if dbusTag == "Metadata" {
				field.Set(reflect.ValueOf(extractMetadata(variant.Value())))
			}
		}
	}

	// Charger les capabilities depuis les propriétés déjà récupérées
	p.Capabilities = p.loadCapabilitiesFromProps(propsPlayer)

	return nil
}

// loadCapabilitiesFromProps charge les capabilities depuis les propriétés déjà récupérées.
// Utilisé par loadFromDBus() pour éviter des appels D-Bus supplémentaires.
// Mappe les propriétés D-Bus (CanPlay, CanPause, etc.) vers le struct Capabilities
// en utilisant reflection et les tags `dbus`.
func (p *Player) loadCapabilitiesFromProps(props map[string]dbus.Variant) Capabilities {
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

		// Récupérer la propriété depuis les props
		if variant, ok := props[dbusTag]; ok {
			if boolVal, ok := extractBool(variant); ok {
				field.SetBool(boolVal)
			}
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

// newPlayer crée un nouveau Player avec connexion au backend
func newPlayer(backend *MPRISBackend, busName string) *Player {
	return &Player{
		backend: backend,
		conn:    backend.conn,
		timeout: backend.timeout,
		BusName: busName,
	}
}
