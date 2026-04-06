package ui

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/b0bbywan/go-odio-api/logger"
)

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

	// Static assets with ETag support (embed.FS has no useful Last-Modified)
	mux.Handle("/ui/static/", etagHandler(http.StripPrefix("/ui/", http.FileServer(http.FS(staticFS)))))
}

// etagHandler wraps an http.Handler to add ETag support for embedded static files.
// ETags are computed once at startup from file content hashes.
func etagHandler(next http.Handler) http.Handler {
	etags := buildETagMap()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/ui/")
		if etag, ok := etags[path]; ok {
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "no-cache")
			if match := r.Header.Get("If-None-Match"); match == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// buildETagMap computes SHA-256 ETags for all files in the embedded static FS.
func buildETagMap() map[string]string {
	etags := make(map[string]string)
	entries, err := staticFS.ReadDir("static")
	if err != nil {
		return etags
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		f, err := staticFS.Open("static/" + entry.Name())
		if err != nil {
			continue
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			logger.Warn("[ui/etag] failed to hash %s: %v", entry.Name(), copyErr)
			continue
		}
		if closeErr != nil {
			logger.Warn("[ui/etag] failed to close %s: %v", entry.Name(), closeErr)
			continue
		}
		etags["static/"+entry.Name()] = fmt.Sprintf(`"%x"`, h.Sum(nil))
	}
	return etags
}
