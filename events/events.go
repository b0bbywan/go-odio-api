package events

const (
	TypeServerInfo       = "server.info"
	TypePlayerUpdated    = "player.updated"
	TypePlayerAdded      = "player.added"
	TypePlayerRemoved    = "player.removed"
	TypePlayerPosition   = "player.position"
	TypeAudioUpdated     = "audio.updated"
	TypeServiceUpdated   = "service.updated"
	TypeBluetoothUpdated = "bluetooth.updated"
	TypePowerAction      = "power.action"
)

type Event struct {
	Type string
	Data any
}

// BackendTypes maps backend names to their event type constants.
var BackendTypes = map[string][]string{
	"mpris":     {TypePlayerUpdated, TypePlayerAdded, TypePlayerRemoved, TypePlayerPosition},
	"audio":     {TypeAudioUpdated},
	"systemd":   {TypeServiceUpdated},
	"bluetooth": {TypeBluetoothUpdated},
	"power":     {TypePowerAction},
}

// NewFilter combines include and exclude type lists into a single filter func.
// A nil return means pass-all.
func NewFilter(include, exclude []string) func(Event) bool {
	inc := FilterTypes(include)
	exc := FilterExcludeTypes(exclude)
	if inc == nil && exc == nil {
		return nil
	}
	return func(e Event) bool {
		return (inc == nil || inc(e)) && (exc == nil || exc(e))
	}
}

// FilterExcludeTypes returns a filter func that blocks events whose Type is in types.
// A nil or empty types slice returns nil (pass-all).
func FilterExcludeTypes(types []string) func(Event) bool {
	if len(types) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(types))
	for _, t := range types {
		set[t] = struct{}{}
	}
	return func(e Event) bool {
		_, blocked := set[e.Type]
		return !blocked
	}
}

// FilterTypes returns a filter func that passes only events whose Type is in types.
// A nil or empty types slice returns nil (pass-all).
func FilterTypes(types []string) func(Event) bool {
	if len(types) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(types))
	for _, t := range types {
		set[t] = struct{}{}
	}
	return func(e Event) bool {
		_, ok := set[e.Type]
		return ok
	}
}

// FilterBackend returns a filter func that passes events belonging to the named
// backends. Backend names are resolved via BackendTypes; unknown names are ignored.
// If no known backend is named, nil is returned (pass-all).
func FilterBackend(backends []string) func(Event) bool {
	var types []string
	for _, name := range backends {
		if ts, ok := BackendTypes[name]; ok {
			types = append(types, ts...)
		}
	}
	return FilterTypes(types)
}
