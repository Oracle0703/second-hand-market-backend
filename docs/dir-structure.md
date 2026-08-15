# 目录结构分析（dir-structure）

## 默认假设
1. 仓库采用单仓（monorepo）承载前后端与文档，便于联调与版本一致性管理。
2. 当前仓库是空白初始化状态，本结构为推荐落地形态。
3. 本期不做拆仓，工程组织以“快速交付商家后台闭环”为优先目标。

## 仓库定位说明
1. 这是单仓前后端一体项目，包含 `frontend/`、`backend/`、`docs/`。
2. 仓库名虽为 `second-hand-market-backend`，但当前实际承载前端、后端与文档。
3. 未来若拆仓，当前目录结构仅代表本期组织方式，不影响业务边界定义。

## 1. 推荐目录树

```text
second-hand-market-backend/
├── frontend/                        # React + TS + Vite
│   ├── public/
│   ├── src/
│   │   ├── app/                     # 应用入口、路由、provider
│   │   ├── pages/                   # 页面级组件（按路由）
│   │   ├── features/                # 业务域模块（auth/audit/products/orders/categories）
│   │   ├── components/              # 通用组件
│   │   ├── services/                # API 请求封装
│   │   ├── stores/                  # 全局状态（轻量）
│   │   ├── hooks/                   # 通用 hooks
│   │   ├── types/                   # 前端共享类型
│   │   ├── utils/                   # 工具函数
│   │   ├── styles/                  # 全局样式与响应式变量
│   │   └── constants/               # 常量/错误码映射
│   ├── .env.development
│   ├── .env.production
│   ├── index.html
│   ├── package.json
│   └── vite.config.ts
├── backend/                         # Go API 服务
│   ├── cmd/
│   │   └── server/                  # 启动入口
│   ├── internal/
│   │   ├── handler/                 # HTTP handler（按模块）
│   │   ├── service/                 # 业务逻辑层
│   │   ├── repo/                    # 数据访问层
│   │   ├── model/                   # GORM 模型定义
│   │   ├── dto/                     # 请求响应 DTO
│   │   ├── middleware/              # 鉴权、日志、限流
│   │   ├── auth/                    # token/session 逻辑
│   │   ├── stateflow/               # 状态机校验（审核/商品/订单）
│   │   ├── filesvc/                 # 文件上传抽象
│   │   └── common/                  # 错误码、工具、常量
│   ├── migrations/                  # 数据库迁移脚本
│   ├── scripts/                     # 本地开发脚本（含管理员初始化、分类初始化、历史商户分类回填）
│   ├── configs/                     # 配置模板
│   ├── tests/                       # 集成测试
│   ├── go.mod
│   └── go.sum
├── docs/                            # 项目文档
├── Makefile
└── README.md
```

## 2. 前端分层建议
1. `pages/` 仅负责页面编排，不直接写复杂业务逻辑。
2. `features/` 按业务域组织（`auth`、`merchantAudit`、`products`、`orders`、`categories`）。
3. `services/` 对接后端 API，统一处理 token、错误码、重试策略。
4. `types/` 与后端 DTO 对齐，避免页面层定义散乱类型。

## 3. 后端分层建议
1. `handler`：参数绑定、鉴权上下文读取、响应封装。
2. `service`：事务控制、状态机校验、跨模块编排。
3. `repo`：仅处理数据库查询与持久化，不做业务判断。
4. `stateflow`：集中定义审核/商品/订单合法流转，避免散落 if-else。
5. `middleware`：request_id、日志、panic recover、权限校验。

## 4. 配置与环境管理
1. 配置分层：`local/dev/staging/prod`，通过环境变量驱动。
2. 必备环境变量：
   - `MYSQL_DSN`
   - `REDIS_ADDR`
   - `JWT_ACCESS_SECRET`
   - `JWT_REFRESH_SECRET`
   - `FILE_PROVIDER`（minio/oss/s3）
   - `FILE_BUCKET`
3. 禁止将生产密钥写入仓库。

## 5. 初始化脚本约定
1. `backend/scripts/migrate`：唯一负责 schema 初始化或迁移的显式命令。
2. `backend/scripts/bootstrap_admin`：每次使用调用者显式提供的身份和密码创建一个管理员，不执行迁移。
3. `backend/scripts/seed_categories`：维护内置一级/二级默认分类，不执行迁移。
4. `backend/scripts/backfill_merchant_categories`：为历史商户复制默认分类，并将旧商品分类映射到商户分类。
5. 上述命令必须显式设置 `DB_DRIVER` 和 `DB_DSN`，不得回退到仓库内的本地数据库文件。
6. bootstrap 密码必须通过 `ADMIN_PASSWORD` 显式提供，不得写入仓库、命令摘要或日志。
7. bootstrap、分类 seed 与商户分类 backfill 需要幂等，可重复执行；长驻 API 不得调用上述操作。

## 6. 测试目录与约定
1. 单元测试与源代码同目录：`*_test.go`。
2. 集成测试集中在 `backend/tests/`，覆盖主流程。
3. 前端 E2E 可放 `frontend/e2e/`（如 Playwright）。

## 7. 可维护性约束
1. 错误码、状态枚举、权限常量集中管理，不允许魔法字符串散落。
2. 新增接口必须同步更新 OpenAPI 与 `docs/backend-api-checklist.md`。
3. 状态流转变更必须同步更新：
   - `docs/specs.md`
   - `docs/data-model.md`
   - `backend/internal/stateflow/`
