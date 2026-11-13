# 高级编排、检索与 MCP 接口

所有 `/api/` 配置请求使用作者登录和 CSRF 校验；配置管理与 MCP 原始调用限实例管理员。作品数据接口仍校验作品成员权限。模型连接现可在页面输入write-only密钥，采用独立master key的加密存储；旧环境变量方式继续兼容。Embedding/MCP等配置的apiKeyEnv仍填写环境变量名，不传实际密钥。页面连接、发现模型及凭据恢复见[供应商设置](provider-settings.md)。

## Provider 与价格

`GET/POST /api/provider-configs` 支持 `protocol` 为 `responses`、`chat`、`ollama`。配置示例：

```json
{"name":"编辑模型","protocol":"responses","baseUrl":"https://api.example.org/v1","apiKeyEnv":"EDITOR_MODEL_KEY","model":"my-editor-model","taskTypes":["review","reader"],"enabled":true,"isDefault":false,"currency":"USD","inputPricePerMillion":2,"outputPricePerMillion":8}
```

`POST /api/provider-configs/{id}/health` 只请求模型目录，不生成文字。`PUT /api/provider-configs/{id}/enabled` 接受 `{"enabled":false}`。保存配置生成新配置 ID，旧任务继续引用原配置快照；停用旧配置会阻止它继续发送调用。

价格留空为 `null`，不会当作零；零可以明确表示免费模型。任一单价已填必须有币种，金额预算要求输入/输出均有已知单价且币种匹配。不同接口即使模型同名，也不借用默认接口价格。服务商未报告用量时实际费用仍未知。价格为估算参数，不能替代账单。

## 工作流与语义检索

`GET/POST /api/workflows` 保存不可变版本，编辑已有 ID 会生成新 ID/版本。步骤支持 `dependsOn`、`condition`、`humanGate`、`maxCost`、`inputSelection`、`promptTemplateId`、`agentId`、`mcpCalls`。条件仅 `always`、`issues-found`、`no-issues`；输入选择仅 `current`、`previous-candidate`。不接受任意程序或 shell。

`POST /api/projects/{pid}/productions` 指定 `workflowId` 和 `chapterIds`，或 `startOrder/endOrder`，`maxChapters` 为 1–50。`GET /api/productions/{id}` 返回每步检查点。`POST /api/productions/{id}/{pause|resume|cancel|approve}` 控制状态，批准人工门时传 `stepId`。重启后稳定任务意图找回原子任务，不自动重复未知调用。

`GET/POST /api/embedding-configs` 配置 `kind=embedding|rerank`、模型、维度和 `apiKeyEnv`。`POST /api/projects/{pid}/embeddings/rebuild` 接受 `{"configId":"..."}`。重建调用真实 `/embeddings`，记录模型/维度/来源哈希。`POST /api/projects/{pid}/semantic-search` 在普通查询字段上增加 `mode=keyword|vector|hybrid`、`embeddingProviderId` 和可选 `rerankProviderId`；Rerank 调用真实 `/rerank`。

检索先过滤作品、当前定稿、有效来源和章节边界，再排序；网络调用后再次检查来源。角色知识查询只投影明确获知内容。向量保存在 PostgreSQL JSONB，以 Go 计算余弦与排名融合，无 pgvector 安装要求。

`POST /api/projects/{pid}/context-compression` 接受 `{"query":{"text":"线索","beforeChapterId":"..."},"maxRunes":1000}`，保存带来源 ID/哈希的派生摘要。RunRequest 的 `contextSummaryId` 使用前重新检查来源与章节边界；Agent 无 `read-history` 时不会向模型发送摘要或其来源。摘要不能充当记忆提案的原文证据。

## MCP 真实调用

`GET/POST /api/mcp-configs` 配置远端 Streamable HTTP 地址：

```json
{"name":"只读研究资料","url":"https://tools.example.org/mcp","apiKeyEnv":"RESEARCH_MCP_KEY","allowedTools":["lookup"],"allowLoopback":false,"enabled":true}
```

外部必须 HTTPS，禁止私网、云元数据、回环及保留地址；实际连接时重新检查 DNS 结果并禁止重定向。明确本机服务可设 `allowLoopback:true` 使用 HTTP，此选项仅放行回环地址。未配置密钥变量时请求失败，不会把变量名当密钥发送。

`POST /api/mcp-configs/{id}/tools` 实际握手后发现工具。`POST /api/mcp-configs/{id}/call` 接受 `{"name":"lookup","arguments":{"query":"线索"}}`，仅 allowlist 内且远端声明 `readOnlyHint:true`、`destructiveHint:false` 的工具可调用。远端声明不等于安全证明，应只接可信服务。

Agent 的 `tools` 需精确包含 `mcp:<configId>:<toolName>`。模型任务提交时传 `mcpCalls:[{"configId":"...","name":"lookup","arguments":{}}]`；排队前真实调用并冻结返回资料。同一意图并发或重放只复用持久结果；失败/进程中断意图不会自动重发，需要核对后使用新意图。`GET /api/mcp-call-logs` 返回出站和本地工具调用的状态、耗时与参数哈希，不记录参数正文或密钥。

本地服务为 `POST /mcp`，默认关闭。设置不少于 24 字符的 `MCP_SERVER_TOKEN` 后，客户端使用 Bearer 环境变量引用连接；它是实例管理员凭证，可读取实例作品，应仅供可信客户端使用。浏览器管理员也可通过 CSRF 保护的 `/api/mcp` 调用。工具为 `read-project`、`read-revision`、`search-history`、`read-memory`，均为只读，不暴露文件、SQL 或进程执行。

协议支持 2025-03-26、2025-06-18、2025-11-25 的 initialize/initialized/tools/list/tools/call，并接收 JSON 或 SSE 响应。实现范围以 [MCP Streamable HTTP 规范](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) 和 [生命周期规范](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle) 为依据；不宣称支持 stdio、OAuth 或全部未来扩展。
