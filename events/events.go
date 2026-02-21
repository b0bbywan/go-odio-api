package events

const (
	TypePlayerUpdated  = "player.updated"
	TypePlayerAdded    = "player.added"
	TypePlayerRemoved  = "player.removed"
	TypeAudioUpdated   = "audio.updated"
	TypeServiceUpdated = "service.updated"
)

type Event struct {
	Type string
	Data any
}

// BackendTypes maps backend names to their event type constants.
var BackendTypes = map[string][]string{
	"mpris":   {TypePlayerUpdated, TypePlayerAdded, TypePlayerRemoved},
	"audio":   {TypeAudioUpdated},
	"systemd": {TypeServiceUpdated},
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
