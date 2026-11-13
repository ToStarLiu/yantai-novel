# 阶段 06：真实写作试用与首版交付 Implementation Plan

> 状态更新（2026-11-13）：本文件的原任务与未勾选路径用于设计溯源，不能作为当前缺失功能清单。阶段07已实现高级系统，阶段08已完成Pi作者助手、零章全书规划及页面供应商配置；最新工程状态见[总纲](README.md)，原文学质量/作者计时/Docker实跑未执行项继续保留。


> **For agentic workers:** 使用 `superpowers:executing-plans` 逐任务执行；经用户明确选择后才使用 `superpowers:subagent-driven-development`。用复选框追踪实际完成情况。

**Goal:** 以二十章评估和作者试用证明价值，完成可部署、可备份、可迁移的单作者自托管首版。

**Architecture:** 复用现有模块完成部署和运行证据收集；评估工具与产品流程共用 Provider、状态和检索入口，避免测试专用捷径。发布候选固定依赖、迁移和项目包格式版本。

**Tech Stack:** 既定 Go/React/PostgreSQL 栈、Docker Compose、浏览器验收、离线故障测试与显式真实模型实验。

**Spec:** [首版设计](../../specs/2026-11-12-novel-workbench-design.md)，全部首版范围；[评估协议](evaluation-protocol.md)。

## Global Constraints

- 首版为单作者 Web 自托管；Go 后端、React/TypeScript 前端、PostgreSQL 唯一业务数据库。
- 正文 Revision 不可变；任何 AI 修改生成候选版本，作者决定是否采用。
- 正文和作者确认的设定是权威来源；摘要、状态快照和检索索引必须保留来源并能重建。
- 长期记忆只从已确认正文或作者明确设定形成；AI 提议经过校验和作者确认后才能生效。
- 模型调用与任务记录绑定；浏览器断开不等于取消任务，明确取消才改变任务状态。
- 状态提交校验输入正文版本和基线状态版本；过期结果保留供查看但不得覆盖当前状态。
- 阶段验收使用固定样例和可检查证据；未执行的检查不得记录为通过。

## 前置与边界

进入本阶段前，阶段 00–05 必须通过各自验收。此阶段不扩展新题材平台、通用 Agent 或批量无人值守生产；试用暴露的核心可靠性和质量问题优先修复。

## Task 06-01：二十章受控评估与首版门槛

**Files:** 创建 `evaluation/fixtures/bridge-20/`、`evaluation/results/release-candidate/summary.md`；扩展 `cmd/eval` 的实验汇总；`docs/release/acceptance.md`。

**Interfaces:** 结果格式沿用评估协议；每个实验关联固定输入、Prompt、模型配置、项目 epoch、状态版本和检索索引版本。

- [ ] 为 20 章构造可人工复核的事实与知识标准答案，覆盖倒叙、物品转手、误信、伤势和伏笔。
- [ ] 比较 A/B/C 配置，对阶段 00 确定的受控集门槛逐项报告；未达到指标时保留逐项失败原因。
- [ ] 重复两次早期事实改写和一次章节重排，验证状态失效、恢复和引用变化。
- [ ] 同时报告正常单章成本和整条链路成本，包含审核、修订、抽取与失败尝试。
- [ ] 将失败分类为模型能力、上下文遗漏、状态错误、索引错误、界面操作和运行故障，针对来源修复再回归。

```powershell
go run ./cmd/eval --fixture evaluation/fixtures/bridge-20 --provider fake --output evaluation/results/release-fake
go run ./cmd/eval --fixture evaluation/fixtures/bridge-20 --provider configured --output evaluation/results/release-live
go run ./cmd/eval retrieval --fixture evaluation/fixtures/retrieval-60 --mode hybrid --split holdout
```

若阶段 04 未采用向量默认链路，最后一条按实际默认模式执行，并注明决定；不强行为发布启用效果更差的路线。

## Task 06-02：作者试用与操作负担

**Files:** 创建 `evaluation/pilot/protocol.md`、`results-template.md`；按实际问题修改对应业务模块与测试。

**Interfaces:** 试用记录每章的生成等待时间、主动修改时间、草稿采用情况、记忆审核时间、误报次数和放弃原因。实际作者原稿默认不进入公共结果文件。

- [ ] 邀请或由用户担任至少一名试用作者；先展示将发送给外部模型的上下文范围和费用配置，再开始真实写作。
- [ ] 完成至少三章实际写作和一次回改，观察作者能否无需开发者解释完成任务。
- [ ] 检查是否需要反复维护重复设定、过多确认、难找的证据或难懂的恢复状态；优先减少实际操作负担。
- [ ] 对照普通续写的同类任务，逐章记录时间与事实错误，明确小样本局限；不宣称统计显著提升。
- [ ] 形成保留、修正、延后功能清单；新增大功能进入下一版决策，不挤入发布阻塞项。

