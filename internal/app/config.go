package app

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"novel/internal/llm"
	"novel/internal/workbench"
	"os"
	"strconv"
	"strings"
	"time"
)

func Service(ctx context.Context) (*workbench.Service, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	if err = db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("database unavailable")
	}
	providerName, err := environmentProviderMode()
	if err != nil {
		db.Close()
		return nil, err
	}
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))
	var provider llm.Provider
	if providerName == "fake" {
		model = "fake-v1"
		provider = &llm.Fake{Handler: func(ctx context.Context, r llm.Request) (llm.Response, error) {
			text := "雨落在窗沿。她把铜印留在桌角，先将尚未厘清的线索写入册中。故事在此留住片刻，等待作者继续。"
			if len(r.ResponseSchema) > 0 {
				if strings.Contains(string(r.ResponseSchema), "changes") {
					text = `{"changes":[]}`
				} else {
					text = `{"issues":[]}`
				}
			}
			select {
			case <-ctx.Done():
				return llm.Response{}, ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
			return llm.Response{Text: text, Raw: []byte(text), ProviderRequestID: "fake-offline"}, nil
		}}
	} else {
		headers := map[string]string{}
		if h := os.Getenv("LLM_ACTOR_AUTHORIZATION"); h != "" {
			headers["x-openai-actor-authorization"] = h
		}
		provider, err = llm.NewCompatible(llm.Config{BaseURL: os.Getenv("LLM_BASE_URL"), APIKey: os.Getenv("LLM_API_KEY"), Model: model, WireAPI: os.Getenv("LLM_WIRE_API"), Headers: headers, Timeout: 120 * time.Second, MaxResponseBytes: 4 << 20})
		if err != nil {
			db.Close()
			return nil, err
		}
	}
	max := 3000
	if v, e := strconv.Atoi(os.Getenv("MAX_OUTPUT_TOKENS")); e == nil && v > 0 && v <= 12000 {
		max = v
	}
	limit := 2
	if v, e := strconv.Atoi(os.Getenv("MAX_CONCURRENT_JOBS")); e == nil && v > 0 && v <= 8 {
		limit = v
	}
	currency := os.Getenv("LLM_CURRENCY")
	basePolicy := workbench.ModelPolicy{Model: model, Currency: currency, InputPricePerMillion: price("LLM_INPUT_PRICE_PER_MILLION"), OutputPricePerMillion: price("LLM_OUTPUT_PRICE_PER_MILLION")}
	policies := map[string]workbench.ModelPolicy{"default": basePolicy}
	for kind, env := range map[string]string{"review": "LLM_REVIEW_MODEL", "memory-extraction": "LLM_MEMORY_MODEL", "planning": "LLM_PLANNING_MODEL"} {
		if selected := os.Getenv(env); selected != "" {
			p := basePolicy
			p.Model = selected
			if selected != model {
				prefix := strings.TrimSuffix(env, "_MODEL")
				p.InputPricePerMillion = price(prefix + "_INPUT_PRICE_PER_MILLION")
				p.OutputPricePerMillion = price(prefix + "_OUTPUT_PRICE_PER_MILLION")
			}
			policies[kind] = p
		}
	}
	return &workbench.Service{DB: db, Provider: provider, Model: model, ProviderName: providerName, MaxOutputTokens: max, MaxConcurrent: limit, JobTimeout: 150 * time.Second, ModelPolicies: policies, MaxJobCost: price("MAX_JOB_COST"), Currency: currency}, nil
}

func price(name string) *float64 {
	if value, err := strconv.ParseFloat(os.Getenv(name), 64); err == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
		return &value
	}
	return nil
}

// Empty installations may open the settings page before choosing a model. A
// partially supplied connection is always an error, never silently a demo.
func environmentProviderMode() (string, error) {
	mode := strings.TrimSpace(os.Getenv("APP_PROVIDER"))
	if mode == "" {
		mode = "auto"
	}
	if mode != "auto" && mode != "fake" && mode != "configured" {
		return "", fmt.Errorf("APP_PROVIDER must be auto, configured or fake")
	}
	if mode == "fake" {
		return "fake", nil
	}
	base := strings.TrimSpace(os.Getenv("LLM_BASE_URL"))
	key := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))
	if mode == "auto" && base == "" && key == "" && model == "" {
		return "fake", nil
	}
	if base == "" || key == "" || model == "" {
		return "", fmt.Errorf("model connection is incomplete: configure LLM_BASE_URL, LLM_API_KEY and LLM_MODEL together, or leave all three empty and choose the model in AI settings")
	}
	return "configured", nil
}
