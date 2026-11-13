# 开源项目优势与当前实现对照

更新：2026-11-12。依据根目录《AI辅助小说创作程序_开源项目调研与技术实施方案_v2_Go架构版》第4–9、13–27节。本表逐项核对产品优势，区分可运行的功能、依赖外部服务的连接能力和仍需作者评价的文学效果。参考不等于复制技术栈或业务代码；Pagekin/NstAgent固定提交核验见 [repositories.md](../../evaluation/repositories.md)。

上一轮矩阵中的“Canvas、世界模拟、工作流、向量、出版、协作未实现”已被本次实际实现取代。接口和存储都有业务校验，不以按钮或通用备注框代替功能。测试使用隔离PostgreSQL及受控Provider；接口契约通过不等于外部模型的真实文学或媒体质量已通过。

## 逐项目映射

| 来源 | 调研中的优势 | 当前实现、代码与验证 | 采用边界 |
|---|---|---|---|
| Pagekin | 设定→章纲→起草→审稿→定向修订→Diff→确认→定稿；不可覆盖正文；历史恢复；可控任务 | `core.go/runs.go/worker.go`，`Editor.tsx/Assistant.tsx/revisions.ts`；AI候选独立保存，接受后新建不可变Revision，恢复也创建新版本；TXT/MD先分章预览。`core_test.go/server_test.go`验证中文来源、409、恢复与导出。 | Production最多50章、预算和人工停止点；参考结构分析独立存储。作者决定采用候选。 |
| OpenFic | Agent/Prompt/Workflow、兼容模型、Embedding/Rerank、多层上下文、压缩、缓存、多端 | `integrations.go`持久Provider/Prompt/Agent；`automation.go`版本化有类型DAG；`semantic.go`关键词/向量/混合/Rerank及来源摘要；`runs.go`冻结输入、摘要采用与缓存；Pi负责受控对话循环；`desktop/`桌面壳和窄屏Web。 | Chat/Responses/Ollama真实协议；页面配置加密凭据、模型发现和默认路由，环境变量继续兼容。向量/Rerank需配置端点，未配默认关键词。DAG受控条件，拒绝任意代码和无限循环；发行验证另记。 |
| Biz Novel Studio | 长链路持久化、制品、暂停/取消/恢复、并发、幂等、追踪 | `wb_runs/attempts/events`、`wb_productions/production_steps`，冻结流程/章节/预算/条件/人工门；断线不取消后台。`runtime_test.go/automation_test.go/worker_stream_test.go`验证租约及未知调用边界。 | 单库事务直接提交记忆与索引，吸收Outbox原子性优势；持久Run Trace与metrics用于排错，未声称已部署OpenTelemetry分布式集群。 |
| OpenWrite | 三栏工作台、Story Codex、Canvas、人物弧、时间线、关系、评论协作、多格式出版 | `studio.go`节点/因果边/坐标/条件/结局/Scene/人物弧；`analysis.go`确认事实图谱；`collaboration.go`账号/角色/presence/SSE；`publication.go`TXT/MD/DOCX/EPUB/PDF；`Studio.tsx`。 | boardVersion/Revision乐观锁；viewer/reviewer/editor/owner权限与跨项目归属校验。共享编辑有409和冲突缓冲，不声称CRDT合字。 |
| Novel Engine | Story Coach、Ghostwriter、First Reader、发展编辑、Task Master、文字编辑、Publisher | `planning/draft/reader/developmental/review/revision/voice/publisher`等真实任务；三层Brief承载主题/欲望/卖点/代价；Issue可选入修订；`studio_features.go`显式采用voice为style设定。 | 作者声音、读者反馈、出版材料先是候选；发展编辑/hook报告不可作为AI正文来源。声称“学会声音”还需真实人评。 |
| iLearn-Lab/NovelClaw | Writing Session、Storyboard、可编辑记忆、Run Trace、快照、恢复 | 作品/章节会话范围，持久board/scenes；`narrative.go`接受/拒绝/编辑；冻结任务/原始响应/Usage；Studio进入完整归档与分支ID重映。`studio_test.go/narrative_archive.go`。 | 人物弧和遗忘是可重建投影，不删除权威来源；页面生命周期与后台任务分离。 |
| NstAgent | 当前状态指导下章、章后抽取、状态修正、版本化规划 | 五类变化及角色知识；Proposal→作者决策→原子State/Fact/Chunk；正文/章纲/epoch/State/Studio版本全链检查；影响报告、20章顺序重建。 | 情绪/目标/信念重要性/recency可持久编辑；未发生计划与确认事实分开，不默认接受抽取。 |
| NovelGenerator | Scene Handoff、因果链、连续性、前因检验、ObservedBy/禁止变化 | Scene的goal/conflict/causes/observedBy/forbiddenChanges及结构化handoff；cause/requires/prerequisiteIds有DAG校验；Scene进入实际任务快照。`studio_test.go`拒绝循环/跨作品/悬空引用。 | 交接候选需作者采用；计划不能冒充已发生记忆。 |
| Long-Story Agent / CoLong | 规划→检索→生成→检查→修订→写回；具体证据 | 有类型任务及Production；Issue精确引用、选中修订、事务记忆应用；真正Provider SSE增量先持久后推送；`stream_test.go/worker_stream_test.go`故障注入。 | 流中断保留未知状态；证据审稿和严重度排序不等于文学真值评分。 |
| AIStoryWriter | 本地兼容、CLI批处理、阶段模型、Prompt、评价、多语言 | `cmd/eval/cmd/retrieval-eval`；Chat/Responses/Ollama、任务多Provider路由；Translation候选；最多50章Production；temperature/topP冻结。 | 本地模型和翻译质量未用协议夹具冒充实际效果；按任务模型与成本已接入真实生成。 |
| LibriScribe | 统一Client、外置Prompt、费用、Schema、自修复、MCP、混合检索/评估 | Prompt版本/规则，受控JSON格式修复后重验闭合Schema/证据；MCP发现/调用/许可/审计；Embedding/Hybrid/Rerank；预算、单价快照、关键词60问。 | JSON修复不补事实，原raw保留。语义接口有真实向量和排序契约测试，尚不称真实Embedding质量已超过基线。 |
| awesome-novel-agent | 中文SOP、卷章纲、任务分工、透明文件、定稿更新 | 三层Brief、book/volume/chapter规划、卷目标/summary、章纲、角色/规则；作者确认；完整JSON/Markdown；版本Agent/Prompt及工具许可。 | 数据库权威+透明导出替代文件双写；不执行下载Skill的任意脚本。书/卷采用严格校验scope/volume和生成基线。 |
| oh-story-claudecode | 扫榜选材、拆书、伏笔、钩子、文风、封面、Skill组织 | `research.go`1–5公开URL真实GET，保存标题/段落/链接/摘录/时间/逐项失败；资料结构分析/重叠证据；Brief卷章规划、Foreshadow、hook报告、voice Profile、封面任务、Agent/Prompt/MCP。 | 无搜索Key清楚显示URL模式；配置搜索HTTP端点才查询。参考从不混入正文Search。文风建议不以词表定罪；图像服务质量另验。 |
| chinese-novelist-skill | 三层需求、受众/冲突/代价/结局、悬念、10–50章、校验 | `StoryBrief`结构字段与中文向导；卷/章规划、Production范围与maxChapters；Lint+证据审稿+hook专项报告。 | 生成读取版本化结构；保留采用/定稿/记忆门，不为无人值守绕过作者。 |
| 7a-z/NovelClaw Skill | 场景策略、WritingProfile、套话/自检 | `StyleTemplates()`对话/动作/调查/过渡/高潮可编辑；voice候选显式采用；`DeterministicReview`精确重复、排版及明确作者规则；文风任务。 | 机械校验只报告确定项；文学情绪/悬疑效果留给候选与作者评价。 |
| CharacterMemory | 真相/获知/推测/误信/秘密分离、重要性/情绪/recency/遗忘 | Knowledge含characterId/truthStatus/章节/证据，角色只读显式获知；人物弧emotion/goal/importance/recency/forgetAfter；Wiki遗忘/衰减投影。 | 不删Fact、不改作者误会；不称人类记忆模拟。角色/未来/旧版隔离有硬测试。 |
| Kandor | 生效区间、证据、关系随时间变化 | Canvas storyTime/validFrom/validTo及时间投影；确认Fact storyTime/chapterOrder；StoryMap关系派生。 | 支持显式区间和未知时间，不把布局/关联当旅行时间推理或因果证明。 |
| LLM Wiki思路 | 原始来源→知识编译→Lint/重建/过期/支持反对 | Revision/设定权威，Proposal确认后编译State/Fact/Chunk；Wiki实体聚合支持/反对、误信与来源；换版事务失效，归档证据链。 | 支持/反对来自已确认事实，不自动添加“客观答案”；引用可回到原文。 |
| Re3 | 总体计划、多候选、重排、一致性编辑 | `candidates.go`2–3独立候选+各自review+严重度重排解释；规划/检索/修订/Diff；独立意图与Usage。`candidates_test.go`。 | 候选费/审稿费透明；排序辅助作者选择，不宣称保证文学质量。 |
| DOC | 分层详细规划、长文控制、候选评价 | book/volume/chapter范围、卷目标/结局、Scene/handoff、作者采用规划、2–3候选review。 | 全书/卷计划不进入确认事实；效果留给人评，实际领域闭环已实现。 |
| E2EPlot | 规划生成联动、事实修订、结构评价 | Canvas因果/条件/结局、选节点生成Scene、analysis/developmental结构候选、定向修订、事实影响报告。 | 确定前置与文学结构建议分层，不用关联连线证明事实因果。 |
| Agents’ Room | 明确输入/输出/权限/上下文/失败/预算 | PromptTemplate/AgentDefinition/tools/MaxOutputTokens；DAG依赖/条件/人工门/预算；冻结MCP结果。 | Agent不能自行开权限；配置管理员专用；失败状态持久保留。 |
| StoryBox | 群像/势力、资源互动、候选事件Sandbox | 选定实体+确认初值；参数化回合改变资源/张力，产生代价/谈判/休整等事件及before/after/diff；确认/冻结/检查点回滚。 | 有边界确定性沙盒，不是无界自治；结果只写sandbox。`TestStudioSandboxPreviewConfirmRollbackNeverWritesCanonical`。 |
| Patchview | 关系画布、拖动纠正、距离、选节点生成、时间滑块 | React Flow持久坐标/边、时间投影；`studio_features.go`选节点→实际Prompt→场景候选→显式采用；关系解释候选。 | 几何距离是作者布局，不伪称语义相似真值；改board使旧候选失效，AI不暗改事实。 |
| NarrativeNode | 剧情节点/依赖/前置/条件/结局、分支、MCP | Plot Graph、deterministic evaluate；fork完整作品与ID重映；MCP工具发现/许可/调用/审计和独立只读服务。 | 条件依赖受控求值，expression为描述而非eval；分支不递归复制；外部MCP独立Bearer，未配置禁用。 |
| KoboldAI | 本地续写、采样、作者注释、动态世界 | Ollama/兼容本地Provider；temperature/topP；指导、确认状态/相关检索/选段重写；动态上下文/来源摘要。 | 不替换Go后端为模型运行器；作者配置模型，原生协议契约已测试。 |
| jacobbeasley/novelai | 大纲→章节→场景、Word、TTS | Brief→卷/章→Scene→正文；DOCX/EPUB/PDF/TXT/MD；TTS持久任务/音频内容/取消。 | speech服务需配置，单次4000字需分段；中断未知不自动重复计费。 |
| Smart Pen / AI Novel Assistant | 富文本、角色/世界、PostgreSQL、职责清楚 | Tiptap JSON服务端派生text/hash；Go领域+同源API/React；迁移、真实PG、Origin/CSRF/session、项目角色。 | 用现技术体系吸收相同产品价值，不重复引入Vue/Supabase。 |
| NOVA | World Bible、矛盾、时间线、关系、版本、多Provider | 设定/Brief/Wiki、确认事实、证据Issue、时序/关系Canvas、不可变Revision、多Provider路由。 | 作者设定与已发生事实区别保留；矛盾由作者决策，不复述上游路线图当效果证明。 |

