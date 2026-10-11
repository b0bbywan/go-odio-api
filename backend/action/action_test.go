package action

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

type fakePlayers struct {
	active string
	calls  []string
}

func (f *fakePlayers) ActivePlayer() (string, bool) { return f.active, f.active != "" }
func (f *fakePlayers) record(op, busName string) error {
	f.calls = append(f.calls, op+" "+busName)
	return nil
}
func (f *fakePlayers) PlayPause(busName string) error { return f.record("playpause", busName) }
func (f *fakePlayers) Play(busName string) error      { return f.record("play", busName) }
func (f *fakePlayers) Pause(busName string) error     { return f.record("pause", busName) }
func (f *fakePlayers) Stop(busName string) error      { return f.record("stop", busName) }
func (f *fakePlayers) Next(busName string) error      { return f.record("next", busName) }
func (f *fakePlayers) Previous(busName string) error  { return f.record("previous", busName) }
func (f *fakePlayers) Forward(busName string) error   { return f.record("forward", busName) }
func (f *fakePlayers) Rewind(busName string) error    { return f.record("rewind", busName) }

type fakeAudio struct{ calls []string }

func (f *fakeAudio) VolumeUpMaster() error   { f.calls = append(f.calls, "up"); return nil }
func (f *fakeAudio) VolumeDownMaster() error { f.calls = append(f.calls, "down"); return nil }
func (f *fakeAudio) ToggleMuteMaster() error { f.calls = append(f.calls, "mute"); return nil }

type fakePower struct{ calls []string }

func (f *fakePower) Reboot() error   { f.calls = append(f.calls, "reboot"); return nil }
func (f *fakePower) PowerOff() error { f.calls = append(f.calls, "poweroff"); return nil }

type fakeServices struct{ calls []string }

func (f *fakeServices) record(op, name string, scope systemd.UnitScope) error {
	f.calls = append(f.calls, op+" "+name+" "+string(scope))
	return nil
}
func (f *fakeServices) StartService(name string, scope systemd.UnitScope) error {
	return f.record("start", name, scope)
}
func (f *fakeServices) StopService(name string, scope systemd.UnitScope) error {
	return f.record("stop", name, scope)
}
func (f *fakeServices) RestartService(name string, scope systemd.UnitScope) error {
	return f.record("restart", name, scope)
}

type fakeBluetooth struct{ calls []string }

func (f *fakeBluetooth) PowerUp() error    { f.calls = append(f.calls, "powerup"); return nil }
func (f *fakeBluetooth) PowerDown() error  { f.calls = append(f.calls, "powerdown"); return nil }
func (f *fakeBluetooth) NewPairing() error { f.calls = append(f.calls, "pairing"); return nil }

var allActions = []string{
	"player.playpause", "player.play", "player.pause", "player.stop",
	"player.next", "player.previous", "player.forward", "player.rewind",
	"volume.up", "volume.down", "volume.mute",
	"power.poweroff", "power.reboot",
	"service.start", "service.stop", "service.restart",
	"bluetooth.powerup", "bluetooth.powerdown", "bluetooth.pairing",
}

