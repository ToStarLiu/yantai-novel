# 完整系统验收记录

> 本文保留开发阶段的本机验收与历史交付记录，文中的本地路径/运行状态不是新安装的默认配置。公开仓库安装请以[README](../../README.md)为准，原始运行追踪和私有数据库不随仓库分发。

日期：2026-11-12（Asia/Shanghai）。交付对象：砚台小说创作系统，Go/React/PostgreSQL、Web/PWA/Windows桌面。原首版边界已按用户指示扩展到完整系统；作者修改时间、文风采用率与真实创作试用仍后置。本记录区分实际实现、实际验证与供应商配置依赖。

## 可运行入口

- 主服务：http://127.0.0.1:8088，使用用户配置的兼容Responses模型，密钥只在本地环境。
- 工作区管理员：`.local/author-credentials.txt`，文件被Git忽略。协作者通过一次性邀请注册；无通用默认密码。
- Windows便携包：`desktop/dist/Yantai-win32-x64/Yantai.exe`，自带PostgreSQL/API/Web/中文字体；数据和随机账户在独立用户目录，不携带本工作区密钥与作品。
- 原创示例：断桥铜印 · 示例作品，三章正文、人物/世界/文风设定；第一章已定稿，真实模型生成的记忆提议保留待作者审核，不自动提交。
- 重启：`pwsh -File scripts/start.ps1`。工具安装、前端构建、显式迁移和启动路径均已提供。
- API契约：[OpenAPI](../../api/openapi.yaml)；使用说明：[README](../../README.md)。

## 功能与验证证据

| 能力 | 实现 | 实际验证 |
|---|---|---|
| 单作者访问控制 | Cookie会话、哈希令牌、Origin/CSRF、登录限速、注销、过期；正文/任务/SSE/导出受保护 | 真实PostgreSQL httptest，未登录和跨来源访问拒绝，令牌仅hash入库 |
| 作品与设定 | 多作品隔离、卷章、人物/世界/规则/总纲/文风、五类场景模板 | HTTP与浏览器创建/编辑、删除确认；外作品ID拒绝 |
| 编辑与版本 | Tiptap中文富文本、服务端派生纯文本/hash、不可变Revision、自动保存、409冲突、失败缓冲、历史恢复新版本 | 浏览器中文输入、刷新、保存故障恢复、两个页面并发、旧版保持、历史恢复 |
| AI创作 | 起草、结构化审稿、选中问题修订、规划建议、唯一选段扩写/压缩/润色、场景交接 | 离线Browser E2E；真实主服务Responses流式审稿成功；规划/交接不会冒充正文 |
| 作者采用 | 独立多候选、段落Diff逐项采用、run来源/epoch/正文/章纲/状态基线服务端核验 | 前端Diff单测；浏览器旧候选禁用；直接绕过前端仍返回409 |
| 记忆审核 | Location/Injury/Ownership/Knowledge/Foreshadow；逐字证据；角色误信分开；逐项编辑/接受/拒绝；原子幂等应用 | 五类状态数据库测试；非空浏览器提议编辑/拒绝/提交；真实模型三条有效提议保留待审核 |
| 改稿影响 | 定稿换版、删除、重排与失效同事务；后文不改写；从有效基线逐章重建 | 20章数据库场景、两类事实变更/重排与恢复；浏览器改旧章后显示影响与重建提示 |
| 检索 | 中文二字词、短语/实体、作品/活动版本/叙事位置/人物知识过滤；来源正文可看 | 60问固定40开发/20保留，保留Recall@10=14/15=93.33%；10无答案均空；跨边界违规0 |
| 上下文透明 | 固定模型、Prompt、章纲/正文/状态版本、设定、允许前文与历史引用 | 输入未来事实隔离测试；浏览器可展开运行上下文；答案文件不进模型 |
| 故事视图与本地检查 | 由确认事实派生时间线/关系图；重复段句/显式规则/伏笔到期等可解释提示 | 图与Lint数据库测试，引用来自正文；浏览器时间线/来源高亮、本地检查不调用模型 |
| 流式与恢复 | 持久attempt先于调用、真实Provider增量→数据库→SSE、LastEventID续读、取消/暂停/恢复、租约代次隔离 | 本地SSE契约与真实数据库增量测试；浏览器断开不取消；真实主服务产生实际delta并保存完整结果 |
| 未知结果与副作用 | 断流/超时/响应保存失败进入NeedsReconciliation；不自动重发；提议与结果/终态同事务 | 故障注入、过期Worker、竞争租约、响应保存故障、原子提议、取消输出保留测试 |
| 成本与预算 | 每次attempt独立用量和价格快照，未知不装0，多币种分组，作品锁内并发预留，输出上限，任务模型路由 | 8并发预算仅1接受；unknown→known不抹账；重试累计用量；价格非数值拒绝 |
| 作品迁移 | JSON完整包、正文/版本/状态/事实/证据ID重映射；TXT/MD预览；Markdown/TXT可读导出 | 数据库非空归档往返、篡改/循环/来源跨作品拒绝；浏览器预览导入、下载与恢复新作品 |
| 部署与备份 | 便携Windows服务、Dockerfile/Compose API+Worker+Postgres+迁移；pg_dump及只恢复新库脚本 | 本机启动/重启通过；两轮隔离数据库恢复，第二轮含5类事实，正文hash及证据一致；Docker未实跑 |

