package server

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"novel/internal/workbench"
	"strconv"
	"time"
)

func (s *Server) registerCopilot(r chi.Router) {
	r.Post("/api/copilot/book-plans/{id}/apply", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ExpectedVersion int64 `json:"expectedVersion"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.ApplyCopilotBookPlan(r.Context(), param(r, "id"), in.ExpectedVersion)
		reply(w, v, e)
	})
	r.Post("/api/copilot/blueprints/{id}/apply", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ExpectedVersion int64 `json:"expectedVersion"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.ApplyCopilotBlueprint(r.Context(), param(r, "id"), in.ExpectedVersion)
		reply(w, v, e)
	})
	base := "/api/projects/{pid}/copilot/sessions"
	r.Get(base, func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListCopilotSessions(r.Context(), param(r, "pid"), r.URL.Query().Get("scope"))
		reply(w, v, e)
	})
	r.Post(base, func(w http.ResponseWriter, r *http.Request) {
		var in workbench.CopilotSessionInput
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.CreateCopilotSession(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Get("/api/copilot/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.GetCopilotSession(r.Context(), param(r, "id"))
		reply(w, v, e)
	})
	r.Post("/api/copilot/sessions/{id}/turns", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.CopilotTurnRequest
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SubmitCopilotTurn(r.Context(), param(r, "id"), in)
		reply(w, v, e)
	})
	r.Get("/api/copilot/turns/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.GetCopilotTurn(r.Context(), param(r, "id"))
		reply(w, v, e)
	})
	r.Get("/api/copilot/turns/{id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.CopilotAttempts(r.Context(), param(r, "id"))
		reply(w, v, e)
	})
	r.Post("/api/copilot/turns/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.CancelCopilotTurn(r.Context(), param(r, "id"))
		reply(w, v, e)
	})
	r.Get("/api/copilot/turns/{id}/events", s.copilotEvents)
}
func (s *Server) copilotEvents(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if _, e := s.Service.GetCopilotTurn(r.Context(), id); e != nil {
		fail(w, e)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if v, e := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); e == nil && v > after {
		after = v
	}
	if r.URL.Query().Get("format") == "json" {
		v, e := s.Service.CopilotEvents(r.Context(), id, after)
		reply(w, v, e)
		return
	}
	flush, ok := w.(http.Flusher)
	if !ok {
		fail(w, fmt.Errorf("不支持事件流"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flush.Flush()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, ok := s.session(r); !ok {
			return
		}
		events, e := s.Service.CopilotEvents(r.Context(), id, after)
		if e != nil {
			return
		}
		for _, event := range events {
			raw, _ := json.Marshal(event)
			if _, e = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Kind, raw); e != nil {
				return
			}
			after = event.ID
		}
		if len(events) > 0 {
			flush.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
