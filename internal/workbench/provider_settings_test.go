package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"novel/internal/llm"
)

func providerSettingsFixture(t *testing.T) (*Service, context.Context) {
	t.Helper()
	t.Setenv("PROVIDER_MASTER_KEY_PATH", filepath.Join(t.TempDir(), "provider-master.key"))
	s, _ := runtimeFixture(t)
	return s, copilotAdmin()
}

func TestProviderSettingsEncryptWriteOnlyKeysAndHeadersAndResolveBothRuntimes(t *testing.T) {
	s, ctx := providerSettingsFixture(t)
	const key = "secret-api-value-not-in-database"
	const header = "secret-actor-value-not-in-database"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("x-openai-actor-authorization") != header {
			t.Error("encrypted credentials were not resolved")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			fmt.Fprint(w, `{"data":[{"id":"model-b"},{"id":"model-a","name":"A"}]}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"实际模型输出"}}],"usage":{"prompt_tokens":12,"completion_tokens":5}}`)
	}))
	defer server.Close()
	input := ProviderSettingsInput{ProviderConfig: ProviderConfig{Name: "页面自定义", Protocol: "chat", BaseURL: server.URL, Model: "model-a", Enabled: true, IsDefault: true, Headers: map[string]string{"x-openai-actor-authorization": header}}, APIKey: key}
	saved, err := s.SaveProviderSetting(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.HasKey || saved.CredentialSource != "stored" || saved.APIKeyEnv != "" || len(saved.StoredHeaderNames) != 1 {
		t.Fatalf("public credentials metadata wrong: %#v", saved)
	}
	settings, err := s.ProviderSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(settings)
	if bytes.Contains(raw, []byte(key)) || bytes.Contains(raw, []byte(header)) || bytes.Contains(raw, []byte("secret:")) {
		t.Fatal("secret or storage reference leaked to UI")
	}
	var rows []byte
	if err = s.DB.QueryRow(ctx, "SELECT json_agg(to_jsonb(c))::text FROM wb_provider_configs c").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rows, []byte(key)) || bytes.Contains(rows, []byte(header)) {
		t.Fatal("plaintext config stored")
	}
	var count int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_provider_secrets WHERE position($1::bytea in ciphertext)>0 OR position($2::bytea in ciphertext)>0", []byte(key), []byte(header)).Scan(&count); err != nil || count != 0 {
		t.Fatal("secret plaintext in ciphertext", err)
	}
	provider, policy, id, err := s.ResolveTaskProvider(ctx, "copilot")
	if err != nil || id != saved.ID || policy.Model != "model-a" {
		t.Fatal(err)
	}
	response, err := provider.Generate(ctx, llm.Request{Model: policy.Model, Messages: []llm.Message{{Role: "user", Content: "实际请求"}}, MaxOutputTokens: 100})
	if err != nil || response.Text != "实际模型输出" {
		t.Fatal(err, response)
	}
	model, err := s.copilotModel(ctx, CopilotInput{ProviderID: saved.ID, Policy: policy, MaxOutputTokens: 100})
	if err != nil || model.APIKey != key || model.Headers["x-openai-actor-authorization"] != header {
		t.Fatal("Pi connection missing actual secret", err)
	}
	discovery, err := s.DiscoverProviderModels(ctx, ProviderSettingsInput{ProviderConfig: saved.ProviderConfig})
	if err != nil || !discovery.Reachable || len(discovery.Models) != 2 || discovery.Models[0].ID != "model-a" {
		t.Fatal("discovery failed", discovery, err)
	}
	health, err := s.CheckProviderHealth(ctx, saved.ID)
	if err != nil || !health.Reachable || !health.CredentialsConfigured || calls != 3 {
		t.Fatal("health did not use real credentials", health, err, calls)
	}
}

