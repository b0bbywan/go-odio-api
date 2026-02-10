package ui

// ============================================================================
// API Response Types (matching JSON API responses)
// ============================================================================

// ServerInfo represents the response from /server
type ServerInfo struct {
	Hostname   string   `json:"hostname"`
	OSPlatform string   `json:"os_platform"`
	OSVersion  string   `json:"os_version"`
	APISW      string   `json:"api_sw"`
	APIVersion string   `json:"api_version"`
	Backends   Backends `json:"backends"`
}

// Backends indicates which backends are enabled
type Backends struct {
	Bluetooth  bool `json:"bluetooth"`
	MPRIS      bool `json:"mpris"`
	PulseAudio bool `json:"pulseaudio"`
	Systemd    bool `json:"systemd"`
}

// Player represents an MPRIS player from /players
type Player struct {
	Name     string            `json:"name"`
	Metadata map[string]string `json:"metadata"`
	Status   string            `json:"status"`
	Position int64             `json:"position"`
	Volume   float64           `json:"volume"`
}

// AudioInfo represents PulseAudio server info from /audio/server
type AudioInfo struct {
	ServerString string  `json:"server_string"`
	DefaultSink  string  `json:"default_sink"`
	Volume       float64 `json:"volume"`
	Muted        bool    `json:"muted"`
}

// AudioClient represents a PulseAudio sink input from /audio/clients
type AudioClient struct {
	Index       uint32  `json:"index"`
	Name        string  `json:"name"`
	Application string  `json:"application"`
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
}

// PlayerView is a view-optimized version of Player for templates
type PlayerView struct {
	Name   string
	Artist string
	Title  string
	Album  string
	State  string // "playing", "paused", "stopped"
}

// ServiceView is a view-optimized version of Service for templates
type ServiceView struct {
	Name        string
	Description string
	Active      bool
	State       string
}
