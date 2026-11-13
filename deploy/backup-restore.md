# 实例备份与恢复

完整PostgreSQL备份与单作品包承担不同用途。实例备份保留所有作品、版本、记忆来源、任务、调用尝试和原始结果；单作品包适合迁移一部小说，排除密钥、登录凭据与可执行队列。

## Windows备份

先暂停新任务和编辑写入，等正在进行的请求完成或明确记录为需核对。脚本默认连接 `127.0.0.1:55432`、角色`novel`、数据库`novel`，不读取或打印模型密钥。

```powershell
pwsh -File scripts/backup.ps1
```

可显式传 `-Database`、`-ServerHost`、`-Port`、`-UserName`、`-OutputDirectory`。需要数据库密码时使用PostgreSQL的`PGPASSFILE`或`PGPASSWORD`，不把密码放命令行。备份默认写入Git忽略目录 `.local/backups/`：

- `novel-<UTC时间>.dump`：`pg_dump --format=custom --no-owner --no-acl`一致数据库快照。
- 同名`.dump.json`：SHA-256、Schema版本、项目/章节/版本/状态/事实/任务尝试数量与正文ID+hash摘要，不含正文内容。
- 若数据库有页面保存的加密供应商凭据，另复制同名`.dump.provider-master.key`并在元数据记录校验摘要。默认读取`.local/provider-master.key`；桌面或自定义实例须先设置正确的`PROVIDER_MASTER_KEY_PATH`。缺少对应主密钥会明确判定备份不完整，保留dump供恢复。

并行自动测试创建的`test_*` schema明确排除，避免它们在pg_dump锁表期间被测试清理。生产应用使用public schema；不把临时测试数据当作业务恢复来源。pg_dump本身使用一致快照，JSON计数在转储后立即采集，因此要暂停应用写入以做严格计数对比。脚本不会把不完整dump标为已验证备份。

备份包含原稿和完整模型响应；供应商主密钥副本能够解密库内凭据，须作为私密备份一并妥善保存。`.env`和作者明文密码仍位于数据库之外，需要另行保存；不把这些值写入备份元数据或文档。

## 恢复到隔离新库

恢复脚本只允许新名称 `novel_restore_[a-z0-9_]{1,40}`，禁止覆盖`novel`、`postgres`及任何已存在数据库。需要有createdb权限的数据库角色。首先选择实际成功备份：

```powershell
pwsh -File scripts/restore.ps1 `
  -BackupFile .local/backups/novel-<时间>.dump `
  -TargetDatabase novel_restore_check01
```

脚本先验证备份SHA-256，创建空库，再执行 `pg_restore --exit-on-error --single-transaction`。失败时保留新库诊断，原数据库不受影响，不自动删除或覆盖文件。

恢复完成后，脚本在副本中隔离可能执行的任务：

- 原`Running`改为`NeedsReconciliation`，清除租约；原调用尝试与原始响应仍保留。
- 原`Pending/RetryScheduled`改为`Paused`，只有显式恢复才会执行。
- 删除会话表中的会话，避免恢复副本复用原登录；重新配置作者账户登录。
- 终态任务、正文、事实、状态、来源与已有用量不改写。

输出 `.restore.json` 回执，比较数量和正文摘要。`sourceMetadataMatches=false`会以错误退出，但副本仍保留；必须检查是否在备份与计数间发生写入，不可忽视差异。

供应商主密钥sidecar会核对SHA-256并恢复到`.local/restored-provider-keys/<新库名>.key`，不会覆盖活动实例主密钥。回执给出`restoredProviderKeyPath`；验证该副本时将`PROVIDER_MASTER_KEY_PATH`设为此路径。Copilot的待执行轮次被取消、执行中轮次及attempt标记为需核对，不自动重发模型请求。

第一次查看副本时使用独立端口和以下进程配置，确保不会发模型请求：

