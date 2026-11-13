# 高级模块离线测试索引

当前 24 个顶层测试函数使用独立 PostgreSQL schema 和 `httptest` 真实 HTTP 请求，不消耗外部模型费用；函数内含多个正反例。测试存在不等于真实厂商服务或文学质量验收。

| 文件 | 顶层测试数 | 覆盖 |
|---|---:|---|
| `internal/workbench/automation_test.go` | 8 | DAG、受限任务、引用检查、坏流程隔离、人工门、稳定意图、50章边界、候选→审稿→修订、预算前置、暂停与不可变版本 |
| `internal/workbench/integrations_test.go` | 6 | 原生 Ollama/健康检查、多 Provider、Prompt 版本、显式缓存、Agent上下文权限、明确画布选择、界面价格与预算快照 |
| `internal/workbench/semantic_test.go` | 3 | 真 Embedding/Rerank 协议、维度/数值、来源过期、派生摘要、网络调用期间失效 |
| `internal/workbench/mcp_test.go` | 5 | 握手/会话/工具、Agent与allowlist、SSRF/重定向、SSE、无任意命令、并发意图检查点、模型输入冻结 |
| `internal/server/mcp_test.go` | 2 | Bearer/Origin、真实作品读取、禁止变更、配置登录保护与通知传输 |

运行前设置本机 `TEST_DATABASE_URL`，数据库账号应有创建测试 schema 权限：

```powershell
.tools/go/bin/go.exe test ./internal/workbench ./internal/server -run 'TestWorkflow|TestProduction|TestProvider|TestVersionedPrompt|TestConfigured|TestAgentPermissions|TestRestrictedAgent|TestSemantic|TestMCP|TestLocalMCP' -count=1
.tools/go/bin/go.exe vet ./internal/workbench ./internal/server
```

正式验收还应结合任务/尝试/流式/预算/叙事状态全量 Go 测试、前端 typecheck/build、浏览器测试、实际配置服务小规模调用，以及后置作者试用。npm audit 的零告警只是已知漏洞数据库在运行时的结果，不代表不存在未知问题。
