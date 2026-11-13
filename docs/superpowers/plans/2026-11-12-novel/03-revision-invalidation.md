# 阶段 03：旧稿影响与状态重建 Implementation Plan

> 状态更新（2026-11-13）：本文件的原任务与未勾选路径用于设计溯源，不能作为当前缺失功能清单。阶段07已实现高级系统，阶段08已完成Pi作者助手、零章全书规划及页面供应商配置；最新工程状态见[总纲](README.md)，原文学质量/作者计时/Docker实跑未执行项继续保留。


> **For agentic workers:** 使用 `superpowers:executing-plans` 逐任务执行；经用户明确选择后才使用 `superpowers:subagent-driven-development`。用复选框追踪实际完成情况。

**Goal:** 作者改变早期定稿、删除或重排章节后，能看到受影响内容，避免继续使用旧事实，并有序重建状态。

**Architecture:** 用不可变来源和依赖边标记派生产物；正文变化和失效标记在同一事务提交。先采取从变化点向后的保守状态失效策略，重建按章串行复用记忆审核流程。

**Tech Stack:** Go、PostgreSQL、现有任务与 Outbox、React 影响报告。

**Spec:** [首版设计](../../specs/2026-11-12-novel-workbench-design.md)，第 6、9 节。

## Global Constraints

- 首版为单作者 Web 自托管；Go 后端、React/TypeScript 前端、PostgreSQL 唯一业务数据库。
- 正文 Revision 不可变；任何 AI 修改生成候选版本，作者决定是否采用。
- 正文和作者确认的设定是权威来源；摘要、状态快照和检索索引必须保留来源并能重建。
- 长期记忆只从已确认正文或作者明确设定形成；AI 提议经过校验和作者确认后才能生效。
- 模型调用与任务记录绑定；浏览器断开不等于取消任务，明确取消才改变任务状态。
- 状态提交校验输入正文版本和基线状态版本；过期结果保留供查看但不得覆盖当前状态。
- 阶段验收使用固定样例和可检查证据；未执行的检查不得记录为通过。

## 前置与边界

依赖阶段 02 的来源、状态基线、变更审核和原子提交。本阶段不自动重写后文、不自动选择两个互相冲突的事实，也不实现并行故事分支。

## Task 03-01：依赖关系与影响计算

**Files:** 创建 `internal/impact/dependency.go`、`service.go`、`service_test.go`；`migrations/000005_impact.up.sql` 及回滚文件；扩展记忆、生成和审稿产物的来源登记。

**Interfaces:**

```go
type SourceRef struct { Kind string; ID string; VersionID string }
type ImpactItem struct {
    Kind string
    ID string
    Reason string
    RequiredAction string
}
type ImpactReport struct {
    ProjectID string
    EarliestChapterID string
    Items []ImpactItem
}
```

`AnalyzeChange(projectId, oldSource, newSource)` 输出待失效状态、待复核正文和待更新索引的清单。内部 ID 使用稳定章节 ID，不以展示章节号充当外键。

- [ ] 用第 5 章铜印由“转交”变为“毁坏”的固定依赖图，先断言第 5 章起的后继状态全部受影响、第 1–4 章仍有效。
- [ ] 写角色信念来源改变、人物别名修改、章纲修改三个用例；章纲变化使依赖草稿待复核，不能回头改已发生事实。
- [ ] 实现循环防护与作品边界；影响图只遍历当前作品。
- [ ] 历史稿保留，活动稿的复核标记不能改变其正文内容。

验证：`go test ./internal/impact/... -run 'TestAnalyzeChange|TestDependencyIsolation' -count=1`。

## Task 03-02：换版、删除和重排的原子失效

**Files:** 修改 `internal/manuscript/service.go`；创建 `internal/impact/invalidate.go`、`invalidate_test.go`；扩展 `internal/generation/context.go` 和状态健康检查。

**Interfaces:** 项目健康状态为 `Ready / MemoryPending / RebuildRequired`。`FinalizeRevision` 在替换已有定稿时同步增加作品版本 `projectEpoch` 并标记依赖失效；任务输入增加 epoch，提交时校验。

```text
POST /api/chapters/{chapterId}/impact-preview   预览候选换版影响
POST /api/chapters/{chapterId}/finalize         确认换版并返回 impactReportId
POST /api/projects/{projectId}/chapter-order   expectedProjectEpoch + chapterIds
DELETE /api/chapters/{chapterId}               软删除并保留历史来源
```

