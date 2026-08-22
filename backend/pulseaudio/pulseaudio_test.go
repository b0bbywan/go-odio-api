package pulseaudio

import (
	"errors"
	"os"
	"testing"

	"github.com/jfreymuth/pulse/proto"
)

func TestDetectServerKind(t *testing.T) {
	tests := []struct {
		name     string
		server   *proto.GetServerInfoReply
		expected AudioServerKind
	}{
		{
			name: "PulseAudio server",
			server: &proto.GetServerInfoReply{
				PackageName: "pulseaudio",
			},
			expected: ServerPulse,
		},
		{
			name: "PipeWire server (lowercase)",
			server: &proto.GetServerInfoReply{
				PackageName: "pipewire-pulse",
			},
			expected: ServerPipeWire,
		},
		{
			name: "PipeWire server (uppercase)",
			server: &proto.GetServerInfoReply{
				PackageName: "PipeWire",
			},
			expected: ServerPipeWire,
		},
		{
			name: "PipeWire server (mixed case)",
			server: &proto.GetServerInfoReply{
				PackageName: "PiPeWiRe",
			},
			expected: ServerPipeWire,
		},
		{
			name: "Unknown server defaults to PulseAudio",
			server: &proto.GetServerInfoReply{
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
		input    proto.PropList
		expected map[string]string
	}{
		{
			name:     "nil map",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    proto.PropList{},
			expected: map[string]string{},
		},
		{
			name: "map with values",
			input: proto.PropList{
				"key1": proto.PropListString("value1"),
				"key2": proto.PropListString("value2"),
				"key3": proto.PropListString("value3"),
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
			if len(tt.input) > 0 {
				tt.input["new_key"] = proto.PropListString("new_value")
				if _, exists := result["new_key"]; exists {
					t.Error("cloneProps() did not create a deep copy")
				}
			}
		})
	}
}

func TestSinkInputMatchesName(t *testing.T) {
	tests := []struct {
		name   string
		props  map[string]string
		target string
		want   bool
	}{
		{
			name: "regular stream name",
			props: map[string]string{
				"media.name": "Playback",
			},
			target: "Playback",
			want:   true,
		},
		{
			name: "bluetooth display name",
			props: map[string]string{
				"media.name":      "Loopback from Cloud Remaster",
				"media.icon_name": "audio-card-bluetooth",
			},
			target: "Cloud Remaster",
			want:   true,
		},
		{
			name: "non-bluetooth prefix is not stripped",
			props: map[string]string{
				"media.name": "Loopback from Cloud Remaster",
			},
			target: "Cloud Remaster",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sinkInputMatchesName(tt.props, tt.target); got != tt.want {
				t.Fatalf("sinkInputMatchesName() = %v, want %v", got, tt.want)
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

func TestSinkStateString(t *testing.T) {
	tests := []struct {
		state    uint32
		expected string
	}{
		{0, "running"},
		{1, "idle"},
		{2, "suspended"},
		{99, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := sinkStateString(tt.state); got != tt.expected {
				t.Errorf("sinkStateString(%d) = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

func TestOutputChanged(t *testing.T) {
	tests := []struct {
		name     string
		a        AudioOutput
		b        AudioOutput
		expected bool
	}{
		{
			name:     "identical",
			a:        AudioOutput{Volume: 0.5, Muted: false, State: "running", Default: false},
			b:        AudioOutput{Volume: 0.5, Muted: false, State: "running", Default: false},
			expected: false,
		},
		{
			name:     "volume changed",
			a:        AudioOutput{Volume: 0.5},
			b:        AudioOutput{Volume: 0.8},
			expected: true,
		},
		{
			name:     "muted changed",
			a:        AudioOutput{Muted: false},
			b:        AudioOutput{Muted: true},
			expected: true,
		},
		{
			name:     "state changed",
			a:        AudioOutput{State: "running"},
			b:        AudioOutput{State: "suspended"},
			expected: true,
		},
		{
			name:     "default changed",
			a:        AudioOutput{Default: false},
			b:        AudioOutput{Default: true},
			expected: true,
		},
		{
			name:     "name differs but state identical",
			a:        AudioOutput{Name: "sink1", Volume: 0.5, State: "idle"},
			b:        AudioOutput{Name: "sink2", Volume: 0.5, State: "idle"},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outputChanged(tt.a, tt.b); got != tt.expected {
				t.Errorf("outputChanged() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDiffOutputs(t *testing.T) {
	tests := []struct {
		name            string
		old             []AudioOutput
		new             []AudioOutput
		wantChangedLen  int
		wantRemovedLen  int
		wantChangedName string
		wantRemovedName string
	}{
		{
			name:           "no changes",
			old:            []AudioOutput{{Name: "sink1", Volume: 0.5, State: "running"}},
			new:            []AudioOutput{{Name: "sink1", Volume: 0.5, State: "running"}},
			wantChangedLen: 0,
			wantRemovedLen: 0,
		},
		{
			name:            "new output added",
			old:             []AudioOutput{{Name: "sink1", Volume: 0.5, State: "running"}},
			new:             []AudioOutput{{Name: "sink1", Volume: 0.5, State: "running"}, {Name: "sink2", State: "idle"}},
			wantChangedLen:  1,
			wantChangedName: "sink2",
			wantRemovedLen:  0,
		},
		{
			name:            "output removed",
			old:             []AudioOutput{{Name: "sink1"}, {Name: "sink2"}},
			new:             []AudioOutput{{Name: "sink1"}},
			wantChangedLen:  0,
			wantRemovedLen:  1,
			wantRemovedName: "sink2",
		},
		{
			name:            "volume changed",
			old:             []AudioOutput{{Name: "sink1", Volume: 0.5, State: "running"}},
			new:             []AudioOutput{{Name: "sink1", Volume: 0.8, State: "running"}},
			wantChangedLen:  1,
			wantChangedName: "sink1",
			wantRemovedLen:  0,
		},
		{
			name:           "default changed",
			old:            []AudioOutput{{Name: "sink1", Default: false}, {Name: "sink2", Default: true}},
			new:            []AudioOutput{{Name: "sink1", Default: true}, {Name: "sink2", Default: false}},
			wantChangedLen: 2,
			wantRemovedLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, removed := diffOutputs(tt.old, tt.new)
			if len(changed) != tt.wantChangedLen {
				t.Errorf("diffOutputs() changed len = %d, want %d", len(changed), tt.wantChangedLen)
			}
			if len(removed) != tt.wantRemovedLen {
				t.Errorf("diffOutputs() removed len = %d, want %d", len(removed), tt.wantRemovedLen)
			}
			if tt.wantChangedName != "" && len(changed) > 0 && changed[0].Name != tt.wantChangedName {
				t.Errorf("diffOutputs() changed[0].Name = %q, want %q", changed[0].Name, tt.wantChangedName)
			}
			if tt.wantRemovedName != "" && len(removed) > 0 && removed[0].Name != tt.wantRemovedName {
				t.Errorf("diffOutputs() removed[0].Name = %q, want %q", removed[0].Name, tt.wantRemovedName)
			}
		})
	}
}

func TestServerInfoFromCache(t *testing.T) {
	t.Run("cache miss returns error", func(t *testing.T) {
		pa := &PulseAudioBackend{kind: ServerPipeWire}
		_, err := pa.ServerInfo()
		if err == nil {
			t.Error("expected error on cache miss, got nil")
		}
	})

	t.Run("no default sink returns error", func(t *testing.T) {
		pa := &PulseAudioBackend{kind: ServerPipeWire}
		pa.outputCache.Store([]AudioOutput{
			{Name: "sink1", Default: false},
			{Name: "sink2", Default: false},
		})
		_, err := pa.ServerInfo()
		if err == nil {
			t.Error("expected error when no default sink, got nil")
		}
	})

	t.Run("reconstructs from default output", func(t *testing.T) {
		pa := &PulseAudioBackend{kind: ServerPipeWire}
		pa.outputCache.Store([]AudioOutput{
			{Name: "sink1", Default: false, Volume: 0.3, Muted: false},
			{Name: "sink2", Default: true, Volume: 0.7, Muted: true},
		})
		info, err := pa.ServerInfo()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Kind != ServerPipeWire {
			t.Errorf("Kind = %v, want %v", info.Kind, ServerPipeWire)
		}
		if info.DefaultSink != "sink2" {
			t.Errorf("DefaultSink = %q, want sink2", info.DefaultSink)
		}
		if info.Volume != 0.7 {
			t.Errorf("Volume = %v, want 0.7", info.Volume)
		}
		if !info.Muted {
			t.Error("Muted = false, want true")
		}
	})
}

func TestCookie(t *testing.T) {
	t.Run("disabled returns DisabledError", func(t *testing.T) {
		pa := &PulseAudioBackend{serveCookie: false}
		_, err := pa.Cookie()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var disabled *DisabledError
		if !errors.As(err, &disabled) {
			t.Errorf("expected DisabledError, got %T: %v", err, err)
		}
	})

	t.Run("enabled but file missing returns error", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		pa := &PulseAudioBackend{serveCookie: true}
		_, err := pa.Cookie()
		if err == nil {
			t.Fatal("expected error for missing cookie file, got nil")
		}
		var disabled *DisabledError
		if errors.As(err, &disabled) {
			t.Errorf("expected file error, got DisabledError")
		}
	})

	t.Run("enabled and file exists returns content", func(t *testing.T) {
		configDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configDir)

		cookieDir := configDir + "/pulse"
		if err := os.MkdirAll(cookieDir, 0700); err != nil {
			t.Fatal(err)
		}
		want := []byte("fakecookiedata")
		if err := os.WriteFile(cookieDir+"/cookie", want, 0600); err != nil {
			t.Fatal(err)
		}

		pa := &PulseAudioBackend{serveCookie: true}
		got, err := pa.Cookie()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != string(want) {
			t.Errorf("Cookie() = %q, want %q", got, want)
		}
	})
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

// TestClientName verifies the naming fallback chain for streams that register
// empty names (issue #130: spotifyd).
func TestClientName(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]string
		want  string
	}{
		{
			name: "media.name wins",
			props: map[string]string{
				"media.name":                 "Playback",
				"application.name":           "Spotify",
				"application.process.binary": "spotify",
			},
			want: "Playback",
		},
		{
			name: "application.name when media.name empty",
			props: map[string]string{
				"media.name":                 "",
				"application.name":           "Spotify",
				"application.process.binary": "spotify",
			},
			want: "Spotify",
		},
		{
			name: "binary when both names empty",
			props: map[string]string{
				"media.name":                 "",
				"application.name":           "",
				"application.process.binary": "spotifyd",
			},
			want: "spotifyd",
		},
		{
			name:  "everything empty",
			props: map[string]string{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientName(tt.props); got != tt.want {
				t.Errorf("clientName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVolumeOf(t *testing.T) {
	tests := []struct {
		name     string
		cv       proto.ChannelVolumes
		expected float32
	}{
		{name: "empty", cv: nil, expected: 0},
		{name: "normal", cv: proto.ChannelVolumes{proto.VolumeNorm, proto.VolumeNorm}, expected: 1},
		{name: "half rounded", cv: proto.ChannelVolumes{proto.VolumeNorm / 2}, expected: 0.5},
		{name: "first channel wins", cv: proto.ChannelVolumes{proto.VolumeNorm / 4, proto.VolumeNorm}, expected: 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := volumeOf(tt.cv); got != tt.expected {
				t.Errorf("volumeOf() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestChannelVolumes(t *testing.T) {
	cv := channelVolumes(2, 0.5)
	if len(cv) != 2 {
		t.Fatalf("channelVolumes() len = %d, want 2", len(cv))
	}
	for i, v := range cv {
		if v != proto.VolumeNorm/2 {
			t.Errorf("channelVolumes()[%d] = %d, want %d", i, v, proto.VolumeNorm/2)
		}
	}
	if got := channelVolumes(0, 1); len(got) != 1 {
		t.Errorf("channelVolumes(0) len = %d, want 1", len(got))
	}
}

func TestParsePulseSink(t *testing.T) {
	pa := &PulseAudioBackend{kind: ServerPulse}
	sink := &proto.GetSinkInfoReply{
		SinkIndex:      3,
		SinkName:       "tunnel.remote",
		Device:         "Remote tunnel",
		Mute:           true,
		ChannelVolumes: proto.ChannelVolumes{proto.VolumeNorm / 2, proto.VolumeNorm / 2},
		State:          1,
		Driver:         "module-tunnel-sink.c",
		Flags:          0x20000,
		ActivePortName: "",
		Properties:     proto.PropList{"device.description": proto.PropListString("Remote")},
	}

	out := pa.parseSink(sink, "tunnel.remote")

	if out.Index != 3 || out.Name != "tunnel.remote" || out.Description != "Remote tunnel" {
		t.Errorf("identity fields = %+v", out)
	}
	if !out.Muted || out.Volume != 0.5 || out.State != "idle" || !out.Default {
		t.Errorf("state fields = %+v", out)
	}
	if !out.IsNetwork || out.Nick != "Remote" || out.Driver != "module-tunnel-sink.c" {
		t.Errorf("pulse-specific fields = %+v", out)
	}
}

func TestParsePipeWireSink(t *testing.T) {
	pa := &PulseAudioBackend{kind: ServerPipeWire}
	sink := &proto.GetSinkInfoReply{
		SinkIndex:      7,
		SinkName:       "raop_sink.kitchen",
		ChannelVolumes: proto.ChannelVolumes{proto.VolumeNorm},
		State:          2,
		Properties: proto.PropList{
			"node.nick":    proto.PropListString("Kitchen"),
			"node.network": proto.PropListString("true"),
		},
	}

	out := pa.parseSink(sink, "other")

	if out.Default || out.Volume != 1 || out.State != "suspended" {
		t.Errorf("state fields = %+v", out)
	}
	if !out.IsNetwork || out.Nick != "Kitchen" {
		t.Errorf("pipewire-specific fields = %+v", out)
	}
}

func TestParseSinkInput(t *testing.T) {
	input := &proto.GetSinkInputInfoReply{
		SinkInputIndex: 42,
		MediaName:      "Playback",
		ChannelVolumes: proto.ChannelVolumes{proto.VolumeNorm / 4, proto.VolumeNorm / 4},
		Muted:          true,
		Corked:         true,
		Properties: proto.PropList{
			"media.name":                 proto.PropListString("Playback"),
			"application.name":           proto.PropListString("Chrome"),
			"application.process.binary": proto.PropListString("chrome"),
		},
	}

	for _, kind := range []AudioServerKind{ServerPulse, ServerPipeWire} {
		t.Run(string(kind), func(t *testing.T) {
			pa := &PulseAudioBackend{kind: kind}
			c := pa.parseSinkInput(input)
			if c.ID != 42 || c.Name != "Playback" || c.App != "Chrome" || c.Binary != "chrome" {
				t.Errorf("identity fields = %+v", c)
			}
			if !c.Muted || c.Volume != 0.25 || c.Backend != kind {
				t.Errorf("state fields = %+v", c)
			}
		})
	}
}

func TestRemoveClient(t *testing.T) {
	pa := &PulseAudioBackend{}
	pa.cache.Store([]AudioClient{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}})

	removed, ok := pa.removeClient(2)
	if !ok || removed.Name != "b" {
		t.Fatalf("removeClient(2) = %+v, %v; want b, true", removed, ok)
	}
	if got := pa.cache.Load(); len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Errorf("cache after remove = %+v, want ids 1,3", got)
	}

	if _, ok := pa.removeClient(42); ok {
		t.Error("removeClient(42) = true, want false for unknown index")
	}
	if got := pa.cache.Load(); len(got) != 2 {
		t.Errorf("cache modified by unknown remove: %+v", got)
	}
}

func TestRemoveOutput(t *testing.T) {
	pa := &PulseAudioBackend{}
	pa.outputCache.Store([]AudioOutput{{Index: 10, Name: "x"}, {Index: 11, Name: "y"}})

	removed, ok := pa.removeOutput(10)
	if !ok || removed.Name != "x" {
		t.Fatalf("removeOutput(10) = %+v, %v; want x, true", removed, ok)
	}
	if got := pa.outputCache.Load(); len(got) != 1 || got[0].Index != 11 {
		t.Errorf("cache after remove = %+v, want index 11", got)
	}

	if _, ok := pa.removeOutput(99); ok {
		t.Error("removeOutput(99) = true, want false for unknown index")
	}
}
