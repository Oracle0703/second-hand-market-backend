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
| `APP_ENV` | 无 | 必须显式设置为 `development`、`test` 或 `production` |
| `ADDR` | `:8080` | 服务监听地址 |
| `DB_DRIVER` | `mysql` | 数据库驱动（默认 mysql） |
| `DB_DSN` | `shm:Shm@123456@tcp(127.0.0.1:3306)/second_hand_market?...` | 数据库连接串（写入 shell 环境文件时建议加引号） |
| `JWT_ACCESS_SECRET` | `dev-access-secret` | Access Token 密钥；生产至少 32 字节 |
| `JWT_REFRESH_SECRET` | `dev-refresh-secret` | Refresh Token 密钥；生产至少 32 字节且不能与 Access 密钥相同 |
| `AUTO_MIGRATE` | `true` | 启动时自动迁移 |
| `FILE_STORAGE_PROVIDER` | `local` | 文件存储方式（当前支持 `local`，后续可扩展 OSS） |
| `FILE_UPLOAD_LOCAL_DIR` | `uploads` | 本地上传落盘目录 |
| `FILE_PUBLIC_BASE_URL` | 空 | 文件对外访问前缀；为空时默认 `/uploads` |
| `FILE_UPLOAD_MAX_MB` | `40` | 图片原图上传上限（MB） |
| `IMAGE_COMPRESS_TARGET_MB` | `20` | 服务端图片压缩目标大小（MB） |
| `IMAGE_PROCESSOR_DRIVER` | `vips` | 图片处理驱动（`vips/passthrough`） |
| `IMAGE_PROCESSOR_BIN` | `vips` | 图片处理命令路径 |
| `BUYER_WECHAT_LOGIN_MODE` | `mock` | 买家微信登录模式（`mock/real/disabled`） |
| `BUYER_WECHAT_APP_ID` | 空 | `real` 模式必填，微信 AppID |
| `BUYER_WECHAT_APP_SECRET` | 空 | `real` 模式必填，微信 AppSecret |
| `BUYER_WECHAT_CODE2SESSION_URL` | 微信官方地址 | `code2session` 请求地址 |
| `BUYER_WECHAT_HTTP_TIMEOUT_SECONDS` | `5` | 微信接口超时时间；`real` 模式允许 1–60 秒 |
| `BUYER_DOUYIN_LOGIN_MODE` | `mock` | 买家抖音登录模式（`mock/real/disabled`） |
| `BUYER_DOUYIN_APP_ID` | 空 | `real` 模式必填，抖音小程序 AppID |
| `BUYER_DOUYIN_APP_SECRET` | 空 | `real` 模式必填，抖音小程序 AppSecret |
| `BUYER_DOUYIN_CODE2SESSION_URL` | 抖音官方地址 | `code2session` 请求地址 |
| `BUYER_DOUYIN_HTTP_TIMEOUT_SECONDS` | `5` | 抖音接口超时时间；`real` 模式允许 1–60 秒 |

生产（MySQL）可参考：`backend/configs/.env.production.mysql.example`  
生产（SQLite，仅临时/单机）可参考：`backend/configs/.env.production.sqlite.example`

前端：

| 文件 | 变量 | 说明 |
| --- | --- | --- |
| `frontend/.env.development` | `VITE_API_BASE_URL=/api/v1` | 本地开发 API 前缀（由 Vite 代理到后端，避免 CORS） |
| `frontend/.env.development` | `VITE_API_PROXY_TARGET=http://localhost:8080` | 本地开发代理目标地址（后端端口变化时修改） |
| `frontend/.env.production` | `VITE_API_BASE_URL=/api/v1` | 生产环境 API 前缀 |

买家小程序（`miniapp`）：

| 变量 | 示例 | 说明 |
| --- | --- | --- |
| `TARO_APP_API_BASE_URL` | `http://localhost:8080/api/v1` | 小程序 API 地址（真机不能用 localhost） |

