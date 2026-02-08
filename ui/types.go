package ui

type BackendsView struct {
	MPRIS      bool
	PulseAudio bool
	Systemd    bool
}

type DashboardView struct {
	Title    string
	Backends BackendsView
}

// Exemple de struct pour les composants
type MPRISPlayer struct {
	Name   string
	Artist string
	Title  string
	State  string
}
