package bluetooth

import "github.com/godbus/dbus/v5"

type agent struct{}

func (a *agent) Release() *dbus.Error {
	return nil
}

func (a *agent) RequestAuthorization(device dbus.ObjectPath) *dbus.Error {
	return nil
}

func (a *agent) AuthorizeService(device dbus.ObjectPath, uuid string) *dbus.Error {
	return nil
}

func (a *agent) RequestConfirmation(device dbus.ObjectPath, passkey uint32) *dbus.Error {
	// auto-accept
	return nil
}

func (a *agent) Cancel() *dbus.Error {
	return nil
}
