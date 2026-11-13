package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"novel/internal/workbench"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	Service *workbench.Service
	Auth    Auth
	WebRoot string
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 34<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		respond(w, 400, map[string]string{"error": "请求内容无效或超过大小限制"})
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		respond(w, 400, map[string]string{"error": "请求只能包含一个JSON对象"})
		return false
	}
	return true
}
func fail(w http.ResponseWriter, err error) {
	if errors.Is(err, workbench.ErrForbidden) {
		respond(w, 403, map[string]string{"error": "没有执行该操作的权限"})
		return
	}
	var conflict *workbench.Conflict
	if errors.As(err, &conflict) {
		respond(w, 409, map[string]string{"error": conflict.Message, "currentRevisionId": conflict.CurrentRevisionID})
		return
	}
	if errors.Is(err, workbench.ErrNotFound) {
		respond(w, 404, map[string]string{"error": "未找到记录"})
		return
	}
	message := err.Error()
	if strings.Contains(message, "SQLSTATE") || strings.Contains(message, "conn") || strings.Contains(message, "scan") {
		message = "数据操作失败，请检查服务状态"
	}
	respond(w, 400, map[string]string{"error": message})
}
func reply(w http.ResponseWriter, value any, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, value)
}
func noContent(w http.ResponseWriter, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func param(r *http.Request, name string) string { return chi.URLParam(r, name) }

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "same-origin")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Cache-Control", "no-store")
			defer func() {
				if recover() != nil {
					respond(w, 500, map[string]string{"error": "服务内部错误"})
				}
			}()
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Service.DB.Ping(r.Context()); err != nil {
			respond(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	r.Get("/api/session", s.sessionInfo)
	r.Post("/api/session", s.login)
	s.registerCollaborationPublic(r)
	r.Post("/mcp", s.mcpProtocol)
	r.Get("/mcp", s.mcpProtocol)
	r.Group(func(r chi.Router) {
		r.Use(s.protect)
		s.registerStudio(r)
		s.registerStudioFeatures(r)
		s.registerAutomation(r)
		s.registerPublishing(r)
		s.registerResearch(r)
		s.registerMCP(r)
		s.registerCollaboration(r)
		s.registerCopilot(r)
		s.registerProviderSettings(r)
		s.registerWritingSkills(r)
		s.registerAuthorMemory(r)
		r.Delete("/api/session", s.logout)
		r.Get("/api/config", func(w http.ResponseWriter, r *http.Request) {
			_, policy, provider, e := s.Service.ResolveTaskProvider(r.Context(), "copilot")
			if e != nil {
				fail(w, e)
				return
			}
			routes := s.Service.ModelRoutes()
			routes["copilot"] = policy
			respond(w, 200, map[string]any{"model": policy.Model, "modelRoutes": routes, "provider": provider, "externalModel": provider != "fake", "maxOutputTokens": s.Service.MaxOutputTokens, "retrievalMode": "keyword", "currency": s.Service.Currency, "costStatus": "Unknown"})
		})
		r.Get("/api/style-templates", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, workbench.StyleTemplates()) })
		r.Get("/api/projects/{pid}/map", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.StoryMap(r.Context(), param(r, "pid"))
			reply(w, v, e)
		})
		r.Get("/api/chapters/{cid}/lint", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.DeterministicReview(r.Context(), param(r, "cid"), r.URL.Query().Get("revisionId"))
			reply(w, v, e)
		})
		r.Get("/api/projects", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.ListProjects(r.Context())
			reply(w, v, e)
		})
		r.Post("/api/projects", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Genre       string `json:"genre"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.CreateProject(r.Context(), in.Title, in.Description, in.Genre)
			reply(w, v, e)
		})
		r.Get("/api/projects/{pid}", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.GetProject(r.Context(), param(r, "pid"))
			reply(w, v, e)
		})
		r.Get("/api/projects/{pid}/preferences", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.Preferences(r.Context(), param(r, "pid"))
			reply(w, v, e)
		})
		r.Put("/api/projects/{pid}/preferences", func(w http.ResponseWriter, r *http.Request) {
			var in workbench.Preferences
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.SavePreferences(r.Context(), param(r, "pid"), in)
			reply(w, v, e)
		})
		r.Get("/api/projects/{pid}/cost", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.CostSummary(r.Context(), param(r, "pid"))
			reply(w, v, e)
		})
		r.Put("/api/projects/{pid}", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Genre       string `json:"genre"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.UpdateProject(r.Context(), param(r, "pid"), in.Title, in.Description, in.Genre)
			reply(w, v, e)
		})
		r.Delete("/api/projects/{pid}", func(w http.ResponseWriter, r *http.Request) {
			noContent(w, s.Service.DeleteProject(r.Context(), param(r, "pid")))
		})
		r.Post("/api/projects/{pid}/settings", func(w http.ResponseWriter, r *http.Request) {
			var in workbench.Setting
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.SaveSetting(r.Context(), param(r, "pid"), in)
			reply(w, v, e)
		})
		r.Delete("/api/projects/{pid}/settings/{sid}", func(w http.ResponseWriter, r *http.Request) {
			noContent(w, s.Service.DeleteSetting(r.Context(), param(r, "pid"), param(r, "sid")))
		})
		r.Post("/api/projects/{pid}/chapters", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Title  string `json:"title"`
				Volume string `json:"volume"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.CreateChapter(r.Context(), param(r, "pid"), in.Title, in.Volume)
			reply(w, v, e)
		})
		r.Get("/api/chapters/{cid}", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.GetChapter(r.Context(), param(r, "cid"))
			reply(w, v, e)
		})
		r.Put("/api/chapters/{cid}", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Title     string `json:"title"`
				Volume    string `json:"volume"`
				Blueprint string `json:"blueprint"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.UpdateChapter(r.Context(), param(r, "cid"), in.Title, in.Volume, in.Blueprint)
			reply(w, v, e)
		})
		r.Delete("/api/chapters/{cid}", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.DeleteChapter(r.Context(), param(r, "cid"))
			reply(w, v, e)
		})
		r.Get("/api/chapters/{cid}/revisions", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.ListRevisions(r.Context(), param(r, "cid"))
			reply(w, v, e)
		})
		r.Get("/api/revisions/{rid}", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.GetRevision(r.Context(), param(r, "rid"))
			reply(w, v, e)
		})
		r.Post("/api/chapters/{cid}/revisions", func(w http.ResponseWriter, r *http.Request) {
			var in workbench.SaveInput
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.SaveRevision(r.Context(), param(r, "cid"), in)
			reply(w, v, e)
		})
		r.Post("/api/chapters/{cid}/restore", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				RevisionID string `json:"revisionId"`
				Expected   string `json:"expectedCurrentRevisionId"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.RestoreRevision(r.Context(), param(r, "cid"), in.RevisionID, in.Expected)
			reply(w, v, e)
		})
		r.Post("/api/chapters/{cid}/finalize", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				RevisionID string `json:"revisionId"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.Finalize(r.Context(), param(r, "cid"), in.RevisionID)
			reply(w, v, e)
		})
		r.Post("/api/chapters/{cid}/impact-preview", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.PreviewImpact(r.Context(), param(r, "cid"))
			reply(w, v, e)
		})
		r.Get("/api/projects/{pid}/impacts", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.ListImpacts(r.Context(), param(r, "pid"))
			reply(w, v, e)
		})
		r.Post("/api/projects/{pid}/chapter-order", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Epoch int64    `json:"expectedProjectEpoch"`
				IDs   []string `json:"chapterIds"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.ReorderChapters(r.Context(), param(r, "pid"), in.Epoch, in.IDs)
			reply(w, v, e)
		})
		r.Get("/api/projects/{pid}/state", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.State(r.Context(), param(r, "pid"))
			reply(w, v, e)
		})
		r.Get("/api/projects/{pid}/memory-proposals", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.ListProposals(r.Context(), param(r, "pid"))
			reply(w, v, e)
		})
		r.Get("/api/memory-proposals/{id}", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.GetProposal(r.Context(), param(r, "id"))
			reply(w, v, e)
		})
		r.Post("/api/chapters/{cid}/memory-proposals", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				SourceRevisionID string                   `json:"sourceRevisionId"`
				BaseStateID      string                   `json:"baseStateId"`
				Epoch            int64                    `json:"epoch"`
				Changes          []workbench.MemoryChange `json:"changes"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.CreateProposal(r.Context(), param(r, "cid"), in.SourceRevisionID, in.BaseStateID, in.Epoch, in.Changes)
			reply(w, v, e)
		})
		r.Post("/api/memory-proposals/{id}/decisions", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Decisions []workbench.MemoryDecision `json:"decisions"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.DecideProposal(r.Context(), param(r, "id"), in.Decisions)
			reply(w, v, e)
		})
		r.Post("/api/memory-proposals/{id}/apply", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				State    string `json:"expectedStateVersionId"`
				Revision string `json:"sourceRevisionId"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.ApplyProposal(r.Context(), param(r, "id"), in.State, in.Revision)
			reply(w, v, e)
		})
		r.Get("/api/projects/{pid}/rebuild", func(w http.ResponseWriter, r *http.Request) {
			p, e := s.Service.GetProject(r.Context(), param(r, "pid"))
			if e != nil {
				fail(w, e)
				return
			}
			v, e := s.Service.StartRebuild(r.Context(), param(r, "pid"), p.Project.Epoch)
			reply(w, v, e)
		})
		r.Post("/api/projects/{pid}/rebuild", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Epoch int64 `json:"expectedProjectEpoch"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.StartRebuild(r.Context(), param(r, "pid"), in.Epoch)
			reply(w, v, e)
		})
		r.Post("/api/projects/{pid}/reindex", func(w http.ResponseWriter, r *http.Request) {
			noContent(w, s.Service.Reindex(r.Context(), param(r, "pid")))
		})
		r.Get("/api/projects/{pid}/search", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			v, e := s.Service.Search(r.Context(), workbench.Query{ProjectID: param(r, "pid"), Text: q.Get("q"), BeforeChapterID: q.Get("beforeChapterId"), CharacterID: q.Get("characterId"), Purpose: q.Get("purpose"), Limit: 10})
			reply(w, v, e)
		})
		r.Post("/api/chapters/{cid}/runs", func(w http.ResponseWriter, r *http.Request) {
			var in workbench.RunRequest
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.SubmitRun(r.Context(), param(r, "cid"), in)
			reply(w, v, e)
		})
		r.Get("/api/projects/{pid}/runs", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.ListRuns(r.Context(), param(r, "pid"))
			reply(w, v, e)
		})
		r.Get("/api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.GetRun(r.Context(), param(r, "id"))
			reply(w, v, e)
		})
		r.Get("/api/runs/{id}/events", s.events)
		for _, action := range []string{"cancel", "pause", "resume", "retry", "regenerate"} {
			action := action
			r.Post("/api/runs/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
				v, e := s.Service.RunAction(r.Context(), param(r, "id"), action)
				reply(w, v, e)
			})
		}
		r.Get("/api/projects/{pid}/export", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.ExportProject(r.Context(), param(r, "pid"))
			if e != nil {
				fail(w, e)
				return
			}
			w.Header().Set("Content-Disposition", `attachment; filename="novel-project.json"`)
			respond(w, 200, v)
		})
		r.Get("/api/projects/{pid}/markdown", func(w http.ResponseWriter, r *http.Request) {
			v, e := s.Service.ExportMarkdown(r.Context(), param(r, "pid"))
			if e != nil {
				fail(w, e)
				return
			}
			if r.URL.Query().Get("format") == "txt" {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("Content-Disposition", `attachment; filename="novel.txt"`)
			} else {
				w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
				w.Header().Set("Content-Disposition", `attachment; filename="novel.md"`)
			}
			_, _ = io.WriteString(w, v)
		})
		r.Post("/api/import", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Package workbench.ProjectPackage `json:"package"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.ImportProject(r.Context(), in.Package)
			reply(w, v, e)
		})
		r.Post("/api/import/preview", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Text string `json:"text"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := workbench.PreviewTextImport(in.Text)
			reply(w, v, e)
		})
		r.Post("/api/projects/{pid}/import", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Chapters []workbench.ImportChapter `json:"chapters"`
			}
			if !decode(w, r, &in) {
				return
			}
			_, e := s.Service.ImportText(r.Context(), param(r, "pid"), in.Chapters)
			if e != nil {
				fail(w, e)
				return
			}
			v, e := s.Service.GetProject(r.Context(), param(r, "pid"))
			reply(w, v, e)
		})
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			respond(w, 404, map[string]string{"error": "接口不存在"})
			return
		}
		root := s.WebRoot
		if root == "" {
			root = "web/dist"
		}
		name := filepath.Clean(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/"))))
		absRoot, _ := filepath.Abs(root)
		absName, _ := filepath.Abs(name)
		if !strings.HasPrefix(absName, absRoot+string(os.PathSeparator)) && absName != absRoot {
			http.NotFound(w, r)
			return
		}
		info, err := os.Stat(name)
		if err != nil || info.IsDir() {
			name = filepath.Join(root, "index.html")
		}
		http.ServeFile(w, r, name)
	})
	return r
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if _, err := s.Service.GetRun(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	flush, ok := w.(http.Flusher)
	if !ok {
		fail(w, fmt.Errorf("不支持事件流"))
		return
	}
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if after == 0 {
		after, _ = strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		events, err := s.Service.Events(r.Context(), id, after)
		if err != nil {
			return
		}
		for _, event := range events {
			raw, _ := json.Marshal(event)
			if _, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.ID, raw); err != nil {
				return
			}
			after = event.ID
		}
		_, _ = fmt.Fprint(w, ": keepalive\n\n")
		flush.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
