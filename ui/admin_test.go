package ui

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// adminSocket serves h on a Unix socket; t.TempDir can exceed sun_path's 108 bytes.
func adminSocket(t *testing.T, h http.Handler) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "odio")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "admin.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return socket
}

func uiServer(t *testing.T, socket string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	h := &Handler{adminSocket: socket, client: NewAPIClient(1)}
	h.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// proxied sends GET path with header through the proxy and returns the
// request the admin app received.
func proxied(t *testing.T, path, host string, header http.Header) *http.Request {
	t.Helper()
	got := make(chan *http.Request, 1)
	srv := uiServer(t, adminSocket(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r
	})))
	req, err := http.NewRequest("GET", srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	select {
	case r := <-got:
		return r
	default:
		t.Fatalf("GET %s did not reach the admin app (status %d)", path, resp.StatusCode)
		return nil
	}
}

func TestAdminProxy_Path(t *testing.T) {
	tests := []struct {
		url, path, query string
	}{
		{"/ui/admin/", "/", ""},
		{"/ui/admin/static/app.js?v=1", "/static/app.js", "v=1"},
		{"/ui/admin/events", "/events", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			r := proxied(t, tt.url, "", nil)
			if r.URL.Path != tt.path || r.URL.RawQuery != tt.query {
				t.Errorf("admin app got %s, want %s?%s", r.URL, tt.path, tt.query)
			}
		})
	}
}

func TestAdminProxy_PostBody(t *testing.T) {
	got := make(chan string, 1)
	srv := uiServer(t, adminSocket(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got <- r.Method + " " + r.PostForm.Get("id")
	})))
	resp, err := http.PostForm(srv.URL+"/ui/admin/dac", url.Values{"id": {"hifiberry"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if g := <-got; g != "POST hifiberry" {
		t.Errorf("admin app got %q, want \"POST hifiberry\"", g)
	}
}

// The admin app trusts X-Forwarded-* on its socket: none of it may come from
// the client, however it is smuggled.
func TestAdminProxy_ForwardedHeadersAreOurs(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
	}{
		{"none", nil},
		{"spoofed", http.Header{
			"X-Forwarded-Prefix": {"//evil.example"},
			"X-Forwarded-Host":   {"evil.example"},
			"X-Forwarded-For":    {"6.6.6.6"},
			"X-Forwarded-Proto":  {"https"},
			"Forwarded":          {"for=6.6.6.6;host=evil.example"},
		}},
		{"repeated", http.Header{
			"X-Forwarded-Prefix": {"/a", "/b"},
			"X-Forwarded-Host":   {"evil.example", "evil2.example"},
		}},
		{"comma list", http.Header{
			"X-Forwarded-Prefix": {"/ui/admin, /evil"},
			"X-Forwarded-For":    {"127.0.0.1, 6.6.6.6"},
		}},
		// Headers named in Connection are dropped as hop-by-hop: ours must survive it.
		{"connection strips ours", http.Header{
			"Connection": {"X-Forwarded-Prefix, X-Forwarded-Host, X-Forwarded-For"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := proxied(t, "/ui/admin/", "odio.local:8018", tt.header)
			want := map[string]string{
				"X-Forwarded-Prefix": adminPrefix,
				"X-Forwarded-Host":   "odio.local:8018",
				"X-Forwarded-For":    "127.0.0.1",
				"X-Forwarded-Proto":  "http",
			}
			for k, v := range want {
				if got := r.Header.Values(k); len(got) != 1 || got[0] != v {
					t.Errorf("%s = %q, want [%s]", k, got, v)
				}
			}
			if f := r.Header.Values("Forwarded"); len(f) != 0 {
				t.Errorf("Forwarded = %q, want it dropped", f)
			}
		})
	}
}

func TestAdminProxy_UpstreamHostIsFixed(t *testing.T) {
	if r := proxied(t, "/ui/admin/", "evil.example", nil); r.Host != "localhost" {
		t.Errorf("admin app saw Host %q, want localhost", r.Host)
	}
}

// Only what lies under the prefix reaches the admin app.
func TestAdminProxy_PrefixBoundary(t *testing.T) {
	hit := make(chan string, 8)
	srv := uiServer(t, adminSocket(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- r.URL.Path
	})))
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	for _, p := range []string{
		"/ui/administrator",
		"/ui/admin.sock",
		"/ui/admin/../static/odio.js",
		"/ui/admin/%2e%2e/static/odio.js",
		"/ui/admin/%2E%2E%2Fstatic/odio.js",
		"/ui/admin/%2f%2fevil.example/",
		"/ui/admin/./events",
		"/ui/./admin/../sections/mpris",
	} {
		t.Run(p, func(t *testing.T) {
			resp, err := client.Get(srv.URL + p)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			select {
			case got := <-hit:
				t.Errorf("%s reached the admin app as %s", p, got)
			default:
			}
		})
	}
}

func TestAdminProxy_RedirectsBarePrefix(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{adminSocket: "/nonexistent.sock"}).RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/ui/admin", nil))
	if loc := rec.Header().Get("Location"); loc != "/ui/admin/" {
		t.Errorf("got %d → %q, want a redirect to /ui/admin/", rec.Code, loc)
	}
}

func TestAdminProxy_NotRegisteredWithoutSocket(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).RegisterRoutes(mux)

	if _, pattern := mux.Handler(httptest.NewRequest("GET", "/ui/admin/", nil)); pattern != "/ui/" {
		t.Errorf("/ui/admin/ routed to %q, want the dashboard's /ui/", pattern)
	}
}

// A socket that is gone answers 502 without telling the client where it was.
func TestAdminProxy_SocketDown(t *testing.T) {
	srv := uiServer(t, "/nonexistent/admin.sock")
	resp, err := http.Get(srv.URL + "/ui/admin/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	if strings.Contains(string(body), "nonexistent") {
		t.Errorf("body leaks the socket path: %q", body)
	}
}

// SSE events must reach the browser as they are sent, not when the stream ends.
func TestAdminProxy_StreamsSSE(t *testing.T) {
	release := make(chan struct{})
	srv := uiServer(t, adminSocket(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})))
	defer close(release)

	resp, err := http.Get(srv.URL + "/ui/admin/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	line := make(chan string, 1)
	go func() {
		l, _ := bufio.NewReader(resp.Body).ReadString('\n')
		line <- l
	}()
	select {
	case l := <-line:
		if l != "data: first\n" {
			t.Errorf("first line = %q", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event held back by the proxy")
	}
}

func TestAdminLink(t *testing.T) {
	socket := adminSocket(t, http.NotFoundHandler())

	tests := []struct {
		name   string
		admin  string
		socket string
		want   string
	}{
		{"nothing configured", "", "", ""},
		{"port link only", ":8021", "", ":8021"},
		{"socket present wins", ":8021", socket, "/ui/admin/"},
		{"socket missing falls back", ":8021", socket + ".missing", ":8021"},
		{"socket missing, no fallback", "", socket + ".missing", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{admin: tt.admin, adminSocket: tt.socket}
			if got := h.adminLink(); got != tt.want {
				t.Errorf("adminLink() = %q, want %q", got, tt.want)
			}
		})
	}
}
