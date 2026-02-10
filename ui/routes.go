package ui

import "net/http"

// RegisterRoutes registers all UI routes to the provided mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Main dashboard page
	mux.HandleFunc("/ui", h.Dashboard)
	mux.HandleFunc("/ui/", h.Dashboard)

	// Section fragments for HTMX updates/polling
	mux.HandleFunc("/ui/sections/mpris", h.MPRISSection)
	mux.HandleFunc("/ui/sections/audio", h.AudioSection)
	mux.HandleFunc("/ui/sections/systemd", h.SystemdSection)
}
