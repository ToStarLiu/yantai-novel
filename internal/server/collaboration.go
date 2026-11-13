package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"novel/internal/workbench"
	"strconv"
	"strings"
	"time"
)

func readRequestBody(r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (34<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 34<<20 {
		return nil, fmt.Errorf("请求超过大小限制")
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, nil
}

func (s *Server) registerCollaborationPublic(r chi.Router) {
	r.Post("/api/invitations/accept", func(w http.ResponseWriter, r *http.Request) {
		if !s.Auth.validOrigin(r) {
			respond(w, 403, map[string]string{"error": "来源校验失败"})
			return
		}
		var in struct {
			Token       string `json:"token"`
			Username    string `json:"username"`
			Password    string `json:"password"`
			DisplayName string `json:"displayName"`
		}
		if !decode(w, r, &in) {
			return
		}
		if strings.EqualFold(strings.TrimSpace(in.Username), s.Auth.Username) {
			respond(w, 400, map[string]string{"error": "该用户名保留给初始管理员"})
			return
		}
		v, e := s.Service.AcceptInvitation(r.Context(), in.Token, in.Username, in.Password, in.DisplayName)
		reply(w, v, e)
	})
}
func (s *Server) registerCollaboration(r chi.Router) {
	r.Get("/api/admin/users", func(w http.ResponseWriter, r *http.Request) { v, e := s.Service.Users(r.Context()); reply(w, v, e) })
	r.Post("/api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Username    string `json:"username"`
			Password    string `json:"password"`
			DisplayName string `json:"displayName"`
		}
		if !decode(w, r, &in) {
			return
		}
		if strings.EqualFold(strings.TrimSpace(in.Username), s.Auth.Username) {
			respond(w, 400, map[string]string{"error": "该用户名保留给初始管理员"})
			return
		}
		v, e := s.Service.CreateUser(r.Context(), in.Username, in.Password, in.DisplayName)
		reply(w, v, e)
	})
	base := "/api/projects/{pid}"
	r.Get(base+"/archive", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ProjectArchive(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Get(base+"/access", func(w http.ResponseWriter, r *http.Request) {
		role, e := s.Service.ProjectRole(r.Context(), param(r, "pid"))
		if e != nil {
			fail(w, e)
			return
		}
		p, _ := workbench.PrincipalFromContext(r.Context())
		respond(w, 200, map[string]any{"role": role, "user": p})
	})
	r.Get(base+"/members", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.Members(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post(base+"/members", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.AddMember(r.Context(), param(r, "pid"), in.Username, in.Role)
		reply(w, v, e)
	})
	r.Put(base+"/members/{uid}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Role string `json:"role"`
		}
		if !decode(w, r, &in) {
			return
		}
		noContent(w, s.Service.ChangeMember(r.Context(), param(r, "pid"), param(r, "uid"), in.Role, false))
	})
	r.Delete(base+"/members/{uid}", func(w http.ResponseWriter, r *http.Request) {
		noContent(w, s.Service.ChangeMember(r.Context(), param(r, "pid"), param(r, "uid"), "", true))
	})
	r.Get(base+"/invitations", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.Invitations(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post(base+"/invitations", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Role string `json:"role"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.CreateInvitation(r.Context(), param(r, "pid"), in.Role)
		reply(w, v, e)
	})
	r.Delete(base+"/invitations/{id}", func(w http.ResponseWriter, r *http.Request) {
		noContent(w, s.Service.RevokeInvitation(r.Context(), param(r, "pid"), param(r, "id")))
	})
	r.Get(base+"/presence", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.Presence(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post(base+"/presence", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.PresenceInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.UpdatePresence(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Get(base+"/changes", s.collaborationEvents)
}

// Resolve ownership before handlers load any body, stream, or task output.
// This central gate also covers all nested Studio, media and research routes.
func (s *Server) authorizeRequest(r *http.Request) error {
	p, _ := workbench.PrincipalFromContext(r.Context())
	if p.Admin {
		return nil
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "api" {
		return workbench.ErrForbidden
	}
	write := r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS"
	top := parts[1]
	// Copilot sessions are private to their creator; their own service gate
	// checks the current project membership on every read, mutation and SSE tick.
	if top == "copilot" {
		return nil
	}
	if top == "admin" || top == "metrics" || top == "provider-configs" || top == "provider-settings" || top == "embedding-configs" || strings.HasPrefix(top, "mcp") {
		return workbench.ErrForbidden
	}
	var pid string
	var err error
	if top == "projects" && len(parts) >= 3 {
		pid = parts[2]
	} else if len(parts) >= 3 {
		id := parts[2]
		query := ""
		switch top {
		case "chapters":
			query = "SELECT project_id FROM wb_chapters WHERE id=$1"
		case "revisions":
			query = "SELECT c.project_id FROM wb_revisions r JOIN wb_chapters c ON c.id=r.chapter_id WHERE r.id=$1"
		case "runs":
			query = "SELECT project_id FROM wb_runs WHERE id=$1"
		case "memory-proposals":
			query = "SELECT project_id FROM wb_proposals WHERE id=$1"
		case "productions":
			query = "SELECT project_id FROM wb_productions WHERE id=$1"
		case "context-summaries":
			query = "SELECT project_id FROM wb_context_summaries WHERE id=$1"
		}
		if query != "" {
			err = s.Service.DB.QueryRow(r.Context(), query, id).Scan(&pid)
			if errors.Is(err, pgx.ErrNoRows) {
				return workbench.ErrNotFound
			}
			if err != nil {
				return err
			}
		}
	}
	if pid == "" {
		switch top {
		case "session", "config", "style-templates", "writing-skill-catalog":
			return nil
		case "projects":
			if len(parts) == 2 {
				return nil
			}
		case "import":
			return nil
		case "workflows", "prompt-templates", "agents", "media":
			if !write {
				return nil
			}
		}
		return workbench.ErrForbidden
	}
	minimum := "viewer"
	if len(parts) >= 4 && top == "projects" && parts[3] == "invitations" {
		minimum = "owner"
	}
	if write {
		minimum = "editor"
		if top == "projects" && len(parts) >= 4 && parts[3] == "copilot" {
			minimum = "viewer"
		}
		if len(parts) >= 4 && (parts[3] == "members" || parts[3] == "invitations") {
			minimum = "owner"
		}
		if top == "projects" && len(parts) == 3 && r.Method == "DELETE" {
			minimum = "owner"
		}
		if top == "projects" && len(parts) >= 4 && parts[3] == "presence" {
			minimum = "viewer"
		}
		if strings.Contains(r.URL.Path, "/studio/comments") {
			minimum = "reviewer"
		}
		if strings.HasSuffix(r.URL.Path, "/hook-review") {
			minimum = "reviewer"
		}
		if strings.HasSuffix(r.URL.Path, "/impact-preview") || strings.HasSuffix(r.URL.Path, "/semantic-search") || strings.HasSuffix(r.URL.Path, "/board/evaluate") {
			minimum = "viewer"
		}
	}
	// Reviewer task submission is parsed by the existing decoder after preserving
	// its body here. Reviewers may never use planning/draft/revision to change prose.
	if write && top == "chapters" && len(parts) == 4 && parts[3] == "runs" {
		role, e := s.Service.ProjectRole(r.Context(), pid)
		if e != nil {
			return e
		}
		if role == "reviewer" {
			var in struct {
				TaskType string `json:"taskType"`
			}
			raw, e := readRequestBody(r)
			if e != nil {
				return e
			}
			if json.Unmarshal(raw, &in) != nil || in.TaskType != "review" {
				return workbench.ErrForbidden
			}
			minimum = "reviewer"
		}
	}
	if write && top == "runs" {
		role, e := s.Service.ProjectRole(r.Context(), pid)
		if e != nil {
			return e
		}
		if role == "reviewer" {
			var typ string
			if e = s.Service.DB.QueryRow(r.Context(), "SELECT type FROM wb_runs WHERE id=$1", parts[2]).Scan(&typ); e != nil {
				return e
			}
			if typ == "review" {
				minimum = "reviewer"
			}
		}
	}
	return s.Service.RequireProjectRole(r.Context(), pid, minimum)
}

func (s *Server) collaborationEvents(w http.ResponseWriter, r *http.Request) {
	pid := param(r, "pid")
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if v, e := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); e == nil && v > after {
		after = v
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		respond(w, 500, map[string]string{"error": "流式响应不可用"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if _, ok = s.session(r); !ok {
			return
		}
		events, err := s.Service.CollaborationEvents(r.Context(), pid, after)
		if err != nil {
			return
		}
		for _, event := range events {
			raw, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "id: %d\nevent: change\ndata: %s\n\n", event.ID, raw)
			after = event.ID
		}
		if len(events) > 0 {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
