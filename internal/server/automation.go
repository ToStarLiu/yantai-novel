package server

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"novel/internal/workbench"
)

func (s *Server) registerAutomation(r chi.Router) {
	r.Get("/api/workflows", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListWorkflows(r.Context())
		reply(w, v, e)
	})
	r.Post("/api/workflows", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.WorkflowDefinition
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveWorkflow(r.Context(), in)
		reply(w, v, e)
	})
	r.Get("/api/projects/{pid}/productions", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListProductions(r.Context(), param(r, "pid"))
		reply(w, v, e)
	})
	r.Post("/api/projects/{pid}/productions", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.ProductionRequest
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.StartProduction(r.Context(), param(r, "pid"), in)
		reply(w, v, e)
	})
	r.Get("/api/productions/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.GetProduction(r.Context(), param(r, "id"))
		reply(w, v, e)
	})
	for _, action := range []string{"pause", "resume", "cancel", "approve"} {
		action := action
		r.Post("/api/productions/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				StepID string `json:"stepId"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, e := s.Service.ProductionAction(r.Context(), param(r, "id"), action, in.StepID)
			reply(w, v, e)
		})
	}
	r.Get("/api/provider-configs", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListProviderConfigs(r.Context())
		reply(w, v, e)
	})
	r.Post("/api/provider-configs", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.ProviderConfig
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveProviderConfig(r.Context(), in)
		reply(w, v, e)
	})
	r.Post("/api/provider-configs/{id}/health", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.CheckProviderHealth(r.Context(), param(r, "id"))
		reply(w, v, e)
	})
	r.Put("/api/provider-configs/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &in) {
			return
		}
		noContent(w, s.Service.SetProviderEnabled(r.Context(), param(r, "id"), in.Enabled))
	})
	r.Get("/api/prompt-templates", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListPromptTemplates(r.Context())
		reply(w, v, e)
	})
	r.Post("/api/prompt-templates", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.PromptTemplate
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SavePromptTemplate(r.Context(), in)
		reply(w, v, e)
	})
	r.Get("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListAgents(r.Context())
		reply(w, v, e)
	})
	r.Post("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.AgentDefinition
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveAgent(r.Context(), in)
		reply(w, v, e)
	})
	r.Get("/api/embedding-configs", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.ListEmbeddingConfigs(r.Context())
		reply(w, v, e)
	})
	r.Post("/api/embedding-configs", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.EmbeddingConfig
		if !decode(w, r, &in) {
			return
		}
		v, e := s.Service.SaveEmbeddingConfig(r.Context(), in)
		reply(w, v, e)
	})
	r.Post("/api/projects/{pid}/embeddings/rebuild", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ConfigID string `json:"configId"`
		}
		if !decode(w, r, &in) {
			return
		}
		count, e := s.Service.RebuildEmbeddings(r.Context(), param(r, "pid"), in.ConfigID)
		reply(w, map[string]int{"indexed": count}, e)
	})
	r.Post("/api/projects/{pid}/semantic-search", func(w http.ResponseWriter, r *http.Request) {
		var in workbench.SemanticQuery
		if !decode(w, r, &in) {
			return
		}
		in.ProjectID = param(r, "pid")
		v, e := s.Service.SemanticSearch(r.Context(), in)
		reply(w, v, e)
	})
	r.Post("/api/projects/{pid}/context-compression", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Query    workbench.Query `json:"query"`
			MaxRunes int             `json:"maxRunes"`
		}
		if !decode(w, r, &in) {
			return
		}
		in.Query.ProjectID = param(r, "pid")
		v, e := s.Service.CompressContext(r.Context(), in.Query, in.MaxRunes)
		reply(w, v, e)
	})
	r.Get("/api/context-summaries/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.GetContextSummary(r.Context(), param(r, "id"))
		reply(w, v, e)
	})
	r.Get("/api/metrics", func(w http.ResponseWriter, r *http.Request) { v, e := s.Service.Metrics(r.Context()); reply(w, v, e) })
}