## 执行命令

高级系统新增能力与证据：

| 能力 | 实现与持久化 | 验证 |
|---|---|---|
| Story Studio | Canvas拖拽/连线/因果前置/条件结局/时间过滤；三层brief/卷计划、scenes/handoff、人物弧 | PG版本/DAG/跨项目/时间测试；Chrome实际拖拽与保存 |
| 世界与资料 | 带来源Wiki、参考独立分析/搜索/重合；确定性沙盘/候选差异/确认/冻结/检查点/回滚；全作品分支与ID重映射 | PG归档往返、沙盘不写canonical；Chrome完整操作 |
| 可配置编辑部 | Prompt不可变版本、Agent typed读取权限、作者声音/读者/发展编辑/钩子/翻译/出版任务；声音显式采用为style | PG来源与旧候选拒绝；Agent权限与模板冻结测试 |
| 长流程 | DAG依赖/条件/人工闸门、Production最多50章、预算、暂停/取消/恢复；2–3候选各自起草及审稿排序 | PG稳定意图、预算竞争、暂停不漏派发、坏作品不影响其他作品；Chrome制作审批与候选排名 |
| Provider/检索 | Chat/Responses/原生Ollama、禁用/健康/路由、采样、显式缓存；HTTP Embedding/Rerank、cosine/RRF混合、来源压缩 | HTTP契约+PG维度/当前版本/知识隔离、远端期间失效拒绝；缓存命中无外呼 |
| 多用户协作 | 用户密码PBKDF2、项目角色、一次性邀请、批注、presence、持久变更SSE | HTTP角色矩阵、成员撤销立即失权、跨项目拒绝；真实两个浏览器邀请登录/只读403 |
| 研究与MCP | 公网URL资料抓取/可选搜索；JSON-RPC初始化/发现/只读调用、schema、allowlist/Agent授权、意图claim与参数hash日志 | HTTP假供应商+PG、SSRF/redirect拒绝、并发同意图一次调用；Chrome真实本机MCP工具调用 |
| 出版与媒体 | DOCX/EPUB3/中文PDF/HTML；图片封面/TTS持久队列/下载/未知结果不自动重发 | 出版XML/包/中文内容与定稿过滤；Chrome四种实际下载；媒体HTTP契约及跨项目隔离 |
| 格式恢复 | 仅JSON围栏/BOM/尾逗号修整，原attempt响应保留，Schema与逐字证据重验 | 真实PG执行成功/伪造证据拒绝测试，不增加模型调用 |
| 跨端 | PWA静态预缓存、不缓存API；Electron含API/PG/中文字体和独立数据目录 | Chrome离线shell+缓存安全；桌面新库初始化、自动登录、关闭PG、sandbox/contextIsolation验证 |

