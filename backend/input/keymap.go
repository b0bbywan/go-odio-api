package input

import (
	"fmt"
	"strings"

	evdev "github.com/holoplot/go-evdev"

	"github.com/b0bbywan/go-odio-api/backend/action"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

// actionNone unmaps a key, typically one bound by default.
const actionNone = "none"

var defaultBindings = []struct {
	code   evdev.EvCode
	action string
}{
	{evdev.KEY_PLAYPAUSE, action.PlayerPlayPause},
	{evdev.KEY_PLAY, action.PlayerPlay},
	{evdev.KEY_PAUSE, action.PlayerPause},
	{evdev.KEY_STOPCD, action.PlayerStop},
	{evdev.KEY_NEXTSONG, action.PlayerNext},
	{evdev.KEY_PREVIOUSSONG, action.PlayerPrevious},
	{evdev.KEY_FASTFORWARD, action.PlayerForward},
	{evdev.KEY_REWIND, action.PlayerRewind},
	{evdev.KEY_VOLUMEUP, action.VolumeUp},
	{evdev.KEY_VOLUMEDOWN, action.VolumeDown},
	{evdev.KEY_MUTE, action.VolumeMute},
}

// keyName is the kernel's name for a key code.
func keyName(code evdev.EvCode) string {
	if name, ok := evdev.KEYToString[code]; ok {
		return name
	}
	return fmt.Sprintf("KEY_%d", code)
}

// buildKeymap layers the user's bindings over the defaults. An entry that
// cannot work is skipped rather than failing startup: loudly for a user
// binding, quietly for a default whose backend is simply disabled.
func (b *InputBackend) buildKeymap(user []config.InputBinding) map[evdev.EvCode]binding {
	keymap := make(map[evdev.EvCode]binding)
	for _, d := range defaultBindings {
		b.bind(keymap, d.code, Binding{Key: keyName(d.code), Action: d.action}, logger.Debug)
	}
	for _, bd := range user {
		key := strings.ToUpper(bd.Key)
		code, ok := evdev.KEYFromString[key]
		switch {
		case !ok:
			logger.Warn("[input] binding skipped: unknown key %q", bd.Key)
		case bd.Action == actionNone:
			delete(keymap, code)
		default:
			b.bind(keymap, code, Binding{Key: key, Action: bd.Action, Unit: bd.Unit}, logger.Warn)
		}
	}
	return keymap
}

// bind maps code to bd once its action resolves, and reports through skip
// when it does not.
func (b *InputBackend) bind(keymap map[evdev.EvCode]binding, code evdev.EvCode, bd Binding, skip func(string, ...any)) {
	act, err := b.actions.Resolve(bd.Action, bd.Unit)
	if err != nil {
		skip("[input] binding %s skipped: %v", bd.Key, err)
		return
	}
	keymap[code] = binding{Binding: bd, repeat: act.Repeat, run: act.Run}
}
