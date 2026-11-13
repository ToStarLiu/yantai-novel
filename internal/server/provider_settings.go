package server

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"novel/internal/workbench"
)

func (s *Server) registerProviderSettings(r chi.Router) {
	r.Get("/api/provider-settings", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ProviderSettings(r.Context())
		reply(w, v, e)
	})
	r.Post("/api/provider-settings", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.ProviderSettingsInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveProviderSetting(r.Context(), in)
		reply(w, v, e)
	})
	r.Post("/api/provider-settings/discover", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.ProviderSettingsInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.DiscoverProviderModels(r.Context(), in)
		reply(w, v, e)
	})
	r.Put("/api/provider-settings/{id}/default", func(w http.ResponseWriter, r *http.Request) {
		noContent(w, s.Service.SetProviderDefault(r.Context(), param(r, "id")))
	})
	r.Put("/api/provider-settings/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &in) {
			return
		}
		noContent(w, s.Service.SetProviderEnabled(r.Context(), param(r, "id"), in.Enabled))
	})
}