实际设置 `TEST_DATABASE_URL` 后运行：

```powershell
. ./scripts/dev.ps1 -Action env
go test ./... -count=1 -timeout=120s
go vet ./...
npm --prefix web run typecheck
npm --prefix web run test -- --run
npm --prefix web run build
npm --prefix web run test:e2e
```

后端顶层测试最终数量和前端最新结果见本文件末尾。后端测试使用随机schema；全部完成后确认无遗留 `test_*`。浏览器专用离线服务8089，不触发真实模型费用；多个Playwright同时运行曾互相清除产物并触发登录限速，已停止并改为独占顺序运行，不能把受干扰的一轮算通过。

浏览器最终覆盖10项：扩展写作工具；非空记忆审核、证据、检索和旧稿失效；保存失败缓冲；真实并发冲突；文本/作品包导入；过期AI候选保护；卷名/Markdown/TXT/删除确认；完整创作主流程；移动登录；移动登录后导航/助手。截图与详细记录见 [前端验收](frontend-verification.md)。

## 真实接口连通与发现的问题

在阶段00三轮12次真实调用之外，本次主服务进行了三次小规模连通调用：

1. 一致性审稿成功：`c99692539b8820978687e2d565b7fbc5`，实际输入5809/输出66 Token，真实流式事件和结果入库。
2. 首次记忆提取格式可解析但实体/字段语义校验失败：`bb9681bce384c8c82b6ed46c5848c4ba`，输入7193/输出2814 Token。保留失败原文和用量，没有自动收费修复或重试。
3. 明确实体ID、客观事实characterId为空、Ownership持有人ID、最多5项及短引文的提取契约后，新的意图成功：`65756b71dddfc6399f0e0c4a8cfb0b9b`，输入7340/输出793 Token，创建三条有证据的Location/Injury/Ownership提议。作者尚未确认，不能把它们算作已生效事实。

费用单价没有可核实依据，全部保持Unknown。模型提取仍可能不符合语义约束，应用拒绝无效结果并保留原始输出；这属于已处理的失败路径，不据此声称长期提取质量达标。

真实服务的细碎token增量曾产生较多事件，已优化为首段立即保存、后续按75ms或256字节批量提交。真实数据库测试中1000个单字增量仅形成13条持久事件，正常结束和断流均无丢字、重复；静默Provider取消也经过测试。

## 研究优势与架构调整

[完整对照矩阵](reference-advantages.md)覆盖调研所述全部项目/研究，区分已实现、部分实现与高级扩展。当前系统吸收了完整创作闭环、作者控制、版本、角色记忆、来源重建、透明上下文、检索、恢复、成本、场景文风及规划工具等主要产品价值；没有照搬上游代码。

实际优化记录在 [D-010至D-014](../superpowers/plans/2026-11-12-novel/decisions.md)：质量评估后置、模块化单体聚合、记忆和索引事务化、统一任务输入校验、真实流式持久化、逐次调用账及并发预算。

## 交付边界

原先列为高级扩展的画布、沙盘、可配置工作流/Agent、Embedding/Rerank、Provider管理、批量制作、桌面/PWA、出版、协作、封面/TTS均已有实现和实际入口。本项目按研究优势作可验证的工程落地，不照搬全部技术框架或提供无限任意代码执行；确定性沙盘、共享版本协作、只读MCP的具体行为见矩阵。

文学质量、作者修改时间、A/B/C长期效果比较按用户指示后置；容器构建/空Docker机器部署尚未实跑。本历史阶段当时未公开发布，根许可证尚未选择；当前源码授权以根目录LICENSE及README为准。具体限制见 [known-limitations](known-limitations.md)。

## 最终验证与本机交付状态

