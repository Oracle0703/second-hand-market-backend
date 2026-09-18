# 架构现状与演进边界

更新日期：2026-09-18。本文描述 `refactor/architecture-boundaries` 分支的实现；尚未合并、尚未部署。基线为 P1 修复 PR #61（`fe8c0fc8`）。

## 系统形态

保留单体 monorepo：React/Vite 商户及管理员 Web、Taro 微信/抖音买家端、Gin/GORM 单一 API、MySQL。当前以线下成交为中心，不含支付、退款、购物车、聊天和履约。

| 边界 | 当前职责 |
| --- | --- |
| `backend/internal/app/*handlers.go` | 参数绑定、身份读取、HTTP 响应；部分非库存领域仍直接操作 GORM |
| `backend/internal/app/inventory_service.go` | 创建订单、完成/关闭订单、库存调整；库存约束、行锁、流水和关键审计 |
| `backend/internal/app/ownership.go` | 归属校验和商品/订单加锁读取 |
| `backend/internal/app/idempotency.go` | 幂等键占用、业务事务、响应保存与重放 |
| `backend/internal/stateflow` | 商品/订单状态转换规则 |
| `backend/internal/media` | 图片校验、转换、超时与落盘辅助 |
| `frontend/src/services/api.ts` | HTTP API；分类兼容字段统一在此转换 |
| `frontend/src/types/category.ts` | 分类前端契约；页面仅使用规范字段 |
| `miniapp/src/libs/react-query.ts` | 自建查询生命周期适配器，非完整 TanStack Query 缓存 |

应用服务暂时留在 `app` 包，避免为目录分层一次性搬迁。服务方法不接收 `gin.Context`；操作人以 `common.Actor` 传入，审计以同步写入回调传入。后续存在第二种调用入口时，可把稳定边界迁移至独立包。

## 事务与库存约束

- 调整库存及订单状态：HTTP 幂等入口提供事务，业务服务在该事务上操作；内部 `Transaction` 为保存点，不产生独立提交。
- 创建订单：应用服务负责事务；行锁、订单、预占、事件、审计一起提交。
- 库存调整/订单关键审计写入失败会导致本次业务回滚。其他旧审计调用暂保持原业务语义，但增加带 request_id 的失败日志，不能视为已经全部改造为强一致审计。
- 行锁和归属校验共用实现；保留数据库库存约束。
- 当前订单数量固定为 1，同一商品最多一笔活动订单。该规则有产品依据，本分支不扩展为多订单预占。
- 新增库存写入口应调用应用服务，禁止另写一套库存扣减逻辑。

## 运行与可观测性

- `/healthz` 是存活检查；`/readyz` 在 2 秒内探测数据库，失败返回 503，不暴露连接串或数据库错误。
- 收到 SIGINT/SIGTERM 后停止接收新连接，最多等待 30 秒完成已进入的请求，然后关闭数据库。
- HTTP 配置读取请求头超时及空闲连接超时；图片处理保留自己的超时边界。
- 结构化请求日志仅记录 request_id、HTTP 方法、路由模板、响应码和耗时；不记录请求正文、URL 查询串、认证头或密钥。
- 现有部署健康检查暂仍使用 `/healthz`。切换为 `/readyz` 是待确认的发布配置变更，本次未改部署工作流或生产探针。

## 前端与小程序契约

Web 分类字段集中为 `Category`；历史 PascalCase 字段在 HTTP API 边界兼容，页面只使用小写字段。该步骤不是完整 OpenAPI 生成：商品、订单等 DTO 仍需逐步集中，不宣称已实现跨端类型自动同步。

Web 查询缓存已有 P1 身份隔离。小程序自建适配器具有以下约定：

- 每个 hook 持有自己的数据；相同 query key 不共享缓存、不去重请求。
- `invalidateQueries` 按 key 前缀刷新当前挂载的查询。
- key/启用状态变化或卸载后，旧请求不能覆盖新状态。
- 一次请求及其重试绑定启动时的 queryFn，不能转而执行另一身份/参数的函数。
- 此封装没有持久缓存、staleTime、后台同步等完整 TanStack 语义；页面不能依赖这些未实现能力。
- 查询生命周期测试覆盖迟到响应、禁用、匹配失效和卸载。小程序登录/退出后的整页状态隔离仍需结合各页面和真机验收。

