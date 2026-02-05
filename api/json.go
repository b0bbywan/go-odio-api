package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

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
		defer r.Body.Close()

		var req T
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
