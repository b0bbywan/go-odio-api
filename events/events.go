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
