package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"novel/internal/workbench"
)

func TestMCPProtocolAuthenticatedActualReadAndMutationRejected(t *testing.T) {
	s := testServer(t)
	session := loginForTest(t, s)
	project := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"MCP真实作品"}`, session))
	t.Setenv("MCP_SERVER_TOKEN", "mcp-test-private-credential-more-than-24")
	call := func(method string, params any, token, origin string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "request-1", "method": method, "params": params})
		r := httptest.NewRequest("POST", testOrigin+"/mcp", strings.NewReader(string(raw)))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		s.mcpProtocol(w, r)
		return w
	}
	if w := call("tools/list", map[string]any{}, "", ""); w.Code != 401 {
		t.Fatal("unauthenticated tool API", w.Code)
	}
	if w := call("tools/list", map[string]any{}, "mcp-test-private-credential-more-than-24", "https://evil.invalid"); w.Code != 403 {
		t.Fatal("invalid browser origin", w.Code)
	}
	if w := call("initialize", map[string]any{"protocolVersion": "2025-11-25"}, "mcp-test-private-credential-more-than-24", ""); !strings.Contains(w.Body.String(), "2025-11-25") {
		t.Fatal(w.Body.String())
	}
	if w := call("tools/call", map[string]any{"name": "read-project", "arguments": map[string]string{"projectId": project.ID}}, "mcp-test-private-credential-more-than-24", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "MCP真实作品") || !strings.Contains(w.Body.String(), `"isError":false`) {
		t.Fatal("tool did not read actual state", w.Body.String())
	}
	if w := call("tools/call", map[string]any{"name": "exec", "arguments": map[string]string{"command": "delete"}}, "mcp-test-private-credential-more-than-24", ""); !strings.Contains(w.Body.String(), `"isError":true`) {
		t.Fatal("shell executed", w.Body.String())
	}
	if w := call("unknown", map[string]any{}, "mcp-test-private-credential-more-than-24", ""); !strings.Contains(w.Body.String(), "-32601") {
		t.Fatal("JSONRPC method error missing", w.Body.String())
	}
}

func TestMCPProtectedConfigurationAndNotificationTransport(t *testing.T) {
	s := testServer(t)
	if w := request(s, "GET", "/api/mcp-configs", "", testLogin{}); w.Code != 401 {
		t.Fatal("unprotected config", w.Code)
	}
	session := loginForTest(t, s)
	if w := request(s, "GET", "/api/mcp-configs", "", session); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := request(s, "POST", "/api/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, session)
	if w.Code != 202 || w.Body.Len() != 0 {
		t.Fatal("notification transport response", w.Code, w.Body.String())
	}
}
