# 二手交易商家后台（Monorepo）

本仓库按 `docs/` 设计文档落地，包含：
- `frontend/`：React + TypeScript + Vite 管理端
- `backend/`：Go + Gin + GORM API 服务
- `docs/`：产品/接口/数据模型/里程碑文档

## 本期范围
- 商家注册、审核、登录
- 商品管理（草稿/上架/下架/多库存/成交/关闭）
- 商家轻量订单（数量、单件成交价、自动总价与库存预占）
- 文件上传（presign/confirm）
- 审计日志查询

不含买家端、支付、退款、售后、营销。

## 快速启动

### 环境变量

后端（`backend/configs/.env.example`）：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ENV` | `development` | 运行环境；生产必须显式设置为 `production` |
| `ADDR` | `:8080` | 服务监听地址 |
| `DB_DRIVER` | `mysql` | 数据库驱动（默认 mysql） |
| `DB_DSN` | `'shm:Shm@123456@tcp(127.0.0.1:3306)/second_hand_market?...'` | 数据库连接串（建议带引号，便于 `source`） |
| `JWT_ACCESS_SECRET` | `replace-access-secret` | Access Token 密钥 |
| `JWT_REFRESH_SECRET` | `replace-refresh-secret` | Refresh Token 密钥 |
| `AUTO_MIGRATE` | `true` | 启动时自动迁移 |
| `FILE_STORAGE_PROVIDER` | `local` | 文件存储方式（当前支持 `local`，后续可扩展 OSS） |
| `FILE_UPLOAD_LOCAL_DIR` | `uploads` | 本地上传落盘目录 |
| `FILE_PUBLIC_BASE_URL` | 空 | 文件对外访问前缀；为空时默认 `/uploads` |
| `FILE_UPLOAD_MAX_MB` | `10` | 单文件业务上限，按 MiB 计算（10,485,760 bytes） |
| `FILE_UPLOAD_MULTIPART_MAX_MB` | `11` | multipart 请求体上限，按 MiB 计算（11,534,336 bytes） |
| `FILE_UPLOAD_IP_HASH_SECRET` | 无 | 匿名来源 HMAC-SHA256 密钥；生产必须显式设置且至少 32 bytes |
| `FILE_UPLOAD_ANON_PRESIGN_PER_HOUR` | `20` | 每个匿名来源每滚动一小时成功 presign 上限 |
| `FILE_UPLOAD_ANON_ACTIVE_FILES` | `5` | 每个匿名来源活跃未绑定文件数上限 |
| `FILE_UPLOAD_ANON_ACTIVE_MB` | `50` | 每个匿名来源活跃未绑定字节上限（MiB） |
| `FILE_UPLOAD_MERCHANT_QUOTA_MB` | `2048` | 每个商家文件记录配额（MiB） |
| `FILE_UPLOAD_GLOBAL_QUOTA_MB` | `20480` | 全部文件记录配额（MiB） |
| `FILE_UPLOAD_CLEANUP_INTERVAL_SECONDS` | `300` | 匿名孤儿文件清理周期 |
| `FILE_UPLOAD_CLEANUP_BATCH_SIZE` | `50` | 单批最多 claim 的清理记录数 |
| `FILE_UPLOAD_CLEANUP_CLAIM_TTL_SECONDS` | `600` | 崩溃 claim 可重试时间 |
| `FILE_UPLOAD_CLEANUP_GRACE_SECONDS` | `1800` | capability 过期后的最小清理宽限期 |
| `TRUSTED_PROXY_CIDRS` | `none` | 可信代理 CIDR；`none` 表示只信任直连 peer |
| `IMAGE_COMPRESS_TARGET_MB` | `20` | 服务端图片压缩目标大小（MB） |
| `IMAGE_PROCESSOR_DRIVER` | `vips` | 图片处理驱动（`vips/passthrough`） |
| `IMAGE_PROCESSOR_BIN` | `vips` | 图片处理命令路径 |
| `BUYER_WECHAT_LOGIN_MODE` | `mock` | 买家微信登录模式（`mock/real`） |
| `BUYER_WECHAT_APP_ID` | 空 | `real` 模式必填，微信 AppID |
| `BUYER_WECHAT_APP_SECRET` | 空 | `real` 模式必填，微信 AppSecret |
| `BUYER_WECHAT_CODE2SESSION_URL` | 微信官方地址 | `code2session` 请求地址 |
| `BUYER_WECHAT_HTTP_TIMEOUT_SECONDS` | `5` | 微信接口超时时间（秒） |
| `BUYER_DOUYIN_LOGIN_MODE` | `mock` | 买家抖音登录模式（`mock/real`） |
| `BUYER_DOUYIN_APP_ID` | 空 | `real` 模式必填，抖音小程序 AppID |
| `BUYER_DOUYIN_APP_SECRET` | 空 | `real` 模式必填，抖音小程序 AppSecret |
| `BUYER_DOUYIN_CODE2SESSION_URL` | 抖音官方地址 | `code2session` 请求地址 |
| `BUYER_DOUYIN_HTTP_TIMEOUT_SECONDS` | `5` | 抖音接口超时时间（秒） |

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

    client_max_body_size 11m;
    error_page 413 = @upload_too_large;

    location /api/v1/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location @upload_too_large {
        internal;
        default_type application/json;
        return 413 '{"code":10008,"message":"upload file too large","request_id":"$request_id"}';
    }
}
```

