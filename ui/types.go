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
	Upgrade    bool `json:"upgrade"`
	Zeroconf   bool `json:"zeroconf"`
}

// UpgradeStatus is the GET /upgrade payload: the detector's contract fields plus
// the live run state. Free detector fields ride under "extra" server-side and
// are unused here. Latest is empty when no detection has run yet.
type UpgradeStatus struct {
	Current          string          `json:"current"`
	Latest           string          `json:"latest"`
	UpgradeAvailable bool            `json:"upgrade_available"`
	CheckedAt        time.Time       `json:"checked_at"`
	Run              *UpgradeRun     `json:"run"`
	LastRun          *UpgradeLastRun `json:"last_run"`
	CanCheck         bool            `json:"can_check"`
	CanUpgrade       bool            `json:"can_upgrade"`
}

// UpgradeLastRun is the verdict of the most recent finished run, kept so a failure
// stays visible after its one-shot SSE event. Step/Error are best-effort.
type UpgradeLastRun struct {
	Success    bool      `json:"success"`
	FinishedAt time.Time `json:"finished_at"`
	Step       string    `json:"step"`
	Error      string    `json:"error"`
}

// UpgradeRun is the live progress of an in-flight upgrade; nil unless one runs.
// Percent is nil until the script streams it (e.g. after an odio-api restart
// mid-run), which the badge shows as an indeterminate ring.
type UpgradeRun struct {
	State   string `json:"state"`
	Percent *int   `json:"percent"`
	Step    string `json:"step"`
}

func (r *UpgradeRun) HasPercent() bool { return r != nil && r.Percent != nil }

func (r *UpgradeRun) PercentValue() int {
	if !r.HasPercent() {
		return 0
	}
	return *r.Percent
}

// RingOffset is the stroke-dashoffset for a circumference-100 ring.
func (r *UpgradeRun) RingOffset() int { return 100 - r.PercentValue() }

// Known reports whether a detection result is available to display.
func (u *UpgradeStatus) Known() bool {
	return u != nil && u.Latest != ""
}

// Running reports whether an upgrade is in flight.
func (u *UpgradeStatus) Running() bool {
	return u != nil && u.Run != nil && u.Run.State == "running"
}

// Checkable / Upgradeable report whether the matching trigger is available, so
// the badge offers an action only when the backend can honour it.
func (u *UpgradeStatus) Checkable() bool   { return u != nil && u.CanCheck }
func (u *UpgradeStatus) Upgradeable() bool { return u != nil && u.CanUpgrade }

// Failed reports whether the last finished run failed and no run is currently in flight.
func (u *UpgradeStatus) Failed() bool {
	return u != nil && u.LastRun != nil && !u.LastRun.Success && !u.Running()
}

// FailureLabel is the badge tooltip for a failed run: step and error when reported.
func (u *UpgradeStatus) FailureLabel() string {
	if u == nil || u.LastRun == nil {
		return ""
	}
	label := "Last upgrade failed"
	if u.LastRun.Step != "" {
		label += " at " + u.LastRun.Step
	}
	if u.LastRun.Error != "" {
		label += ": " + u.LastRun.Error
	}
	if !u.LastRun.FinishedAt.IsZero() {
		label += " · " + u.LastRun.FinishedAt.Local().Format("02 Jan 2006, 15:04")
	}
	return label
}

// CheckedAtLabel is the last-check time for the badge tooltip, in local time,
// or empty when no detection has run.
func (u *UpgradeStatus) CheckedAtLabel() string {
	if u == nil || u.CheckedAt.IsZero() {
		return ""
	}
	return u.CheckedAt.Local().Format("02 Jan 2006, 15:04")
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
	Name              string             `json:"bus_name"` // API returns "bus_name", not "name"
	Identity          string             `json:"identity"` // human-readable name from MPRIS (e.g. "Chrome", "Spotify")
	Metadata          map[string]string  `json:"metadata"`
	Status            string             `json:"playback_status"` // API returns "playback_status", not "status"
	Position          int64              `json:"position"`
	PositionUpdatedAt time.Time          `json:"position_updated_at"`
	Rate              float64            `json:"rate"`
	Volume            *float64           `json:"volume"`
	Shuffle           bool               `json:"shuffle"`     // omitted by backend when false (omitempty)
	LoopStatus        *string            `json:"loop_status"` // pointer: nil ⇒ player doesn't expose LoopStatus
	Capabilities      PlayerCapabilities `json:"capabilities"`
}

