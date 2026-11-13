# 阶段 00：技术与质量基线 Implementation Plan

> **For agentic workers:** 使用 `superpowers:executing-plans` 逐任务执行；经用户明确选择后才使用 `superpowers:subagent-driven-development`。用复选框追踪实际完成情况。

**Goal:** 用固定的三章样例跑通“章纲→草稿→有证据的审稿”，建立可检查的质量、费用和故障基线。

**Architecture:** Go 命令行入口调用统一 Provider，PostgreSQL 保存任务、尝试与原始产物；先无编辑器和通用工作流。真实模型运行显式开启，默认测试只调用假 Provider。

**Tech Stack:** Go、PostgreSQL、pgx、sqlc、Docker Compose、兼容模型接口、Go testing。

**Spec:** [首版设计](../../specs/2026-11-12-novel-workbench-design.md)，重点为第 5、10、13 节。

## Global Constraints

- 首版为单作者 Web 自托管；Go 后端、React/TypeScript 前端、PostgreSQL 唯一业务数据库。
- 正文 Revision 不可变；任何 AI 修改生成候选版本，作者决定是否采用。
- 正文和作者确认的设定是权威来源；摘要、状态快照和检索索引必须保留来源并能重建。
- 长期记忆只从已确认正文或作者明确设定形成；AI 提议经过校验和作者确认后才能生效。
- 模型调用与任务记录绑定；浏览器断开不等于取消任务，明确取消才改变任务状态。
- 状态提交校验输入正文版本和基线状态版本；过期结果保留供查看但不得覆盖当前状态。
- 阶段验收使用固定样例和可检查证据；未执行的检查不得记录为通过。

## 入口与范围

前置：审阅设计，确认用户可接受 PostgreSQL 自托管依赖。阶段 00 不要求作者先提供完整小说，也不下载大量参考项目。

交付：能运行的验证程序、三章原创测试材料、运行结果 JSON、模型参数与费用快照、两个参考仓库的核验笔记。模型写作质量不足时先定位原因，不继续铺开 UI。

## Task 00-01：建立受控故事与评估口径

**Files:** 在 `evaluation/fixtures/bridge-3/` 内创建 `story.md`、`chapters/01.md` 至 `chapters/03.md`、`blueprints.json`、`expected-facts.json`；创建 `evaluation/protocol.md` 和 `evaluation/repositories.md`。

**Interfaces:** 样例统一使用稳定 ID：人物 `lin-chuan`、`shen-yue`；物品 `bronze-seal`；秘密 `seal-origin`。标准答案包括事实、来源引用、首次揭示位置、角色知道/不知道的信息和至少一个错误信念。

- [x] 写三章短样例：第一章林川左臂受伤并拾得铜印；第二章沈月认出其来源但只告诉林川“与王都有关”；第三章二人准备开箱。正文明确留下可引用证据。
- [x] 标注 12 个可判定问题，其中 8 个应回答、4 个应承认未知；另做一份包含伤势消失、物品重复持有、秘密泄露的错误草稿。
- [x] 写明正确检测与误报的计算口径，区分事实抽取和文学评价；采用[评估协议](evaluation-protocol.md)中的实验配置与记录字段。
- [x] 核验 Pagekin、NstAgent 的仓库地址、固定提交、许可证文件和本阶段关注的代码位置；分别记录实际观察与项目自述，无法运行时保留具体原因，不据此认定能力成立。

验收：另一位读者仅凭样例和证据能复核 12 个答案；正确草稿和错误草稿不得共用活动索引。

## Task 00-02：建立验证程序和最小持久运行记录

**Files:** 创建 `go.mod`、`go.sum`、`cmd/eval/main.go`、`internal/llm/types.go`、`internal/generation/run.go`、`internal/job/intent.go`、`migrations/000001_baseline.up.sql`、对应 `.down.sql`、`sqlc.yaml`、`deploy/compose.yaml`、`.env.example`、`.gitignore`、`README.md`。

**Interfaces:** 在 `internal/llm/types.go` 定义以下契约；适配器自行处理兼容接口的具体协议差异。

```go
type Message struct { Role string; Content string }
type Request struct {
    Model string
    Messages []Message
    MaxOutputTokens int
    Temperature *float64
    ResponseSchema []byte
}
type Usage struct {
    InputTokens int64
    OutputTokens int64
    CachedInputTokens int64
    Reported bool
}
type Response struct {
    Text string
    Raw []byte
    ProviderRequestID string
    Usage Usage
}
type Provider interface {
    Generate(context.Context, Request) (Response, error)
}
```

运行记录包含 `intent_id / job_id / attempt_id / input_hash / prompt_version / provider / model / started_at / finished_at / outcome / raw_artifact_id / usage / price_snapshot`。同一意图只创建一个业务任务；每次外部调用有独立 attempt。

- [x] 检查现有目录和 Git 状态；开始实现时才初始化代码仓库，配置忽略模型密钥、私密原稿和真实模型原始输出。
- [x] 核对并锁定受支持工具链、镜像和依赖版本，记录初始化日期与来源；建立最小项目表、任务表、尝试表和产物表，执行迁移及回滚验证。
- [x] 先验证重复 `intent_id` 不会创建第二个任务、attempt 在外部调用前已提交；然后实现对应唯一约束与持久化调用顺序。
- [x] 提供 `eval` 的 `--fixture`、`--provider fake|configured`、`--output` 参数；真实调用必须显式选择 `configured`，缺失凭据时在发请求前退出。

