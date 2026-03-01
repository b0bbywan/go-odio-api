package mpris

import (
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"

	idbus "github.com/b0bbywan/go-odio-api/backend/internal/dbus"
	"github.com/b0bbywan/go-odio-api/cache"
)

func TestGetPlayerFromCache(t *testing.T) {
	backend := &MPRISBackend{
		cache: cache.New[[]Player](0),
	}

	// Populate cache with test players
	players := []Player{
		{
			BusName:        "org.mpris.MediaPlayer2.spotify",
			Identity:       "Spotify",
			PlaybackStatus: StatusPlaying,
			Capabilities: Capabilities{
				CanPlay:       true,
				CanPause:      true,
				CanGoNext:     true,
				CanGoPrevious: true,
			},
		},
		{
			BusName:        "org.mpris.MediaPlayer2.vlc",
			Identity:       "VLC",
			PlaybackStatus: StatusPaused,
			Capabilities: Capabilities{
				CanPlay:       true,
				CanPause:      true,
				CanGoNext:     false,
				CanGoPrevious: false,
			},
		},
	}
	backend.cache.Set(CACHE_KEY, players)

	tests := []struct {
		name       string
		busName    string
		wantErr    bool
		wantPlayer *Player
	}{
		{
			name:    "find spotify player",
			busName: "org.mpris.MediaPlayer2.spotify",
			wantErr: false,
			wantPlayer: &Player{
				BusName:        "org.mpris.MediaPlayer2.spotify",
				Identity:       "Spotify",
				PlaybackStatus: StatusPlaying,
				Capabilities: Capabilities{
					CanPlay:       true,
					CanPause:      true,
					CanGoNext:     true,
					CanGoPrevious: true,
				},
			},
		},
		{
			name:    "find vlc player",
			busName: "org.mpris.MediaPlayer2.vlc",
			wantErr: false,
			wantPlayer: &Player{
				BusName:        "org.mpris.MediaPlayer2.vlc",
				Identity:       "VLC",
				PlaybackStatus: StatusPaused,
				Capabilities: Capabilities{
					CanPlay:       true,
					CanPause:      true,
					CanGoNext:     false,
					CanGoPrevious: false,
				},
			},
		},
		{
			name:       "player not found",
			busName:    "org.mpris.MediaPlayer2.nonexistent",
			wantErr:    true,
			wantPlayer: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player, err := backend.GetPlayerFromCache(tt.busName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPlayerFromCache(%q) error = %v, wantErr %v", tt.busName, err, tt.wantErr)
			}
			if tt.wantPlayer != nil && player != nil {
				if player.BusName != tt.wantPlayer.BusName {
					t.Errorf("BusName = %q, want %q", player.BusName, tt.wantPlayer.BusName)
				}
				if player.Identity != tt.wantPlayer.Identity {
					t.Errorf("Identity = %q, want %q", player.Identity, tt.wantPlayer.Identity)
				}
				if player.PlaybackStatus != tt.wantPlayer.PlaybackStatus {
					t.Errorf("PlaybackStatus = %q, want %q", player.PlaybackStatus, tt.wantPlayer.PlaybackStatus)
				}
				if player.CanPlay() != tt.wantPlayer.CanPlay() {
					t.Errorf("CanPlay() = %v, want %v", player.CanPlay(), tt.wantPlayer.CanPlay())
				}
				if player.CanGoNext() != tt.wantPlayer.CanGoNext() {
					t.Errorf("CanGoNext() = %v, want %v", player.CanGoNext(), tt.wantPlayer.CanGoNext())
				}
			}
		})
	}
}

func TestGetPlayerFromCacheEmptyCache(t *testing.T) {
	backend := &MPRISBackend{
		cache: cache.New[[]Player](0),
	}

	player, err := backend.GetPlayerFromCache("org.mpris.MediaPlayer2.test")
	if err == nil {
		t.Error("GetPlayerFromCache should return error when cache is empty")
	}
	if player != nil {
		t.Error("GetPlayerFromCache should return nil when cache is empty")
	}
}

