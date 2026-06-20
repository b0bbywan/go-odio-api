package ui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLoadTemplates verifies that all templates load without panic
// This is the most critical test - if templates don't load, the app will panic on startup
func TestLoadTemplates(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LoadTemplates panicked: %v", r)
		}
	}()

	tmpl := LoadTemplates()
	if tmpl == nil {
		t.Fatal("LoadTemplates returned nil")
	}

	// Verify all required templates are defined
	requiredTemplates := []string{
		"base",
		"dashboard",
		"content",
		"section-mpris",
		"section-pulseaudio",
		"section-systemd",
		"section-bluetooth",
		"section-upgrade",
		"upgrade-ring",
		"mpris-player",
		"pulseaudio-sink",
		"systemd-unit",
	}

	for _, name := range requiredTemplates {
		if tmpl.Lookup(name) == nil {
			t.Errorf("Required template '%s' not found", name)
		}
	}
}

// TestSectionTemplates verifies all section templates can be executed without panic
func TestSectionTemplates(t *testing.T) {
	tmpl := LoadTemplates()

	tests := []struct {
		name     string
		template string
		data     interface{}
	}{
		{
			name:     "MPRIS section with empty players",
			template: "section-mpris",
			data:     []PlayerView{},
		},
		{
			name:     "MPRIS section with player",
			template: "section-mpris",
			data: []PlayerView{
				{Name: "test", Artist: "Artist", Title: "Title", State: "Playing"},
			},
		},
		{
			name:     "PulseAudio section",
			template: "section-pulseaudio",
			data: &AudioData{
				Kind: "pipewire",
				DefaultSink: &AudioOutput{
					Name:        "test-sink",
					Description: "Test Sink",
					Volume:      0.5,
					Muted:       false,
					Default:     true,
				},
				Clients: []AudioClient{},
			},
		},
		{
			name:     "Systemd section with empty services",
			template: "section-systemd",
			data:     []ServiceView{},
		},
		{
			name:     "Systemd section with service",
			template: "section-systemd",
			data: []ServiceView{
				{Name: "test.service", Active: true, State: "running"},
			},
		},
		{
			name:     "Bluetooth section powered off",
			template: "section-bluetooth",
			data:     &BluetoothView{Powered: false},
		},
		{
			name:     "Bluetooth section powered on",
			template: "section-bluetooth",
			data:     &BluetoothView{Powered: true, ConnectedCount: 2},
		},
		{
			name:     "Bluetooth section pairing active",
			template: "section-bluetooth",
			data:     &BluetoothView{Powered: true, PairingActive: true, PairingUntilMs: 1_700_000_000_000},
		},
		{
			name:     "Upgrade badge up to date",
			template: "section-upgrade",
			data:     &UpgradeStatus{Current: "1.0", Latest: "1.0", UpgradeAvailable: false},
		},
		{
			name:     "Upgrade badge update available",
			template: "section-upgrade",
			data:     &UpgradeStatus{Current: "1.0", Latest: "1.1", UpgradeAvailable: true},
		},
		{
			name:     "Upgrade badge unknown (no detection)",
			template: "section-upgrade",
			data:     (*UpgradeStatus)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Template %s panicked: %v", tt.template, r)
				}
			}()

			var buf bytes.Buffer
			err := tmpl.ExecuteTemplate(&buf, tt.template, tt.data)
			if err != nil {
				t.Fatalf("Failed to execute %s: %v", tt.template, err)
			}
			if buf.Len() == 0 {
				t.Errorf("%s produced empty output", tt.template)
			}
		})
	}
}

