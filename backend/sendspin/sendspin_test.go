package sendspin

import (
	"context"
	"testing"

	ssp "github.com/Sendspin/sendspin-go/pkg/sendspin"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
)

func TestNewNilWhenDisabled(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name string
		cfg  *config.SendspinConfig
		want bool // want nil backend
	}{
		{"nil config", nil, true},
		{"disabled", &config.SendspinConfig{Enabled: false}, true},
		{"enabled", &config.SendspinConfig{Enabled: true, PlayerName: "x"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, err := New(ctx, tt.cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if (b == nil) != tt.want {
				t.Fatalf("New nil = %v, want %v", b == nil, tt.want)
			}
		})
	}
}

func TestControlNotConnected(t *testing.T) {
	b, err := New(context.Background(), &config.SendspinConfig{Enabled: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for name, fn := range map[string]func() error{
		"Play":      b.Play,
		"Pause":     b.Pause,
		"Stop":      b.Stop,
		"SetVolume": func() error { return b.SetVolume(50) },
		"SetMuted":  func() error { return b.SetMuted(true) },
	} {
		if err := fn(); err != ErrNotConnected {
			t.Errorf("%s() = %v, want ErrNotConnected", name, err)
		}
	}
}

func TestStatusMapsStateAndMetadata(t *testing.T) {
	b, _ := New(context.Background(), &config.SendspinConfig{Enabled: true})

	b.onStateChange(ssp.PlayerState{State: "playing", Volume: 70, Connected: true, Codec: "flac"})

	st := b.Status()
	if !st.Connected || st.State != "playing" || st.Volume != 70 || st.Codec != "flac" {
		t.Fatalf("status = %+v, missing mapped state fields", st)
	}
	if st.Track != nil {
		t.Errorf("Track = %+v, want nil with no metadata", st.Track)
	}

	b.onMetadata(ssp.Metadata{Title: "Song", Artist: "Artist", Duration: 200})
	st = b.Status()
	if st.Track == nil || st.Track.Title != "Song" || st.Track.Artist != "Artist" || st.Track.Duration != 200 {
		t.Fatalf("Track = %+v, want populated", st.Track)
	}
}

func TestCallbacksEmitEvents(t *testing.T) {
	b, _ := New(context.Background(), &config.SendspinConfig{Enabled: true})

	b.onStateChange(ssp.PlayerState{State: "paused"})
	b.onMetadata(ssp.Metadata{Title: "T"})

	got := map[string]bool{}
	for range 2 {
		select {
		case e := <-b.Events():
			got[e.Type] = true
		default:
			t.Fatal("expected an event, channel empty")
		}
	}
	if !got[events.TypeSendspinUpdated] || !got[events.TypeSendspinMetadata] {
		t.Errorf("events seen = %v, want both updated and metadata", got)
	}
}
