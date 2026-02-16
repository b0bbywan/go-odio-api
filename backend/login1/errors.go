package login1

// InvalidBusNameError indicates that a busName is invalid
type InvalidBusNameError struct {
	BusName string
	Reason  string
}

func (e *InvalidBusNameError) Error() string {
	return "invalid bus name: " + e.Reason
}

// CapabilityError indicates that an action is not supported by the player
type CapabilityError struct {
	Required string
}

func (e *CapabilityError) Error() string {
	return "action not allowed (requires " + e.Required + ")"
}

type dbusTimeoutError struct{}

func (e *dbusTimeoutError) Error() string {
	return "D-Bus call timeout"
}
