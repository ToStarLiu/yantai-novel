# 参考仓库核验：Pagekin 与 NstAgent

核验日期：2026-11-12（Asia/Shanghai）。范围：通过 GitHub API 固定提交、检查目录和少量源码、读取许可证与运行说明。没有克隆完整仓库、安装其依赖、运行其测试、启动产品或调用模型。本记录证明所列文件及实现结构可供复查，不能证明其产品体验、模型质量或故障恢复已经在本机成立。

本项目目前只借鉴设计思想，没有从这两个仓库复制实现代码、Prompt、测试或资产。将来采用具体文件时，应另行登记来源、提交和该文件的许可证要求。

## 1. 固定版本

| 项目 | 实际仓库与默认分支 | 本次固定提交 | 提交者时间（UTC） | 许可证文件 |
|---|---|---|---|---|
| Pagekin | [EthanYoQ/Pagekin](https://github.com/EthanYoQ/Pagekin)，`master` | [`5494761ad6e96abc0c06366b50662b1de50249f1`](https://github.com/EthanYoQ/Pagekin/commit/5494761ad6e96abc0c06366b50662b1de50249f1) | `2026-11-11T19:06:00Z` | [该提交的 LICENSE](https://github.com/EthanYoQ/Pagekin/blob/5494761ad6e96abc0c06366b50662b1de50249f1/LICENSE)：GNU GPL Version 3 全文 |
| NstAgent | [zhennan1/NstAgent](https://github.com/zhennan1/NstAgent)，`main` | [`3785cef454cca879375b9e24a841709d7f91cd12`](https://github.com/zhennan1/NstAgent/commit/3785cef454cca879375b9e24a841709d7f91cd12) | `2026-10-20T07:19:35Z` | [该提交的 LICENSE](https://github.com/zhennan1/NstAgent/blob/3785cef454cca879375b9e24a841709d7f91cd12/LICENSE)：MIT，署名为 Zhennan Wan and Jianfei Chen |

提交通过 `/repos/{owner}/{repo}/commits/{default_branch}` 查询，路径通过 `/git/trees/{sha}?recursive=1` 核对；随后使用包含完整 SHA 的 `raw.githubusercontent.com` URL 读取文件。以上日期是 Git 提交元数据，不是本项目的运行日期或上游发布认证。许可证仅核验根文件，未完成逐文件和依赖清单审计。

## 2. Pagekin：作者确认、审稿与候选合并

### 项目自述

[README](https://github.com/EthanYoQ/Pagekin/blob/5494761ad6e96abc0c06366b50662b1de50249f1/README.md) 和仓库介绍将其描述为本地优先的 AI 小说写作桌面软件，覆盖设定、蓝图、正文、审稿和定稿，模型需自行配置。该描述属于上游自述；本次没有在桌面应用中复现完整流程。

### 实际源码观察

| 关注点 | 固定源码位置 | 本次读到的行为 | 对本项目的具体启发 |
|---|---|---|---|
| 写作阶段和来源冻结 | [`src/services/workflows/chapter-workflow.ts`](https://github.com/EthanYoQ/Pagekin/blob/5494761ad6e96abc0c06366b50662b1de50249f1/src/services/workflows/chapter-workflow.ts) | `FrozenDraftSourceIdentity` 包含草稿身份、状态与内容版本；`RefineFromReviewParams` 单独接收保存的人类确认审稿内容；存在独立审稿、依审稿修订和定稿工作流入口。 | 生成、审稿、修订、定稿各自引用明确输入版本；模型审稿文本不能直接成为作者指令。 |
| 修订前后的输入校验 | [`src/services/workflows/commands/review-revision-command.ts`](https://github.com/EthanYoQ/Pagekin/blob/5494761ad6e96abc0c06366b50662b1de50249f1/src/services/workflows/commands/review-revision-command.ts) | `executePrepared` 通过 IPC 准备来源并核对章节、内容哈希和源内容；恢复时检查 `sourceStatus` 与 `canResume`；生成后有独立 `commit-revision` 调用。 | 本项目应保存输入快照，拒绝过期结果自动覆盖；还需用自己的 PostgreSQL 事务测试验证持久化边界。 |
| 作者合并候选 | [`src/components/editor/merge-segments.ts`](https://github.com/EthanYoQ/Pagekin/blob/5494761ad6e96abc0c06366b50662b1de50249f1/src/components/editor/merge-segments.ts)、[`src/components/stage/chapter/MergeView.tsx`](https://github.com/EthanYoQ/Pagekin/blob/5494761ad6e96abc0c06366b50662b1de50249f1/src/components/stage/chapter/MergeView.tsx) | 前者按段落对齐构造差异段，选择类型包括原稿、修稿和手改；后者提供全部选择原稿/修稿以及把合并文本交给完成回调的操作。 | 阶段 01 需真正验证中文段落、重复句子和部分接受；不能用一次整稿替换冒充逐项采用。 |
| 审稿闭环 | [`src/shared/review-cycle.ts`](https://github.com/EthanYoQ/Pagekin/blob/5494761ad6e96abc0c06366b50662b1de50249f1/src/shared/review-cycle.ts) | 区分客观与文学问题，保留未核验、未知、未解决、已解决和作者豁免等状态；复查逻辑统计候选证据在合并稿中的出现情况。 | 工程证据校验与文学评价应分开；文本发生改变本身不代表问题已解决。 |
| Prompt 组装 | [`src/services/prompts/prompt-builder.ts`](https://github.com/EthanYoQ/Pagekin/blob/5494761ad6e96abc0c06366b50662b1de50249f1/src/services/prompts/prompt-builder.ts) | 存在分别面向章节和审稿的 Builder，填充变量、追加必需上下文并裁剪空可选段落。章节 Builder 还提供未来蓝图等输入。 | 保留 Prompt 版本与确切输入，但未来计划须带清楚的可见性约束；不能直接把全部可用材料混入角色知识。 |

运行说明核对：[package.json](https://github.com/EthanYoQ/Pagekin/blob/5494761ad6e96abc0c06366b50662b1de50249f1/package.json) 的项目名仍是 `ai-novel-writer`，版本为 `1.4.0`，包含 Electron 安装钩子、Vite、TypeScript 和 Vitest 脚本。它不是本项目 Go/Web 骨架的现成依赖。为保持本阶段验证规模，本次未安装整套桌面依赖，也未执行其中的安装钩子。

未核验：真实模型审稿准确率、中文 Diff 的实际交互质量、IPC 后端事务实现、崩溃窗口、备份恢复、跨平台安装包和第三方资产许可证。仅凭这些前端/服务文件，不对上述能力作通过判断。

## 3. NstAgent：叙事状态引导生成与批量更新

### 项目自述

[README](https://github.com/zhennan1/NstAgent/blob/3785cef454cca879375b9e24a841709d7f91cd12/README.md) 描述的是无需训练的长篇生成框架：先规划冻结大纲，再按章生成；写作可查询旧章，随后更新角色、过去事件与未来要求。README 提供生成、基线比较和评估命令，并链接上游结果。论文或上游结果不替代本项目三章样例的独立实验。

### 实际源码观察

| 关注点 | 固定源码位置 | 本次读到的行为 | 对本项目的具体启发 |
|---|---|---|---|
| 公共入口冻结协议 | [`code/agent/nstagent.py`](https://github.com/zhennan1/NstAgent/blob/3785cef454cca879375b9e24a841709d7f91cd12/code/agent/nstagent.py) | 将状态更新模式固定为 `narrative_ops`，协议版本为 `narrative_ops_prompt_v1`，细节指导放在章级 Prompt；校验入口参数。 | 对实验冻结协议、Prompt 和配置，避免一次实验中途改变语义。 |
| 章级上下文与工具循环 | [`code/agent/agent_core.py`](https://github.com/zhennan1/NstAgent/blob/3785cef454cca879375b9e24a841709d7f91cd12/code/agent/agent_core.py)，`chapter_prompt`、`_run_chapter_loop`、`generate_story` | 构造包含大纲和当前状态的 Prompt，工具循环负责读、查、修正、写入和更新状态；完整故事按章推进。不同状态模式共享该核心，阅读时必须区分公共入口固定的模式与其他实验选项。 | 本项目先固定单章链路，之后独立比较基础上下文、确认状态和检索的收益。 |
| 状态容器 | 同文件 `StoryState`（约第 2038 行） | 状态分为 `character_states`、`past_events`、`future_requirements` 三个集合，并有各自递增 ID 和序列化/恢复方法。 | 可借鉴紧凑状态投影；本项目还必须独立保存来源版本、人物知识、错误信念、叙事位置与故事时间。未来要求不作为已发生事实。 |
| 原子提议应用 | 同文件 `_tool_narrative_ops_update`（约第 4486 行）与 `_tool_update`（约第 4886 行） | 公共模式接收四个数组：角色状态更新、已发生事件、新增未来要求、解决未来要求。先复制状态，在副本上校验操作，再决定是否写回；通用批量入口也有整批校验机制。 | 先校验再提交值得借鉴，但这是内存状态操作，不等价于本项目数据库中的事务、版本冲突控制和作者审核。 |
| 检查点边界 | 同文件 `save_checkpoint`（约第 1234 行）、`load_checkpoint` 与 `generate_story` | 检查点通过临时文件及 `os.replace` 保存，读取时核对指纹；保存异常会被记录为警告；故事继续机制围绕已完成章节。 | 本项目模型调用前需持久化 attempt，结果未知需独立处置，不能仅凭章后文件检查点承诺外部调用不重复计费。 |
| 模型调用与用量 | [`code/agent/generation_common.py`](https://github.com/zhennan1/NstAgent/blob/3785cef454cca879375b9e24a841709d7f91cd12/code/agent/generation_common.py)，`ChatResult`、`chat_completion` | 共享模型调用、响应提取和长度控制辅助逻辑，与状态/故事编排分离。 | Provider 契约保持独立，模型原始响应、用量与运行记录由本项目显式保存。 |

运行前提核对：[requirements.txt](https://github.com/zhennan1/NstAgent/blob/3785cef454cca879375b9e24a841709d7f91cd12/requirements.txt) 标注 Python 3.11 并锁定论文实验用依赖，包括 `openai==2.29.0`、`httpx==0.28.1` 等；README 命令需要模型配置和生成预算。本次未建立该 Python 环境、未下载完整数据集、未运行生成命令，避免把参考阅读扩展为另一套推理系统。

未核验：本机复现实验分数、模型断线后的计费行为、独立人物知识隔离、来源引用精度，以及本项目要求的作者逐项记忆确认流程。所读实现以模型工具循环为中心，不能直接认定满足本项目的人工审核和旧稿重建契约。

## 4. 本阶段采用的结论

1. Pagekin 优先参考作者确认、来源冻结、差异合并和可追踪审稿；阶段 01 用本项目数据模型重建这些行为。
2. NstAgent 优先参考状态投影、章级 Prompt 与先校验再应用；阶段 02 补上有来源的 ChangeSet 和人工确认。
3. 两个项目的实现均不能替代阶段 00 的独立质量证据。当前没有把它们加入 `go.mod` 或运行依赖，也未将上游能力标为本项目已完成。
4. 后续若确需复用代码，先对选定文件和依赖完成许可证记录，再评估适配成本；本次核验不构成整仓采用审计。
