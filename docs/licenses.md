# 实际依赖与许可证清单

核验日期：2026-11-13。版本依据为Go、Web、桌面和harness各自的模块/锁文件；许可证依据为实际下载包与上游许可原文。本清单对应当前工作区，升级依赖后需随锁文件更新。

## Go 后端依赖

| 模块 | 锁定版本 | 许可证 | 本地核验文件 |
|---|---|---|---|
| `github.com/jackc/pgx/v5` | `v5.13.0` | MIT | 模块缓存 `github.com/jackc/pgx/v5@v5.13.0/LICENSE` |
| `github.com/go-chi/chi/v5` | `v5.5.0` | MIT | 模块缓存 `github.com/go-chi/chi/v5@v5.5.0/LICENSE` |
| `github.com/go-pdf/fpdf` | `v0.9.0` | MIT | 模块缓存 `github.com/go-pdf/fpdf@v0.9.0/LICENSE` |
| `github.com/jackc/pgpassfile` | `v1.0.0` | MIT | 模块缓存 `github.com/jackc/pgpassfile@v1.0.0/LICENSE` |
| `github.com/jackc/pgservicefile` | `v0.0.0-20240606120523-5a60cdf6a761` | MIT | 对应模块缓存根 `LICENSE` |
| `github.com/jackc/puddle/v2` | `v2.2.3` | MIT | 模块缓存 `github.com/jackc/puddle/v2@v2.2.3/LICENSE` |
| `golang.org/x/sync` | `v0.22.0` | BSD-3-Clause | 对应模块缓存根 `LICENSE` |
| `gopkg.in/yaml.v3` | `v3.0.1` | MIT 与 Apache-2.0 许可声明，按原文保留 | 模块缓存根 `LICENSE` 与 `NOTICE` |
| `golang.org/x/text` | `v0.41.0` | BSD-3-Clause | 对应模块缓存根 `LICENSE` |

可用 `go env GOMODCACHE` 定位本机模块缓存。以上许可证原文已实际读取；Go 的标准库与工具链还带有其发行包中的 `LICENSE`、`PATENTS` 和第三方声明。

## 前端直接运行依赖

| 包 | 锁定版本 | 许可证 | 已安装包的原文 |
|---|---|---|---|
| `@tanstack/react-query` | `5.107.1` | MIT | `node_modules/@tanstack/react-query/LICENSE` |
| `@tiptap/react` | `3.33.1` | MIT | `node_modules/@tiptap/react/LICENSE.md` |
| `@tiptap/starter-kit` | `3.33.1` | MIT | `node_modules/@tiptap/starter-kit/LICENSE.md` |
| `@xyflow/react` | `12.12.1` | MIT | `node_modules/@xyflow/react/LICENSE` |
| `diff` | `9.0.0` | BSD-3-Clause | `node_modules/diff/LICENSE` |
| `lucide-react` | `1.72.0` | ISC；部分 Feather 衍生图标另含 MIT 声明 | `node_modules/lucide-react/LICENSE` 包含两部分原文 |
| `react` | `19.3.1` | MIT | `node_modules/react/LICENSE` |
| `react-dom` | `19.3.1` | MIT | `node_modules/react-dom/LICENSE` |

Tiptap/ProseMirror 的间接运行依赖也固定在锁文件中，不使用 Tiptap 商业扩展。图标来自 `lucide-react` 包，本项目没有从参考小说程序复制图标、字体、封面或其他资产。

## 直接开发与验证依赖

| 包 | 锁定版本 | 许可证 | 主要许可证文件 |
|---|---|---|---|
| `@playwright/test` | `1.65.0` | Apache-2.0 | `LICENSE`、`NOTICE` |
| `@types/react` | `19.3.0` | MIT | `LICENSE` |
| `@types/react-dom` | `19.3.0` | MIT | `LICENSE` |
| `@vitejs/plugin-react` | `6.3.0` | MIT | `LICENSE` |
| `prettier` | `3.10.1` | MIT | `LICENSE` |
| `typescript` | `7.0.2` | Apache-2.0 | `LICENSE`、`NOTICE.txt` |
| `vite` | `8.3.8` | MIT | `LICENSE.md` |
| `vitest` | `5.1.0` | MIT | `LICENSE.md` |

当前 npm 锁文件含 173 个包记录（不计项目自身，包含平台可选包）：MIT 121、Apache-2.0 26、MPL-2.0 12、BSD-3-Clause 3、ISC 10、0BSD 1。此数字是锁文件记录数量，不等同于当前平台安装数量或最终浏览器 bundle 的模块数量。新增画布依赖的 d3、Zustand 等间接依赖按锁文件逐包核验，没有把 React Flow 的商用服务引入为运行依赖。

