package login1

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
