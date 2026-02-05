package mpris

// CapabilityError indique qu'une action n'est pas supportée par le player
type CapabilityError struct {
	Required string
}

func (e *CapabilityError) Error() string {
	return "action not allowed (requires " + e.Required + ")"
}

// PlayerNotFoundError indique qu'un player n'existe pas
type PlayerNotFoundError struct {
	BusName string
}

func (e *PlayerNotFoundError) Error() string {
	return "player not found: " + e.BusName
}

// InvalidBusNameError indique qu'un busName est invalide
type InvalidBusNameError struct {
	BusName string
	Reason  string
}

func (e *InvalidBusNameError) Error() string {
	return "invalid player name: " + e.Reason
}

// ValidationError indique qu'un paramètre est invalide
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