验收：有原始计时和具体问题记录，结论能回到实际章节；试用未完成则此任务保持未完成。

## Task 06-03：部署、备份与恢复

**Files:** 完成 `deploy/compose.yaml`、`Dockerfile`、`.env.example`、`deploy/README.md`、`backup-restore.md`、`upgrade.md`；创建 `web/tests/e2e/restore.spec.ts`；补齐 `internal/export/` 格式兼容校验。

**Interfaces:** 部署以固定镜像和迁移版本启动；完整数据库备份与单作品包恢复分别验收。项目包导入先验证大小、路径、格式和版本，不接受路径穿越或覆盖实例配置。

- [ ] 在空数据库和新目录按说明启动，配置作者凭据与模型引用，确认默认端口、持久卷和健康检查。
- [ ] 备份前记录作品数量、正文版本数、状态版本和源哈希；恢复到隔离空实例后逐项比较。
- [ ] 项目包迁移到另一空实例，重建检索索引，验证正文和来源；运行中的任务恢复为不可执行历史记录，不能因导入自动发模型请求。
- [ ] 测试升级迁移与兼容版本检查；不兼容升级在操作前要求备份，并记录可回退的边界。
- [ ] 检查远程部署登录、TLS 终止、会话、SSE 和导出流程，配置示例中无真实密钥。

验证：`docker compose -f deploy/compose.yaml config`、`npm --prefix web run test:e2e -- restore.spec.ts`；备份恢复演练记录命令、目标和对比结果，绝不覆盖现有用户实例。

## Task 06-04：发布候选复核与交付

**Files:** 创建 `docs/release/known-limitations.md`、`CHANGELOG.md`、`docs/licenses.md`；更新根 README 与计划总纲的实际状态。

- [ ] 运行后端完整测试、前端测试/类型检查/构建和关键浏览器流程；只在新改动或未决问题需要时重复测试。
- [ ] 清点实际采用的代码、包、Prompt、字体和其他资产；记录确切版本、许可证文件和分发要求，未采用的研究仓库不写入已用依赖。
- [ ] 明确已支持的 Provider 能力、中文检索限制、复杂时间线限制、取消与未知结果语义。
- [ ] 将验收矩阵每项链接到证据；数据丢失、跨作品泄露、过期覆盖和不可恢复迁移问题清零。
- [ ] 形成可供用户审阅的发布候选与启动说明；外部上传、公开发布、部署到用户服务器等动作按届时授权执行。

```powershell
go test ./... -count=1
npm --prefix web run test -- --run
npm --prefix web run typecheck
npm --prefix web run build
npm --prefix web run test:e2e
```

## 阶段退出标准

- [ ] 受控质量指标达到约定门槛，作者试用有可核对结论。
- [ ] 正常写作、回改、检索、任务恢复和导出迁移的完整路径通过。
- [ ] 空实例部署、完整备份恢复和单作品恢复均有实际记录。
- [ ] 运行依赖、费用、已知限制、第三方清单与升级方法已交付。
- [ ] 所有阶段实际完成后才能将总纲标记为首版完成；剩余事项按下一版单独规划。

## 2026-11-12 实施调整与验证登记

用户后续明确要求先完成整个系统、生成质量和作者试用后置，并授权直接优化架构及同步计划。本文件原任务列表保留为设计溯源，不将未执行的原路径命令或可选研究复选框伪记为完成。实际实现按业务聚合于 `internal/workbench/`，HTTP统一在 `internal/server/`，仍由同一PostgreSQL持久化、事务和版本边界约束；目录名变化不改变作者控制与来源规则。

工程实现和实际验收见 [系统验收记录](../../../../docs/release/acceptance.md)。代码验证使用真实隔离PostgreSQL、Chrome浏览器和离线Provider；真实服务连通验证单独记录。正文、记忆和检索一条完整闭环已经交付，文学质量及作者修改时间按用户要求保留为后续评估。

当前替代设计：索引更新与记忆应用在同一数据库事务提交，不加无必要Outbox；默认关键词检索通过60题评估。阶段07已实现可配置Embedding/Rerank/混合检索及有类型DAG/Production，真实语义模型的质量增益比较仍后置。阶段08由Pi承载对话循环和上下文压缩，Go继续管理权限、版本、预算和作者采用。运行记录/导出格式与当前OpenAPI为接口依据。