package server

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"novel/internal/workbench"
)

func (s *Server) registerStudioFeatures(r chi.Router) {
	r.Post("/api/chapters/{cid}/hook-review", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.RunRequest
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SubmitHookReview(r.Context(), param(r, "cid"), in)
		reply(w, v, e)
	})
	base := "/api/projects/{pid}/studio"
	r.Post(base+"/voice/apply", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.ApplyVoiceInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.ApplyVoiceProfile(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	for suffix, feature := range map[string]string{"/node-scene": "canvas-scene", "/relations/explain": "relation-explanation"} {
		feature := feature
		r.Post(base+suffix, func(w http.ResponseWriter, r *http.Request) {
			var in workbench.CanvasTaskInput
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.SubmitCanvasTask(r.Context(), param(r, "pid"), in, feature)
			reply(w, v, e)
		})
	}
	r.Post(base+"/scenes/apply-candidate", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.ApplySceneInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.ApplyCanvasScene(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
}