```powershell
. ./scripts/dev.ps1 -Action env
$env:DATABASE_URL='postgres://novel@127.0.0.1:55432/novel_restore_check01?sslmode=disable'
$env:APP_PROVIDER='fake'
$env:RUN_WORKER='false'
$env:APP_ADDR='127.0.0.1:8098'
$env:PUBLIC_ORIGIN='http://127.0.0.1:8098'
go run ./cmd/api
```

核对作品列表、中文正文、历史版本、定稿、记忆来源、任务历史和导出。完整恢复验证后，再安排停机切换DATABASE_URL；不能让原实例和恢复实例同时写同一项目并误以为自动同步。切换真实模型前先核对Paused/NeedsReconciliation任务，逐项决定保留或新建意图。

## 2026-11-12实际恢复演练

北京时间20:36左右，用本机PostgreSQL18.6实际执行：

- 成功转储：`.local/backups/novel-20261112T123617666Z.dump`。
- 成功恢复新库：`novel_restore_20261112_123646`；原库`novel`未覆盖。
- 回执：`.local/backups/novel-20261112T123617666Z.dump.novel_restore_20261112_123646.restore.json`，`sourceMetadataMatches=true`。

| 对比字段 | 来源备份元数据 | 恢复副本 |
|---|---:|---:|
| Schema版本 | 5 | 5 |
| 作品 | 3 | 3 |
| 章节（含软删除） | 2 | 2 |
| 正文版本 | 8 | 8 |
| 状态版本 | 1 | 1 |
| 事实 | 0 | 0 |
| Web任务 | 3 | 3 |
| Web调用尝试 | 3 | 3 |
| 正文ID+hash摘要 | `16a70c7cbd324689e8a9df2ac2684b98` | 相同 |

该首轮快照的事实数为0，因此另在隔离恢复库中通过应用服务创建原创章节，审核并提交位置、伤势、物品、角色知识、伏笔五类事实，再做第二轮完整转储恢复。没有向原开发库插入验证作品，也没有调用模型。

第二轮北京时间20:40实际执行：

- 来源隔离库：`novel_restore_20261112_123646`。
- 转储：`.local/backups/novel_restore_20261112_123646-20261112T124030590Z.dump`。
- 新恢复副本：`novel_restore_full_20261112_124031`。
- 结果：Schema5、作品4、章节3、正文版本10、状态2、事实5、任务3、attempt3完全一致；正文摘要`de03acd7b9857f127a0b2cdac5b16515`一致，回执`sourceMetadataMatches=true`。
- 五类事实的revisionId、hash与原文quote均在恢复副本回查通过；单作品ID重映和证据重建另由 `TestNarrativeArchiveRoundtripPreservesMappedEvidence` 等真实数据库测试覆盖。

两项故障已实际发现并修正：第一次pg_dump遇到正在被测试删除的临时schema，现排除`test_*`；第一次恢复的隔离SQL含中文在本机psql参数编码下报错，现使用ASCII恢复原因并在新副本重试通过。失败副本`novel_restore_20261112_123617`为诊断残留，不用于应用连接。成功副本与回执保留供复查。

## 高级系统恢复演练

2026-11-12补充实际完成schema15、schema16两轮新库恢复。清理前54作品、137正文版本、7条事实和全部高级配置/历史由完整dump保存；schema15恢复为`novel_restore_advanced_final`。最终schema16清理后备份位于`.local/backups/advanced-final`，恢复为`novel_restore_release_final`，保留1部原创示例和5条确认事实。

新增`verify-table-counts.sql`逐表核对38个非会话/在线状态业务表计数，正文hash摘要也一致，两份回执均`sourceMetadataMatches=true`。恢复脚本同时暂停Production、隔离Running媒体、取消未派发媒体、标记候选需注意与MCP未知调用、清除会话/presence、撤销未使用邀请。恢复副本不会自动重发外部调用。

## 2026-11-13加密供应商恢复演练

