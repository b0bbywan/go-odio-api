package ui

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path"
	"strings"
)

const adminPrefix = "/ui/admin"

// adminProxy serves the admin app listening on socket under adminPrefix; the
// app is expected to root its URLs at X-Forwarded-Prefix.
func adminProxy(socket string) http.Handler {
	var dialer net.Dialer
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "localhost"
			pr.Out.Host = "localhost"
			pr.SetXForwarded()
			// Set, not add: Rewrite keeps a client's own X-Forwarded-Prefix.
			pr.Out.Header.Set("X-Forwarded-Prefix", adminPrefix)
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", socket)
			},
		},
	}
	return http.StripPrefix(adminPrefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isCleanPath(r.URL.Path) {
			http.Error(w, "Bad path", http.StatusBadRequest)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
}

// isCleanPath rejects what the mux lets through encoded, like %2e%2e or %2f%2f.
func isCleanPath(p string) bool {
	c := path.Clean(p)
	return strings.HasPrefix(p, "/") && (p == c || p == c+"/")
}

// adminLink prefers the proxy while the socket exists; with systemd socket
// activation it does before the app itself is started.
func (h *Handler) adminLink() (link string, proxied bool) {
	if h.adminSocket != "" {
		if _, err := os.Stat(h.adminSocket); err == nil {
			return adminPrefix + "/", true
		}
	}
	return h.admin, false
}
