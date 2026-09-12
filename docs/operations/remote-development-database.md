# Windows 远程开发数据库

此流程仅用于 Windows 本地开发。数据库身份校验通过前，API 不会启动；远程开发配置固定关闭自动迁移和默认数据写入。

```powershell
Copy-Item backend\configs\.env.remote-dev.example backend\.env.remote-dev
# 编辑 backend\.env.remote-dev，替换 DB_DSN 中的密码和 DB_EXPECTED_SERVER_UUID

.\scripts\remote-dev\start-tunnel.ps1
.\scripts\remote-dev\test-database.ps1
.\scripts\remote-dev\start-api.ps1
```

结束后可运行：

```powershell
.\scripts\remote-dev\stop-tunnel.ps1
```

管理端继续使用现有 Vite `/api/v1` 代理。小程序模拟器启动时显式设置：

```powershell
$env:TARO_APP_API_BASE_URL = 'http://127.0.0.1:8080/api/v1'
cd miniapp
npm run dev:weapp
```

真机中的 `localhost` 或 `127.0.0.1` 指向手机自身，不能访问电脑上的 API；真机调试应改用电脑局域网地址或可访问的测试 HTTPS 地址。
