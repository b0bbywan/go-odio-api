package bluetooth

import (
	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

type bluezAgent struct{}

func (a *bluezAgent) Release() *dbus.Error {
	logger.Debug("[bluetooth] agent Release() called")
	return nil
}

func (a *bluezAgent) RequestAuthorization(device dbus.ObjectPath) *dbus.Error {
	logger.Debug("[bluetooth] agent RequestAuthorization() called for device %v", device)
	return nil
}

func (a *bluezAgent) AuthorizeService(device dbus.ObjectPath, uuid string) *dbus.Error {
	logger.Debug("[bluetooth] agent AuthorizeService() called for device %v, uuid %s", device, uuid)
	return nil
}

func (a *bluezAgent) RequestConfirmation(device dbus.ObjectPath, passkey uint32) *dbus.Error {
	logger.Debug("[bluetooth] agent RequestConfirmation() called for device %v, passkey %d", device, passkey)
	// auto-accept
	return nil
}

func (a *bluezAgent) Cancel() *dbus.Error {
	logger.Debug("[bluetooth] agent Cancel() called")
	return nil
}