// TestUpgradeBadgeTemplate asserts the badge label per state and that the
// last-check time is surfaced in the tooltip; every state is a re-check button.
func TestUpgradeBadgeTemplate(t *testing.T) {
	tmpl := LoadTemplates()
	checked := time.Date(2026, 6, 15, 20, 46, 34, 0, time.UTC)

	// Badge is icon-only; state is asserted via the tooltip and a distinguishing
	// SVG path fragment of the expected icon.
	tests := []struct {
		name        string
		status      *UpgradeStatus
		wantInTitle string
		wantIconSVG string // path fragment unique to the expected icon
		wantPost    string // hx-post target for the click
		wantConfirm bool   // destructive action must be confirmed
	}{
		{
			name:        "up to date surfaces checked-at in tooltip, check icon, re-checks",
			status:      &UpgradeStatus{Current: "1.0", Latest: "1.0", UpgradeAvailable: false, CheckedAt: checked, CanCheck: true},
			wantInTitle: "Up to date · checked " + (&UpgradeStatus{CheckedAt: checked}).CheckedAtLabel(),
			wantIconSVG: "M20 6 9 17l-5-5", // icon-check
			wantPost:    "/upgrade/check",
		},
		{
			name:        "update available surfaces latest version, arrow-up icon, installs with confirm",
			status:      &UpgradeStatus{Current: "1.0", Latest: "1.1", UpgradeAvailable: true, CheckedAt: checked, CanUpgrade: true},
			wantInTitle: "Upgrade available: 1.1",
			wantIconSVG: "M12 19V5", // icon-arrow-up
			wantPost:    "/upgrade/start",
			wantConfirm: true,
		},
		{
			name:        "unknown shows check prompt, refresh icon, re-checks",
			status:      &UpgradeStatus{CanCheck: true},
			wantInTitle: "Check for upgrades",
			wantIconSVG: "M21 3v5h-5", // icon-rotate-cw
			wantPost:    "/upgrade/check",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tmpl.ExecuteTemplate(&buf, "section-upgrade", tt.status); err != nil {
				t.Fatalf("execute section-upgrade: %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, tt.wantInTitle) {
				t.Errorf("expected tooltip %q in output, got: %s", tt.wantInTitle, out)
			}
			if !strings.Contains(out, tt.wantIconSVG) {
				t.Errorf("expected icon path %q in output, got: %s", tt.wantIconSVG, out)
			}
			if !strings.Contains(out, `hx-post="`+tt.wantPost+`"`) {
				t.Errorf("expected hx-post %q in output, got: %s", tt.wantPost, out)
			}
			if gotConfirm := strings.Contains(out, "hx-confirm="); gotConfirm != tt.wantConfirm {
				t.Errorf("hx-confirm present = %v, want %v; output: %s", gotConfirm, tt.wantConfirm, out)
			}
		})
	}
}

func TestUpgradeBadgeRunning(t *testing.T) {
	tmpl := LoadTemplates()
	pct := 42

	render := func(status *UpgradeStatus) string {
		t.Helper()
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "section-upgrade", status); err != nil {
			t.Fatalf("execute section-upgrade: %v", err)
		}
		return buf.String()
	}

	// Running badge is the ring's sse-swap target, no click action.
	out := render(&UpgradeStatus{Latest: "1.1", Run: &UpgradeRun{State: "running", Percent: &pct}})
	if !strings.Contains(out, `sse-swap="upgrade-progress"`) {
		t.Errorf("running badge should be the ring sse-swap target, got: %s", out)
	}
	if strings.Contains(out, "hx-post=") {
		t.Errorf("running badge should not be clickable, got: %s", out)
	}

	// Ring fill: stroke-dashoffset = 100 - percent.
	if !strings.Contains(out, `stroke-dashoffset="58"`) {
		t.Errorf("ring should reflect 42%% (offset 58), got: %s", out)
	}

	// No percent yet → spinner, no ring.
	noPct := render(&UpgradeStatus{Latest: "1.1", Run: &UpgradeRun{State: "running"}})
	if !strings.Contains(noPct, "spinner") || strings.Contains(noPct, "upgrade-ring-fill") {
		t.Errorf("running without percent should be a spinner, got: %s", noPct)
	}
}

