# 升级与迁移

数据库Schema由编号SQL迁移控制。API/Worker启动时检查支持的Schema版本，不静默升级生产库。迁移命令按序在事务中应用，重复`up`幂等；`down`会完整删除业务表，不能当作生产回滚按钮。

## v0.4.0 → v0.5.0

本轮从schema21升级到schema23，新增账号私有作者记忆与私人创作任务上下文。备份应同时保留数据库和匹配的provider-master.key；单作品导出不含私人记忆，不能替代实例备份。Windows便携版会在启动前显式迁移，操作与数据目录保留见[v0.5.0发布说明](../docs/release/v0.5.0.md#从旧版升级)。源码部署仍按下列流程停写、备份、迁移并切换同版本API/Worker。

## 升级流程

1. 记录当前应用版本、依赖锁文件与`schema_migrations`版本；暂停新生成、编辑及索引写入。
2. 等已有模型调用完成。结果未知时保留NeedsReconciliation，不能为升级自动重试。
3. 执行完整备份，恢复到新隔离库，按 [备份恢复](backup-restore.md) 比较计数、正文摘要与来源。只有已验证可恢复的备份才作为升级回退点。
4. 在隔离副本上先运行新版本迁移及后端/前端检查，启动 `APP_PROVIDER=fake`、`RUN_WORKER=false` 的验证服务，检查导入导出与历史可读。
5. 停止旧API/Worker，在正式库显式运行迁移，再启动同版本API/Worker。
6. 检查健康端点、登录、保存冲突、SSE、状态来源、任务恢复与备份入口；确认后恢复写入。

Windows命令入口：

```powershell
pwsh -File scripts/backup.ps1
pwsh -File scripts/dev.ps1 -Action migrate-up
pwsh -File scripts/dev.ps1 -Action test
```

Docker入口：

```powershell
docker compose --env-file .env -f deploy/compose.yaml stop api worker
docker compose --env-file .env -f deploy/compose.yaml run --rm migrate
docker compose --env-file .env -f deploy/compose.yaml up -d api worker
```

更新镜像构建应在切换前完成。Compose迁移容器没有持续运行的Worker，失败会阻止依赖服务启动。当前Docker命令未在本机容器环境执行，须在目标部署独立验证。

## 回退边界

仅应用代码回退要求旧代码支持当前Schema；不能假定向下兼容。若迁移不可逆，恢复到升级前备份的新库，使用对应旧版本应用，验证后再切换连接。升级后产生的新编辑不会自动出现在旧备份中，需要单独导出迁移或人工处理，不能覆盖新稿假装无损回滚。

检索索引是派生数据，可从已确认有效正文重建；正文Revision、作者设定、已审核事实和证据是权威数据，不能通过“清缓存”删除。索引更新与记忆状态提交目前同一PostgreSQL事务，未引入第二套消息队列或Outbox迁移。

单作品包有独立`formatVersion`，不支持版本在导入前拒绝。服务端验证ID归属、正文哈希、来源与父链，不信任包内模型ID或可执行任务。密钥与会话不随包迁移。