MPL-2.0 记录来自构建工具 `lightningcss@1.33.0` 及 11 个平台二进制可选包。它们属于开发/构建依赖；当前未修改这些包。构建产物不能通过省略依赖目录就省略实际带入产物的第三方通知。发布二进制或静态前端时，应随实际使用的模块保留其许可证、版权与 NOTICE，分发构建工具时同样保留相应文件。Playwright 下载的浏览器发行包还包含其自有许可和第三方声明，属于测试环境，不打入当前应用镜像。

## Pi运行时

Pi为实际引入的运行依赖：`@earendil-works/pi-agent-core`、`@earendil-works/pi-ai`均锁定`1.5.2`，MIT，Mario Zechner；上游许可原文保存在`harness/PI-LICENSE.txt`。本项目只开放Go授权的小说工具，没有引入上游文件读写、shell或编码代理工具。

实际bundle还包含同版本的`pi-schema`、`pi-telemetry`（MIT）、`openai@7.19.0`（Apache-2.0）及`partial-json@0.1.7`（MIT）。`harness/bundle.mjs`根据esbuild实际输入生成`bundle/THIRD-PARTY-NOTICES.txt`并复制各包许可证；桌面和容器按目录携带这些文件。Node.js发行包的`LICENSE`也随桌面运行时携带。完整开发依赖固定在`harness/package-lock.json`；不能把锁文件中未打入产物的所有可选供应商SDK都说成实际运行组件。

## 基础设施与参考项目

桌面包装单独使用 `desktop/package-lock.json`：`electron@45.3.1`（MIT，原文 `desktop/node_modules/electron/LICENSE`）和 `@electron/packager@20.3.0`（BSD-2-Clause，原文包根 `LICENSE`）。52 条间接/直接锁记录为 MIT 36、BlueOak-1.0.0 7、ISC 4、BSD-2-Clause 3、Apache-2.0 2。Electron 分发还保留 `LICENSE` 与 `LICENSES.chromium.html`，后者包含 Chromium 等组件的通知，不能用 Electron 自身 MIT 一项替代。桌面包同时携带的 PostgreSQL 和中文字体各保留独立许可；打包工具本身不等于最终安装包运行依赖。

Go 1.27.4、Node 24.22.3、sqlc 1.31.1、PostgreSQL 18.6 是当前固定开发工具，来源见 [工具链记录](development/toolchain.md)。PostgreSQL 发行包的 `COPYRIGHT`、Node 的 `LICENSE` 和 sqlc 发行包的许可随各自工具保留；`.tools/` 不纳入本仓库。Compose/Dockerfile 的基础镜像也由各镜像发行方的许可证与包声明约束。

仓库内中文 PDF 字体为 `assets/fonts/NotoSansSC-Regular.ttf`，随附完整 `assets/fonts/OFL-NotoSansSC.txt`，许可证 SIL Open Font License 1.1；嵌入 PDF 或分发镜像时需一并保留许可证和版权声明。`fpdf` 的 MIT 许可证不替代字体自身许可证。Windows 系统字体回退仅用于该系统本机使用，不随项目复制分发。

Pagekin（固定核验提交为 GPLv3）和 NstAgent（MIT）仅用于阅读与设计参考。**没有从 Pagekin 复制 GPL 实现、Prompt、测试或资产，也没有从 NstAgent 复制实现文件。** 本项目自行实现作者确认、来源冻结、叙事状态和任务恢复等行为。源码观察范围与固定提交见 [参考仓库核验](../evaluation/repositories.md)。其他调研项目也不是本项目已经引入的依赖。

项目原始代码与文档采用 **GPL-3.0-only**，完整条款见根目录 [LICENSE](../LICENSE)，项目声明见 [NOTICE.md](../NOTICE.md)。第三方依赖和资产保留各自许可证；本清单不改变这些上游授权。

## 0.3.0 运行包通知材料

`scripts/collect-runtime-licenses.mjs`按API/Worker实际链接的Go模块及Web生产锁记录收集原始LICENSE/NOTICE/COPYRIGHT/PATENTS，结果保存在`assets/licenses`。当前89个组件（含Go运行时）的清单见`assets/licenses/THIRD-PARTY-NOTICES.txt`。桌面打包前自动重新生成并整体复制该目录，Docker运行镜像也携带目录；Pi实际bundle的6个组件另由harness生成通知，Node原文随桌面Node运行时保留。开发工具及未实际分发的可选包不据此冒称为产品运行组件。

## 写作 Skill 来源

推荐目录只内置外部URL、提交和许可元数据；五项短中文指南由本项目独立编写。作者导入的第三方正文及LICENSE/NOTICE随作品资料包保存，外部规则的许可不改变项目根授权。具体来源与兼容性见[写作Skill调研](release/writing-skills-research.md)。

0.4.0加入yaml.v3解析标准frontmatter，当前实际Go/Web运行组件通知增为90项，YAML的原始LICENSE和NOTICE随包保留；Pi仍为6项bundle组件。
