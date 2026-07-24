# 全仓库深度 Code Review

> **文档状态**：本文是 Grok 侧底稿。后续审查结论与条目编号（F-*）以合并增强版为准：
> **`docs/full-project-code-review-2026-07-24.md`**（已交叉复核本文 R-* 条目，并补充 Intent 索引、幂等、分类 schema 等项）。
> 两份文档结论一致处可对照阅读；若表述或分级有差异，以 full-project 文档为准。

- 审查日期：2026-07-24
- 审查基线：`master` / `1bde7d9`（审查时本地领先 `origin/master` 5 个提交）
- 审查范围：`backend/`、`frontend/`、`miniapp/`、`docs/`、仓库根配置与 Git 跟踪状态
- 审查方式：架构通读、关键路径静态复核、与既有文档/规格对照；不依赖本轮新增自动化测试结果
- 相关文档：`docs/full-project-code-review-2026-07-24.md`（合并增强版 / 当前主审查文档）

## 1. 总体结论

当前仓库是一个业务闭环已基本成形的二手交易 monorepo：商家后台、平台审核、买家小程序与后端 API 已连成主链路。工程骨架可读，后端集成测试与买家域设计有一定质量。

但存在多项**生产发布阻断问题**，集中在：

1. 安全默认值与固定管理员凭据
2. 文件 ID 绑定缺少归属与类型校验
3. 订单唯一索引设计错误
4. 审核流程缺少执照预览
5. 小程序 access token 过期后无法自动恢复会话
6. 匿名上传资源耗尽风险
7. 库存模型与成交状态不一致

**发布判断：不建议在修复 P0/P1 问题前部署到生产环境。**

本文只记录审查发现与判断，不包含修复排期或实施计划。

## 2. 项目画像

| 端 | 技术 | 职责 |
| --- | --- | --- |
| `backend/` | Go 1.22 + Gin + GORM | 商家 / 管理员 / 买家 API、鉴权、上传、状态机 |
| `frontend/` | React 18 + Vite + Ant Design Pro | 商家后台与平台审核 |
| `miniapp/` | Taro 3.6（微信 / 抖音） | 买家浏览、收藏、历史、门店联系 |

### 2.1 已实现的主业务链路

1. 商家注册 → 管理员审核 → 商品草稿 / 上下架 / 关闭
2. 轻量订单：创建后锁定商品 → 完成或关闭
3. 买家小程序：浏览、收藏 / 历史（游客 + 登录合并）、意向或电话联系门店
4. 文件：presign → multipart 上传 → 本地存储（可扩展 OSS）与图片压缩（vips）

### 2.2 架构印象

- 后端按 handler 聚合，状态机抽离为 `stateflow`，具备幂等、操作日志、商家 scope（`onboarding` / `full`），业务骨架清晰。
- `internal/app` 体积偏大（例如 `buyer_handlers.go` 超过千行），分层偏薄，后续扩展成本会上升。
- 文档体量大，但早期范围说明与现状已明显漂移（例如 overview 仍写不含买家端，实际已有完整 miniapp）。
- 设计文档提到 Redis 会话 / 限流与对象存储抽象；实现侧主要是内存限流与本地 `uploads` 静态托管。

### 2.3 分维度评价

| 维度 | 评级 | 说明 |
| --- | --- | --- |
| 业务完整度 | 中高 | 商家经营 + 买家浏览主链路基本齐全 |
| 代码结构 | 中 | 可读，但后端单包过大、分层偏薄 |
| 安全 | 低（阻断） | 默认密钥 / 管理员、文件越权、匿名上传 |
| 数据一致性 | 中偏低 | 订单唯一索引、库存模型有硬伤 |
| 测试 | 中 | 后端集成 / 安全测试较扎实；前端弱；若干边界缺回归 |
| 运维 / 发布 | 中偏低 | migration 与 AutoMigrate 双轨；本地业务库曾被 Git 跟踪 |
| 文档 | 中 | 规格细，但与实现 / 范围不一致处较多 |

## 3. 问题汇总

