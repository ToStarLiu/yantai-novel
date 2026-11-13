package workbench

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

type MCPConfig struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	APIKeyEnv     string   `json:"apiKeyEnv"`
	AllowedTools  []string `json:"allowedTools"`
	AllowLoopback bool     `json:"allowLoopback"`
	Enabled       bool     `json:"enabled"`
}
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations struct {
		ReadOnlyHint    bool `json:"readOnlyHint"`
		DestructiveHint bool `json:"destructiveHint"`
	} `json:"annotations"`
}
type MCPToolCall struct {
	ConfigID  string          `json:"configId"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
type MCPToolResult struct {
	ConfigID          string          `json:"configId"`
	Name              string          `json:"name"`
	Content           json.RawMessage `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError"`
	ReadOnly          bool            `json:"readOnly"`
}
type MCPCallLog struct {
	ID            string    `json:"id"`
	ConfigID      string    `json:"configId"`
	ToolName      string    `json:"toolName"`
	ArgumentsHash string    `json:"argumentsHash"`
	Status        string    `json:"status"`
	DurationMs    int64     `json:"durationMs"`
	Error         string    `json:"error"`
	CreatedAt     time.Time `json:"createdAt"`
}

func validateMCPURL(ctx context.Context, raw string, allowLoopback bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("MCP URL必须是无凭据和查询参数的HTTP地址")
	}
	if allowLoopback {
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("MCP回环协议无效")
		}
	} else if u.Scheme != "https" {
		return fmt.Errorf("外部MCP必须使用HTTPS")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("MCP主机解析失败")
	}
	for _, address := range addresses {
		if !allowedMCPIP(address.IP, allowLoopback) {
			return fmt.Errorf("MCP地址指向禁止的内网、回环或保留网络")
		}
	}
	return nil
}

// Validate both DNS answers and the actual dial address. Proxy and redirects are
// disabled, so a DNS change cannot redirect the request onto an internal service.
func allowedMCPIP(ip net.IP, loopback bool) bool {
	if loopback {
		return ip.IsLoopback()
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16"} {
		if netip.MustParsePrefix(raw).Contains(address) {
			return false
		}
	}
	return true
}

func (s *Service) SaveMCPConfig(ctx context.Context, c MCPConfig) (MCPConfig, error) {
	if !validName(c.Name) || len(c.AllowedTools) > 100 {
		return c, fmt.Errorf("MCP名称或工具范围无效")
	}
	if c.APIKeyEnv != "" && !envReference.MatchString(c.APIKeyEnv) {
		return c, fmt.Errorf("MCP密钥只能引用环境变量")
	}
	for _, name := range c.AllowedTools {
		if strings.TrimSpace(name) == "" || len(name) > 128 {
			return c, fmt.Errorf("MCP工具名无效")
		}
	}
	if err := validateMCPURL(ctx, c.URL, c.AllowLoopback); err != nil {
		return c, err
	}
	c.ID = job.NewID()
	raw, _ := json.Marshal(c.AllowedTools)
	_, err := s.DB.Exec(ctx, "INSERT INTO wb_mcp_configs(id,name,url,api_key_env,allowed_tools,allow_loopback,enabled) VALUES($1,$2,$3,$4,$5,$6,$7)", c.ID, c.Name, c.URL, c.APIKeyEnv, raw, c.AllowLoopback, c.Enabled)
	return c, err
}
func (s *Service) SetMCPEnabled(ctx context.Context, id string, enabled bool) error {
	tag, err := s.DB.Exec(ctx, "UPDATE wb_mcp_configs SET enabled=$2 WHERE id=$1", id, enabled)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (s *Service) ListMCPConfigs(ctx context.Context) ([]MCPConfig, error) {
	rows, err := s.DB.Query(ctx, "SELECT id,name,url,api_key_env,allowed_tools,allow_loopback,enabled FROM wb_mcp_configs ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MCPConfig{}
	for rows.Next() {
		var c MCPConfig
		var tools []byte
		if err = rows.Scan(&c.ID, &c.Name, &c.URL, &c.APIKeyEnv, &tools, &c.AllowLoopback, &c.Enabled); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(tools, &c.AllowedTools); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Service) mcpConfig(ctx context.Context, id string) (MCPConfig, error) {
	configs, err := s.ListMCPConfigs(ctx)
	if err != nil {
		return MCPConfig{}, err
	}
	for _, c := range configs {
		if c.ID == id && c.Enabled {
			return c, nil
		}
	}
	return MCPConfig{}, fmt.Errorf("MCP配置不存在或已停用")
}

type mcpClient struct {
	config  MCPConfig
	client  *http.Client
	session string
	version string
	next    int
}

func newMCPClient(ctx context.Context, config MCPConfig) (*mcpClient, error) {
	if err := validateMCPURL(ctx, config.URL, config.AllowLoopback); err != nil {
		return nil, err
	}
	if config.APIKeyEnv != "" && os.Getenv(config.APIKeyEnv) == "" {
		return nil, fmt.Errorf("MCP密钥环境变量未配置")
	}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("MCP连接地址无效")
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("MCP主机解析失败")
		}
		for _, ip := range ips {
			if !allowedMCPIP(ip.IP, config.AllowLoopback) {
				return nil, fmt.Errorf("MCP实际连接地址被拒绝")
			}
		}
		dialer := net.Dialer{Timeout: 10 * time.Second}
		for _, ip := range ips {
			connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return connection, nil
			}
		}
		return nil, fmt.Errorf("MCP连接失败")
	}}
	return &mcpClient{config: config, version: "2025-11-25", client: &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *mcpClient) close() { c.client.CloseIdleConnections() }

