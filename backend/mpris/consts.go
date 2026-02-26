package mpris

const (
	CACHE_KEY = "players"

	// MPRIS D-Bus constants
	MPRIS_PREFIX       = "org.mpris.MediaPlayer2"
	MPRIS_PATH         = "/org/mpris/MediaPlayer2"
	MPRIS_INTERFACE    = "org.mpris.MediaPlayer2"
	MPRIS_PLAYER_IFACE = "org.mpris.MediaPlayer2.Player"

	// D-Bus system constants
	DBUS_INTERFACE  = "org.freedesktop.DBus"
	DBUS_PROP_IFACE = "org.freedesktop.DBus.Properties"

	// D-Bus signal names
	DBUS_PROP_CHANGED_SIGNAL = DBUS_PROP_IFACE + ".PropertiesChanged"
	DBUS_NAME_OWNER_CHANGED  = DBUS_INTERFACE + ".NameOwnerChanged"

	// MPRIS Player methods
	MPRIS_METHOD_PLAY         = MPRIS_PLAYER_IFACE + ".Play"
	MPRIS_METHOD_PAUSE        = MPRIS_PLAYER_IFACE + ".Pause"
	MPRIS_METHOD_PLAY_PAUSE   = MPRIS_PLAYER_IFACE + ".PlayPause"
	MPRIS_METHOD_STOP         = MPRIS_PLAYER_IFACE + ".Stop"
	MPRIS_METHOD_NEXT         = MPRIS_PLAYER_IFACE + ".Next"
	MPRIS_METHOD_PREVIOUS     = MPRIS_PLAYER_IFACE + ".Previous"
	MPRIS_METHOD_SEEK         = MPRIS_PLAYER_IFACE + ".Seek"
	MPRIS_METHOD_SET_POSITION = MPRIS_PLAYER_IFACE + ".SetPosition"
)

// MPRIS_NO_TRACK is the well-known track ID meaning "no current track".
// SetPosition is a no-op for this value, so we fall back to relative Seek.
const MPRIS_NO_TRACK = "/org/mpris/MediaPlayer2/TrackList/NoTrack"

const (
	StatusPlaying PlaybackStatus = "Playing"
	StatusPaused  PlaybackStatus = "Paused"
	StatusStopped PlaybackStatus = "Stopped"
)

const (
	LoopNone     LoopStatus = "None"
	LoopTrack    LoopStatus = "Track"
	LoopPlaylist LoopStatus = "Playlist"
)