func TestUpdatePlayer(t *testing.T) {
	backend := &MPRISBackend{
		cache: cache.New[[]Player](0),
	}

	// Initial cache state
	initialPlayers := []Player{
		{
			BusName:        "org.mpris.MediaPlayer2.spotify",
			Identity:       "Spotify",
			PlaybackStatus: StatusPaused,
			Volume:         0.5,
		},
		{
			BusName:        "org.mpris.MediaPlayer2.vlc",
			Identity:       "VLC",
			PlaybackStatus: StatusStopped,
			Volume:         1.0,
		},
	}
	backend.cache.Set(CACHE_KEY, initialPlayers)

	// Update an existing player
	updatedPlayer := Player{
		BusName:        "org.mpris.MediaPlayer2.spotify",
		Identity:       "Spotify",
		PlaybackStatus: StatusPlaying,
		Volume:         0.8,
		Capabilities: Capabilities{
			CanPlay:  true,
			CanPause: true,
		},
	}

	err := backend.UpdatePlayer(updatedPlayer)
	if err != nil {
		t.Fatalf("UpdatePlayer failed: %v", err)
	}

	// Verify the player was updated
	player, err := backend.GetPlayerFromCache("org.mpris.MediaPlayer2.spotify")
	if err != nil {
		t.Fatalf("Updated player should be found in cache: %v", err)
	}
	if player.PlaybackStatus != StatusPlaying {
		t.Errorf("PlaybackStatus = %q, want %q", player.PlaybackStatus, StatusPlaying)
	}
	if player.Volume != 0.8 {
		t.Errorf("Volume = %.2f, want %.2f", player.Volume, 0.8)
	}

	// Verify other player wasn't affected
	player2, err := backend.GetPlayerFromCache("org.mpris.MediaPlayer2.vlc")
	if err != nil {
		t.Fatalf("Other player should still be in cache: %v", err)
	}
	if player2.PlaybackStatus != StatusStopped {
		t.Error("Other player should not be affected by update")
	}
}

func TestUpdatePlayerAddNew(t *testing.T) {
	backend := &MPRISBackend{
		cache: cache.New[[]Player](0),
	}

	// Initial cache with one player
	initialPlayers := []Player{
		{
			BusName:  "org.mpris.MediaPlayer2.spotify",
			Identity: "Spotify",
		},
	}
	backend.cache.Set(CACHE_KEY, initialPlayers)

	// Add a new player
	newPlayer := Player{
		BusName:        "org.mpris.MediaPlayer2.vlc",
		Identity:       "VLC",
		PlaybackStatus: StatusPlaying,
		Capabilities: Capabilities{
			CanPlay: true,
		},
	}

	err := backend.UpdatePlayer(newPlayer)
	if err != nil {
		t.Fatalf("UpdatePlayer failed: %v", err)
	}

	// Verify the new player was added
	player, err := backend.GetPlayerFromCache("org.mpris.MediaPlayer2.vlc")
	if err != nil {
		t.Fatalf("New player should be found in cache: %v", err)
	}
	if player.PlaybackStatus != StatusPlaying {
		t.Error("New player should be playing")
	}

	// Verify we now have 2 players in cache
	players, _ := backend.cache.Get(CACHE_KEY)
	if len(players) != 2 {
		t.Errorf("Cache should contain 2 players, got %d", len(players))
	}
}

