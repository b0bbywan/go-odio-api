package mpris

import (
	"context"
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
	CanPlay       bool `json:"can_play"`
	CanPause      bool `json:"can_pause"`
	CanGoNext     bool `json:"can_go_next"`
	CanGoPrevious bool `json:"can_go_previous"`
	CanSeek       bool `json:"can_seek"`
	CanControl    bool `json:"can_control"`
}

// Listener écoute les changements MPRIS via signaux D-Bus
type Listener struct {
	backend *MPRISBackend
	ctx     context.Context
	cancel  context.CancelFunc
	conn    *dbus.Conn

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
	BusName        string            `json:"bus_name"`
	Identity       string            `json:"identity"`
	PlaybackStatus PlaybackStatus    `json:"playback_status"`
	LoopStatus     LoopStatus        `json:"loop_status,omitempty"`
	Shuffle        bool              `json:"shuffle,omitempty"`
	Volume         float64           `json:"volume,omitempty"`
	Position       int64             `json:"position,omitempty"`
	Rate           float64           `json:"rate,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
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
