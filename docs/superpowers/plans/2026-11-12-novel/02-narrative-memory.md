# 阶段 02：有证据的叙事记忆 Implementation Plan

> 状态更新（2026-11-13）：本文件的原任务与未勾选路径用于设计溯源，不能作为当前缺失功能清单。阶段07已实现高级系统，阶段08已完成Pi作者助手、零章全书规划及页面供应商配置；最新工程状态见[总纲](README.md)，原文学质量/作者计时/Docker实跑未执行项继续保留。


> **For agentic workers:** 使用 `superpowers:executing-plans` 逐任务执行；经用户明确选择后才使用 `superpowers:subagent-driven-development`。用复选框追踪实际完成情况。

**Goal:** 十章写作中，系统能提出、审核并提交五类有来源的状态变化，准确区分事实与人物信念。

**Architecture:** 记忆提取生成候选 ChangeSet；规则与作者审核后，事务内创建新状态版本。检索与摘要均不能绕过确认流程成为事实。

**Tech Stack:** Go、PostgreSQL JSONB/关系表、JSON Schema、React；沿用阶段 01 任务与版本契约。

**Spec:** [首版设计](../../specs/2026-11-12-novel-workbench-design.md)，第 6、7、8 节。

## Global Constraints

- 首版为单作者 Web 自托管；Go 后端、React/TypeScript 前端、PostgreSQL 唯一业务数据库。
- 正文 Revision 不可变；任何 AI 修改生成候选版本，作者决定是否采用。
- 正文和作者确认的设定是权威来源；摘要、状态快照和检索索引必须保留来源并能重建。
- 长期记忆只从已确认正文或作者明确设定形成；AI 提议经过校验和作者确认后才能生效。
- 模型调用与任务记录绑定；浏览器断开不等于取消任务，明确取消才改变任务状态。
- 状态提交校验输入正文版本和基线状态版本；过期结果保留供查看但不得覆盖当前状态。
- 阶段验收使用固定样例和可检查证据；未执行的检查不得记录为通过。

## 前置与边界

阶段 01 的正文定稿、任务输入快照、版本冲突和证据定位已可用。本阶段实现位置、伤势、物品归属、人物知识、伏笔五类状态；不做完整世界模拟和自主遗忘。

## Task 02-01：定义事实、信念和叙事位置

**Files:** 创建 `internal/narrative/fact.go`、`belief.go`、`position.go`、`state.go`、测试；`schemas/memory-change-set-v1.json`；`migrations/000003_narrative.up.sql` 及回滚文件。

**Interfaces:**

```go
type NarrativePosition struct {
    ChapterID string
    ChapterOrder int
    SceneOrder int
}
type StoryTime struct {
    Day *int
    Label string
    Precision string // Day, Approximate, Unknown
}
type Evidence struct {
    RevisionID string
    PlainTextHash string
    Quote string
    Prefix string
    Suffix string
}
```

事实记录 `subject / predicate / object / validTime / revealedAt / evidence / status`；信念另外记录 `characterId / acquiredAt / sourceKind / confidence / truthStatus`。status 固定为 `Proposed / Confirmed / Rejected / Superseded`；置信度不是事实已确认的替代条件。

- [ ] 先测试：同一事实允许角色 A 知道而角色 B 不知道；错误信念不能覆盖客观事实；未知故事时间不被转成第 0 天。
- [ ] 用稳定实体 ID 表示物品与人物；物品归属变化必须引用旧归属或明确“此前未知”。
- [ ] 保留作者手工设定的独立来源记录；模型遇到冲突只能提出问题。
- [ ] 为快照标注 `schemaVersion` 和源 Revision；叙事顺序与故事时间的字段分开存储。

验证：`go test ./internal/narrative/... -run 'TestFact|TestBelief|TestStoryTime' -count=1`。

## Task 02-02：提取与校验记忆变更

**Files:** 创建 `internal/narrative/extract.go`、`changeset.go`、`validate.go`、测试；`prompts/memory/extract-v1.md`；任务类型新增 `memory-extraction`。

**Interfaces:** `ProposeChangeSet(projectId, chapterId, sourceRevisionId, baseStateVersionId)` 创建提取任务；输出 ChangeSet 关联相同输入。`ValidateChangeSet` 返回阻塞错误与需确认警告；未知实体、空证据和来源未定稿是阻塞错误。

- [ ] 用假 Provider 输出“沈月知道秘密，林川只知道王都线索”；验证正确落为不同角色知识。
- [ ] 注入不存在的证据、错误作品 ID、新实体悬空引用和已改变的源版本，先写拒绝测试，再实现校验。
- [ ] 提取器只能读取本次定稿正文、相关旧状态和作者设定，不读取未来章纲作为已发生事实。
- [ ] 新实体临时引用通过服务端 ID 映射解决；模型直接给出的数据库新 ID 不被信任。
- [ ] 允许作者拒绝、编辑单个提议；编辑结果保留模型原提议和作者处理记录。