- 高级扩展前的后端基线：87项顶层测试通过（包含真实PostgreSQL集成测试），go vet通过，API/Worker二进制构建通过；高级系统最终结果在下文登记。
- 前端：TypeScript、生产构建、3项单测通过；10项真实Chrome E2E整套通过43.4秒，最后实体名称显示优化单独回归受影响项通过。
- OpenAPI：44路径、内部引用全部可解析；手工记忆提案端点已同步契约。
- 已将68部本次浏览器测试项目先完整备份、逐部JSON归档，再清理；书房仅保留原创示例。可恢复归档位于.local/e2e-archive和.local/backups/final-before-test-cleanup，不涉及用户其他作品。
- 最新服务保持本机8088运行；真实模型记忆提议保持Pending，等待作者在系统内决定。临时离线8089验收服务将在交付时停止，避免混淆。
- 未执行公开发布、推送、用户服务器部署或模型质量正式试用。

## 完整系统最终验收

2026-11-12高级系统集成后的执行结果：

- 后端全量144项顶层测试通过，0失败、0跳过，真实PostgreSQL测试没有略过；`go vet ./...`通过。其后发现并以migration16修复Production章节外键导致作品删除失败，新增真实PG删除回归通过，另一作品和全局工作流保留；再次vet和构建通过。
- 前端TypeScript、3项单测、生产构建通过。17项Chrome E2E整套通过（单执行器1.4分钟），随后新增专项及受影响4项通过，共18个不同用例。实际覆盖新增规划采用/历史、跨浏览器邀请权限、真实MCP、画布场景、声音采用、钩子、Provider价格、出版与候选。
- OpenAPI为118路径、146操作、133个Schema，所有内部引用可解析；web和desktop依赖audit均0已知告警。候选源码对本地敏感配置值扫描0命中。
- 桌面真实`Yantai.exe`启动确认`app.isPackaged=true`：首次初始化独立PG18数据库，API健康200、自动登录并显示书房；新建作品200，退出数据库关闭。`contextIsolation=true`、`nodeIntegration=false`、`sandbox=true`。
- 最后桌面升级复测发现旧PWA Service Worker使新版原生客户端导航失败；原生客户端启动前仅清除旧静态缓存/Service Worker，保留作者缓冲和cookie，并先建立会话再单次加载。最终包在同一旧数据目录连续重启三次通过，数据保留、退出关闭数据库；浏览器PWA缓存逻辑保持启用。
- DOCX通过本机Word16只读渲染为5页并逐页检查，文字提取与OOXML一致；独立PDF4页逐页检查中文、标点、缩进与页码。EPUB/HTML包结构和中文内容检查、浏览器下载字节验收通过。详见[DOCX渲染报告](publication-docx-qa.md)。
- schema15完整实例备份恢复到新库，38个非会话/在线状态业务表逐表计数和正文hash匹配。随后schema16最终备份恢复到`novel_restore_release_final`，`sourceMetadataMatches=true`；恢复脚本隔离正文任务、Production、媒体、候选、MCP输入，清除会话和presence并撤销未用邀请，不触发外部副作用。
- 高级浏览器产生的53部已明确识别测试作品全部先JSON归档，再清理；另有清理前全实例dump。归档位于`.local/e2e-archive-advanced`与`.local/backups/advanced-before-cleanup`，保留只读活动档案及历史。测试全局配置和4个测试用户也有dump可恢复；书房保留原创示例及5条确认事实。
- 主服务保持8088运行，使用原有用户配置的真实模型；临时8089已停止。Windows便携包和ZIP位于`desktop/dist`，不含私密配置和工作区数据库。

单作品迁移保留正文/叙事来源/Studio历史/研究与只读活动；实例配置、原始attempt及媒体binary由全库备份保留。见[迁移范围](project-archive.md)。文学质量、修改时间与长期采用率继续按用户指示后置；供应商未配置的媒体、搜索或Embedding不会以演示数据伪装成功。

## 0.3.0 作者书房与 Pi Copilot — 2026-11-13

