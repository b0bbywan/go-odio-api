package bluetooth

import (
	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

const defaultPinCode = "0000"
const defaultPassKey uint32 = 0

type bluezAgent struct {
	backend *BluetoothBackend
}

func (a *bluezAgent) Path() dbus.ObjectPath {
	return dbus.ObjectPath(AGENT_PATH)
}

func (a *bluezAgent) Interface() string {
	return AGENT_IFACE
}

func (a *bluezAgent) Release() *dbus.Error {
	logger.Debug("[bluetooth] Agent.Release called")
	return nil
}

func (a *bluezAgent) RequestPinCode(device dbus.ObjectPath) (string, *dbus.Error) {
	logger.Debug("[bluetooth] Agent.RequestPinCode for %s", device)
	a.backend.trustDevice(device)
	return defaultPinCode, nil
}

func (a *bluezAgent) DisplayPinCode(device dbus.ObjectPath, pincode string) *dbus.Error {
	logger.Debug("[bluetooth] Agent.DisplayPinCode for %s: %s", device, pincode)
	return nil
}

func (a *bluezAgent) RequestPasskey(device dbus.ObjectPath) (uint32, *dbus.Error) {
	logger.Debug("[bluetooth] Agent.RequestPasskey for %s", device)
	a.backend.trustDevice(device)
	return defaultPassKey, nil
}

func (a *bluezAgent) DisplayPasskey(device dbus.ObjectPath, passkey uint32, entered uint16) *dbus.Error {
	logger.Debug("[bluetooth] Agent.DisplayPasskey for %s: %06d (entered: %d)", device, passkey, entered)
	return nil
}

func (a *bluezAgent) RequestConfirmation(device dbus.ObjectPath, passkey uint32) *dbus.Error {
	logger.Debug("[bluetooth] Agent.RequestConfirmation for %s (passkey: %06d) — auto-accepting", device, passkey)
	a.backend.trustDevice(device)
	return nil
}

func (a *bluezAgent) RequestAuthorization(device dbus.ObjectPath) *dbus.Error {
	logger.Debug("[bluetooth] Agent.RequestAuthorization for %s — auto-accepting", device)
	return nil
}

func (a *bluezAgent) AuthorizeService(device dbus.ObjectPath, uuid string) *dbus.Error {
	logger.Debug("[bluetooth] Agent.AuthorizeService for %s (uuid: %s) — auto-accepting", device, uuid)
	return nil
}

func (a *bluezAgent) Cancel() *dbus.Error {
	logger.Debug("[bluetooth] Agent.Cancel called")
	return nil
}
