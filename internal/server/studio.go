package server

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"novel/internal/workbench"
	"strconv"
)

// registerStudio is mounted inside the authenticated, CSRF-protected API group.
func (s *Server) registerStudio(r chi.Router) {
	base := "/api/projects/{pid}/studio"
	r.Get(base+"/board/at", func(w http.ResponseWriter, r *http.Request) {
		day, err := strconv.Atoi(r.URL.Query().Get("day"))
		if err != nil {
			fail(w, err)
			return
		}
		v, e := s.Service.StudioBoardAt(r.Context(), param(r, "pid"), day)
		reply(w, v, e)
	})
	r.Get(base+"/history/{kind}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.StudioHistory(r.Context(), param(r, "pid"), param(r, "kind"), r.URL.Query().Get("id"))
		reply(w, v, e)
	})
	r.Post(base+"/history/{kind}/restore", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID       string `json:"id"`
			Expected int64  `json:"expectedVersion"`
			Version  int64  `json:"version"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.RestoreStudioHistory(r.Context(), param(r, "pid"), param(r, "kind"), in.ID, in.Expected, in.Version)
		reply(w, v, e)
	})
	r.Get(base, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.Studio(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Put(base+"/board", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.StudioWrite[workbench.StudioBoard]
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveStudioBoard(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Post(base+"/board/evaluate", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Completed []string `json:"completedNodeIds"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.EvaluateStudioBoard(r.Context(), param(r, "pid"), in.Completed)
		reply(w, v, e)
	})
	r.Put(base+"/brief", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.StudioWrite[workbench.StoryBrief]
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveStudioBrief(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Put(base+"/scenes", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.StudioWrite[workbench.SceneCollection]
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveStudioScenes(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Put(base+"/arcs", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.StudioWrite[workbench.CharacterArcs]
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveStudioArcs(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Post(base+"/fork", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Title string `json:"title"`
			Epoch int64  `json:"expectedEpoch"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.ForkStudioProject(r.Context(), param(r, "pid"), in.Title, in.Epoch)
		reply(w, v, e)
	})
	r.Get(base+"/branches", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.StudioBranches(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Get(base+"/references", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.StudioReferences(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post(base+"/references", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Title      string `json:"title"`
			Text       string `json:"text"`
			SourceNote string `json:"sourceNote"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.AddStudioReference(r.Context(), param(r, "pid"), in.Title, in.Text, in.SourceNote)
		reply(w, v, e)
	})
	r.Get(base+"/references/search", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.SearchStudioReferences(r.Context(), param(r, "pid"), r.URL.Query().Get("q"))
		reply(w, v, e)
	})
	r.Delete(base+"/references/{refid}", func(w http.ResponseWriter, r *http.Request) {
		noContent(w, s.Service.DeleteStudioReference(r.Context(), param(r, "pid"), param(r, "refid")))
	})
	r.Get(base+"/wiki", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.StudioWiki(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Get(base+"/comments", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.StudioComments(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post(base+"/comments", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.StudioCommentInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.AddStudioComment(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Put(base+"/comments/{commentid}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Expected int64  `json:"expectedVersion"`
			Text     string `json:"text"`
			Resolved bool   `json:"resolved"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.UpdateStudioComment(r.Context(), param(r, "pid"), param(r, "commentid"), in.Expected, in.Text, in.Resolved)
		reply(w, v, e)
	})
	r.Get(base+"/sandboxes", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.StudioSandboxes(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post(base+"/sandboxes", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.SandboxCreate
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.CreateStudioSandbox(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	for _, action := range []string{"simulate", "confirm", "freeze", "rollback"} {
		action := action
		r.Post(base+"/sandboxes/{boxid}/"+action, func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Expected   int64  `json:"expectedVersion"`
				Steps      int    `json:"steps"`
				Seed       int64  `json:"seed"`
				Checkpoint string `json:"checkpoint"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.StudioSandboxAction(r.Context(), param(r, "pid"), param(r, "boxid"), action, in.Expected, in.Steps, in.Seed, in.Checkpoint)
			reply(w, v, e)
		})
	}
}