| 编号 | 级别 | 问题 | 影响 |
| --- | --- | --- | --- |
| R-01 | P0 | 默认密钥和固定管理员凭据可进入生产 | 管理端可能被直接接管 |
| R-02 | P1 | 商品图片及营业执照 file_id 缺少归属 / 类型 / 扫描状态校验 | 跨商家文件越权与敏感文件泄露 |
| R-03 | P1 | 订单唯一索引不允许同一商品存在第二笔历史订单 | 第二笔订单无法关闭或完成 |
| R-04 | P1 | 管理员审核页无法查看营业执照 | 核心资质审核流程不可执行 |
| R-05 | P1 | miniapp 的 401 自动刷新分支不可达 | access token 过期后会话无法恢复 |
| R-06 | P1 | 匿名上传缺少限流、配额和请求体前置限制 | 磁盘 / 内存资源可被耗尽 |
| R-07 | P1 | 库存字段与订单完成逻辑冲突 | 多库存商品首次成交后即错误售罄 |
| R-08 | P2 | frontend 退出登录未注销服务端 session | refresh token 在退出后仍可使用 |
| R-09 | P2 | SQL migration 表名与 GORM 默认表名不一致 | 纯 migration 部署时上传链路可能不可用 |
| R-10 | P2 | Git 跟踪本地业务数据库 | 业务数据与会话元数据可能进入仓库历史 |
| R-11 | P2 | 买家小程序登录默认 mock 模式 | 生产漏配时可用任意 code 登录 |
| R-12 | P2 | 资质文件经 `/uploads` 公开可读 | 执照直链可被猜测或枚举访问 |
| R-13 | P2 | 文档与实现范围漂移 | 容易误导排期、验收与运维假设 |
| R-14 | P2 | 缺少可见的仓库级 lint / CI 与前端测试深度 | 回归依赖人工，前端质量信号弱 |
| R-15 | P3 | 意向页面代码存在但未注册到小程序页面配置 | 死代码或半成品入口，增加维护噪音 |
| R-16 | P3 | Access JWT 不校验 session 吊销状态 | logout 后 access 在 TTL 内仍可用 |
| R-17 | P3 | 分类按全局 name 查找、幂等表无清理、多实例内存限流失效 | 长期运维与数据一致性隐患 |

## 4. 详细发现

### R-01 [P0] 默认密钥和固定管理员凭据可进入生产

证据：

- `backend/internal/app/config.go` 为 DB DSN、access secret、refresh secret 和买家登录模式提供开发默认值。
- `backend/internal/app/server.go` 的 `seedDefaults` 在空库中创建 `admin` / `superadmin`，固定密码 `Admin@123456`。
- `README.md` 明文记录默认管理员账号。
- 生产启动路径没有“弱默认值拒绝启动”的校验。

影响：

新生产环境如果漏配环境变量，攻击者可使用公开固定密码登录，或在默认 JWT secret 未替换时伪造管理员 token。

### R-02 [P1] 文件 ID 缺少归属、类型和扫描状态校验

证据：

- `backend/internal/app/product_handlers.go` 创建 / 编辑商品时直接保存请求中的 `image_file_ids`。
- `backend/internal/app/auth_handlers.go` 与商家重提交流程接收 `license_file_id`，未见上传主体与业务类型校验。
- 买家侧会按商品关联的 file id 解析并返回文件 URL。
- 上传成功后 `scan_status` 直接记为 `PASS`，无真实内容安全扫描。

缺失检查至少包括：

- 文件记录是否存在
- 文件是否属于当前账号或商家
- 商品图片是否为 `PRODUCT_IMAGE`
- 营业执照是否为 `MERCHANT_LICENSE`
- 扫描状态是否允许对外绑定 / 展示

影响：

顺序文件 ID 可被猜测。恶意商家可以引用其他商家图片，或把营业执照挂到公开商品上，再通过买家接口暴露 URL。

### R-03 [P1] 同一商品的第二笔历史订单无法关闭或完成

证据：

- `backend/internal/model/models.go` 为订单建立 `(product_id, is_active)` 唯一索引 `uk_product_active`。
- `backend/internal/app/order_handlers.go` 在订单关闭或完成时将 `is_active` 更新为 `false`。

该索引本意是限制“同一商品仅一笔活动订单”，但把 `is_active=false` 也纳入唯一约束后，会错误限制历史 inactive 订单数量。商品经历第一笔关闭订单后可以重新上架并创建第二笔订单，但第二笔再转为 inactive 时会与第一笔历史订单冲突。

同类模式风险：

- `BuyerIntent` 使用 `(buyer_id, product_id, is_open)` 唯一索引，多次关闭后再开/再关时也可能撞约束。

### R-04 [P1] 管理员无法查看营业执照

证据：

- 产品规格要求审核详情展示证照图片。
- `backend/internal/app/admin_handlers.go` 审核详情只返回 `license_file_id`。
- `frontend/src/pages/admin/merchants/ReviewDetailPage.tsx` 只显示文件数字 ID，没有图片 URL 或预览入口。

影响：

管理员可以点击通过或驳回，但无法看到审核所需的核心资质材料，审核流程在业务上不完整。

### R-05 [P1] miniapp 的 401 自动刷新分支不可达

证据：

- `miniapp/src/services/request.ts` 在 HTTP 非 2xx 时立即抛错。
- 其后才根据业务码 `10002` 尝试 refresh。
- 后端未授权响应使用 HTTP 401。
- 对比：`frontend/src/services/http.ts` 已正确在 axios 错误分支处理 401 刷新与并发单飞。

