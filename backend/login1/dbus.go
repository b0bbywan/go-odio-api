package login1

import (
	"context"
	"errors"

	"github.com/godbus/dbus/v5"
)

// call issues a method call bounded by the backend timeout. The deadline has
// to be on the call itself: obj.Call blocks until a reply, so wrapping it
// afterwards never times out.
func (l *Login1Backend) call(obj dbus.BusObject, method string, args ...interface{}) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()
	c := obj.CallWithContext(ctx, method, 0, args...)
	if errors.Is(c.Err, context.DeadlineExceeded) {
		c.Err = &dbusTimeoutError{}
	}
	return c
}

func (l *Login1Backend) callMethod(busName, method string, args ...interface{}) error {
	return l.call(l.conn.Object(busName, LOGIN1_PATH), method, args...).Err
}

// callDBusMethod calls a D-Bus method and returns the call for further processing
func (l *Login1Backend) callDBusMethod(method string, args ...interface{}) (*dbus.Call, error) {
	call := l.call(l.conn.Object(LOGIN1_PREFIX, LOGIN1_PATH), method, args...)
	if call.Err != nil {
		return nil, call.Err
	}
	return call, nil
}

// extractString extracts a string from a dbus.Call result
func extractString(call *dbus.Call) (string, error) {
	var result string
	if err := call.Store(&result); err != nil {
		return "", err
	}
	return result, nil
}
