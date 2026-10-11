package mpris

import (
	"testing"
	"time"
)

func TestActivePlayer(t *testing.T) {
	const (
		spotify = "org.mpris.MediaPlayer2.spotify"
		vlc     = "org.mpris.MediaPlayer2.vlc"
		kodi    = "org.mpris.MediaPlayer2.kodi"
	)
	now := time.Now()
	older, newer := now.Add(-time.Minute), now
	player := func(busName string, status PlaybackStatus, positionAt time.Time) Player {
		return Player{BusName: busName, PlaybackStatus: status, PositionUpdatedAt: positionAt}
	}

	tests := []struct {
		name    string
		players []Player
		want    string
		wantOK  bool
	}{
		{name: "cache never loaded"},
		{name: "no players", players: []Player{}},
		{
			name:    "only player, stopped",
			players: []Player{player(spotify, StatusStopped, older)},
			want:    spotify, wantOK: true,
		},
		{
			name: "playing beats paused and stopped, whatever the position age",
			players: []Player{
				player(spotify, StatusPaused, newer),
				player(vlc, StatusPlaying, older),
				player(kodi, StatusStopped, newer),
			},
			want: vlc, wantOK: true,
		},
		{
			name: "paused beats stopped",
			players: []Player{
				player(spotify, StatusStopped, newer),
				player(vlc, StatusPaused, older),
			},
			want: vlc, wantOK: true,
		},
		{
			name: "several playing, latest position wins",
			players: []Player{
				player(spotify, StatusPlaying, older),
				player(vlc, StatusPlaying, newer),
			},
			want: vlc, wantOK: true,
		},
		{
			name: "several paused, latest position wins",
			players: []Player{
				player(spotify, StatusPaused, newer),
				player(vlc, StatusPaused, older),
			},
			want: spotify, wantOK: true,
		},
		{
			name: "same status and position age keeps cache order",
			players: []Player{
				player(spotify, StatusStopped, older),
				player(vlc, StatusStopped, older),
			},
			want: spotify, wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &MPRISBackend{}
			if tt.players != nil {
				b.players.Store(tt.players)
			}

			got, ok := b.ActivePlayer()
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ActivePlayer() = (%q, %t), want (%q, %t)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
