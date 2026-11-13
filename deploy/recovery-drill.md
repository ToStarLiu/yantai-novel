# 任务恢复与故障演练

任务生命周期属于数据库和Worker，浏览器仅订阅进度。关闭页面、SSE连接断开、刷新都不会取消生成。取消、暂停、恢复和重新生成是显式操作；未知结果不能归入可安全重试。

## 实际工程验证入口

所有故障测试使用假Provider或本地HTTP模拟，不消耗真实模型配额。

```powershell
pwsh -File scripts/dev.ps1 -Action test
```

重点测试位于 `internal/workbench/runtime_test.go`、`narrative_test.go`、`retrieval_test.go`、`internal/server/server_test.go` 和阶段00的`internal/generation/pipeline_test.go`：

| 故障或竞态 | 实现行为 |
|---|---|
| 同一意图重复请求 | 唯一意图约束返回同一任务，输入不同则冲突 |
| 两Worker竞争 | PostgreSQL事务与行锁、租约代次；失去代次不能提交 |
| 页面/SSE断开 | Worker不使用订阅请求context，事件持久保存后可重放 |
| 调用开始前崩溃 | 任务与attempt状态用于决定是否可重新领取 |
| 可能已发送但无响应 | NeedsReconciliation，原始尝试保留，不自动再发 |
| 模型完成但保存响应失败 | NeedsReconciliation，不能当成普通失败自动重试 |
| 取消与完成竞争 | 保存已经产生的输出与用量；取消先记录时不采用正文或记忆 |
| 暂停 | 在当前步骤完成后生效，已产生结果保留；恢复前重新验证输入 |
| 旧稿、章纲或状态变化 | 运行结果标stale；采用前再校验正文/epoch/章纲/状态基线 |
| 未审核或过期记忆提交 | 作者决策、来源、基线、章节顺序同事务校验，失败无半份状态 |
| 旧稿换版 | 同事务失效后续状态/事实/索引，后续正文只标待复核 |

具体哪些用例已经执行及结果应以最终验收记录为准；本文件是运行语义与演练入口，不把待执行的容器故障命令当成通过证据。

## 进程重启

普通停机先让Worker停止领取任务，给在途操作有限时间保存结果。强制退出后不要手动把所有Running改为Pending。Worker的 `RecoverExpired` 等待租约到期后检查未完成attempt：可能已执行的调用进入NeedsReconciliation，没有外部调用的工作才可回到Pending。

需要复现进程终止时使用专用测试实例：

1. 设置 `APP_PROVIDER=fake`，记录测试任务ID和attempt ID。
2. 启动受控慢调用任务，确认attempt已持久提交。
3. 终止专用Worker，保留数据库和日志。
4. 租约过期后启动同配置Worker，核对任务进入预期状态。
5. 断开重连SSE，核对持久事件可续读；记录是否产生第二个attempt。

不得对用户真实收费调用做破坏性故障注入。测试环境的恢复按钮也不能绕过正文和记忆基线变化。

## 未知结果处理

界面保留未知任务、原输入、attempt、原始结果与已知用量。仅在提供商有已验证的幂等或结果查询能力时自动核对；当前通用兼容接口不承诺此能力。没有核对手段时，可以保留未知结果，或由作者明确重新生成一个新意图，了解可能增加调用次数。

“重新生成”是新业务意图，不是网络重试。旧任务不删除，成本账目不清零；费用缺少价格证据时继续显示Unknown。

## 从完整备份恢复

`scripts/restore.ps1`只恢复到新库，并把Running改NeedsReconciliation、Pending/RetryScheduled改Paused，清除租约与会话。副本首次启动配置 `APP_PROVIDER=fake`、`RUN_WORKER=false`。核对来源与任务状态后再恢复真实调用，不能让恢复副本自动执行原库剩余队列。

当前实现将记忆、状态快照与索引更新置于同一PostgreSQL事务，没有异步索引Outbox积压；事务失败一并回滚。关键词检索只读有效已确认来源，向量/Rerank路线尚未经过本轮对照，不参与恢复正确性的前提。