## 数据库与发布

API 启动不执行迁移、seed 或开户。生产变更使用独立数据库命令和显式 SQL；迁移目录有校验和及执行门禁。测试中的 AutoMigrate 不等于生产启动迁移。

自动 CI 覆盖 Go 测试/vet、Web 测试/构建、API 镜像、临时 MySQL 库存并发及部署回滚演练。小程序测试与微信/抖音构建保留在手动触发的 `Miniapp validation` 工作流中，使用示例 API 地址，不上传平台。真机、平台隐私授权、域名白名单仍需独立验收。

Deploy 在 main 的 CI 成功后自动发布前后端至 staging；production 通过手动 dispatch 和 Environment 审批发布。两者使用独立部署目录、Compose 项目和回环端口；production 复用原 MySQL 与上传目录。

## 后续按需求推进

1. 单独修复既有 P2（意向唯一约束、游客收藏再次合并、上架分类检查、买家状态检查等），不要以此次架构重构代替功能验收。
2. 按页面/领域逐步集中剩余响应 DTO，增加契约测试后再选择自动生成方案。
3. 库存服务可继续收拢商品编辑/上架等规则；当前已经抽取订单和手动调整用例，未声称整个 app 包已分层完毕。
4. 单实例继续使用本地文件、进程内限流；扩展多实例前先处理共享存储、共享限流及连接池容量预算。
5. 增加运行指标、告警、备份恢复演练；本轮请求日志和就绪检查只是基础能力。
6. 只有明确出现多笔订单并行成交需求，才拆分展示状态与预占关系，新增兼容迁移和并发验收。

以上是可逐项审核的演进路径，不要求重写项目或引入微服务。

---

## 历史方案快照（2026-09-12，供追溯）

以下保留原方案，部分描述已过时，尤其 API 自动迁移及分层现状。当前实现与本分支范围以本文上半部分为准。

# 架构演进方案（基于现状）

- 原始方案日期：2026-07-24
- 更新日期：2026-09-12
- 当前基线：`main`（HEAD `ad74202`，工作区还包含待提交的隐私授权、CI、证据和文档整理）
- 目标：在**不推倒重来、不拆微服务**的前提下，给出贴合本仓库真实代码的演进路径
- 性质：架构方案和后续路线，不是当前施工单；生产发布仍以 `docs/release-readiness.md` 为准
- 原则：**先止血与收敛边界，再局部加厚分层；禁止为了“看起来像 clean architecture”而大搬家**

---

## 0. 一句话结论

当前系统是一个**已经跑通业务闭环的模块化单体 monorepo**。
大方向合理；真正该演进的不是“上微服务 / 上知识图谱 / 全量 DDD”，而是：

1. 把**订单库存**从胖 handler 里收成明确应用服务
2. 把**schema / 发布**从双轨收敛成单一事实来源
3. 把**跨端读模型**（尤其 `stock` 语义）固定下来
4. 把文档里的“目标架构”和代码里的“实际架构”对齐，避免继续按假想目录开发

---

## 1. 现状画像（以仓库真实代码为准）

### 1.1 实际系统形态

```text
miniapp/   (Taro 买家端，微信/抖音)
frontend/  (React 商家后台 + 管理端)
backend/   (Go Gin 单一 API)
docs/      (当前规格、发布清单、历史设计与交付记录)
scripts/   (smoke)
deploy/    (acceptance compose 等)
```

| 端 | 技术 | 约略体量 | 实际职责 |
| --- | --- | --- | --- |
| `backend/` | Go 1.22 + Gin + GORM | ~8k 行 Go | 商家 / 管理员 / 买家 API、鉴权、上传、状态与库存 |
| `frontend/` | React + Vite + Ant Design Pro | ~4k 行 TS/TSX | 商家经营 + 平台审核 |
| `miniapp/` | Taro 3.x | ~2k 行 TS/TSX | 浏览、收藏、历史、门店联系；**暂无下单** |
| `docs/` | Markdown | 当前/历史规格与交付记录 | 当前入口为 `docs/README.md`；独立 review 文件已删除 |

