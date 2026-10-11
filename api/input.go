package api

import (
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/input"
)

func (s *Server) registerInputRoutes(b *input.InputBackend) {
	s.mux.HandleFunc(
		"GET /input",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.GetStatus(), nil
		}),
	)
}