本节为当前交付状态，前文保留各阶段历史验收。默认入口现为书房，经新书向导、作品概览与全书规划，再进入章节写作；作者可以在零章节时讨论全书，确认候选后再安排人物世界和卷章。顶栏的供应商配置不依赖选中作品，支持地址、协议、密钥、模型发现、默认、启停和不可变编辑版本。

Pi 1.5.2 Agent Core/AI是实际执行依赖。Go逐次授权模型请求及预算，记录attempt/真实用量、检查来源版本、成员权限与作者采用；私有会话、持久事件、SSE游标重放、取消、历史清理和压缩均已实现。全书候选仅在显式采用时更新plotPlan，正文和已确认事实保持作者控制。

实际验证证据：

- 后端最终完整185项顶层测试通过，0失败、0跳过，含真实PostgreSQL与空安装自动配置接管；日志为`.local/release-030-final-tests.jsonl`（workbench130.934秒）。末尾上下文限额修复的全套Copilot回归也通过（workbench32.097秒/server4.622秒）。`go vet ./...`及API/Worker构建通过。
- Node源码与独立发行bundle各17项测试通过：真实Pi循环、本机Responses/Chat工具往返与用量、禁止自动重试、无终态断流记未知、免密不发Authorization、book零章工具链、版本重建、长对话及单条长请求压缩、完整call/result配对、清除重复UI制品、原文游标无丢段以及大量长Diff的有界预览。
- 前端5项单测、TypeScript与生产构建通过；23个不同Chrome E2E用例分批全部通过。包含作者全旅程、网页供应商、选段/Diff、刷新恢复/停止/压缩、移动布局、保存失败、并发409、记忆审核、画布、协作真实403、MCP、出版及PWA。详见[前端记录](frontend-verification.md)。
- 使用用户配置的真实Responses接口完成一次受控Pi工具链：4个实际attempt，输入23138/输出277 Token，供应商报告用量；read_chapter→create_revision_candidate→show_diff成功，源正文版本不变。原始连通报告保存在`.local/copilot-real-smoke.json`并由清理前全库dump保留。费用单价未知保持Unknown，没有将其记为免费；文学质量按要求后置。
- 页面凭据AES-GCM密文、write-only读取、同地址留空复用/跨地址重填、不可变编辑旧任务保持、superseded不再参与新路由、免密及管理员权限由PG/本机HTTP测试验证。schema20独立备份恢复保存两代凭据，主密钥sidecar与恢复文件SHA匹配，两次本地HTTP验证原密钥和敏感header，外部模型调用0次。详见[备份恢复](../../deploy/backup-restore.md)。
- 清理前全实例备份含51作品、17私有Copilot会话、36轮对话、92次attempt、6个全书方案及供应商密钥sidecar；位于`.local/backups/copilot-before-cleanup`。50部明确识别的测试作品先逐部JSON归档到`.local/e2e-archive-copilot`后清理，测试全局配置也先留dump。该非空完整备份实际恢复到新库`novel_restore_copilot_history`，17会话/36轮次/92attempt/6方案及真实连通成功记录完整保留，44表计数与正文hash一致、无自动重发。
- 最终schema20实例恢复至新库`novel_restore_copilot_final`，44张业务表计数和正文摘要一致，sourceMetadataMatches=true；保留1部原创示例、3章、8个正文版本、5条确认事实。恢复脚本隔离待执行/未知Copilot调用及其余执行队列，不触发真实接口。当前无遗留test_* schema。
- OpenAPI为131路径、161操作、149个Schema，879个内部引用全部可解析。Web、desktop、harness依赖audit均0已知漏洞；候选源码及许可材料416个文件对本机敏感配置值扫描0命中。Pi实际bundle的6个组件许可及Node发行许可随桌面和容器运行目录保留。

发行桌面和主服务的最终构建、启动及校验记录见下方交付回执。容器构建尚未实跑，文学质量、作者修改计时和长期对照不列为工程验收通过。

### 最终运行与交付回执

