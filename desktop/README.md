# 砚台桌面客户端

Windows x64 便携包自带 API、PostgreSQL、中文字体、Node.js 与 Pi harness。解压后启动 `Yantai.exe`，进入书房；不需要另装数据库或 Node.js。模型默认是用于体验流程的演示模型，在 **更多 → AI 供应商** 中配置自己的服务。

## 数据与升级

作品、账号、供应商配置和作者记忆保存在独立的 Electron 用户数据目录下的 `workbench` 中，可从应用菜单“打开数据与模型配置目录”查看。程序目录不保存作者数据库。供应商密钥加密存储，对应主密钥为数据目录内的 `provider-master.key`；`model.env` 保留为兼容入口。

升级前完全退出旧版，确认数据库已关闭，再备份整个数据目录；数据库与对应的 `provider-master.key` 必须一起保存。将新版解压到新目录后启动，默认沿用原用户数据目录并应用数据库迁移。不要用发行包内容覆盖、清空或替换自己的数据目录；迁移后的库不能直接交给旧版程序降级使用。诊断时可通过 `NOVEL_DESKTOP_DATA` 指定单独的数据目录，隔离测试数据。

## 源码开发

先运行仓库根目录的 `scripts/start.ps1`，再在另一个终端执行：

```powershell
. ./scripts/dev.ps1 -Action env
npm --prefix desktop ci
npm --prefix desktop start
```

`NOVEL_DESKTOP_URL` 可指定已有的自托管 HTTP(S) 服务。渲染器禁用 Node，启用 sandbox、contextIsolation 和 webSecurity；外链交给系统浏览器打开。打包版本缺少内置 API 时会明确停止，不连接到可能属于另一实例的 8088 服务。

## 构建便携包

当前构建入口支持 Windows x64 主机，需要项目内固定版本工具链与 npm 依赖。先完成 Web 和 Pi 构建，再打包：

```powershell
. ./scripts/dev.ps1 -Action env
npm --prefix web ci
npm --prefix harness ci
npm --prefix desktop ci
npm --prefix web run build
npm --prefix harness run build
npm --prefix harness run bundle
npm --prefix desktop run package
node desktop/verify-package.mjs --require-clean
```

正式发行前应先提交最终源码与版本变更，确保桌面、Web、harness 三个 package.json 版本一致，且工作区干净。打包脚本会自行从当前源码构建 Go API，不复制开发目录中的旧可执行文件。清理和覆盖只针对已校验的生成目录 `desktop/resources/backend` 与 `desktop/dist/Yantai-win32-x64`，遇到符号链接或目录异常会停止。打包期间不要修改或提交源码。

输出目录为 `desktop/dist/Yantai-win32-x64`。桌面 shell 仅打入 package.json、main.cjs 和 backend.cjs，运行资源按明确列表复制，不包括工作区 .env、.local、作者数据库、登录信息或供应商主密钥。包内保留项目 LICENSE/NOTICE、Pi 实际 bundle 依赖、Go/Web、Node.js、Electron/Chromium、PostgreSQL 和字体的许可证。

包内 `resources/backend/build-info.json` 记录版本、源码提交与树、是否存在未提交改动以及运行资源 SHA-256 清单。打包完成后会自动检查 ASAR 版本、shell 来源哈希、必需资源、全部 backend 文件哈希和私有路径，并在 `desktop/dist/package-verification.json` 留下回执；独立验证命令还会核对当前源码树，`--require-clean` 拒绝由未提交工作区构建的包。合并提交与构建提交可以不同，但发布标签必须指向相同源码树。

静态验证不代替真实桌面启动：发布仍需在独立数据目录验证首次启动、完整工作流、退出后数据库关闭、重启数据保留，以及最终 ZIP 的 CRC、SHA-256 和下载结果。

发行 ZIP 可在正式干净构建后运行 `python scripts/package-release.py`（Python 3.11+，仅标准库）。脚本拒绝已有同名压缩包，校验完整目录没有私有运行数据，完成 ZIP CRC 检查后生成版本化 ZIP、`release.json` 和 `SHA256SUMS.txt`；上传前再次核对源码树与校验值。
