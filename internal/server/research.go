package server

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"novel/internal/workbench"
)

func (s *Server) registerResearch(r chi.Router) {
	base := "/api/projects/{pid}/research"
	r.Get(base, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListResearch(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post(base, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Topic string   `json:"topic"`
			URLs  []string `json:"urls"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.Research(r.Context(), param(r, "pid"), in.Topic, in.URLs)
		reply(w, v, e)
	})
	r.Get(base+"/config", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"urlResearch": true, "webSearchConfigured": workbench.ResearchSearchAvailable(), "maxSources": 5, "maxBytesPerSource": 2 << 20, "timeoutSeconds": 10})
	})
	r.Post(base+"/search", func(w http.ResponseWriter, r *http.Request) {
		if _, e := s.Service.GetProject(r.Context(), param(r, "pid")); e != nil {
			fail(w, e)
			return
		}
		var in struct {
			Query string `json:"query"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := workbench.ResearchSearch(r.Context(), in.Query)
		reply(w, v, e)
	})
	r.Post("/api/projects/{pid}/studio/brief/apply-plan", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.ApplyPlanInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.ApplyStudioPlan(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Get("/api/projects/{pid}/studio/references/overlap", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ReferenceOverlap(r.Context(), param(r, "pid"), r.URL.Query().Get("revisionId"))
		reply(w, v, e)
	})
}
