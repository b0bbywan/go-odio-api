package ui

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/b0bbywan/go-odio-api/logger"
)

//go:embed templates
var templatesFS embed.FS

func LoadTemplates() *template.Template {
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 {
			return a * b
		},
	}
	// Load all .gohtml templates recursively
	tmpl := template.New("").Funcs(funcMap)
	return template.Must(tmpl.ParseFS(templatesFS,
		"templates/base.gohtml",
		"templates/pages/*.gohtml",
		"templates/sections/*.gohtml",
		"templates/components/*.gohtml",
	))
}

// Handler manages UI routes and rendering
type Handler struct {
	tmpl   *template.Template
	client *APIClient
}

// NewHandler creates a new UI handler with API client
func NewHandler(apiPort int) *Handler {
	return &Handler{
		tmpl:   LoadTemplates(),
		client: NewAPIClient(apiPort),
	}
}

// Dashboard renders the main dashboard page
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	logger.Debug("[ui] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Fetch server info to know which backends are available
	logger.Debug("[ui] → API GET /server")
	serverInfo, err := h.client.GetServerInfo()
	if err != nil {
		logger.Error("[ui] Failed to fetch server info: %v", err)
		http.Error(w, "Failed to load server information", http.StatusInternalServerError)
		return
	}
	logger.Debug("[ui] ← API /server: %d backends enabled",
		boolToInt(serverInfo.Backends.MPRIS)+
			boolToInt(serverInfo.Backends.PulseAudio)+
			boolToInt(serverInfo.Backends.Systemd))

	// Build view data
	data := DashboardView{
		Title:      "Go-Odio",
		ServerInfo: serverInfo,
	}

	// Conditionally fetch data based on enabled backends
	if serverInfo.Backends.MPRIS {
		logger.Debug("[ui] → API GET /players")
		if players, err := h.client.GetPlayers(); err == nil {
			data.Players = convertPlayers(players)
			logger.Debug("[ui] ← API /players: %d players", len(players))
		} else {
			logger.Warn("[ui] Failed to fetch players: %v", err)
		}
	}

	if serverInfo.Backends.PulseAudio {
		logger.Debug("[ui] → API GET /audio/server")
		if audioInfo, err := h.client.GetAudioInfo(); err == nil {
			data.AudioInfo = audioInfo
			logger.Debug("[ui] ← API /audio/server: volume=%.2f muted=%v", audioInfo.Volume, audioInfo.Muted)
		} else {
			logger.Warn("[ui] Failed to fetch audio info: %v", err)
		}

		logger.Debug("[ui] → API GET /audio/clients")
		if audioClients, err := h.client.GetAudioClients(); err == nil {
			data.AudioClients = audioClients
			logger.Debug("[ui] ← API /audio/clients: %d clients", len(audioClients))
		} else {
			logger.Warn("[ui] Failed to fetch audio clients: %v", err)
		}
	}

	if serverInfo.Backends.Systemd {
		logger.Debug("[ui] → API GET /services")
		if services, err := h.client.GetServices(); err == nil {
			data.Services = convertServices(services)
			logger.Debug("[ui] ← API /services: %d services", len(services))
		} else {
			logger.Warn("[ui] Failed to fetch services: %v", err)
		}
	}

	// Render template
	if err := h.tmpl.ExecuteTemplate(w, "dashboard", data); err != nil {
		logger.Error("[ui] Template execution failed: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// MPRISSection renders just the MPRIS section (for HTMX updates)
func (h *Handler) MPRISSection(w http.ResponseWriter, r *http.Request) {
	logger.Debug("[ui] %s %s (HTMX section refresh)", r.Method, r.URL.Path)

	logger.Debug("[ui] → API GET /players")
	players, err := h.client.GetPlayers()
	if err != nil {
		logger.Error("[ui] Failed to fetch players: %v", err)
		http.Error(w, "Failed to load players", http.StatusInternalServerError)
		return
	}
	logger.Debug("[ui] ← API /players: %d players", len(players))

	playerViews := convertPlayers(players)
	if err := h.tmpl.ExecuteTemplate(w, "section-mpris", playerViews); err != nil {
		logger.Error("[ui] Template execution failed: %v", err)
		http.Error(w, "Failed to render section", http.StatusInternalServerError)
	}
}

// AudioSection renders just the PulseAudio section (for HTMX updates)
func (h *Handler) AudioSection(w http.ResponseWriter, r *http.Request) {
	logger.Debug("[ui] %s %s (HTMX section refresh)", r.Method, r.URL.Path)

	logger.Debug("[ui] → API GET /audio/server")
	audioInfo, err := h.client.GetAudioInfo()
	if err != nil {
		logger.Error("[ui] Failed to fetch audio info: %v", err)
		http.Error(w, "Failed to load audio info", http.StatusInternalServerError)
		return
	}
	logger.Debug("[ui] ← API /audio/server: volume=%.2f muted=%v", audioInfo.Volume, audioInfo.Muted)

	logger.Debug("[ui] → API GET /audio/clients")
	audioClients, err := h.client.GetAudioClients()
	if err != nil {
		logger.Error("[ui] Failed to fetch audio clients: %v", err)
		http.Error(w, "Failed to load audio clients", http.StatusInternalServerError)
		return
	}
	logger.Debug("[ui] ← API /audio/clients: %d clients", len(audioClients))

	data := struct {
		AudioInfo    *AudioInfo
		AudioClients []AudioClient
	}{
		AudioInfo:    audioInfo,
		AudioClients: audioClients,
	}

	if err := h.tmpl.ExecuteTemplate(w, "section-pulseaudio", data); err != nil {
		logger.Error("[ui] Template execution failed: %v", err)
		http.Error(w, "Failed to render section", http.StatusInternalServerError)
	}
}

// SystemdSection renders just the Systemd section (for HTMX updates)
func (h *Handler) SystemdSection(w http.ResponseWriter, r *http.Request) {
	logger.Debug("[ui] %s %s (HTMX section refresh)", r.Method, r.URL.Path)

	logger.Debug("[ui] → API GET /services")
	services, err := h.client.GetServices()
	if err != nil {
		logger.Error("[ui] Failed to fetch services: %v", err)
		http.Error(w, "Failed to load services", http.StatusInternalServerError)
		return
	}
	logger.Debug("[ui] ← API /services: %d services", len(services))

	serviceViews := convertServices(services)
	if err := h.tmpl.ExecuteTemplate(w, "section-systemd", serviceViews); err != nil {
		logger.Error("[ui] Template execution failed: %v", err)
		http.Error(w, "Failed to render section", http.StatusInternalServerError)
	}
}

// convertPlayers converts API Player types to view-optimized PlayerView
func convertPlayers(players []Player) []PlayerView {
	views := make([]PlayerView, 0, len(players))
	for _, p := range players {
		views = append(views, PlayerView{
			Name:   p.Name,
			Artist: p.Metadata["xesam:artist"],
			Title:  p.Metadata["xesam:title"],
			Album:  p.Metadata["xesam:album"],
			State:  p.Status,
		})
	}
	return views
}

// convertServices converts API Service types to view-optimized ServiceView
func convertServices(services []Service) []ServiceView {
	views := make([]ServiceView, 0, len(services))
	for _, s := range services {
		views = append(views, ServiceView{
			Name:        s.Name,
			Description: s.Description,
			Active:      s.ActiveState == "active",
			State:       s.SubState,
		})
	}
	return views
}

// boolToInt converts bool to int for counting
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