仓库名仍是 `second-hand-market-backend`，但实际已是前后端 + 小程序一体仓。这是历史命名，不构成拆仓理由。

### 1.2 文档目标架构 vs 代码现实

`docs/dir-structure.md` 曾规划：

```text
backend/internal/
  handler/   # HTTP
  service/   # 业务
  repo/      # 数据访问
  filesvc/   # 文件抽象
  ...
```

**当前真实结构（截至 2026-09-12）：**

```text
backend/internal/
  app/          # 几乎全部用例：路由注册 + handler + 事务 + SQL 拼装
  auth/         # JWT / 密码
  common/       # 错误码、响应、actor
  dto/          # 请求 DTO（薄）
  handler/      # 空目录
  media/        # 图片处理（相对干净）
  middleware/   # 鉴权、request_id、admin session
  model/        # GORM 模型
  stateflow/    # 状态转移表（小而有用）
```

前端同样存在“规划有 `features/`、实际几乎全在 `pages/` + `services/`”的落差。

**判断：** 文档写的是理想分层，代码走的是交付优先的事务脚本。后面演进必须以代码现实为起点，不能假装 service/repo 已经存在。

### 1.3 后端真实调用形态

当前主路径基本是：

```text
Gin route
  → middleware (OptionalAuth / RequireAuth / scope / admin session)
  → Server.handleXxx (同一 package app)
      → s.DB.Transaction + GORM
      → stateflow / ownership / idempotency / operation_log
      → gin.H / DTO 拼响应
```

几个能说明问题的事实：

| 信号 | 含义 |
| --- | --- |
| `buyer_handlers.go` ~1176 行 | 买家域 HTTP + 查询拼装 + 登录合并全在一个文件 |
| `product_handlers.go` ~599 行 | 商品 CRUD/状态/删除/图片清理耦合 |
| `order_handlers.go` ~385 行 | **库存不变量已落在这里**，是最值钱也最危险的核心 |
| `server.go` 同时负责 DB 打开、AutoMigrate、seed、路由 | 组合根偏重 |
| `internal/handler` 为空 | 分层迁移曾设想但未落地 |
| 集成测试集中在 `backend/tests` | 对状态机业务是正确策略，应保留并加强 |

这不是“不会分层”，而是**单体事务脚本已经支撑起完整闭环**，并在多库存阶段开始吃紧。

### 1.4 前端 / 小程序真实形态

**frontend**

- 路由 + 角色守卫在 `app/`
- 页面按 admin/merchant/auth 划分，直接调 `services/api.ts`
- `features/` 基本空置
- 类型多在页面内局部定义，与后端 DTO 无共享契约
- 对当前后台页面数量：可接受，但 API 字段语义靠人脑对齐

**miniapp**

- pages + `services/buyer.ts` + session store
- 关键契约依赖后端买家响应里的 `stock` 字段名
- 本阶段不下单，是正确的产品边界，应在架构上继续守住

### 1.5 已实现能力 vs 文档早期假设的漂移

| 早期/部分文档假设 | 代码现实 | 架构含义 |
| --- | --- | --- |
| 本期无买家端 | 已有完整 buyer API + miniapp | overview 类文档需持续以代码为准修订 |
| Redis 会话/限流 | 主要是内存限流 + DB session | 单机假设 |
| 对象存储抽象 | 当前为本地 `uploads`，通过后端受控路由访问 | 文件域仍是 MVP；对象存储可按门槛替换 |
| handler/service/repo | 几乎全在 `app` | 目标结构未落地 |
| 库存固定 1 / LOCKED 主路径 | 当前已实现预占、释放、扣减和售罄恢复；订单请求数量仍固定 1 | 领域规则已进入稳定基线，后续扩展需复用同一事务实现 |
| 分类/管理员后台可管理 | 商家分类 CRUD 已落地；管理员账号仍由脚本初始化 | 保持权限边界清晰 |

### 1.6 当前架构评分（阶段适配，不是学院派）