- [ ] 先测试“换版成功但失效标记失败”时整个事务回滚；旧状态不可在两次提交之间短暂被当成有效新状态。
- [ ] 对删除和重排采用相同失效入口，从最早变化的叙事位置开始处理。
- [ ] 生成前检查状态健康；任务执行中发生换版时，结果标为过期候选，禁止替换当前稿或提交状态。
- [ ] 后续正文只标记待复核，编辑、导出和历史查看继续可用；界面说明哪些生成被阻塞及原因。

验证：`go test ./internal/manuscript/... ./internal/impact/... ./internal/generation/... -count=1`。

## Task 03-03：从有效检查点重建状态

**Files:** 创建 `internal/impact/rebuild.go`、`rebuild_test.go`；扩展任务类型 `state-rebuild`；创建前端 `web/src/features/impact/RebuildPanel.tsx`。

**Interfaces:** `StartRebuild(projectId, expectedProjectEpoch)` 固定当前章节顺序和定稿版本清单；每章产生独立 MemoryChangeSet，复用阶段 02 的审核和应用接口。

- [ ] 先验证重建从最后一个有效快照开始，不从任意最近快照开始；中间拒绝变更时停在该章并保存进度。
- [ ] 重建逐章提取与确认；失效快照用于历史比较，不能作为新状态基线。
- [ ] 重建期间正文再变化，暂停当前重建并显示新的影响范围；不得混合两个 epoch 的状态。
- [ ] 人工锁定事实保留出处；新正文与人工设定冲突时列出双向证据，由作者处理。
- [ ] 为每一步保存输入与产物，重启后能显示最后成功位置；精细恢复和租约故障在阶段 05 加固。

验证：`go test ./internal/impact/... -run 'TestRebuild' -count=1`。

## Task 03-04：作者影响报告与回改实验

**Files:** 创建 `web/src/features/impact/ImpactReport.tsx`、`AffectedChapterList.tsx`；`web/tests/e2e/revision-impact.spec.ts`；`evaluation/fixtures/bridge-10/edits.json`。

**Interfaces:** 报告分别展示“需要重建的状态”“建议复核的正文”“需重新生成的报告”；给出来源变化、受影响原因和原文跳转。

- [ ] 浏览器执行第 5 章物品改写，确认第 6–10 章有提示，系统没有自动改动这些章节。
- [ ] 执行第二次角色知识来源改写，确认未来检索不会继续使用旧定稿为证据。
- [ ] 做一次章节重排，验证依赖使用稳定 ID，展示顺序改变后可见性判断同步变化。
- [ ] 完成重建并检查五类关键状态与人工标准答案一致；保存重建费用与作者处理时间。

验证：`npm --prefix web run test:e2e -- revision-impact.spec.ts`；运行阶段 02 的记忆回归测试，保证正常向前写作仍可用。

## 阶段退出标准

- [ ] 两次旧章事实修改和一次重排均产生正确的影响范围。
- [ ] 过期状态不会进入新生成，过期运行结果不能覆盖新稿。
- [ ] 下游正文保留，作者可选择如何修订。
- [ ] 重建可在冲突处停下并继续，人工设定没有被静默覆盖。
- [ ] 历史版本和失效证据仍可查看；活动来源与历史来源清楚区分。

## 2026-11-12 实施调整与验证登记

用户后续明确要求先完成整个系统、生成质量和作者试用后置，并授权直接优化架构及同步计划。本文件原任务列表保留为设计溯源，不将未执行的原路径命令或可选研究复选框伪记为完成。实际实现按业务聚合于 `internal/workbench/`，HTTP统一在 `internal/server/`，仍由同一PostgreSQL持久化、事务和版本边界约束；目录名变化不改变作者控制与来源规则。

工程实现和实际验收见 [系统验收记录](../../../../docs/release/acceptance.md)。代码验证使用真实隔离PostgreSQL、Chrome浏览器和离线Provider；真实服务连通验证单独记录。正文、记忆和检索一条完整闭环已经交付，文学质量及作者修改时间按用户要求保留为后续评估。

当前替代设计：索引更新与记忆应用在同一数据库事务提交，不加无必要Outbox；默认关键词检索通过60题评估。阶段07已实现可配置Embedding/Rerank/混合检索及有类型DAG/Production，真实语义模型的质量增益比较仍后置。阶段08由Pi承载对话循环和上下文压缩，Go继续管理权限、版本、预算和作者采用。运行记录/导出格式与当前OpenAPI为接口依据。