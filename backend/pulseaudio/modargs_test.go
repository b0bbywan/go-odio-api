package pulseaudio

import (
	"reflect"
	"testing"
)

func TestParseModuleArgs(t *testing.T) {
	tests := []struct {
		name string
		args string
		want map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"only whitespace", " \t ", map[string]string{}},
		{"bare value", "restore_device=false", map[string]string{"restore_device": "false"}},
		{
			// PulseAudio 17.0, module-native-protocol-tcp loaded by odios pulse-tcp.sh
			"acl with semicolons",
			"auth-ip-acl=127.0.0.1;192.168.1.0/24",
			map[string]string{"auth-ip-acl": "127.0.0.1;192.168.1.0/24"},
		},
		{
			// PulseAudio 17.0, module-alsa-card loaded by module-udev-detect
			"double quoted values",
			`device_id="0" name="platform-3f00b840.mailbox" card_name="alsa_card.platform-3f00b840.mailbox" namereg_fail=false tsched=no fixed_latency_range=no ignore_dB=no deferred_volume=yes use_ucm=yes avoid_resampling=no card_properties="module-udev-detect.discovered=1"`,
			map[string]string{
				"device_id":           "0",
				"name":                "platform-3f00b840.mailbox",
				"card_name":           "alsa_card.platform-3f00b840.mailbox",
				"namereg_fail":        "false",
				"tsched":              "no",
				"fixed_latency_range": "no",
				"ignore_dB":           "no",
				"deferred_volume":     "yes",
				"use_ucm":             "yes",
				"avoid_resampling":    "no",
				"card_properties":     "module-udev-detect.discovered=1",
			},
		},
		{
			// args shape of the pulse-mono.sh workaround (odios discussion #86)
			"remap sink workaround",
			"master=alsa_output.platform-3f00b840.mailbox.stereo-fallback sink_name=mono sink_properties=device.description=Mono channels=1 channel_map=mono",
			map[string]string{
				"master":          "alsa_output.platform-3f00b840.mailbox.stereo-fallback",
				"sink_name":       "mono",
				"sink_properties": "device.description=Mono",
				"channels":        "1",
				"channel_map":     "mono",
			},
		},
		{
			// pipewire-pulse 1.4.2, loaded with
			// pactl load-module module-remap-sink sink_name=t master=@DEFAULT_SINK@ 'sink_properties="device.description=\"Mono test\""'
			"escaped quotes inside double quotes",
			`sink_name=t master=@DEFAULT_SINK@ sink_properties="device.description=\"Mono test\""`,
			map[string]string{
				"sink_name":       "t",
				"master":          "@DEFAULT_SINK@",
				"sink_properties": `device.description="Mono test"`,
			},
		},
		{
			"single quoted with spaces",
			`sink_properties='device.description="Mono test"'`,
			map[string]string{"sink_properties": `device.description="Mono test"`},
		},
		{
			"escapes",
			`a="say \"hi\"" b='it\'s' c=one\ two d=back\\slash`,
			map[string]string{"a": `say "hi"`, "b": "it's", "c": "one two", "d": `back\slash`},
		},
		{"empty values", `a= b="" c=''`, map[string]string{"a": "", "b": "", "c": ""}},
		{"value at end without space", "a=", map[string]string{"a": ""}},
		{"pair right after a quote", `a="x"b=y`, map[string]string{"a": "x", "b": "y"}},
		{"tabs and newlines", "a=1\tb=2\nc=3", map[string]string{"a": "1", "b": "2", "c": "3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseModuleArgs(tt.args)
			if err != nil {
				t.Fatalf("parseModuleArgs(%q) error: %v", tt.args, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseModuleArgs(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseModuleArgs_Invalid(t *testing.T) {
	invalid := []struct {
		name string
		args string
	}{
		// What PulseAudio received from `pactl load-module ... sink_properties='device.description="Mono test"'`,
		// rejected with "Module initialization failed".
		{"quote inside bare value", `sink_name=t sink_properties=device.description="Mono test"`},
		// pipewire-pulse 1.4.2 also lists native PipeWire modules, with SPA-JSON arguments.
		{"pipewire native module", "{\n        nice.level    = -11\n        rt.prio       = 88\n        #rt.time.soft = -1\n    }"},
		{"key without value", "sink_name"},
		{"key with space", "sink name=t"},
		{"value without key", "=t"},
		{"unterminated double quote", `a="open`},
		{"unterminated single quote", `a='open`},
		{"trailing backslash", `a=x\`},
		{"duplicate key", "a=1 a=2"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := parseModuleArgs(tt.args); err == nil {
				t.Errorf("parseModuleArgs(%q) = %v, want an error", tt.args, got)
			}
		})
	}
}
