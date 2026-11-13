# 阶段 01：作者工作台 Implementation Plan

> 状态更新（2026-11-13）：本文件的原任务与未勾选路径用于设计溯源，不能作为当前缺失功能清单。阶段07已实现高级系统，阶段08已完成Pi作者助手、零章全书规划及页面供应商配置；最新工程状态见[总纲](README.md)，原文学质量/作者计时/Docker实跑未执行项继续保留。


> **For agentic workers:** 使用 `superpowers:executing-plans` 逐任务执行；经用户明确选择后才使用 `superpowers:subagent-driven-development`。用复选框追踪实际完成情况。

**Goal:** 作者能登录自托管实例，创建作品，编辑并保存前三章，查看 AI 候选、审稿和修订差异，导出作品。

**Architecture:** Go API 与 Worker 共用领域模块和数据库；React 使用同源 API；任务先持久化再执行。正文版本和 AI 候选分开保存，选用候选通过版本校验。

**Tech Stack:** Go、chi、PostgreSQL、pgx/sqlc、React/TypeScript、Tiptap、TanStack Query、SSE、前端测试与浏览器测试工具。

**Spec:** [首版设计](../../specs/2026-11-12-novel-workbench-design.md)，第 4、6、8、11 节。

## Global Constraints

- 首版为单作者 Web 自托管；Go 后端、React/TypeScript 前端、PostgreSQL 唯一业务数据库。
- 正文 Revision 不可变；任何 AI 修改生成候选版本，作者决定是否采用。
- 正文和作者确认的设定是权威来源；摘要、状态快照和检索索引必须保留来源并能重建。
- 长期记忆只从已确认正文或作者明确设定形成；AI 提议经过校验和作者确认后才能生效。
- 模型调用与任务记录绑定；浏览器断开不等于取消任务，明确取消才改变任务状态。
- 状态提交校验输入正文版本和基线状态版本；过期结果保留供查看但不得覆盖当前状态。
- 阶段验收使用固定样例和可检查证据；未执行的检查不得记录为通过。

## 前置与范围

进入本阶段前，阶段 00 产物、Provider 和最小运行记录必须通过验证。此阶段只用作者设定、章纲和上一章结尾构建上下文，界面明确尚未启用长期记忆和自动历史检索。

## Task 01-01：作品、正文版本与单作者访问

**Files:** 创建 `cmd/api/main.go`、`internal/auth/session.go`、`middleware.go`、`session_test.go`；`internal/project/service.go`、`http.go`；`internal/manuscript/revision.go`、`service.go`、`http.go`、`revision_test.go`；`migrations/000002_workbench.up.sql` 及回滚文件；`api/openapi.yaml`。

**Interfaces:**

```text
POST   /api/session                         建立会话
DELETE /api/session                         注销
POST   /api/projects                        新建作品
GET    /api/projects/{projectId}              作品与导航树
POST   /api/projects/{projectId}/chapters     新章
GET    /api/chapters/{chapterId}/revisions    历史列表
POST   /api/chapters/{chapterId}/revisions    新版本，包含 expectedCurrentRevisionId
POST   /api/chapters/{chapterId}/finalize     确认指定 revisionId
```

`SaveRevision` 接收 Tiptap JSON、纯文本派生值和期望当前版本；服务器重新生成或校验纯文本和哈希。冲突返回 `409 + currentRevisionId`，不覆盖任何版本。所有对象查询校验其所属作品。

- [ ] 先验证旧版本不可变、两个编辑者基于同一旧版本保存只有一个成功、未登录访问任务与正文被拒绝。
- [ ] 建立作品、卷、章、人物/世界设定和章纲的表与 API；元数据支持编辑，正文只新增版本。
- [ ] 实现单作者凭据与会话、注销、登录限速、写操作的 Origin/CSRF 校验；会话 Cookie 的安全选项区分本地 HTTP 与远程 HTTPS。
- [ ] 实现迁移入口 `go run ./cmd/api migrate up`，服务启动时检查数据库 Schema 版本，不静默修改生产库。

验证：`go test ./internal/auth/... ./internal/project/... ./internal/manuscript/... -count=1`；数据库集成测试覆盖 409 冲突与跨作品 ID 误用。

## Task 01-02：编辑与自动保存

**Files:** 创建 `web/package.json`、锁文件、`web/src/app.tsx`、`features/session/`、`features/project/`、`features/editor/ChapterEditor.tsx`、`useAutosave.ts`、对应测试；`features/planning/BlueprintEditor.tsx`；根 `README.md` 更新运行入口。

**Interfaces:** 自动保存以 `expectedCurrentRevisionId` 为基线；成功后更新本地基线。页面展示“保存中 / 已保存 / 保存失败 / 有版本冲突”，失败时保留编辑缓冲。

- [ ] 配置前端 `test`、`build`、`typecheck`、`test:e2e` 脚本；锁定包管理器版本，前端构建输出由 Go 或反向代理同源提供。
- [ ] 先测试保存失败不丢缓冲、乱序响应不把旧版本设为当前、离开页面前未保存内容可见。
- [ ] 实现左侧作品树、中央正文、右侧章纲/助手区域；优先保证键盘编辑、中文输入法、滚动和明确保存状态。
- [ ] 实现人物、世界设定和章纲的简明表单；数据表与正文编辑分离，避免每次输入同步整个项目。
- [ ] 做一次真实浏览器往返：编辑中文段落、刷新、打开历史版本、恢复为新的当前 Revision；恢复保留历史。

验证：`npm --prefix web run test -- --run`、`npm --prefix web run typecheck`、`npm --prefix web run build`；浏览器检查刷新后内容一致。

## Task 01-03：持久生成任务与可续读输出