// AudioOutput represents a PulseAudio/PipeWire sink from /audio
type AudioOutput struct {
	Index       uint32  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Nick        string  `json:"nick,omitempty"`
	Muted       bool    `json:"muted"`
	Volume      float64 `json:"volume"`
	State       string  `json:"state"`
	Default     bool    `json:"default"`
	Driver      string  `json:"driver,omitempty"`
	ActivePort  string  `json:"active_port,omitempty"`
	IsNetwork   bool    `json:"is_network,omitempty"`
}

// AudioClient represents a PulseAudio sink input from /audio
type AudioClient struct {
	Index       uint32  `json:"index"`
	Name        string  `json:"name"`
	Application string  `json:"app"` // API returns "app", not "application"
	Volume      float64 `json:"volume"`
	Muted       bool    `json:"muted"`
	Corked      bool    `json:"corked"`
}

// AudioData holds the combined audio state from GET /audio
type AudioData struct {
	Kind        string       // "pulseaudio" or "pipewire"
	DefaultSink *AudioOutput // the output with default=true
	Clients     []AudioClient
	Outputs     []AudioOutput
}

// Service represents a systemd service from /services
type Service struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Scope       string `json:"scope"` // "system" or "user"
	URL         string `json:"url,omitempty"`
}

// BluetoothDevice represents a Bluetooth device from /bluetooth
type BluetoothDevice struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Paired    bool   `json:"paired"`
	Bonded    bool   `json:"bonded"`
	Trusted   bool   `json:"trusted"`
	Connected bool   `json:"connected"`
}

// Label is the display label: the name, falling back to the address. Used as
// both the sort key and the rendered label so the two never diverge.
func (d BluetoothDevice) Label() string {
	if d.Name != "" {
		return d.Name
	}
	return d.Address
}

// BluetoothStatus represents the current Bluetooth state from /bluetooth
type BluetoothStatus struct {
	Powered       bool              `json:"powered"`
	Discoverable  bool              `json:"discoverable"`
	Pairable      bool              `json:"pairable"`
	PairingActive bool              `json:"pairing_active"`
	PairingUntil  *time.Time        `json:"pairing_until,omitempty"`
	Scanning      bool              `json:"scanning"`
	KnownDevices  []BluetoothDevice `json:"known_devices,omitempty"`
}

// ============================================================================
// View Models (for template rendering)
// ============================================================================

// DashboardView is the main view model for the dashboard page
type DashboardView struct {
	Title      string
	ServerInfo *ServerInfo
	Players    []PlayerView
	AudioData  *AudioData
	Services   []ServiceView
	Bluetooth  *BluetoothView
	Upgrade    *UpgradeStatus
}

// PlayerView is a view-optimized version of Player for templates
type PlayerView struct {
	Name        string // Full bus_name for API endpoints (e.g., org.mpris.MediaPlayer2.spotify)
	DisplayName string // Truncated name for display (e.g., spotify)
	Artist      string
	Title       string
	Album       string
	ArtUrl      string   // Cover art proxy path (/players/{name}/cover), empty if unavailable
	State       string   // "playing", "paused", "stopped"
	Volume      *float64 // Volume level 0.0-1.0
	CanPlay     bool
	CanPause    bool
	CanNext     bool
	CanPrev     bool
	CanStop     bool // alias for CanControl on the API side
	// Shuffle / repeat — visibility is gated on LoopStatus presence: most
	// players that expose LoopStatus also expose Shuffle, and the backend's
	// omitempty on Shuffle prevents reliable detection on its own.
	CanShuffle bool
	Shuffle    bool
	CanLoop    bool
	LoopStatus string // "None", "Track", "Playlist" — empty when CanLoop is false
	// Seeker fields
	Position          int64   // Current position in microseconds (as of PositionUpdatedAt)
	Duration          int64   // Track duration in microseconds (from mpris:length)
	Rate              float64 // Playback rate (1.0 = normal speed)
	CanSeek           bool
	PositionUpdatedAt string // RFC3339 timestamp of the last per-player position write
}

// ServiceView is a view-optimized version of Service for templates
type ServiceView struct {
	Name        string
	Description string
	Active      bool
	State       string
	IsUser      bool   // true if scope is "user", false if "system"
	URL         string // optional, may be ":port" / "/path" / full URL — resolved client-side
}

// BluetoothView is the view model for the bluetooth section
type BluetoothView struct {
	Powered        bool
	PairingActive  bool
	PairingUntilMs int64 // pairing deadline as epoch millis, for the client-side countdown
	Scanning       bool
	ConnectedCount int
	Devices        []BluetoothDevice
}
