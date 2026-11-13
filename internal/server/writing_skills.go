package server

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"novel/internal/workbench"
	"strconv"
)

func (s *Server) registerWritingSkills(r chi.Router) {
	r.Get("/api/writing-skill-catalog", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(workbench.WritingSkillCatalog())
	})
	base := "/api/projects/{pid}/writing-skills"
	r.Get(base, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListWritingSkills(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post(base+"/preview", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.WritingSkillPreviewInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.PreviewWritingSkill(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Post(base, func(w http.ResponseWriter, r *http.Request) {
		var in workbench.WritingSkillSave
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveWritingSkill(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Put(base+"/{sid}", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.WritingSkillUpdate
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.UpdateWritingSkill(r.Context(), param(r, "pid"), param(r, "sid"), in)
		reply(w, v, e)
	})
	r.Delete(base+"/{sid}", func(w http.ResponseWriter, r *http.Request) {
		version, e := strconv.ParseInt(r.URL.Query().Get("expectedVersion"), 10, 64)
		if e != nil || version < 1 {
			fail(w, fmt.Errorf("删除Skill需要当前expectedVersion"))
			return
		}
		noContent(w, s.Service.DeleteWritingSkill(r.Context(), param(r, "pid"), param(r, "sid"), version))
	})
	r.Get(base+"/{sid}/export", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.GetWritingSkill(r.Context(), param(r, "pid"), param(r, "sid"))
		if e != nil {
			fail(w, e)
			return
		}
		format := r.URL.Query().Get("format")
		if format == "markdown" {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="`+v.Name+`-SKILL.md"`)
			w.Write([]byte(v.Content))
			return
		}
		if format != "" && format != "bundle" {
			fail(w, fmt.Errorf("导出格式无效"))
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+v.Name+`-skill.json"`)
		respond(w, 200, v.SkillPreview)
	})
}