func TestProviderSettingsEditsKeepImmutableCredentialsAndRejectImplicitEndpointChange(t *testing.T) {
	s, ctx := providerSettingsFixture(t)
	first, err := s.SaveProviderSetting(ctx, ProviderSettingsInput{ProviderConfig: ProviderConfig{Name: "我的模型", Protocol: "responses", BaseURL: "https://model.example/v1", Model: "old-model", Enabled: true, IsDefault: true, Headers: map[string]string{"x-api-key": "header-secret"}}, APIKey: "api-secret"})
	if err != nil {
		t.Fatal(err)
	}
	edit := ProviderSettingsInput{ProviderConfig: first.ProviderConfig}
	edit.Model = "new-model"
	edit.Headers = map[string]string{}
	second, err := s.SaveProviderSetting(ctx, edit)
	if err != nil || second.ID == first.ID || !second.HasKey {
		t.Fatal("immutable edit failed", second, err)
	}
	old, err := s.providerConfig(ctx, first.ID)
	if err != nil || old.Model != "old-model" {
		t.Fatal("old model changed", err)
	}
	oldKey, err := s.resolvedProviderSecret(ctx, old.APIKeyEnv)
	if err != nil || oldKey != "api-secret" {
		t.Fatal("old key lost", err)
	}
	newer, err := s.providerConfig(ctx, second.ID)
	if err != nil || len(newer.Headers) != 1 {
		t.Fatal("stored sensitive header not reused", err)
	}
	edit.BaseURL = "https://other.example/v1"
	if _, err = s.SaveProviderSetting(ctx, edit); err == nil {
		t.Fatal("existing key silently sent to changed endpoint")
	}
	edit = ProviderSettingsInput{ProviderConfig: second.ProviderConfig, ClearAPIKey: true}
	if _, err = s.SaveProviderSetting(ctx, edit); err == nil {
		t.Fatal("cleared key accepted without explicit keyless mode")
	}
	edit.NoAuth = true
	third, err := s.SaveProviderSetting(ctx, edit)
	if err != nil || third.HasKey || third.CredentialSource != "none" {
		t.Fatal("explicit key clear failed", third, err)
	}
	if key, err := s.resolvedProviderSecret(ctx, old.APIKeyEnv); err != nil || key != "api-secret" {
		t.Fatal("explicit new config damaged old queued credentials", err)
	}
	settings, err := s.ProviderSettings(ctx)
	if err != nil || len(settings.Providers) != 1 || settings.Providers[0].ID != third.ID {
		t.Fatal("superseded config cluttered current settings", settings, err)
	}
	if _, err = s.ProviderByID(ctx, first.ID); err != nil {
		t.Fatal("superseded config unavailable to queued task", err)
	}
	if _, err = s.SaveProviderSetting(ctx, ProviderSettingsInput{ProviderConfig: first.ProviderConfig}); !IsConflict(err) {
		t.Fatal("concurrent old edit was not rejected", err)
	}
	if _, err = s.ProviderSettings(WithPrincipal(context.Background(), Principal{ID: "viewer"})); err != ErrForbidden {
		t.Fatal("non-admin can read global settings", err)
	}
}

func TestProviderSettingsSupersededTaskRouteDoesNotReturnAfterEdit(t *testing.T) {
	s, ctx := providerSettingsFixture(t)
	first, err := s.SaveProviderSetting(ctx, ProviderSettingsInput{ProviderConfig: ProviderConfig{Name: "专用审稿", Protocol: "chat", BaseURL: "https://model.example/v1", Model: "old-review", Enabled: true, TaskTypes: []string{"review"}}, APIKey: "keep-old-queue-key"})
	if err != nil {
		t.Fatal(err)
	}
	edit := ProviderSettingsInput{ProviderConfig: first.ProviderConfig}
	edit.Model = "new-default"
	edit.TaskTypes = []string{}
	edit.IsDefault = true
	newer, err := s.SaveProviderSetting(ctx, edit)
	if err != nil {
		t.Fatal(err)
	}
	_, policy, id, err := s.ResolveTaskProvider(ctx, "review")
	if err != nil || id != newer.ID || policy.Model != "new-default" {
		t.Fatal("superseded task route returned", id, policy, err)
	}
	old, err := s.providerConfig(ctx, first.ID)
	if err != nil || old.SupersededBy != newer.ID || old.Model != "old-review" || !old.Enabled {
		t.Fatal("old snapshot not preserved", old, err)
	}
}

