package server

import (
	"fmt"
	"net/http"
	"novel/internal/workbench"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *Server) registerAuthorMemory(r chi.Router) {
	r.Get("/api/author-memories", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListAuthorMemories(r.Context(), r.URL.Query().Get("projectId"), r.URL.Query().Get("status"))
		reply(w, v, e)
	})
	r.Post("/api/author-memories", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.AuthorMemoryInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveAuthorMemory(r.Context(), in)
		reply(w, v, e)
	})
	r.Get("/api/author-memories/export", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ExportAuthorMemories(r.Context())
		if e != nil {
			fail(w, e)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="author-memories.json"`)
		respond(w, http.StatusOK, v)
	})
	r.Post("/api/author-memories/import", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.AuthorMemoryImport
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.ImportAuthorMemories(r.Context(), in)
		reply(w, v, e)
	})
	r.Put("/api/author-memories/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.AuthorMemoryUpdate
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.UpdateAuthorMemory(r.Context(), param(r, "id"), in)
		reply(w, v, e)
	})
	r.Delete("/api/author-memories/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ExpectedVersion int64 `json:"expectedVersion"`
		}
		// Match other versioned delete routes while retaining JSON-body clients.
		if raw := r.URL.Query().Get("expectedVersion"); raw != "" {
			version, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || version < 1 {
				fail(w, fmt.Errorf("忘记记忆需要当前expectedVersion"))
				return
			}
			in.ExpectedVersion = version
		} else if !decode(w, r, &in) {
			return
		}
		if in.ExpectedVersion < 1 {
			fail(w, fmt.Errorf("忘记记忆需要当前expectedVersion"))
			return
		}
		noContent(w, s.Service.DeleteAuthorMemory(r.Context(), param(r, "id"), in.ExpectedVersion))
	})
	r.Get("/api/author-memory-settings", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.GetAuthorMemorySettings(r.Context())
		reply(w, v, e)
	})
	r.Put("/api/author-memory-settings", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.AuthorMemorySettingsUpdate
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.UpdateAuthorMemorySettings(r.Context(), in)
		reply(w, v, e)
	})
}
