package input

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	evdev "github.com/holoplot/go-evdev"

	"github.com/b0bbywan/go-odio-api/backend/action"
	"github.com/b0bbywan/go-odio-api/config"
)

// fakeActions resolves the names resolves accepts, all of them when nil, into
// actions that record their run.
type fakeActions struct {
	resolves func(name, unit string) bool
	repeat   map[string]bool
	runs     []string
}

func (f *fakeActions) Resolve(name, unit string) (action.Action, error) {
	if f.resolves != nil && !f.resolves(name, unit) {
		return action.Action{}, errors.New(name + ": unresolved")
	}
	run := strings.TrimSpace(name + " " + unit)
	return action.Action{Repeat: f.repeat[name], Run: func() error {
		f.runs = append(f.runs, run)
		return nil
	}}, nil
}

// published strips the resolved actions, which cannot be compared.
func published(keymap map[evdev.EvCode]binding) map[evdev.EvCode]Binding {
	bindings := make(map[evdev.EvCode]Binding, len(keymap))
	for code, bd := range keymap {
		bindings[code] = bd.Binding
	}
	return bindings
}

func TestBuildKeymapDefaults(t *testing.T) {
	b := &InputBackend{actions: &fakeActions{}}

	keymap := published(b.buildKeymap(nil))

	if len(keymap) != len(defaultBindings) {
		t.Errorf("keymap has %d bindings, want %d", len(keymap), len(defaultBindings))
	}
	want := Binding{Key: "KEY_PLAYPAUSE", Action: "player.playpause"}
	if got := keymap[evdev.KEY_PLAYPAUSE]; got != want {
		t.Errorf("KEY_PLAYPAUSE = %+v, want %+v", got, want)
	}
	if _, bound := keymap[evdev.KEY_POWER]; bound {
		t.Error("KEY_POWER must not be bound by default")
	}
}

// A default whose action does not resolve, its backend being disabled, is left out.
func TestBuildKeymapSkipsUnresolvedDefaults(t *testing.T) {
	b := &InputBackend{actions: &fakeActions{resolves: func(name, _ string) bool {
		return strings.HasPrefix(name, "volume.")
	}}}

	keymap := published(b.buildKeymap(nil))

	want := map[evdev.EvCode]Binding{
		evdev.KEY_VOLUMEUP:   {Key: "KEY_VOLUMEUP", Action: "volume.up"},
		evdev.KEY_VOLUMEDOWN: {Key: "KEY_VOLUMEDOWN", Action: "volume.down"},
		evdev.KEY_MUTE:       {Key: "KEY_MUTE", Action: "volume.mute"},
	}
	if !reflect.DeepEqual(keymap, want) {
		t.Errorf("keymap = %+v, want %+v", keymap, want)
	}
}

func TestBuildKeymapUserBindings(t *testing.T) {
	b := &InputBackend{actions: &fakeActions{resolves: func(name, unit string) bool {
		switch {
		case strings.HasPrefix(name, "player."):
			return name != "player.bogus"
		case strings.HasPrefix(name, "service."):
			return unit != ""
		}
		return false
	}}}

	keymap := published(b.buildKeymap([]config.InputBinding{
		{Key: "KEY_RED", Action: "service.restart", Unit: "kodi.service"},
		{Key: "key_playpause", Action: "player.stop"},
		{Key: "KEY_NEXTSONG", Action: "none"},
		{Key: "KEY_BOGUS", Action: "player.next"},
		{Key: "KEY_PREVIOUSSONG", Action: "player.bogus"},
		{Key: "KEY_POWER", Action: "power.poweroff"},
		{Key: "KEY_GREEN", Action: "service.start"},
	}))

	want := map[evdev.EvCode]Binding{
		evdev.KEY_RED:       {Key: "KEY_RED", Action: "service.restart", Unit: "kodi.service"},
		evdev.KEY_PLAYPAUSE: {Key: "KEY_PLAYPAUSE", Action: "player.stop"},
		// An invalid override leaves the default in place.
		evdev.KEY_PREVIOUSSONG: {Key: "KEY_PREVIOUSSONG", Action: "player.previous"},
		evdev.KEY_PLAY:         {Key: "KEY_PLAY", Action: "player.play"},
		evdev.KEY_PAUSE:        {Key: "KEY_PAUSE", Action: "player.pause"},
		evdev.KEY_STOPCD:       {Key: "KEY_STOPCD", Action: "player.stop"},
		evdev.KEY_FASTFORWARD:  {Key: "KEY_FASTFORWARD", Action: "player.forward"},
		evdev.KEY_REWIND:       {Key: "KEY_REWIND", Action: "player.rewind"},
	}
	if !reflect.DeepEqual(keymap, want) {
		t.Errorf("keymap = %+v, want %+v", keymap, want)
	}
}

func TestBuildKeymapKeepsResolvedAction(t *testing.T) {
	actions := &fakeActions{repeat: map[string]bool{"volume.up": true}}
	b := &InputBackend{actions: actions}

	keymap := b.buildKeymap([]config.InputBinding{
		{Key: "KEY_RED", Action: "service.restart", Unit: "kodi.service"},
	})

	if !keymap[evdev.KEY_VOLUMEUP].repeat || keymap[evdev.KEY_RED].repeat {
		t.Errorf("repeat = %t for KEY_VOLUMEUP and %t for KEY_RED, want true and false",
			keymap[evdev.KEY_VOLUMEUP].repeat, keymap[evdev.KEY_RED].repeat)
	}
	if err := keymap[evdev.KEY_RED].run(); err != nil {
		t.Fatalf("run() = %v", err)
	}
	if got, want := actions.runs, []string{"service.restart kodi.service"}; !reflect.DeepEqual(got, want) {
		t.Errorf("runs = %v, want %v", got, want)
	}
}
