# 发布前收口清单

更新时间：2026-09-12
状态：代码回归基线已通过；生产发布仍待环境签收

## 1. 当前版本范围

- Go/Gin/GORM 后端：商家入驻、管理员审核、商户分类、商品五状态、库存调整、轻量订单、买家域和审计。
- React/Vite 管理端：审核、仪表盘、分类、商品、库存、订单、意向、账户和日志页面。
- Taro 小程序：微信/抖音双构建，商户范围商品浏览、收藏、历史、登录合并、门店导航、电话联系和隐私授权。
- 图片：presign/upload/confirm、JPEG/PNG/WebP/HEIC/HEIF 校验、40 MB 原图限制、detail-v1 展示图和受控 `/uploads`。

## 2. 已完成自动验证

以下结果对应当前工作区最近一次完整回归：

| 检查 | 结果 |
| --- | --- |
| `make test` | 通过 |
| `go vet ./...` | 通过 |
| `cd frontend && npm run test` | 39/39 通过 |
| `cd frontend && npm run build` | 通过；仅有既有 Rollup chunk warning |
| `cd miniapp && npm test` | 173/173 通过 |
| `cd miniapp && npm run build:weapp` | 通过 |
| `cd miniapp && npm run build:tt` | 通过 |

代码回归不等于生产发布；验收证据应绑定实际提交 SHA 保存，不把本地 `.tmp/`、`.worktrees/` 或 `deploy/acceptance/evidence/` 当作源码文档。

## 3. 发布前必须完成

### 后端与数据库

1. 生产 `APP_ENV=production`，显式配置 `DB_DSN`、两枚独立随机 JWT 密钥，关闭 `AUTO_MIGRATE` 和 `SEED_DEFAULTS`。
2. 使用独立迁移凭据执行迁移；运行 `backend/scripts/verify_database` 和对应 preflight/postflight，核对数据库名、服务 UUID、账号和迁移版本。
3. 新商户默认分类和历史商户分类回填分别执行并记录结果。
4. 确认当前商品状态不含 `CLOSED`，异常 `SOLD + stock > 0` 和 `reserved_stock > stock` 均为 0。

### 图片与网关

1. `/api/v1/*` 反向代理到后端根地址，避免 `/api/api/v1`；`/uploads/*` 必须反代到后端，不得使用 Nginx alias、CDN 或对象存储直链绕过检查。
2. 目标环境安装 vips/libheif，`IMAGE_PROCESSOR_DRIVER=vips`，`FILE_PUBLIC_BASE_URL=`。
3. 旧客户端兼容期可配置 `PUBLIC_UPLOAD_BASE_URL`；新小程序发布并确认采用后再移除兼容配置。
4. 按“预检 -> canary -> 分批回填 -> 严格模式 -> 延迟清理”执行 `detail-v1`，每一步保留 run-id、SHA256 和失败项。

### 管理端与小程序

1. 管理端生产 API 前缀保持 `/api/v1`，完成登录、审核、分类级联、商品状态、库存和订单回归。
2. 小程序完成真实微信/抖音凭据、平台域名/证书、iOS/Android 真机登录和游客数据合并验证。
3. 真机验证电话、门店导航、图片展示、分享落地、弱网重试；抖音隐私授权必须由用户真实点击触发。

## 4. 发布顺序与回滚

1. 先确认数据库迁移和网关配置，再发布后端。
2. 发布管理端和小程序；小程序按微信/抖音平台审核流程分别提交。
3. 发布后立即执行 `/healthz`、管理端 smoke、买家页面 smoke 和关键图片请求检查。
4. 保留上一版后端镜像、前端构建产物和小程序包。出现状态/库存/图片安全异常时先停止写流量，再按已验证版本回滚；数据库迁移回退需单独评估，不直接执行 down。

## 5. 当前风险与结论

- 真实平台凭据、合法域名、证书和真机证据不在本地回归范围内。
- 图片严格管线需要目标环境的 vips/libheif 和独立数据迁移窗口。
- 当前内存限流仅适用于单实例；多实例上线前需要共享限流方案。

**结论：代码基线可进入测试环境验收；在上述环境与真机门禁完成前，不判定为生产可发布。**