说明：
- 后端服务自身已经注册 `/api/v1/*` 路由，因此 `proxy_pass` 指向后端根地址即可，不要改成 `http://127.0.0.1:8080/api/`。
- 如果线上当前存在 `/api/api/v1/*`，需要同步调整 Nginx 或网关规则后再重新发布前端和小程序。
- 两层生产 Nginx 都必须使用 11 MiB request-body 上限和 JSON 413；用户可见的文件上限仍是 10 MiB。生产代理变更尚未执行。

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
CGO_ENABLED=0 go run ./cmd/server
```

生产环境（SQLite 示例）：
```bash
cd backend
cp configs/.env.production.sqlite.example .env.production
# 修改 .env.production 里的 JWT 密钥与 DB_DSN 路径
set -a && source .env.production && set +a
CGO_ENABLED=0 go run ./cmd/server
```

生产环境（MySQL 推荐）：
```bash
cd backend
cp configs/.env.production.mysql.example .env.production
# 修改 .env.production 里的 DB_DSN 和 JWT 密钥
set -a && source .env.production && set +a
CGO_ENABLED=0 go run ./cmd/server
```

说明：
- 在 macOS 未同意 Xcode License 时，`go run` 可能因为 cgo 失败；可用 `CGO_ENABLED=0` 启动。
- 如需真实微信登录，需额外设置 `BUYER_WECHAT_LOGIN_MODE=real` 与微信密钥变量。
- 如需真实抖音登录，需额外设置 `BUYER_DOUYIN_LOGIN_MODE=real` 与抖音密钥变量。
- 图片上传后会落盘到 `FILE_UPLOAD_LOCAL_DIR`；只有已通过检查的商品图片允许通过 `/uploads/<object_key>` 公开访问，营业执照只能走管理员鉴权内容接口。
- 当前支持 `jpg/jpeg/png/webp/heic/heif`，苹果 `Live Photo` 仅支持其中静态图，不支持配套 `mov`。
- 原图业务上限为 10 MiB；presign、multipart 文件头、实际读取和处理结果执行同一字节边界，multipart 请求体上限为 11 MiB。
- 匿名 presign 使用可信来源 IP 的 HMAC 标识，并由数据库 guard 串行执行 20 次/小时、5 个活跃文件、50 MiB 活跃字节限制；商家和全局配额分别为 2 GiB 与 20 GiB。
- 自动清理仅处理迁移后创建、匿名、未绑定且 `cleanup_after` 到期的记录；迁移前记录、认证上传和已绑定文件不会进入候选集合。
- 本地如未安装 `vips/libheif`，可临时设置 `IMAGE_PROCESSOR_DRIVER=passthrough`；生产 Docker 应使用 `vips`。

### 后端 Docker
```bash
docker build -f backend/Dockerfile -t second-hand-market-backend .
docker run --rm -p 8080:8080 --env-file backend/configs/.env.production.mysql.example second-hand-market-backend
```

管理员不会在服务启动时自动创建。首次部署应先完成数据库迁移，再通过受控密码文件逐个创建所需账号：

```bash
cd backend
DB_DRIVER=mysql \
DB_DSN='<通过部署 secret 注入>' \
ADMIN_BOOTSTRAP_USERNAME='<管理员账号>' \
ADMIN_BOOTSTRAP_DISPLAY_NAME='<显示名>' \
ADMIN_BOOTSTRAP_ROLE=SUPER_ADMIN \
ADMIN_BOOTSTRAP_PASSWORD_FILE='<权限为 0600 的密码文件>' \
go run ./scripts/bootstrap_admin
```

脚本不回显密码、不覆盖已有管理员，也不自行执行迁移。`APP_ENV=production` 时，默认 JWT secret、示例 MySQL DSN 或空管理员库都会使服务拒绝启动。已有管理员通过后台“安全设置”自助改密，成功后旧 session 立即失效。

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

F-06 的隔离 MySQL 8.4 验收必须使用专用路径和 Compose project，并在取得单独书面授权后运行：

```bash
ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-anonymous-upload-governance-smoke
```

该命令不得指向生产数据库或生产上传目录；当前状态为代码侧已修复、测试服务器未审核、生产未执行 `0008` 且未部署。

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
SMOKE_ADMIN_USERNAME="$ADMIN_USERNAME" \
SMOKE_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
API_BASE_URL=http://localhost:8080/api/v1 \
node scripts/smoke-flow.mjs

# 5) 买家页面级冒烟（需要后端已启动）
SMOKE_ADMIN_USERNAME="$ADMIN_USERNAME" \
SMOKE_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
API_BASE_URL=http://localhost:8080/api/v1 \
node scripts/smoke-miniapp-page-e2e.mjs
```

