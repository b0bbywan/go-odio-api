package mpris

// ActivePlayer picks the player a command without an explicit target should
// reach: playing before paused before stopped, and within the same status the
// one whose position moved last.
func (m *MPRISBackend) ActivePlayer() (string, bool) {
	players := m.players.Load()
	if len(players) == 0 {
		return "", false
	}

	best := &players[0]
	for i := 1; i < len(players); i++ {
		if moreActive(&players[i], best) {
			best = &players[i]
		}
	}
	return best.BusName, true
}

func moreActive(a, b *Player) bool {
	if ra, rb := statusRank(a.PlaybackStatus), statusRank(b.PlaybackStatus); ra != rb {
		return ra > rb
	}
	return a.PositionUpdatedAt.After(b.PositionUpdatedAt)
}

func statusRank(s PlaybackStatus) int {
	switch s {
	case StatusPlaying:
		return 2
	case StatusPaused:
		return 1
	}
	return 0
}
