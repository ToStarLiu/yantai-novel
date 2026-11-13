# 开发工具链与安装记录

初始化日期：2026-11-12（Asia/Shanghai）。本阶段只需要 Go、sqlc 与 PostgreSQL；Node/npm、Docker 和其他推理运行时不作为阶段 00 的安装前提。所有便携工具位于忽略提交的 `.tools/`，数据库和日志位于 `.local/`。

后续Web系统已增加Node.js24.22.3/npm11.19.1，setup脚本包含该便携工具。[官方下载](https://nodejs.org/dist/v24.22.3/node-v24.22.3-win-x64.zip)的[官方SHA256](https://nodejs.org/dist/v24.22.3/SHASUMS256.txt)为 `e612af99f626ab415ea225af6bce5c971feb448cb5cae250bb1c3fa50c5f7d9f`，下载校验及执行通过。dev脚本仅在当前进程加入Node路径，应用入口为scripts/start.ps1。前端由package-lock锁定，后端新增chi5.5.0由go.mod/go.sum锁定。

## 固定版本与来源

| 工具 | 固定版本 | 来源与核验 |
|---|---|---|
| Go | `go1.27.4 windows/amd64` | [官方下载 JSON](https://go.dev/dl/?mode=json)、[发行记录及支持政策](https://go.dev/doc/devel/release)。1.27.4 发布于 2026-11-10；按官方支持政策，1.27 与 1.26 是当前支持分支，设计原先的 1.24 下限不代表仍应使用它开发。 |
| sqlc | `v1.31.1` | [官方 GitHub release](https://github.com/sqlc-dev/sqlc/releases/tag/v1.31.1)、[release API](https://api.github.com/repos/sqlc-dev/sqlc/releases/tags/v1.31.1)，核对 Windows amd64 ZIP 的资产摘要；生成文件也记录该版本。 |
| PostgreSQL | `18.6` | [官方版本支持表](https://www.postgresql.org/support/versioning/)、[官方 Windows 下载说明](https://www.postgresql.org/download/windows/) 指向 EDB 提供的安装器和 ZIP。18 分支在支持期内。 |
| pgx | `v5.13.0` | `go.mod` 与 `go.sum` 锁定，模块由 [Go 模块服务](https://proxy.golang.org/github.com/jackc/pgx/v5/@v/v5.13.0.info) 发布记录核验；间接依赖以实际锁文件为准。 |

2026-11-12 核验时，PostgreSQL 官方版本表展示 18.6，EDB 当前下载页展示 18.7；为与已核验开发版本一致，本阶段锁定实际可下载并已执行 `postgres --version` 的 18.6，而不把 EDB 页面“当前版本”当作本项目已测试版本。后续版本升级需单独运行迁移和持久化回归。

下载文件：

| 文件 | 下载地址 | SHA-256 依据 |
|---|---|---|
| Go Windows x64 ZIP | [go1.27.4.windows-amd64.zip](https://go.dev/dl/go1.27.4.windows-amd64.zip) | 官方 JSON：`178ab98950ac69ab25069307dc6ade79f3816e414cce95b4da166a115cde1e7e` |
| sqlc Windows x64 ZIP | [sqlc_1.31.1_windows_amd64.zip](https://github.com/sqlc-dev/sqlc/releases/download/v1.31.1/sqlc_1.31.1_windows_amd64.zip) | 官方 release asset digest：`352711fa7dcb05dcdfefca0ad71b2c9a74fd090f8d7fc609419de4cbc725429f` |
| PostgreSQL Windows x64 ZIP | [postgresql-18.6-1-windows-x64-binaries.zip](https://get.enterprisedb.com/postgresql/postgresql-18.6-1-windows-x64-binaries.zip) | 本地下载文件：`fbe23da234ee31547bf8a36d29dfd81e82b849df2d2b78d2eecb43d360252f8c`。本地计算，仅用于传输和重复下载诊断；本次未获得上游发布的历史文件摘要，不将此值作为上游认证。 |

`setup-dev.ps1` 仅在可执行文件缺失时下载和解压，Go/sqlc 解压前与上述上游摘要比较；已有工具运行版本检查。脚本不安装 Windows 服务、不修改永久 PATH、不修改系统环境变量。PostgreSQL 历史包由 EDB 的 HTTPS 地址提供，安装记录保留本地 SHA-256。

## Windows 便携开发入口

在项目根目录运行：

```powershell
./scripts/setup-dev.ps1
./scripts/dev.ps1 -Action db-start
./scripts/dev.ps1 -Action migrate-up
./scripts/dev.ps1 -Action generate
./scripts/dev.ps1 -Action test
./scripts/dev.ps1 -Action eval --fixture evaluation/fixtures/bridge-3 --provider fake --output evaluation/results/baseline-fake
./scripts/dev.ps1 -Action db-stop
```

命令结果以实际运行记录为准；工具安装完成本身不代表应用测试通过。`migrate-down` 显式回滚当前业务迁移，会删除该开发数据库中的业务表，限专用开发实例使用。它不删除 PostgreSQL 数据目录。`test` 的数据库集成测试使用 `TEST_DATABASE_URL`，每个测试自行建立独立 Schema 并在结束时清理，正常作品表不作为测试数据。

开发数据库首次运行使用 `initdb -U novel -A trust -E UTF8 --locale=C`，仅监听 `127.0.0.1:55432`，不对局域网开放。`trust` 是本机开发配置；远程部署使用 Compose 密码配置及相应访问控制。数据库名及开发用户均为 `novel`，数据在 `.local/pgdata`，日志在 `.local/postgres.log`。停止采用 `pg_ctl ... stop -m fast`，保留数据以供再次启动；脚本不会删除旧数据库或覆盖不同大版本的数据目录。

`dev.ps1` 读取 `.env` 中的字面 `KEY=VALUE`（允许整值单引号或双引号），不执行任何表达式，也不输出值。注释放独立行，不进行变量替换；已有进程环境优先于 `.env`。尚未配置时采用本机开发连接地址。需要在当前 PowerShell 手工使用 `go`/`sqlc` 时，可点源加载环境：

```powershell
. ./scripts/dev.ps1 -Action env
go version
sqlc version
```

真实模型使用 `LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`，以及与接口匹配的 `LLM_WIRE_API`；可选 `LLM_ACTOR_AUTHORIZATION` 同样视为秘密。示例选择 `responses`，需要聊天补全协议的服务可改为 `chat`。Web任务与Copilot支持环境和页面的模型价格快照；未配置可核实价格时保持 `Unknown/null`，不能把缺失价格写成零。阶段00 CLI的历史评估口径见对应基线记录。

Go 默认官方代理在本次网络中直连超时，因此脚本仅在 `GOPROXY` 尚未设置时使用 `https://goproxy.cn,direct`。该镜像是下载通道，不是依赖信任来源；脚本没有关闭 Go 模块校验。可在启动脚本前通过进程环境将 `GOPROXY` 改为官方代理或自己的镜像。依赖确切版本和校验和仍由 `go.mod`、`go.sum` 管理。

## 可选 Docker Compose

`deploy/compose.yaml` 固定 `postgres:18.6`，映射同一个回环开发端口，使用命名卷持久化，并要求外部提供 `POSTGRES_PASSWORD`。不要同时启动便携数据库和 Compose 数据库。Docker Compose 并未在当前机器安装，因此该配置是否能在 Docker 上启动，应在具备 Docker 的环境单独验证，不记录为本机已通过。

先在自己的 `.env` 中设置密码，并在数据库 URL 中使用 URL 编码后的密码；调用 Compose 时指定项目根目录的环境文件：

```powershell
docker compose --env-file .env -f deploy/compose.yaml up -d postgres
docker compose --env-file .env -f deploy/compose.yaml ps
```

Compose 的数据目录使用 PostgreSQL 18 镜像约定的 `/var/lib/postgresql` 挂载点。上面仅启动数据库的命令保留为开发入口；当前 `deploy/compose.yaml` 已提供显式迁移、API、Worker和PostgreSQL完整服务，以及页面AI配置所需的共享持久密钥卷。镜像构建还会构建Web与Pi harness，容器内Node执行Pi。完整启动说明见[部署文档](../../deploy/README.md)。本机没有Docker，未将配置存在或静态检查记为容器实跑通过。
