# 买家小程序发布前收口（miniapp-release-readiness）

更新时间：2026-03-12

## 1. 当前已完成能力
1. 买家页面与意向闭环：浏览、收藏、浏览记录、提交意向、状态回读。
2. 商家后台意向处理闭环：`NEW -> CONTACTED -> CLOSED`。
3. 页面级自动化冒烟脚本：`scripts/smoke-miniapp-page-e2e.mjs`。
4. 买家微信登录后端支持 `mock/real/disabled` 三种模式：
   - `BUYER_WECHAT_LOGIN_MODE=mock`：开发回归。
   - `BUYER_WECHAT_LOGIN_MODE=real`：调用微信 `code2session`。
   - `BUYER_WECHAT_LOGIN_MODE=disabled`：显式关闭微信登录。
5. 后端在当前机器可通过 `CGO_ENABLED=0` 启动（绕开 Xcode License 阻塞）。

## 2. 已自动验证通过项

## 2.1 本轮（2026-03-12）执行结果
1. `cd backend && CGO_ENABLED=0 ... go test ./...`：通过。
2. `cd miniapp && npm run test`：通过（4 files / 5 tests）。
3. `cd miniapp && npm run build:weapp`：通过。
4. `node --check scripts/smoke-miniapp-page-e2e.mjs`：通过。
5. 后端启动验证：`APP_ENV=development CGO_ENABLED=0 ... BUYER_WECHAT_LOGIN_MODE=mock go run ./cmd/server` + `/healthz`：通过。
6. 页面级 smoke：`API_BASE_URL=http://localhost:8080/api/v1 node scripts/smoke-miniapp-page-e2e.mjs`：通过。
7. `real` 模式行为验证（占位凭据）：
   - `APP_ENV=development ADDR=:8081 BUYER_WECHAT_LOGIN_MODE=real ... go run ./cmd/server`：启动通过。
   - 调用 `/buyer/auth/wechat-login`：返回 `10002 wechat login failed`（证明走到 real 分支，凭据无效）。

## 2.2 前序基线（已通过）
1. 商家前端 `test/build` 通过。
2. 小程序 `test/build:weapp` 通过。
3. `scripts/smoke-buyer-intent-flow.mjs` 通过。

## 3. 仍需人工验证项
1. 真实微信凭据下的真机登录（iOS + Android）。
2. 真机登录后的 guest merge 页面表现（收藏/浏览不丢失、不重复）。
3. 真机分享落地链路。
4. 真机弱网下登录失败与重试体验。

## 4. 真机联调阻塞项
1. **真实微信配置阻塞（环境/配置阻塞）**：测试/发布环境尚未确认注入有效 `BUYER_WECHAT_APP_ID`、`BUYER_WECHAT_APP_SECRET`。
2. **微信平台配置阻塞（环境阻塞）**：合法 request 域名、证书、微信后台配置未在本次验收中完成签收。
3. **真实真机验收阻塞（验证阻塞）**：尚未拿到真实 `wx.login code` 完成 `real` 模式全链路通过证据。

## 5. 发布判断

## 5.1 当前结论
**不可发布**。

## 5.2 不可发布原因
1. 真实微信登录虽已具备代码接入能力，但未完成真实凭据+真机链路验收。
2. 真机高风险项（分享落地、双端兼容、弱网）未完成最终签收。

## 5.3 达到可灰度/可发布的门槛
1. 完成 F12 买家身份迁移后，目标平台启用 `real` 并验证成功；迁移前两种平台保持 `disabled`，禁止用 `mock` 绕过。
2. iOS/Android 真机完成登录、merge、提交意向、状态回读全链路。
3. 真机分享落地通过。
4. 页面 smoke 与基础回归全部再跑一轮并保留日志证据。

## 6. 上线前最后一轮检查项
1. 后端环境变量核对：`APP_ENV=production`，微信/抖音的 `LOGIN_MODE/APP_ID/APP_SECRET` 与 F12 迁移状态一致。
2. 微信后台合法域名与证书核对。
3. 启动后端并健康检查：`/healthz`。
4. 执行页面 smoke：`scripts/smoke-miniapp-page-e2e.mjs`。
5. 执行真机清单：
   - 首页 -> 列表 -> 详情
   - 游客收藏/浏览
   - 登录后 guest merge
   - 提交意向
   - 商家处理后买家状态回读
   - 分享落地
6. 任一失败即判定不可发布。
