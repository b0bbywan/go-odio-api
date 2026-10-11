// Package action names the things a key, or any other trigger, can do to the
// player, audio, power, service and bluetooth backends.
package action

import (
	"errors"
	"fmt"

	"github.com/b0bbywan/go-odio-api/backend/bluetooth"
	"github.com/b0bbywan/go-odio-api/backend/login1"
	"github.com/b0bbywan/go-odio-api/backend/mpris"
	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

// Action names, as written in the configuration.
const (
	PlayerPlayPause = "player.playpause"
	PlayerPlay      = "player.play"
	PlayerPause     = "player.pause"
	PlayerStop      = "player.stop"
	PlayerNext      = "player.next"
	PlayerPrevious  = "player.previous"
	PlayerForward   = "player.forward"
	PlayerRewind    = "player.rewind"

	VolumeUp   = "volume.up"
	VolumeDown = "volume.down"
	VolumeMute = "volume.mute"

	PowerOff    = "power.poweroff"
	PowerReboot = "power.reboot"

	ServiceStart   = "service.start"
	ServiceStop    = "service.stop"
	ServiceRestart = "service.restart"

	BluetoothPowerUp   = "bluetooth.powerup"
	BluetoothPowerDown = "bluetooth.powerdown"
	BluetoothPairing   = "bluetooth.pairing"
)

var (
	ErrUnknown  = errors.New("unknown action")
	ErrDisabled = errors.New("backend is disabled")
	ErrNoUnit   = errors.New("needs a unit")
	ErrNoPlayer = errors.New("no player to control")
)

// Action is an action bound to the backend it drives.
type Action struct {
	// Repeat marks an action worth firing again while its trigger is held.
	Repeat bool
	Run    func() error
}

type playerControl interface {
	ActivePlayer() (string, bool)
	PlayPause(busName string) error
	Play(busName string) error
	Pause(busName string) error
	Stop(busName string) error
	Next(busName string) error
	Previous(busName string) error
	Forward(busName string) error
	Rewind(busName string) error
}

type audioControl interface {
	VolumeUpMaster() error
	VolumeDownMaster() error
	ToggleMuteMaster() error
}

type powerControl interface {
	Reboot() error
	PowerOff() error
}

type serviceControl interface {
	StartService(name string, scope systemd.UnitScope) error
	StopService(name string, scope systemd.UnitScope) error
	RestartService(name string, scope systemd.UnitScope) error
}

type bluetoothControl interface {
	PowerUp() error
	PowerDown() error
	NewPairing() error
}

// One table per backend: an action's table is the backend it needs.

var playerActions = map[string]struct {
	repeat bool
	do     func(p playerControl, busName string) error
}{
	PlayerPlayPause: {do: playerControl.PlayPause},
	PlayerPlay:      {do: playerControl.Play},
	PlayerPause:     {do: playerControl.Pause},
	PlayerStop:      {do: playerControl.Stop},
	PlayerNext:      {do: playerControl.Next},
	PlayerPrevious:  {do: playerControl.Previous},
	PlayerForward:   {repeat: true, do: playerControl.Forward},
	PlayerRewind:    {repeat: true, do: playerControl.Rewind},
}

var audioActions = map[string]struct {
	repeat bool
	do     func(audioControl) error
}{
	VolumeUp:   {repeat: true, do: audioControl.VolumeUpMaster},
	VolumeDown: {repeat: true, do: audioControl.VolumeDownMaster},
	VolumeMute: {do: audioControl.ToggleMuteMaster},
}

var powerActions = map[string]func(powerControl) error{
	PowerOff:    powerControl.PowerOff,
	PowerReboot: powerControl.Reboot,
}

var serviceActions = map[string]func(s serviceControl, unit string, scope systemd.UnitScope) error{
	ServiceStart:   serviceControl.StartService,
	ServiceStop:    serviceControl.StopService,
	ServiceRestart: serviceControl.RestartService,
}

var bluetoothActions = map[string]func(bluetoothControl) error{
	BluetoothPowerUp:   bluetoothControl.PowerUp,
	BluetoothPowerDown: bluetoothControl.PowerDown,
	BluetoothPairing:   bluetoothControl.NewPairing,
}

// Targets holds the backends actions act on; nil for a disabled one.
type Targets struct {
	players   playerControl
	audio     audioControl
	power     powerControl
	services  serviceControl
	bluetooth bluetoothControl
}

func New(
	players *mpris.MPRISBackend,
	audio *pulseaudio.PulseAudioBackend,
	power *login1.Login1Backend,
	services *systemd.SystemdBackend,
	bt *bluetooth.BluetoothBackend,
) *Targets {
	t := &Targets{}
	// Assigning a nil backend pointer through an interface would make the field
	// non-nil; only set it for a real backend so a disabled one stays nil.
	if players != nil {
		t.players = players
	}
	if audio != nil {
		t.audio = audio
	}
	if power != nil {
		t.power = power
	}
	if services != nil {
		t.services = services
	}
	if bt != nil {
		t.bluetooth = bt
	}
	return t
}

// Resolve binds the named action to its backend; unit is the systemd user unit
// of a service.* action. It fails for an action that could never run, so the
// caller can reject it up front rather than on use.
func (t *Targets) Resolve(name, unit string) (Action, error) {
	a, err := t.resolve(name, unit)
	if err != nil {
		return Action{}, fmt.Errorf("%s: %w", name, err)
	}
	return a, nil
}

func (t *Targets) resolve(name, unit string) (Action, error) {
	if a, ok := playerActions[name]; ok {
		if t.players == nil {
			return Action{}, ErrDisabled
		}
		return Action{Repeat: a.repeat, Run: func() error {
			busName, ok := t.players.ActivePlayer()
			if !ok {
				return ErrNoPlayer
			}
			return a.do(t.players, busName)
		}}, nil
	}

	if a, ok := audioActions[name]; ok {
		if t.audio == nil {
			return Action{}, ErrDisabled
		}
		return Action{Repeat: a.repeat, Run: func() error {
			return a.do(t.audio)
		}}, nil
	}

	if do, ok := powerActions[name]; ok {
		if t.power == nil {
			return Action{}, ErrDisabled
		}
		return Action{Run: func() error {
			return do(t.power)
		}}, nil
	}

	if do, ok := serviceActions[name]; ok {
		if t.services == nil {
			return Action{}, ErrDisabled
		}
		if unit == "" {
			return Action{}, ErrNoUnit
		}
		return Action{Run: func() error {
			return do(t.services, unit, systemd.ScopeUser)
		}}, nil
	}

	if do, ok := bluetoothActions[name]; ok {
		if t.bluetooth == nil {
			return Action{}, ErrDisabled
		}
		return Action{Run: func() error {
			return do(t.bluetooth)
		}}, nil
	}

	return Action{}, ErrUnknown
}
