# AI 辅助小说创作程序：开源项目调研、产品设计与技术实施方案

> 文档版本：v2.0（Go 架构版）  
> 调研日期：2026-11-12  
> 适用对象：准备开发 AI 辅助长篇小说创作产品的个人开发者或小型团队  
> 建议技术方向：Go + React/TypeScript + PostgreSQL/pgvector + Temporal + Python/vLLM + OpenAI-compatible API  
> 文档目标：汇总值得参考的开源项目，提炼可复用的产品、架构、长篇记忆、Agent、RAG、工作流和工程实践，并给出可直接执行的 MVP 与后续演进方案。

---

## 目录

1. [执行摘要](#1-执行摘要)
2. [产品定位与核心问题](#2-产品定位与核心问题)
3. [开源项目总览](#3-开源项目总览)
4. [第一梯队：完整产品型项目](#4-第一梯队完整产品型项目)
5. [第二梯队：长篇记忆与一致性引擎](#5-第二梯队长篇记忆与一致性引擎)
6. [第三梯队：Agent、Skill 与自动成书工作流](#6-第三梯队agentskill-与自动成书工作流)
7. [第四梯队：世界观、角色记忆与知识图谱组件](#7-第四梯队世界观角色记忆与知识图谱组件)
8. [第五梯队：研究型项目与算法](#8-第五梯队研究型项目与算法)
9. [可补充参考的项目](#9-可补充参考的项目)
10. [项目横向比较与选型](#10-项目横向比较与选型)
11. [建议重点克隆和阅读的仓库](#11-建议重点克隆和阅读的仓库)
12. [建议产品定位](#12-建议产品定位)
13. [完整产品功能架构](#13-完整产品功能架构)
14. [核心领域模型](#14-核心领域模型)
15. [长篇小说上下文与记忆架构](#15-长篇小说上下文与记忆架构)
16. [RAG 与检索设计](#16-rag-与检索设计)
17. [Agent 与 Workflow 设计](#17-agent-与-workflow-设计)
18. [章节生产流水线](#18-章节生产流水线)
19. [一致性检查体系](#19-一致性检查体系)
20. [Prompt 工程方案](#20-prompt-工程方案)
21. [模型路由与成本控制](#21-模型路由与成本控制)
22. [Go 优先的技术架构建议](#22-go-优先的技术架构建议)
23. [数据库与存储设计](#23-数据库与存储设计)
24. [后台任务、并发和可靠性](#24-后台任务并发和可靠性)
25. [编辑器与前端交互](#25-编辑器与前端交互)
26. [安全、隐私与版权](#26-安全隐私与版权)
27. [测试与质量评估](#27-测试与质量评估)
28. [MVP 范围与开发路线图](#28-mvp-范围与开发路线图)
29. [商业化前的许可证检查](#29-商业化前的许可证检查)
30. [最终建议](#30-最终建议)

---

# 1. 执行摘要

如果目标是开发一款真正适用于长篇小说的 AI 写作程序，最重要的不是“接入一个大模型并提供聊天框”，而是建立一套可持续、可追踪、可恢复、可人工控制的小说生产系统。

长篇小说产品必须处理以下核心问题：

1. **长篇上下文无法全部放入一次模型调用。**
2. **角色、地点、物品和关系会随章节发生变化。**
3. **角色只能知道自己有理由知道的信息。**
4. **伏笔、承诺、悬念和未完成情节需要跨几十章追踪。**
5. **AI 生成必须允许作者审核、拒绝、局部修改和回滚。**
6. **长时间批量生成任务必须支持暂停、恢复、重试和幂等。**
7. **模型成本、上下文长度和质量之间需要动态平衡。**
8. **小说写作包含策划、写作、审稿、修订、记忆更新等不同职责，不能全部交给一个万能 Prompt。**

综合调研结果，建议不要直接 Fork 某个大型项目作为最终产品，而是分别吸收以下项目的长处：

- **Pagekin**：用户体验、章节状态、审稿与 Diff。
- **OpenFic**：Agent 工作台、RAG、上下文管理、跨平台架构。
- **Biz Novel Studio**：长时间整本生产任务、暂停恢复、状态回灌。
- **NstAgent**：Narrative State Tracking，也就是叙事状态追踪。
- **NovelClaw**：动态记忆、可检查运行记录和章节后写回。
- **LibriScribe**：清晰的多 Agent 职责拆分。
- **awesome-novel-agent**：中文网文工作流、文件化知识库、卷纲与章纲 SOP。
- **NovelGenerator**：因果规划、Scene Handoff 和连续性检查。
- **CharacterMemory / Kandor**：角色主观记忆与时间知识图谱。

建议第一版产品定位为：

> **本地优先、作者主导、面向中文长篇小说的 AI 创作工作台。AI 负责规划、起草、检查和提出修订，作者保留最终决定权。**

建议的第一版技术栈：

```text
Core Backend
├─ Go 1.24+
├─ net/http + chi
├─ pgx + sqlc
├─ PostgreSQL + pgvector
├─ SSE（模型流式输出）
├─ Temporal Go SDK（复杂长工作流阶段引入）
├─ OpenTelemetry
└─ OpenAI-compatible API Client

AI / Inference
├─ 云模型：OpenAI-compatible API
├─ 本地模型：Python + vLLM
├─ Embedding Service
└─ Cross-Encoder Reranker

Frontend
├─ React + TypeScript
├─ Tiptap
├─ TanStack Query
├─ Zustand
├─ React Flow
└─ Tauri（第二阶段桌面化）
```

选择 Go 而不是全 Rust 的原因，不是忽略性能，而是本系统的大部分耗时来自 LLM、数据库、向量检索和网络。Go 已足以提供高吞吐的 I/O 并发，同时能显著降低开发、编译、招聘和长期维护成本。Rust 保留给未来真正需要极低延迟或资源控制的基础设施组件，例如自研模型网关、Tokenizer、检索内核或本地桌面核心。

第一版只做七件事：

1. 创建小说项目。
2. 管理人物、世界观和文风。
3. 生成并编辑故事大纲、卷纲和章纲。
4. 根据章纲生成单章草稿。
5. 维护结构化 Narrative State。
6. 执行一致性检查和文风检查。
7. 作者确认定稿后，更新记忆和时间线。

---

# 2. 产品定位与核心问题

## 2.1 不要做成普通聊天套壳

普通 AI 写作产品常见流程如下：

```text
用户输入提示词
→ 模型生成一段文字
→ 用户继续追问
→ 对话越来越长
→ 上下文被截断
→ 人物和剧情开始失忆
```

这种方案适用于短文、广告文案和简单续写，但不适合几十万字的长篇小说。

真正的小说创作工具应把以下对象显式建模：

```text
作品
卷
章
场景
角色
角色状态
世界规则
地点
物品
势力
时间线
事件
情节线
伏笔
悬念
作者承诺
章节蓝图
叙事状态
文风配置
Prompt 版本
Agent 定义
工作流
生成运行记录
审稿问题
修订版本
```

## 2.2 AI 的正确定位

AI 不应该默认替代作者，而应扮演几个边界明确的角色：

- 策划助手：把灵感变成故事前提、大纲、卷纲、章纲。
- 连贯性助手：检查事实、时间、人物知识和世界规则。
- 写作助手：根据明确蓝图生成可编辑草稿。
- 发展编辑：检查人物弧、节奏、冲突和场景必要性。
- 文风编辑：只改语言，不自行更改剧情事实。
- 记忆管理员：章节定稿后更新 Story Bible 和 Narrative State。

## 2.3 推荐的产品原则

1. **作者主导**：任何正文修改必须可查看、可拒绝、可回滚。
2. **本地优先**：原稿默认保存在用户本地或自托管数据库。
3. **状态优先**：重要事实进入结构化状态，不只依赖向量检索。
4. **运行可追踪**：每次模型调用记录输入快照、Prompt 版本、模型、输出和成本。
5. **流程可恢复**：批量创作中断后可以从最后成功步骤继续。
6. **模型可替换**：应用只依赖 OpenAI-compatible 或统一 Provider 抽象。
7. **Prompt 可版本化**：Prompt 变更不能悄悄影响旧项目的可复现性。
8. **人工门禁**：大纲、章纲、草稿和定稿之间应有明确确认点。

---

# 3. 开源项目总览

下面按照“完整产品、长篇引擎、Agent/Skill、记忆组件、研究方案”分类。

| 项目 | 类型 | 最值得参考的能力 | 主要技术形态 | 许可证注意 |
|---|---|---|---|---|
| Pagekin | 完整产品 | 本地桌面、创作流程、审稿、修订 Diff | Electron / TypeScript | GPL-3.0 |
| OpenFic | 完整产品 | Agent、RAG、工作流、跨端 | Python + Web/Desktop/Mobile | Apache-2.0 |
| Biz Novel Studio | 完整产品/生产引擎 | 整本生产链、自动导演、暂停恢复 | TypeScript 全栈 | 使用前核实当前 LICENSE |
| OpenWrite | 完整产品 | Editor、Canvas、Codex、协作 | Bun / TypeScript / Web | AGPL-3.0 |
| Novel Engine | 完整产品 | 虚拟编辑部、Voice Profile、出版流程 | 桌面应用 | AGPL-3.0 |
| NovelClaw | 长篇引擎 | 动态记忆、可检查运行、状态快照 | Python/Web/Docker | MIT（以仓库为准） |
| NstAgent | 研究/引擎 | Narrative State Tracking | Python | MIT |
| NovelGenerator | 长篇引擎 | 因果规划、Scene Handoff、连续性检查 | TypeScript/React | BSL 1.1，需谨慎 |
| LibriScribe | Agent 框架 | 清晰的多 Agent 分工、检索、成本记录 | Python | MIT |
| awesome-novel-agent | Skill/Agent 包 | 中文网文 SOP、记忆和任务调度 | Markdown/Python/Skill | AGPL + 商业条件 |
| oh-story-claudecode | Skill 包 | 扫榜、拆书、网文策划、去 AI 味 | Agent Skills | 核实仓库许可证 |
| chinese-novelist-skill | Skill 包 | 10–50 章中文小说、悬念钩子、自动校验 | JavaScript/Skill | 核实仓库许可证 |
| AIStoryWriter | 生成引擎 | 本地模型、批量长文生成、自动评价 | Python/Ollama | AGPL-3.0 |
| CharacterMemory | 记忆组件 | 角色主观记忆、遗忘、情绪权重 | Python/FastAPI/MCP | GPL-3.0 |
| Kandor | 世界模拟组件 | 时间知识图谱、世界模拟 | Python | 核实仓库许可证 |
| Re3/DOC/E2EPlot | 研究方案 | 多候选、重排、事实修订 | 论文/研究代码 | 查看各仓库 |
| Agents’ Room | 研究方案 | 专职 Agent 多步协作 | 论文/研究 | 论文与代码分别检查 |
| StoryBox | 研究方案 | 多角色 Sandbox 涌现剧情 | 研究代码 | 研究用途优先 |

> 注意：Star、Fork、版本、活跃度和许可证都可能变化。正式采用代码前，应以仓库当前 `LICENSE`、最新 Release、Commit、Issues 和依赖清单为准。

---

# 4. 第一梯队：完整产品型项目

## 4.1 Pagekin

- GitHub：<https://github.com/EthanYoQ/Pagekin>
- 定位：本地优先的免费 AI 小说写作软件。
- 产品形态：Windows/macOS 桌面应用。
- 模型方式：用户配置 API Key，或使用 Ollama 本地模型。
- 主要特色：设定、架构、章节蓝图、正文、审稿、修稿、定稿全过程。

### 4.1.1 产品流程

Pagekin 的流程可概括为：

```text
创建作品
→ 填写类型、受众、语言、总章数、单章长度
→ 生成或编辑故事前提
→ 建立角色名单
→ 建立世界观
→ 创建情节大纲
→ 分批生成章节蓝图
→ 生成单章草稿
→ AI 审稿
→ 选择审稿意见
→ 生成修稿
→ 逐段比较差异
→ 作者确认
→ 定稿
```

### 4.1.2 值得复用的产品思想

1. **明确的章节生命周期**

```text
Planning
Drafting
Drafted
Reviewing
Reviewed
Revising
Finalized
```

2. **AI 不直接覆盖正文**

建议所有 AI 修改都产生新 Revision。作者可以：

- 接受全部修改。
- 接受某一段。
- 拒绝某一段。
- 在 AI 版本基础上继续手改。
- 恢复到任意历史版本。

3. **参考小说与用户原稿隔离**

参考小说只能用于：

- 结构分析。
- 节奏分析。
- 文风特征抽取。
- 章节功能识别。

不应直接混入用户正文检索库，否则容易出现近似复述和版权风险。

4. **批量任务可控制**

批量写章必须支持：

- 暂停。
- 取消。
- 查看进度。
- 失败重试。
- 从已有结果继续。
- 设置最多生成章数。

### 4.1.3 不建议照搬的部分

- GPL-3.0 不适合作为闭源商业产品的直接代码基础。
- Electron 带来的应用体积和内存占用较大。
- 桌面单机模式与多租户 SaaS 的身份、权限、隔离设计不同。
- 固定步骤适合新手，但专业作者可能需要自定义工作流。

### 4.1.4 阅读重点

```text
src/
electron/
plugins/
docs/
test/
package.json
README_zh.md
```

重点观察：项目文件格式、章节状态、任务运行侧栏、导入导出、知识库隔离、Diff UI。

---

## 4.2 OpenFic

- GitHub：<https://github.com/syrizelink/OpenFic>
- 定位：跨平台、AI Native、Agent 驱动的小说创作工作台。
- 部署：Docker、pip、桌面应用。
- 数据理念：本地持久化、自托管。
- 许可证：Apache-2.0，对商业借鉴相对友好。

### 4.2.1 核心功能

- 小说专用编辑器。
- 自定义 Agent。
- 自定义 Prompt。
- 自定义工作流。
- OpenAI-compatible 模型。
- Embedding 与 Rerank。
- 长项目语义检索。
- 多层上下文管理。
- 自动压缩和动态截断。
- 模型调用缓存。
- Web、桌面和移动端适配。

### 4.2.2 最值得研究的架构思想

1. **Agent 是产品中的一等对象**

建议 Agent 数据结构至少包括：

```text
AgentDefinition
├─ Name
├─ Role
├─ SystemPromptTemplateId
├─ AllowedTools
├─ InputSchema
├─ OutputSchema
├─ ContextPolicy
├─ ModelPolicy
├─ MaxIterations
├─ RetryPolicy
└─ PermissionPolicy
```

2. **上下文不是简单拼接**

需要一个 Context Builder，根据当前任务选择：

- 项目固定设定。
- 本卷概要。
- 当前章纲。
- 相关人物卡。
- Narrative State。
- 前文检索片段。
- 上一场景交接。
- 作者本次指导。

3. **成本优先的上下文层级**

可以按优先级分层：

```text
P0：硬规则、当前任务、当前章纲
P1：必须出现的人物状态、上一章结尾
P2：召回的相关事件、伏笔、地点信息
P3：文风示例、远期背景
```

当 Token 不足时从 P3 向上裁剪，但 P0 永不裁剪。

### 4.2.3 阅读重点

```text
backend/
frontend/
desktop/
mobile/
docs/develop/
docker-compose.yml
```

重点研究：Provider 抽象、Agent 工具协议、向量检索、上下文压缩、数据库后端和多端通信。

---

## 4.3 Biz Novel Studio / AI-Novel-Writing-Assistant

- GitHub：<https://github.com/ExplosiveCoderflome/AI-Novel-Writing-Assistant>
- 定位：长篇小说 AI Native 生产系统。
- 核心关键词：Agent、世界观、RAG、写法引擎、整本生产工作流。

### 4.3.1 核心流程

```text
一句灵感
→ 创作方向
→ 世界设定
→ 角色设计
→ 卷战略
→ 章节任务
→ 正文生成
→ 审核
→ 自动修复
→ 状态回灌
→ 下一章
```

### 4.3.2 最大价值：长链路任务系统

整本生成可能运行很久，因此应把每个步骤保存为持久化任务，而不是一个 HTTP 请求：

```text
BookProductionRun
├─ DirectionGenerationStep
├─ WorldBuildingStep
├─ CharacterPlanningStep
├─ VolumePlanningStep
├─ ChapterPlanningStep[N]
├─ ChapterDraftStep[N]
├─ ChapterReviewStep[N]
├─ ChapterRevisionStep[N]
├─ MemoryUpdateStep[N]
└─ FinalAuditStep
```

每一步都必须具有：

- 输入快照。
- 输出制品。
- 状态。
- 开始和结束时间。
- 重试次数。
- Prompt 版本。
- 模型和参数。
- Token 和成本。
- 错误信息。
- 幂等键。

### 4.3.3 与高并发和分布式系统学习的结合点

这个项目非常适合作为高并发和分布式任务系统的实战主题。使用 Go 可重点练习：

- Worker Pool。
- Goroutine 与 Channel。
- `context.Context` 取消和超时传播。
- Retry/Backoff。
- 幂等。
- Outbox Pattern。
- 分布式锁。
- 限流。
- 任务分区。
- SignalR 实时进度。
- OpenTelemetry Trace。

---

## 4.4 OpenWrite

- GitHub：<https://github.com/ilrein/openwrite>
- 项目主页：<https://ilrein.github.io/openwrite/>
- 定位：面向小说家、编剧和创意作者的开源 AI 写作平台。

### 4.4.1 可参考功能

- 富文本编辑器。
- Story Canvas。
- 角色弧。
- 时间线。
- 情节结构模板。
- 人物关系图。
- Story Codex。
- 实时协作。
- 评论和建议。
- 版本控制。
- 写作进度和分析。
- PDF、DOCX、EPUB 导出。

### 4.4.2 最值得借鉴的界面结构

建议产品主界面不要是单一聊天页，而应是：

```text
左侧：作品树
├─ 设定
├─ 角色
├─ 地点
├─ 时间线
├─ 卷
└─ 章节

中央：主编辑器/画布

右侧：AI 助手
├─ 当前上下文
├─ 写作操作
├─ 审稿问题
├─ 记忆引用
└─ 运行状态
```

### 4.4.3 注意事项

项目主页列出的功能不一定全部达到同等成熟度。分析时应同时检查代码、Release、Issues 和测试，而不是只看宣传页面。

---

## 4.5 Novel Engine

- 项目主页：<https://john-paul-ruf.github.io/novel-engine/>
- 定位：本地桌面出版工作室。
- 特色：七个不同职责的 AI 编辑角色和十五阶段创作流程。

### 4.5.1 七个角色

1. Story Coach：挖掘想法、主题、人物和卖点。
2. Ghostwriter：学习作者声音并起草正文。
3. First Reader：以普通读者视角反馈阅读感受。
4. Developmental Editor：检查人物弧、节奏、结构和叙事逻辑。
5. Task Master：把反馈转成具体修订任务。
6. Copy Editor：处理语法、一致性和机械性问题。
7. Publisher：出版元数据、简介、关键词、投稿材料。

### 4.5.2 关键启发

不要让所有 Agent 都能修改正文。建议权限如下：

| Agent | 可读范围 | 可写范围 |
|---|---|---|
| Planner | 故事前提、设定、概要 | 大纲、卷纲、章纲 |
| Writer | 本章上下文 | 新草稿 |
| Continuity Reviewer | 全局状态和当前正文 | ReviewIssue，不直接改正文 |
| Style Reviewer | 文风配置和当前正文 | StyleIssue，不改剧情事实 |
| Revision Agent | 原稿和已批准问题 | 新 Revision |
| Memory Curator | 定稿章节和旧状态 | 新 NarrativeState 草案 |
| Publisher | 定稿全文和元数据 | 导出与宣传材料 |

---

# 5. 第二梯队：长篇记忆与一致性引擎

## 5.1 NovelClaw

- GitHub：<https://github.com/iLearn-Lab/NovelClaw>
- 定位：动态记忆优先的长篇故事创作框架。

### 5.1.1 核心对象

- Writing Session。
- Storyboard。
- Manuscript。
- Character View。
- World View。
- Editable Memory Bank。
- Run Trace。
- State Snapshot。

### 5.1.2 可借鉴点

1. 每次运行都留下可检查记录。
2. 章节完成后写回记忆。
3. 用户可以手动修正记忆。
4. 记忆既有结构化内容，也可配合检索。
5. 中断后可以从状态快照恢复。

### 5.1.3 建议实现的 Run Trace

```json
{
  "runId": "...",
  "projectId": "...",
  "taskType": "ChapterDraft",
  "chapterId": "...",
  "contextItems": [
    { "type": "CharacterState", "id": "...", "reason": "appears-in-chapter" },
    { "type": "TimelineEvent", "id": "...", "reason": "semantic-retrieval" }
  ],
  "promptVersion": "chapter-writer-v7",
  "model": "provider/model",
  "parameters": { "temperature": 0.8 },
  "outputArtifactId": "...",
  "tokenUsage": {},
  "durationMs": 0,
  "status": "Succeeded"
}
```

---

## 5.2 NstAgent

- GitHub：<https://github.com/zhennan1/NstAgent>
- 研究方向：Narrative State Tracking，面向完整长篇小说的训练无关 Agent 框架。

### 5.2.1 核心算法

```text
冻结的全书大纲
+ 当前叙事状态 State_t
+ 当前章节任务
+ 必要的前文搜索
→ 生成第 t+1 章
→ 从新章节抽取状态变化
→ 生成 State_t+1
```

### 5.2.2 为什么优于把全文塞进上下文

- Token 成本更低。
- 状态可检查。
- 矛盾容易定位。
- 不依赖超长上下文模型。
- 一章失败不会破坏整本任务。
- 可以单独修正某个角色状态。
- 可以对每章进行可重复生成。

### 5.2.3 推荐 Narrative State Schema

```json
{
  "version": 18,
  "asOfChapter": 18,
  "storyClock": {
    "calendar": "第三纪元132年冬月初六",
    "relativeDay": 27,
    "timeOfDay": "夜"
  },
  "characters": {
    "character-id": {
      "name": "林川",
      "locationId": "north-stone-bridge",
      "physicalState": ["左臂受伤", "失血但可行动"],
      "emotionalState": ["警惕", "愧疚"],
      "currentGoals": ["找到失踪信使"],
      "knowledge": [
        {
          "factId": "fact-001",
          "belief": "王都存在叛徒",
          "confidence": 0.7,
          "source": "沈月的暗示",
          "acquiredChapter": 17,
          "truthStatus": "Unverified"
        }
      ],
      "inventory": ["断裂的铜印"],
      "relationships": {
        "shen-yue": {
          "type": "UneasyAlliance",
          "trust": 0.42,
          "sinceChapter": 12
        }
      }
    }
  },
  "activePlotThreads": [],
  "resolvedPlotThreads": [],
  "foreshadowing": [],
  "unfulfilledPromises": [],
  "worldChanges": [],
  "continuityConstraints": [],
  "nextChapterRequirements": []
}
```

### 5.2.4 冻结大纲并不表示不能修改

“冻结”应理解为版本化，不是永久不可改。合理流程是：

```text
OutlineVersion 1 已批准
→ 写到第 12 章
→ 作者要求改变第 3 卷方向
→ 创建 OutlineVersion 2
→ 记录从第 13 章开始生效
→ 对未写章节重新规划
→ 已定稿章节保留历史事实
```

---

## 5.3 NovelGenerator

- GitHub：<https://github.com/KazKozDev/NovelGenerator>
- 定位：本地 AI 小说生成器，支持 Gemini 和 Ollama。
- 重点：因果规划、持久故事记忆、场景交接、连续性检查。

### 5.3.1 Scene Handoff

每个场景结束后生成交接包：

```json
{
  "sceneId": "18-3",
  "endLocation": "北境石桥下层",
  "presentCharacters": ["林川", "沈月"],
  "departedCharacters": ["老周"],
  "newFacts": ["桥下有王都军械箱"],
  "inventoryChanges": ["林川获得断裂铜印"],
  "emotionalChanges": ["林川对沈月的信任略微上升"],
  "openActions": ["二人正准备打开军械箱"],
  "requiredContinuity": ["下一场景必须从打开军械箱前后开始"],
  "forbiddenChanges": ["林川左臂伤势不能无故消失"]
}
```

### 5.3.2 因果链

可以为每个事件保存：

```text
CauseEventIds
Trigger
Action
ImmediateResult
LongTermConsequences
AffectedEntities
ObservedByCharacters
```

这样一致性检查不只是询问“有没有矛盾”，还可以检查新事件是否有足够前因。

### 5.3.3 许可证提醒

该项目搜索结果显示采用 Business Source License 1.1。研究设计没有问题，但复制代码、部署服务或商业使用前必须阅读当前许可证的转换日期、额外使用限制和生产使用条款。

---

## 5.4 Long-Story Agent

- 项目页：<https://xiao-zi-chen.github.io/CoLong-Idea-Studio/>
- 方向：多 Agent、RAG、动态记忆、自动评价和迭代修订。

### 5.4.1 Agent 划分

```text
Analyzer
Organizer
Writer
Plot Agent
Character Agent
World Agent
Retriever
Memory
Evaluator
Consistency Checker
Realtime Editor
```

### 5.4.2 闭环流程

```text
规划
→ 检索上下文
→ 多 Agent 起草
→ 质量评分
→ 一致性检查
→ 定向修订
→ 状态写回
→ 下一轮
```

### 5.4.3 产品化时的改进

- 不要让 Evaluator 只返回一个总分。
- 必须返回具体问题、证据位置、严重程度和建议动作。
- Revision Agent 只处理作者批准或自动规则允许的问题。
- 修订后重新运行受影响的检查项。

---

## 5.5 AIStoryWriter

- GitHub：<https://github.com/datacrystals/AIStoryWriter>
- 定位：根据提示生成中长篇或完整小说，强调本地模型和长输出。

适合参考：

- 不同阶段使用不同模型。
- 本地 Ollama 接入。
- CLI 批量运行。
- Prompt 配置。
- 自动评价。
- 多语言转换。

不足：更像生成 Pipeline，而不是成熟写作工作台。

---

# 6. 第三梯队：Agent、Skill 与自动成书工作流

## 6.1 LibriScribe

- GitHub：<https://github.com/guerra2fernando/libriscribe>
- 定位：多 Agent 图书创作系统。
- 许可证：MIT。

### 6.1.1 Agent 划分

- ConceptGeneratorAgent。
- OutlinerAgent。
- CharacterGeneratorAgent。
- WorldbuildingAgent。
- ChapterWriterAgent。
- EditorAgent。
- ContentReviewerAgent。
- StyleEditorAgent。
- FormattingAgent。

### 6.1.2 工程能力

- 多模型 Provider。
- OpenRouter。
- YAML Prompt。
- 成本跟踪。
- JSON 自修复。
- MCP 插件。
- 关键词、语义和混合检索。
- Recall@k、Precision@k、MRR 检索指标。

### 6.1.3 最适合学习的内容

代码规模比完整桌面产品更容易理解，建议重点阅读：

```text
src/libriscribe/
prompts/
plugins/
examples/
tests/
```

重点关注：Agent 基类、统一 LLM Client、结构化输出解析、失败恢复和 Prompt 外置化。

---

## 6.2 awesome-novel-agent

- GitHub：<https://github.com/modoojunko/awesome-novel-agent>
- 定位：支持多种 Coding Agent 的中文小说创作 Agent/Skill 套件。

### 6.2.1 角色结构

- novel-agent：总指挥。
- volume-planner：卷纲。
- chapter-planner：章纲。
- prompt-crafter：组装写作提示。
- writer：正文写作。
- reader：读者反馈。
- updater：设定、时间线、角色状态和记忆更新。

### 6.2.2 文件系统即知识库

```text
story.md
settings/
├─ world-setting.md
├─ genre-setting.md
└─ writing-style.md
volumes/
chapters/
prompts/
archives/
.agent/status.md
memory/
```

这种方案适合 MVP：

- 透明。
- 易于 Git 版本控制。
- 用户可直接编辑。
- Agent 容易读取。
- 迁移成本低。

### 6.2.3 可借鉴的调度原则

- 总指挥不直接写正文。
- 子 Agent 完成后必须验证产物。
- 章节定稿后才允许 Updater 修改长期记忆。
- 当前阶段由状态文件和实际文件共同判断。
- 文件存在不等于阶段成功，还需验证内容和状态。

---

## 6.3 oh-story-claudecode

- GitHub：<https://github.com/zenstory-ai/oh-story-claudecode>
- 方向：中文网文全流程 Skill 包。

适合研究：

- 扫榜和选材。
- 爆款结构拆解。
- 长篇设定。
- 大纲和细纲。
- 伏笔追踪。
- 章节钩子。
- 去 AI 味。
- 封面生成。
- Coding Agent Skill 组织方式。

需要注意：扫榜、仿写和参考作品分析应建立版权与合规边界，不能让系统大段复制受版权保护作品。

---

## 6.4 chinese-novelist-skill

- GitHub：<https://github.com/PenglongHuang/chinese-novelist-skill>
- 方向：中文长篇和网文连载 Skill。

可借鉴能力：

- 分层问答。
- 先确认题材、受众和核心冲突。
- 创作记忆。
- 悬念钩子。
- 章节自动校验。
- 10–50 章生成流程。

关键启发：不要在需求不足时立刻生成整本内容。应通过三层问题逐步约束：

```text
第一层：题材、受众、篇幅、平台、视角
第二层：核心冲突、主角欲望、失败代价、结局方向
第三层：世界规则、角色关系、禁区、文风和节奏
```

---

## 6.5 7a-z/NovelClaw Skill

- GitHub：<https://github.com/7a-z/NovelClaw>
- 特点：场景化文风路由、AI 套话检测和自检。

### 6.5.1 场景策略路由

```text
Combat
→ 短句
→ 动作优先
→ 提高动词密度
→ 增加声音、冲击和空间变化

Emotion
→ 微表情
→ 潜台词
→ 环境映照
→ 减少直接解释情绪

Suspense
→ 限制视角
→ 不完整信息
→ 延迟解释
→ 声音、光线、异常细节

General
→ 信息密度适中
→ 动作和描述平衡
→ 避免模板化总结
```

建议将其抽象为 `WritingProfile`，允许作者按题材和场景覆盖。

---

# 7. 第四梯队：世界观、角色记忆与知识图谱组件

## 7.1 CharacterMemory

- GitHub：<https://github.com/FrancescoCaracciolo/CharacterMemory>
- 定位：模拟角色的记忆、遗忘和关系。

### 7.1.1 重要思想：角色并非共享全知数据库

每个角色应该拥有自己的 Knowledge Boundary：

```text
ObjectiveTruth：客观真相
ObservedFact：角色亲眼见到
HeardInformation：别人告诉角色
Inference：角色推测
FalseBelief：错误信念
Secret：角色掌握但未公开
ForgottenMemory：已遗忘或变得模糊
```

### 7.1.2 回忆权重

可按以下因素计算：

```text
RecallScore =
  RecencyWeight
  × EmotionalImpact
  × LocationRelevance
  × RelationshipRelevance
  × RetrievalFrequency
  × NarrativeImportance
```

不建议只用相似度排序，否则极其重要但措辞不同的旧事可能无法召回。

### 7.1.3 小说场景中的用途

- 防止角色知道自己不该知道的秘密。
- 支持误会、欺骗和错误推理。
- 生成符合角色经历的对话。
- 模拟多年后记忆模糊。
- 处理不同角色对同一事件的不同理解。

---

## 7.2 Kandor

- 项目页：<https://harryluumn.github.io/kandor/>
- 定位：具有时间知识图谱记忆的虚构世界模拟库。

### 7.2.1 时间关系

普通关系表：

```text
A --盟友--> B
```

时间关系应为：

```text
A --盟友--> B，Chapter 1–18
A --怀疑--> B，Chapter 12–18
A --敌对--> B，Chapter 19–当前
```

### 7.2.2 推荐的数据模型

```text
TemporalRelation
├─ SubjectEntityId
├─ Predicate
├─ ObjectEntityId
├─ ValidFromChapter
├─ ValidToChapter
├─ RecordedAt
├─ EvidenceEventIds
├─ Confidence
└─ Version
```

### 7.2.3 世界模拟用途

- 势力变化。
- 领土变化。
- 人物关系变化。
- 物品归属。
- 法律和规则生效时间。
- 角色身份暴露时间。
- 历史事件对当前局势的影响。

---

## 7.3 LLM Wiki 思路

- 原始思路：<https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f>
- 扩展讨论：<https://gist.github.com/rohitg00/2067ab416f7bbe447c1977edaaa681e2>

### 7.3.1 三层结构

```text
Raw Sources
├─ 正文
├─ 作者笔记
├─ 参考资料
└─ 访谈/灵感

Compiled Wiki
├─ 人物页
├─ 地点页
├─ 物品页
├─ 势力页
├─ 事件页
└─ 主题页

Schema and Lint
├─ 必填字段
├─ 交叉链接
├─ 冲突检测
├─ 过期标记
└─ 置信度
```

### 7.3.2 章节定稿后的编译流程

```text
ChapterFinalized
→ ExtractEntities
→ ExtractEvents
→ UpdateCharacterPages
→ UpdateLocationPages
→ UpdateRelationships
→ UpdateTimeline
→ UpdateForeshadowing
→ DetectContradictions
→ MarkSupersededFacts
→ RebuildAffectedEmbeddings
```

### 7.3.3 记忆生命周期

每条事实可包括：

- 来源。
- 首次出现章节。
- 最近确认章节。
- 支持证据数量。
- 反对证据。
- 置信度。
- 是否被新事实覆盖。
- 是否仍然有效。
- 是否允许被遗忘或降权。

---

# 8. 第五梯队：研究型项目与算法

## 8.1 Re3、DOC、E2EPlot

项目页：<https://storyrl.github.io/>

### 8.1.1 Re3 流程

```text
结构化总体计划
→ 根据计划和当前状态生成多个候选
→ 按情节连贯性和前提相关性重排
→ 选择最佳候选
→ 对候选进行事实一致性编辑
```

### 8.1.2 产品化建议

普通章节：生成一个候选。  
重要章节：生成两个候选。  
高潮、开篇、付费卡点：生成三个候选并评分。

评分维度：

- 与章纲的吻合度。
- 人物行为合理性。
- 因果充分性。
- 文风符合度。
- 重复率。
- 悬念推进。
- 连续性。

缺点：成本高，延迟长。因此应作为“高质量模式”而不是默认模式。

---

## 8.2 Agents’ Room

- 论文：<https://arxiv.org/abs/2410.02603>
- OpenReview：<https://openreview.net/forum?id=HfWcFs7XLR>

核心思想：叙事写作被分解为多个明确子任务，由不同 Agent 分阶段协作，而不是依赖一个复杂的万能 Prompt。

产品化时，每个 Agent 应明确：

- 输入。
- 输出 Schema。
- 工具。
- 上下文范围。
- 是否可修改正文。
- 最大步骤数。
- 失败策略。
- 质量门槛。
- Token 预算。

---

## 8.3 StoryBox

- 项目页：<https://storyboxproject.github.io/>
- 方向：多 Agent Sandbox 中通过角色互动产生涌现事件，再由 Storyteller 写成故事。

### 8.3.1 世界层级

```text
World
→ Region
→ Zone
→ Area
→ Object
```

### 8.3.2 适用场景

- 群像故事。
- 权谋。
- 战争。
- 模拟经营。
- 势力博弈。
- 游戏式叙事。

### 8.3.3 不适用场景

- 需要严格控制谜底的本格推理。
- 需要精确节拍的短篇反转。
- 成本受限的简单续写。

建议把 Sandbox 作为后期高级功能，而不是 MVP。

---

## 8.4 Patchview

- 项目页：<https://mj-storytelling.github.io/project/patchview>
- 方向：用“磁铁与灰尘”的视觉隐喻组织世界元素，并允许用户通过拖动纠正 AI 的理解。

可用于设计：

- 世界观关系画布。
- 人物与阵营距离。
- 概念相似度。
- 拖动后重新生成关系说明。
- 选中几个节点生成冲突或场景。
- 时间滑块查看关系变化。

---

## 8.5 NarrativeNode 及叙事节点编辑器

GitHub Topic：<https://github.com/topics/narrative-generation>

适合参考：

- 节点式剧情编辑。
- 场景依赖。
- 前置条件。
- 分支结局。
- 情节条件。
- AI Tool Use。
- MCP 集成。

建议在中后期使用 React Flow 实现 Plot Graph，而第一版仍以列表式卷纲和章纲为主。

---

# 9. 可补充参考的项目

## 9.1 KoboldAI

- GitHub：<https://github.com/henk717/KoboldAI>
- 适合参考：本地模型、续写模式、采样参数、作者注释、动态世界信息注入。
- 局限：更偏文本生成前端，不是完整长篇项目管理系统。

## 9.2 jacobbeasley/novelai

- GitHub：<https://github.com/jacobbeasley/novelai>
- 支持大纲、章节、场景、Word 导出和 TTS 有声书。
- 项目较小，适合理解端到端流程，但不应作为生产级底座。

## 9.3 Smart Pen / AI Novel Assistant

- GitHub：<https://github.com/stevendong/ai-novel-assistant>
- 适合参考：Vue 3、Tiptap、角色和世界观管理、PostgreSQL/Supabase、前后端分离。
- 工程规模较小，可作为常规 SaaS 结构参考。

## 9.4 NOVA

- 项目页：<https://dession-wu.github.io/ai_novel_agent/>
- 规划能力：World Bible、矛盾检测、时间线、关系、版本控制、多 Provider。
- 采用前要核实功能究竟是已实现、实验状态还是路线图。

---

# 10. 项目横向比较与选型

## 10.1 按目标选择

| 你的目标 | 优先参考 |
|---|---|
| 做成熟桌面写作产品 | Pagekin |
| 做自由 Agent 工作台 | OpenFic |
| 做整本自动生产链 | Biz Novel Studio |
| 做编辑器、画布和 Codex | OpenWrite |
| 做虚拟编辑团队 | Novel Engine |
| 做长篇状态追踪 | NstAgent |
| 做动态记忆和运行追踪 | NovelClaw |
| 做因果和场景连续性 | NovelGenerator |
| 快速理解多 Agent 后端 | LibriScribe |
| 做中文网文工作流 | awesome-novel-agent、oh-story、chinese-novelist-skill |
| 做角色主观记忆 | CharacterMemory |
| 做时间关系图谱 | Kandor |
| 做涌现剧情 | StoryBox |

## 10.2 不建议的选择方式

不要只根据 Star 决定：

- 高 Star 的 Skill 可能没有完整应用。
- 产品介绍可能比实际代码超前。
- 最近出现的项目 Star 增长快，但测试和文档可能不足。
- 许可证可能不允许闭源商用。
- Demo 能运行不代表支持长时间、多人和大量数据。

## 10.3 仓库评估清单

对每个候选仓库检查：

```text
[ ] LICENSE 是否适合你的商业模式
[ ] 最近 90 天是否仍有提交
[ ] 是否有 Release，而不只是 main 分支
[ ] 是否有安装文档
[ ] 是否有自动测试
[ ] 是否能从零运行 Demo
[ ] 数据格式是否公开
[ ] 是否绑定特定模型
[ ] 是否支持 OpenAI-compatible API
[ ] 是否有数据库迁移
[ ] 是否有任务恢复机制
[ ] 是否记录 Token 和成本
[ ] 是否支持导入和导出
[ ] Issues 中是否存在数据丢失问题
[ ] 是否存在硬编码 Prompt 和密钥风险
[ ] 依赖中是否有高危漏洞
```

---

# 11. 建议重点克隆和阅读的仓库

## 11.1 第一批：必须看

```bash
git clone https://github.com/EthanYoQ/Pagekin.git
git clone https://github.com/syrizelink/OpenFic.git
git clone https://github.com/ExplosiveCoderflome/AI-Novel-Writing-Assistant.git
git clone https://github.com/zhennan1/NstAgent.git
git clone https://github.com/iLearn-Lab/NovelClaw.git
git clone https://github.com/guerra2fernando/libriscribe.git
git clone https://github.com/modoojunko/awesome-novel-agent.git
git clone https://github.com/KazKozDev/NovelGenerator.git
```

## 11.2 推荐阅读顺序

1. 先用 Pagekin 和 OpenFic，理解真正用户会如何写一本书。
2. 阅读 LibriScribe，理解 Agent 分工。
3. 阅读 NstAgent，理解 Narrative State。
4. 阅读 NovelClaw，理解状态写回和 Trace。
5. 阅读 Biz Novel Studio，理解长任务和恢复。
6. 阅读 awesome-novel-agent，理解中文网文 SOP。
7. 最后阅读 NovelGenerator 的因果、Scene Handoff 和无人值守流程。

## 11.3 阅读时输出统一笔记

每个仓库都按同一模板记录：

```markdown
# Repository Review

## 1. 产品定位
## 2. 核心用户流程
## 3. 技术栈
## 4. 数据模型
## 5. LLM Provider 抽象
## 6. Prompt 存储方式
## 7. Agent/Workflow 设计
## 8. 上下文构造
## 9. 记忆/RAG
## 10. 后台任务和恢复
## 11. 编辑器与版本管理
## 12. 测试
## 13. 许可证
## 14. 可以借鉴
## 15. 不应照搬
```

---

# 12. 建议产品定位

## 12.1 推荐定位

> 面向中文长篇小说作者的本地优先 AI 创作工作台。以作者为最终决策者，通过结构化设定、叙事状态、检索记忆和多阶段审稿，帮助作者稳定完成从灵感到定稿的全过程。

## 12.2 与现有 AI 工具的差异

不是：

- 普通聊天框。
- 一键生成整本小说。
- 单纯续写。
- 只提供 Prompt 模板。

而是：

- 作品级知识库。
- 角色与世界状态系统。
- 卷章场景管理。
- 可恢复的生成工作流。
- 人机协作审稿。
- 可解释的引用和记忆来源。
- 模型无关。
- 本地优先。

## 12.3 目标用户

第一阶段建议只服务一种核心用户：

- 正在写 10 万字以上中文小说的个人作者。
- 愿意维护角色和设定。
- 需要 AI 协助，但不愿失去控制权。
- 可以自行提供模型 API Key，或运行 Ollama。

暂时不要同时服务：短视频脚本、营销文案、论文、商业报告和多人编剧团队。过宽定位会让工作流失去重点。

---

# 13. 完整产品功能架构

## 13.1 书房

- 新建作品。
- 导入 TXT/MD/EPUB/DOCX。
- 项目模板。
- 封面和元数据。
- 字数、章节和定稿进度。
- 最近任务。
- 本地备份和恢复。

## 13.2 创作设定

- 题材。
- 受众。
- 目标字数。
- 章节数。
- 单章长度。
- 叙事视角。
- 时间语态。
- 内容限制。
- 平台偏好。
- 文风配置。

## 13.3 Story Bible

- 人物。
- 地点。
- 势力。
- 物品。
- 能力体系。
- 社会规则。
- 历史。
- 术语。
- 关系图。
- 世界硬规则。

## 13.4 故事规划

- Logline。
- Premise。
- 主题。
- 主角欲望。
- 失败代价。
- 核心冲突。
- 结局方向。
- 全书大纲。
- 卷纲。
- 章纲。
- 场景卡。

## 13.5 正文编辑

- 富文本/Markdown。
- 自动保存。
- 历史版本。
- 选段续写。
- 扩写。
- 压缩。
- 润色。
- 对话改写。
- 视角改写。
- 文风改写。
- AI 修改 Diff。

## 13.6 审稿与修订

- 一致性检查。
- 时间线检查。
- 人物知识检查。
- 世界规则检查。
- 重复表达检查。
- 节奏检查。
- 视角检查。
- AI 味检查。
- 问题筛选。
- 定向修稿。

## 13.7 记忆与追踪

- Narrative State。
- Timeline。
- Plot Threads。
- Foreshadowing。
- Promises/Payoffs。
- Character State。
- Scene Handoff。
- Memory Change Review。

## 13.8 模型与成本

- Provider 配置。
- 模型路由。
- 调用历史。
- Token 用量。
- 费用估算。
- 并发限制。
- 缓存命中。
- 失败率和延迟。

---

# 14. 核心领域模型

建议先建立以下聚合：

## 14.1 NovelProject

```text
NovelProject
├─ Id
├─ Title
├─ Description
├─ Language
├─ Genre
├─ TargetAudience
├─ TargetWordCount
├─ Status
├─ ActiveOutlineVersionId
├─ ActiveWritingProfileId
├─ CreatedAt
└─ UpdatedAt
```

## 14.2 Manuscript

```text
Volume
└─ Chapter
   └─ Scene
```

正文建议使用“制品 + 版本”模式：

```text
Chapter
├─ Metadata
├─ CurrentRevisionId
└─ ChapterRevision[]

ChapterRevision
├─ Content
├─ SourceType: Human | AI | Mixed
├─ ParentRevisionId
├─ CreatedBy
├─ GenerationRunId
└─ CreatedAt
```

## 14.3 World Entity

统一实体基类：

```text
StoryEntity
├─ Id
├─ ProjectId
├─ EntityType
├─ Name
├─ Aliases
├─ Summary
├─ AttributesJson
├─ ValidFromChapter
├─ ValidToChapter
└─ Version
```

## 14.4 PlotThread

```text
PlotThread
├─ Title
├─ Type
├─ IntroducedChapter
├─ ExpectedPayoffRange
├─ Status
├─ Importance
├─ RelatedEntityIds
├─ EvidenceEventIds
└─ ResolutionChapter
```

状态：

```text
Planned
Introduced
Developing
Escalated
Resolved
Abandoned
Contradicted
```

## 14.5 ReviewIssue

```text
ReviewIssue
├─ Type
├─ Severity
├─ ChapterId
├─ StartOffset
├─ EndOffset
├─ Evidence
├─ Explanation
├─ SuggestedAction
├─ Status
├─ DetectedBy
└─ GenerationRunId
```

严重程度：

```text
Info
Minor
Major
Critical
```

---

# 15. 长篇小说上下文与记忆架构

## 15.1 四层记忆

### 第一层：不可违反的 Canonical Facts

- 世界硬规则。
- 已定稿的关键事实。
- 作者锁定设定。
- 人物不可变核心属性。

### 第二层：当前 Narrative State

- 当前时间。
- 人物位置和状态。
- 持有物品。
- 已知信息。
- 当前目标。
- 关系状态。
- 活跃情节线。

### 第三层：可检索的历史记忆

- 老章节片段。
- 早期事件。
- 对话。
- 场景摘要。
- 人物相关经历。

### 第四层：短期工作记忆

- 上一场景。
- 当前章纲。
- 当前作者指导。
- 当前生成草稿。

## 15.2 章节上下文组装

```text
System Instruction
+ Author Writing Profile
+ Canonical Constraints
+ Current Volume Plan
+ Current Chapter Blueprint
+ Current Narrative State
+ Relevant Character Cards
+ Relevant Timeline Events
+ Retrieved Historical Passages
+ Previous Scene Handoff
+ Last Chapter Ending
+ Author Instruction
+ Required Output Schema
```

## 15.3 上下文预算

假设模型允许 64K Token，可分配：

```text
系统和输出约束        3K
文风配置              3K
全书与卷概要          5K
章纲和场景蓝图        4K
Narrative State      8K
人物和地点卡          7K
检索历史片段         12K
上一章或上一场景      8K
生成预留             12K
缓冲                  2K
```

这只是初始建议，应根据实际模型、语言和章节长度动态调整。

## 15.4 记忆写回必须经过审核

不要让模型直接修改主状态。流程应为：

```text
定稿章节
→ AI 生成 MemoryChangeSet
→ 规则校验
→ 与旧状态 Diff
→ 作者可选审核
→ 应用变更
→ 创建新 NarrativeStateVersion
```

`MemoryChangeSet` 应包括：

- 新增事实。
- 更新事实。
- 终止事实。
- 新事件。
- 关系变化。
- 物品变化。
- 角色知识变化。
- 新伏笔。
- 已兑现伏笔。
- 新的下一章约束。

---

# 16. RAG 与检索设计

## 16.1 不要只做向量搜索

建议采用 Hybrid Retrieval：

```text
Metadata Filter
+ Keyword/BM25
+ Vector Similarity
+ Rerank
+ Rule-based Boost
```

## 16.2 索引单位

不要只按固定 Token 切块。小说更适合按语义结构切分：

- 场景。
- 事件。
- 对话段。
- 人物出场段。
- 世界设定条目。
- 章节摘要。

每个 Chunk 带元数据：

```text
ProjectId
VolumeId
ChapterId
SceneId
EntityIds
EventIds
POVCharacterId
StoryTimeRange
ContentType
CanonLevel
RevisionId
```

## 16.3 查询构造

为当前章生成检索意图：

```json
{
  "entities": ["林川", "沈月", "断裂铜印"],
  "plotThreads": ["王都叛徒"],
  "locations": ["北境石桥"],
  "timeRange": "Chapter 1-18",
  "questions": [
    "林川此前为何怀疑王都存在叛徒？",
    "断裂铜印最后一次出现在哪里？"
  ]
}
```

## 16.4 排序加权

可使用：

```text
FinalScore =
  0.35 × VectorScore
+ 0.20 × KeywordScore
+ 0.20 × EntityOverlap
+ 0.10 × Recency
+ 0.10 × NarrativeImportance
+ 0.05 × CanonConfidence
```

系数应通过测试集调优，不应固定相信一次设计。

## 16.5 检索评估

建立 50–200 个真实问题的小型基准集，测量：

- Recall@5。
- Recall@10。
- Precision@5。
- MRR。
- 必要事实遗漏率。
- 错时代事实召回率。
- 错角色知识泄露率。

---

# 17. Agent 与 Workflow 设计

## 17.1 推荐 Agent

1. Intake Agent：需求采集。
2. Story Architect：全书结构。
3. World Builder：世界规则。
4. Character Designer：人物与关系。
5. Volume Planner：卷纲。
6. Chapter Planner：章纲。
7. Scene Planner：场景蓝图。
8. Chapter Writer：正文草稿。
9. Continuity Reviewer：事实连续性。
10. Character Reviewer：人物行为和知识边界。
11. Style Reviewer：文风与 AI 痕迹。
12. Revision Agent：定向修稿。
13. Memory Curator：状态更新。
14. Export Agent：导出。

## 17.2 不要过度 Agent 化

起步阶段无需 14 个独立服务。可先用一个 Workflow Engine 和多个 Agent Definition：

```text
同一个 Worker
+ 不同 Prompt
+ 不同 Context Policy
+ 不同 Output Schema
+ 不同 Tool Permission
```

只有当并发、隔离或成本需要时，才拆成独立 Worker。

## 17.3 工作流定义示例

```yaml
name: chapter-production-v1
steps:
  - id: plan
    agent: chapter-planner
    output: ChapterBlueprint
    approval: required

  - id: draft
    agent: chapter-writer
    input:
      - ChapterBlueprint
      - NarrativeState
    output: ChapterDraft

  - id: continuity-review
    agent: continuity-reviewer
    output: ReviewIssue[]

  - id: style-review
    agent: style-reviewer
    output: ReviewIssue[]

  - id: revise
    agent: revision-agent
    condition: approvedIssues.count > 0
    output: ChapterRevision

  - id: finalize
    type: human-approval

  - id: memory-update
    agent: memory-curator
    output: MemoryChangeSet
```

## 17.4 工具权限

示例：

```text
Writer
- ReadChapterBlueprint
- ReadContextPackage
- CreateDraft
- 不允许 UpdateCanonicalFacts

MemoryCurator
- ReadFinalizedChapter
- ReadNarrativeState
- ProposeMemoryChangeSet
- 不允许直接覆盖状态

Reviewer
- ReadManuscript
- SearchStoryMemory
- CreateReviewIssue
- 不允许修改正文
```

---

# 18. 章节生产流水线

## 18.1 完整流程

```text
1. 检查前置条件
2. 生成/加载章纲
3. 构建 Context Package
4. 检索相关历史
5. 验证 Token Budget
6. 生成草稿
7. 结构化解析
8. 连贯性审查
9. 人物审查
10. 文风审查
11. 作者选择问题
12. 定向修订
13. Diff 审核
14. 作者定稿
15. 提取 MemoryChangeSet
16. 验证记忆变更
17. 应用状态版本
18. 生成 Scene Handoff
19. 更新检索索引
20. 解锁下一章
```

## 18.2 章纲 Schema

```json
{
  "chapterNumber": 18,
  "title": "桥下的军械箱",
  "povCharacterId": "lin-chuan",
  "storyTime": "冬月初六夜",
  "locations": ["north-stone-bridge"],
  "characters": ["lin-chuan", "shen-yue"],
  "chapterGoal": "确认叛徒与王都军械的联系",
  "openingHook": "桥下传来铁链拖过石面的声音",
  "conflicts": [],
  "turningPoint": "军械箱上的编号属于早已撤销的禁军营",
  "requiredFacts": [],
  "forbiddenContradictions": [],
  "foreshadowingToAdvance": [],
  "endingHook": "箱底刻着林川父亲的旧名",
  "targetWordCount": 3000
}
```

## 18.3 失败处理

- 结构化输出解析失败：先执行 JSON Repair，不立即重复昂贵生成。
- 模型限流：指数退避并尊重 Retry-After。
- 上下文超限：按策略重新压缩，不随机截断。
- 内容检查失败：保存原始输出，创建失败制品，允许人工查看。
- 用户取消：传递 CancellationToken，并标记 Cancelled，不标记 Failed。
- Worker 崩溃：租约到期后由其他 Worker 恢复。

---

# 19. 一致性检查体系

## 19.1 检查分类

### A. 硬事实

- 人物姓名、年龄、身份。
- 地点与距离。
- 物品归属。
- 伤势。
- 生死状态。
- 世界规则。

### B. 时间线

- 旅行时间。
- 昼夜。
- 事件先后。
- 年龄计算。
- 同一人物是否同时出现在两地。

### C. 角色知识

- 角色是否知道该秘密。
- 信息来源是否成立。
- 错误信念是否被无故纠正。
- 对话是否泄露作者视角信息。

### D. 人物行为

- 动机。
- 当前目标。
- 关系状态。
- 性格变化是否有铺垫。

### E. 情节结构

- 章纲要求是否完成。
- 伏笔是否推进。
- 是否过早兑现。
- 是否出现无前因事件。
- 是否重复相同冲突。

### F. 语言和风格

- 叙事视角漂移。
- 时态漂移。
- 高频套话。
- 过度总结。
- 对话同质化。
- 不必要排比。
- AI 常见模板句。

## 19.2 两阶段检查

第一阶段使用确定性规则：

- 名称和别名。
- 时间计算。
- 物品状态。
- 章节编号。
- 字数。
- 禁词。
- 重复 n-gram。

第二阶段使用 LLM 判断：

- 动机。
- 节奏。
- 潜台词。
- 关系变化。
- 因果合理性。
- 主题和文风。

## 19.3 问题必须带证据

不接受：

> 这一章人物可能不一致。

应返回：

```json
{
  "type": "CharacterKnowledgeViolation",
  "severity": "Major",
  "character": "林川",
  "textEvidence": "林川立即认出铜印属于内廷密探",
  "stateEvidence": "截至第17章，林川只知道铜印与王都有关，不知道内廷密探标记",
  "explanation": "角色使用了尚未获得的信息",
  "suggestedAction": "改为怀疑铜印来自王都，或在本章增加合理的信息获取过程"
}
```

---

# 20. Prompt 工程方案

## 20.1 Prompt 必须版本化

```text
PromptTemplate
├─ Id
├─ Name
├─ Version
├─ Role
├─ Template
├─ InputSchema
├─ OutputSchema
├─ ChangeLog
├─ CreatedAt
└─ DeprecatedAt
```

## 20.2 Writer Prompt 结构

```text
1. 角色职责
2. 不可违反规则
3. 小说文风
4. 本章目标
5. 章纲
6. 当前叙事状态
7. 相关角色
8. 历史证据
9. 上一场景交接
10. 作者额外要求
11. 输出格式
12. 自检清单
```

## 20.3 审稿 Prompt 不要诱导改写

Reviewer 只输出问题，不直接重写正文。否则会混淆“发现问题”和“产生新版本”两个阶段。

## 20.4 结构化输出

优先使用 JSON Schema 或受约束输出。每个 Schema 必须：

- 字段清晰。
- 枚举固定。
- 允许空数组。
- 不要求模型生成数据库 ID。
- 服务端重新验证。
- 保存原始响应。

## 20.5 防止 Prompt 膨胀

- 共用规则放静态 System Prompt。
- 项目规则以精简 Canonical Summary 注入。
- 历史正文只按需检索。
- 同一内容避免同时以全文、摘要和状态重复注入。
- 为 Context Package 生成 Token 统计。

---

# 21. 模型路由与成本控制

## 21.1 任务分级

| 任务 | 推荐模型等级 | 原因 |
|---|---|---|
| JSON 抽取 | 低成本模型 | 格式明确、可重试 |
| 摘要和标签 | 低成本模型 | 风险较低 |
| 章纲 | 中高等级 | 需要结构和因果 |
| 正文写作 | 高等级 | 文风和长程连贯性重要 |
| 一致性检查 | 中高等级 | 需要理解多源状态 |
| 文风检查 | 高等级或专用模型 | 主观性强 |
| Embedding | 专用 Embedding 模型 | 成本和稳定性更好 |
| Rerank | 专用 Reranker | 提升检索精度 |

## 21.2 路由策略

```text
ModelPolicy
├─ PreferredModels
├─ FallbackModels
├─ MaxInputTokens
├─ MaxOutputTokens
├─ MaxCostPerRun
├─ Timeout
├─ Temperature
├─ SupportsJsonSchema
└─ ProviderConcurrencyLimit
```

## 21.3 成本管理

必须记录：

- 输入 Token。
- 输出 Token。
- 缓存 Token。
- 模型单价快照。
- 估算费用。
- 重试消耗。
- 每部作品累计费用。
- 每章费用。
- 每种任务费用。

可设置：

```text
项目日预算
单章预算
单次调用预算
高质量模式最大候选数
达到预算后的停止策略
```

## 21.4 缓存

适合缓存：

- Embedding。
- Rerank 输入相同的结果。
- 不变设定摘要。
- Prompt 编译结果。
- Token 计数。

不应盲目缓存：

- 带随机性的正文生成。
- 状态已改变后的旧上下文结果。
- 包含临时作者指导的生成。

---

# 22. Go 优先的技术架构建议

## 22.1 最终语言结论

本项目推荐采用多语言分层，而不是强求全栈统一语言：

```text
核心业务后端：Go
AI 推理与模型算法：Python
Web 前端：TypeScript + React
桌面壳：Tauri
数据库：PostgreSQL + pgvector
长工作流：Temporal
大规模向量检索：Qdrant（达到真实瓶颈后再引入）
```

### 为什么核心后端选择 Go

AI 小说系统主要是 I/O 密集型：等待 LLM Token、PostgreSQL、向量检索、对象存储和人工审批。即便 Rust 将某段 API 内部处理从 4 ms 降低到 2 ms，相比一次 20–120 秒的章节生成，用户几乎感知不到。真正影响体验的是首 Token 延迟、上下文长度、模型并发、Prompt Cache、检索质量、失败恢复和流式输出。

Go 的优势：

- Goroutine 和 Channel 适合模型并发调用、Worker Pool 和 Fan-out/Fan-in。
- `context.Context` 可统一传播取消、超时和请求生命周期。
- 编译速度快，部署为单一二进制。
- `net/http`、`pprof`、Race Detector 和测试工具成熟。
- Temporal Go SDK 成熟，适合跨小时或跨天的 Durable Workflow。
- 开发和维护成本明显低于全 Rust，更适合个人或 2–5 人团队。
- 性能远高于本产品早期的真实需求上限。

### Rust 的正确位置

Rust 不是被排除，而是不作为第一版核心业务语言。以下情况再使用 Rust：

- 自研高吞吐模型网关或 Tokenizer。
- 需要极低内存占用和稳定 P99/P999 延迟。
- 编写本地桌面侧高性能核心。
- 自研检索、索引或文本处理内核。
- 团队已有成熟 Rust 经验。

### Python 的正确位置

Python 只负责 AI 生态强相关能力，不承担核心业务数据库和权限：

- vLLM GPU 推理。
- Transformers/PyTorch。
- Embedding。
- Cross-Encoder Reranker。
- 模型量化与 LoRA。
- 实验性 NLP 和模型评测。

Go 通过 HTTP/gRPC 或 OpenAI-compatible API 调用 Python 服务。Python 服务崩溃、GPU OOM 或升级时，不应带崩核心应用。

## 22.2 总体架构

```text
React + TypeScript + Tiptap
        │
        │ HTTPS + SSE/WebSocket
        ▼
Go Modular Monolith
├─ API/BFF
├─ Project
├─ Manuscript
├─ Story Bible
├─ Narrative State
├─ Context Builder
├─ Model Gateway
├─ Retrieval
├─ Review
├─ Cost/Quota
└─ Workflow Client
        │
        ├─ PostgreSQL + pgvector
        ├─ S3 / MinIO
        ├─ Temporal
        ├─ Cloud LLM APIs
        └─ Python/vLLM + Embedding/Reranker
```

第一版必须采用**模块化单体**，而不是微服务。建议部署为：

```text
1 个 Go API
1 个 Go Worker
1 个 PostgreSQL
1 个 React 前端
外部云模型 API
Docker Compose
```

只有以下边界明确且具备独立扩缩容需求时才拆服务：

1. Inference Service：GPU 和 Python 依赖独立。
2. Retrieval Service：检索负载开始影响事务数据库。
3. Export Service：EPUB、DOCX、PDF 依赖复杂且容易阻塞。

## 22.3 Go 工程技术选型

```text
HTTP             net/http + chi
Database Driver  pgx
SQL              sqlc
Migration        Atlas 或 golang-migrate
Workflow         Temporal Go SDK
Logging          slog 或 zap
Tracing          OpenTelemetry
Metrics          Prometheus
Validation       go-playground/validator
JSON Schema      santhosh-tekuri/jsonschema
SSE              net/http 原生流式响应
WebSocket        coder/websocket
CLI              Cobra
Testing          testing + testcontainers-go
Config           环境变量 + koanf（可选）
```

### 为什么优先 chi，不默认选择 Fiber

本项目的性能瓶颈不在路由器。`chi` 与标准库 `net/http` 完全兼容，HTTP Client、Tracing、中间件、SSE、测试和第三方库集成更自然。Fiber 的 Benchmark 很快，但其 `fasthttp` 生态与标准库存在差异，不值得为微小的路由层收益增加长期摩擦。

### 为什么使用 pgx + sqlc

- `pgx` 对 PostgreSQL 特性支持好。
- `sqlc` 根据真实 SQL 生成类型安全 Go 代码。
- 复杂检索、JSONB、锁和批处理可直接优化 SQL。
- 避免 ORM 在复杂查询和批量任务中生成不可控 SQL。

不建议核心数据层过度依赖重量 ORM。若使用 GORM，应限制在简单 CRUD，而 Narrative State、任务租约、检索过滤和统计查询仍使用显式 SQL。

## 22.4 解决方案目录

```text
cmd/
├─ api/
├─ worker/
└─ cli/

internal/
├─ project/
├─ manuscript/
├─ storybible/
├─ narrative/
├─ planning/
├─ generation/
├─ review/
├─ memory/
├─ retrieval/
├─ llm/
├─ workflow/
├─ persistence/
├─ observability/
└─ platform/

api/
├─ openapi.yaml
└─ generated/

prompts/
schemas/
workflows/
migrations/
web/
tests/
evaluation/
```

### 模块边界原则

- 模块拥有自己的业务规则和 Repository 接口。
- 其他模块不能直接修改其内部表。
- API DTO、数据库模型和领域对象分离。
- LLM 返回的 JSON 永远视为不可信输入，必须经过 Schema 和业务验证。
- Narrative State 只能通过显式变更集更新。
- 所有跨模块副作用通过应用服务或领域事件协调。

## 22.5 并发模型

Go 特别适合以下模式：

### 并行审稿

```text
章节草稿完成
├─ Continuity Reviewer
├─ Character Reviewer
└─ Style Reviewer
      ↓
合并 ReviewIssue
```

使用 `errgroup.WithContext` 并行调用。任一关键审稿失败时可以取消同组任务；非关键审稿失败则记录部分成功，不应丢弃已有结果。

### Provider 并发控制

为每个 Provider 和模型维护独立的信号量：

```text
OpenAI/model-A       20 concurrent
Anthropic/model-B    10 concurrent
Local vLLM/model-C    4 concurrent
Embedding            64 batch items
```

具体数字必须根据账号限额、GPU 显存和压测结果调整，不能写死在代码中。

### 取消传播

```text
HTTP Request Cancelled
→ context cancelled
→ Model Gateway 停止读取流
→ Activity 请求取消
→ Job 标记 Cancelled
→ 已产生的部分输出保存为 Draft Artifact
```

不要把用户取消记录为系统失败。

## 22.6 Temporal 工作流

当流程包含人工等待、长时间运行、服务重启恢复和多步骤补偿时，引入 Temporal：

```text
ChapterWorkflow
├─ BuildContextActivity
├─ GenerateDraftActivity
├─ ParallelReviewActivities
├─ WaitForAuthorSignal
├─ ReviseActivity
├─ WaitForFinalizeSignal
├─ ExtractMemoryChangeSetActivity
├─ ApplyNarrativeStateActivity
└─ ReindexActivity
```

边界要求：

- Workflow 只做确定性编排。
- 数据库、LLM、Qdrant、对象存储调用全部放在 Activity。
- Activity 必须以幂等键防止重复扣费和重复写入。
- 人工审批使用 Signal。
- 长批量任务通过 Continue-As-New 控制历史大小。
- 工作流版本升级使用 Temporal Versioning/Patching 策略。

MVP 暂不引入 Temporal 时，可用 PostgreSQL Job Table + `FOR UPDATE SKIP LOCKED` + Lease + Retry + Idempotency Key。出现人工审批、多步骤恢复或任务跨版本升级需求时应尽快迁移，不要继续自研复杂工作流引擎。

## 22.7 数据与检索

### 第一阶段

```text
PostgreSQL
├─ 关系型权威数据
├─ JSONB 扩展属性
├─ Full-Text Search
└─ pgvector
```

优点是事务、JOIN、过滤和向量检索在一个系统内完成，运维最简单。

### 何时加入 Qdrant

满足以下任一真实条件后再评估：

- 向量规模达到数千万并持续增长。
- 检索 QPS 和索引构建明显影响 PostgreSQL 事务负载。
- 需要复杂稠密、稀疏、多向量混合检索。
- 需要检索层独立扩缩容。

此时：

```text
PostgreSQL = Source of Truth
Qdrant = Derived Search Index
```

Qdrant 索引必须可以从 PostgreSQL 和对象存储重建，不能成为正文、角色状态或 Canonical Facts 的唯一来源。

## 22.8 前端与桌面

前端保持 TypeScript：

```text
React
Tiptap
TanStack Query
Zustand
React Flow
Yjs（多人协作阶段）
```

桌面化优先 Tauri：

```text
React Web UI
+ Tauri Shell
+ 本地 SQLite/PostgreSQL 兼容存储层
+ Go Core 作为本地 Sidecar 或远程 API
```

如果最终产品明确只做桌面单机，也可以评估 Rust/Tauri 本地核心，但不要因此重写已经稳定的 Go 领域逻辑，除非性能测试证明有必要。

## 22.9 部署演进

### 阶段 1：MVP

```text
Go API
Go Worker
PostgreSQL + pgvector
React
云端 LLM
Docker Compose
```

### 阶段 2：产品验证后

```text
Temporal
Python/vLLM
Embedding Service
Reranker
S3/MinIO
OpenTelemetry + Prometheus + Grafana
```

### 阶段 3：规模化

```text
Qdrant
独立 Inference Service
独立 Export Service
只在真实扩缩容需求下使用 Kubernetes
```

不要在第一版引入 Kubernetes、Service Mesh、Kafka 和十几个微服务。它们不会提高小说质量，却会增加本地开发、部署、排错和版本协调成本。

## 22.10 Go 与 Rust 的最终决策表

| 条件 | 选择 Go | 选择 Rust |
|---|---:|---:|
| 个人或 2–5 人团队 | 是 | 通常否 |
| 6 个月内推出产品 | 是 | 风险较高 |
| 主要调用云端 LLM | 是 | 收益有限 |
| 高并发 I/O、SSE、Worker | 是 | 也适合 |
| 极低内存和稳定 P999 | 可用 | 更优 |
| 自研模型网关/检索内核 | 可用 | 更优 |
| 团队已有 Rust 专家 | 可选 | 是 |
| 快速调整领域模型与工作流 | 更优 | 较慢 |

本项目默认结论：**Go 是综合工程最优选择，Rust 是特定基础设施场景的性能上限选择。**

---

# 23. 数据库与存储设计

## 23.1 MVP：SQLite

适合单机桌面：

- 部署简单。
- 数据文件可备份。
- 无需数据库服务。
- 配合 FTS5 做关键词检索。

## 23.2 服务版：PostgreSQL

适合：

- 多用户。
- Worker 并发。
- JSONB。
- pgvector。
- 行锁和事务。
- 复杂查询。

## 23.3 文件和数据库的边界

推荐：

- 数据库保存元数据、状态、索引和运行记录。
- 正文 Revision 可保存在数据库，也可作为 Markdown 文件保存。
- 导出制品保存在文件存储。
- 原始模型响应可压缩保存到对象存储。

## 23.4 备份

每个项目支持：

```text
project.json
manuscript/
world/
planning/
memory/
prompts/
runs/
assets/
```

导出包应可在无数据库时恢复基本内容。

---

# 24. 后台任务、并发和可靠性

## 24.1 Job 数据结构

```text
GenerationJob
├─ Id
├─ ProjectId
├─ JobType
├─ Status
├─ Priority
├─ InputArtifactIds
├─ OutputArtifactId
├─ IdempotencyKey
├─ LeaseOwner
├─ LeaseExpiresAt
├─ Attempt
├─ MaxAttempts
├─ ScheduledAt
├─ StartedAt
├─ CompletedAt
├─ CancellationRequestedAt
└─ Error
```

## 24.2 状态

```text
Pending
Leased
Running
WaitingForUser
Paused
RetryScheduled
Succeeded
Failed
Cancelled
DeadLettered
```

## 24.3 幂等

例如：

```text
IdempotencyKey =
ProjectId + ChapterId + TaskType +
InputRevisionHash + PromptVersion + ModelPolicyVersion
```

重复请求时返回既有任务，而不是重复花费 Token。

## 24.4 并发限制

推荐限制层级：

- Provider 全局并发。
- 模型并发。
- 用户并发。
- 项目并发。
- 同一章节写操作互斥。

同一项目可以并行进行不冲突的读取/分析任务，但不要并行修改同一个 Narrative State 基线。

## 24.5 Channel 与 Worker

Go MVP 可用：

```text
goroutine
+ buffered channel
+ errgroup
+ context.Context
+ weighted semaphore
+ PostgreSQL lease
```

对 LLM 流式调用，必须在读取响应流、SSE 转发、任务状态和数据库写入之间传播同一个取消上下文。对于 CPU 密集的 Token 统计、Diff 或压缩任务，应设置独立 Worker Pool，不能无界创建 Goroutine。

多人/分布式版本再引入：

- PostgreSQL Job Queue。
- Hangfire。
- Quartz.NET。
- RabbitMQ。
- Azure Service Bus。

不要在第一版同时引入多个队列系统。

## 24.6 重试策略

可重试：

- 429。
- 临时 5xx。
- 网络超时。
- Provider 临时不可用。

不自动重试：

- API Key 无效。
- 余额不足。
- Prompt/Schema 配置错误。
- 内容超过硬限制且压缩策略失败。
- 用户主动取消。

## 24.7 可观测性

OpenTelemetry Trace：

```text
WorkflowRun
└─ ChapterProduction
   ├─ BuildContext
   ├─ RetrieveMemory
   ├─ LlmDraft
   ├─ ParseOutput
   ├─ ContinuityReview
   ├─ Revision
   └─ MemoryUpdate
```

指标：

- 任务成功率。
- P50/P95 延迟。
- Provider 429 比率。
- JSON 解析失败率。
- 平均单章 Token。
- 平均单章成本。
- 重试率。
- 上下文裁剪次数。
- 用户接受 AI 修改比例。

---

# 25. 编辑器与前端交互

## 25.1 编辑器

推荐 Tiptap：

- 扩展能力强。
- ProseMirror 生态。
- 适合评论、选区和自定义节点。
- 可构建 AI 操作菜单。

## 25.2 AI 修改必须用 Diff

显示方式：

- 删除：红色。
- 新增：绿色。
- 改写：逐段并排。
- 支持接受/拒绝单个变化。
- 保留原版本。

## 25.3 上下文透明

每次 AI 操作旁显示：

- 使用了哪些人物卡。
- 使用了哪些历史章节。
- 使用了哪个 Narrative State 版本。
- 使用了哪个 Prompt 版本。
- 使用的模型。
- Token 和费用。

这样用户发现错误时可以判断是召回错误、状态错误还是模型错误。

## 25.4 任务 UI

```text
任务名称
状态
当前步骤
进度
已用时间
Token
估算费用
暂停
取消
查看日志
查看原始输出
重试
```

## 25.5 不要让聊天框占据产品中心

聊天适合探索，但核心写作仍应围绕：

- 项目树。
- 正文编辑器。
- 章纲。
- 状态卡。
- 审稿问题。
- 版本记录。

---

# 26. 安全、隐私与版权

## 26.1 API Key

- 桌面端使用操作系统安全存储。
- 服务端使用加密 Secret Store。
- 日志不能记录完整 Key。
- 前端不得保存服务端 Provider Key。
- 支持用户自带 Key。

## 26.2 原稿隐私

- 默认本地保存。
- 明确标注调用云模型时哪些文本会发送到第三方。
- 提供本地模型模式。
- 提供删除项目和模型调用数据的功能。
- 不默认使用用户原稿训练模型。

## 26.3 参考作品与仿写

产品应避免：

- 要求模型复制某位在世作者的大段独特表达。
- 直接输出近似原文。
- 将导入的版权小说混入用户正文索引并无边界调用。

更安全的做法：

- 提取高层特征：句长、视角、节奏、对话比例、描写密度。
- 生成抽象 Writing Profile。
- 运行近似度/长片段重复检查。
- 参考文本索引与用户原稿索引分开。

## 26.4 Prompt Injection

导入文档中的文字可能包含类似“忽略之前指令”。因此：

- 检索内容作为不可信数据。
- 明确标记引用边界。
- Tool 参数服务端校验。
- Agent 不得根据检索片段提升权限。
- 禁止任意文件系统路径访问。

---

# 27. 测试与质量评估

## 27.1 单元测试

- 章节状态机。
- Narrative State 应用。
- 时间关系有效区间。
- Token Budget。
- Context 排序。
- IdempotencyKey。
- Retry 分类。

## 27.2 集成测试

- 数据库事务。
- Worker 租约。
- Provider Mock。
- 流式输出。
- JSON Repair。
- Embedding 索引。
- 导入导出。

## 27.3 Golden Tests

保存固定输入和期望结构，不要求模型文字完全相同，而检查：

- 必填字段。
- 不得改变的事实。
- 必须出现的事件。
- 禁止信息泄露。
- 字数范围。
- POV。

## 27.4 长篇一致性测试集

自己构造一部 20–30 章的小型测试小说，故意设置：

- 物品多次转手。
- 角色受伤。
- 秘密只被个别人知道。
- 关系由盟友变敌人。
- 跨章伏笔。
- 时间跳跃。
- 同名别名。
- 假线索。

检查系统是否能正确生成、检索和发现矛盾。

## 27.5 人工评价

每章 1–5 分：

- 章纲完成度。
- 人物一致性。
- 世界一致性。
- 因果。
- 节奏。
- 文风。
- 可读性。
- 重复度。
- 修改成本。

最重要的产品指标不是模型自评分，而是：

- 作者保留了多少 AI 草稿。
- 作者修改比例。
- 从草稿到定稿所需时间。
- 连续写作多少章后仍保持稳定。

---

# 28. MVP 范围与开发路线图

## 28.1 第 0 阶段：技术验证，1–2 周

目标：证明单章流水线可工作。

- Console 或简易 Web。
- OpenAI-compatible Client。
- 项目、人物、章纲 JSON。
- 单章生成。
- 结构化审稿。
- 记录 Token 和耗时。

退出标准：能稳定跑完“章纲 → 草稿 → 问题”流程 20 次。

## 28.2 第 1 阶段：写作 MVP，4–6 周

- 项目管理。
- 角色和世界观。
- 卷、章结构。
- Tiptap 编辑器。
- 单章流式生成。
- 章节 Revision。
- AI Diff。
- SQLite。
- 基础设置和 Provider。

退出标准：可以从新建作品写出并定稿前 3 章，应用重启后数据不丢失。

## 28.3 第 2 阶段：长篇记忆，4–6 周

- Narrative State。
- Timeline。
- PlotThread。
- Foreshadowing。
- Scene Handoff。
- 章节定稿后的 MemoryChangeSet。
- 记忆 Diff 和版本。

退出标准：测试小说连续生成 20 章，关键人物、物品和秘密没有明显无故漂移。

## 28.4 第 3 阶段：RAG，3–5 周

- pgvector 或本地向量库。
- 场景/事件切块。
- Hybrid Retrieval。
- Rerank。
- 检索引用可视化。
- 检索基准集。

退出标准：关键事实 Recall@10 达到团队设定门槛，例如 90% 以上；实际阈值应由测试集决定。

## 28.5 第 4 阶段：可靠后台任务，3–5 周

- 持久 Job。
- 暂停/取消/恢复。
- 幂等。
- Retry。
- Worker 并发限制。
- SignalR 进度。
- OpenTelemetry。

退出标准：进程在任意步骤被杀死后，重启能安全恢复，不重复写状态、不重复扣费或能识别重复调用。

## 28.6 第 5 阶段：可配置 Agent 和 Workflow，4–6 周

- Prompt 管理。
- Agent Definition。
- Workflow Definition。
- Tool Permission。
- Schema 编辑。
- 内置模板。

退出标准：无需改代码即可替换章节审稿流程或增加新的 Reviewer。

## 28.7 暂缓功能

- 社区。
- 多人实时协作。
- 自动发网文平台。
- 小说转漫画/视频。
- 有声书。
- 复杂世界模拟。
- 多租户计费。
- 移动 App。

这些功能很诱人，但会分散对“长篇稳定写作”核心价值的投入。

---

# 29. 商业化前的许可证检查

## 29.1 许可证大致影响

### MIT / Apache-2.0

通常更适合商业借鉴和二次开发，但仍需保留要求的版权和许可证声明。Apache-2.0 还包含明确的专利条款。

### GPL-3.0

如果你分发包含 GPL 代码的衍生应用，通常会带来源码开放义务。具体是否构成衍生作品需要专业法律判断。

### AGPL-3.0

除了分发，还可能涉及通过网络提供服务时的源码提供义务。对闭源 SaaS 尤其需要谨慎。

### BSL

Business Source License 不是传统开源许可证，常在一定日期后转换为开放许可证，但在转换前可能限制生产或竞争用途。

## 29.2 你的安全策略

1. 把项目作为“设计参考”和“行为参考”。
2. 不逐段复制受强 Copyleft 限制的实现。
3. 对可采用的 MIT/Apache 组件建立第三方清单。
4. 保存依赖的 LICENSE 和 NOTICE。
5. 商业发布前做一次软件许可证审计。
6. 对 Skill、Prompt 和知识库内容也检查版权，不要只检查代码。

## 29.3 建议的第三方清单字段

```text
Component
Version
Repository
License
Modified?
Distributed?
UsedAsService?
NoticeRequired?
SourceDisclosureRisk?
Owner
ReviewDate
```

---

# 30. 最终建议

## 30.0 技术栈最终更新

本方案的默认技术栈已从 C# 调整为：

```text
Go 核心后端
+ Python AI 推理
+ TypeScript/React 前端
+ PostgreSQL/pgvector
+ Temporal 长工作流
+ Qdrant（规模达到真实瓶颈后）
```

这里的“性能最优”不是只比较 HTTP Benchmark，而是同时优化吞吐、尾延迟、开发速度、故障恢复、可维护性和团队成本。对于 AI 小说系统，Go 更接近综合工程最优；Rust 保留在真正需要极低资源和基础设施级性能的组件中。

## 30.1 最优组合

不要复制一个项目，而是组合思想：

```text
Pagekin
→ 用户体验、章节生命周期、审稿与 Diff

OpenFic
→ Agent、RAG、上下文、跨平台和 Provider

Biz Novel Studio
→ 整本生产任务、恢复和状态回灌

NstAgent
→ Narrative State Tracking

NovelClaw
→ 动态记忆、Trace、状态快照

LibriScribe
→ Agent 边界、统一模型客户端、结构化输出

awesome-novel-agent
→ 中文网文 SOP、卷章工作流、文件化知识

NovelGenerator
→ 因果链、Scene Handoff、连续性检查

CharacterMemory + Kandor
→ 角色主观知识和时间关系图谱
```

## 30.2 最关键的三个技术资产

如果只能优先实现三项，应选择：

1. **Narrative State**：让系统知道当前世界处于什么状态。
2. **Chapter Workflow**：让每一章都经过可恢复、可审核的流程。
3. **Revision + Trace**：让作者可以看懂 AI 做了什么并随时回退。

## 30.3 第一版目录建议

```text
NovelAI/
├─ src/
├─ tests/
├─ prompts/
│  ├─ planning/
│  ├─ writing/
│  ├─ review/
│  └─ memory/
├─ schemas/
├─ workflows/
├─ evaluation/
│  ├─ fixtures/
│  ├─ retrieval-benchmark/
│  └─ long-story-benchmark/
├─ docs/
│  ├─ architecture.md
│  ├─ domain-model.md
│  ├─ context-policy.md
│  ├─ prompts.md
│  └─ licenses.md
└─ samples/
```

## 30.4 下一步具体行动

### 第一天

- Clone Pagekin、OpenFic、NstAgent、LibriScribe。
- 分别运行 Demo。
- 按统一模板记录架构和功能。

### 第一周

- 建立 Go Module、`cmd/api`、`cmd/worker` 和 `internal` 模块目录。
- 用 `pgx + sqlc` 实现 `NovelProject`、`Character`、`Chapter`、`ChapterRevision`。
- 接入一个 OpenAI-compatible Provider。
- 使用 `net/http + SSE` 跑通单章流式生成。

### 第二周

- 加入 ChapterBlueprint。
- 加入结构化 ReviewIssue。
- 加入 Revision Diff。
- 记录 Token、模型、Prompt 版本和运行日志。

### 第三至第四周

- 实现 NarrativeState v1。
- 实现章节定稿后的 MemoryChangeSet。
- 做一部 10 章测试小说。
- 每章验证时间、位置、物品、秘密和关系。

## 30.5 最终判断

这个方向有现实价值，但“生成文字”本身并不是最强护城河。大量通用 AI 工具已经可以生成和改写文本。真正有差异化的部分是：

- 长篇状态管理。
- 中文网文方法论。
- 角色知识边界。
- 伏笔和承诺追踪。
- 可恢复的整书工作流。
- 人工可控的审稿与 Diff。
- 引用透明的记忆检索。
- 本地隐私和模型自由。

因此，产品目标不应是“比通用聊天工具更会写一句话”，而应是：

> **让作者能够在几十万字、数百个设定和长期创作过程中，持续知道故事走到哪里、人物知道什么、哪些伏笔尚未兑现，并能安全地让 AI 参与每一个步骤。**

---

# 附录 A：主要项目链接

1. Pagekin：<https://github.com/EthanYoQ/Pagekin>
2. OpenFic：<https://github.com/syrizelink/OpenFic>
3. Biz Novel Studio：<https://github.com/ExplosiveCoderflome/AI-Novel-Writing-Assistant>
4. OpenWrite：<https://github.com/ilrein/openwrite>
5. Novel Engine：<https://john-paul-ruf.github.io/novel-engine/>
6. NovelClaw：<https://github.com/iLearn-Lab/NovelClaw>
7. NstAgent：<https://github.com/zhennan1/NstAgent>
8. NovelGenerator：<https://github.com/KazKozDev/NovelGenerator>
9. LibriScribe：<https://github.com/guerra2fernando/libriscribe>
10. awesome-novel-agent：<https://github.com/modoojunko/awesome-novel-agent>
11. oh-story-claudecode：<https://github.com/zenstory-ai/oh-story-claudecode>
12. chinese-novelist-skill：<https://github.com/PenglongHuang/chinese-novelist-skill>
13. AIStoryWriter：<https://github.com/datacrystals/AIStoryWriter>
14. CharacterMemory：<https://github.com/FrancescoCaracciolo/CharacterMemory>
15. Long-Story Agent：<https://xiao-zi-chen.github.io/CoLong-Idea-Studio/>
16. StoryBox：<https://storyboxproject.github.io/>
17. Long-form Story Generation and Evaluation：<https://storyrl.github.io/>
18. Agents’ Room：<https://arxiv.org/abs/2410.02603>
19. Patchview：<https://mj-storytelling.github.io/project/patchview>
20. Kandor：<https://harryluumn.github.io/kandor/>
21. LLM Wiki：<https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f>
22. KoboldAI：<https://github.com/henk717/KoboldAI>
23. Small NovelAI：<https://github.com/jacobbeasley/novelai>
24. Smart Pen：<https://github.com/stevendong/ai-novel-assistant>
25. NOVA：<https://dession-wu.github.io/ai_novel_agent/>

---

# 附录 B：开始开发前的决策表

```text
[ ] 产品是本地桌面、Web 自托管，还是 SaaS？
[ ] 第一目标用户是中文网文作者还是传统小说作者？
[ ] 第一版是否允许一键批量成书？
[ ] 用户是否自带 API Key？
[ ] 是否支持 Ollama？
[ ] 正文用 Markdown 还是富文本 JSON？
[ ] SQLite 与 PostgreSQL 如何切换？
[ ] Narrative State 的最小 Schema 是什么？
[ ] 哪些事实属于 Canon，不允许模型自行修改？
[ ] 章节定稿后记忆是否需要人工确认？
[ ] Prompt 是否可由用户编辑？
[ ] Agent 是否允许调用文件和网络工具？
[ ] 单章成本上限是多少？
[ ] 如何处理导入的版权文本？
[ ] 如何导出和恢复整个项目？
[ ] 采用的开源代码是否完成许可证审计？
```

---

# 附录 C：术语表

- **Story Bible**：小说世界设定总库，包括人物、地点、规则、物品和历史。
- **Narrative State**：某一章节之后故事世界的当前结构化状态。
- **Canonical Fact**：已经确认、后续不得随意违反的事实。
- **Scene Handoff**：一个场景结束时交给下一场景的状态说明。
- **Plot Thread**：跨章节推进的情节线。
- **Foreshadowing**：伏笔。
- **Promise/Payoff**：作者向读者建立的期待及其兑现。
- **RAG**：检索增强生成，在生成前检索相关资料并提供给模型。
- **Rerank**：对初步检索结果进行二次排序。
- **Agent**：具有明确职责、上下文、工具和输出约束的模型执行单元。
- **Workflow**：多个任务和人工节点组成的可恢复流程。
- **Artifact**：工作流产生的可保存制品，如章纲、草稿、审稿报告和状态变更。
- **Run Trace**：一次任务使用的输入、Prompt、模型、工具、输出、耗时和成本记录。
- **Idempotency**：同一请求重复执行不会产生重复副作用。
