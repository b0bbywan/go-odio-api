package ui

import "net/http"

// RegisterRoutes registers all UI routes to the provided mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Main dashboard page
	mux.HandleFunc("/ui", h.Dashboard)
	mux.HandleFunc("/ui/", h.Dashboard)

	// SSE event stream (HTML fragments)
	mux.HandleFunc("GET /ui/events", h.SSEEvents)

	// Section fragments (fallback / initial load)
	mux.HandleFunc("/ui/sections/mpris", h.MPRISSection)
	mux.HandleFunc("/ui/sections/audio", h.AudioSection)
	mux.HandleFunc("/ui/sections/systemd", h.SystemdSection)
	mux.HandleFunc("/ui/sections/bluetooth", h.BluetoothSection)

	// Static assets (CSS)
	mux.Handle("/ui/static/", http.StripPrefix("/ui/", http.FileServer(http.FS(staticFS))))
}
