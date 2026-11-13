# 公开源码与发行

日期：2026-11-13。官方仓库：[ToStarLiu/yantai-novel](https://github.com/ToStarLiu/yantai-novel)。

- 原始代码和文档：GPL-3.0-only；[LICENSE](../../LICENSE)与[NOTICE.md](../../NOTICE.md)。第三方资产/依赖按[清单](../licenses.md)保留原始通知。
- 公开材料：中英文README、功能截图、贡献/安全/行为/支持说明、Bug/建议表单及PR模板。安全私密报告入口已启用。
- 本地上传前检查：461个索引文件，对本地真实私密配置值逐字节比对0命中；未包含.env、.local、.tools、node_modules、数据库或构建包。后续公开文件也需持续遵守排除规则。
- CI在临时PostgreSQL/Go/Node环境执行真实数据库与Pi工具回归，另验前端类型、单测和构建；[工作流说明](../development/ci.md)。云端是否通过以对应commit的[Actions](https://github.com/ToStarLiu/yantai-novel/actions)为准。
- 桌面二进制仅作为[Releases](https://github.com/ToStarLiu/yantai-novel/releases)附件，源码与版本标签对应，ZIP内携带项目及第三方许可。Windows包和Docker运行目录保留项目LICENSE/NOTICE。

文学效果、容器部署和其他未运行环境仍保留其验证边界；公开源码不改变作者数据归属，不包含开发时的私有追踪与备份。

首次公开ZIP的SHA-256：`85b0185e5ba4aedc4dd4610f1b9dc8f8f00cdedff13d40c5cbf9f6c0169ee25b`。1787个文件CRC通过；实际EXE两次启动、指南采用与退出数据库关闭均通过。Linux字体路径修复有独立回归测试，不通过跳过出版测试掩盖兼容问题。
