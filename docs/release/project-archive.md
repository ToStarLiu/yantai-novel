# 作品包覆盖范围与恢复边界

2026-11-12：针对高级模块完成了一次带非空数据的真实PostgreSQL往返测试，使用所有当前迁移（含`000015_project_archive`）。测试文件：`internal/workbench/project_archive_test.go`。

| 数据 | 作品JSON包 | 导入行为 |
|---|---|---|
| 项目信息、角色/世界/规则/文风/大纲设定 | 包含；显式采用的voice就是style设定 | 新作品、新ID |
| 全章节、软删除章节、全部正文Revision/父来源/hash | 包含 | 原子验证、ID重映；不覆盖现作品 |
| 确认状态/事实、历史提案及证据 | narrative归档包含 | 验证所属、Revision/hash/quote；重建派生Chunk |
| Canvas、Brief、Scene、人物弧、参考资料、批注、沙盒 | Studio当前文档及全部历史版本 | 保留文档版本；所有内部实体/节点/章节/证据ID重映 |
| 沙盒预览、已确认State、事件、全部检查点 | sandbox文档包含 | 仍为独立沙盒；不改变正文/Fact |
| Research公开来源、抓取时间/摘录/链接、逐项失败 | research包含 | 新报告ID；referenceId映射到导入资料 |
| 未采用Run输入/输出/用量、attempt状态摘要 | activity只读档案包含 | 不写wb_runs，不恢复Worker队列；原来源命名空间保留 |
| Production工作流快照/步骤、候选组排序 | activity只读档案包含 | 不创建Production/Candidate可执行任务 |
| 分支来源/目标说明 | activity只读manifest包含 | 不复制另一作品或递归分支；活跃分支通过独立fork创建 |
| 媒体封面/TTS任务和用量 | 只包含元数据 | 不恢复媒体队列；binary需单独素材下载或全库备份 |
| 项目预算偏好 | activity只读保留 | 不擅自启用旧费用/模型配置 |
| 用户、密码、会话、成员/邀请、在线状态 | 不包含 | 当前导入者成为新作品owner；管理员仍可管理 |
| 全局Provider/Prompt/Agent/Workflow/Embedding/MCP配置 | 不导入全局配置 | Activity保留实际Production流程/Run快照，不迁移机器端点/密钥 |
| Chunk/Embedding/调用缓存/检索摘要/事件投影 | 不作为权威迁移 | Chunk重建；Embedding等按新环境重新生成 |

作品包限32MiB，超限返回明确错误；大型作品、媒体素材、全量运行原始响应与机器配置应使用 [全库备份恢复](../../deploy/backup-restore.md)。作品JSON不是数据库备份。原始Provider响应不批量复制到activity，权威Run Trace仍在源数据库/全库备份；可读输入/输出和用量保留。

`GET /api/projects/:pid/archive`可读取当前与已导入的只读活动档案。导入不会触发模型、媒体、Embedding或MCP调用，即使历史状态标为Running。重复导入/再次导出仍保留原活动档案，不借用旧任务ID执行操作。

实测夹具含7类Studio当前文档、9个以上历史版本、参考来源与一个失败URL、已确认沙盒检查点、未采用analysis结果、Production、2候选组和成功媒体元数据。两轮导入后正文/Studio/Research均能读取；新作品`wb_runs/wb_productions/wb_candidate_groups/wb_media`均为0。另有来源/循环/引用伪造测试验证原子失败不留下半个作品。


## 0.4.0 写作指南迁移

formatVersion1新增可选writingSkills，旧作品包兼容。导出当前未删除Skill，含正文、参考资料、声明来源、用途、启停与优先级；导入原子校验后创建新ID/v1。真实PG往返确认内容和引用完整，路径伪造拒绝且不留半个作品。Skill历史/被删除版本及私有对话只随完整数据库备份保存。