| 维度 | 评分 | 说明 |
| --- | --- | --- |
| 业务边界（三角色、订单只在商家后台） | 好 | 产品架构清楚 |
| 系统形态（单 API 单体 monorepo） | 好 | 匹配体量与团队 |
| 代码内部分层 | 中 | 能跑，核心域开始挤 |
| 数据/迁移/发布 | 中低 | AutoMigrate + SQL 双轨是实伤 |
| 跨端契约治理 | 中低 | 同名字段不同语义已出现 |
| 多实例/云原生准备度 | 低 | 尚非当前主矛盾 |
| 文档与实现一致性 | 中高 | 当前文档已收敛；历史方案保留时点上下文 |

**总评：现阶段合理；需要的是收敛演进，不是重写。**

---

## 2. 目标架构（仍保持单体）

### 2.1 明确不做什么

本方案**明确排除**以下事项作为近中期目标：

1. 拆成 merchant-service / buyer-service / file-service 等微服务
2. 引入完整 DDD 战术大全（聚合仓库事件溯源等）
3. 上代码知识图谱平台作为研发基础设施
4. 前端强行 feature-sliced 大迁移
5. 仅仅为了目录好看，把 `app/*.go` 一次性物理搬家

这些在当前体量下成本高于收益，且会干扰正在进行的生产加固与多库存发布。

### 2.2 目标形态：加厚后的模块化单体

```text
                    ┌──────────── frontend (merchant/admin) ────────────┐
                    │                                                   │
 clients            ├──────────── miniapp (buyer) ──────────────────────┤
                    │                                                   │
                    └─────────────────────┬─────────────────────────────┘
                                          │ /api/v1
                                          v
┌──────────────────────────────────────────────────────────────────────┐
│ backend (single deployable)                                          │
│                                                                      │
│  transport/http   : 绑定参数、鉴权上下文、状态码、响应包装              │
│  app/<domain>     : 用例编排、事务边界、幂等、操作日志                  │
│  domain/*         : 不变量与状态规则（先从 inventory/order 开始）       │
│  platform/*       : auth、files、media、clock、id                      │
│  store/gorm       : 模型与查询（暂不强制完整 repo 接口）                │
│  migrate          : SQL 为生产 schema 权威来源                         │
└──────────────────────────────────────────────────────────────────────┘
```

说明：

- 名字可以仍落在 `internal/` 下，不一定叫 clean arch 教科书那套。
- **先按域切开，再考虑目录改名。**
- GORM 可以继续用；关键不变量用“条件更新 + 事务 + 测试”保证，不靠 ORM 魔法。

### 2.3 目标包职责（按优先级渐进出现）

| 包/模块 | 何时出现 | 职责 | 现在对应物 |
| --- | --- | --- | --- |
| `http` 或瘦 handler | 阶段 B | 只做 bind/auth actor/response | `app/*_handlers.go` 前 20% |
| `app/order` 或 `service/inventory` | 阶段 B | 创建/完成/关闭订单用例 | `order_handlers.go` 事务体 |
| `app/product` | 阶段 C | 上下架/关闭/删除/编辑约束 | `product_handlers.go` |
| `app/buyer_catalog` | 阶段 C | 买家列表详情与 stock 映射 | `buyer_handlers.go` 前半 |
| `view` / `presenter` | 阶段 B/C | Merchant/Buyer 响应装配 | handler 里零散 `gin.H` |
| `platform/files` | 阶段 D | 上传与访问策略 | `file_handlers.go` + `media` + `/uploads` |
| `migrate` 流程 | 已落地 | SQL 权威；生产 API 禁止自动迁移和 seed | `migrations/` + `scripts/migrate` |
| 完整 `repo` 接口层 | 不急 | 仅当存储要可替换或查询重复到痛时 | 直接 GORM |

### 2.4 核心域对象（先概念，不强制上复杂类型系统）

优先用“文档 + 函数 + 测试”固化，不必先造一堆 interface：

