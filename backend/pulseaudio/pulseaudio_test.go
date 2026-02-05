package pulseaudio

import (
	"testing"

	"github.com/the-jonsey/pulseaudio"
)

func TestDetectServerKind(t *testing.T) {
	tests := []struct {
		name     string
		server   *pulseaudio.Server
		expected AudioServerKind
	}{
		{
			name: "PulseAudio server",
			server: &pulseaudio.Server{
				PackageName: "pulseaudio",
			},
			expected: ServerPulse,
		},
		{
			name: "PipeWire server (lowercase)",
			server: &pulseaudio.Server{
				PackageName: "pipewire-pulse",
			},
			expected: ServerPipeWire,
		},
		{
			name: "PipeWire server (uppercase)",
			server: &pulseaudio.Server{
				PackageName: "PipeWire",
			},
			expected: ServerPipeWire,
		},
		{
			name: "PipeWire server (mixed case)",
			server: &pulseaudio.Server{
				PackageName: "PiPeWiRe",
			},
			expected: ServerPipeWire,
		},
		{
			name: "Unknown server defaults to PulseAudio",
			server: &pulseaudio.Server{
				PackageName: "unknown-audio-server",
			},
			expected: ServerPulse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectServerKind(tt.server)
			if result != tt.expected {
				t.Errorf("detectServerKind() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCloneProps(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:     "nil map",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]string{},
		},
		{
			name: "map with values",
			input: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cloneProps(tt.input)

			// Check if both are nil
			if tt.input == nil && result != nil {
				t.Errorf("cloneProps() = %v, want nil", result)
				return
			}

			// Check length
			if len(result) != len(tt.expected) {
				t.Errorf("cloneProps() length = %d, want %d", len(result), len(tt.expected))
				return
			}

			// Check values
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("cloneProps()[%s] = %s, want %s", k, result[k], v)
				}
			}

			// Ensure it's a deep copy (modifying original shouldn't affect clone)
			if tt.input != nil && len(tt.input) > 0 {
				tt.input["new_key"] = "new_value"
				if _, exists := result["new_key"]; exists {
					t.Error("cloneProps() did not create a deep copy")
				}
			}
		})
	}
}

