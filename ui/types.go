package ui

import "time"

// ============================================================================
// API Response Types (matching JSON API responses)
// ============================================================================

// PowerCapabilities represents what power actions are available from /power
type PowerCapabilities struct {
	Reboot   bool `json:"reboot"`
	PowerOff bool `json:"power_off"`
}

// ServerInfo represents the response from /server
type ServerInfo struct {
	Hostname   string             `json:"hostname"`
	OSPlatform string             `json:"os_platform"`
	OSVersion  string             `json:"os_version"`
	APISW      string             `json:"api_sw"`
	APIVersion string             `json:"api_version"`
	Backends   Backends           `json:"backends"`
	Power      *PowerCapabilities `json:"-"`
}

// Backends indicates which backends are enabled
type Backends struct {
	Bluetooth  bool `json:"bluetooth"`
	MPRIS      bool `json:"mpris"`
	Power      bool `json:"power"`
	PulseAudio bool `json:"pulseaudio"`
	Systemd    bool `json:"systemd"`
	Zeroconf   bool `json:"zeroconf"`
}

// PlayerCapabilities represents what transport actions a player supports
type PlayerCapabilities struct {
	CanPlay       bool `json:"can_play"`
	CanPause      bool `json:"can_pause"`
	CanGoNext     bool `json:"can_go_next"`
	CanGoPrevious bool `json:"can_go_previous"`
	CanSeek       bool `json:"can_seek"`
	CanControl    bool `json:"can_control"`
}

// Player represents an MPRIS player from /players
type Player struct {
	Name         string             `json:"bus_name"` // API returns "bus_name", not "name"
	Metadata     map[string]string  `json:"metadata"`
	Status       string             `json:"playback_status"` // API returns "playback_status", not "status"
	Position     int64              `json:"position"`
	Rate         float64            `json:"rate"`
	Volume       *float64           `json:"volume"`
	Capabilities PlayerCapabilities `json:"capabilities"`
}

// AudioInfo represents PulseAudio server info from /audio/server
type AudioInfo struct {
	Kind         string  `json:"kind"` // "pulseaudio" or "pipewire"
	ServerString string  `json:"server_string"`
	DefaultSink  string  `json:"default_sink"`
	Volume       float64 `json:"volume"`
	Muted        bool    `json:"muted"`
}

// AudioClient represents a PulseAudio sink input from /audio/clients
type AudioClient struct {
	Index       uint32  `json:"index"`
	Name        string  `json:"name"`
	Application string  `json:"app"` // API returns "app", not "application"
	Volume      float64 `json:"volume"`
	Muted       bool    `json:"muted"`
}

// Service represents a systemd service from /services
type Service struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Scope       string `json:"scope"` // "system" or "user"
}

// BluetoothDevice represents a known Bluetooth device from /bluetooth
type BluetoothDevice struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Trusted   bool   `json:"trusted"`
	Connected bool   `json:"connected"`
}

// BluetoothStatus represents the current Bluetooth state from /bluetooth
type BluetoothStatus struct {
	Powered       bool              `json:"powered"`
	Discoverable  bool              `json:"discoverable"`
	Pairable      bool              `json:"pairable"`
	PairingActive bool              `json:"pairing_active"`
	PairingUntil  *time.Time        `json:"pairing_until,omitempty"`
	KnownDevices  []BluetoothDevice `json:"known_devices,omitempty"`
}

// ============================================================================
// View Models (for template rendering)
// ============================================================================

// DashboardView is the main view model for the dashboard page
type DashboardView struct {
	Title        string
	ServerInfo   *ServerInfo
	Players      []PlayerView
	AudioInfo    *AudioInfo
	AudioClients []AudioClient
	Services     []ServiceView
	Bluetooth    *BluetoothView
}

// PlayerView is a view-optimized version of Player for templates
type PlayerView struct {
	Name        string // Full bus_name for API endpoints (e.g., org.mpris.MediaPlayer2.spotify)
	DisplayName string // Truncated name for display (e.g., spotify)
	Artist      string
	Title       string
	Album       string
	ArtUrl      string   // Cover art URL (http/https only, empty if unavailable)
	State       string   // "playing", "paused", "stopped"
	Volume      *float64 // Volume level 0.0-1.0
	CanPlay     bool
	CanPause    bool
	CanNext     bool
	CanPrev     bool
	// Seeker fields
	Position       int64   // Current position in microseconds (as of CacheUpdatedAt)
	Duration       int64   // Track duration in microseconds (from mpris:length)
	Rate           float64 // Playback rate (1.0 = normal speed)
	CanSeek        bool
	CacheUpdatedAt string // RFC3339 timestamp of the last cache write (from X-Cache-Updated-At header)
}

// ServiceView is a view-optimized version of Service for templates
type ServiceView struct {
	Name        string
	Description string
	Active      bool
	State       string
	IsUser      bool // true if scope is "user", false if "system"
}

// BluetoothView is the view model for the bluetooth section
type BluetoothView struct {
	Powered            bool
	PairingActive      bool
	PairingSecondsLeft int
	ConnectedCount     int
}