```text
InventoryPolicy
  - available = stock - reserved
  - reserve(qty)   条件：status=ON_SHELF && available >= qty
  - release(qty)   条件：reserved >= qty
  - consume(qty)   条件：stock >= qty && reserved >= qty
  - maybeMarkSold  仅 stock == 0

OrderLifecycle
  - create / complete / close
  - 行锁订单 + 条件更新商品
  - 维护 LOCKED / active_order_id

ProductLifecycle
  - on_shelf / off_shelf / delete
  - delete 要求无活动订单且符合业务约束

CatalogViews
  - MerchantProductView: stock, reserved_stock, available_stock
  - BuyerProductView: stock := available_stock（兼容字段）
```

这些规则今天已经散落实现；演进目标是**有唯一实现点**，而不是再写一份平行文档。

---

## 3. 主要矛盾与对应策略

### 3.1 威胁 A：核心规则继续堆在 handler

**现状证据：** 多库存逻辑、行锁、幂等、操作日志都在 `order_handlers.go` / `product_handlers.go`。
**风险：** 小程序一旦下单、或出现退款回库，文件会继续膨胀，回归成本指数上升。
**策略：** 只抽取“高价值域”，从订单库存开刀；买家收藏历史等 CRUD 可暂留 handler。

### 3.2 威胁 B：Schema 双轨

**现状证据：** 生产和远程开发已强制 `AUTO_MIGRATE=false`，schema 由 `migrations/*.sql` 和显式命令推进。
**风险：** 生产结构与模型标签漂移，多库存唯一索引问题会重演。
**策略：** 生产 schema 以 SQL migration 为权威；AutoMigrate 降级为本地开发便利或彻底关闭生产路径。

### 3.3 威胁 C：跨端同名字段不同语义

**现状证据：** 买家 `stock` = 可售；商家 `stock` = 总库存。
**风险：** 新接口/新页面按“字面字段”对接即错。
**策略：** 固定 View 装配函数 + API 文档/测试双锁；禁止直接序列化同一 `model.Product` 给不同端。

### 3.4 威胁 D：单机基础设施假设

**现状证据：** 内存限流、本地受控 uploads、开发环境 mock 登录默认可用，生产禁止 mock。
**风险：** 多实例或正式公网后行为变化。
**策略：** 按业务门槛升级（先文件私有化与真实登录，再谈 Redis 限流）。

### 3.5 威胁 E：文档多真相

**现状证据：** 历史方案和当前规格并存，且独立 review 文件已删除。
**风险：** 人与 AI 都可能读到过期规则。
**策略：** 建“当前有效文档索引”，历史审查保持快照，不回写装成从未发生。

### 3.6 威胁 F：空目录与假分层

**现状证据：** `internal/handler`、`frontend/src/features` 空。
**风险：** 后来者按空目录“补齐分层”却无迁移策略，制造更大混乱。
**策略：** 要么删空目录，要么在本方案中规定“何时才能往里放第一份代码”。

---

## 4. 分阶段演进路线

> 每阶段都必须：**可独立合并、可回滚、不阻断当前生产验收**。
> 阶段 A 已基本收敛；阶段 B 建议在多库存和图片管线生产稳定后开始。

### 阶段 A — 收敛与门禁（1 周量级，低风险）

**当前状态：大部分已完成。** `docs/README.md`、显式迁移命令、生产启动门禁和回归测试已落地；剩余工作是持续把新增路由和平台能力同步到当前文档。

**目标：** 不改业务行为，先让架构“可治理”。

#### A1. 文档与索引对齐

1. 已维护 `docs/README.md`（当前有效规格索引）：
   - 权威业务规则：`specs.md` / `data-model.md` / `backend-api-checklist.md`
   - 当前发布：`release-readiness.md`、`miniapp-release-readiness.md`
   - 历史快照：`decisions/`、`delivery/`
2. 已修订 `project-overview.md` 中与现实冲突的范围句。
3. 已将 `dir-structure.md` 改为真实结构；目标分层保留在本文作为演进目标。
4. 已在当前文档中标注空目录意图，避免假分层。

#### A2. Schema 治理策略落地

1. 生产：`AUTO_MIGRATE=false` 或仅允许“加列幂等、禁止依赖其删约束”。
2. 关键约束变更（唯一索引、CHECK）**必须**走 `migrations/*.sql`。
3. 发布检查单固定：
   - 备份
   - 预检（active orders、LOCKED 计数）
   - 执行 SQL
   - `SHOW INDEX` / 不变量 SQL
   - 再发应用
