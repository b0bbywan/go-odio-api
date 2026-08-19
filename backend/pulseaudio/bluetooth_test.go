package pulseaudio

import (
	"reflect"
	"testing"

	"github.com/jfreymuth/pulse/proto"
)

func TestParseBluetoothCodecs(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		expected []string
		wantErr  bool
	}{
		{
			name:     "server reply",
			out:      `[{"name":"sbc","description":"SBC"},{"name":"aptx_hd","description":"aptX HD"}]`,
			expected: []string{"sbc", "aptx_hd"},
		},
		{
			name:     "empty list",
			out:      "[]",
			expected: []string{},
		},
		{
			name:    "garbage",
			out:     "not json",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBluetoothCodecs(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseBluetoothCodecs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseBluetoothCodecs() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPickBluetoothCodec(t *testing.T) {
	tests := []struct {
		name      string
		available []string
		current   string
		expected  string
	}{
		{"best available", []string{"sbc", "aptx", "aptx_hd"}, "sbc", "aptx_hd"},
		{"fallback to aptx", []string{"sbc", "aptx"}, "sbc", "aptx"},
		{"nothing better than sbc", []string{"sbc"}, "sbc", ""},
		{"already on aptx, hd missing", []string{"sbc", "aptx"}, "aptx", ""},
		{"already on aptx, hd offered", []string{"sbc", "aptx", "aptx_hd"}, "aptx", "aptx_hd"},
		{"already on best", []string{"aptx_hd", "aptx"}, "aptx_hd", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickBluetoothCodec(tt.available, tt.current); got != tt.expected {
				t.Errorf("pickBluetoothCodec(%v, %q) = %q, want %q", tt.available, tt.current, got, tt.expected)
			}
		})
	}
}

func TestBluetoothAddress(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"bluez_source.C8_2A_DD_A7_D5_0D.a2dp_source", "C8_2A_DD_A7_D5_0D"},
		{"bluez_source.C8_2A_DD_A7_D5_0D", "C8_2A_DD_A7_D5_0D"},
	}
	for _, tt := range tests {
		if got := bluetoothAddress(tt.name); got != tt.expected {
			t.Errorf("bluetoothAddress(%q) = %q, want %q", tt.name, got, tt.expected)
		}
	}
}

func TestEnsureBluetoothCodecSkipsBest(t *testing.T) {
	pa := &PulseAudioBackend{}
	src := &proto.GetSourceInfoReply{
		SourceName: "bluez_source.C8_2A_DD_A7_D5_0D.a2dp_source",
		Properties: proto.PropList{"bluetooth.codec": proto.PropListString(preferredBluetoothCodecs[0])},
	}

	pa.ensureBluetoothCodec(src)

	if _, attempted := pa.btCodecAttempted.Load("C8_2A_DD_A7_D5_0D"); attempted {
		t.Errorf("device already on %s must not be attempted", preferredBluetoothCodecs[0])
	}
}
