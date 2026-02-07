package bluetooth

import "github.com/godbus/dbus/v5"

type bluezAgent struct{}

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
