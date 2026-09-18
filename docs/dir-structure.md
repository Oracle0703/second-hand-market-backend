# 目录结构

更新时间：2026-09-12
状态：按当前仓库实际目录整理

## 1. 仓库树

```text
second-hand-market-backend/
├── backend/
│   ├── cmd/server/                    # API 启动入口
│   ├── internal/
│   │   ├── app/                       # 配置、路由和 HTTP handler
│   │   ├── auth/                      # JWT、会话和买家平台登录
│   │   ├── common/                    # 响应、错误码、上下文
│   │   ├── dto/                       # 请求/响应结构
│   │   ├── media/                     # 图片检测、压缩和上传存储
│   │   ├── middleware/                # request ID、鉴权、scope、限流
│   │   ├── model/                     # GORM 模型
│   │   └── stateflow/                 # 商家/商品/订单状态规则
│   ├── migrations/                    # 0001-0009 显式 SQL 迁移
│   ├── scripts/                       # migrate、bootstrap、seed、backfill、verify
│   ├── tests/                         # 集成和安全测试
│   ├── configs/                       # development/remote/prod 示例
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── src/
│   │   ├── app/                       # 路由、Layout、鉴权守卫
│   │   ├── pages/admin/                # 管理员审核和日志
│   │   ├── pages/auth/                 # 登录、注册、审核状态
│   │   ├── pages/merchant/             # 仪表盘、分类、商品、订单、意向、账户
│   │   ├── services/                  # API 请求封装
│   │   ├── stores/                    # 登录和运行时状态
│   │   ├── constants/                 # 状态、错误码、权限常量
│   │   ├── types/                     # TypeScript 类型
│   │   └── styles/                    # 全局样式
│   ├── package.json
│   └── vite.config.ts
├── miniapp/
│   ├── src/
│   │   ├── pages/                     # home/category/search/product/favorite/history/intent/me/store-guide
│   │   ├── components/                # 商品卡片、隐私授权等
│   │   ├── services/                  # buyer API、请求和登录
│   │   ├── stores/                    # 买家会话、商户入口和游客资产
│   │   ├── hooks/                     # 页面数据和平台能力 hooks
│   │   ├── utils/                     # 联系电话、状态、环境配置
│   │   ├── assets/                    # tabbar 和页面资源
│   │   └── app.tsx/app.config.ts      # Taro 应用入口和平台配置
│   ├── tests/                         # Vitest 回归测试
│   ├── config/                        # dev/prod 平台配置
│   ├── package.json
│   └── project.config.json
├── docs/                              # 当前文档索引、规格、清单和历史设计
├── scripts/                           # 冒烟和验收脚本
├── .github/                           # CI 工作流
├── Makefile
└── AGENTS.md
```

## 2. 代码归属

### 后端

- 路由注册统一在 `backend/internal/app/server.go`。
- 商品、库存和订单编排在 `backend/internal/app/*_handlers.go`；状态合法性复用 `stateflow`。
- 买家公开读取、登录、收藏、历史和意向仍由 `app` handler 提供，模型位于 `model/models.go`。
- 数据库 schema 只通过 `backend/scripts/migrate` 执行 `backend/migrations`，API 启动不会自动迁移或 seed。

### 管理端

路由和页面入口在 `frontend/src/app/App.tsx`；导航在 `Layout.tsx`。商品状态、库存弹窗和分类管理位于 `pages/merchant`，管理员审核位于 `pages/admin`。

### 买家小程序

页面按 Taro 路由组织，网络请求统一经过 `miniapp/src/services`。商户入口通过 `merchant_no` 传递；电话和导航分别封装在 `utils/contact.ts` 与门店指南页面，抖音隐私授权由全局 `PrivacyAuthorizationDialog` 配合平台回调处理。

## 3. 新增代码约定

1. 新路由先在 `server.go` 注册，再补 API checklist、权限和测试。
2. 新状态必须同时更新 model、stateflow、当前规格、页面文案和验收清单。
3. 数据库变化必须提供 up/down（或明确不可逆原因）迁移和迁移测试。
4. 不在 `docs/` 创建独立 review 文件；设计结论直接回写规格或专题交付记录。
5. 生产密钥、数据库凭据、上传文件和本地构建目录不得进入 Git。

## 2026-09-18 架构改进分支补充（未部署）

库存调整及订单写用例收拢到 `backend/internal/app/inventory_service.go`，不依赖 Gin；`ownership.go` 共用归属/行锁。运行生命周期及 `/readyz` 位于 `runtime.go`。Web 分类契约集中在 `frontend/src/types/category.ts`。完整边界及未完成项见[架构现状与演进边界](architecture-evolution-plan-2026-07-24.md)。
