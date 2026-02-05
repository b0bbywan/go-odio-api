package mpris

import (
	"context"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
)

// PlaybackStatus represents the current playback state
type PlaybackStatus string

// LoopStatus represents the current loop/repeat state
type LoopStatus string

// MPRISBackend gère les connexions aux lecteurs multimédias via MPRIS
type MPRISBackend struct {
	conn    *dbus.Conn
	ctx     context.Context
	timeout time.Duration

	// cache permanent (pas d'expiration)
	cache *cache.Cache[[]Player]

	// listener pour les changements MPRIS
	listener *Listener

	// heartbeat pour mettre à jour Position des players en lecture
	heartbeat *Heartbeat
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

// Player représente un lecteur multimédia MPRIS
type Player struct {
	backend    *MPRISBackend // Backend parent (non exporté)
	conn       *dbus.Conn    // Connexion D-Bus (non exporté)
	timeout    time.Duration // Timeout pour les appels D-Bus (non exporté)
	uniqueName string        // Unique connection name D-Bus (ex: :1.107)

	BusName string `json:"bus_name"`

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

// Capabilities représente les actions supportées par un lecteur
type Capabilities struct {
	CanPlay       bool `json:"can_play" dbus:"CanPlay"`
	CanPause      bool `json:"can_pause" dbus:"CanPause"`
	CanGoNext     bool `json:"can_go_next" dbus:"CanGoNext"`
	CanGoPrevious bool `json:"can_go_previous" dbus:"CanGoPrevious"`
	CanSeek       bool `json:"can_seek" dbus:"CanSeek"`
	CanControl    bool `json:"can_control" dbus:"CanControl"`
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