func TestRemovePlayer(t *testing.T) {
	backend := &MPRISBackend{
		cache: cache.New[[]Player](0),
	}

	// Populate cache with two players
	players := []Player{
		{
			BusName:  "org.mpris.MediaPlayer2.spotify",
			Identity: "Spotify",
		},
		{
			BusName:  "org.mpris.MediaPlayer2.vlc",
			Identity: "VLC",
		},
	}
	backend.cache.Set(CACHE_KEY, players)

	// Remove one player
	err := backend.RemovePlayer("org.mpris.MediaPlayer2.spotify")
	if err != nil {
		t.Fatalf("RemovePlayer failed: %v", err)
	}

	// Verify player was removed
	_, err = backend.GetPlayerFromCache("org.mpris.MediaPlayer2.spotify")
	if err == nil {
		t.Error("Removed player should not be found in cache")
	}

	// Verify other player is still there
	player, err := backend.GetPlayerFromCache("org.mpris.MediaPlayer2.vlc")
	if err != nil {
		t.Errorf("Other player should still be in cache: %v", err)
	}
	if player.Identity != "VLC" {
		t.Errorf("Identity = %q, want %q", player.Identity, "VLC")
	}

	// Verify cache size
	cachedPlayers, _ := backend.cache.Get(CACHE_KEY)
	if len(cachedPlayers) != 1 {
		t.Errorf("Cache should contain 1 player, got %d", len(cachedPlayers))
	}
}

func TestInvalidateCache(t *testing.T) {
	backend := &MPRISBackend{
		cache: cache.New[[]Player](0),
	}

	// Populate cache
	players := []Player{
		{BusName: "org.mpris.MediaPlayer2.test", Identity: "Test"},
	}
	backend.cache.Set(CACHE_KEY, players)

	// Verify cache is populated
	_, err := backend.GetPlayerFromCache("org.mpris.MediaPlayer2.test")
	if err != nil {
		t.Fatalf("Cache should be populated: %v", err)
	}

	// Invalidate cache
	backend.InvalidateCache()

	// Verify cache is empty
	_, err = backend.GetPlayerFromCache("org.mpris.MediaPlayer2.test")
	if err == nil {
		t.Error("Cache should be empty after invalidation")
	}
}

func TestExtractMetadata(t *testing.T) {
	// Test with empty metadata
	emptyMetadata := extractMetadata(nil)
	if len(emptyMetadata) != 0 {
		t.Error("Empty metadata should return empty map")
	}

	// Test with non-map type
	invalidMetadata := extractMetadata("invalid")
	if len(invalidMetadata) != 0 {
		t.Error("Invalid metadata type should return empty map")
	}
}

// Error types tests