## 验证入口

- `core_test.go/export_test.go`：正文来源、409、新版本恢复、软删除、原子包导入/导出；AI采用检查Revision/章纲/epoch/State/Studio/Canvas基线。
- `studio_test.go`：Canvas竞争/循环、Scene/Observer归属、人物弧、隔离资料、Wiki、沙盒、分支重映、时间与历史恢复、Prompt版本。
- `studio_features_test.go`：节点真正进模型Messages；Scene采用不改正文；改画布拒旧候选；voice显式采用与过期拒绝；hook报告不能冒充正文。
- `collaboration_test.go`及`internal/server/collaboration_test.go`：PBKDF2、成员隔离、最后owner、并发邀请单次成功、撤销现session失权、角色边界、真实SSE、事务回滚不通知、导入归属。
- `research_test.go`：URL/private/重定向/大小/MIME、部分失败保存、搜索契约、书卷采用、重叠来源。受控HTTP不冒充已抓取实时榜单。
- `automation_test.go/integrations_test.go/mcp_test.go`：DAG/Production/人工门/恢复/预算、Provider/缓存/Prompt/Agent/MCP权限审计。
- `semantic_test.go`：实际向量/混合/Rerank、坏维度/索引/排序/越界过滤、摘要引用接入；真实服务质量不以夹具成绩替代。
- `candidates_test.go/publication_test.go/media_test.go`：候选生产/审稿重排、出版文件结构、封面/TTS内容/取消/中断。
- `runtime_test.go/worker_stream_test.go`及`internal/llm/*_test.go`：租约/未知调用/重试/取消、真流式、持久重放、协议、格式修复。
- [60问基线](../../evaluation/retrieval-baseline.md)：固定40开发/20保留、20章、独立qrels；关键词Recall@10开发91.43%、保留93.33%；10项未知/不可见全空，跨作品/旧Revision/未来/角色泄漏0；4项纯同义漏召回保留。这不是向量成绩。
- [备份恢复](../../deploy/backup-restore.md)：两轮真实pg_dump/restore到新隔离库；第二轮含五类事实与Revision/hash/quote回查；恢复默认停Worker，未知Running不重发。
- [作品包审计](project-archive.md)：Studio全部历史/沙盒检查点/Research来源完整往返；Run/Production/Candidate/媒体元数据保留只读activity档案，不恢复可计费队列；媒体binary另行下载或全库备份。

