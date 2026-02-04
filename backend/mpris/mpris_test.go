package mpris

import (
	"testing"

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
	backend.cache.Set(cacheKey, players)

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
	backend.cache.Set(cacheKey, initialPlayers)

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
	backend.cache.Set(cacheKey, initialPlayers)

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
	players, _ := backend.cache.Get(cacheKey)
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
	backend.cache.Set(cacheKey, players)

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
	cachedPlayers, _ := backend.cache.Get(cacheKey)
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
	backend.cache.Set(cacheKey, players)

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
