package server

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"novel/internal/workbench"
)

func (s *Server) registerPublishing(r chi.Router) {
	r.Post("/api/chapters/{cid}/candidates", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.CandidateRequest
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.StartCandidates(r.Context(), param(r, "cid"), in)
		reply(w, v, e)
	})
	r.Get("/api/projects/{pid}/candidates", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListCandidateGroups(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Get("/api/projects/{pid}/publish/{format}", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		v, e := s.Service.Publish(r.Context(), param(r, "pid"), param(r, "format"), workbench.PublicationOptions{Author: q.Get("author"), Language: q.Get("language"), UseFinal: q.Get("useFinal") == "true", IncludeSynopsis: q.Get("includeSynopsis") == "true"})
		if e != nil {
			fail(w, e)
			return
		}
		w.Header().Set("Content-Type", v.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, v.Filename))
		_, _ = w.Write(v.Data)
	})
	r.Get("/api/media/config", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, s.Service.MediaCapabilities()) })
	r.Get("/api/projects/{pid}/media", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListMedia(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post("/api/projects/{pid}/media", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.MediaRequest
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SubmitMedia(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Get("/api/projects/{pid}/media/{id}/content", func(w http.ResponseWriter, r *http.Request) {
		data, mime, e := s.Service.MediaContent(r.Context(), param(r, "pid"), param(r, "id"))
		if e != nil {
			fail(w, e)
			return
		}
		w.Header().Set("Content-Type", mime)
		_, _ = w.Write(data)
	})
	r.Post("/api/projects/{pid}/media/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		noContent(w, s.Service.CancelMedia(r.Context(), param(r, "pid"), param(r, "id")))
	})
}
