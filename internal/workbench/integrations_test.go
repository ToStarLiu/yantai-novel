package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"novel/internal/job"
	"novel/internal/llm"
)

func TestProviderConfigSecretReferenceHealthAndNativeOllama(t *testing.T) {
	s, _ := runtimeFixture(t)
	ctx := context.Background()
	t.Setenv("TEST_PROVIDER_SECRET", "not-returned-secret")
	gets, posts := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets++
			if r.URL.Path != "/api/tags" {
				t.Error(r.URL.Path)
			}
			fmt.Fprint(w, `{"models":[]}`)
			return
		}
		posts++
		if r.URL.Path != "/api/chat" {
			t.Error(r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] != false || body["model"] != "local-model" {
			t.Error(body)
		}
		fmt.Fprint(w, `{"done":true,"message":{"content":"本地候选"},"prompt_eval_count":12,"eval_count":4}`)
	}))
	defer server.Close()
	config, err := s.SaveProviderConfig(ctx, ProviderConfig{Name: "本地原生", Protocol: "ollama", BaseURL: server.URL, APIKeyEnv: "TEST_PROVIDER_SECRET", Model: "local-model", TaskTypes: []string{"draft"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListProviderConfigs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "not-returned-secret") {
		t.Fatal("secret value returned to UI")
	}
	health, err := s.CheckProviderHealth(ctx, config.ID)
	if err != nil || !health.Reachable || gets != 1 || posts != 0 {
		t.Fatalf("health billed a generation %#v %v", health, err)
	}
	provider, policy, id, err := s.ResolveTaskProvider(ctx, "draft")
	if err != nil || policy.Model != "local-model" || id != config.ID {
		t.Fatalf("route %#v %v", policy, err)
	}
	result, err := provider.Generate(ctx, llm.Request{Model: policy.Model, Messages: []llm.Message{{Role: "user", Content: "起草"}}, MaxOutputTokens: 500})
	if err != nil || result.Text != "本地候选" || !result.Usage.Reported {
		t.Fatalf("native response %#v %v", result, err)
	}
	if _, err = s.SaveProviderConfig(ctx, ProviderConfig{Name: "明文密钥", Protocol: "chat", BaseURL: server.URL, APIKeyEnv: "sk-secret-invalid!", Model: "m"}); err == nil {
		t.Fatal("secret string accepted instead of env reference")
	}
}

func TestVersionedPromptAgentAndCacheInvalidation(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	prompt, err := s.SavePromptTemplate(ctx, PromptTemplate{Name: "审阅读者", TaskType: "reader", Version: 1, Prompt: "按原文给首读反馈", Rules: []string{"不改正文"}})
	if err != nil {
		t.Fatal(err)
	}
	newVersion, err := s.SavePromptTemplate(ctx, prompt)
	if err != nil || newVersion.ID == prompt.ID || newVersion.Version != 2 {
		t.Fatalf("editing prompt did not preserve immutable version %#v %v", newVersion, err)
	}
	duplicate := prompt
	duplicate.ID = ""
	if _, err = s.SavePromptTemplate(ctx, duplicate); !IsConflict(err) {
		t.Fatalf("duplicate prompt version should be conflict %v", err)
	}
	agent, err := s.SaveAgent(ctx, AgentDefinition{Name: "首读者", TaskType: "reader", PromptTemplateID: prompt.ID, Tools: []string{"read-history"}, MaxOutputTokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetAgent(ctx, agent.ID); err != nil || got.PromptTemplateID != prompt.ID {
		t.Fatalf("agent lost prompt %#v %v", got, err)
	}
	run := submit(t, s, c, "draft")
	if err = s.PutRunCache(ctx, run.Input, RunOutput{Text: "缓存候选"}); err != nil {
		t.Fatal(err)
	}
	cached, ok, err := s.GetRunCache(ctx, run.Input)
	if err != nil || !ok || cached.Text != "缓存候选" {
		t.Fatalf("cache failed %#v %v %v", cached, ok, err)
	}
	if _, err = s.UpdateChapter(ctx, c.ID, c.Title, "", "章纲已改变"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err = s.GetRunCache(ctx, run.Input); err != nil || ok {
		t.Fatalf("stale cache returned %v %v", ok, err)
	}
}

func TestConfiguredProviderWorkerRouteAndExplicitCacheAvoidsSecondCall(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	t.Setenv("TEST_CHAT_SECRET", "local-secret")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request["model"] != "second-model" || r.Header.Get("Authorization") != "Bearer local-secret" {
			t.Error("incorrect frozen provider route")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"实际第二Provider候选"}}],"usage":{"prompt_tokens":20,"completion_tokens":4}}`)
	}))
	defer server.Close()
	config, err := s.SaveProviderConfig(ctx, ProviderConfig{Name: "第二Provider", Protocol: "chat", BaseURL: server.URL, APIKeyEnv: "TEST_CHAT_SECRET", Model: "second-model", TaskTypes: []string{"draft"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: "cache-one", TaskType: "draft", UseCache: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.Input.Provider != config.ID || first.Input.Model != "second-model" {
		t.Fatal("configured provider route not frozen")
	}
	claimed := claim(t, s)
	if err = s.ExecuteRun(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	second, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: "cache-two", TaskType: "draft", UseCache: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ExecuteRun(ctx, claim(t, s)); err != nil {
		t.Fatal(err)
	}
	cached, err := s.GetRun(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !cached.Output.Cached || cached.AttemptCount != 0 || cached.Cost == nil || *cached.Cost != 0 || calls != 1 {
		t.Fatalf("explicit cache made a second request %#v calls=%d", cached, calls)
	}
	// Default author regeneration remains a new paid intention, never implicit cache.
	third, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: "cache-opt-out", TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ExecuteRun(ctx, claim(t, s)); err != nil {
		t.Fatal(err)
	}
	uncached, err := s.GetRun(ctx, third.ID)
	if err != nil || uncached.Output.Cached || calls != 2 {
		t.Fatalf("cache opt out ignored %#v calls=%d %v", uncached, calls, err)
	}
}

func TestAgentPermissionsRemoveUnapprovedContextAndRetainEntityIdentifiers(t *testing.T) {
	source := RunInput{Source: "作者本次明确选中的稿件", ContextSummaryID: "derived-summary", ContextSummary: &ContextSummary{Summary: "派生历史秘密", Sources: []Citation{{Content: "派生原文秘密"}}}, Settings: []Setting{{ID: "person", Content: "隐藏设定"}}, Previous: "历史正文", Memory: "作者真相", StudioContext: json.RawMessage(`{"future":"秘密计划"}`), Citations: []Citation{{Content: "秘密历史"}}, MemoryContext: &MemoryContext{EntityIDs: []string{"person"}, Entities: []Setting{{ID: "person", Kind: "character", Name: "林川", Content: "秘密身世"}}, Facts: []Fact{{Object: "角色秘密"}}}}
	ApplyAgentPermissions(&source, AgentDefinition{Tools: []string{}})
	raw, _ := json.Marshal(source)
	for _, secret := range []string{"隐藏设定", "历史正文", "作者真相", "秘密计划", "秘密历史", "秘密身世", "角色秘密", "派生历史秘密", "派生原文秘密", "derived-summary"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("unapproved context %s leaked", secret)
		}
	}
	if source.Source != "作者本次明确选中的稿件" || len(source.MemoryContext.EntityIDs) != 1 || source.MemoryContext.Entities[0].Name != "林川" {
		t.Fatal("explicit input or identity metadata lost")
	}
	preserved := RunInput{Settings: []Setting{{Content: "作者设定"}}, Previous: "前文", Memory: "记忆", StudioContext: json.RawMessage(`{"outline":"规划"}`)}
	ApplyAgentPermissions(&preserved, AgentDefinition{Tools: []string{"read-settings", "read-history", "query-memory"}})
	if len(preserved.Settings) != 1 || preserved.Previous == "" || preserved.Memory == "" || len(preserved.StudioContext) == 0 {
		t.Fatal("granted context discarded")
	}
	knowledgeSummary := RunInput{ContextSummaryID: "knowledge-summary", ContextSummary: &ContextSummary{Summary: "已确认角色知识", Sources: []Citation{{Kind: "Knowledge"}}}}
	ApplyAgentPermissions(&knowledgeSummary, AgentDefinition{Tools: []string{"read-history"}})
	if knowledgeSummary.ContextSummary != nil || knowledgeSummary.ContextSummaryID != "" {
		t.Fatal("query-memory permission bypassed by derived knowledge summary")
	}
}

func TestRestrictedAgentExplicitCanvasSelectionKeepsOnlySelectedPlanningMaterial(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	_, err := s.SaveStudioBoard(ctx, c.ProjectID, StudioWrite[StudioBoard]{Data: StudioBoard{Nodes: []StudioNode{{ID: "chosen", Type: "plot", Label: "作者明确选择", Content: "只选择的场景材料"}, {ID: "hidden", Type: "plot", Label: "未选择的未来真相", Content: "隐藏终章计划"}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveSetting(ctx, c.ProjectID, Setting{Kind: "world", Name: "秘密世界设定", Content: "未授权世界真相"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := s.SaveAgent(ctx, AgentDefinition{Name: "受限关系分析", TaskType: "analysis", Tools: []string{}, MaxOutputTokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: "explicit-canvas-permission", TaskType: "analysis", AgentID: agent.ID, CanvasNodeIDs: []string{"chosen"}, CanvasBoardVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(run.Input.Request)
	for _, secret := range []string{"未选择的未来真相", "隐藏终章计划", "秘密世界设定", "未授权世界真相"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("unselected planning context leaked", secret)
		}
	}
	if !strings.Contains(string(raw), "只选择的场景材料") {
		t.Fatal("author explicitly selected source was removed")
	}
}

func TestConfiguredProviderPricesEnableBudgetsAndFreezeWithRun(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	// Database-managed providers never borrow another endpoint's price merely
	// because its model happens to use the same name.
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: "local-priced", Currency: "USD", InputPricePerMillion: ptr(999), OutputPricePerMillion: ptr(999)}}
	base := ProviderConfig{Name: "自定义模型计价", Protocol: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "local-priced", TaskTypes: []string{"draft"}, Enabled: true, IsDefault: true}
	unpriced, err := s.SaveProviderConfig(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	_, policy, _, err := s.ResolveTaskProvider(ctx, "draft")
	if err != nil || policy.InputPricePerMillion != nil || policy.OutputPricePerMillion != nil || policy.Currency != "" {
		t.Fatalf("unpriced endpoint borrowed policy %#v %v", policy, err)
	}
	if _, err = s.SavePreferences(ctx, c.ProjectID, Preferences{MaxCost: ptr(1), Currency: "USD", MaxOutputTokens: 500}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"}); err == nil {
		t.Fatal("unpriced configured provider bypassed budget")
	}
	if err = s.SetProviderEnabled(ctx, unpriced.ID, false); err != nil {
		t.Fatal(err)
	}
	base.Currency = "USD"
	base.InputPricePerMillion = ptr(2)
	base.OutputPricePerMillion = ptr(4)
	priced, err := s.SaveProviderConfig(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal("UI configured price could not satisfy budget", err)
	}
	if run.Input.Provider != priced.ID || run.Input.Policy.Currency != "USD" || run.Input.Policy.InputPricePerMillion == nil || *run.Input.Policy.InputPricePerMillion != 2 || run.Input.Policy.OutputPricePerMillion == nil || *run.Input.Policy.OutputPricePerMillion != 4 || requestEstimate(run.Input) == nil {
		t.Fatalf("price snapshot absent %#v", run.Input.Policy)
	}
	base.InputPricePerMillion = ptr(20)
	if _, err = s.SaveProviderConfig(ctx, base); err != nil {
		t.Fatal(err)
	}
	old, err := s.GetRun(ctx, run.ID)
	if err != nil || *old.Input.Policy.InputPricePerMillion != 2 {
		t.Fatal("new provider price mutated queued policy", err)
	}
	_, policy, _, err = s.ResolveTaskProvider(ctx, "draft")
	if err != nil || *policy.InputPricePerMillion != 20 {
		t.Fatal("new configured route price not used", err)
	}
	for _, price := range []float64{-1, math.NaN(), math.Inf(1)} {
		bad := base
		bad.InputPricePerMillion = ptr(price)
		if _, err = s.SaveProviderConfig(ctx, bad); err == nil {
			t.Fatal("invalid price accepted", price)
		}
	}
	zero := base
	zero.Name = "免费本地模型"
	zero.InputPricePerMillion = ptr(0)
	zero.OutputPricePerMillion = ptr(0)
	if _, err = s.SaveProviderConfig(ctx, zero); err != nil {
		t.Fatal("zero cost rejected", err)
	}
	_, policy, _, err = s.ResolveTaskProvider(ctx, "draft")
	if err != nil || policy.InputPricePerMillion == nil || *policy.InputPricePerMillion != 0 {
		t.Fatal("zero price became unknown", err)
	}
	configs, err := s.ListProviderConfigs(ctx)
	if err != nil || len(configs) != 4 {
		t.Fatalf("config prices could not be read %#v %v", configs, err)
	}
}
