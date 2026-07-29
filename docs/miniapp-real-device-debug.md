# 买家小程序真机联调说明（miniapp-real-device-debug）

更新时间：2026-03-11

## 1. 目标与范围
本说明只覆盖本阶段目标：真机联调准备、环境切换、页面链路验收与发布前收口。

不在本说明范围：支付、退款、IM、推荐、购物车等新业务扩展。

---

## 2. 联调前提与环境变量

### 2.1 后端环境变量（`backend`）
后端由 `backend/internal/app/config.go` 读取以下变量：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `APP_ENV` | 无（必填） | 运行环境：`development/test/production` |
| `ADDR` | `:8080` | 服务监听地址 |
| `DB_DRIVER` | `mysql` | 数据库驱动 |
| `DB_DSN` | 本机 MySQL `second_hand_market` 连接串 | 数据库连接串 |
| `JWT_ACCESS_SECRET` | `dev-access-secret` | access token 密钥 |
| `JWT_REFRESH_SECRET` | `dev-refresh-secret` | refresh token 密钥 |
| `ACCESS_TTL_SECONDS` | `7200`（2h） | access token 时效 |
| `REFRESH_TTL_SECONDS` | `604800`（7d） | refresh token 时效 |
| `AUTO_MIGRATE` | `true` | 启动时自动迁移 |

### 2.2 小程序构建变量（`miniapp`）
小程序请求层读取：`process.env.TARO_APP_API_BASE_URL`（`miniapp/src/services/request.ts`）。

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `TARO_APP_API_BASE_URL` | `http://localhost:8080/api/v1` | 小程序 API 基础地址 |

说明：
1. 若不设置该变量，构建产物会固化默认地址 `http://localhost:8080/api/v1`。
2. 真机调试时，`localhost` 指向手机本机，通常不可访问电脑后端。
3. 真机联调建议显式设置为可达地址（内网 IP 或测试域名）。

---

## 3. 微信登录真实联调前提（appid / secret / 配置）

## 3.1 当前代码现状（阻塞项）
当前后端 `POST /api/v1/buyer/auth/wechat-login` 支持三种模式：
1. `BUYER_WECHAT_LOGIN_MODE=mock`：开发态 mock openid。
2. `BUYER_WECHAT_LOGIN_MODE=real`：调用微信 `code2session` 获取 `openid/unionid`。
3. `BUYER_WECHAT_LOGIN_MODE=disabled`：显式关闭微信登录。

结论：代码层已具备真实接入能力，但**真实联调仍依赖 AppID/AppSecret、合法域名和真机验收完成**。

## 3.2 真实微信登录必备条件
要做真实联调，至少满足：
1. 微信小程序 `AppID` 已创建且团队可用。
2. 后端持有对应 `AppSecret`，并启用 `BUYER_WECHAT_LOGIN_MODE=real`。
3. 小程序后台已配置 `request 合法域名`（测试/生产域名）。
4. API 网关与后端 HTTPS 证书可被微信客户端接受。
5. 测试账号（买家、商家）与测试商品数据已准备。

## 3.3 本地/测试如何验证登录
当前阶段按两层验证：
1. 流程验证（可做）：小程序点“授权登录”后，`/buyer/auth/wechat-login` 成功、`/buyer/guest/merge` 成功、`/buyer/me/summary` 显示登录态。
2. 真实性验证（发布前必做）：`BUYER_WECHAT_LOGIN_MODE=real` 下，微信真实 `code2session` 返回 `openid/unionid` 链路验证。

---

## 4. 本地与测试环境 API Base 切换

## 4.1 本地后端联调

```bash
# 终端 A：启动后端
cd backend
mkdir -p .cache/go/mod .cache/go/build
APP_ENV=development GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go run ./cmd/server

# 终端 B：启动小程序 watch 构建（示例使用本机 8080）
cd miniapp
TARO_APP_API_BASE_URL=http://127.0.0.1:8080/api/v1 npm run dev:weapp
```

说明：
1. 仅模拟器联调可直接用 `127.0.0.1`。
2. 真机调试时请改为电脑局域网 IP（例如 `http://192.168.x.x:8080/api/v1`）或测试 HTTPS 域名。

## 4.2 测试环境联调

```bash
cd miniapp
TARO_APP_API_BASE_URL=https://test-api.example.com/api/v1 npm run dev:weapp
```

建议：
1. 使用测试环境域名时，同步在小程序后台配置合法域名。
2. 若走临时联调域名，开发工具需开启“调试时不校验域名/TLS/HTTPS 证书”（仅开发验证使用）。

---

## 5. 小程序构建产物与调试入口

## 5.1 构建产物

```bash
cd miniapp
npm run build:weapp
```

产物目录：`miniapp/dist`。

## 5.2 调试入口
1. 微信开发者工具导入项目根目录 `miniapp`（以 Taro 工程方式）或直接导入 `miniapp/dist`（以小程序源码方式）。
2. 勾选编译后可在模拟器 / 真机预览打开页面。
3. 关键路由：
   - 首页：`pages/home/index`
   - 列表：`pages/product/list/index`
   - 详情：`pages/product/detail/index?id=<product_id>`
   - 收藏：`pages/favorite/index`
   - 浏览记录：`pages/history/index`
   - 登录：`pages/login/index`
   - 意向提交：`pages/intent/create/index?product_id=<product_id>`
   - 我的意向：`pages/intent/list/index`

---

## 6. 真机可测项 vs 仅正式环境可测项

## 6.1 当前真机可测（本仓库即可）
1. 首页 -> 列表 -> 详情的浏览链路。
2. 游客收藏与浏览记录生成。
3. 登录按钮触发与会话写入。
4. guest merge 后收藏/浏览归并。
5. 登录后提交意向、我的意向列表状态展示。
6. 商家处理意向后，买家端状态回读（`处理中/已联系/已关闭`）。

## 6.2 仅正式环境或真实微信接入后可测
1. 真实微信 `code2session` 与 `openid/unionid` 一致性。
2. 合法域名/证书在微信客户端强校验下的稳定性。
3. 分享卡片在正式包内的落地质量（标题、封面、打开路径、参数保持）。
4. 多机型（iOS/Android）与弱网场景下的登录授权稳定性。

---

## 7. 本阶段结论
1. 真机联调基础（启动、切环境、路由入口、验收链路）已具备。
2. 当前高风险阻塞转为“真实微信配置与真机签收未完成”，需在发布判断中单列为前置检查项。