2026-11-13最后构建的API、Web入口和Pi bundle，分别与桌面包内对应文件SHA-256完全一致。Windows便携包为0.3.0，数据库schema20；Node24.22.3及Pi1.5.2、89个Go/Web运行组件通知与6个Pi bundle组件通知完整保留。原生EXE实际启动后，零章节全书规划通过包内Pi生成候选，作者明确采用成功；退出PG关闭，重启保留作品、采用状态及私有对话，第二次退出PG仍正常关闭。Electron的contextIsolation/sandbox均为true，nodeIntegration=false；回执`.local/desktop-copilot-final.json`。

主服务当前运行于127.0.0.1:8088，使用用户原有gpt-6.1-sol配置。Chrome实际登录后验证默认书房、唯一原创示例、书籍规划及AI供应商设置可达，密码框为空，无pageerror；没有发送新生成请求，回执`.local/main-delivery-smoke.json`。临时8089测试服务已停止。

最终大结果边界使用真实Node估算器测量，包含最大启动快照、系统规则、工具Schema及当前call/result：全书概览11043、本地检查13268、本章正文13992、设定13872、章节记忆12330、全书记忆8534，均低于14000，活动输入预算16000。原文、设定、目录、事实和检查分页均能完整续读；完整候选与Diff仍保存供作者审阅，模型只收有界预览。

ZIP文件大小、SHA-256及CRC检测结果由`desktop/dist/release.json`记录；目录包和ZIP不含工作区.env、作品数据库、登录信息或供应商主密钥。最终截图见[书房](screenshots/release-library.png)、[规划](screenshots/release-book.png)、[原生桌面规划](screenshots/desktop-book.png)和[桌面重启](screenshots/desktop-restarted.png)。

最终ZIP已生成并通过全部1780个文件的CRC检测：262489391字节（约250.3MiB），SHA-256：`aead61da6cdf150ea6116db7445dd96324af0d45bb31b004e8c9a6c9608a23c2`。版本0.3.0/schema20，与发行回执一致。


## 0.4.0 作品级写作 Skills — 2026-11-13

本节为当前交付状态。书籍概览与章节工作台提供写作Skills入口；支持Markdown、ZIP、GitHub固定提交、JSON资料包预览/确认导入、按作品启停/用途/优先级、编辑与同名完整包换版、导出复用、移除和不可变历史。来源、许可证、引用正文和兼容性提示可查看；文件来源声明不能伪造为GitHub现场核验。

实际验证：

