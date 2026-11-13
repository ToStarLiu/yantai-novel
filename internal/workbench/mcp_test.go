package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"novel/internal/job"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMCPRealHandshakeToolCallAgentPermissionAndSecretSafeLogs(t *testing.T) {
	s, _ := runtimeFixture(t)
	ctx := context.Background()
	t.Setenv("TEST_MCP_KEY", "mcp-private-key-not-in-ui")
	var calls atomic.Int64
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mcp-private-key-not-in-ui" {
			t.Error("missing referenced secret")
		}
		var input struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("invalid JSONRPC request")
			return
		}
		result := any(map[string]any{})
		switch input.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "actual-session")
			result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}}
		case "notifications/initialized":
			if r.Header.Get("Mcp-Session-Id") != "actual-session" {
				t.Error("session not propagated")
			}
			w.WriteHeader(202)
			return
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "lookup", "inputSchema": map[string]any{"type": "object"}, "annotations": map[string]bool{"readOnlyHint": true}}, map[string]any{"name": "delete", "annotations": map[string]bool{"readOnlyHint": false, "destructiveHint": true}}}}
		case "tools/call":
			calls.Add(1)
			var call struct {
				Name      string            `json:"name"`
				Arguments map[string]string `json:"arguments"`
			}
			_ = json.Unmarshal(input.Params, &call)
			if call.Name != "lookup" || call.Arguments["query"] != "铜印" {
				t.Error("actual tool arguments lost")
			}
			result = map[string]any{"content": []any{map[string]string{"type": "text", "text": "真实远端返回：铜印在磨坊"}}, "structuredContent": map[string]any{"source": "remote-v1"}}
		default:
			t.Error("unexpected method", input.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": input.ID, "result": result})
	}))
	defer remote.Close()
	config, err := s.SaveMCPConfig(ctx, MCPConfig{Name: "只读资料库", URL: remote.URL, APIKeyEnv: "TEST_MCP_KEY", AllowedTools: []string{"lookup", "delete"}, AllowLoopback: true, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := s.SaveAgent(ctx, AgentDefinition{Name: "资料员", TaskType: "reader", Tools: []string{"mcp:" + config.ID + ":lookup"}, MaxOutputTokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	call := MCPToolCall{ConfigID: config.ID, Name: "lookup", Arguments: json.RawMessage(`{"query":"铜印"}`)}
	if _, err = s.ExecuteAgentMCP(ctx, agent.ID, []MCPToolCall{call, {ConfigID: config.ID, Name: "delete", Arguments: json.RawMessage(`{}`)}}); err == nil || calls.Load() != 0 {
		t.Fatal("complete permission preflight must precede network side effects", err)
	}
	results, err := s.ExecuteAgentMCP(ctx, agent.ID, []MCPToolCall{call})
	if err != nil || len(results) != 1 || !results[0].ReadOnly || !strings.Contains(string(results[0].Content), "真实远端返回") || calls.Load() != 1 {
		t.Fatalf("actual tool call missing %#v %v", results, err)
	}
	if _, err = s.CallMCPTool(ctx, MCPToolCall{ConfigID: config.ID, Name: "delete", Arguments: json.RawMessage(`{}`)}); err == nil || calls.Load() != 1 {
		t.Fatal("destructive tool dispatched", err)
	}
	if _, err = s.CallMCPTool(ctx, MCPToolCall{ConfigID: config.ID, Name: "not-allowed", Arguments: json.RawMessage(`{}`)}); err == nil || calls.Load() != 1 {
		t.Fatal("unlisted tool dispatched", err)
	}
	logs, err := s.MCPCallLogs(ctx)
	if err != nil || len(logs) != 3 {
		t.Fatalf("audit logs %#v %v", logs, err)
	}
	configs, _ := s.ListMCPConfigs(ctx)
	raw, _ := json.Marshal([]any{configs, logs})
	for _, private := range []string{"mcp-private-key-not-in-ui", "铜印", "真实远端返回"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private content leaked into configuration or logs", private)
		}
	}
	if err = s.SetMCPEnabled(ctx, config.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CallMCPTool(ctx, call); err == nil || calls.Load() != 1 {
		t.Fatal("disabled integration still dispatched", err)
	}
}

func TestMCPSSRFAndRedirectRejected(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "198.18.0.1", "192.0.2.1", "2002:7f00:1::", "64:ff9b::7f00:1", "::ffff:127.0.0.1"} {
		if allowedMCPIP(net.ParseIP(ip), false) {
			t.Error("unsafe address allowed", ip)
		}
	}
	if !allowedMCPIP(net.ParseIP("8.8.8.8"), false) || !allowedMCPIP(net.ParseIP("127.0.0.1"), true) || allowedMCPIP(net.ParseIP("8.8.8.8"), true) {
		t.Fatal("explicit loopback/public boundaries broken")
	}
	ctx := context.Background()
	for _, address := range []string{"http://example.com/mcp", "https://127.0.0.1/mcp", "https://169.254.169.254/mcp", "https://user:password@example.com/mcp", "https://example.com/mcp?token=secret"} {
		if validateMCPURL(ctx, address, false) == nil {
			t.Error("unsafe URL allowed", address)
		}
	}
	var targetCalls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	client, err := newMCPClient(ctx, MCPConfig{URL: redirect.URL, AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	if err = client.initialize(ctx); err == nil || targetCalls.Load() != 0 {
		t.Fatal("redirect followed", err)
	}
}

func TestMCPSSEMatchingResultStopsWithoutWaitingForConnectionClose(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/message\"}\n\n")
		fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":999,\"result\":{}}\n\n")
		fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":\n")
		fmt.Fprint(w, "data: {\"protocolVersion\":\"2025-11-25\"}}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer remote.Close()
	client, err := newMCPClient(context.Background(), MCPConfig{URL: remote.URL, AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	raw, err := client.request(context.Background(), "initialize", map[string]any{}, false)
	if err != nil || !strings.Contains(string(raw), "2025-11-25") {
		t.Fatalf("matching SSE result not parsed %s %v", raw, err)
	}
}

func TestLocalMCPToolsReadAuthorityAndNoArbitraryCommands(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	raw, _ := json.Marshal(map[string]string{"revisionId": c.CurrentRevisionID})
	value, err := s.CallLocalMCPTool(ctx, "read-revision", raw)
	if err != nil || value.(Revision).ID != c.CurrentRevisionID {
		t.Fatalf("real revision unavailable %#v %v", value, err)
	}
	for _, name := range []string{"shell", "exec", "sql", "delete-project"} {
		if _, err = s.CallLocalMCPTool(ctx, name, raw); err == nil {
			t.Error("arbitrary command accepted", name)
		}
	}
	for _, tool := range LocalMCPTools() {
		if !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint || !json.Valid(tool.InputSchema) {
			t.Fatal("unsafe tool definition", tool)
		}
	}
}

func TestMCPIntentCheckpointPreventsConcurrentAndRepeatedDispatch(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		result := any(map[string]any{})
		switch input.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25"}
		case "notifications/initialized":
			w.WriteHeader(202)
			return
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "lookup", "annotations": map[string]any{"readOnlyHint": true}}}}
		case "tools/call":
			if calls.Add(1) == 1 {
				close(entered)
				<-release
			}
			result = map[string]any{"content": []any{map[string]string{"type": "text", "text": "持久工具结果"}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": input.ID, "result": result})
	}))
	defer remote.Close()
	config, err := s.SaveMCPConfig(ctx, MCPConfig{Name: "intent tool", URL: remote.URL, AllowLoopback: true, AllowedTools: []string{"lookup"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := s.SaveAgent(ctx, AgentDefinition{Name: "tool-agent", TaskType: "draft", Tools: []string{"mcp:" + config.ID + ":lookup"}, MaxOutputTokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	call := MCPToolCall{ConfigID: config.ID, Name: "lookup", Arguments: json.RawMessage(`{}`)}
	intent, hash := job.NewID(), job.Hash([]byte("stable request"))
	done := make(chan error, 1)
	go func() { _, err := s.RunMCPForIntent(ctx, intent, hash, agent.ID, []MCPToolCall{call}); done <- err }()
	<-entered
	if _, err = s.RunMCPForIntent(ctx, intent, hash, agent.ID, []MCPToolCall{call}); !IsConflict(err) {
		close(release)
		t.Fatal("concurrent duplicate was not fenced", err)
	}
	if _, err = s.RunMCPForIntent(ctx, intent, job.Hash([]byte("changed")), agent.ID, []MCPToolCall{call}); !IsConflict(err) {
		close(release)
		t.Fatal("changed input accepted", err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	result, err := s.RunMCPForIntent(ctx, intent, hash, agent.ID, []MCPToolCall{call})
	if err != nil || len(result) != 1 || calls.Load() != 1 || !strings.Contains(string(result[0].Content), "持久工具结果") {
		t.Fatalf("intent result lost %#v %v calls=%d", result, err, calls.Load())
	}
	// The normal generation path consumes the same persisted tool result and puts
	// its content into the frozen request; replaying the intent creates no new call.
	request := RunRequest{IntentID: job.NewID(), TaskType: "draft", AgentID: agent.ID, MCPCalls: []MCPToolCall{call}}
	run, err := s.SubmitRun(ctx, c.ID, request)
	if err != nil || len(run.Input.MCPResults) != 1 || calls.Load() != 2 {
		t.Fatalf("queued MCP context missing %#v %v", run.Input.MCPResults, err)
	}
	user := run.Input.Request.Messages[len(run.Input.Request.Messages)-1].Content
	if !strings.Contains(user, "持久工具结果") {
		t.Fatal("MCP result absent from frozen model input")
	}
	replayed, err := s.SubmitRun(ctx, c.ID, request)
	if err != nil || replayed.ID != run.ID || calls.Load() != 2 {
		t.Fatalf("queue replay redispatched tool %v calls=%d", err, calls.Load())
	}
	if err = s.ValidateMCPIntent(ctx, request.IntentID, agent.ID, request.MCPCalls); err != nil {
		t.Fatal("same MCP request rejected", err)
	}
	if err = s.ValidateMCPIntent(ctx, request.IntentID, agent.ID, nil); !IsConflict(err) {
		t.Fatal("changed MCP call list accepted", err)
	}
	if err = s.ValidateMCPIntent(ctx, request.IntentID, "changed-agent", request.MCPCalls); !IsConflict(err) {
		t.Fatal("changed agent accepted", err)
	}
	plain := submit(t, s, c, "draft")
	if err = s.ValidateMCPIntent(ctx, plain.IntentID, agent.ID, request.MCPCalls); !IsConflict(err) {
		t.Fatal("new tool allowed on existing plain intent", err)
	}
}