生产网关约定：
- 统一对外暴露 `/api/v1/*`，不要再额外挂一层 `/api`，否则会出现 `/api/api/v1/*`。
- 管理端生产环境保持 `frontend/.env.production` 中的 `VITE_API_BASE_URL=/api/v1`。
- 小程序生产环境保持 `TARO_APP_API_BASE_URL=https://<你的域名>/api/v1`。

Nginx 示例：
```nginx
server {
    server_name market.meaningful.ink;

    location /api/v1/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

说明：
- 后端服务自身已经注册 `/api/v1/*` 路由，因此 `proxy_pass` 指向后端根地址即可，不要改成 `http://127.0.0.1:8080/api/`。
- 如果线上当前存在 `/api/api/v1/*`，需要同步调整 Nginx 或网关规则后再重新发布前端和小程序。

微信构建：
```bash
cd miniapp
npm install
npm run dev:weapp
```

抖音构建：
```bash
cd miniapp
npm install
npm run dev:tt
```

### 后端
```bash
cd backend
GOPROXY=https://goproxy.cn,direct go mod tidy
APP_ENV=development CGO_ENABLED=0 go run ./cmd/server
```

生产环境（SQLite 示例）：
```bash
cd backend
cp configs/.env.production.sqlite.example .env
# 修改被 .gitignore 排除的 .env：替换 DB_DSN 和两枚 JWT 密钥；F12 完成前保持登录 disabled
set -a && source .env && set +a
CGO_ENABLED=0 go run ./cmd/server
```

生产环境（MySQL 推荐）：
```bash
cd backend
cp configs/.env.production.mysql.example .env
# 修改被 .gitignore 排除的 .env：替换 DB_DSN 和两枚 JWT 密钥；F12 完成前保持登录 disabled
set -a && source .env && set +a
CGO_ENABLED=0 go run ./cmd/server
```

说明：
- 在 macOS 未同意 Xcode License 时，`go run` 可能因为 cgo 失败；可用 `CGO_ENABLED=0` 启动。
- `APP_ENV` 缺失或拼写错误时服务拒绝启动；Docker 镜像默认使用 `production`。
- 生产环境拒绝两种平台的 `mock` 登录；`disabled` 可作为安全过渡，启用平台时再切为 `real`。
- 现有 mock 买家完成后续 F12 身份迁移前，生产模板默认关闭两种登录，避免真实 OpenID 新建出重复账号。
- `real` 模式必须配置对应 AppID、AppSecret、1–60 秒超时和代码内固定的官方 HTTPS 换码地址。
- 两枚生产 JWT 密钥必须独立生成、至少 32 字节；启动检查会拒绝示例值及明显低多样性或重复模式，但不能替代安全随机生成。可分别执行 `openssl rand -base64 48`。
- 图片上传后会落盘到 `FILE_UPLOAD_LOCAL_DIR`，并通过 `/uploads/<object_key>` 提供访问（可通过 `FILE_PUBLIC_BASE_URL` 切换到 CDN/OSS 域名）。
- 当前支持 `jpg/jpeg/png/webp/heic/heif`，苹果 `Live Photo` 仅支持其中静态图，不支持配套 `mov`。
- 原图大小上限为 `40MB`；后端会统一压缩，优先将图片控制在 `20MB` 内。
- 本地如未安装 `vips/libheif`，可临时设置 `IMAGE_PROCESSOR_DRIVER=passthrough`；生产 Docker 应使用 `vips`。

### 后端 Docker
```bash
docker build -f backend/Dockerfile -t second-hand-market-backend .
```

运行容器时应由部署环境注入不带 shell 引号的环境变量，并为上传目录（以及使用 SQLite 时的数据库目录）挂载持久卷。具体运行参数留到独立部署分支收口，避免把可供 `source` 的 `.env` 直接交给 Docker `--env-file`。

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
APP_ENV=development GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build GOPROXY=https://proxy.golang.org,direct go run ./cmd/server

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
- [抖音小程序构建与排障](docs/miniapp-douyin-build-troubleshooting.md)