- Go全量205项顶层测试全部通过，0失败、0测试跳过（无测试文件的包另行标识）；真实PG已启用。workbench170.596秒、server28.026秒，日志.local/release-040-final-tests.jsonl。go vet以及API/Worker构建通过。
- Node源码和独立Pi bundle各19项通过，含真实Pi先读Skill再读作品与候选，Responses/Chat本机往返，以及旧Skill页清理后保留ID/version/hash/path/offset续读指针。压缩提示保留具体写作约束并区分未读资料；不把Skill名称当完整规则。
- 新增3项Chrome Skills专项，加已有作者旅程2项/Copilot3项，共8个不同浏览器用例通过；前端5单测、TypeScript、生产构建通过。覆盖完整引用换版、跨书复用、只读角色、409草稿和390px工作台。截图见[Skill列表](screenshots/writing-skills-library.png)、[手机](screenshots/writing-skills-mobile.png)、[资料包](screenshots/writing-skills-package-preview.png)、[当前与冻结指南](screenshots/writing-skills-current-and-frozen.png)。
- 单次任务实际Provider请求含完整主Skill，引用只列目录；超过6000字的主规则在入队前拒绝，提示精简或Copilot。六个并行启用Skill加4000字正文的真实Node估算为13622 tokens，读取2000字Skill为10676，均小于16000。目录和引用都可分页，不只预览前几项。
- Skill变更令旧任务和正文/章纲/全书方案采用失效，运行中修改被识别，下一轮对话重建旧摘要/工具上下文。记忆提取不注入文风，Skill变更不阻碍基于有效定稿的事实提案；两类行为均有真实PG测试。
- 六项开源推荐均经真实GitHub获取成功：StoryDeslop（MIT/3份引用）、Writing Principles（Apache-2.0/4）、Creative Writing Craft（Apache-2.0/11）、Story Review（Apache-2.0/14）、Scene Craft（MIT/9）、Humanizer（MIT/6）。固定提交、正文hash与warning保存在.local/skill-import-previews；脚本没有执行，未自动保存/启用到作者作品。详细[来源与取舍](writing-skills-research.md)。
- 当前Skill与完整资料已接入作品JSON迁移，真实PG往返和恶意相对路径原子拒绝通过。清理前完整实例备份包含18作品、11个Skill、28个Skill历史版本、7私有会话/11轮/33次attempt。实际恢复至novel_restore_writing_skills_040，46业务表计数/正文摘要匹配，sourceMetadataMatches=true；7份可导出当前指南的主文/引用/来源/hash与归档逐项一致，28个版本保留，运行队列为0，未触发模型。记录.local/writing-skills-restore-verification.json。
- 17部明确识别测试作品先逐部JSON归档至.local/e2e-archive-writing-skills后清理，完整历史另有上述dump；主书房只保留原创示例和5条确认事实，无test_* schema。当前最终备份另存.local/backups/writing-skills-final。
- OpenAPI136路径、159 Schema、906内部引用均可解析。YAML新增运行依赖的原始LICENSE/NOTICE已随90个Go/Web组件通知打包。333个候选源码/文档文件对本机私密配置值扫描0命中，桌面不携带工作区.env、数据库或主密钥。

### 实际运行与发行回执

本机8088已升级新API/Web/Pi。真实Chrome登录确认书房、唯一示例、Skill模板/公开目录和供应商设置可访问，无pageerror；本轮没有新增外部模型收费请求。回执.local/main-writing-skills-smoke.json。临时8089已停止。

Windows便携包0.4.0/schema21实际启动，UI导入原创指南，经包内Pi读取read_writing_skill并冻结v1，生成零章节全书候选、作者显式采用；退出PG关闭，重启指南/作品/对话/采用状态保留。contextIsolation与sandbox为true，nodeIntegration为false；回执.local/desktop-skills-final.json。API、Web入口与Pi bundle和当前源码构建的SHA完全匹配。

发行ZIP位于desktop/dist/Yantai-Windows-x64.zip，262848388字节（约250.7MiB），1785文件CRC全通过，SHA-256：`b55e3ecec810953954e713dd8fff19eff5c7dac44ed3a0f404cf5edaca2de6f9`。详见同目录release.json。

工程接入与文学效果分别验收：本轮未正式测试Skill对作者修改时间、长期声音一致性或文学质量的收益；指南可编辑且可按任务选择，不能保证模型逐条遵循或零AI味。


## 首次 GitHub 开源发行

2026-11-13，官方仓库[ToStarLiu/yantai-novel](https://github.com/ToStarLiu/yantai-novel)，采用GPL-3.0-only。首次云端回归发现fpdf对Linux字体绝对路径的处理问题，改为Go读取字体字节后注册，新增Unicode/含空格绝对路径、工作目录独立及缺失文件回归；Windows真实PG出版测试与vet通过。公开发行包重新构建且真实EXE导入Skill、Pi读取/规划采用、退出关闭PG和重启保留均通过，回执.local/desktop-github-final.json。

本次公开ZIP包含项目LICENSE/NOTICE和上游运行组件通知，262847866字节、1787个文件CRC通过，SHA-256：`85b0185e5ba4aedc4dd4610f1b9dc8f8f00cdedff13d40c5cbf9f6c0169ee25b`。此公开包替代前文开发阶段同版本的本地ZIP，最新校验元数据随Releases附带release.json。GitHub全量CI执行结果以对应commit的Actions为准。
