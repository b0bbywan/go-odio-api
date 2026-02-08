package ui

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates/**/*.gohtml
var templatesFS embed.FS

func LoadTemplates() *template.Template {
	return template.Must(template.New("").ParseFS(templatesFS, "templates/**/*.gohtml"))
}

type UIHandler struct {
	tmpl *template.Template
	api  APIClient // ton client interne pour /server etc.
}

func (h *UIHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	info, err := h.api.GetServerInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := DashboardView{
		Title: "Go-Odio",
		Backends: BackendsView{
			MPRIS:      info.Backends.MPRIS,
			PulseAudio: info.Backends.PulseAudio,
			Systemd:    info.Backends.Systemd,
		},
	}

	if err := h.tmpl.ExecuteTemplate(w, "dashboard", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