func TestProviderSettingsMissingMasterAndTamperedCiphertextFailWithoutSecretLeak(t *testing.T) {
	s, ctx := providerSettingsFixture(t)
	saved, err := s.SaveProviderSetting(ctx, ProviderSettingsInput{ProviderConfig: ProviderConfig{Name: "加密模型", Protocol: "chat", BaseURL: "https://model.example/v1", Model: "m", Enabled: true}, APIKey: "never-printed-secret"})
	if err != nil {
		t.Fatal(err)
	}
	config, err := s.providerConfig(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	original := os.Getenv("PROVIDER_MASTER_KEY_PATH")
	t.Setenv("PROVIDER_MASTER_KEY_PATH", filepath.Join(t.TempDir(), "missing.key"))
	if _, err = s.resolvedProviderSecret(ctx, config.APIKeyEnv); err == nil || strings.Contains(err.Error(), "never-printed-secret") {
		t.Fatal("missing master not handled", err)
	}
	if _, err = os.Stat(os.Getenv("PROVIDER_MASTER_KEY_PATH")); !os.IsNotExist(err) {
		t.Fatal("read silently created a replacement master key")
	}
	t.Setenv("PROVIDER_MASTER_KEY_PATH", original)
	if _, err = s.DB.Exec(ctx, "UPDATE wb_provider_secrets SET ciphertext=set_byte(ciphertext,5,get_byte(ciphertext,5)#255) WHERE id=$1", strings.TrimPrefix(config.APIKeyEnv, "secret:")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProviderByID(ctx, config.ID); err == nil || strings.Contains(err.Error(), "never-printed-secret") {
		t.Fatal("tamper accepted or secret leaked", err)
	}
}

func TestProviderDiscoveryDoesNotPersistOrGenerateAndKeylessOmitsAuthorization(t *testing.T) {
	s, ctx := providerSettingsFixture(t)
	gets, posts := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("keyless request included Authorization")
		}
		if r.Method == "GET" {
			gets++
			fmt.Fprint(w, `{"data":[{"id":"local"}]}`)
		} else {
			posts++
			fmt.Fprint(w, `{"choices":[{"message":{"content":"local answer"}}]}`)
		}
	}))
	defer server.Close()
	in := ProviderSettingsInput{ProviderConfig: ProviderConfig{Name: "本机免密", Protocol: "chat", BaseURL: server.URL, Model: "local", NoAuth: true, Enabled: true}}
	discovered, err := s.DiscoverProviderModels(ctx, in)
	if err != nil || !discovered.Reachable || gets != 1 || posts != 0 {
		t.Fatal("discovery billed or failed", discovered, err)
	}
	var count int
	_ = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_provider_configs").Scan(&count)
	if count != 0 {
		t.Fatal("discovery persisted configuration")
	}
	saved, err := s.SaveProviderSetting(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := s.ProviderByID(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Generate(ctx, llm.Request{Model: "local", Messages: []llm.Message{{Role: "user", Content: "hello"}}, MaxOutputTokens: 100}); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatal("generation not called")
	}
}

func TestProviderSettingsConcurrentDefaultsAndEnvironmentConnection(t *testing.T) {
	s, ctx := providerSettingsFixture(t)
	t.Setenv("LLM_BASE_URL", "https://configured.example/v1")
	t.Setenv("LLM_API_KEY", "environment-secret")
	t.Setenv("LLM_ACTOR_AUTHORIZATION", "actor-environment")
	view, err := s.ProviderSettings(ctx)
	if err != nil || view.Active.CredentialSource != "none" {
		t.Fatal("fake active mislabeled", err)
	}
	p, err := s.SaveProviderSetting(ctx, ProviderSettingsInput{ProviderConfig: ProviderConfig{ID: "environment", Name: "保存环境连接", Protocol: "responses", BaseURL: "https://configured.example/v1", Model: "m", Enabled: true, IsDefault: true}})
	if err != nil || !p.HasKey || p.CredentialSource != "environment" {
		t.Fatal("environment connection could not become editable config", p, err)
	}
	changed := ProviderSettingsInput{ProviderConfig: p.ProviderConfig}
	changed.BaseURL = "https://different.example/v1"
	if _, err = s.SaveProviderSetting(ctx, changed); err == nil {
		t.Fatal("explicit environment key reference crossed an edited endpoint")
	}
	changed.APIKey = "a-new-connection-key"
	changed.Headers = map[string]string{"X-Openai-Actor-Authorization": "env:LLM_ACTOR_AUTHORIZATION"}
	if _, err = s.SaveProviderSetting(ctx, changed); err == nil {
		t.Fatal("explicit environment header reference crossed an edited endpoint")
	}
	copilot, err := s.SaveProviderSetting(ctx, ProviderSettingsInput{ProviderConfig: ProviderConfig{Name: "对话专用连接", Protocol: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "copilot-only", Enabled: true, TaskTypes: []string{"copilot"}}})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ProviderSettings(ctx)
	if err != nil || view.Active.ID != copilot.ID {
		t.Fatal("active model ignored a specific Copilot route", err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.SaveProviderSetting(ctx, ProviderSettingsInput{ProviderConfig: ProviderConfig{Name: fmt.Sprintf("并发%d", i), Protocol: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "m", Enabled: true, IsDefault: true}})
			errors <- e
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var defaults int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_provider_configs WHERE is_default").Scan(&defaults); err != nil || defaults != 1 {
		t.Fatal("concurrent defaults not unique", defaults, err)
	}
	if err = s.SetProviderDefault(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SetProviderEnabled(ctx, p.ID, false); err != nil {
		t.Fatal(err)
	}
	if err = s.SetProviderDefault(ctx, p.ID); err == nil {
		t.Fatal("disabled provider became default")
	}
}