4. 本地开发可继续 AutoMigrate，但 CI 增加“模型标签不含已废弃 unique”检查（已有单测应保留）。

#### A3. 架构守护测试（先加测试，不抽代码）

在不改包结构的前提下补齐：

1. 多库存并发创建（stock=5）
2. 同单完成/关闭竞争
3. LOCKED 遗留数据处理策略的回归
4. 买家/商家 stock 语义对测
5. （建议）MySQL 8 烟测脚本进 `scripts/` 或 `deploy/acceptance`

**阶段 A 完成标准**

- 新人/AI 能在 10 分钟内知道“以哪几份文档为准”
- 生产迁移不再依赖口头约定
- 多库存关键不变量有自动化看守

---

### 阶段 B — 抽出订单库存应用服务（2–4 天有效开发，中收益）

**当前状态：尚未抽成独立 service。** 订单库存规则已在 `order_handlers.go` 事务中正确实现，下一次架构改动应以测试保护下的渐进抽取为目标，不把“已实现业务规则”误写成“已完成分层”。

**目标：** 把最值钱的规则从 HTTP 里挪走，handler 变薄。

#### B1. 建议落点（最小扰动）

优先采用“同模块渐进”，避免大爆炸改 import：

```text
backend/internal/
  app/
    order_handlers.go          # 仅 HTTP
    product_handlers.go        # 暂不动或只调用 service
  service/
    inventory/
      order_service.go         # Create/Complete/Close
      stock.go                 # reserve/release/consume 纯规则或小函数
      views.go                 # total/reserved/available 计算
```

若不想新增 `service` 目录，也可先：

```text
backend/internal/app/inventory_service.go
```

**关键是依赖方向，不是文件夹美学：**

```text
handler → inventory service → gorm.DB / model / stateflow
```

禁止 service 引用 gin.Context（可传入 actor、requestID、写日志回调）。

#### B2. 迁移步骤（推荐）

1. 先把 `orderTotalDealPriceCent`、预占/双减/释放的条件更新，原样搬到 service。
2. `handleCreateOrder` / `doOrderAction` 改为调用 service。
3. 保持 API JSON 字段不变。
4. 现有 `multi_stock_order_test.go`、并发测试必须继续绿。
5. 再考虑给 service 加表驱动单测（不启动 HTTP）。

#### B3. 同时做的读模型固化

```go
// 示意，非最终 API
func BuyerStock(p model.Product) int { return p.Stock - p.ReservedStock }
func MerchantInventory(p model.Product) (stock, reserved, available int)
```

买家 handler 与商家 handler 都只许走这些函数，禁止各写各的减法。

**阶段 B 完成标准**

- 订单创建/完成/关闭主路径业务代码不在 gin handler 内超过薄封装
- 新增库存规则时只需改 service + 测试
- HTTP 层无库存算术副本

---

### 阶段 C — 按域切开胖文件（按痛点触发，不设死线）

**目标：** 降低并行修改冲突，而不是追求目录完成度。

#### C1. 拆分顺序（按收益）

1. **inventory/order**（阶段 B 待做；当前规则仍在 handler 事务中）
2. **product lifecycle**（删除/上架校验与预占耦合）
3. **buyer catalog**（列表/详情/图片拼装，`buyer_handlers.go` 前半）
4. **buyer social graph**（收藏/历史/guest merge）
5. **auth**（admin/merchant/buyer 登录发 token 已相对集中，可后置）
6. **admin review**（体量不大，可后置）

#### C2. 拆分手法

- 一次只拆一个域
- 先搬函数，不改路由 path
- 每拆一次只要求相关测试绿
- 不要在同 PR 里改前端

#### C3. 前端配套（轻量）

不必上完整 features 分层，只做：

1. `frontend/src/types/api.ts`（或按域拆）沉淀订单/商品库存字段
2. 商品列表/详情/创建订单共用类型
3. 禁止页面里再解释“stock 到底是什么”——注释写在 types

小程序：

1. 在 `services/buyer.ts` 旁注释：`stock` 为可售
2. 若未来增加 `available_stock`，保持兼容，不急着改页面

