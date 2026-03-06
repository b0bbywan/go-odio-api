package ui

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/b0bbywan/go-odio-api/logger"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

func LoadTemplates() *template.Template {
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"fmtMicros": func(us int64) string {
			total := int(us / 1_000_000)
			return fmt.Sprintf("%d:%02d", total/60, total%60)
		},
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict requires an even number of arguments")
			}
			d := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				d[key] = values[i+1]
			}
			return d, nil
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
		Title:      "Odio",
		ServerInfo: serverInfo,
	}

	// Conditionally fetch data based on enabled backends
	if serverInfo.Backends.MPRIS {
		logger.Debug("[ui] → API GET /players")
		if players, err := h.client.GetPlayers(); err == nil {
			data.Players = players
			logger.Debug("[ui] ← API /players: %d players", len(players))
		} else {
			logger.Warn("[ui] Failed to fetch players: %v", err)
		}
	}

	if serverInfo.Backends.PulseAudio {
		logger.Debug("[ui] → API GET /audio")
		if audioData, err := h.client.GetAudio(); err == nil {
			data.AudioData = audioData
			logger.Debug("[ui] ← API /audio: %d clients, %d outputs", len(audioData.Clients), len(audioData.Outputs))
		} else {
			logger.Warn("[ui] Failed to fetch audio data: %v", err)
		}
	}

	if serverInfo.Backends.Bluetooth {
		logger.Debug("[ui] → API GET /bluetooth")
		if bt, err := h.client.GetBluetoothStatus(); err == nil {
			data.Bluetooth = bt
			logger.Debug("[ui] ← API /bluetooth: powered=%v pairing=%v", bt.Powered, bt.PairingActive)
		} else {
			logger.Warn("[ui] Failed to fetch bluetooth status: %v", err)
		}
	}

	if serverInfo.Backends.Systemd {
		logger.Debug("[ui] → API GET /services")
		if services, err := h.client.GetServices(); err == nil {
			data.Services = services
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

	if err := h.tmpl.ExecuteTemplate(w, "section-mpris", players); err != nil {
		logger.Error("[ui] Template execution failed: %v", err)
		http.Error(w, "Failed to render section", http.StatusInternalServerError)
	}
}

// AudioSection renders just the PulseAudio section (for HTMX updates)
func (h *Handler) AudioSection(w http.ResponseWriter, r *http.Request) {
	logger.Debug("[ui] %s %s (HTMX section refresh)", r.Method, r.URL.Path)

	logger.Debug("[ui] → API GET /audio")
	audioData, err := h.client.GetAudio()
	if err != nil {
		logger.Error("[ui] Failed to fetch audio data: %v", err)
		http.Error(w, "Failed to load audio data", http.StatusInternalServerError)
		return
	}
	logger.Debug("[ui] ← API /audio: %d clients, %d outputs", len(audioData.Clients), len(audioData.Outputs))

	if err := h.tmpl.ExecuteTemplate(w, "section-pulseaudio", audioData); err != nil {
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

	if err := h.tmpl.ExecuteTemplate(w, "section-systemd", services); err != nil {
		logger.Error("[ui] Template execution failed: %v", err)
		http.Error(w, "Failed to render section", http.StatusInternalServerError)
	}
}

// BluetoothSection renders just the Bluetooth section (for HTMX updates)
func (h *Handler) BluetoothSection(w http.ResponseWriter, r *http.Request) {
	logger.Debug("[ui] %s %s (HTMX section refresh)", r.Method, r.URL.Path)

	logger.Debug("[ui] → API GET /bluetooth")
	btStatus, err := h.client.GetBluetoothStatus()
	if err != nil {
		logger.Error("[ui] Failed to fetch bluetooth status: %v", err)
		http.Error(w, "Failed to load bluetooth status", http.StatusInternalServerError)
		return
	}
	logger.Debug("[ui] ← API /bluetooth: powered=%v pairing=%v", btStatus.Powered, btStatus.PairingActive)

	if err := h.tmpl.ExecuteTemplate(w, "section-bluetooth", btStatus); err != nil {
		logger.Error("[ui] Template execution failed: %v", err)
		http.Error(w, "Failed to render section", http.StatusInternalServerError)
	}
}

// boolToInt converts bool to int for counting
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