验证：`go test ./internal/job/... ./internal/generation/... -count=1`；期望意图去重和尝试记录顺序检查通过，重建数据库后迁移仍可完整执行。

## Task 00-03：实现 Provider、故障分类与结构化审稿

**Files:** 创建 `internal/llm/fake.go`、`compatible.go`、`failure.go`、对应测试；`internal/review/issue.go`、`validate.go`、`validate_test.go`；`prompts/writing/chapter-v1.md`、`prompts/review/continuity-v1.md`、`schemas/review-issues-v1.json`。

**Interfaces:** `FailureDisposition(err error, mayHaveExecuted bool) string` 返回 `Retryable / Permanent / NeedsReconciliation`；`ValidateIssues(source string, issues []Issue) error` 校验报告证据确实来自该输入。

```go
type Issue struct {
    Kind string `json:"kind"`
    Severity string `json:"severity"`
    Quote string `json:"quote"`
    Explanation string `json:"explanation"`
    SuggestedAction string `json:"suggestedAction"`
}
```

- [x] 使用本地 HTTP 测试服务覆盖成功、429、无效密钥、非法 JSON、连接中断和请求超时；不向真实接口发送测试请求。
- [x] 用表驱动测试要求：无效密钥为 `Permanent`；尚未提交请求的连接失败可为 `Retryable`；可能已执行且结果未知为 `NeedsReconciliation`。
- [x] 实现适配器超时、响应大小上限、原始响应保存和错误分类；不自动重试结果未知的生成。
- [x] 校验 JSON Schema 与证据引用，保存解析错误及原始输出；格式修复不得产生原文没有的事实。若修复仍失败，则产生可查看失败结果。
- [x] 为“证据引用不在输入中”增加真实断言，防止只验证 JSON 外形。

```go
func TestValidateIssuesRejectsInventedEvidence(t *testing.T) {
    issues := []Issue{{
        Kind: "Continuity", Severity: "Major", Quote: "铜印已经熔毁",
        Explanation: "与前文冲突", SuggestedAction: "核对物品状态",
    }}
    if err := ValidateIssues("林川把铜印放进口袋。", issues); err == nil {
        t.Fatal("invented evidence must be rejected")
    }
}
```

验证：`go test ./internal/llm/... ./internal/review/... -count=1`；未知结果不产生第二次网络调用，伪造引用被拒绝。

## Task 00-04：完成端到端实验并决定是否进入工作台阶段

**Files:** 完成 `cmd/eval/main.go`、`internal/generation/pipeline.go`、`pipeline_test.go`；创建本次实验结果及 `evaluation/results/<run-id>/summary.md`。

**Interfaces:** 验证入口接收固定章纲、已知设定和前文，输出草稿、审稿报告、调用明细和失败状态；使用 `Provider` 注入假实现或真实实现。

- [x] 先用假 Provider 检查完整产物链、输入快照和失败产物均能在数据库查回，再接入真实实验。
- [x] 运行同一批章纲三次真实实验，记录成功率、Token、费用、时间和人工评价；不得在测试日志泄露凭据。
- [x] 对错误草稿检查至少三个已标注矛盾，对正确草稿单独记录误报；结果较差时保留证据并调整提示或配置。
- [x] 整理执行说明和本阶段实际耗时，用结果估算阶段 01 工作量；提交本地验证结果的脱敏摘要。

```powershell
docker compose -f deploy/compose.yaml up -d postgres
go test ./... -count=1
go run ./cmd/eval --fixture evaluation/fixtures/bridge-3 --provider fake --output evaluation/results/baseline-fake
go run ./cmd/eval --fixture evaluation/fixtures/bridge-3 --provider configured --output evaluation/results/baseline-live
```

最后一条消耗外部模型额度，仅在已配置且用户授权使用该接口的条件下执行。

## 阶段退出标准

- [x] 三章材料和 12 项标准答案均有来源证据，至少 3 项错误注入可被核对。
- [x] 离线端到端链路和持久化测试通过；真实运行的成功与失败均有记录。
- [x] 未知模型结果不会被当成安全重试；费用未知不会显示成 0。
- [ ] 作者实际阅读至少一份生成草稿，评价质量（用户后续明确将此项后置，不再阻塞开发，见D-010）。
- [x] README 中登记工具链、依赖版本、启动方法和实验结果路径。

真实模型效果不合格则留在本阶段调整；记录具体差距，不能以“之后加 RAG”代替本阶段结论。

## 2026-11-12 执行记录

工程验证和三轮真实模型实验已完成。作者阅读原前置按用户新指示后置，不再阻塞后续开发。阶段00当时的实际命令、34项顶层测试、故障注入、真实用量及局限见 [阶段00验收记录](../../../../evaluation/baseline-status.md)，逐项语义评分见 [真实实验标注](../../../../evaluation/live-annotations.md)；最新系统测试以系统验收为准。

真实运行配置为 `gpt-6.1-sol` / Responses / `store:false`，三轮各4次调用全部成功，没有额外重试。模型金额未知，按协议保留 null；12题模型问答未执行。前两章是冻结前文，只起草第三章，不声称三章逐章生成或A/B/C增益已完成。

计划引用的 superpowers 技能在本机未安装，未声称使用。按本文件任务顺序实现，并通过当前会话的并行审阅和真实数据库测试核验。最初若干测试因环境或实现缺失未能执行，未记录为通过；后续集成测试均设置真实TEST_DATABASE_URL实跑。