func TestExtractModuleSource(t *testing.T) {
	tests := []struct {
		name     string
		arg      string
		expected string
	}{
		{
			name:     "valid source with quotes",
			arg:      `source="bluez_source.C8_2A_DD_A7_D5_0D.a2dp_source" sink=alsa_output`,
			expected: "bluez_source.C8_2A_DD_A7_D5_0D.a2dp_source",
		},
		{
			name:     "valid source without quotes",
			arg:      `source=bluez_source.test sink=alsa_output`,
			expected: "bluez_source.test",
		},
		{
			name:     "source at end",
			arg:      `sink=alsa_output source="bluez_source.device"`,
			expected: "bluez_source.device",
		},
		{
			name:     "no source parameter",
			arg:      `sink=alsa_output rate=48000`,
			expected: "",
		},
		{
			name:     "empty string",
			arg:      "",
			expected: "",
		},
		{
			name:     "source only",
			arg:      `source="bluez_source.only"`,
			expected: "bluez_source.only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractModuleSource(tt.arg)
			if result != tt.expected {
				t.Errorf("extractModuleSource() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestClientChanged(t *testing.T) {
	tests := []struct {
		name     string
		a        AudioClient
		b        AudioClient
		expected bool
	}{
		{
			name: "identical clients",
			a: AudioClient{
				Volume: 0.5,
				Muted:  false,
				Corked: false,
			},
			b: AudioClient{
				Volume: 0.5,
				Muted:  false,
				Corked: false,
			},
			expected: false,
		},
		{
			name: "different volume",
			a: AudioClient{
				Volume: 0.5,
				Muted:  false,
				Corked: false,
			},
			b: AudioClient{
				Volume: 0.7,
				Muted:  false,
				Corked: false,
			},
			expected: true,
		},
		{
			name: "different muted",
			a: AudioClient{
				Volume: 0.5,
				Muted:  false,
				Corked: false,
			},
			b: AudioClient{
				Volume: 0.5,
				Muted:  true,
				Corked: false,
			},
			expected: true,
		},
		{
			name: "different corked",
			a: AudioClient{
				Volume: 0.5,
				Muted:  false,
				Corked: false,
			},
			b: AudioClient{
				Volume: 0.5,
				Muted:  false,
				Corked: true,
			},
			expected: true,
		},
		{
			name: "all different",
			a: AudioClient{
				Volume: 0.5,
				Muted:  false,
				Corked: false,
			},
			b: AudioClient{
				Volume: 0.8,
				Muted:  true,
				Corked: true,
			},
			expected: true,
		},
		{
			name: "different name but same state (should be false)",
			a: AudioClient{
				Name:   "client1",
				Volume: 0.5,
				Muted:  false,
				Corked: false,
			},
			b: AudioClient{
				Name:   "client2",
				Volume: 0.5,
				Muted:  false,
				Corked: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clientChanged(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("clientChanged() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUpdateOrAddClient(t *testing.T) {
	pa := &PulseAudioBackend{}

	tests := []struct {
		name      string
		oldMap    map[string]AudioClient
		newClient AudioClient
		expected  AudioClient
	}{
		{
			name: "add new client",
			oldMap: map[string]AudioClient{
				"client1": {Name: "client1", Volume: 0.5},
			},
			newClient: AudioClient{Name: "client2", Volume: 0.7},
			expected:  AudioClient{Name: "client2", Volume: 0.7},
		},
		{
			name: "update existing client with changes",
			oldMap: map[string]AudioClient{
				"client1": {Name: "client1", Volume: 0.5, Muted: false},
			},
			newClient: AudioClient{Name: "client1", Volume: 0.8, Muted: true},
			expected:  AudioClient{Name: "client1", Volume: 0.8, Muted: true},
		},
		{
			name: "keep old client when no changes",
			oldMap: map[string]AudioClient{
				"client1": {Name: "client1", Volume: 0.5, Muted: false},
			},
			newClient: AudioClient{Name: "client1", Volume: 0.5, Muted: false},
			expected:  AudioClient{Name: "client1", Volume: 0.5, Muted: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pa.updateOrAddClient(tt.oldMap, tt.newClient)
			if result.Name != tt.expected.Name ||
				result.Volume != tt.expected.Volume ||
				result.Muted != tt.expected.Muted {
				t.Errorf("updateOrAddClient() = %+v, want %+v", result, tt.expected)
			}
		})
	}
}

func TestRemoveMissingClients(t *testing.T) {
	pa := &PulseAudioBackend{}

	tests := []struct {
		name       string
		oldMap     map[string]AudioClient
		newClients []AudioClient
		expected   []AudioClient
	}{
		{
			name: "all clients exist",
			oldMap: map[string]AudioClient{
				"client1": {Name: "client1"},
				"client2": {Name: "client2"},
			},
			newClients: []AudioClient{
				{Name: "client1"},
				{Name: "client2"},
			},
			expected: []AudioClient{
				{Name: "client1"},
				{Name: "client2"},
			},
		},
		{
			name: "remove missing client",
			oldMap: map[string]AudioClient{
				"client1": {Name: "client1"},
			},
			newClients: []AudioClient{
				{Name: "client1"},
				{Name: "client2"}, // Not in oldMap
			},
			expected: []AudioClient{
				{Name: "client1"},
			},
		},
		{
			name:   "empty old map removes all",
			oldMap: map[string]AudioClient{},
			newClients: []AudioClient{
				{Name: "client1"},
				{Name: "client2"},
			},
			expected: []AudioClient{},
		},
		{
			name: "empty new clients",
			oldMap: map[string]AudioClient{
				"client1": {Name: "client1"},
			},
			newClients: []AudioClient{},
			expected:   []AudioClient{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pa.removeMissingClients(tt.oldMap, tt.newClients)
			if len(result) != len(tt.expected) {
				t.Errorf("removeMissingClients() length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i].Name != tt.expected[i].Name {
					t.Errorf("removeMissingClients()[%d].Name = %s, want %s", i, result[i].Name, tt.expected[i].Name)
				}
			}
		})
	}
}