上列Go文件位于`internal/workbench/`，另有明确路径者除外。协作及高级功能的定向PG测试已通过；最终全项目测试、浏览器、桌面打包、部署以 [acceptance.md](acceptance.md) 的实际记录为准，不把源码存在当发行已验证。

## 合并优势时的选择

1. 正文与记忆最终由作者决定。抽取、结构/声音/关系分析、世界模拟和出版文案先产候选，各领域显式确认后才写入；沙盒确认仍只写沙盒。
2. PostgreSQL唯一权威，完整JSON/Markdown/出版导出提供透明性和迁移，替代文件双写；单库记忆/索引用同一事务实现原子行为，不引入重复Outbox队列。
3. 工作流采用有类型/依赖/预算/人工门DAG，不执行任意Shell或无限自主循环；MCP有独立工具许可和来源快照。
4. 关键词保留可测基线，Embedding/Hybrid/Rerank是真实可配接口，需配置服务并做同预算对照，不能用手造向量声称质量更高。
5. 世界模拟有实体/参数/资源/张力/回合/事件/检查点/diff，提供可审阅探索，不宣称无界自治或人类行为模型。
6. 多人共享采用持久角色/presence/SSE/本地冲突缓冲/409；未实现CRDT字符合并，不将共享编辑描述为无冲突同时合字。
7. 桌面壳/同源Web/窄屏复用服务和会话；未发原生iOS/Android商店包，发行验证独立登记。
8. 持久attempt/原始响应/Usage/事件/metrics提供追踪；没有安装Temporal、OpenTelemetry Collector/Grafana集群。这些基础设施名称不是必须复制的产品功能。

## 当前交付边界

这是从第一版闭环继续扩展后的系统：写作、版本、记忆、检索、工作流、Canvas/分支/Scene、资料研究/Wiki/沙盒、协作、出版媒体与跨端连接均有持久接口和业务实现，高级模块已不再作为“以后再做”的清单保留。

验证范围仍须诚实：外部Embedding/Rerank/媒体服务效果、原生移动商店发行、任意代码无限工作流、CRDT合字和文学质量保证没有被伪装为完成。按用户要求，作者采用率、修改时间、长篇声音和文学评价后置，不作为继续实现的许可门槛。API契约见 [openapi.yaml](../../api/openapi.yaml)。