// TestUpgradeBadgeFailed verifies a failed last run surfaces as a red alert that takes
// priority over an available upgrade, offers a retry only when upgradeable, and is hidden
// once a run is live or the last run succeeded.
func TestUpgradeBadgeFailed(t *testing.T) {
	tmpl := LoadTemplates()
	render := func(status *UpgradeStatus) string {
		t.Helper()
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "section-upgrade", status); err != nil {
			t.Fatalf("execute section-upgrade: %v", err)
		}
		return buf.String()
	}
	const alertPath = "m21.73 18" // icon-alert-triangle
	failed := &UpgradeLastRun{Success: false, Step: "mpd", Error: "disk full"}

	// Failure beats an available upgrade, and offers a retry when upgradeable.
	out := render(&UpgradeStatus{Latest: "1.1", UpgradeAvailable: true, CanUpgrade: true, LastRun: failed})
	if !strings.Contains(out, alertPath) {
		t.Errorf("failed badge should show the alert icon, got: %s", out)
	}
	if !strings.Contains(out, `hx-post="/upgrade/start"`) || !strings.Contains(out, "disk full") {
		t.Errorf("failed+upgradeable badge should retry and carry the error, got: %s", out)
	}

	// No upgrade trigger → static alert, not clickable.
	static := render(&UpgradeStatus{Latest: "1.1", UpgradeAvailable: true, LastRun: failed})
	if strings.Contains(static, "hx-post=") || !strings.Contains(static, alertPath) {
		t.Errorf("failed badge without trigger should be a static alert, got: %s", static)
	}

	// A live run hides the last failure; a successful last run shows no alert.
	pct := 10
	if r := render(&UpgradeStatus{Latest: "1.1", Run: &UpgradeRun{State: "running", Percent: &pct}, LastRun: failed}); strings.Contains(r, alertPath) {
		t.Errorf("a running run should hide the last failure, got: %s", r)
	}
	if ok := render(&UpgradeStatus{Latest: "1.0", LastRun: &UpgradeLastRun{Success: true}}); strings.Contains(ok, alertPath) {
		t.Errorf("a successful last run should not show the alert, got: %s", ok)
	}

	// A failure stays visible and retryable even when the system reads as up to date.
	if up := render(&UpgradeStatus{Current: "1.0", Latest: "1.0", UpgradeAvailable: false, CanUpgrade: true, LastRun: failed}); !strings.Contains(up, alertPath) || !strings.Contains(up, `hx-post="/upgrade/start"`) {
		t.Errorf("an up-to-date system with a failed last run should show a retryable alert, got: %s", up)
	}
	// A failure with no detection result still surfaces; the retry just omits the empty version.
	if nr := render(&UpgradeStatus{CanUpgrade: true, LastRun: failed}); !strings.Contains(nr, alertPath) || strings.Contains(nr, "Retry upgrade to ?") {
		t.Errorf("a failure without a detection result should show the alert and a version-less retry, got: %s", nr)
	}
}

// TestUpgradeBadgeGatedActions verifies the badge renders a static icon (no
// hx-post) when the matching trigger is unavailable, e.g. result-file-only mode.
func TestUpgradeBadgeGatedActions(t *testing.T) {
	tmpl := LoadTemplates()

	render := func(status *UpgradeStatus) string {
		t.Helper()
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "section-upgrade", status); err != nil {
			t.Fatalf("execute section-upgrade: %v", err)
		}
		return buf.String()
	}

	cases := []struct {
		name    string
		status  *UpgradeStatus
		wantSVG string
	}{
		{"available without upgrade trigger", &UpgradeStatus{Latest: "1.1", UpgradeAvailable: true}, "M12 19V5"},
		{"up to date without check trigger", &UpgradeStatus{Latest: "1.0", UpgradeAvailable: false}, "M20 6 9 17l-5-5"},
		{"unknown without check trigger", &UpgradeStatus{}, "M21 3v5h-5"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			out := render(tt.status)
			if strings.Contains(out, "hx-post=") {
				t.Errorf("no trigger available, badge must not be clickable, got: %s", out)
			}
			if !strings.Contains(out, tt.wantSVG) {
				t.Errorf("expected icon path %q (still shown as indicator), got: %s", tt.wantSVG, out)
			}
		})
	}
}

