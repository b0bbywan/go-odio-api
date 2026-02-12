package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/b0bbywan/go-odio-api/logger"
)

const maxRequestBodySize = 1 << 20 // 1 MB

type setVolumeRequest struct {
	Volume float32 `json:"volume"`
}

func JSONHandler(h func(http.ResponseWriter, *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := h(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// withBody parse et valide le body JSON, puis appelle next
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

		// Check Content-Type header
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}

		// Limit request body size to prevent DOS attacks
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

		var req T
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Check if error is due to body size limit
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

// validateVolume valide qu'un volume est entre 0 et 1
func validateVolume(req *setVolumeRequest) error {
	if req.Volume < 0 || req.Volume > 1 {
		return errors.New("volume must be between 0 and 1")
	}
	return nil
}
