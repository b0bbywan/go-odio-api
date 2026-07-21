package mpris

const (
	CACHE_KEY = "players"

	// MPRIS D-Bus constants
	MPRIS_PREFIX    = "org.mpris.MediaPlayer2"
	MPRIS_PATH      = "/org/mpris/MediaPlayer2"
	MPRIS_INTERFACE = "org.mpris.MediaPlayer2"

	MPRIS_PLAYER_IFACE    = MPRIS_PREFIX + ".Player"
	MPRIS_TRACKLIST_IFACE = MPRIS_PREFIX + ".TrackList"

	// D-Bus system constants
	DBUS_INTERFACE  = "org.freedesktop.DBus"
	DBUS_PROP_IFACE = "org.freedesktop.DBus.Properties"

	// D-Bus method names
	DBUS_LIST_NAMES_METHOD   = DBUS_INTERFACE + ".ListNames"
	DBUS_ADD_MATCH_METHOD    = DBUS_INTERFACE + ".AddMatch"
	DBUS_PROP_GET            = DBUS_PROP_IFACE + ".Get"
	DBUS_PROP_GET_ALL        = DBUS_PROP_IFACE + ".GetAll"
	DBUS_PROP_SET            = DBUS_PROP_IFACE + ".Set"
	DBUS_PROP_CHANGED_SIGNAL = DBUS_PROP_IFACE + ".PropertiesChanged"
	DBUS_NAME_OWNER_CHANGED  = DBUS_INTERFACE + ".NameOwnerChanged"
	DBUS_GET_NAME_OWNER      = DBUS_INTERFACE + ".GetNameOwner"

	// MPRIS Player methods
	MPRIS_METHOD_PLAY         = MPRIS_PLAYER_IFACE + ".Play"
	MPRIS_METHOD_PAUSE        = MPRIS_PLAYER_IFACE + ".Pause"
	MPRIS_METHOD_PLAY_PAUSE   = MPRIS_PLAYER_IFACE + ".PlayPause"
	MPRIS_METHOD_STOP         = MPRIS_PLAYER_IFACE + ".Stop"
	MPRIS_METHOD_NEXT         = MPRIS_PLAYER_IFACE + ".Next"
	MPRIS_METHOD_PREVIOUS     = MPRIS_PLAYER_IFACE + ".Previous"
	MPRIS_METHOD_SEEK         = MPRIS_PLAYER_IFACE + ".Seek"
	MPRIS_METHOD_SET_POSITION = MPRIS_PLAYER_IFACE + ".SetPosition"

	// MPRIS TrackList methods
	MPRIS_METHOD_GET_TRACKS_METADATA = MPRIS_TRACKLIST_IFACE + ".GetTracksMetadata"
	MPRIS_METHOD_ADD_TRACK           = MPRIS_TRACKLIST_IFACE + ".AddTrack"
	MPRIS_METHOD_REMOVE_TRACK        = MPRIS_TRACKLIST_IFACE + ".RemoveTrack"
	MPRIS_METHOD_GO_TO               = MPRIS_TRACKLIST_IFACE + ".GoTo"

	// MPRIS TrackList signals
	MPRIS_SIGNAL_TRACKLIST_REPLACED     = MPRIS_TRACKLIST_IFACE + ".TrackListReplaced"
	MPRIS_SIGNAL_TRACK_ADDED            = MPRIS_TRACKLIST_IFACE + ".TrackAdded"
	MPRIS_SIGNAL_TRACK_REMOVED          = MPRIS_TRACKLIST_IFACE + ".TrackRemoved"
	MPRIS_SIGNAL_TRACK_METADATA_CHANGED = MPRIS_TRACKLIST_IFACE + ".TrackMetadataChanged"
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
