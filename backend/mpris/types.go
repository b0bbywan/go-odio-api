package mpris

import (
	"context"
	"reflect"
	"sync"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
)

const (
	cacheKey = "players"

	// MPRIS D-Bus constants
	mprisPrefix      = "org.mpris.MediaPlayer2"
	mprisPath        = "/org/mpris/MediaPlayer2"
	mprisInterface   = "org.mpris.MediaPlayer2"
	mprisPlayerIface = "org.mpris.MediaPlayer2.Player"
	dbusPropIface    = "org.freedesktop.DBus.Properties"
)

// PlaybackStatus represents the current playback state
type PlaybackStatus string

const (
	StatusPlaying PlaybackStatus = "Playing"
	StatusPaused  PlaybackStatus = "Paused"
	StatusStopped PlaybackStatus = "Stopped"
)

// LoopStatus represents the current loop/repeat state
type LoopStatus string

const (
	LoopNone     LoopStatus = "None"
	LoopTrack    LoopStatus = "Track"
	LoopPlaylist LoopStatus = "Playlist"
)

// Capabilities représente les actions supportées par un lecteur
type Capabilities struct {
	CanPlay       bool `json:"can_play" dbus:"CanPlay"`
	CanPause      bool `json:"can_pause" dbus:"CanPause"`
	CanGoNext     bool `json:"can_go_next" dbus:"CanGoNext"`
	CanGoPrevious bool `json:"can_go_previous" dbus:"CanGoPrevious"`
	CanSeek       bool `json:"can_seek" dbus:"CanSeek"`
	CanControl    bool `json:"can_control" dbus:"CanControl"`
}

// CapabilityRef représente une référence à un champ de Capabilities
type CapabilityRef struct {
	FieldName string
}

// CapabilityError indique qu'une action n'est pas supportée par le player
type CapabilityError struct {
	Required string
}

func (e *CapabilityError) Error() string {
	return "action not allowed (requires " + e.Required + ")"
}

// Caps contient les références à toutes les capabilities, auto-générées depuis Capabilities
var Caps = struct {
	CanPlay       CapabilityRef
	CanPause      CapabilityRef
	CanGoNext     CapabilityRef
	CanGoPrevious CapabilityRef
	CanSeek       CapabilityRef
	CanControl    CapabilityRef
}{}

func init() {
	// Auto-générer les CapabilityRef depuis la struct Capabilities
	capsType := reflect.TypeOf(Capabilities{})
	capsVal := reflect.ValueOf(&Caps).Elem()

	for i := 0; i < capsType.NumField(); i++ {
		field := capsType.Field(i)
		capRef := CapabilityRef{FieldName: field.Name}
		capsVal.Field(i).Set(reflect.ValueOf(capRef))
	}
}

// Listener écoute les changements MPRIS via signaux D-Bus
type Listener struct {
	backend *MPRISBackend
	ctx     context.Context
	cancel  context.CancelFunc

	// Déduplication : dernier état connu par player
	lastState   map[string]PlaybackStatus
	lastStateMu sync.RWMutex
}

// MPRISBackend gère les connexions aux lecteurs multimédias via MPRIS
type MPRISBackend struct {
	conn *dbus.Conn
	ctx  context.Context

	// cache permanent (pas d'expiration)
	cache *cache.Cache[[]Player]

	// listener pour les changements MPRIS
	listener *Listener
}

// Player représente un lecteur multimédia MPRIS
type Player struct {
	conn    *dbus.Conn // Connexion D-Bus (non exporté)
	BusName string     `json:"bus_name"`

	Identity       string            `json:"identity" dbus:"Identity" iface:"org.mpris.MediaPlayer2"`
	PlaybackStatus PlaybackStatus    `json:"playback_status" dbus:"PlaybackStatus" iface:"org.mpris.MediaPlayer2.Player"`
	LoopStatus     LoopStatus        `json:"loop_status,omitempty" dbus:"LoopStatus" iface:"org.mpris.MediaPlayer2.Player"`
	Shuffle        bool              `json:"shuffle,omitempty" dbus:"Shuffle" iface:"org.mpris.MediaPlayer2.Player"`
	Volume         float64           `json:"volume,omitempty" dbus:"Volume" iface:"org.mpris.MediaPlayer2.Player"`
	Position       int64             `json:"position,omitempty" dbus:"Position" iface:"org.mpris.MediaPlayer2.Player"`
	Rate           float64           `json:"rate,omitempty" dbus:"Rate" iface:"org.mpris.MediaPlayer2.Player"`
	Metadata       map[string]string `json:"metadata,omitempty" dbus:"Metadata" iface:"org.mpris.MediaPlayer2.Player"`
	Capabilities   Capabilities      `json:"capabilities"`
}

// Request types pour l'API

type SeekRequest struct {
	Offset int64 `json:"offset"`
}

type PositionRequest struct {
	TrackID  string `json:"track_id"`
	Position int64  `json:"position"`
}

type VolumeRequest struct {
	Volume float64 `json:"volume"`
}

type LoopRequest struct {
	Loop string `json:"loop"`
}

type ShuffleRequest struct {
	Shuffle bool `json:"shuffle"`
}
