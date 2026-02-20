package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/b0bbywan/go-odio-api/logger"
)

const maxRequestBodySize = 1 << 20 // 1 MB

type setVolumeRequest struct {
	Volume float32 `json:"volume"`
}

// statusError is an error carrying an HTTP status code, recognised by JSONHandler.
type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string { return e.msg }

// httpError wraps an error with an HTTP status code for JSONHandler to use.
func httpError(code int, err error) error {
	return &statusError{code: code, msg: err.Error()}
}

// JSONHandler wraps a handler returning (data, error) into an http.HandlerFunc:
//   - statusError → that HTTP code + plain-text body
//   - plain error → 500
//   - non-nil data → 200 with JSON body
func JSONHandler(h func(http.ResponseWriter, *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := h(w, r)
		if err != nil {
			code := http.StatusInternalServerError
			var se *statusError
			if errors.As(err, &se) {
				code = se.code
			}
			http.Error(w, err.Error(), code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// withBody parses and validates a JSON request body, then calls next.
func withBody[T any](
	validate func(*T) error,
	next func(w http.ResponseWriter, r *http.Request, req *T),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := r.Body.Close(); err != nil {
				logger.Info("Failed to close request body: %v", err)
			}
		}()

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

		var req T
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		if validate != nil {
			if err := validate(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		next(w, r, &req)
	}
}

// setCacheHeader sets X-Cache-Updated-At to the given timestamp.
func setCacheHeader(w http.ResponseWriter, t time.Time) {
	if !t.IsZero() {
		w.Header().Set("X-Cache-Updated-At", t.UTC().Format(time.RFC3339))
	}
}

// listHandler wraps a backend list function into a JSONHandler that also sets
// the X-Cache-Updated-At header from the provided timestamp function.
func listHandler[T any](list func() (T, error), updatedAt func() time.Time) http.HandlerFunc {
	return JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
		data, err := list()
		if err != nil {
			return nil, err
		}
		setCacheHeader(w, updatedAt())
		return data, nil
	})
}

// validateVolume validates that a volume is between 0 and 1
func validateVolume(req *setVolumeRequest) error {
	if req.Volume < 0 || req.Volume > 1 {
		return errors.New("volume must be between 0 and 1")
	}
	return nil
}