影响：

access token 过期时，请求在到达刷新逻辑前已经失败。客户端仍保留旧 session，后续页面会持续携带过期 token，除非用户手动重新登录或清理本地状态。

### R-06 [P1] 匿名上传存在资源耗尽风险

证据：

- `backend/internal/app/server.go` 注册公开的 `/files/presign` 与 `/files/upload`。
- `backend/internal/app/file_handlers.go` 允许匿名用户上传 `MERCHANT_LICENSE`。
- 业务文件大小检查发生在 multipart 解析之后；未见统一请求体前置上限。
- `memoryRateLimiter` 主要用于买家收藏 / 意向 / 登录等接口，上传与注册路径基本未覆盖。
- 未见未绑定文件的定期清理机制。

影响：

攻击者可反复上传，或提交远大于业务限制的 multipart 请求，先消耗内存 / 临时磁盘再被业务层拒绝。

### R-07 [P1] 库存模型与订单状态不一致

证据：

- DTO 与前端商品表单允许任意正库存。
- 订单完成后直接将商品状态设为 `SOLD`，没有扣减库存。
- 产品文档仍偏“二手商品库存为 1”的模型。

影响：

库存为 2 的商品售出 1 件后，数据库库存仍为 2，但商品状态已经不可继续销售，展示数据与经营状态同时失真。

### R-08 [P2] frontend 退出登录没有注销服务端 session

证据：

- `frontend/src/app/Layout.tsx` 退出时只清理本地状态并跳转登录页。
- 前端 API 封装未在退出路径调用 `/auth/logout`。
- 后端与 miniapp 已具备 logout 能力；买家流程测试覆盖了 logout 后 refresh 失效。

影响：

界面看起来已退出，但对应 refresh token 与服务端 session 仍然有效。此前泄露的 refresh token 仍可继续换取 access token。

### R-09 [P2] SQL migration 与 GORM 表名不一致

证据：

- `backend/migrations/0001_init.up.sql` 创建表 `files`。
- `backend/internal/model/models.go` 中 `FileRecord` 未定义 `TableName()`，GORM 默认使用 `file_records`。
- 运行时默认 `AUTO_MIGRATE=true`，本地 SQLite 路径依赖 AutoMigrate 更容易“看起来正常”。

影响：

若生产只执行 SQL migration 并关闭 AutoMigrate，文件相关接口会访问不存在的 `file_records` 表。同时维护手写 migration 与 AutoMigrate 会持续放大 schema 漂移。

### R-10 [P2] Git 跟踪本地业务数据库

证据：

- 审查时 `backend/app.db` 处于 Git 跟踪状态（约 344KB）。
- `.gitignore` 已包含 `backend/app.db` 与 `backend/*.db` 规则，但历史提交中仍可能保留副本。
- `frontend/.env.development` 与 `frontend/.env.production` 也被跟踪；当前内容主要是 API 前缀，敏感度较低，但习惯不佳。

影响：

账号信息、联系方式、会话哈希和文件元数据可能已进入仓库及历史提交。仅从工作区删除无法清除历史副本。

### R-11 [P2] 买家登录默认 mock 模式

证据：

- `BUYER_WECHAT_LOGIN_MODE` / `BUYER_DOUYIN_LOGIN_MODE` 默认 `mock`。
- mock 路径将 code 映射为稳定假 openid（例如 `mock_wx_` + code），不访问第三方。

影响：

生产若未显式切到 `real` 并配置密钥，任意知道接口的人都可以用自选 code 完成“登录”。

### R-12 [P2] 资质文件公开静态访问

证据：

- 本地存储模式下服务注册 `Static("/uploads", ...)`。
- 营业执照与商品图共用可猜测 / 可枚举的 object key 空间，并生成公开 URL。

影响：

即使管理端不展示执照预览，直接访问上传 URL 仍可能读到敏感资质图片。

### R-13 [P2] 文档与实现漂移

典型不一致：

| 文档说法 | 实际实现 |
| --- | --- |
| 本期不含买家端 | 已有完整 miniapp 与 buyer API |
| Redis 会话 / 限流 | DB session + 进程内内存限流 |
| 对象存储抽象 | 本地 `uploads` + 静态目录 |
| 管理员仅 bootstrap 脚本预置 | 启动时固定密码 seed |
| 审核展示证照 | 仅返回 / 展示 file id |
| 单商品单活动订单 | 唯一索引误伤历史 inactive 订单 |

文档更适合作为“意图说明书”，不能直接当作当前实现的单一事实来源。

### R-14 [P2] 质量门禁偏弱

证据：

- 后端有较完整的 `tests/` 集成与安全用例。
- frontend 仅少量 colocated 测试（登录、路由守卫、错误码）。
- miniapp 近期补了 toolchain、布局、session、API base 等针对性测试，覆盖面仍偏“点状”。
- 仓库未见统一 lint / CI 配置作为默认质量门禁。

