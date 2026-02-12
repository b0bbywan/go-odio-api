package bluetooth

import "github.com/godbus/dbus/v5"

type Agent1Client interface {
	Release() *dbus.Error                                                    // Callback doesn't trigger on unregister
	RequestPinCode(device dbus.ObjectPath) (pincode string, err *dbus.Error) // Triggers for pairing when SSP is off and cap != CAP_NO_INPUT_NO_OUTPUT
	DisplayPinCode(device dbus.ObjectPath, pincode string) *dbus.Error
	RequestPasskey(device dbus.ObjectPath) (passkey uint32, err *dbus.Error) // SSP on, toolz.AGENT_CAP_KEYBOARD_ONLY
	DisplayPasskey(device dbus.ObjectPath, passkey uint32, entered uint16) *dbus.Error
	RequestConfirmation(device dbus.ObjectPath, passkey uint32) *dbus.Error
	RequestAuthorization(device dbus.ObjectPath) *dbus.Error
	AuthorizeService(device dbus.ObjectPath, uuid string) *dbus.Error
	Cancel() *dbus.Error
	Path() dbus.ObjectPath
	Interface() string
}

type bluezAgent struct {
	backend *BluetoothBackend
}

func (a *bluezAgent) Release() *dbus.Error {
	return nil
}

func (a *bluezAgent) RequestAuthorization(device dbus.ObjectPath) *dbus.Error {
	return nil
}

func (a *bluezAgent) AuthorizeService(device dbus.ObjectPath, uuid string) *dbus.Error {
	return nil
}

func (a *bluezAgent) RequestConfirmation(device dbus.ObjectPath, passkey uint32) *dbus.Error {
	// auto-accept
	return nil
}

func (a *bluezAgent) Cancel() *dbus.Error {
	return nil
}