// TestComponentTemplates verifies all component templates can be executed without panic
func TestComponentTemplates(t *testing.T) {
	tmpl := LoadTemplates()

	tests := []struct {
		name     string
		template string
		data     interface{}
	}{
		{
			name:     "MPRIS player",
			template: "mpris-player",
			data: PlayerView{
				Name:   "test-player",
				Artist: "Test Artist",
				Title:  "Test Title",
				State:  "Playing",
			},
		},
		{
			name:     "PulseAudio sink",
			template: "pulseaudio-sink",
			data: AudioClient{
				Name:        "test-sink",
				Application: "Test App",
				Volume:      0.75,
				Muted:       false,
			},
		},
		{
			name:     "PulseAudio sink muted",
			template: "pulseaudio-sink",
			data: AudioClient{
				Name:   "test-sink",
				Volume: 0.5,
				Muted:  true,
			},
		},
		{
			name:     "Systemd unit active",
			template: "systemd-unit",
			data: ServiceView{
				Name:        "test.service",
				Description: "Test Service",
				Active:      true,
				State:       "running",
			},
		},
		{
			name:     "Systemd unit inactive",
			template: "systemd-unit",
			data: ServiceView{
				Name:   "test.service",
				Active: false,
				State:  "stopped",
			},
		},
		{
			name:     "Systemd unit with URL",
			template: "systemd-unit",
			data: ServiceView{
				Name:        "mympd.service",
				Description: "myMPD",
				Active:      true,
				State:       "running",
				IsUser:      true,
				URL:         ":8080",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Template %s panicked: %v", tt.template, r)
				}
			}()

			var buf bytes.Buffer
			err := tmpl.ExecuteTemplate(&buf, tt.template, tt.data)
			if err != nil {
				t.Fatalf("Failed to execute %s: %v", tt.template, err)
			}
			if buf.Len() == 0 {
				t.Errorf("%s produced empty output", tt.template)
			}
		})
	}
}