验证：`go test ./internal/narrative/... -run 'TestExtract|TestValidateChangeSet' -count=1`。

## Task 02-03：事务性应用与上下文投影

**Files:** 创建 `internal/narrative/apply.go`、`repository.go`、`context.go`、`apply_test.go`、`context_test.go`；`internal/generation/context.go` 扩展状态引用；`migrations/000004_outbox.up.sql` 及回滚文件。

**Interfaces:**

```go
type ApplyCommand struct {
    ProjectID string
    ChangeSetID string
    ExpectedStateVersionID string
    SourceRevisionID string
}
type ApplyResult struct {
    StateVersionID string
    AlreadyApplied bool
}
```

`ApplyChangeSet(ctx, cmd)` 在事务中校验基线和活动定稿版本，保存变更、快照、依赖边和派生更新 Outbox。唯一约束保证同一 ChangeSet 最多应用一次；状态冲突返回 409。

- [ ] 先测试两份 ChangeSet 竞争同一基线、重复提交、源正文已换版和事务中断；不得留下半个状态版本。
- [ ] 生成上下文只注入相关人物/物品/伏笔的已确认状态，并保存确切 stateVersionId。
- [ ] 角色知识输入由人物可知信息投影；作者真相另区展示并带明确约束，不把计划和秘密无区别混入对话上下文。
- [ ] 人工锁定事实与自动提议冲突时阻止应用并列出证据；不得直接替换旧事实。

验证：`go test ./internal/narrative/... ./internal/generation/... -count=1`；并发测试要求只产生一个后继状态版本。

## Task 02-04：记忆审核界面与十章实验

**Files:** 创建 `web/src/features/memory/ChangeSetReview.tsx`、`StatePanel.tsx`、`EvidenceLink.tsx`、测试；`web/tests/e2e/memory.spec.ts`；`evaluation/fixtures/bridge-10/`。

**Interfaces:**

```text
POST /api/chapters/{chapterId}/memory-proposals
GET  /api/memory-proposals/{changeSetId}
POST /api/memory-proposals/{changeSetId}/decisions  单项接受/拒绝/编辑
POST /api/memory-proposals/{changeSetId}/apply      携带基线与源版本
GET  /api/projects/{projectId}/state                状态版本、健康状态和来源
```

- [ ] 界面显示旧值、新值、证据、人物是否知道、警告；接受前可跳到准确正文版本。
- [ ] 明确区分正文定稿与记忆就绪；下章起草需要的状态未确认时给出可操作阻塞原因。
- [ ] 建十章受控故事，五类状态均跨至少两章变化；包含物品转手、伤势延续和不同角色对秘密的认知。
- [ ] 比较评估协议中的 A/B 配置，记录实际 Token、事实错误、误报和作者处理时间。

验证：`npm --prefix web run test:e2e -- memory.spec.ts`、`go run ./cmd/eval --fixture evaluation/fixtures/bridge-10 --provider fake --output evaluation/results/memory-fake`；真实模型单独执行并记录。

## 阶段退出标准

- [ ] 五类状态全部有正确应用、拒绝和来源追踪样例。
- [ ] 角色未知/误信不被自动纠正；未来计划不变成事实。
- [ ] 重复提交、基线竞争、正文换版测试通过。
- [ ] 十章实验记录实际收益与不足，作者能完成审核而无需手工重建整套设定。
- [ ] 文档说明本阶段尚未支持旧稿修改后的自动重建；该风险由阶段 03 关闭。

## 2026-11-12 实施调整与验证登记

用户后续明确要求先完成整个系统、生成质量和作者试用后置，并授权直接优化架构及同步计划。本文件原任务列表保留为设计溯源，不将未执行的原路径命令或可选研究复选框伪记为完成。实际实现按业务聚合于 `internal/workbench/`，HTTP统一在 `internal/server/`，仍由同一PostgreSQL持久化、事务和版本边界约束；目录名变化不改变作者控制与来源规则。

工程实现和实际验收见 [系统验收记录](../../../../docs/release/acceptance.md)。代码验证使用真实隔离PostgreSQL、Chrome浏览器和离线Provider；真实服务连通验证单独记录。正文、记忆和检索一条完整闭环已经交付，文学质量及作者修改时间按用户要求保留为后续评估。

当前替代设计：索引更新与记忆应用在同一数据库事务提交，不加无必要Outbox；默认关键词检索通过60题评估。阶段07已实现可配置Embedding/Rerank/混合检索及有类型DAG/Production，真实语义模型的质量增益比较仍后置。阶段08由Pi承载对话循环和上下文压缩，Go继续管理权限、版本、预算和作者采用。运行记录/导出格式与当前OpenAPI为接口依据。