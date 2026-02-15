package ui

import (
	"bytes"
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
			data: struct {
				AudioInfo    *AudioInfo
				AudioClients []AudioClient
			}{
				AudioInfo: &AudioInfo{
					DefaultSink: "test-sink",
					Volume:      0.5,
					Muted:       false,
				},
				AudioClients: []AudioClient{},
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
			data:     &BluetoothView{Powered: true, PairingActive: true, PairingSecondsLeft: 42},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPlayers(tt.input)
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
			}
		})
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

	t.Run("pairing active with future deadline", func(t *testing.T) {
		until := time.Now().Add(45 * time.Second)
		got := convertBluetooth(&BluetoothStatus{
			PairingActive: true,
			PairingUntil:  &until,
		})
		if !got.PairingActive {
			t.Error("expected PairingActive=true")
		}
		if got.PairingSecondsLeft <= 0 || got.PairingSecondsLeft > 45 {
			t.Errorf("expected PairingSecondsLeft in (0,45], got %d", got.PairingSecondsLeft)
		}
	})

	t.Run("pairing active with expired deadline", func(t *testing.T) {
		until := time.Now().Add(-5 * time.Second)
		got := convertBluetooth(&BluetoothStatus{
			PairingActive: true,
			PairingUntil:  &until,
		})
		if got.PairingSecondsLeft != 0 {
			t.Errorf("expected PairingSecondsLeft=0, got %d", got.PairingSecondsLeft)
		}
	})
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
			}
		})
	}
}
