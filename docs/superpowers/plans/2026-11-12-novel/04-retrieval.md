# 阶段 04：受约束的历史检索 Implementation Plan

> 状态更新（2026-11-13）：本文件的原任务与未勾选路径用于设计溯源，不能作为当前缺失功能清单。阶段07已实现高级系统，阶段08已完成Pi作者助手、零章全书规划及页面供应商配置；最新工程状态见[总纲](README.md)，原文学质量/作者计时/Docker实跑未执行项继续保留。


> **For agentic workers:** 使用 `superpowers:executing-plans` 逐任务执行；经用户明确选择后才使用 `superpowers:subagent-driven-development`。用复选框追踪实际完成情况。

**Goal:** 历史检索提供有来源、无失效版本泄露的上下文，并用保留测试集证明其对写作的价值。

**Architecture:** PostgreSQL 保存派生检索块和活动来源标记；先做实体与关键词基线，再比较可选向量检索。作品、活动版本和可见性过滤是硬条件，排序得分不能绕过。

**Tech Stack:** Go、PostgreSQL FTS、可选 pgvector/外部 Embedding 接口、React 引用面板。

**Spec:** [首版设计](../../specs/2026-11-12-novel-workbench-design.md)，第 5、7、9、13 节；[评估协议](evaluation-protocol.md)。

## Global Constraints

- 首版为单作者 Web 自托管；Go 后端、React/TypeScript 前端、PostgreSQL 唯一业务数据库。
- 正文 Revision 不可变；任何 AI 修改生成候选版本，作者决定是否采用。
- 正文和作者确认的设定是权威来源；摘要、状态快照和检索索引必须保留来源并能重建。
- 长期记忆只从已确认正文或作者明确设定形成；AI 提议经过校验和作者确认后才能生效。
- 模型调用与任务记录绑定；浏览器断开不等于取消任务，明确取消才改变任务状态。
- 状态提交校验输入正文版本和基线状态版本；过期结果保留供查看但不得覆盖当前状态。
- 阶段验收使用固定样例和可检查证据；未执行的检查不得记录为通过。

## 前置与边界

阶段 03 已能提供活动版本、失效标记、叙事位置与重建状态。不把未采用草稿、未来章纲和外部参考小说混进活动正文检索集合；不以全文进入模型上下文冒充检索。

## Task 04-01：来源一致的切块与索引更新

**Files:** 创建 `internal/retrieval/chunk.go`、`index.go`、`index_test.go`；`migrations/000006_retrieval.up.sql` 及回滚文件；Outbox 消费器增加索引任务。

**Interfaces:** Chunk 保存 `projectId / chapterId / revisionId / sceneId / entityIds / narrativePosition / storyTime / sourceHash / visibility / content`；`IndexRevision` 在重复执行时替换同一来源的派生块，不制造重复引用。

- [ ] 先测试段落/场景边界、超长段落二次切分、来源定位和多次索引幂等。
- [ ] 原文场景和确认事实分别切块，摘要标记为摘要并保留原文链接；摘要不能成为唯一证据。
- [ ] 数据库换版事务立刻使旧块不可见，异步索引尚未完成时显示索引进度；不能先召回旧块再等待删除。
- [ ] 全量重建只依赖权威正文/事实，删除索引后可重建同一组来源。

验证：`go test ./internal/retrieval/... -run 'TestIndex|TestChunk' -count=1`。

## Task 04-02：关键词与实体检索基线

**Files:** 创建 `internal/retrieval/tokenize.go`、`keyword.go`、`query.go`、`visibility.go`、对应测试；`internal/generation/context.go` 加入引用预算。

**Interfaces:**

```go
type Query struct {
    ProjectID string
    ProjectEpoch int64
    BeforeChapterID string
    CharacterID string
    Purpose string // AuthorReview, CharacterKnowledge
    Text string
    EntityIDs []string
    Limit int
}
```

`Search(ctx, Query)` 返回已过滤的 Chunk 与证据；`CharacterKnowledge` 只允许该角色已获取的信息，`AuthorReview` 可读作者层真相但仍排除未来未发生计划与失效版本。

- [ ] 写隔离测试：其他作品、旧定稿、未来章节、未采用草稿、角色未知秘密均不得出现在不允许的查询中。
- [ ] 实现人物别名与实体匹配；中文文本使用受测的中文二字切词与英文词项配合 `simple` FTS，另保留原文精确短语匹配。分词策略与版本随索引保存。
- [ ] 无答案返回空证据和明确缺失，不生成貌似相关的事实；索引不完整时返回警告。
- [ ] 上下文超预算时保留硬规则与章纲，裁剪低优先级历史引用；记录实际保留/丢弃的来源。