func TestActions(t *testing.T) {
	const vlc = "org.mpris.MediaPlayer2.vlc"
	players := &fakePlayers{active: vlc}
	audio := &fakeAudio{}
	power := &fakePower{}
	services := &fakeServices{}
	bt := &fakeBluetooth{}
	targets := &Targets{players: players, audio: audio, power: power, services: services, bluetooth: bt}

	for _, name := range allActions {
		a, err := targets.Resolve(name, "kodi.service")
		if err != nil {
			t.Fatalf("Resolve(%q) = %v", name, err)
		}
		if err := a.Run(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	registered := len(playerActions) + len(audioActions) + len(powerActions) + len(serviceActions) + len(bluetoothActions)
	if registered != len(allActions) {
		t.Errorf("%d actions registered, %d exercised here", registered, len(allActions))
	}

	wantPlayers := []string{
		"playpause " + vlc, "play " + vlc, "pause " + vlc, "stop " + vlc,
		"next " + vlc, "previous " + vlc, "forward " + vlc, "rewind " + vlc,
	}
	if !reflect.DeepEqual(players.calls, wantPlayers) {
		t.Errorf("player calls = %v, want %v", players.calls, wantPlayers)
	}
	if want := []string{"up", "down", "mute"}; !reflect.DeepEqual(audio.calls, want) {
		t.Errorf("audio calls = %v, want %v", audio.calls, want)
	}
	if want := []string{"poweroff", "reboot"}; !reflect.DeepEqual(power.calls, want) {
		t.Errorf("power calls = %v, want %v", power.calls, want)
	}
	wantServices := []string{"start kodi.service user", "stop kodi.service user", "restart kodi.service user"}
	if !reflect.DeepEqual(services.calls, wantServices) {
		t.Errorf("service calls = %v, want %v", services.calls, wantServices)
	}
	if want := []string{"powerup", "powerdown", "pairing"}; !reflect.DeepEqual(bt.calls, want) {
		t.Errorf("bluetooth calls = %v, want %v", bt.calls, want)
	}
}

func TestRepeat(t *testing.T) {
	targets := &Targets{
		players: &fakePlayers{}, audio: &fakeAudio{}, power: &fakePower{},
		services: &fakeServices{}, bluetooth: &fakeBluetooth{},
	}
	repeatable := map[string]bool{
		"player.forward": true, "player.rewind": true,
		"volume.up": true, "volume.down": true,
	}

	for _, name := range allActions {
		a, err := targets.Resolve(name, "kodi.service")
		if err != nil {
			t.Fatalf("Resolve(%q) = %v", name, err)
		}
		if a.Repeat != repeatable[name] {
			t.Errorf("%s: Repeat = %t, want %t", name, a.Repeat, repeatable[name])
		}
	}
}

func TestResolveErrors(t *testing.T) {
	all := &Targets{players: &fakePlayers{}, audio: &fakeAudio{}, power: &fakePower{}, services: &fakeServices{}}

	tests := []struct {
		name    string
		targets *Targets
		action  string
		unit    string
		want    error
	}{
		{name: "unknown action", targets: all, action: "player.bogus", want: ErrUnknown},
		{name: "empty action", targets: all, action: "", want: ErrUnknown},
		{name: "unknown group", targets: all, action: "bogus.next", want: ErrUnknown},
		{name: "players disabled", targets: &Targets{audio: &fakeAudio{}}, action: "player.next", want: ErrDisabled},
		{name: "audio disabled", targets: &Targets{players: &fakePlayers{}}, action: "volume.up", want: ErrDisabled},
		{name: "power disabled", targets: &Targets{}, action: "power.poweroff", want: ErrDisabled},
		{name: "services disabled", targets: &Targets{}, action: "service.start", unit: "kodi.service", want: ErrDisabled},
		{name: "service without a unit", targets: all, action: "service.start", want: ErrNoUnit},
		{name: "bluetooth disabled", targets: &Targets{}, action: "bluetooth.pairing", want: ErrDisabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := tt.targets.Resolve(tt.action, tt.unit)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Resolve() = %v, want %v", err, tt.want)
			}
			if !strings.HasPrefix(err.Error(), tt.action+": ") {
				t.Errorf("error %q does not name the action", err)
			}
			if a.Run != nil {
				t.Error("a failed Resolve must not return a runnable action")
			}
		})
	}
}

func TestPlayerActionWithoutPlayer(t *testing.T) {
	players := &fakePlayers{}
	targets := &Targets{players: players}

	a, err := targets.Resolve("player.playpause", "")
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}
	if err := a.Run(); err != ErrNoPlayer {
		t.Errorf("Run() = %v, want %v", err, ErrNoPlayer)
	}
	if len(players.calls) != 0 {
		t.Errorf("player calls = %v, want none", players.calls)
	}
}

// The nil backend pointers of disabled backends must not leak into the
// interfaces, or their actions would resolve and panic when run.
func TestNewWithoutBackends(t *testing.T) {
	targets := New(nil, nil, nil, nil, nil)

	for _, name := range allActions {
		if _, err := targets.Resolve(name, "kodi.service"); !errors.Is(err, ErrDisabled) {
			t.Errorf("Resolve(%q) = %v, want %v", name, err, ErrDisabled)
		}
	}
}
