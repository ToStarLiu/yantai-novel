# 部署与日常运行

该系统支持个人作者和作品成员协作：React/Tiptap前端、Go API/Worker、PostgreSQL。API、SSE与前端同源提供；另有PWA和自带数据库的Windows便携包。初始管理员在服务端环境配置；文本模型可在登录后的“AI供应商”页面填写地址、模型和密钥，也兼容原环境配置。页面密钥加密保存在数据库，本机主密钥单独持久化；其他账户通过一次性邀请创建，密钥值不下发前端。生成质量和作者试用按用户指示后置。

## Windows 本机运行

从项目根目录执行。脚本安装项目内便携工具，不改永久PATH、不安装数据库服务；数据库只监听回环地址。

```powershell
pwsh -File scripts/setup-dev.ps1
pwsh -File scripts/dev.ps1 -Action db-start
pwsh -File scripts/dev.ps1 -Action migrate-up
```

先将 `.env.example` 复制为 `.env` 并配置作者账户。模型可以启动后在页面配置，也可在环境中一次填写完整连接：

```dotenv
DATABASE_URL=postgres://novel@127.0.0.1:55432/novel?sslmode=disable
AUTHOR_USERNAME=author
AUTHOR_PASSWORD=<至少12字符的自选密码>
APP_ADDR=127.0.0.1:8088
PUBLIC_ORIGIN=http://127.0.0.1:8088
COOKIE_SECURE=false
APP_PROVIDER=auto
RUN_WORKER=true
LLM_BASE_URL=
LLM_MODEL=
LLM_WIRE_API=responses
LLM_API_KEY=
MAX_OUTPUT_TOKENS=3000
MAX_CONCURRENT_JOBS=2
```

`.env` 只接受字面 `KEY=VALUE`，不执行Shell表达式；已有进程变量优先。使用 `pwsh -File scripts/start.ps1` 完成前端构建和服务启动；也可在项目根目录的 PowerShell 中运行：

```powershell
. ./scripts/dev.ps1 -Action env
$taskNodeDir = Join-Path (Get-Location) '.tools/node-v24.22.3-win-x64'
$env:PATH = $taskNodeDir + [IO.Path]::PathSeparator + $env:PATH
npm --prefix web ci
npm --prefix web run build
npm --prefix harness ci
npm --prefix harness run build
$env:HARNESS_NODE_PATH = Join-Path $taskNodeDir 'node.exe'
$env:HARNESS_ENTRY_PATH = Join-Path (Get-Location) 'harness/dist/main.js'
go run ./cmd/api
```

本地浏览器访问 [工作台](http://127.0.0.1:8088)，使用自己配置的账户登录。`APP_PROVIDER=auto`（默认）在 `LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL` 全空时以明确标识的演示模型启动；三项完整则使用该环境连接。填写部分连接会报错，不能悄悄回退为演示。显式 `configured` 要求完整有效连接，显式 `fake` 用于离线验证。登录后保存并启用默认页面连接立即生效，无需重启；fake不证明写作质量，也不应混同于已采用的真实正文。

在API内运行Worker适合本机单实例。需要分进程时设置API的 `RUN_WORKER=false`，再在相同配置下运行 `go run ./cmd/worker`。模型任务、事件、调用尝试均在数据库；关闭页面不会取消任务。

## 新机器 Docker Compose

前提：Git、Docker Engine及Docker Compose可用，端口8088/55432未被占用。选择项目发布对应版本，保留 `go.sum` 与前端锁文件。

1. 将 `.env.example` 复制为 `.env` 并配置作者密码。保留 `APP_PROVIDER=auto` 可先以演示模式启动，在页面配置模型；也可一次填写完整的模型地址、模型名与密钥。
2. 为Compose的 `POSTGRES_PASSWORD` 设置至少32字符的随机十六进制/URL安全字符串。当前Compose将密码插入数据库URL；若使用`@:/#?%`等保留字符，需改为显式URL编码的DATABASE_URL配置，不能直接原样拼入URL。
3. 本机调试保持 `PUBLIC_ORIGIN=http://localhost:8088`、`COOKIE_SECURE=false`。浏览器必须使用与PUBLIC_ORIGIN完全相同的scheme/host/port。
4. 先备份已有实例，再执行：

```powershell
docker compose --env-file .env -f deploy/compose.yaml up --build -d
docker compose --env-file .env -f deploy/compose.yaml ps
docker compose --env-file .env -f deploy/compose.yaml logs --tail 100 api worker
```

Compose包含PostgreSQL、显式迁移容器、API和独立Worker；迁移成功后才启动API/Worker。`deploy/Dockerfile` 分阶段构建 Web、Go API/Worker 与 Pi harness，运行镜像包含 Node.js、编译产物、字体、运行依赖许可、证书和时区，以非root用户提供服务。镜像API内Worker默认关闭，由独立Worker容器执行任务。

PostgreSQL 18命名卷挂载在 `/var/lib/postgresql`。API、Worker和迁移共享 `app_secrets:/app/data`，页面凭据的 `PROVIDER_MASTER_KEY_PATH=/app/data/provider-master.key` 随该卷保留。数据库备份还需同一实例的主密钥；恢复副本使用独立主密钥文件，见[备份恢复](backup-restore.md)。`docker compose down` 保留卷；不要把带 `-v` 的删除卷命令用于日常停机。Docker Compose与本机便携PostgreSQL占用同一55432端口时只能运行一种。

本次机器未安装Docker，因此Dockerfile/Compose的本机容器启动没有实测证据；实际已验证的是Windows便携Go/PostgreSQL、真实数据库测试、HTTP接口、前端构建及浏览器流程。容器环境需按上面命令独立核验，不能把文件存在写成容器已运行。

## 远程HTTPS部署

将API的8088保持绑定服务器回环，由Nginx/Caddy等反向代理终止TLS。设置 `PUBLIC_ORIGIN=https://你的域名`、`COOKIE_SECURE=true`，不要把开发数据库trust配置暴露到网络。

Nginx位置段示例（证书、域名和server配置由部署环境提供）：

```nginx
location / {
    proxy_pass http://127.0.0.1:8088;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;
    proxy_read_timeout 300s;
    client_max_body_size 34m;
}
```

关闭代理缓冲使SSE事件及时送达；连接断开后，客户端可按事件ID续读已保存记录。登录和写操作校验Origin及CSRF，错误域名/端口会被拒绝。

## 数据与运行边界

- 完整实例备份使用 `scripts/backup.ps1`；恢复到新库使用 `scripts/restore.ps1`，流程见 [备份恢复](backup-restore.md)。单作品JSON包用于跨实例迁移，不包含登录、密钥或可执行任务。
- 模型输出是候选，作者采用才产生正文版本；定稿和记忆确认分开。修改旧定稿会产生影响报告与保守失效，不自动改写后文。
- 当前检索默认为受作品、版本、章节位置限制的关键词/实体检索；没有假称已经完成向量/Rerank质量对照。
- 记忆提交与派生索引更新在同一个PostgreSQL事务内完成，以原子性保证一致，当前没有另建通用Outbox/消息队列。
- 费用缺少实际价格依据时显示Unknown/null；Token记录不等于账单。API只提供脱敏模型配置。
- 升级、迁移与恢复前先停止接收新任务；未知外部调用结果需核对，不能因为重启就自动再发。详见 [恢复演练](recovery-drill.md) 与 [升级说明](upgrade.md)。
