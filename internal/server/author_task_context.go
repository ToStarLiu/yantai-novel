package server

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (s *Server) registerAuthorTaskContext(r chi.Router) {
	r.Get("/api/runs/{id}/author-context", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.GetAuthorTaskContext(r.Context(), param(r, "id"))
		reply(w, v, e)
	})
}
