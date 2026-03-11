# 二手交易商家后台（Monorepo）

本仓库按 `docs/` 设计文档落地，包含：
- `frontend/`：React + TypeScript + Vite 管理端
- `backend/`：Go + Gin + GORM API 服务
- `docs/`：产品/接口/数据模型/里程碑文档

## 本期范围
- 商家注册、审核、登录
- 商品管理（草稿/上架/下架/锁定/成交/关闭）
- 轻量订单与商品状态联动
- 文件上传（presign/confirm）
- 审计日志查询

不含买家端、支付、退款、售后、营销。

## 快速启动

### 环境变量

后端（`backend/configs/.env.example`）：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `ADDR` | `:8080` | 服务监听地址 |
| `DB_DRIVER` | `sqlite` | 数据库驱动（当前默认 sqlite） |
| `DB_DSN` | `file:app.db?cache=shared&_foreign_keys=on` | 数据库连接串 |
| `JWT_ACCESS_SECRET` | `replace-access-secret` | Access Token 密钥 |
| `JWT_REFRESH_SECRET` | `replace-refresh-secret` | Refresh Token 密钥 |
| `AUTO_MIGRATE` | `true` | 启动时自动迁移 |
| `BUYER_WECHAT_LOGIN_MODE` | `mock` | 买家微信登录模式（`mock/real`） |
| `BUYER_WECHAT_APP_ID` | 空 | `real` 模式必填，微信 AppID |
| `BUYER_WECHAT_APP_SECRET` | 空 | `real` 模式必填，微信 AppSecret |
| `BUYER_WECHAT_CODE2SESSION_URL` | 微信官方地址 | `code2session` 请求地址 |
| `BUYER_WECHAT_HTTP_TIMEOUT_SECONDS` | `5` | 微信接口超时时间（秒） |

前端：

| 文件 | 变量 | 说明 |
| --- | --- | --- |
| `frontend/.env.development` | `VITE_API_BASE_URL=http://localhost:8080/api/v1` | 本地开发 API 地址 |
| `frontend/.env.production` | `VITE_API_BASE_URL=/api/v1` | 生产环境 API 前缀 |

买家小程序（`miniapp`）：

| 变量 | 示例 | 说明 |
| --- | --- | --- |
| `TARO_APP_API_BASE_URL` | `http://localhost:8080/api/v1` | 小程序 API 地址（真机不能用 localhost） |

### 后端
```bash
cd backend
GOPROXY=https://goproxy.cn,direct go mod tidy
CGO_ENABLED=0 go run ./cmd/server
```

说明：
- 在 macOS 未同意 Xcode License 时，`go run` 可能因为 cgo 失败；可用 `CGO_ENABLED=0` 启动。
- 如需真实微信登录，需额外设置 `BUYER_WECHAT_LOGIN_MODE=real` 与微信密钥变量。

默认管理员账号（初始化自动写入）：
- `admin / Admin@123456`
- `superadmin / Admin@123456`

### 前端
```bash
cd frontend
npm install
npm run dev
```

## 测试
```bash
make test
```

当前已覆盖：
- 状态机单元测试（`backend/internal/stateflow`）
- 主流程集成测试（`backend/tests/integration_flow_test.go`）

## 发布前回归命令

```bash
# 1) 后端回归
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build GOPROXY=https://proxy.golang.org,direct go test ./...

# 2) 前端回归
cd ../frontend
npm run test
npm run build

# 3) 冒烟（另开终端先启动后端）
cd ../backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build GOPROXY=https://proxy.golang.org,direct go run ./cmd/server

# 4) 执行主链路冒烟
cd ..
API_BASE_URL=http://localhost:8080/api/v1 node scripts/smoke-flow.mjs

# 5) 买家页面级冒烟（需要后端已启动）
API_BASE_URL=http://localhost:8080/api/v1 node scripts/smoke-miniapp-page-e2e.mjs
```

`smoke-miniapp-page-e2e.mjs` 前置条件：
- 后端 `/healthz` 可用。
- 如后端为 `BUYER_WECHAT_LOGIN_MODE=real`，需要额外传入 `BUYER_WECHAT_LOGIN_CODE=<wx.login 获取的临时 code>`。

## restricted login 边界

- `PENDING/REJECTED` 商家允许登录，返回 `token_scope=onboarding`。
- `onboarding` 仅可访问入驻流程接口（`merchant/profile`、`merchant/reapply`、资质上传）。
- 商品/订单/仪表盘/商家日志/账号设置对 `onboarding` 统一返回 `10006`。

## 文档索引

- [项目总览](docs/project-overview.md)
- [产品规格](docs/specs.md)
- [前端页面规划](docs/frontend-pages.md)
- [后端接口清单](docs/backend-api-checklist.md)
- [收口验收清单](docs/acceptance-checklist.md)
- [发布前就绪清单](docs/release-readiness.md)
