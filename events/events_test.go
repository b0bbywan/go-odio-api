package events

import "testing"

func TestFilterTypes_Nil(t *testing.T) {
	if FilterTypes(nil) != nil {
		t.Error("FilterTypes(nil) should return nil")
	}
	if FilterTypes([]string{}) != nil {
		t.Error("FilterTypes([]) should return nil")
	}
}

func TestFilterTypes_Match(t *testing.T) {
	f := FilterTypes([]string{TypePlayerUpdated, TypePlayerAdded})
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if !f(Event{Type: TypePlayerUpdated}) {
		t.Errorf("filter should pass %s", TypePlayerUpdated)
	}
	if !f(Event{Type: TypePlayerAdded}) {
		t.Errorf("filter should pass %s", TypePlayerAdded)
	}
	if f(Event{Type: TypePlayerRemoved}) {
		t.Errorf("filter should block %s", TypePlayerRemoved)
	}
	if f(Event{Type: TypeAudioUpdated}) {
		t.Errorf("filter should block %s", TypeAudioUpdated)
	}
}

func TestFilterBackend_Unknown(t *testing.T) {
	if FilterBackend([]string{"unknown"}) != nil {
		t.Error("FilterBackend with unknown names should return nil (pass-all)")
	}
	if FilterBackend(nil) != nil {
		t.Error("FilterBackend(nil) should return nil")
	}
}

func TestFilterBackend_MPRIS(t *testing.T) {
	f := FilterBackend([]string{"mpris"})
	if f == nil {
		t.Fatal("expected non-nil filter for mpris")
	}
	for _, typ := range BackendTypes["mpris"] {
		if !f(Event{Type: typ}) {
			t.Errorf("mpris filter should pass %s", typ)
		}
	}
	if f(Event{Type: TypeAudioUpdated}) {
		t.Error("mpris filter should block audio.updated")
	}
	if f(Event{Type: TypeServiceUpdated}) {
		t.Error("mpris filter should block service.updated")
	}
}

func TestFilterBackend_Multi(t *testing.T) {
	f := FilterBackend([]string{"mpris", "audio"})
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	for _, typ := range BackendTypes["mpris"] {
		if !f(Event{Type: typ}) {
			t.Errorf("filter should pass mpris event %s", typ)
		}
	}
	if !f(Event{Type: TypeAudioUpdated}) {
		t.Error("filter should pass audio.updated when audio backend is included")
	}
	if f(Event{Type: TypeServiceUpdated}) {
		t.Error("filter should block service.updated")
	}
}

func TestBackendTypes_Completeness(t *testing.T) {
	all := []string{
		TypePlayerUpdated, TypePlayerAdded, TypePlayerRemoved,
		TypeAudioUpdated, TypeServiceUpdated,
	}
	covered := make(map[string]bool)
	for _, types := range BackendTypes {
		for _, t := range types {
			covered[t] = true
		}
	}
	for _, typ := range all {
		if !covered[typ] {
			t.Errorf("event type %q is not covered by any backend in BackendTypes", typ)
		}
	}
}