验证：`go test ./internal/retrieval/... ./internal/generation/... -count=1`；中文姓名、别名、物品及同义表达分开统计。

## Task 04-03：向量链路对照与融合

**Files:** 创建 `internal/retrieval/embedding.go`、`vector.go`、`fusion.go`、测试；`evaluation/fixtures/retrieval-60/questions.json`、`qrels.json`、`split.json`；扩展 `cmd/eval` 的 `retrieval` 子命令。

**Interfaces:** Embedding 配置记录 `provider / model / dimension / indexVersion`；不允许不同维度或模型的向量混查。多路召回使用按排名融合，硬过滤在各路检索和最终输出均校验。

- [ ] 先为 60 个问题标注相关来源和 40/20 开发/保留分组；包含至少 10 个无答案或不可见信息问题。
- [ ] 用假 Embedding 验证维度不符、索引模型切换和失效过滤；真实质量测量单独执行。
- [ ] 比较关键词、向量、融合三条链路，记录 Recall@10、无答案行为、延迟和成本；只在开发集调参。
- [ ] 向量在保留集没有稳定收益时保留关键词为默认；形成明确结论，而不是为完成技术栈强制启用。
- [ ] Reranker 仅作为可选对照项，证明增益后才纳入运行依赖。

```powershell
go run ./cmd/eval retrieval --fixture evaluation/fixtures/retrieval-60 --mode keyword --split holdout
go run ./cmd/eval retrieval --fixture evaluation/fixtures/retrieval-60 --mode hybrid --split holdout
```

真实向量链路的外部服务消耗需记录；离线测试不下载大模型或要求 GPU。

## Task 04-04：引用透明与写作对照

**Files:** 创建 `web/src/features/context/ContextInspector.tsx`、`CitationCard.tsx`；`web/tests/e2e/retrieval.spec.ts`；更新上下文快照导出格式。

- [ ] 展示本次运行使用的设定、stateVersionId、历史章节、引用片段与召回原因；能跳到当时的来源版本。
- [ ] 旧稿变化后引用显示历史属性，新的查询不再使用；后台索引未就绪时提示范围。
- [ ] 对相同章纲比较 B/C 配置及近似等预算对照，记录事实遗漏、误报和作者修改时间。
- [ ] 提供作品级手工重建索引入口，不让作者管理向量 ID 和底层分词参数。

验证：`npm --prefix web run test:e2e -- retrieval.spec.ts`，以及本阶段全套检索测试。

## 阶段退出标准

- [ ] 隔离与可见性测试要求的错误引用数量为 0。
- [ ] 至少 60 个标注问题，保留集 Recall@10 达到协议门槛；失败时列出具体缺失类型。
- [ ] 索引可从权威来源重建，换版不会等待索引更新才隐藏旧事实。
- [ ] 默认检索路线有实测依据，向量与 Reranker 不因文档列出就自动成为必需组件。
- [ ] 作者能理解本次 AI 使用了哪些历史信息。

## 2026-11-12 实施调整与验证登记

用户后续明确要求先完成整个系统、生成质量和作者试用后置，并授权直接优化架构及同步计划。本文件原任务列表保留为设计溯源，不将未执行的原路径命令或可选研究复选框伪记为完成。实际实现按业务聚合于 `internal/workbench/`，HTTP统一在 `internal/server/`，仍由同一PostgreSQL持久化、事务和版本边界约束；目录名变化不改变作者控制与来源规则。

工程实现和实际验收见 [系统验收记录](../../../../docs/release/acceptance.md)。代码验证使用真实隔离PostgreSQL、Chrome浏览器和离线Provider；真实服务连通验证单独记录。正文、记忆和检索一条完整闭环已经交付，文学质量及作者修改时间按用户要求保留为后续评估。

当前替代设计：索引更新与记忆应用在同一数据库事务提交，不加无必要Outbox；默认关键词检索通过60题评估。阶段07已实现可配置Embedding/Rerank/混合检索及有类型DAG/Production，真实语义模型的质量增益比较仍后置。阶段08由Pi承载对话循环和上下文压缩，Go继续管理权限、版本、预算和作者采用。运行记录/导出格式与当前OpenAPI为接口依据。