**阶段 C 完成标准**

- 不再出现 1000+ 行的必改核心文件成为所有需求冲突点
- 新需求能明确落到单一域目录

---

### 阶段 D — 平台能力升级（按业务门槛，不与 C 绑死）

这些是**独立门禁**，对应已有审查项，不阻塞商家多库存稳定运行。

| 能力 | 触发条件 | 架构动作 |
| --- | --- | --- |
| 文件私有化 | 真实商家执照审核前 | `/uploads` 受控；admin 文件流式读取；`files` 模块独立 |
| file_id 归属绑定 | 同上 | 绑定校验进 files/product/merchant 用例，不散落 |
| 真实微信/抖音登录 | 体验用户可迁移后 | 关闭 mock 默认；登录模式进生产校验 |
| 多实例 | 真正要水平扩展时 | 限流/会话外部化；去掉进程内假设 |
| 对象存储 | 本地盘与备份成为负担时 | `media`/`files` 已有缝可插，不必先重构全局 |
| 小程序下单 | 产品确认 | **复用阶段 B 的 inventory service**，只加 buyer 下单入口与权限，禁止复制一套库存逻辑 |

特别强调：

> 若阶段 B 没做就开放买家下单，库存逻辑被复制到 buyer handler 的概率极高。
> **小程序交易是阶段 B 的主要业务驱动之一。**

---

### 阶段 E — 可选远景（明确“可以不做”）

仅当团队变大或系统边界变复杂时再评估：

1. 真正的 `repo` 接口（换 DB 或读写分离时）
2. 出站消息/事件（到店通知、短信、多端推送）
3. 前后端 OpenAPI 生成类型
4. 拆仓（frontend/miniapp/backend 独立版本节奏冲突时）
5. 知识图谱类工具（仅当“找关系”成本长期高于维护成本）

---

## 5. 与近期施工的关系

### 5.1 必须先于或并行于架构炫技的事项

来自当前发布门禁与已落地加固，**优先级高于阶段 C 大拆分**：

1. 真实平台和真机登录/隐私验收
2. 图片回填、严格模式和受控 `/uploads` 发布门禁
3. 管理员 session、数据库身份和迁移策略
4. 备份与 MySQL 维护窗
5. 多库存并发/竞争测试持续补齐

这些不完成，谈“优美分层”没有意义。

### 5.2 建议时间叠放

```text
现在
  ├─ 真实平台/真机与图片管线验收         [硬]
  ├─ 阶段 A 文档与 schema 门禁           [基本完成]
  └─ 多库存生产发布稳定
        │
        ▼
     阶段 B 抽取 inventory service        [高收益]
        │
        ▼
     阶段 C 按痛点拆 buyer/product        [中]
        │
        ▼
     阶段 D 文件/登录/多实例              [门槛驱动]
```

---

## 6. 目标目录演进示意（最终可能态，非一次性 PR）

```text
backend/
  cmd/server/
  internal/
    auth/
    common/
    middleware/
    model/                 # 持久化模型（可继续叫 model）
    dto/                   # 入站请求 DTO
    stateflow/             # 通用转移表
    media/
    service/
      inventory/           # 订单库存用例（先有）
      product/             # 随后
      buyercatalog/        # 随后
      merchantauth/        # 可选
    http/                  # 或保留 app 作 http 层
      router.go
      order_handler.go
      product_handler.go
      buyer_handler.go
      admin_handler.go
      files_handler.go
    view/                  # Merchant/Buyer 响应装配
  migrations/              # 生产权威 schema
  scripts/
  tests/                   # 保留黑盒集成测试
```

前端保持：

```text
frontend/src/
  app/
  pages/                   # 继续承载页面
  services/
  types/                   # 补齐共享 API 类型（新增重点）
  stores/
  constants/
  # features/ 仅在某域页面复用逻辑变复杂时再迁入
```

小程序保持 page + service，不为架构而架构。

---

## 7. 设计约束（写进团队约定）