func TestCapabilityError(t *testing.T) {
	err := &CapabilityError{Required: "CanPlay"}
	expected := "action not allowed (requires CanPlay)"
	if err.Error() != expected {
		t.Errorf("CapabilityError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestPlayerNotFoundError(t *testing.T) {
	err := &PlayerNotFoundError{BusName: "org.mpris.MediaPlayer2.spotify"}
	expected := "player not found: org.mpris.MediaPlayer2.spotify"
	if err.Error() != expected {
		t.Errorf("PlayerNotFoundError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestInvalidBusNameError(t *testing.T) {
	err := &InvalidBusNameError{
		BusName: "invalid",
		Reason:  "must start with org.mpris.MediaPlayer2.",
	}
	expected := "invalid player name: must start with org.mpris.MediaPlayer2."
	if err.Error() != expected {
		t.Errorf("InvalidBusNameError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestValidationError(t *testing.T) {
	tests := []struct {
		name     string
		err      *ValidationError
		expected string
	}{
		{
			name:     "with field",
			err:      &ValidationError{Field: "Volume", Message: "must be between 0 and 1"},
			expected: "Volume: must be between 0 and 1",
		},
		{
			name:     "without field",
			err:      &ValidationError{Field: "", Message: "invalid parameter"},
			expected: "invalid parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("ValidationError.Error() = %q, want %q", tt.err.Error(), tt.expected)
			}
		})
	}
}

func TestDBusTimeoutError(t *testing.T) {
	err := &idbus.TimeoutError{}
	expected := "dbus: call timed out"
	if err.Error() != expected {
		t.Errorf("TimeoutError.Error() = %q, want %q", err.Error(), expected)
	}
}

// validateBusName tests

func TestValidateBusName(t *testing.T) {
	tests := []struct {
		name    string
		busName string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid bus name",
			busName: "org.mpris.MediaPlayer2.spotify",
			wantErr: false,
		},
		{
			name:    "valid bus name with instance",
			busName: "org.mpris.MediaPlayer2.vlc.instance123",
			wantErr: false,
		},
		{
			name:    "empty bus name",
			busName: "",
			wantErr: true,
			errMsg:  "empty bus name",
		},
		{
			name:    "missing prefix",
			busName: "com.example.player",
			wantErr: true,
			errMsg:  "must start with org.mpris.MediaPlayer2.",
		},
		{
			name:    "double dots",
			busName: "org.mpris.MediaPlayer2..spotify",
			wantErr: true,
			errMsg:  "contains illegal characters",
		},
		{
			name:    "contains slash",
			busName: "org.mpris.MediaPlayer2.spo/tify",
			wantErr: true,
			errMsg:  "contains illegal characters",
		},
		{
			name:    "contains null byte",
			busName: "org.mpris.MediaPlayer2.spo\x00tify",
			wantErr: true,
			errMsg:  "contains illegal characters",
		},
		{
			name:    "contains newline",
			busName: "org.mpris.MediaPlayer2.spo\ntify",
			wantErr: true,
			errMsg:  "contains illegal characters",
		},
		{
			name:    "contains carriage return",
			busName: "org.mpris.MediaPlayer2.spo\rtify",
			wantErr: true,
			errMsg:  "contains illegal characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBusName(tt.busName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBusName(%q) error = %v, wantErr %v", tt.busName, err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if invErr, ok := err.(*InvalidBusNameError); ok {
					if invErr.Reason != tt.errMsg {
						t.Errorf("validateBusName(%q) error reason = %q, want %q", tt.busName, invErr.Reason, tt.errMsg)
					}
				}
			}
		})
	}
}

// D-Bus extractor tests

func TestExtractString(t *testing.T) {
	tests := []struct {
		name      string
		variant   dbus.Variant
		wantValue string
		wantOk    bool
	}{
		{
			name:      "valid string",
			variant:   dbus.MakeVariant("test string"),
			wantValue: "test string",
			wantOk:    true,
		},
		{
			name:      "empty string",
			variant:   dbus.MakeVariant(""),
			wantValue: "",
			wantOk:    true,
		},
		{
			name:      "not a string",
			variant:   dbus.MakeVariant(123),
			wantValue: "",
			wantOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := idbus.ExtractString(tt.variant)
			if ok != tt.wantOk {
				t.Errorf("extractString() ok = %v, want %v", ok, tt.wantOk)
			}
			if value != tt.wantValue {
				t.Errorf("extractString() value = %q, want %q", value, tt.wantValue)
			}
		})
	}
}

func TestExtractBool(t *testing.T) {
	tests := []struct {
		name      string
		variant   dbus.Variant
		wantValue bool
		wantOk    bool
	}{
		{
			name:      "true",
			variant:   dbus.MakeVariant(true),
			wantValue: true,
			wantOk:    true,
		},
		{
			name:      "false",
			variant:   dbus.MakeVariant(false),
			wantValue: false,
			wantOk:    true,
		},
		{
			name:      "not a bool",
			variant:   dbus.MakeVariant("true"),
			wantValue: false,
			wantOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := idbus.ExtractBool(tt.variant)
			if ok != tt.wantOk {
				t.Errorf("extractBool() ok = %v, want %v", ok, tt.wantOk)
			}
			if value != tt.wantValue {
				t.Errorf("extractBool() value = %v, want %v", value, tt.wantValue)
			}
		})
	}
}

