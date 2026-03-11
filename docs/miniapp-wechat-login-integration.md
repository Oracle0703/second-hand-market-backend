# 买家小程序真实微信登录接入说明（miniapp-wechat-login-integration）

更新时间：2026-03-11

## 1. 当前状态与阻塞结论

## 1.1 当前代码能力
后端 `POST /api/v1/buyer/auth/wechat-login` 已支持两种模式：
1. `mock`：`openid = mock_wx_<code>`（用于本地开发与自动化）。
2. `real`：调用微信 `code2session` 获取 `openid/unionid`。

相关配置入口：`backend/internal/app/config.go`。

## 1.2 当前仍存在的发布阻塞
即使代码已支持 `real`，以下任一项缺失仍会阻塞发布：
1. 未提供有效 `BUYER_WECHAT_APP_ID` / `BUYER_WECHAT_APP_SECRET`。
2. 小程序后台未配置合法 request 域名（HTTPS）。
3. 真机未完成 `wx.login -> backend -> code2session -> buyer session` 全链路验收。

---

## 2. 接入所需配置 / 密钥 / 平台项

## 2.1 后端环境变量（必填/可选）

| 变量 | 必填 | 示例 | 说明 |
| --- | --- | --- | --- |
| `BUYER_WECHAT_LOGIN_MODE` | 是 | `real` / `mock` | 登录模式，发布前必须切到 `real` |
| `BUYER_WECHAT_APP_ID` | `real` 模式必填 | `wx123...` | 微信小程序 AppID |
| `BUYER_WECHAT_APP_SECRET` | `real` 模式必填 | `abcdef...` | 微信小程序 AppSecret |
| `BUYER_WECHAT_CODE2SESSION_URL` | 否 | `https://api.weixin.qq.com/sns/jscode2session` | 微信 code2session 地址 |
| `BUYER_WECHAT_HTTP_TIMEOUT_SECONDS` | 否 | `5` | 后端请求微信超时 |

说明：`mock` 模式下 `APP_ID/APP_SECRET` 可为空；`real` 模式下为空会直接返回配置错误。

## 2.2 微信公众平台配置（必做）
1. 申请并确认小程序 `AppID` / `AppSecret`。
2. 在小程序后台配置服务器域名（request 合法域名），要求 HTTPS。
3. 准备至少 2 个可登录测试微信号（避免单账号缓存/态污染）。
4. 若依赖 unionid，需确认开放平台绑定策略。

## 2.3 小程序端配置（必做）
1. `TARO_APP_API_BASE_URL` 指向可被微信客户端访问的 HTTPS API。
2. 真机调试时禁止使用 `localhost`。

---

## 3. 本地 / 测试 / 真机验证方式

## 3.1 本地验证（开发机）

### 路径 A：mock 验证（当前默认）
用途：验证业务流程，不验证微信真实性。

```bash
# 后端
cd backend
CGO_ENABLED=0 GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build \
BUYER_WECHAT_LOGIN_MODE=mock \
go run ./cmd/server

# 小程序
cd ../miniapp
TARO_APP_API_BASE_URL=http://<你的电脑IP>:8080/api/v1 npm run dev:weapp
```

验收点：登录成功、guest merge 成功、意向可提交。

### 路径 B：real 验证（本地连接微信）
用途：验证真实 `code2session`。

```bash
cd backend
CGO_ENABLED=0 GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build \
BUYER_WECHAT_LOGIN_MODE=real \
BUYER_WECHAT_APP_ID=<真实appid> \
BUYER_WECHAT_APP_SECRET=<真实secret> \
go run ./cmd/server
```

前提：
1. 开发机可访问公网微信接口。
2. 小程序使用与 `AppID` 对应项目。
3. API 域名已在微信后台合法域名白名单中。

## 3.2 测试环境验证（推荐）
1. 测试后端设置 `BUYER_WECHAT_LOGIN_MODE=real`。
2. 注入测试环境 `APP_ID/APP_SECRET`。
3. 通过微信开发者工具 + 真机预览进行登录。
4. 验证接口：
   - `POST /api/v1/buyer/auth/wechat-login`
   - `POST /api/v1/buyer/guest/merge`
   - `GET /api/v1/buyer/me/summary`

验收通过标准：
1. 首次登录创建 buyer。
2. 同微信重复登录命中同一 buyer（openid 一致）。
3. 登录后游客收藏/浏览记录被合并。

## 3.3 真机验证（发布门禁）
1. iOS 与 Android 各完成 1 次完整登录链路。
2. 登录后提交意向并检查状态回读。
3. 分享后落地详情页，再次触发登录流程无异常。
4. 弱网场景下登录失败提示可恢复。

---

## 4. smoke 与自动化联调建议

## 4.1 页面 smoke 脚本
`scripts/smoke-miniapp-page-e2e.mjs` 默认使用 mock code。

当后端为 `real` 模式时：
1. 需要传入真实微信登录 code 才能通过登录步骤。
2. 该 code 需由 `wx.login` 即时获取，短时有效且一次性。

建议：
1. 流程自动化仍在 `mock` 模式跑全链路。
2. 真实登录在真机手工验收中单独签收。

---

## 5. 上线前必须完成 vs 可灰度后完成

## 5.1 必须在上线前完成（阻塞发布）
1. `BUYER_WECHAT_LOGIN_MODE=real` 在目标发布环境生效。
2. `APP_ID/APP_SECRET` 配置正确，`code2session` 可用。
3. 真机双端（iOS/Android）登录链路验收通过。
4. 登录后 guest merge 数据正确（收藏/浏览不丢失、不重复）。

## 5.2 可灰度后完成（不阻塞首发）
1. unionid 相关扩展策略（如跨端账号体系扩展）。
2. 登录失败细分文案优化。
3. 更细粒度登录链路监控看板。

---

## 6. 执行清单（工程可执行）
1. 配置负责人提供 `APP_ID/APP_SECRET` 到测试与生产环境。
2. 后端环境变量切为 `BUYER_WECHAT_LOGIN_MODE=real`。
3. 运维确认 API 域名 HTTPS 与微信合法域名配置完成。
4. QA 按真机清单完成 iOS/Android 登录与意向闭环验收。
5. 发布前复核 `docs/miniapp-release-readiness.md` 阻塞项全部清零。
