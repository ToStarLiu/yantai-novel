# 阶段 05：任务恢复与故障治理 Implementation Plan

> 状态更新（2026-11-13）：本文件的原任务与未勾选路径用于设计溯源，不能作为当前缺失功能清单。阶段07已实现高级系统，阶段08已完成Pi作者助手、零章全书规划及页面供应商配置；最新工程状态见[总纲](README.md)，原文学质量/作者计时/Docker实跑未执行项继续保留。


> **For agentic workers:** 使用 `superpowers:executing-plans` 逐任务执行；经用户明确选择后才使用 `superpowers:subagent-driven-development`。用复选框追踪实际完成情况。

**Goal:** 任务在断线、进程重启、租约竞争、取消和模型结果未知时保留正确业务状态，作者能理解并处理恢复选项。

**Architecture:** 加固已有 PostgreSQL 任务与尝试模型，使用租约代次阻止过期 Worker 提交。数据库副作用幂等、外部调用结果核对和作者重新生成分开处理，不追求无法保证的跨服务恰好一次。

**Tech Stack:** Go Worker、PostgreSQL SKIP LOCKED/事务、已有 Outbox、SSE、故障注入测试；首版不引入 Temporal。

**Spec:** [首版设计](../../specs/2026-11-12-novel-workbench-design.md)，第 8、10 节；[故障评估协议](evaluation-protocol.md)。

## Global Constraints

- 首版为单作者 Web 自托管；Go 后端、React/TypeScript 前端、PostgreSQL 唯一业务数据库。
- 正文 Revision 不可变；任何 AI 修改生成候选版本，作者决定是否采用。
- 正文和作者确认的设定是权威来源；摘要、状态快照和检索索引必须保留来源并能重建。
- 长期记忆只从已确认正文或作者明确设定形成；AI 提议经过校验和作者确认后才能生效。
- 模型调用与任务记录绑定；浏览器断开不等于取消任务，明确取消才改变任务状态。
- 状态提交校验输入正文版本和基线状态版本；过期结果保留供查看但不得覆盖当前状态。
- 阶段验收使用固定样例和可检查证据；未执行的检查不得记录为通过。

## 前置与边界

阶段 00/01 已建立基础持久任务，阶段 02/03 已实现原子业务提交与过期保护。本阶段处理其实际故障窗口，不重写通用工作流平台；同一作品的状态提交和重建步骤保持串行。

## Task 05-01：状态转换与租约代次

**Files:** 修改 `internal/job/worker.go`、`lease.go`；创建 `transition.go`、`transition_test.go`、`lease_test.go`；`migrations/000007_job_reliability.up.sql` 及回滚文件。

**Interfaces:** Job 增加 `leaseGeneration / leaseExpiresAt / owner / nextAttemptAt`。`Claim` 返回租约代次，续租与完成提交均需匹配代次；数据库时间作为租约判断依据。

- [ ] 先测试两 Worker 同时领取、租约到期重新领取、旧 Worker 迟到提交；只有当前租约可写运行结果和业务副作用。
- [ ] 写清状态转换表：终态不被普通重试复活；`WaitingForUser` 不占模型并发；只有明确操作能从 Paused 恢复。
- [ ] 任务完成与取消竞态采用事务锁定：完成先提交则取消返回已完成；取消先记录则完成仅保存部分/候选产物，任务为 Cancelled，不自动采用正文或应用记忆。
- [ ] Worker 停止时先停止领取任务，有限时间等待运行结束；未完成任务通过租约和 attempt 记录接管。

验证：`go test ./internal/job/... -run 'TestTransition|TestLease|TestCancelRace' -count=1`，并在支持环境执行 `go test -race ./internal/job/...`。

## Task 05-02：调用核对、重试与重新生成

**Files:** 创建 `internal/job/reconcile.go`、`retry.go`、对应测试；`internal/llm/capabilities.go`；扩展尝试记录和费用字段。

**Interfaces:** Provider 能力显式包含 `SupportsIdempotency / SupportsResultLookup / SupportsCancellation`，默认均为 false；能力为 true 需有对应契约测试。作者 `regenerate` 创建新 intentId，网络重复请求复用原 intentId。

- [ ] 在“调用前”“提供商接收后”“响应读取中”“响应已保存但任务未结束”“业务事务已提交”注入进程故障，逐个验证恢复结果。
- [ ] 对明确可重试的限流/临时错误使用有上限的指数退避与抖动，遵守 Retry-After；无效配置和余额问题等待作者修复。
- [ ] 可能已执行的超时/断线进入 NeedsReconciliation；有结果查询能力时核对，否则提供“保留未知结果”和“知悉可能增加费用后重新生成”的操作。
- [ ] 测试两个相同输入的作者主动重新生成会得到不同业务任务，重复提交同一个意图则不会。
- [ ] 费用分别标记 `Reported / Estimated / Unknown`，重试和已取消调用的已知费用计入作品总额。

