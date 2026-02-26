package login1

import (
	"github.com/godbus/dbus/v5"

	idbus "github.com/b0bbywan/go-odio-api/backend/internal/dbus"
)

func (l *Login1Backend) getObj() dbus.BusObject {
	return idbus.GetObject(l.conn, LOGIN1_PREFIX, LOGIN1_PATH)
}

func (l *Login1Backend) callMethod(method string, args ...interface{}) error {
	return idbus.CallMethod(l.getObj(), method, args...)
}

// callDBusMethod calls a D-Bus method and returns the call for further processing.
func (l *Login1Backend) callDBusMethod(method string, args ...interface{}) (*dbus.Call, error) {
	call := l.getObj().Call(method, 0, args...)
	if err := idbus.CallWithTimeout(call); err != nil {
		return nil, err
	}
	return call, nil
}