func (c *mcpClient) request(ctx context.Context, method string, params any, notification bool) (json.RawMessage, error) {
	c.next++
	payload := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if !notification {
		payload["id"] = c.next
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("MCP参数编码失败")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("MCP请求构造失败")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", c.version)
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
	}
	if key := os.Getenv(c.config.APIKeyEnv); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MCP网络请求失败，未自动重试")
	}
	defer response.Body.Close()
	if session := response.Header.Get("Mcp-Session-Id"); session != "" {
		c.session = session
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("MCP HTTP状态%d", response.StatusCode)
	}
	if notification {
		return nil, nil
	}
	limited := &io.LimitedReader{R: response.Body, N: (2 << 20) + 1}
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(limited)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		var event []string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				event = append(event, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
			if line == "" && len(event) > 0 {
				candidate := []byte(strings.Join(event, "\n"))
				event = nil
				var envelope struct {
					ID *int `json:"id"`
				}
				if json.Unmarshal(candidate, &envelope) == nil && envelope.ID != nil && *envelope.ID == c.next {
					return c.decode(candidate)
				}
			}
			if limited.N <= 0 {
				return nil, fmt.Errorf("MCP响应过大")
			}
		}
		if len(event) > 0 {
			return c.decode([]byte(strings.Join(event, "\n")))
		}
		return nil, fmt.Errorf("MCP响应中断，未自动重试")
	}
	body, err := io.ReadAll(limited)
	if err != nil || len(body) > 2<<20 {
		return nil, fmt.Errorf("MCP响应中断或过大")
	}
	return c.decode(body)
}
func (c *mcpClient) decode(body []byte) (json.RawMessage, error) {
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.JSONRPC != "2.0" || envelope.ID != c.next || len(envelope.Result) == 0 || len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		return nil, fmt.Errorf("MCP JSON-RPC响应无效或工具调用失败")
	}
	return envelope.Result, nil
}
func (c *mcpClient) initialize(ctx context.Context) error {
	raw, err := c.request(ctx, "initialize", map[string]any{"protocolVersion": c.version, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "novel-workbench", "version": "0.1.0"}}, false)
	if err != nil {
		return err
	}
	var value struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(raw, &value) != nil || !MCPProtocolSupported(value.ProtocolVersion) {
		return fmt.Errorf("MCP协议版本不受支持")
	}
	c.version = value.ProtocolVersion
	_, err = c.request(ctx, "notifications/initialized", map[string]any{}, true)
	return err
}
func MCPProtocolSupported(version string) bool {
	return version == "2025-03-26" || version == "2025-06-18" || version == "2025-11-25"
}
func (c *mcpClient) tools(ctx context.Context) ([]MCPTool, error) {
	out := []MCPTool{}
	cursor := ""
	for page := 0; page < 10; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.request(ctx, "tools/list", params, false)
		if err != nil {
			return nil, err
		}
		var value struct {
			Tools      []MCPTool `json:"tools"`
			NextCursor string    `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &value) != nil || len(value.Tools) > 1000 {
			return nil, fmt.Errorf("MCP工具列表无效")
		}
		out = append(out, value.Tools...)
		if value.NextCursor == "" {
			return out, nil
		}
		if value.NextCursor == cursor {
			return nil, fmt.Errorf("MCP分页游标未前进")
		}
		cursor = value.NextCursor
	}
	return nil, fmt.Errorf("MCP工具分页超过限制")
}
func (s *Service) MCPTools(ctx context.Context, configID string) ([]MCPTool, error) {
	config, err := s.mcpConfig(ctx, configID)
	if err != nil {
		return nil, err
	}
	client, err := newMCPClient(ctx, config)
	if err != nil {
		return nil, err
	}
	defer client.close()
	if err = client.initialize(ctx); err != nil {
		return nil, err
	}
	return client.tools(ctx)
}
func (s *Service) CallMCPTool(ctx context.Context, call MCPToolCall) (MCPToolResult, error) {
	started := time.Now()
	result := MCPToolResult{ConfigID: call.ConfigID, Name: call.Name, ReadOnly: true}
	status := "Failed"
	defer func() {
		message := "调用未完成，查看配置与只读权限"
		if status == "Succeeded" {
			message = ""
		}
		logCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = s.DB.Exec(logCtx, "INSERT INTO wb_mcp_calls(id,config_id,tool_name,arguments_hash,status,duration_ms,error) SELECT $1,id,$3,$4,$5,$6,$7 FROM wb_mcp_configs WHERE id=$2", job.NewID(), call.ConfigID, call.Name, job.Hash(call.Arguments), status, time.Since(started).Milliseconds(), message)
	}()
	var arguments map[string]json.RawMessage
	if len(call.Arguments) > 128<<10 || json.Unmarshal(call.Arguments, &arguments) != nil || arguments == nil {
		return result, fmt.Errorf("MCP工具参数必须是JSON对象且不能过大")
	}
	config, err := s.mcpConfig(ctx, call.ConfigID)
	if err != nil {
		return result, err
	}
	allowed := false
	for _, name := range config.AllowedTools {
		if name == call.Name {
			allowed = true
		}
	}
	if !allowed {
		return result, fmt.Errorf("MCP工具不在显式允许列表")
	}
	client, err := newMCPClient(ctx, config)
	if err != nil {
		return result, err
	}
	defer client.close()
	if err = client.initialize(ctx); err != nil {
		return result, err
	}
	tools, err := client.tools(ctx)
	if err != nil {
		return result, err
	}
	readOnly := false
	for _, tool := range tools {
		if tool.Name == call.Name && tool.Annotations.ReadOnlyHint && !tool.Annotations.DestructiveHint {
			readOnly = true
		}
	}
	if !readOnly {
		return result, fmt.Errorf("只允许声明只读且非破坏性的MCP工具")
	}
	raw, err := client.request(ctx, "tools/call", map[string]any{"name": call.Name, "arguments": call.Arguments}, false)
	if err != nil {
		return result, err
	}
	var output struct {
		Content           json.RawMessage `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}
	if json.Unmarshal(raw, &output) != nil || len(output.Content) == 0 {
		return result, fmt.Errorf("MCP工具产物无效")
	}
	result.Content, result.StructuredContent, result.IsError = output.Content, output.StructuredContent, output.IsError
	if output.IsError {
		return result, fmt.Errorf("MCP工具报告失败")
	}
	status = "Succeeded"
	return result, nil
}
func (s *Service) ExecuteAgentMCP(ctx context.Context, agentID string, calls []MCPToolCall) ([]MCPToolResult, error) {
	if len(calls) > 8 {
		return nil, fmt.Errorf("单任务MCP调用超过8次")
	}
	agent, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, tool := range agent.Tools {
		allowed[tool] = true
	}
	// Check the complete request before making any external call.
	for _, call := range calls {
		if !allowed["mcp:"+call.ConfigID+":"+call.Name] {
			return nil, fmt.Errorf("Agent无权调用该MCP工具")
		}
	}
	out := []MCPToolResult{}
	for _, call := range calls {
		result, err := s.CallMCPTool(ctx, call)
		if err != nil {
			return nil, err
		}
		out = append(out, result)
	}
	return out, nil
}
func (s *Service) MCPCallLogs(ctx context.Context) ([]MCPCallLog, error) {
	rows, err := s.DB.Query(ctx, "SELECT * FROM (SELECT id,config_id,tool_name,arguments_hash,status,duration_ms,error,created_at FROM wb_mcp_calls UNION ALL SELECT id,'local-server',tool_name,arguments_hash,status,duration_ms,'',created_at FROM wb_mcp_server_calls) calls ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MCPCallLog{}
	for rows.Next() {
		var log MCPCallLog
		if err = rows.Scan(&log.ID, &log.ConfigID, &log.ToolName, &log.ArgumentsHash, &log.Status, &log.DurationMs, &log.Error, &log.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, rows.Err()
}

// A claimed intent is never automatically dispatched again, including after a
// process crash. Read-only tools still cost time and can have remote side effects
// despite annotations; a failed or ambiguous intent needs an explicit new intent.
func (s *Service) RunMCPForIntent(ctx context.Context, intentID, requestHash, agentID string, calls []MCPToolCall) ([]MCPToolResult, error) {
	if intentID == "" || len(intentID) > 128 || len(requestHash) != 64 || len(calls) == 0 || len(calls) > 8 {
		return nil, fmt.Errorf("MCP意图或请求摘要无效")
	}
	tag, err := s.DB.Exec(ctx, "INSERT INTO wb_mcp_run_inputs(intent_id,request_hash,agent_id,status) VALUES($1,$2,$3,'Running') ON CONFLICT(intent_id) DO NOTHING", intentID, requestHash, agentID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		var hash, agent, status string
		var raw []byte
		if err = s.DB.QueryRow(ctx, "SELECT request_hash,agent_id,status,results FROM wb_mcp_run_inputs WHERE intent_id=$1", intentID).Scan(&hash, &agent, &status, &raw); err != nil {
			return nil, err
		}
		if hash != requestHash || agent != agentID {
			return nil, &Conflict{Message: "同一MCP意图不能更改输入或Agent"}
		}
		if status != "Succeeded" {
			return nil, &Conflict{Message: "MCP意图已调用或可能中断，请核对日志；需要重提时使用新意图"}
		}
		var results []MCPToolResult
		if err = json.Unmarshal(raw, &results); err != nil {
			return nil, err
		}
		return results, nil
	}
	results, callErr := s.ExecuteAgentMCP(ctx, agentID, calls)
	status, message := "Succeeded", ""
	if callErr != nil {
		status = "Failed"
		message = "工具调用失败或中断，禁止自动重发该意图"
	}
	raw, err := json.Marshal(results)
	if err != nil {
		return nil, err
	}
	finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = s.DB.Exec(finishCtx, "UPDATE wb_mcp_run_inputs SET status=$2,results=$3,error=$4,finished_at=now() WHERE intent_id=$1 AND status='Running'", intentID, status, raw, message); err != nil {
		return nil, fmt.Errorf("MCP结果持久化失败，需核对已执行调用")
	}
	return results, callErr
}

// Check existing model intents before dispatching any newly requested external
// tool. The final SubmitRun transaction still compares the entire frozen input.
func (s *Service) ValidateMCPIntent(ctx context.Context, intentID, agentID string, calls []MCPToolCall) error {
	var raw []byte
	err := s.DB.QueryRow(ctx, "SELECT input FROM wb_runs WHERE intent_id=$1", intentID).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var input RunInput
	if err = json.Unmarshal(raw, &input); err != nil {
		return err
	}
	oldCalls := canonicalMCPCalls(input.MCPCalls)
	newCalls := canonicalMCPCalls(calls)
	if input.AgentID != agentID || !bytes.Equal(oldCalls, newCalls) {
		return &Conflict{Message: "同一任务意图不能更改Agent或外部工具输入"}
	}
	return nil
}

func canonicalMCPCalls(calls []MCPToolCall) []byte {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		var arguments any
		_ = json.Unmarshal(call.Arguments, &arguments)
		out = append(out, map[string]any{"configId": call.ConfigID, "name": call.Name, "arguments": arguments})
	}
	raw, _ := json.Marshal(out)
	return raw
}

// Local tools are deliberately typed and read-only; no arbitrary SQL, file or
// process execution is exposed through MCP.
func LocalMCPTools() []MCPTool {
	definitions := []struct{ name, description, schema string }{
		{"read-project", "读取作品、章节目录与设定", `{"type":"object","required":["projectId"],"properties":{"projectId":{"type":"string"}},"additionalProperties":false}`},
		{"read-revision", "按修订ID读取原文与来源哈希", `{"type":"object","required":["revisionId"],"properties":{"revisionId":{"type":"string"}},"additionalProperties":false}`},
		{"search-history", "检索当前有效定稿历史，支持章节边界和角色信息边界", `{"type":"object","required":["projectId","text"],"properties":{"projectId":{"type":"string"},"text":{"type":"string"},"beforeChapterId":{"type":"string"},"characterId":{"type":"string"},"purpose":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":30}},"additionalProperties":false}`},
		{"read-memory", "读取作者确认的叙事状态与事实来源", `{"type":"object","required":["projectId"],"properties":{"projectId":{"type":"string"}},"additionalProperties":false}`},
	}
	out := []MCPTool{}
	for _, definition := range definitions {
		tool := MCPTool{Name: definition.name, Description: definition.description, InputSchema: json.RawMessage(definition.schema)}
		tool.Annotations.ReadOnlyHint = true
		out = append(out, tool)
	}
	return out
}
func (s *Service) CallLocalMCPTool(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	var input struct {
		ProjectID       string `json:"projectId"`
		RevisionID      string `json:"revisionId"`
		Text            string `json:"text"`
		BeforeChapterID string `json:"beforeChapterId"`
		CharacterID     string `json:"characterId"`
		Purpose         string `json:"purpose"`
		Limit           int    `json:"limit"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.ProjectID == "" && input.RevisionID == "" {
		return nil, fmt.Errorf("MCP工具参数无效")
	}
	switch name {
	case "read-project":
		return s.GetProject(ctx, input.ProjectID)
	case "read-revision":
		return s.GetRevision(ctx, input.RevisionID)
	case "search-history":
		return s.Search(ctx, Query{ProjectID: input.ProjectID, Text: input.Text, BeforeChapterID: input.BeforeChapterID, CharacterID: input.CharacterID, Purpose: input.Purpose, Limit: input.Limit})
	case "read-memory":
		return s.State(ctx, input.ProjectID)
	default:
		return nil, fmt.Errorf("MCP工具不存在或没有执行权限")
	}
}

func (s *Service) RecordMCPServerCall(name string, arguments json.RawMessage, succeeded bool, duration time.Duration) {
	status := "Failed"
	if succeeded {
		status = "Succeeded"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = s.DB.Exec(ctx, "INSERT INTO wb_mcp_server_calls(id,tool_name,arguments_hash,status,duration_ms) VALUES($1,$2,$3,$4,$5)", job.NewID(), name, job.Hash(arguments), status, duration.Milliseconds())
}