// TestConvertPlayers verifies player conversion logic
func TestConvertPlayers(t *testing.T) {
	tests := []struct {
		name     string
		input    []Player
		expected []PlayerView
	}{
		{
			name:     "empty players",
			input:    []Player{},
			expected: []PlayerView{},
		},
		{
			name: "player with metadata",
			input: []Player{
				{
					Name:   "test-player",
					Status: "Playing",
					Metadata: map[string]string{
						"xesam:artist": "Test Artist",
						"xesam:title":  "Test Title",
						"xesam:album":  "Test Album",
					},
				},
			},
			expected: []PlayerView{
				{
					Name:   "test-player",
					Artist: "Test Artist",
					Title:  "Test Title",
					Album:  "Test Album",
					State:  "Playing",
				},
			},
		},
		{
			name: "player without metadata",
			input: []Player{
				{
					Name:     "test-player",
					Status:   "Paused",
					Metadata: map[string]string{},
				},
			},
			expected: []PlayerView{
				{
					Name:  "test-player",
					State: "Paused",
				},
			},
		},
		{
			name: "player with file:// artUrl gets cover proxy URL",
			input: []Player{
				{
					Name:   "org.mpris.MediaPlayer2.mpd",
					Status: "Playing",
					Metadata: map[string]string{
						"mpris:artUrl":  "file:///tmp/cover.jpg",
						"mpris:trackid": "/org/mpd/track/1",
					},
				},
			},
			expected: []PlayerView{
				{
					Name:   "org.mpris.MediaPlayer2.mpd",
					ArtUrl: "/players/org.mpris.MediaPlayer2.mpd/cover?a=file%3A%2F%2F%2Ftmp%2Fcover.jpg&t=%2Forg%2Fmpd%2Ftrack%2F1",
					State:  "Playing",
				},
			},
		},
		{
			name: "player with https:// artUrl gets cover proxy URL",
			input: []Player{
				{
					Name:   "org.mpris.MediaPlayer2.spotify",
					Status: "Playing",
					Metadata: map[string]string{
						"mpris:artUrl":  "https://i.scdn.co/image/abc123",
						"mpris:trackid": "/com/spotify/track/abc123",
					},
				},
			},
			expected: []PlayerView{
				{
					Name:   "org.mpris.MediaPlayer2.spotify",
					ArtUrl: "/players/org.mpris.MediaPlayer2.spotify/cover?a=https%3A%2F%2Fi.scdn.co%2Fimage%2Fabc123&t=%2Fcom%2Fspotify%2Ftrack%2Fabc123",
					State:  "Playing",
				},
			},
		},
		{
			name: "player without artUrl has empty ArtUrl",
			input: []Player{
				{
					Name:     "org.mpris.MediaPlayer2.vlc",
					Status:   "Playing",
					Metadata: map[string]string{},
				},
			},
			expected: []PlayerView{
				{
					Name:  "org.mpris.MediaPlayer2.vlc",
					State: "Playing",
				},
			},
		},
		{
			name: "stopped player is filtered out",
			input: []Player{
				{
					Name:     "org.mpris.MediaPlayer2.vlc",
					Status:   "Stopped",
					Metadata: map[string]string{},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPlayers(tt.input)
			if tt.expected == nil {
				if len(result) != 0 {
					t.Fatalf("Expected no players, got %d", len(result))
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d players, got %d", len(tt.expected), len(result))
			}
			for i := range result {
				if result[i].Name != tt.expected[i].Name {
					t.Errorf("Player %d: expected name '%s', got '%s'", i, tt.expected[i].Name, result[i].Name)
				}
				if result[i].State != tt.expected[i].State {
					t.Errorf("Player %d: expected state '%s', got '%s'", i, tt.expected[i].State, result[i].State)
				}
				if result[i].ArtUrl != tt.expected[i].ArtUrl {
					t.Errorf("Player %d: expected ArtUrl '%s', got '%s'", i, tt.expected[i].ArtUrl, result[i].ArtUrl)
				}
			}
		})
	}
}

// TestPlayerDisplayName verifies the precedence Identity > cleaned bus_name
// and capitalization of the first letter.
func TestPlayerDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		input    Player
		expected string
	}{
		{
			name:     "identity wins over bus_name and is returned as-is",
			input:    Player{Name: "org.mpris.MediaPlayer2.chromium.instance5961", Identity: "Chrome"},
			expected: "Chrome",
		},
		{
			name:     "identity is not re-capitalized",
			input:    Player{Identity: "audacious"},
			expected: "audacious",
		},
		{
			name:     "fallback strips instance suffix and capitalizes",
			input:    Player{Name: "org.mpris.MediaPlayer2.chromium.instance5961"},
			expected: "Chromium",
		},
		{
			name:     "fallback for plain bus_name",
			input:    Player{Name: "org.mpris.MediaPlayer2.spotify"},
			expected: "Spotify",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := playerDisplayName(tt.input); got != tt.expected {
				t.Errorf("playerDisplayName(%+v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestGetAudio_FilterCorked verifies that corked audio clients are excluded
func TestGetAudio_FilterCorked(t *testing.T) {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/audio", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{
			"kind": "pipewire",
			"clients": [
				{"index": 1, "name": "Spotify", "app": "spotify", "volume": 1.0, "muted": false, "corked": false},
				{"index": 2, "name": "Firefox", "app": "firefox", "volume": 0.5, "muted": false, "corked": true}
			],
			"outputs": []
		}`)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	server := httptest.NewServer(apiMux)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	client := NewAPIClient(port)

	data, err := client.GetAudio()
	if err != nil {
		t.Fatalf("GetAudio failed: %v", err)
	}
	if len(data.Clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(data.Clients))
	}
	if data.Clients[0].Name != "Spotify" {
		t.Errorf("expected Spotify, got %s", data.Clients[0].Name)
	}
}

// TestConvertBluetooth verifies bluetooth conversion logic
func TestConvertBluetooth(t *testing.T) {
	t.Run("nil status returns nil", func(t *testing.T) {
		if got := convertBluetooth(nil); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("powered off no devices", func(t *testing.T) {
		got := convertBluetooth(&BluetoothStatus{Powered: false})
		if got.Powered {
			t.Error("expected Powered=false")
		}
		if got.ConnectedCount != 0 {
			t.Errorf("expected ConnectedCount=0, got %d", got.ConnectedCount)
		}
	})

	t.Run("counts only connected devices", func(t *testing.T) {
		got := convertBluetooth(&BluetoothStatus{
			Powered: true,
			KnownDevices: []BluetoothDevice{
				{Address: "AA:BB:CC:DD:EE:01", Connected: true},
				{Address: "AA:BB:CC:DD:EE:02", Connected: false},
				{Address: "AA:BB:CC:DD:EE:03", Connected: true},
			},
		})
		if got.ConnectedCount != 2 {
			t.Errorf("expected ConnectedCount=2, got %d", got.ConnectedCount)
		}
	})

	t.Run("pairing active exposes the deadline", func(t *testing.T) {
		until := time.Now().Add(45 * time.Second)
		got := convertBluetooth(&BluetoothStatus{
			PairingActive: true,
			PairingUntil:  &until,
		})
		if !got.PairingActive {
			t.Error("expected PairingActive=true")
		}
		// The raw deadline is passed through; clamping/decrement is client-side.
		if got.PairingUntilMs != until.UnixMilli() {
			t.Errorf("expected PairingUntilMs=%d, got %d", until.UnixMilli(), got.PairingUntilMs)
		}
	})

	t.Run("no pairing leaves the deadline zero", func(t *testing.T) {
		got := convertBluetooth(&BluetoothStatus{PairingActive: false})
		if got.PairingUntilMs != 0 {
			t.Errorf("expected PairingUntilMs=0, got %d", got.PairingUntilMs)
		}
	})

	t.Run("sorts connected first, then discovered, then known by label", func(t *testing.T) {
		got := convertBluetooth(&BluetoothStatus{
			Powered: true,
			KnownDevices: []BluetoothDevice{
				{Address: "AA:BB:CC:DD:EE:02", Name: "Bose", Bonded: true},                 // known, idle
				{Address: "AA:BB:CC:DD:EE:01", Name: "JBL", Bonded: true, Connected: true}, // connected
				{Address: "AA:BB:CC:DD:EE:03", Name: "Newbie"},                             // freshly discovered
			},
		})
		want := []string{"JBL", "Newbie", "Bose"}
		for i, w := range want {
			if got.Devices[i].Name != w {
				t.Errorf("device order = [%s %s %s], want %v",
					got.Devices[0].Name, got.Devices[1].Name, got.Devices[2].Name, want)
				break
			}
		}
	})
}

// TestBluetoothDevicesTemplate renders the device dropdown and asserts the
// connect/disconnect buttons carry the device address in hx-vals (html/template
// must not mangle the embedded JSON), and that a nameless device falls back to
// its address.
func TestBluetoothDevicesTemplate(t *testing.T) {
	tmpl := LoadTemplates()
	view := &BluetoothView{
		Powered: true,
		Devices: []BluetoothDevice{
			{Address: "40:C1:F6:D4:67:88", Name: "JBL Go 3", Connected: true},
			{Address: "2C:41:A1:BD:D1:45", Name: "Bose Solo 5", Trusted: true},
			{Address: "A8:71:16:71:A0:9B"}, // discovered, no name
		},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "section-bluetooth", view); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	out := buf.String()

	wants := []string{
		"JBL Go 3",
		"Bose Solo 5",
		"A8:71:16:71:A0:9B",                // nameless device falls back to its address
		`hx-post="/bluetooth/disconnect"`,  // connected device → disconnect
		`hx-post="/bluetooth/connect"`,     // others → connect
		`{"address": "40:C1:F6:D4:67:88"}`, // hx-vals JSON survives html/template
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("expected %q in rendered section, got:\n%s", w, out)
		}
	}
}

// TestSystemdUnitTemplate_URLLink asserts that when a service has a URL, the
// description is rendered as a clickable <a> wired to openServiceUrl, and
// without one the description stays plain text.
func TestSystemdUnitTemplate_URLLink(t *testing.T) {
	tmpl := LoadTemplates()

	tests := []struct {
		name    string
		view    ServiceView
		wantSub string
		denySub string
	}{
		{
			name:    "with URL renders link",
			view:    ServiceView{Name: "mympd.service", Description: "myMPD", Active: true, URL: ":8080"},
			wantSub: `onclick="openServiceUrl(':8080'); return false;"`,
		},
		{
			name:    "with URL has tooltip",
			view:    ServiceView{Name: "mympd.service", Description: "myMPD", Active: true, URL: ":8080"},
			wantSub: `title=":8080"`,
		},
		{
			name:    "with URL appends external-link arrow",
			view:    ServiceView{Name: "mympd.service", Description: "myMPD", Active: true, URL: ":8080"},
			wantSub: "↗",
		},
		{
			name:    "without URL renders no anchor",
			view:    ServiceView{Name: "mpd.service", Description: "MPD", Active: true},
			denySub: "openServiceUrl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tmpl.ExecuteTemplate(&buf, "systemd-unit", tt.view); err != nil {
				t.Fatalf("ExecuteTemplate: %v", err)
			}
			out := buf.String()
			if tt.wantSub != "" && !strings.Contains(out, tt.wantSub) {
				t.Errorf("expected %q in output, got:\n%s", tt.wantSub, out)
			}
			if tt.denySub != "" && strings.Contains(out, tt.denySub) {
				t.Errorf("did not expect %q in output, got:\n%s", tt.denySub, out)
			}
		})
	}
}

// TestConvertServices verifies service conversion logic
func TestConvertServices(t *testing.T) {
	tests := []struct {
		name     string
		input    []Service
		expected []ServiceView
	}{
		{
			name:     "empty services",
			input:    []Service{},
			expected: []ServiceView{},
		},
		{
			name: "active service",
			input: []Service{
				{
					Name:        "test.service",
					Description: "Test Service",
					ActiveState: "active",
					SubState:    "running",
				},
			},
			expected: []ServiceView{
				{
					Name:        "test.service",
					Description: "Test Service",
					Active:      true,
					State:       "running",
				},
			},
		},
		{
			name: "service with URL",
			input: []Service{
				{
					Name:        "mympd.service",
					Description: "myMPD",
					ActiveState: "active",
					SubState:    "running",
					Scope:       "user",
					URL:         ":8080",
				},
			},
			expected: []ServiceView{
				{
					Name:        "mympd.service",
					Description: "myMPD",
					Active:      true,
					State:       "running",
					IsUser:      true,
					URL:         ":8080",
				},
			},
		},
		{
			name: "inactive service",
			input: []Service{
				{
					Name:        "test.service",
					ActiveState: "inactive",
					SubState:    "dead",
				},
			},
			expected: []ServiceView{
				{
					Name:   "test.service",
					Active: false,
					State:  "dead",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertServices(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d services, got %d", len(tt.expected), len(result))
			}
			for i := range result {
				if result[i].Active != tt.expected[i].Active {
					t.Errorf("Service %d: expected active=%v, got %v", i, tt.expected[i].Active, result[i].Active)
				}
				if result[i].URL != tt.expected[i].URL {
					t.Errorf("Service %d: expected URL=%q, got %q", i, tt.expected[i].URL, result[i].URL)
				}
			}
		})
	}
}