MySQL 并发验收冒烟会写入持久测试行，只能在隔离环境运行，绝不能指向生产环境：

```bash
ACCEPTANCE_DB_ENGINE=mysql8.4 \
ACCEPTANCE_CONFIRM_ISOLATED=I_UNDERSTAND_THIS_WRITES_TEST_DATA \
SMOKE_ADMIN_USERNAME="$SMOKE_ADMIN_USERNAME" \
SMOKE_ADMIN_PASSWORD="$SMOKE_ADMIN_PASSWORD" \
API_BASE_URL=http://127.0.0.1:18082/api/v1 \
make acceptance-mysql-smoke
```

`smoke-miniapp-page-e2e.mjs` 前置条件：
- 后端 `/healthz` 可用。
- `ADMIN_USERNAME` / `ADMIN_PASSWORD` 由本地或 CI secret 环境注入；冒烟脚本不再包含固定管理员口令。
- 脚本会创建商家、商品和订单测试数据，只在本地或隔离的非生产环境执行；生产写验证遵循下方的受控专用测试商品流程，不运行此脚本。
- 如后端为 `BUYER_WECHAT_LOGIN_MODE=real`，需要额外传入 `BUYER_WECHAT_LOGIN_CODE=<wx.login 获取的临时 code>`。

## Production multi-stock release boundary

Isolated acceptance on a production-data clone passed MySQL 8.4.8 migration, index, CHECK, AutoMigrate compatibility, concurrency, administrator security, and desktop/mobile browser checks. `frontend npm run build` and `frontend npm test` pass on the current branch. Production migration, deployment, administrator rotation, and real production write verification remain undone.

The production maintenance window must use this exact order:

```text
recoverable backup evidence
-> protected yaner fingerprint
-> 0004 preflight
-> 0004 up migration exactly once
-> 0004 postflight
-> 0005 file_records preflight
-> 0005 file_records up migration exactly once
-> 0005 file_records postflight
-> deploy API and admin frontend together
-> health/auth/read checks
-> controlled dedicated test product create/close/complete
-> protected yaner fingerprint comparison
-> 30-60 minute observation
```

The canonical multi-stock gate files are:

- `backend/migrations/0004_merchant_multi_stock.preflight.sql`
- `backend/migrations/0004_merchant_multi_stock.up.sql`
- `backend/migrations/0004_merchant_multi_stock.postflight.sql`

The canonical file-table gate files are:

```text
0005_file_records_table.preflight.sql
-> 0005_file_records_table.up.sql
-> 0005_file_records_table.postflight.sql
```

- `backend/migrations/0005_file_records_table.preflight.sql`
- `backend/migrations/0005_file_records_table.up.sql`
- `backend/migrations/0005_file_records_table.postflight.sql`

The canonical file table is `file_records`. Migration `0005` renames the
legacy `files`-only state, treats the existing `file_records`-only state as a
verified no-op, and stops when both or neither table exists. Local model and
migration-artifact tests pass. The complete MySQL 8.4.8 matrix passed with
`make acceptance-file-schema-smoke` on the dedicated acceptance host, including
the F-16 category AutoMigrate RED-to-GREEN check. Migration `0005` has not been
run in production and still requires separate production authorization.

Stop before migration when active orders are nonzero, `LOCKED` products are nonzero, the old index shape differs from the expected unique `(product_id,is_active)` definition, recoverable backup evidence is missing, or `yaner` is absent/duplicated or its pre-release fingerprint cannot be captured. For `LOCKED > 0`, report affected row IDs and active-order counts for explicit business approval; do not apply a blanket status rewrite.

Production must not run `smoke-mysql-concurrency.mjs`. Production write validation uses only a dedicated test merchant/product with small quantities and performs create -> close and create -> complete. It must not use `yaner` data or rotate an existing administrator password merely for testing. Once multi-stock orders exist, production rollback is forward-fix only: do not restore the old unique index or old order code.

F-12, F-13, license governance, miniapp ordering, and MySQL root rotation remain outside this release.

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
