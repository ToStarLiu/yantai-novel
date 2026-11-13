# GitHub Actions 持续集成

工作流：[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)。在push、pull_request及手动触发时运行，令牌仅有contents:read；checkout不持久保存凭据，Actions固定到核验过的完整提交SHA。工作流不读取项目密钥、不配置真实模型、不推送发行包。

## 检查范围

| Job | 实际步骤 |
|---|---|
| Go、PostgreSQL与Pi | PostgreSQL18.6临时服务；Go1.27.4；Node24.22.3；按锁文件安装harness；编译与打包；源码和独立bundle测试；go mod verify、go vet；全量真实PG测试；API/Worker构建 |
| Web | Node24.22.3；npm ci；TypeScript；单元测试；生产构建 |

PostgreSQL只使用CI临时账户，测试自行创建/清理随机schema。设置TEST_DATABASE_URL，避免集成测试因无数据库而跳过；结束后检查JSON测试事件，只要存在测试级skip即失败（没有测试文件的包不计入）。后端日志保留为7天Actions产物，不上传数据库、密钥或作者作品。

所有Provider测试使用明确的演示Provider或本机HTTP夹具，不依赖OpenAI/Anthropic等外部账户。中文PDF测试使用仓库自带OFL字体的绝对路径。CI不会在主机安装Windows工具，也不会执行本机setup脚本。

目前一部分Go工具循环测试仍沿用Windows便携Node路径。Linux CI仅在忽略目录.tools/创建node.exe符号链接，实际执行setup-node提供的Linux Node；harness必须先编译，否则测试会被skip检测拦下。该链接不进入源码和发行包。

## 本地复现核心命令

先准备Go1.27.4、Node24.22.3和可用PostgreSQL18，使用独立可丢弃数据库；Windows开发环境入口见[工具链](toolchain.md)。

```text
npm --prefix harness ci
npm --prefix harness run build
npm --prefix harness test
npm --prefix harness run bundle
npm --prefix web ci
npm --prefix web run typecheck
npm --prefix web run test -- --run
npm --prefix web run build
go mod verify
go vet ./...
go test ./... -count=1 -timeout=240s
```

完整真实PG测试需TEST_DATABASE_URL；PDF_FONT_PATH可指向assets/fonts/NotoSansSC-Regular.ttf的绝对路径。独立bundle测试设置HARNESS_TEST_ENTRY为harness/bundle/main.mjs的绝对路径，再运行harness test。

## 验证边界

工作流已通过actionlint1.7.12静态校验；是否在GitHub执行成功以对应commit的Actions结果为准。本地205项后端、19项Pi及浏览器/桌面验收属于本地证据，不能冒充云端CI结果。

当前工作流不运行浏览器E2E、Electron打包、Docker镜像构建或模型文学质量评估。浏览器测试需要独立的离线API服务和浏览器安装；桌面发布需要Windows运行组件与单独验收。不要把CI绿色等同于作者质量通过或所有交付平台已经实跑。
