package backend

import (
	"context"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/backend/login1"
	"github.com/b0bbywan/go-odio-api/events"
)

func TestBroadcaster_Subscribe_ReceivesAll(t *testing.T) {
	upstream := make(chan events.Event, 4)
	b := NewBroadcaster(context.Background(), upstream)

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	upstream <- events.Event{Type: events.TypePlayerUpdated}
	upstream <- events.Event{Type: events.TypeAudioUpdated}

	for _, want := range []string{events.TypePlayerUpdated, events.TypeAudioUpdated} {
		select {
		case got := <-ch:
			if got.Type != want {
				t.Errorf("got %s, want %s", got.Type, want)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timed out waiting for event %s", want)
		}
	}
}

func TestBroadcaster_SubscribeFunc_FiltersEvents(t *testing.T) {
	upstream := make(chan events.Event, 4)
	b := NewBroadcaster(context.Background(), upstream)

	filter := func(e events.Event) bool { return e.Type == events.TypePlayerUpdated }
	ch := b.SubscribeFunc(filter)
	defer b.Unsubscribe(ch)

	// Send one matching and one non-matching event.
	upstream <- events.Event{Type: events.TypePlayerUpdated}
	upstream <- events.Event{Type: events.TypeAudioUpdated}

	// Only the player event should arrive.
	select {
	case got := <-ch:
		if got.Type != events.TypePlayerUpdated {
			t.Errorf("got %s, want %s", got.Type, events.TypePlayerUpdated)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for player.updated event")
	}

	// Audio event must not be in the channel.
	select {
	case got := <-ch:
		t.Errorf("unexpected event %s delivered through filter", got.Type)
	case <-time.After(30 * time.Millisecond):
		// expected: nothing received
	}
}

func TestBroadcaster_SubscribeFunc_NilFilterPassesAll(t *testing.T) {
	upstream := make(chan events.Event, 4)
	b := NewBroadcaster(context.Background(), upstream)

	ch := b.SubscribeFunc(nil)
	defer b.Unsubscribe(ch)

	upstream <- events.Event{Type: events.TypeServiceUpdated}

	select {
	case got := <-ch:
		if got.Type != events.TypeServiceUpdated {
			t.Errorf("got %s, want %s", got.Type, events.TypeServiceUpdated)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for service.updated event")
	}
}

func TestBroadcaster_PowerActionEventFlowsThrough(t *testing.T) {
	upstream := make(chan events.Event, 4)
	b := NewBroadcaster(context.Background(), upstream)

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	upstream <- events.Event{Type: events.TypePowerAction, Data: login1.PowerActionData{Action: "reboot"}}

	select {
	case got := <-ch:
		if got.Type != events.TypePowerAction {
			t.Errorf("got %s, want %s", got.Type, events.TypePowerAction)
		}
		data, ok := got.Data.(login1.PowerActionData)
		if !ok {
			t.Fatalf("data is %T, want PowerActionData", got.Data)
		}
		if data.Action != "reboot" {
			t.Errorf("data.Action = %q, want %q", data.Action, "reboot")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for power.action event")
	}
}

func TestNewBroadcasterFromBackend_Login1Nil_NoPanic(t *testing.T) {
	b := &Backend{Login1: nil}
	broadcaster := newBroadcasterFromBackend(context.Background(), b)
	ch := broadcaster.Subscribe()
	defer broadcaster.Unsubscribe(ch)
	// No events expected, just verify no panic and channel is usable.
	select {
	case got := <-ch:
		t.Errorf("unexpected event %s from empty backend", got.Type)
	case <-time.After(20 * time.Millisecond):
		// expected
	}
}

func TestBroadcaster_MultipleSubscribersIndependentFilters(t *testing.T) {
	upstream := make(chan events.Event, 8)
	b := NewBroadcaster(context.Background(), upstream)

	allCh := b.Subscribe()
	defer b.Unsubscribe(allCh)

	audioOnly := b.SubscribeFunc(func(e events.Event) bool { return e.Type == events.TypeAudioUpdated })
	defer b.Unsubscribe(audioOnly)

	upstream <- events.Event{Type: events.TypeAudioUpdated}
	upstream <- events.Event{Type: events.TypePlayerUpdated}

	// allCh should receive both events.
	for _, want := range []string{events.TypeAudioUpdated, events.TypePlayerUpdated} {
		select {
		case got := <-allCh:
			if got.Type != want {
				t.Errorf("allCh: got %s, want %s", got.Type, want)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("allCh: timed out waiting for %s", want)
		}
	}

	// audioOnly should receive only audio.updated.
	select {
	case got := <-audioOnly:
		if got.Type != events.TypeAudioUpdated {
			t.Errorf("audioOnly: got %s, want audio.updated", got.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("audioOnly: timed out waiting for audio.updated")
	}

	select {
	case got := <-audioOnly:
		t.Errorf("audioOnly: unexpected event %s", got.Type)
	case <-time.After(30 * time.Millisecond):
		// expected: nothing
	}
}