验证：`go test ./internal/job/... ./internal/llm/... -run 'TestRetry|TestReconcile|TestIntent|TestCost' -count=1`。

## Task 05-03：持久步骤、暂停恢复与 Outbox

**Files:** 创建 `internal/job/step.go`、`outbox.go`、`step_test.go`、`outbox_test.go`；修改 `internal/impact/rebuild.go` 和 `internal/narrative/apply.go`。

**Interfaces:** 每个步骤有稳定 `stepId / inputArtifactIds / outputArtifactIds / status`；人工审核批准事件携带目标 ChangeSet 和期望基线，不仅携带“继续”布尔值。

- [ ] 从每个已提交步骤后重启 Worker，验证只执行未完成步骤；完成的记忆提交和索引任务不得重复产生事实。
- [ ] 暂停默认在步骤边界生效；正在执行的模型调用可完成并保存产物，界面显示“暂停将在当前步骤结束后生效”。
- [ ] 恢复时重新校验输入 epoch 和正文版本；已过期的流程进入需重规划状态，不能继续提交旧结果。
- [ ] Outbox 消费幂等，消费者崩溃后重新投递不会复制索引或状态；失败事件保留可查看原因。
- [ ] 人工审核等待不持有数据库锁或占用 Worker 执行槽。

验证：`go test ./internal/job/... ./internal/impact/... ./internal/narrative/... -count=1`。

## Task 05-04：并发、预算与恢复界面

**Files:** 创建 `internal/job/limits.go`、`budget.go`、测试；`web/src/features/runs/RecoveryActions.tsx`、`RunHistory.tsx`；`web/tests/e2e/job-recovery.spec.ts`；`deploy/recovery-drill.md`。

**Interfaces:** 作品和 Provider 并发上限、单次 Token/估算费用上限从配置读取。多 Worker 的全局并发额度由数据库协调，不能只用进程内信号量声称是全局限流。

- [ ] 测试超预算在发出外部请求前被拒绝；实际费用超出估算时停止后续步骤并显示差额，不篡改账目。
- [ ] UI 区分重试同一失败步骤、从检查点恢复、主动重新生成和核对未知结果；每个按钮说明费用与输入是否改变。
- [ ] 用假 Provider 做进程终止/重启演练，不以真实收费请求进行破坏性测试。
- [ ] 检查每条任务日志可用 runId/attemptId 关联，常规日志不包含原稿全文、密钥与完整 Prompt。
- [ ] 记录何种故障仍需人工处理；若确需 Temporal，另写迁移决策，不能在此阶段临时加入第二套编排。

```powershell
go test ./internal/job/... ./internal/impact/... ./internal/narrative/... -count=1
npm --prefix web run test:e2e -- job-recovery.spec.ts
docker compose -f deploy/compose.yaml restart worker
```

最后一条只对专用测试实例执行，测试先记录正在执行的假任务 ID。

## 阶段退出标准

- [ ] 故障注入每个窗口均有预期结果和实际证据。
- [ ] 失去租约的 Worker 无法提交业务结果；重复投递不产生重复状态。
- [ ] 断线不取消任务，暂停语义明确，输入变化后不能盲目恢复。
- [ ] 未知调用结果可见且可核对，不承诺提供商不具备的计费幂等能力。
- [ ] 多 Worker 限流与作品预算测试通过，恢复操作能由作者理解。

## 2026-11-12 实施调整与验证登记

用户后续明确要求先完成整个系统、生成质量和作者试用后置，并授权直接优化架构及同步计划。本文件原任务列表保留为设计溯源，不将未执行的原路径命令或可选研究复选框伪记为完成。实际实现按业务聚合于 `internal/workbench/`，HTTP统一在 `internal/server/`，仍由同一PostgreSQL持久化、事务和版本边界约束；目录名变化不改变作者控制与来源规则。

工程实现和实际验收见 [系统验收记录](../../../../docs/release/acceptance.md)。代码验证使用真实隔离PostgreSQL、Chrome浏览器和离线Provider；真实服务连通验证单独记录。正文、记忆和检索一条完整闭环已经交付，文学质量及作者修改时间按用户要求保留为后续评估。

当前替代设计：索引更新与记忆应用在同一数据库事务提交，不加无必要Outbox；默认关键词检索通过60题评估。阶段07已实现可配置Embedding/Rerank/混合检索及有类型DAG/Production，真实语义模型的质量增益比较仍后置。阶段08由Pi承载对话循环和上下文压缩，Go继续管理权限、版本、预算和作者采用。运行记录/导出格式与当前OpenAPI为接口依据。