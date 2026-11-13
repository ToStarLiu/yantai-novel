# 砚台桌面客户端

Windows便携包自带API、PostgreSQL、中文字体、Node.js与Pi harness，启动`dist/Yantai-win32-x64/Yantai.exe`即可进入书房。作品和随机账户保存在独立用户数据目录，升级保留原数据。直接在“AI供应商”页面配置接口和模型；密钥加密存储，本机主密钥位于同一数据目录的`provider-master.key`。`model.env`保留为兼容入口。

开发时可先运行`scripts/start.ps1`，再在desktop目录`npm ci && npm start`；`NOVEL_DESKTOP_URL`可指定自托管HTTP(S)服务。构建便携包前先构建Go API到`.tools/api-advanced-next.exe`、Web生产目录及`harness/bundle`，再执行`npm run package`。包内保留Pi及实际打入bundle依赖的许可证、Node发行许可、Electron/Chromium、PostgreSQL和字体声明。

渲染器禁Node、启用sandbox/contextIsolation，外链由系统浏览器打开。备份桌面实例时同时保留数据库和对应`provider-master.key`，否则无法解密页面保存的供应商凭据。