影响：

P0/P1 类边界问题缺少强制回归；前端与跨端契约更容易静默退化。

### R-15 [P3] 意向页代码与小程序页面注册不一致

证据：

- `miniapp/src/pages/intent/` 目录存在。
- `miniapp/src/app.config.ts` 的 `pages` 未注册意向相关页面。
- 近期改动将联系方式集中到门店电话常量，并有“意向入口隐藏”相关测试。

影响：

更像产品策略切换后的残留，不一定是线上缺陷，但会增加后续维护和误用成本。

### R-16 [P3] Access token 与 session 吊销不同步

证据：

- logout 将 `auth_sessions.revoked_at` 置位。
- 受保护请求的中间件只解析并信任 access JWT 签名与过期时间，不查询 session 是否已撤销。

影响：

主动退出或强制下线后，access token 在剩余 TTL（默认 2 小时）内仍可调用受保护接口。对管理端与商家端尤其敏感。

### R-17 [P3] 其他工程质量项

- `ensureDefaultCategories` / `findOrCreateCategory` 按全局 `name` 查找，跨父级同名分类可能串数据。
- `IdempotencyRecord` 无过期清理策略，表会持续增长。
- 内存限流在多实例部署下不共享，进程重启后计数归零。
- Soft delete 与 username 唯一索引叠加后，删除账号会永久占用用户名。
- Gin 固定 `ReleaseMode`，本地排障体验一般。
- miniapp 将 Node / npm 版本钉死，有利于可复现构建，但对协作环境要求更高。

## 5. 做得好的地方

1. **角色与 scope 分层清楚**：商家 `onboarding` / `full` 与 `RequireFullMerchantScope` 使用到位。
2. **商品 / 订单状态机独立**：`stateflow` 与字段级可编辑矩阵比散落条件判断更可维护。
3. **后端测试有分量**：restricted login、买家登录刷新 logout、上传与集成流都有覆盖。
4. **买家域设计用心**：device 游客、登录 merge、收藏 / 历史 owner_key、意向限流与幂等。
5. **小程序工程近期质量在提升**：toolchain lock、home grid、API base 解析、session 启动安全等有针对性测试。
6. **上传路径防穿越**与图片 MIME / 压缩策略有认真实现。
7. **frontend 401 刷新**实现正确（含并发单飞），可作为 miniapp 对齐参考。

## 6. 测试与验证缺口

现有测试通过（或局部通过）不能证明下列边界正确，审查时至少观察到这些缺口：

- 空配置 / 弱默认值的生产启动必须失败
- 默认管理员初始化与首次改密策略
- 商品图片及营业执照的跨商家越权绑定
- 错误文件类型、扫描未通过文件和不存在文件的绑定
- 同一商品连续创建并关闭或完成两笔订单
- miniapp 请求层收到 HTTP 401 后刷新并重放请求
- 多个并发 401 只触发一次 refresh
- 匿名上传请求体上限、频率和未绑定文件清理
- 库存大于 1 时的下单、扣减和售罄状态
- frontend 退出登录调用服务端 logout
- 仅通过 SQL migrations 初始化空库后的文件上传流程
- mock 登录模式在生产配置下被拒绝

补充说明：同日另一份审查记录中，backend 与 miniapp 测试 / 构建曾通过；frontend 因本机 FileProvider 依赖映射异常未能取得可信结果。该环境问题不代表 frontend 本身通过验证。

## 7. 与文档宣称的差距（摘要）

1. 仓库已从“商家后台”扩展为“商家后台 + 买家小程序”，但部分总览文档未同步。
2. 非功能设计中的 Redis、对象存储、即时吊销等能力，实现完整度低于文档表述。
3. 审核、库存、单活动订单等关键业务规则，文档意图与代码约束不完全一致。

## 8. 发布判断

| 类别 | 编号 | 发布态度 |
| --- | --- | --- |
| 阻断 | R-01 ~ R-07 | 修复前不应上生产 |
| 上线前应处理 | R-08 ~ R-12 | 否则存在会话、数据、隐私与运维风险 |
| 应纳入后续治理 | R-13 ~ R-17 | 不影响“能否跑起来”，但影响长期正确性与可维护性 |

**最终判断：当前版本不满足生产发布条件。**

## 9. 审查边界与说明

1. 本次为深度静态审查与路径复核，不替代渗透测试、压测和完整 E2E。
2. 未在本轮重新完整执行全量 frontend / backend / miniapp 测试矩阵；验证缺口见第 6 节。
3. 未对远端生产环境配置做实地核对；若生产已人工规避部分默认值问题，仍需以代码路径是否强制约束为准。
4. 本文不包含修复排期、任务拆分或实施计划。
