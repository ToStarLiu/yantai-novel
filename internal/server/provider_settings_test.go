package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"novel/internal/workbench"
)

func TestProviderSettingsHTTPWriteOnlyCredentialsDiscoveryAndEffectiveModel(t *testing.T) {
	t.Setenv("PROVIDER_MASTER_KEY_PATH", filepath.Join(t.TempDir(), "master.key"))
	s := testServer(t)
	session := loginForTest(t, s)
	const secret = "http-api-secret-do-not-return"
	const header = "http-actor-secret-do-not-return"
	count := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if r.Method != "GET" || r.URL.Path != "/models" {
			t.Error("discovery used generation endpoint")
		}
		if r.Header.Get("Authorization") != "Bearer "+secret || r.Header.Get("x-openai-actor-authorization") != header {
			t.Error("credentials missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"selected-model"}]}`))
	}))
	defer provider.Close()
	input := workbench.ProviderSettingsInput{ProviderConfig: workbench.ProviderConfig{Name: "UI自定义服务", Protocol: "responses", BaseURL: provider.URL, Model: "selected-model", Enabled: true, IsDefault: true, Headers: map[string]string{"x-openai-actor-authorization": header}}, APIKey: secret}
	result := request(s, "POST", "/api/provider-settings/discover", jsonBody(input), session)
	discovery := decodeResponse[workbench.ProviderDiscovery](t, result)
	if !discovery.Reachable || count != 1 {
		t.Fatal("discovery failed")
	}
	savedResponse := request(s, "POST", "/api/provider-settings", jsonBody(input), session)
	saved := decodeResponse[workbench.ProviderSettingPublic](t, savedResponse)
	if saved.ID == "" || !saved.HasKey || saved.CredentialSource != "stored" {
		t.Fatal("keyless config returned", saved)
	}
	for _, response := range []*httptest.ResponseRecorder{savedResponse, request(s, "GET", "/api/provider-settings", "", session), request(s, "GET", "/api/provider-configs", "", session)} {
		if strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), header) {
			t.Fatal("HTTP read leaked write-only credentials")
		}
	}
	config := decodeResponse[map[string]any](t, request(s, "GET", "/api/config", "", session))
	if config["model"] != "selected-model" || config["provider"] != saved.ID || config["externalModel"] != true {
		t.Fatal("effective configured model mislabeled", config)
	}
	edit := workbench.ProviderSettingsInput{ProviderConfig: saved.ProviderConfig}
	edit.Model = "another-model"
	second := decodeResponse[workbench.ProviderSettingPublic](t, request(s, "POST", "/api/provider-settings", jsonBody(edit), session))
	if !second.HasKey || second.ID == saved.ID {
		t.Fatal("edit lost stored key")
	}
	if response := request(s, "PUT", "/api/provider-settings/"+saved.ID+"/default", `{}`, session); response.Code != 409 {
		t.Fatal("superseded config selected", response.Code)
	}
	defaultResponse := request(s, "PUT", "/api/provider-settings/"+second.ID+"/default", `{}`, session)
	if defaultResponse.Code != 200 {
		t.Fatal(defaultResponse.Code, defaultResponse.Body.String())
	}
	_ = decodeResponse[map[string]bool](t, request(s, "PUT", "/api/provider-settings/"+second.ID+"/enabled", `{"enabled":false}`, session))
	if request(s, "PUT", "/api/provider-settings/"+second.ID+"/default", `{}`, session).Code == 200 {
		t.Fatal("disabled provider set as default")
	}
	unauthorized := request(s, "GET", "/api/provider-settings", "", testLogin{})
	if unauthorized.Code != 401 {
		t.Fatal("unauthenticated settings access", unauthorized.Code)
	}
}

func TestProviderSettingsHTTPRejectsNonAdminAndDoesNotEchoSupplierErrors(t *testing.T) {
	t.Setenv("PROVIDER_MASTER_KEY_PATH", filepath.Join(t.TempDir(), "master.key"))
	s := testServer(t)
	user, err := s.Service.CreateUser(context.Background(), "settings-viewer", "strong-test-password", "Viewer")
	if err != nil {
		t.Fatal(err)
	}
	_ = user
	login := request(s, "POST", "/api/session", `{"username":"settings-viewer","password":"strong-test-password"}`, testLogin{})
	if login.Code != 200 {
		t.Fatal(login.Code)
	}
	var data struct {
		CSRF string `json:"csrfToken"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &data)
	session := testLogin{Cookie: login.Result().Cookies()[0], CSRF: data.CSRF}
	for _, path := range []string{"/api/provider-settings", "/api/provider-settings/discover"} {
		method := "GET"
		body := ""
		if strings.HasSuffix(path, "discover") {
			method = "POST"
			body = `{"baseUrl":"http://localhost:11434","protocol":"ollama"}`
		}
		if response := request(s, method, path, body, session); response.Code != 403 {
			t.Fatal("non-admin settings access", path, response.Code, response.Body.String())
		}
	}
	admin := loginForTest(t, s)
	supplier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte("echo-sensitive-key-value"))
	}))
	defer supplier.Close()
	response := request(s, "POST", "/api/provider-settings/discover", jsonBody(map[string]any{"baseUrl": supplier.URL, "protocol": "chat", "apiKey": "echo-sensitive-key-value"}), admin)
	if response.Code != 200 || strings.Contains(response.Body.String(), "echo-sensitive-key-value") {
		t.Fatal("supplier response/secret leaked", response.Code, response.Body.String())
	}
}