func TestExtractInt64(t *testing.T) {
	tests := []struct {
		name      string
		variant   dbus.Variant
		wantValue int64
		wantOk    bool
	}{
		{
			name:      "positive int64",
			variant:   dbus.MakeVariant(int64(12345)),
			wantValue: 12345,
			wantOk:    true,
		},
		{
			name:      "negative int64",
			variant:   dbus.MakeVariant(int64(-999)),
			wantValue: -999,
			wantOk:    true,
		},
		{
			name:      "zero",
			variant:   dbus.MakeVariant(int64(0)),
			wantValue: 0,
			wantOk:    true,
		},
		{
			name:      "not an int64",
			variant:   dbus.MakeVariant("123"),
			wantValue: 0,
			wantOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := idbus.ExtractInt64(tt.variant)
			if ok != tt.wantOk {
				t.Errorf("extractInt64() ok = %v, want %v", ok, tt.wantOk)
			}
			if value != tt.wantValue {
				t.Errorf("extractInt64() value = %v, want %v", value, tt.wantValue)
			}
		})
	}
}

func TestExtractFloat64(t *testing.T) {
	tests := []struct {
		name      string
		variant   dbus.Variant
		wantValue float64
		wantOk    bool
	}{
		{
			name:      "float64",
			variant:   dbus.MakeVariant(0.75),
			wantValue: 0.75,
			wantOk:    true,
		},
		{
			name:      "zero",
			variant:   dbus.MakeVariant(0.0),
			wantValue: 0.0,
			wantOk:    true,
		},
		{
			name:      "negative",
			variant:   dbus.MakeVariant(-3.14),
			wantValue: -3.14,
			wantOk:    true,
		},
		{
			name:      "not a float64",
			variant:   dbus.MakeVariant(int64(123)),
			wantValue: 0,
			wantOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := idbus.ExtractFloat64(tt.variant)
			if ok != tt.wantOk {
				t.Errorf("extractFloat64() ok = %v, want %v", ok, tt.wantOk)
			}
			if value != tt.wantValue {
				t.Errorf("extractFloat64() value = %v, want %v", value, tt.wantValue)
			}
		})
	}
}

