package mpris

// CapabilityError indicates that an action is not supported by the player
type CapabilityError struct {
	Required string
}

func (e *CapabilityError) Error() string {
	return "action not allowed (requires " + e.Required + ")"
}

// PlayerNotFoundError indicates that a player doesn't exist
type PlayerNotFoundError struct {
	BusName string
}

func (e *PlayerNotFoundError) Error() string {
	return "player not found: " + e.BusName
}

// InvalidBusNameError indicates that a busName is invalid
type InvalidBusNameError struct {
	BusName string
	Reason  string
}

func (e *InvalidBusNameError) Error() string {
	return "invalid player name: " + e.Reason
}

// ValidationError indicates that a parameter is invalid
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

type dbusTimeoutError struct{}

func (e *dbusTimeoutError) Error() string {
	return "D-Bus call timeout"
}
