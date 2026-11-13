package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novel/internal/llm"
	"novel/internal/testutil"
	"novel/internal/workbench"
)

func TestInvalidPriceNeverBecomesAnEstimate(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-1", "bad", ""} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("TEST_PRICE", value)
			if price("TEST_PRICE") != nil {
				t.Fatal("invalid price accepted")
			}
		})
	}
	t.Setenv("TEST_PRICE", "0")
	if p := price("TEST_PRICE"); p == nil || *p != 0 {
		t.Fatal("explicit zero price not preserved")
	}
}

func TestEnvironmentProviderAutoSelectionAndExplicitErrors(t *testing.T) {
	for _, tc := range []struct {
		name, mode, base, key, model, want string
		invalid                            bool
	}{
		{name: "unset starts demo", want: "fake"},
		{name: "auto starts demo", mode: "auto", want: "fake"},
		{name: "auto complete uses provider", mode: "auto", base: "https://model.example/v1", key: "fixture-secret", model: "m", want: "configured"},
		{name: "unset complete uses provider", base: "https://model.example/v1", key: "fixture-secret", model: "m", want: "configured"},
		{name: "explicit complete", mode: "configured", base: "https://model.example/v1", key: "fixture-secret", model: "m", want: "configured"},
		{name: "explicit demo", mode: "fake", base: "incomplete-value", want: "fake"},
		{name: "auto partial endpoint", mode: "auto", base: "https://model.example/v1", invalid: true},
		{name: "auto partial key", mode: "auto", key: "fixture-secret", invalid: true},
		{name: "auto partial model", mode: "auto", model: "m", invalid: true},
		{name: "configured empty is an error", mode: "configured", invalid: true},
		{name: "configured missing model", mode: "configured", base: "https://model.example/v1", key: "fixture-secret", invalid: true},
		{name: "unknown mode", mode: "typo", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_PROVIDER", tc.mode)
			t.Setenv("LLM_BASE_URL", tc.base)
			t.Setenv("LLM_API_KEY", tc.key)
			t.Setenv("LLM_MODEL", tc.model)
			mode, err := environmentProviderMode()
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid connection silently started")
				}
				if strings.Contains(err.Error(), "fixture-secret") {
					t.Fatal("error leaked a credential")
				}
				return
			}
			if err != nil || mode != tc.want {
				t.Fatal("wrong startup mode", mode, err)
			}
		})
	}
}

func TestAutoProviderStartsEmptyAndPageConnectionTakesOverWithoutRestart(t *testing.T) {
	pool := testutil.Database(t)
	connection, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	query := connection.Query()
	query.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	connection.RawQuery = query.Encode()
	t.Setenv("DATABASE_URL", connection.String())
	t.Setenv("APP_PROVIDER", "auto")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("LLM_ACTOR_AUTHORIZATION", "")
	t.Setenv("LLM_WIRE_API", "chat")
	t.Setenv("PROVIDER_MASTER_KEY_PATH", filepath.Join(t.TempDir(), "master.key"))
	ctx := workbench.WithPrincipal(context.Background(), workbench.Principal{ID: "startup-admin", Admin: true})
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer local-fixture-key" {
			t.Error("startup connection credential did not resolve")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"local fixture reply"}}],"usage":{"prompt_tokens":4,"completion_tokens":3}}`)
	}))
	defer server.Close()
	service, err := Service(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer service.DB.Close()
	if service.ProviderName != "fake" || service.Model != "fake-v1" || calls != 0 {
		t.Fatal("empty installation did not start as an explicit demo")
	}
	saved, err := service.SaveProviderSetting(ctx, workbench.ProviderSettingsInput{ProviderConfig: workbench.ProviderConfig{Name: "Page configured after empty startup", Protocol: "chat", BaseURL: server.URL, Model: "page-model", Enabled: true, IsDefault: true}, APIKey: "local-fixture-key"})
	if err != nil {
		t.Fatal(err)
	}
	provider, policy, id, err := service.ResolveTaskProvider(ctx, "copilot")
	if err != nil || id != saved.ID || policy.Model != "page-model" {
		t.Fatal("page connection required a restart", err)
	}
	output, err := provider.Generate(ctx, llm.Request{Model: policy.Model, Messages: []llm.Message{{Role: "user", Content: "local fixture request"}}, MaxOutputTokens: 50})
	if err != nil || output.Text != "local fixture reply" || calls != 1 {
		t.Fatal("new connection did not execute", err)
	}
	t.Setenv("LLM_BASE_URL", server.URL)
	t.Setenv("LLM_API_KEY", "local-fixture-key")
	t.Setenv("LLM_MODEL", "environment-model")
	configured, err := Service(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer configured.DB.Close()
	if configured.ProviderName != "configured" || configured.Model != "environment-model" || calls != 1 {
		t.Fatal("complete environment still started as demo")
	}
	if _, err = configured.Provider.Generate(ctx, llm.Request{Model: configured.Model, Messages: []llm.Message{{Role: "user", Content: "environment fixture request"}}, MaxOutputTokens: 50}); err != nil || calls != 2 {
		t.Fatal("environment provider did not execute", err)
	}
	_, policy, id, err = configured.ResolveTaskProvider(ctx, "copilot")
	if err != nil || id != saved.ID || policy.Model != "page-model" {
		t.Fatal("existing page default lost precedence", err)
	}
}
