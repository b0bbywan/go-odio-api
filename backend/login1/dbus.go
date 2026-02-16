package login1

import (
	"time"

	"github.com/godbus/dbus/v5"
)

// callWithTimeout executes a D-Bus call with timeout
func callWithTimeout(call *dbus.Call, timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		done <- call.Err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return &dbusTimeoutError{}
	}
}

func (l *Login1Backend) callMethod(busName, method string, args ...interface{}) error {
	obj := l.conn.Object(busName, LOGIN1_PATH)
	return l.callWithTimeout(obj.Call(method, 0, args...))
}

func (l *Login1Backend) callWithTimeout(call *dbus.Call) error {
	return callWithTimeout(call, l.timeout)
}

// callDBusMethod calls a D-Bus method and returns the call for further processing
func (l *Login1Backend) callDBusMethod(method string, args ...interface{}) (*dbus.Call, error) {
	obj := l.conn.Object(LOGIN1_PREFIX, LOGIN1_PATH)
	call := obj.Call(method, 0, args...)
	if err := l.callWithTimeout(call); err != nil {
		return nil, err
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