1. **一个可部署后端**服务三角色；用 middleware 与 scope 分权，不按角色拆进程。
2. **库存不变量只允许一个实现点**；新入口必须调用同一 service。
3. **买家与商家响应禁止直接暴露同一产品 JSON 形状而不经 view。**
4. **生产结构变更以 SQL migration 为准。**
5. **集成测试是架构的一部分**；抽 service 后不得删除 API 级回归。
6. **新依赖默认不进核心路径**（Redis/OSS/消息队列按门槛引入）。
7. **文档分“当前有效”与“历史快照”**；AI/人优先读当前有效索引。
8. **`yaner` 与现网数据保护规则属于发布架构**，任何迁移方案必须显式包含。
9. **空目录不得假装分层已完成。**
10. **不做微服务，除非有明确的独立扩展/独立失败域需求。**

---

## 8. 成功度量

### 8.1 过程度量

| 指标 | 现在 | 阶段 B 后期望 |
| --- | --- | --- |
| 修改库存规则需要碰的文件数 | 3–6+（handler/测试/文档） | 1 个 service + 测试 + 文档 |
| `order_handlers.go` 业务密度 | 高 | 以 HTTP 编排为主 |
| 生产迁移是否依赖 AutoMigrate 删索引 | 否 | 否 |
| 买家/商家 stock 语义测试 | 有部分 | 固化为必跑对测 |
| 1000+ 行必改核心文件数 | 至少 1（buyer） | 逐步下降 |

### 8.2 质量门禁

1. `go test ./...` 全绿
2. 多库存并发与完成/关闭竞争测试全绿
3. 关键 API 契约 smoke 全绿
4. 迁移预检脚本可在维护窗执行
5. 前端/小程序不因后端内部分层而改协议

---

## 9. 风险与反模式

| 反模式 | 为什么避免 |
| --- | --- |
| 先建齐 handler/service/repo 空壳再填肉 | 制造假进度与循环依赖 |
| 一次 PR 搬空 `app` 包 | 审查不可能，回归面过大 |
| 为每个 GORM 模型写完整 repository interface | 当前无替换需求，纯样板 |
| 前后端上 monorepo 内共享运行时代码 | 技术栈不同，收益低 |
| 用图谱/平台替代测试与迁移门禁 | 解决不了不变量 |
| 小程序下单时复制一套扣库存 | 制造永久分叉 |
| 边发布多库存边做大拆分 | 问题归因困难 |

---

## 10. 建议的“第一次落地 PR”长什么样

当 P1 修复完成后，架构向的第一个实质 PR 建议仅包含：

1. 新增 `internal/service/inventory`（或 `app/inventory_service.go`）
2. 迁移 Create/Complete/Close 的事务体
3. 抽出 `BuyerStock` / `MerchantInventory`
4. 不改任何 API 字段
5. 全量后端测试 + 多库存相关测试
6. 在 `docs/README.md` 或对应当前规格中记一笔：库存规则实现点变更

**不要**在同一 PR 中：

- 拆 buyer_handlers
- 改前端目录
- 上 Redis
- 改路由前缀
- 重命名整个 `app` 包

---

## 11. 最终建议

1. **承认现状：务实单体，已经成功支撑闭环。**
2. **不要为了“架构完整”重写。**
3. **把演进预算花在订单库存、schema 门禁、跨端读模型三处。**
4. **用业务门槛拉动平台升级（文件、登录、多实例），而不是日历式大重构。**
5. **Codex/Grok 等 AI 助手的最佳辅助，是稳定的当前文档索引 + 强测试，而不是先造图谱。**

若只选一件事作为架构投资：

> **抽出 inventory/order 应用服务，并让生产 schema 以 SQL migration 为唯一权威。**

这件事对你们下一阶段（稳产、真实商家、未来小程序交易）的复利最大。

---

## 12. 文档状态

- 本文原始日期为 2026-07-24，已按 2026-09-12 代码基线更新。
- 本文不替代生产发布清单；环境、迁移和平台审核以 `docs/release-readiness.md` 与 `docs/miniapp-release-readiness.md` 为准。
- 独立 review 文件已删除；其仍有价值的结论已合并到当前规格、专题设计和交付记录。
- 阶段 B 仍是后续架构工作，实施后需回写本文“阶段状态”和验证证据。
