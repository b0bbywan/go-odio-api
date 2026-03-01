package dbus

import "fmt"

// TimeoutError is returned when a D-Bus call exceeds its deadline.
type TimeoutError struct{}

func (e *TimeoutError) Error() string { return "dbus: call timed out" }

// SignalError is returned when a D-Bus signal body is malformed.
type SignalError struct {
	Reason string
}

func (e *SignalError) Error() string { return fmt.Sprintf("dbus: signal error: %s", e.Reason) }
