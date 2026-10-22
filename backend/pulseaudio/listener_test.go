package pulseaudio

import (
	"reflect"
	"testing"

	"github.com/jfreymuth/pulse/proto"

	"github.com/b0bbywan/go-odio-api/events"
)

func TestModuleEvent_NotifiesProcessingChanges(t *testing.T) {
	srv := newFakeServer(hwSink)
	pa := &PulseAudioBackend{events: make(chan events.Event, 2)}
	pa.processing = newTestProcessing(t, srv, false, true)
	reconcile(t, pa.processing, ServerPulse)
	srv.mods = nil // unloaded by someone else
	l := &Listener{backend: pa}
	ev := proto.SubscribeEvent{Event: proto.EventModule | proto.EventRemove, Index: 101}

	l.handle(ev)
	l.handle(ev)

	want := events.Event{
		Type: events.TypeAudioProcessing,
		Data: []ProcessingFeature{{Name: featureNetworkSink, State: stateDisabled}},
	}
	select {
	case got := <-pa.events:
		if !reflect.DeepEqual(got, want) {
			t.Errorf("event = %+v, want %+v", got, want)
		}
	default:
		t.Fatal("no event")
	}
	select {
	case e := <-pa.events:
		t.Errorf("unexpected second event %+v", e)
	default:
	}
}