**Files:** 创建 `cmd/worker/main.go`、`internal/job/worker.go`、`lease.go`、`events.go`、`http.go`、对应测试；`internal/generation/context.go`、`snapshot.go`、`chapter.go`；扩展 `internal/llm/types.go` 与适配器；前端 `features/runs/RunPanel.tsx`。

**Interfaces:** 保留阶段 00 `Provider.Generate`；新增可选的 `StreamingProvider.Stream(ctx, req, emit func(Delta) error) (Response, error)`，`Delta{Text string}`。不支持流式时返回完成事件。

```text
POST /api/chapters/{chapterId}/runs    { intentId, taskType, inputRevisionId, blueprintVersionId }
GET  /api/runs/{runId}                 状态、输入快照、产物、用量
GET  /api/runs/{runId}/events          SSE，支持 Last-Event-ID
POST /api/runs/{runId}/cancel          持久取消请求
```

事件先写数据库再推送，包含递增事件 ID。订阅连接的请求 context 只控制订阅；Worker 使用任务自己的生命周期和超时。

- [ ] 先测试断开 SSE 后任务继续、重复提交相同 intentId 返回同一 Run、输入旧版本的产物不自动成为当前正文。
- [ ] 用事务领取一个基础租约，限制 Worker 并发；任务开始前保存正文、章纲、Prompt、模型配置和作者指令快照。
- [ ] 按批持久化流式输出，允许刷新后续读；持久化失败时停止继续发起副作用，保存明确错误。
- [ ] 生成结果保存为候选 Artifact，界面提供查看输入、费用与采用入口；状态更新不依赖前端保持在线。

验证：`go test ./internal/job/... ./internal/generation/... -count=1`；端到端测试断线重连前后的内容无重复或缺段。

## Task 01-04：证据审稿与修订接受

**Files:** 创建 `internal/review/http.go`、`anchor.go`、`revision_request.go`、测试；前端 `features/review/IssueList.tsx`、`features/revisions/RevisionDiff.tsx`、`applyHunks.ts`、`applyHunks.test.ts`。

**Interfaces:** 问题引用具体 Revision；修订任务输入仅包含作者选中的 issueId。Diff 使用基线正文与候选正文计算不重叠文本变更段；接受结果创建新 Revision。富文本内不直接套用模型偏移。

- [ ] 测试中文、重复句子、段落移动和部分接受；采用时若当前稿已改变，返回冲突并要求重新比较。
- [ ] 用 `revisionId + hash + quote + prefix + suffix` 定位证据；无法唯一定位的问题保留但不自动高亮错误位置。
- [ ] 允许逐个接受/拒绝不重叠变更段；重叠或结构复杂的改动整段展示，作者编辑后作为新版本保存。
- [ ] 审稿重跑与旧报告保留并行；旧报告显示其正文版本，不套用到新稿上。

验证：`go test ./internal/review/... -count=1`、`npm --prefix web run test -- --run`；人工查看一次中文分段 Diff。

## Task 01-05：导入、导出与完整三章流程

**Files:** 创建 `internal/export/project.go`、`import.go`、测试；`web/tests/e2e/workbench.spec.ts`、`deploy/README.md`；扩展 Compose 使前端/API/Worker 可启动。

**Interfaces:** 项目包包含 `manifest.json`、人物/世界设定、章纲、正文 JSON 与可读 Markdown、Revision 来源；记录格式版本，暂不导出密钥、登录凭据和可执行任务。

- [ ] 测试 TXT/Markdown 导入分章；无法自动分章时提供预览并由作者确认，不猜测后直接覆盖。
- [ ] 测试导出包再导入空作品后正文、章纲、版本对应关系一致，导出包中无密钥或会话。
- [ ] 完成新建作品→前三章编辑/生成→审稿→局部采用→定稿→重启服务→导出的浏览器流程。
- [ ] 登记实际缺陷和结果；长期记忆未上线的限制在应用中清楚显示。

验收命令：`go test ./... -count=1`、`npm --prefix web run test:e2e -- workbench.spec.ts`、`docker compose -f deploy/compose.yaml up --build -d`。

## 阶段退出标准

- [ ] 三章写作与审稿流程可完成，重启不丢已保存内容。
- [ ] AI 不覆盖正文；保存冲突和过期生成结果可见且可处理。
- [ ] 页面刷新可续看任务，取消是明确操作。
- [ ] 登录保护覆盖正文、任务、SSE 和导出。
- [ ] 导入导出往返通过，阶段结果登记于总纲。

## 2026-11-12 实施调整与验证登记

用户后续明确要求先完成整个系统、生成质量和作者试用后置，并授权直接优化架构及同步计划。本文件原任务列表保留为设计溯源，不将未执行的原路径命令或可选研究复选框伪记为完成。实际实现按业务聚合于 `internal/workbench/`，HTTP统一在 `internal/server/`，仍由同一PostgreSQL持久化、事务和版本边界约束；目录名变化不改变作者控制与来源规则。

工程实现和实际验收见 [系统验收记录](../../../../docs/release/acceptance.md)。代码验证使用真实隔离PostgreSQL、Chrome浏览器和离线Provider；真实服务连通验证单独记录。正文、记忆和检索一条完整闭环已经交付，文学质量及作者修改时间按用户要求保留为后续评估。

当前替代设计：索引更新与记忆应用在同一数据库事务提交，不加无必要Outbox；默认关键词检索通过60题评估。阶段07已实现可配置Embedding/Rerank/混合检索及有类型DAG/Production，真实语义模型的质量增益比较仍后置。阶段08由Pi承载对话循环和上下文压缩，Go继续管理权限、版本、预算和作者采用。运行记录/导出格式与当前OpenAPI为接口依据。