package pulseaudio

// NotFoundError indicates that a pulseaudio resource (sink, client, output) was not found.
type NotFoundError struct {
	Resource string
	Name     string
}

func (e *NotFoundError) Error() string {
	return e.Resource + " not found: " + e.Name
}

// NotReadyError indicates that the backend is not yet ready (cache not populated).
type NotReadyError struct {
	Message string
}

func (e *NotReadyError) Error() string {
	return e.Message
}
