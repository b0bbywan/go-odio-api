package events

const (
	TypeServerInfo     = "server.info"
	TypePlayerUpdated  = "player.updated"
	TypePlayerAdded    = "player.added"
	TypePlayerRemoved  = "player.removed"
	TypePlayerPosition = "player.position"
	TypeAudioUpdated   = "audio.updated"
	TypeServiceUpdated = "service.updated"
)

type Event struct {
	Type string
	Data any
}