func TestExtractMetadataMap(t *testing.T) {
	tests := []struct {
		name    string
		variant dbus.Variant
		wantOk  bool
	}{
		{
			name: "valid metadata map",
			variant: dbus.MakeVariant(map[string]dbus.Variant{
				"xesam:title": dbus.MakeVariant("Test Song"),
			}),
			wantOk: true,
		},
		{
			name:    "empty map",
			variant: dbus.MakeVariant(map[string]dbus.Variant{}),
			wantOk:  true,
		},
		{
			name:    "not a map",
			variant: dbus.MakeVariant("invalid"),
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := idbus.ExtractVariantMap(tt.variant)
			if ok != tt.wantOk {
				t.Errorf("extractMetadataMap() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

// Player capability methods tests

func TestPlayerCanPause(t *testing.T) {
	tests := []struct {
		name         string
		capabilities Capabilities
		want         bool
	}{
		{
			name:         "can pause",
			capabilities: Capabilities{CanPause: true},
			want:         true,
		},
		{
			name:         "cannot pause",
			capabilities: Capabilities{CanPause: false},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := &Player{Capabilities: tt.capabilities}
			if got := player.CanPause(); got != tt.want {
				t.Errorf("Player.CanPause() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlayerCanGoPrevious(t *testing.T) {
	tests := []struct {
		name         string
		capabilities Capabilities
		want         bool
	}{
		{
			name:         "can go previous",
			capabilities: Capabilities{CanGoPrevious: true},
			want:         true,
		},
		{
			name:         "cannot go previous",
			capabilities: Capabilities{CanGoPrevious: false},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := &Player{Capabilities: tt.capabilities}
			if got := player.CanGoPrevious(); got != tt.want {
				t.Errorf("Player.CanGoPrevious() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlayerCanSeek(t *testing.T) {
	tests := []struct {
		name         string
		capabilities Capabilities
		want         bool
	}{
		{
			name:         "can seek",
			capabilities: Capabilities{CanSeek: true},
			want:         true,
		},
		{
			name:         "cannot seek",
			capabilities: Capabilities{CanSeek: false},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := &Player{Capabilities: tt.capabilities}
			if got := player.CanSeek(); got != tt.want {
				t.Errorf("Player.CanSeek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlayerCanControl(t *testing.T) {
	tests := []struct {
		name         string
		capabilities Capabilities
		want         bool
	}{
		{
			name:         "can control",
			capabilities: Capabilities{CanControl: true},
			want:         true,
		},
		{
			name:         "cannot control",
			capabilities: Capabilities{CanControl: false},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := &Player{Capabilities: tt.capabilities}
			if got := player.CanControl(); got != tt.want {
				t.Errorf("Player.CanControl() = %v, want %v", got, tt.want)
			}
		})
	}
}

// formatMetadataValue tests

func TestFormatMetadataValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{
			name:  "single string",
			value: "test",
			want:  "test",
		},
		{
			name:  "string array",
			value: []string{"Artist 1", "Artist 2", "Artist 3"},
			want:  "Artist 1, Artist 2, Artist 3",
		},
		{
			name:  "empty string array",
			value: []string{},
			want:  "",
		},
		{
			name:  "single element string array",
			value: []string{"Solo Artist"},
			want:  "Solo Artist",
		},
		{
			name:  "interface array with strings",
			value: []interface{}{"Item 1", "Item 2"},
			want:  "Item 1, Item 2",
		},
		{
			name:  "interface array with mixed types",
			value: []interface{}{"text", 123, true},
			want:  "text, 123, true",
		},
		{
			name:  "empty interface array",
			value: []interface{}{},
			want:  "",
		},
		{
			name:  "integer",
			value: 42,
			want:  "42",
		},
		{
			name:  "float",
			value: 3.14,
			want:  "3.14",
		},
		{
			name:  "boolean",
			value: true,
			want:  "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMetadataValue(tt.value)
			if got != tt.want {
				t.Errorf("formatMetadataValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Extended extractMetadata tests

func TestExtractMetadataWithRealData(t *testing.T) {
	// Test with complete metadata map
	metadata := map[string]dbus.Variant{
		"xesam:title":       dbus.MakeVariant("Test Song"),
		"xesam:artist":      dbus.MakeVariant([]string{"Artist 1", "Artist 2"}),
		"xesam:album":       dbus.MakeVariant("Test Album"),
		"xesam:albumArtist": dbus.MakeVariant([]string{"Album Artist"}),
		"xesam:genre":       dbus.MakeVariant([]string{"Rock", "Alternative"}),
		"mpris:trackid":     dbus.MakeVariant("/org/mpris/MediaPlayer2/track/1"),
		"mpris:artUrl":      dbus.MakeVariant("file:///path/to/art.jpg"),
		"mpris:length":      dbus.MakeVariant(int64(240000000)),
	}

	result := extractMetadata(metadata)

	expectedKeys := []string{
		"xesam:title",
		"xesam:artist",
		"xesam:album",
		"xesam:albumArtist",
		"xesam:genre",
		"mpris:trackid",
		"mpris:artUrl",
		"mpris:length",
	}

	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("extractMetadata() missing key %q", key)
		}
	}

	// Verify specific values
	if result["xesam:title"] != "Test Song" {
		t.Errorf("title = %q, want %q", result["xesam:title"], "Test Song")
	}

	if result["xesam:artist"] != "Artist 1, Artist 2" {
		t.Errorf("artist = %q, want %q", result["xesam:artist"], "Artist 1, Artist 2")
	}
}

// Struct tags validation tests

func TestPlayerStructTags(t *testing.T) {
	// Test Player struct tags
	playerType := reflect.TypeOf(Player{})

	expectedFields := map[string]struct {
		dbusTag  string
		ifaceTag string
	}{
		"Identity":       {dbusTag: "Identity", ifaceTag: "org.mpris.MediaPlayer2"},
		"PlaybackStatus": {dbusTag: "PlaybackStatus", ifaceTag: "org.mpris.MediaPlayer2.Player"},
		"LoopStatus":     {dbusTag: "LoopStatus", ifaceTag: "org.mpris.MediaPlayer2.Player"},
		"Shuffle":        {dbusTag: "Shuffle", ifaceTag: "org.mpris.MediaPlayer2.Player"},
		"Volume":         {dbusTag: "Volume", ifaceTag: "org.mpris.MediaPlayer2.Player"},
		"Position":       {dbusTag: "Position", ifaceTag: "org.mpris.MediaPlayer2.Player"},
		"Rate":           {dbusTag: "Rate", ifaceTag: "org.mpris.MediaPlayer2.Player"},
		"Metadata":       {dbusTag: "Metadata", ifaceTag: "org.mpris.MediaPlayer2.Player"},
	}

	for i := 0; i < playerType.NumField(); i++ {
		field := playerType.Field(i)
		dbusTag := field.Tag.Get("dbus")

		// Skip fields without dbus tag
		if dbusTag == "" {
			continue
		}

		expected, ok := expectedFields[field.Name]
		if !ok {
			t.Errorf("Unexpected field with dbus tag: %s (tag: %q)", field.Name, dbusTag)
			continue
		}

		if dbusTag != expected.dbusTag {
			t.Errorf("Field %s: dbus tag = %q, want %q", field.Name, dbusTag, expected.dbusTag)
		}

		ifaceTag := field.Tag.Get("iface")
		if ifaceTag != expected.ifaceTag {
			t.Errorf("Field %s: iface tag = %q, want %q", field.Name, ifaceTag, expected.ifaceTag)
		}

		// Mark as found
		delete(expectedFields, field.Name)
	}

	// Check if all expected fields were found
	if len(expectedFields) > 0 {
		for fieldName := range expectedFields {
			t.Errorf("Missing expected field with dbus tag: %s", fieldName)
		}
	}
}

func TestCapabilitiesStructTags(t *testing.T) {
	// Test Capabilities struct tags
	capsType := reflect.TypeOf(Capabilities{})

	expectedTags := map[string]string{
		"CanPlay":       "CanPlay",
		"CanPause":      "CanPause",
		"CanGoNext":     "CanGoNext",
		"CanGoPrevious": "CanGoPrevious",
		"CanSeek":       "CanSeek",
		"CanControl":    "CanControl",
	}

	for i := 0; i < capsType.NumField(); i++ {
		field := capsType.Field(i)
		dbusTag := field.Tag.Get("dbus")

		expectedTag, ok := expectedTags[field.Name]
		if !ok {
			t.Errorf("Unexpected field: %s", field.Name)
			continue
		}

		if dbusTag != expectedTag {
			t.Errorf("Field %s: dbus tag = %q, want %q", field.Name, dbusTag, expectedTag)
		}

		// Mark as found
		delete(expectedTags, field.Name)
	}

	// Check if all expected fields were found
	if len(expectedTags) > 0 {
		for fieldName := range expectedTags {
			t.Errorf("Missing expected field with dbus tag: %s", fieldName)
		}
	}
}

// loadCapabilitiesFromProps tests

func TestLoadCapabilitiesFromProps(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]dbus.Variant
		want  Capabilities
	}{
		{
			name: "all capabilities true",
			props: map[string]dbus.Variant{
				"CanPlay":       dbus.MakeVariant(true),
				"CanPause":      dbus.MakeVariant(true),
				"CanGoNext":     dbus.MakeVariant(true),
				"CanGoPrevious": dbus.MakeVariant(true),
				"CanSeek":       dbus.MakeVariant(true),
				"CanControl":    dbus.MakeVariant(true),
			},
			want: Capabilities{
				CanPlay:       true,
				CanPause:      true,
				CanGoNext:     true,
				CanGoPrevious: true,
				CanSeek:       true,
				CanControl:    true,
			},
		},
		{
			name: "all capabilities false",
			props: map[string]dbus.Variant{
				"CanPlay":       dbus.MakeVariant(false),
				"CanPause":      dbus.MakeVariant(false),
				"CanGoNext":     dbus.MakeVariant(false),
				"CanGoPrevious": dbus.MakeVariant(false),
				"CanSeek":       dbus.MakeVariant(false),
				"CanControl":    dbus.MakeVariant(false),
			},
			want: Capabilities{
				CanPlay:       false,
				CanPause:      false,
				CanGoNext:     false,
				CanGoPrevious: false,
				CanSeek:       false,
				CanControl:    false,
			},
		},
		{
			name: "mixed capabilities",
			props: map[string]dbus.Variant{
				"CanPlay":       dbus.MakeVariant(true),
				"CanPause":      dbus.MakeVariant(true),
				"CanGoNext":     dbus.MakeVariant(false),
				"CanGoPrevious": dbus.MakeVariant(false),
				"CanSeek":       dbus.MakeVariant(true),
				"CanControl":    dbus.MakeVariant(true),
			},
			want: Capabilities{
				CanPlay:       true,
				CanPause:      true,
				CanGoNext:     false,
				CanGoPrevious: false,
				CanSeek:       true,
				CanControl:    true,
			},
		},
		{
			name: "partial capabilities (missing properties default to false)",
			props: map[string]dbus.Variant{
				"CanPlay":  dbus.MakeVariant(true),
				"CanPause": dbus.MakeVariant(true),
			},
			want: Capabilities{
				CanPlay:       true,
				CanPause:      true,
				CanGoNext:     false,
				CanGoPrevious: false,
				CanSeek:       false,
				CanControl:    false,
			},
		},
		{
			name:  "empty properties map",
			props: map[string]dbus.Variant{},
			want: Capabilities{
				CanPlay:       false,
				CanPause:      false,
				CanGoNext:     false,
				CanGoPrevious: false,
				CanSeek:       false,
				CanControl:    false,
			},
		},
		{
			name: "properties with wrong types are ignored",
			props: map[string]dbus.Variant{
				"CanPlay":   dbus.MakeVariant(true),
				"CanPause":  dbus.MakeVariant("not a bool"), // Wrong type
				"CanGoNext": dbus.MakeVariant(123),          // Wrong type
				"CanSeek":   dbus.MakeVariant(false),
			},
			want: Capabilities{
				CanPlay:       true,
				CanPause:      false, // Ignored due to wrong type
				CanGoNext:     false, // Ignored due to wrong type
				CanGoPrevious: false,
				CanSeek:       false,
				CanControl:    false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := &Player{}
			got := player.loadCapabilitiesFromProps(tt.props)

			if got.CanPlay != tt.want.CanPlay {
				t.Errorf("CanPlay = %v, want %v", got.CanPlay, tt.want.CanPlay)
			}
			if got.CanPause != tt.want.CanPause {
				t.Errorf("CanPause = %v, want %v", got.CanPause, tt.want.CanPause)
			}
			if got.CanGoNext != tt.want.CanGoNext {
				t.Errorf("CanGoNext = %v, want %v", got.CanGoNext, tt.want.CanGoNext)
			}
			if got.CanGoPrevious != tt.want.CanGoPrevious {
				t.Errorf("CanGoPrevious = %v, want %v", got.CanGoPrevious, tt.want.CanGoPrevious)
			}
			if got.CanSeek != tt.want.CanSeek {
				t.Errorf("CanSeek = %v, want %v", got.CanSeek, tt.want.CanSeek)
			}
			if got.CanControl != tt.want.CanControl {
				t.Errorf("CanControl = %v, want %v", got.CanControl, tt.want.CanControl)
			}
		})
	}
}
