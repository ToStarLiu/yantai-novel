# 参与贡献

欢迎改进砚台。可以从可复现的问题、作者使用反馈、文档、写作 Skill 兼容性或小范围修复开始。提交前请阅读 [README](README.md)、[行为准则](CODE_OF_CONDUCT.md) 和 [安全政策](SECURITY.md)；本项目原创代码采用 GPL-3.0-only，全文见根目录 [LICENSE](LICENSE)。除明确说明的第三方内容外，提交到本项目的贡献也应按 GPL-3.0-only 提供；第三方素材保留各自许可。项目不要求转让版权，也未设置额外的贡献者许可协议（CLA）。

## 先说明要解决的问题

- 缺陷：使用 Bug 表单，提供应用版本、运行方式、复现步骤及脱敏错误信息。
- 功能：说明作者在什么场景遇到障碍、希望怎样完成任务，以及如何判断改进有效。
- 较大变更：先开 Issue 讨论业务边界和兼容性，再实现。界面、数据库、任务行为的改变应能独立评审。
- 安全漏洞：按 [SECURITY.md](SECURITY.md) 私密报告，不在公开 Issue 或 PR 中放利用细节、密钥或作者原稿。

## 准备开发环境

Windows x64 提供项目内工具安装脚本，工具不会安装为系统数据库服务。首次准备：

```powershell
pwsh -File scripts/setup-dev.ps1
if (-not (Test-Path -LiteralPath .env)) { Copy-Item -LiteralPath .env.example -Destination .env }
# 编辑 .env，设置至少 12 字符的 AUTHOR_PASSWORD；已有配置不要覆盖。
. ./scripts/dev.ps1 -Action env
pwsh -File scripts/dev.ps1 -Action db-start
npm --prefix web ci
npm --prefix harness ci
npm --prefix harness run build
npm --prefix harness run bundle
```

用 `pwsh -File scripts/start.ps1` 启动工作台；用 `pwsh -File scripts/start.ps1 -Offline` 显式启动演示模型。其他系统请按 [工具链说明](docs/development/toolchain.md) 准备锁定版本的依赖。Docker 的配置与验证边界见 [部署说明](deploy/README.md)。

`.env` 只接受字面 `KEY=VALUE` 赋值，不执行 Shell 表达式；现有进程环境变量优先。模型连接也可在页面“AI 供应商”配置。普通开发和回归使用演示 Provider 或本机测试服务，真实模型实验须另行明确配置并记录费用。

## 验证变更

在项目根目录的 PowerShell 中，先加载环境，再运行与改动相关的检查：

```powershell
. ./scripts/dev.ps1 -Action env

# 首次创建专用测试库；已有同名库时请先核对用途，不要删除或覆盖。
# createdb -h 127.0.0.1 -p 55432 -U novel novel_test
$env:TEST_DATABASE_URL = 'postgres://novel@127.0.0.1:55432/novel_test?sslmode=disable'
go test ./... -count=1 -timeout=240s
go vet ./...

npm --prefix web run typecheck
npm --prefix web run test -- --run
npm --prefix web run build

npm --prefix harness run build
npm --prefix harness test
npm --prefix harness run bundle
```

数据库测试会在 `TEST_DATABASE_URL` 指向的库里创建、迁移并移除随机 Schema，连接角色需要相应权限。没有设置该变量时，数据库用例会明确跳过；请在 PR 中如实写出跳过项。不要把未执行的检查标记为通过。Go 变更使用 `gofmt`；SQL 查询或模型改变时运行 `pwsh -File scripts/dev.ps1 -Action generate`，并检查生成代码。

浏览器用例在 `web/tests/e2e/`。它们会创建和修改作品，需要专用测试实例与数据库，勿指向作者日常实例。该实例必须使用 `APP_PROVIDER=fake`，默认地址为 `http://127.0.0.1:8089`，`PUBLIC_ORIGIN` 必须完全匹配；测试账号由 `E2E_USERNAME` / `E2E_PASSWORD` 提供，且应与该实例的管理员配置一致。

```powershell
# 在已启动的专用演示实例上执行；浏览器安装与通道须匹配配置。
npm --prefix web run test:e2e
```

[Playwright 配置](web/playwright.config.ts) 默认使用本机 Chrome，单执行器顺序运行，不自动启动 API；可用 `PLAYWRIGHT_CHANNEL` 和 `E2E_BASE_URL` 调整。不要并行启动多份使用同一测试数据和输出目录的 Playwright。工程验收历史见 [验收记录](docs/release/acceptance.md)，它不代表所有新环境都已验证。

## 实现时需要保持的约定

- Go 管理权限、版本、预算、持久数据和作者采用；Pi 执行被授权的对话与工具循环。详见 [上下文工程](docs/development/copilot-context.md)。
- 模型输出保留为候选；正文版本、已确认事实、人物已知与误信需要保持来源边界。摘要、导入 Skill 和外部资料不能成为事实或权限的替代。
- 数据库更改新增编号迁移。避免改写已发布的迁移，更新受影响的导入导出、备份及 API 契约；不要对有价值的数据运行 `migrate-down`。
- HTTP 契约变更同步 `api/openapi.yaml`，前后端一起验证权限、CSRF、跨作品隔离和过期版本。
- 外部请求结果未知时保留需核对状态，不能无提示自动重发。未知费用不能记为零。
- 修复业务错误时增加能复现错误的测试；仅文案或简单样式变化可以提供人工核对记录。界面改动附桌面/移动截图，并遮去作品和账户信息。

## 提交 Pull Request

1. 从当前主分支创建自己的分支，每个 PR 聚焦一个问题。
2. 检查 diff、文件大小及 `git status`。不要提交 `.env`、数据库、备份、主密钥、Token、个人作品、调试日志、原始测试追踪、`node_modules` 或生成的桌面安装包；公开产品截图需确认不含私人内容。
3. 写明触发问题、变更后的行为、验证命令和结果；数据库或兼容性变化说明迁移与回退边界。关联已有 Issue，附必要截图。
4. 保留真实失败和未执行的验证，不用“全部通过”代替证据。讨论围绕实现和作者体验，按反馈迭代。

新增代码、写作 Skill、字体、图片或示例须有明确来源与使用权。不要提交未经许可的整篇小说、付费课程或模仿特定作者的原文语料；第三方许可和通知需随实际分发保留，见 [许可证清单](docs/licenses.md)。