北京时间 02:56，使用全新的隔离来源库 `novel_restore_provider_src_20261112_185613` 应用全部迁移至 schema 20，存入一部虚构作品、两条正文版本、两代不可变供应商配置及两份加密凭据（虚构 API 密钥与敏感请求头）。没有读取或修改主库供应商配置。

实际调用 `scripts/backup.ps1` 与 `scripts/restore.ps1`，成功恢复至另一个新库 `novel_restore_provider_dst_20261112_185613`：

- 转储目录：`.local/backups/provider-drill-20261112_185613/`。
- 转储文件：`novel_restore_provider_src_20261112_185613-20261112T185616915Z.dump`，同时保留元数据和独立主密钥 sidecar。
- 原始恢复回执：同一转储文件后缀 `.novel_restore_provider_dst_20261112_185613.restore.json`。
- 凭据与本地 HTTP 验证回执：原始恢复回执后缀 `.provider-verification.json`。
- 演练入口与最近路径索引：`.local/provider-restore-drill/main.go` 和 `.local/provider-restore-drill/latest.json`；均位于 Git 忽略目录。

| 核对项 | 结果 |
|---|---|
| Schema / 作品 / 章节 / 正文版本 | 20 / 1 / 1 / 2，来源与副本一致 |
| 业务表计数 | 44 张表一致，`sourceMetadataMatches=true` |
| 正文 ID+hash 摘要 | `0519aa2d9aac4acf561dcd62ce126320`，恢复前后一致 |
| 供应商配置 / 加密凭据 | 2 / 2，恢复后完整保留 |
| Sidecar SHA-256 | 与备份元数据匹配 |
| 恢复主密钥 SHA-256 | 与备份元数据匹配，恢复到独立文件，来源主密钥不变 |
| 旧代与当前代凭据 | 使用回执 `restoredProviderKeyPath` 解密，两代均经本地 HTTP 检查正确的 API 密钥与敏感请求头 |
| 页面安全视图 | 当前列表仅显示新版本，无明文凭据或内部密文引用 |
| 外部模型请求 / 费用 | 0 / 0；2 次 HTTP 请求仅发送到本机契约服务 |

回执中的隔离主密钥路径为 `.local/restored-provider-keys/novel_restore_provider_dst_20261112_185613.key`。演练副本、虚构 fixture、转储和回执保留供复查；没有启动后台任务或切换活动应用数据库。此次没有发现备份恢复脚本错误。

## Docker备份命令

容器部署可将同样的pg_dump/pg_restore流程在postgres容器内执行；不要依赖复制正在写入的数据目录作为一致备份。使用与数据库大版本兼容的客户端，保留自定义dump和SHA-256，恢复到新的数据库或隔离实例。示例：

```powershell
docker compose --env-file .env -f deploy/compose.yaml exec -T postgres pg_dump -U novel -d novel -Fc -f /tmp/novel.dump
docker compose --env-file .env -f deploy/compose.yaml cp postgres:/tmp/novel.dump .local/backups/container-novel.dump
```

本次机器未运行Docker，上述容器命令是部署说明，不能标记为已执行。使用本项目恢复脚本进行核对时，数据库需能从所指定的回环端口访问。

## 2026-11-13：Copilot 非空会话与最终实例恢复

清理前完整转储`.local/backups/copilot-before-cleanup/novel-20261112T193151722Z.dump`实际恢复至新库`novel_restore_copilot_history`，17个私有会话、36轮对话、92个attempt、6个全书候选及真实Responses成功记录完整保留。44张业务表计数及正文ID/hash摘要一致，回执`sourceMetadataMatches=true`；恢复后Pending/Running轮次和Running attempt均为0，没有启动模型Worker。供应商主密钥sidecar核对并恢复至独立新路径，原实例未改动。

随后最终schema20实例的转储`.local/backups/copilot-final/novel-20261112T193157142Z.dump`恢复至新库`novel_restore_copilot_final`，1部示例/3章/8版本/5条确认事实、44张表计数及正文摘要全部匹配。以上回执均保留在各dump同目录；时间戳为UTC，实际本地日期为2026-11-13。
