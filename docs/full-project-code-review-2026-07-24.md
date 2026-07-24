# 全项目代码审查报告

- 审查日期：2026-07-24
- 审查基线：`master` / `1bde7d9`
- 线上复核基线：远程部署目录 `c5381d1`；运行镜像未携带 Git commit 标签，不能据此严格证明镜像来源
- 审查范围：`backend/`、`frontend/`、`miniapp/`、`migrations/`、`scripts/`、`docs/` 及仓库配置
- 交叉复核：`docs/deep-code-review-2026-07-24.md`（Grok）
- 审查方式：静态代码审查、两份审查结论交叉核对、需求与接口契约对照、自动化测试、构建验证、针对性数据库复现，以及生产服务器只读 SSH / HTTP / schema / 聚合数据核验
- 总体结论：服务当前可用不等于已经完成生产验收，但也不需要无序停站重构。当前应先处理 mock 身份、公开资质文件、公网 8081、示例数据库口令和已出现的库存不一致；其余问题按受影响业务流程和下一次发布分批处理。

## 1. 审查范围

本次审查针对项目从零到当前交付状态的整体实现，不局限于近期提交或单个模块，重点覆盖以下方面：

- 后端配置加载、启动初始化、路由和中间件
- JWT、refresh token、服务端 session、角色和资源权限
- 商家注册、审核、商品、文件上传、订单和买家流程
- frontend 路由保护、登录刷新、退出登录、商家与管理员页面
- miniapp 会话存储、请求封装、登录、商品、收藏和多端构建配置
- SQL migrations 与 GORM 模型的一致性
- 产品需求、接口文档和实际实现的一致性
- 仓库中的敏感文件、数据库和生产配置风险

问题证据按以下强度记录：

- **已复现**：通过最小数据库场景或现有测试路径确认会出现错误。
- **静态确认**：控制流、数据流或路由配置足以证明问题成立。
- **线上已确认**：生产配置、schema、非敏感聚合数据或只读 HTTP 检查直接证明问题适用于当前环境。
- **线上适用但未触发**：缺陷路径和线上约束都存在，但当前数据尚未到达触发条件。
- **线上已缓解**：代码仍有风险，但当前部署的外部配置降低了其中一部分可利用性；不代表代码问题已修复。
- **工程风险**：依赖部署拓扑、外部配置或未来扩展，不能等同于当前必现缺陷。

本报告的优先级表示当前环境的处置窗口，不表示所有 P1 都要同时停站重写：

- **P0**：已确认正在发生的系统接管、严重数据破坏或大规模敏感数据泄露，需要立即停用相关入口。当前只读证据没有确认 P0。
- **P1**：应立即止血，或必须在对应业务流程再次发生前修复；可以是小范围配置、数据或迁移操作。
- **P2**：纳入下一次相关版本或运维窗口，不是当前停站条件。
- **P3**：扩展、可维护性或纵深防御治理项。

## 2. 问题汇总

| 编号 | 级别 | 问题 | 当前处置窗口 |
| --- | --- | --- | --- |
| F-01 | P1 | 弱默认配置和固定管理员凭据可进入生产 | 立即验证并轮换实际管理员口令；代码防护随下次后端发布 |
| F-02 | P2 | 商品图片及营业执照文件 ID 缺少归属校验 | 下一次商家入驻或新增不受信任商家前 |
| F-03 | P1 | 订单唯一索引既阻止同商品多笔 active 订单，也不允许第二笔历史订单 | 商家多库存订单发布前 |
| F-04 | P2 | 管理员审核页无法查看营业执照 | 下一次商家资质审核前 |
| F-05 | P2 | miniapp 的 401 自动刷新分支不可达 | 下一次 miniapp 发布 |
| F-06 | P2 | 匿名上传缺少限流、配额和清理机制 | 下一次后端/网关发布；当前保留 10 MB 入口限制 |
| F-07 | P1 | 后台允许多库存，但订单没有数量、预占和扣减逻辑 | 立即核对 3 个异常商品，商家多库存订单发布前修正规则 |
| F-08 | P2 | frontend 退出登录未注销服务端 session | 下一次 frontend 发布 |
| F-09 | P2 | migration 表名与 GORM 表名不一致 | 下一次新建数据库或关闭 AutoMigrate 前 |
| F-10 | P2 | Git 跟踪本地业务数据库 | 下一次共享仓库历史或轮换会话前完成处置 |
| F-11 | P2 | 买家意向唯一索引只允许一笔已关闭历史记录 | 恢复买家意向入口前 |
| F-12 | P1 | 生产买家登录使用 mock 身份 | 立即限制新 mock 身份，完成迁移方案后再切 real |
| F-13 | P1 | 营业执照通过公开静态目录匿名可读 | 立即隔离现有 1 份执照，不影响商品图片 |
| F-14 | P2 | 注销 session 后 access token 仍可继续访问 | 下一次认证发布；先确认可接受的吊销时延 |
| F-15 | P2 | 幂等记录在业务执行后写入且忽略写入错误 | 关键写流量扩大或依赖幂等保证前 |
| F-16 | P3 | 分类模型、migration 和初始化查询的唯一性语义不一致 | 分类扩展或 schema 迁移前 |

部署环境另有 7 项独立发现，避免与代码缺陷编号混用：

| 编号 | 级别 | 问题 | 当前处置窗口 |
| --- | --- | --- | --- |
| D-01 | P1 | 公网 `8081` 可绕过 HTTPS 和宿主 Nginx | 验证域名入口后立即收口端口 |
| D-02 | P1 | 生产仍用示例数据库口令，部署配置未版本化且权限偏宽 | 立即轮换在用口令/收紧权限；配置版本化后续完成 |
| D-03 | P2 | 应用、宿主 Nginx、容器 Nginx 的上传上限互相冲突 | 下一次网关发布 |
| D-04 | P2 | 未找到可确认的异机或离线数据库/上传文件备份 | 先核实云快照/root 任务；确无备份再补最小方案 |
| D-05 | P2 | API/Web 无 healthcheck、资源限制和日志轮转 | 下一次运维窗口 |
| D-06 | P3 | 运行镜像没有 commit 标签或构建版本标签 | 建立下一版镜像发布流程时 |
| D-07 | P3 | HTTPS 响应缺少 HSTS、CSP 等安全响应头 | 入口收口后的常规加固窗口 |

## 3. 详细发现

### F-01 [P1；若默认管理员口令仍有效则升级 P0] 弱默认配置和固定管理员凭据可进入生产

证据：

- `backend/internal/app/config.go:41` 为数据库密码、access secret 和 refresh secret 提供开发默认值。
- `backend/internal/app/server.go:145` 在服务启动时执行默认数据初始化。
- `backend/internal/app/server.go:151` 在空库中创建 `admin` 和 `superadmin`，并写入仓库公开的固定初始化密码；报告不重复记录该值。
- `backend/scripts/bootstrap_admin/main.go:36` 的独立初始化脚本也使用相同固定密码。
- `README.md:144` 公开记录默认管理员账号，启动路径没有生产模式的弱默认值校验。

影响：

新生产环境如果漏配环境变量，攻击者可以使用公开的固定密码登录，或在默认 JWT secret 未替换时伪造管理员 token。当前实现没有在生产模式下拒绝弱默认值，也没有强制首次登录修改密码。

线上补充：access/refresh secret 已替换且不是开发默认值，这一部分已由配置缓解；但 DB DSN 仍使用仓库示例口令，两个管理员账号共享同一密码哈希，且代码没有管理员改密入口。为保持只读，本次没有尝试使用默认口令登录，因此不能声称默认管理员口令已被直接验证，仍应在发布前完成显式轮换证明。

建议：

1. 先由运维直接确认两个现有管理员是否仍使用初始化口令；无论验证结果如何，都轮换管理员口令和当前应用/root 数据库口令，并留下不含秘密的验证记录。JWT secret 已经替换，没有泄露证据时不需要为追求形式完整而再次盲目轮换。
2. 下一次后端发布增加生产配置校验：数据库密码、access secret、refresh secret 和管理员初始化口令不得采用开发默认值；固定密码 seed 改为显式的一次性初始化命令或随机临时凭据。
3. 管理员改密和首次登录强制改密可随认证版本补齐。完整 secret manager 属于后续部署治理，不是完成本次止血的前置条件。

### F-02 [P2] 文件 ID 缺少归属、类型和扫描状态校验

证据：

- `backend/internal/app/product_handlers.go:62` 创建商品时直接保存请求中的 `image_file_ids`。
- `backend/internal/app/product_handlers.go:162` 编辑商品时存在相同行为。
- `backend/internal/app/buyer_handlers.go:352` 根据商品关联的文件 ID 解析并返回文件 URL。
- `backend/internal/app/auth_handlers.go:45` 和 `backend/internal/app/merchant_handlers.go:63` 接收营业执照文件 ID，但未验证上传主体和业务类型。

缺失的检查包括：

- 文件记录是否存在
- 文件是否属于当前账号或商家
- 商品图片是否为 `PRODUCT_IMAGE`
- 营业执照是否为 `MERCHANT_LICENSE`
- 文件扫描状态是否为 `PASS`

影响：

顺序文件 ID 可被猜测。恶意商家可以引用其他商家的图片，或把营业执照等文件挂到公开商品上，再通过买家接口暴露文件 URL。

建议：

建立统一的文件绑定校验函数，在数据库事务内检查文件存在性、上传主体、商家归属、业务类型和扫描状态，再允许写入业务记录。

### F-03 [P1] 订单唯一索引与多库存、多订单需求冲突

证据：

- `backend/internal/model/models.go:174` 为订单建立 `(product_id, is_active)` 唯一索引。
- `backend/internal/app/order_handlers.go:202` 在订单关闭或完成时将 `is_active` 更新为 `false`。

该索引限制一个商品只能有一笔 active 订单，也错误地限制一个商品只能有一笔 inactive 订单。产品规则现已确认为同一商品可以有多件库存和多笔商家后台订单，因此“每个商品最多一笔 active 订单”本身也不再成立。即使按旧单 active 流程，第二笔订单转为 inactive 时仍会与第一笔历史订单冲突。

复现结果：

在 SQLite 中创建第一笔 inactive 订单和第二笔 active 订单后，将第二笔更新为 inactive，数据库返回：

```text
UNIQUE constraint failed: orders.product_id, orders.is_active
```

建议：

删除 `uk_product_active` 唯一索引，改为 `(product_id, is_active)` 普通查询索引；订单增加 `quantity`，商品增加 `reserved_stock`，通过事务内条件更新保证预占总量不超过库存。第一版停止使用单值 `active_order_id`，但暂不删除列，以降低回滚成本。

线上目前 2 笔历史订单分属不同商品且均为 inactive，可将历史 `quantity` 回填为 1。完整迁移与并发控制见 `docs/production-hardening-repair-plan-2026-07-24.md`，必须在商家后台开放多库存订单前完成。

### F-04 [P2] 管理员无法查看营业执照

证据：

- `docs/specs.md:69` 要求审核详情展示证照图片。
- `backend/internal/app/admin_handlers.go:80` 审核详情只返回 `license_file_id`。
- `frontend/src/pages/admin/merchants/ReviewDetailPage.tsx:142` 只显示文件数字 ID，没有图片 URL 或预览入口。

影响：

管理员可以点击审核通过或驳回，但无法看到审核所需的核心资质材料，审核流程在业务上不完整。

建议：

后端返回经过授权的文件访问地址或短期签名 URL；前端提供图片预览、加载失败状态，并保留文件 ID 供审计排查。

### F-05 [P2] miniapp 的 401 自动刷新分支不可达

证据：

- `miniapp/src/services/request.ts:137` 遇到 HTTP 非 2xx 响应时立即抛出异常。
- `miniapp/src/services/request.ts:149` 才处理业务码 `10002` 并刷新 token。
- 后端未授权响应使用 HTTP 401。

影响：

access token 过期时，请求在到达刷新分支前已经失败。客户端仍保留旧 session，后续页面会持续携带过期 token，除非用户手动重新登录或清理本地状态。

建议：

在抛出 HTTP 错误前识别 401/`10002`，执行一次并发安全的 token 刷新并重放原请求；刷新失败时统一清理会话并进入登录流程。增加请求层单元测试，覆盖并发 401、刷新失败和避免无限重试。

### F-06 [P2] 匿名上传存在资源耗尽风险

证据：

- `backend/internal/app/server.go:264` 注册公开上传路由。
- `backend/internal/app/file_handlers.go:30` 允许匿名用户上传 `MERCHANT_LICENSE`。
- `backend/internal/app/file_handlers.go:121` 在 `FormFile` 解析 multipart 后才检查业务文件大小。

影响：

匿名调用没有频率限制、总量配额、一次性上传凭证或未绑定文件清理机制。攻击者可反复上传，也可以提交远大于业务限制的 multipart 请求，让框架先消耗内存或临时磁盘再拒绝。

建议：

在路由或反向代理层限制请求体大小；对匿名上传实施 IP/设备限流和短期上传凭证；定期清理未绑定文件，并设置账号、商家和全局存储配额。

### F-07 [P1] 多库存字段没有对应的订单数量和库存事务

证据：

- `backend/internal/dto/dto.go:49` 允许任意正库存。
- `frontend/src/pages/merchant/products/CreatePage.tsx:127` 和 `frontend/src/pages/merchant/products/EditPage.tsx:176` 只把库存限制为不小于 1，仍允许提交大于 1 的值。
- `backend/internal/app/order_handlers.go:205` 完成一笔订单后直接把整个商品设置为 `SOLD`，没有扣减库存。
- 产品文档仍把二手商品库存描述为固定 1。

影响：

库存为 2 的商品售出 1 件后，数据库库存仍为 2，但商品状态已经不可继续销售，展示数据和业务状态同时失真。

线上补充：9 个未删除商品中有 3 个库存不为 1；2 个仍为 `ON_SHELF`，最大库存 28，另有 1 个已经是 `SOLD` 但库存仍为 6。该问题已经产生线上不一致数据，不再只是模型推演。

建议：

产品规则已确认为商家后台支持同商品多件库存和多笔订单，小程序本轮不直接下单。订单增加正整数 `quantity`，`deal_price_cent` 表示单件成交价，总价由服务端计算；商品增加预占库存，创建订单预占、关闭释放、完成扣减，只有总库存归零才标记 `SOLD`。

现有 3 条异常商品不能再直接视为脏数据并归一为 1，应由商家核对真实总库存。其中已经 `SOLD` 但库存仍为 6 的记录仍然矛盾，修正前应冻结相关成交入口。详细数据模型、迁移和回滚方案见 `docs/production-hardening-repair-plan-2026-07-24.md`。

### F-08 [P2] frontend 退出登录没有注销服务端 session

证据：

- `frontend/src/app/Layout.tsx:37` 退出时只清理 localStorage 并跳转。
- `frontend/src/services/api.ts` 没有调用 `/auth/logout` 的退出方法。

影响：

用户界面看起来已经退出，但对应 refresh token 和服务端 session 仍然有效。此前泄露的 refresh token 仍可继续换取 access token。

建议：

退出时先调用服务端 logout，再清理本地状态；即使服务端请求失败也应完成本地退出。补充测试验证 logout 请求和异常降级行为。

### F-09 [P2] SQL migration 与 GORM 表名不一致

证据：

- `backend/migrations/0001_init.up.sql:150` 创建表 `files`。
- `backend/internal/model/models.go:201` 定义 `FileRecord`，但没有 `TableName()`，GORM 默认使用 `file_records`。
- 当前由 AutoMigrate 生成的 SQLite 数据库实际使用 `file_records`。

影响：

如果生产环境只执行 SQL migration 并关闭 AutoMigrate，文件相关接口会访问不存在的 `file_records` 表。继续同时依赖手写 migration 和 AutoMigrate 还会扩大 schema 漂移。

建议：

选定唯一 schema 管理方式，并使模型表名与 migration 完全一致。为全新数据库增加从 migration 启动服务的集成测试。

### F-10 [P2] Git 跟踪本地业务数据库

证据：

- `backend/app.db` 处于 Git 跟踪状态，虽然当前 `.gitignore` 已包含数据库忽略规则。
- 审查时数据库中可见商家、买家、auth session 和文件记录。

影响：

账号信息、联系方式、会话哈希和文件元数据可能已进入仓库及历史提交。只在新提交删除文件无法清除 Git 历史中的副本。

建议：

确认数据是否真实后，从当前跟踪和必要的 Git 历史中清理数据库；轮换可能受影响的密钥和会话；使用不含业务数据的 fixture 初始化开发环境。

### F-11 [P2] 买家意向唯一索引只允许一笔已关闭历史记录

证据：

- `backend/internal/model/models.go:309` 为 `BuyerIntent` 建立 `(buyer_id, product_id, is_open)` 唯一索引。
- `backend/internal/app/merchant_intent_handlers.go:226` 关闭意向时将 `is_open` 更新为 `false`。
- `backend/tests/buyer_flow_test.go:396` 验证第一笔意向关闭后可以创建第二笔，但测试没有继续关闭第二笔。

影响：

该索引与订单索引存在相同问题：同一买家对同一商品只能保存一笔 `is_open=false` 的历史意向。第二笔意向可以创建，但关闭时会与第一笔历史记录冲突。该错误已用同结构 SQLite 表复现：

```text
UNIQUE constraint failed: buyer_intents.buyer_id, buyer_intents.product_id, buyer_intents.is_open
```

当前 miniapp 有意隐藏了意向入口，因此严重级别低于订单问题；但后端 API、商家页面和自动化测试仍把该流程作为已实现能力。

建议：

与订单迁移采用同一小范围方案：在 MySQL 8 增加生成的可空 `open_marker`，open 行取固定非空值，closed 行取 `NULL`，唯一约束改为 `(buyer_id, product_id, open_marker)`。先建立并验证新索引，再删除旧三列唯一索引，并把测试延伸到“创建第一笔、关闭、创建第二笔、再次关闭”的完整周期。线上 `buyer_intents=0`，无需数据清理；该迁移只需在恢复意向入口前完成。

### F-12 [P1] 买家小程序登录默认使用 mock 身份

证据：

- `backend/internal/app/config.go:54` 和 `backend/internal/app/config.go:59` 将微信、抖音登录模式默认设置为 `mock`。
- `backend/internal/app/miniapp_auth.go:116` 和 `backend/internal/app/miniapp_auth.go:175` 在 mock 模式下直接把客户端 code 拼接成 openid，不向平台校验 code。
- 启动过程没有生产环境标识，也没有拒绝 mock 登录模式的配置校验。

影响：

生产漏配时，调用者可用任意非空 code 创建买家身份。该问题不等同于管理员接管，但会绕过微信/抖音身份真实性，影响买家资料、意向、收藏、历史和滥用追踪。线上明确配置为微信 `mock`，抖音变量未配置且回落到 `mock`；现有 19 个买家账号全部为微信 mock openid，因此从 P2 提升为当前生产 P1。

建议：

不能在现状上直接把配置从 `mock` 裸切为 `real`：mock openid 无法自动映射到微信真实 openid，直接切换会让现有 19 个买家形成新的账号，收藏、浏览历史和其他归属数据也可能被割裂。应按以下顺序迁移：

1. 先在非生产环境验证 AppID、AppSecret、平台合法域名、code 换取会话和回滚配置；临时限制生产继续创建新的 mock 身份。
2. 盘点 19 个 mock 买家的收藏、浏览历史和其他归属数据，按实际数据价值选择“旧账号重置”或“用户完成真实登录后一次性绑定/合并”，并保存迁移前备份和审计记录。
3. 完成少量账号试迁移和回滚验证后再切换生产 `real`；随后增加生产启动拒绝 mock 的校验。mock 模式仅保留在显式开发/测试环境。

### F-13 [P1] 营业执照通过无鉴权静态目录公开

证据：

- `backend/internal/app/server.go:66` 将整个本地上传目录注册为公开 `/uploads` 静态路由。
- `backend/internal/app/file_handlers.go:85` 把营业执照和商品图存入同一公开文件空间，只通过业务类型目录区分。
- `backend/internal/common/idgen.go:11` 生成的文件业务号由秒级时间和四位进程序列组成，不是不可预测的随机标识。
- `backend/internal/app/file_handlers.go:304` 为所有本地文件返回永久公开 URL，没有授权或有效期。

影响：

知道、泄露或预测到 URL 的匿名访问者可以绕过管理员权限直接读取营业执照。F-04 要求的是受授权的审核预览，当前公开静态 URL 不能作为合规替代。

线上补充：生产使用 `FILE_STORAGE_PROVIDER=local`，公开前缀为 `https://market.meaningful.ink/uploads`。在不输出 URL、文件名或内容的前提下，对现有一条 `MERCHANT_LICENSE/PASS` 记录执行匿名 GET，返回 `HTTP 200` 和 `image/png`，问题已实际确认，因此提升为 P1。

建议：

先提供管理员鉴权预览接口，或在迁移窗口内提供仅管理员可访问的临时受限入口。随后只移动或改名线上现有 1 份营业执照、更新对应文件记录，并确认旧公开 URL 已不可访问。商品图片继续走公开静态目录，不需要全局轮换所有上传 URL。

后续新上传的资质文件应进入私有路径，通过管理员鉴权接口或短期签名 URL 访问；高熵 object key 和访问审计可随文件存储治理补齐。

### F-14 [P2] 注销 session 后 access token 仍然有效

证据：

- `backend/internal/app/auth_handlers.go:262` 注销时只给 `auth_sessions.revoked_at` 赋值。
- `backend/internal/middleware/auth.go:26` 受保护请求只验证 access JWT 的签名和过期时间，不读取 session 或账号状态。
- `backend/internal/app/config.go:44` 默认 access token 有效期为 2 小时。

影响：

即使客户端正确调用 logout，旧 access token 在过期前仍可访问受保护接口；账号在签发 token 后被禁用，也不会立即反映到现有 access token。refresh token 会被正确拒绝，但这不是即时吊销。

建议：

明确产品接受的吊销时延。管理端若要求即时退出或强制下线，应在中间件校验 session/账号状态，或采用短 access TTL 加集中式吊销版本。至少增加“logout 后旧 access token”的契约测试。

### F-15 [P2] 幂等记录写入不是原子操作

证据：

- `backend/internal/app/idempotency.go:22` 先查询幂等记录。
- `backend/internal/app/idempotency.go:39` 在不存在记录时先执行实际业务操作。
- `backend/internal/app/idempotency.go:44` 业务成功后才写幂等记录，并直接忽略 `Create` 错误。
- `backend/internal/model/models.go:328` 的幂等记录没有过期字段，仓库中也没有清理任务。

影响：

两个并发同 key 请求可以同时读到“记录不存在”并进入业务函数，无法保证文档声明的“只执行一次”。即使唯一索引最终阻止第二条幂等记录，第二次业务副作用可能已经发生。记录还会永久保存响应 JSON，数据量持续增长。

建议：

在业务执行前原子占用幂等 key，记录 `PROCESSING/SUCCEEDED/FAILED` 状态，并让相同 key 的并发请求等待或读取最终结果；不能忽略记录写入错误。增加保留期限和清理任务，并添加并发回归测试。

### F-16 [P3] 分类唯一性语义在三处不一致

证据：

- `backend/migrations/0001_init.up.sql:71` 定义 `(parent_id, name)` 复合唯一索引。
- `backend/internal/model/models.go:121` 的 `ParentID` 没有加入 `uk_parent_name`，GORM AutoMigrate 实际只对 `name` 建唯一索引。
- `backend/internal/app/server.go:208` 初始化分类时也只按全局 `name` 查找，并可能把已有同名分类移动到另一个父级。

影响：

手写 migration 与 AutoMigrate 会生成不同约束；未来允许不同父分类下存在同名子分类时，AutoMigrate 会拒绝数据，初始化逻辑则可能串改父级。当前固定 seed 名称没有重复，因此这是扩展和迁移风险，不是当前主链路必现错误。

建议：

给 `ParentID` 补齐复合唯一索引标签，按 `parent_id + name` 查询和更新，并增加 MySQL migration 与 AutoMigrate schema 一致性测试。

## 4. 线上环境复核基线

本节记录的是 2026-07-24 对现有服务器的只读检查结果。它用于回答“问题是否适用于当前生效环境”，不替代渗透测试、云平台审计或恢复演练。

### 4.1 版本与可追溯性

- 远程源码目录 HEAD 为 `c5381d1`，工作区干净。
- 本地审查基线 `1bde7d9` 比远程目录多 8 个提交。`backend/` 和 `frontend/` 在两者之间没有源码差异；F-05 涉及的 `miniapp/src/services/request.ts` 也没有差异。
- 当前本地 miniapp 配置有意隐藏意向页，远程源码 `c5381d1` 仍注册意向页；SSH 无法证明微信/抖音平台当前发布的是哪一个小程序构建包。
- API/Web 镜像只有 compose 生成的镜像 SHA 和项目标签，没有 Git commit、构建时间版本或 release 标签。远程源码 HEAD 只能作为部署目录基线，不能作为运行镜像来源的严格证明。

### 4.2 运行拓扑与健康状态

| 组件 | 当前状态 | 端口与暴露 |
| --- | --- | --- |
| `secondhand-market-api` | 运行约 3 个月，0 次重启、无 OOM | `127.0.0.1:8082 -> 8080` |
| `secondhand-market-web` | 运行约 3 个月，0 次重启、无 OOM | `0.0.0.0:8081 -> 80`，公网可达 |
| `secondhand-market-mysql` | 运行约 4 个月，Docker health 为 healthy | 仅容器网络 `3306/33060`，未映射宿主端口 |

- `https://market.meaningful.ink/healthz`、宿主本地 API/Web health 均返回 200。
- 公网 IP 的 `http://<ip>:8081/healthz` 也返回 200，证明存在绕过 HTTPS 入口的路径。
- 主机根磁盘使用率 18%，约 63 GB 可用；内存 3.4 GiB、约 1.7 GiB 可用，无 swap；上传目录约 65.9 MB。当前资源没有迫近耗尽，但缺少配额和限额仍是风险。

### 4.3 生产配置与数据摘要

以下只记录非敏感值、布尔判断或聚合数量，不记录密码、token、手机号、openid、object key 和文件 URL：

- MySQL 8.4.8，业务库约 1.25 MB，`AUTO_MIGRATE=true`。
- 文件存储为本地 `/data/uploads`，公开前缀为 `https://market.meaningful.ink/uploads`。
- 微信登录明确为 `mock`；抖音登录变量未配置，代码默认回落到 `mock`。
- JWT access/refresh secret 均已替换且不是开发默认值；DB DSN 仍使用仓库示例数据库口令。
- 业务数量：2 个管理员、1 个商家账号、1 个商家、13 个商品（其中 9 个未删除）、2 个订单、19 个买家、0 个买家意向、51 个 auth session、25 个文件记录。
- 线上表名为 `file_records`，不存在 migration 声明的 `files`；订单、买家意向和分类唯一索引分别是 `(product_id,is_active)`、`(buyer_id,product_id,is_open)` 和仅 `name`。

### 4.4 核验边界

- 本轮没有登录任何生产账号，没有修改数据库、配置、容器或远程文件，也没有重启服务。
- 当前权限无法确认阿里云磁盘快照、RDS 级备份或 root 用户 cron；报告只能说明“未找到可见备份证据”，不能断言云端一定没有备份。
- 为保持只读，没有尝试默认管理员口令，也没有持有生产 access token，因此 F-01 的实际管理员口令和 F-14 的旧 access token 可用性未做动态登录验证。

## 5. F 类问题的线上适用性

| 编号 | 线上状态 | 线上证据与判断 |
| --- | --- | --- |
| F-01 | 部分确认 / 部分缓解 | JWT secret 已替换；DB DSN 仍用示例口令，两个管理员共享同一密码哈希，且无管理员改密入口。默认管理员口令未通过登录验证，不能宣称已确认或已轮换。 |
| F-02 | 代码适用，未发现现有越权关联 | 现有 11 个商品图片关联均存在、类型为商品图、状态为 PASS，且上传账号属于同一商家；唯一营业执照关联也存在且类型/状态正确。代码仍没有强制这些条件，不能用当前数据干净证明漏洞不存在。 |
| F-03 | schema 已确认，目标流程尚未开放 | 线上唯一索引正是 `(product_id,is_active)`；2 笔订单均为 inactive 且分属不同商品，尚未撞约束。已确认的多库存需求允许同商品多笔 active 订单，因此该索引不仅会阻止第二笔历史订单，也直接阻止目标流程。 |
| F-04 | 线上已确认 | 线上已有 1 个 APPROVED 商家和 1 个有效营业执照，但部署中的后端/前端仍只返回和显示文件 ID，审核页面无法预览证照。 |
| F-05 | 线上已确认 | 线上受保护买家接口对无效 access token 返回 HTTP 401 和业务码 `10002`；部署基线的 miniapp 请求层在读取业务码前先对非 2xx 抛错，刷新路径不可达。 |
| F-06 | 部分缓解，核心风险仍存在 | 宿主 Nginx 为 20 MB、Web 容器为 10 MB，可限制公网单请求体；应用仍按 40 MB 处理，匿名上传没有频率、总量、配额或清理机制，线上已有 13 个未绑定商品文件记录。 |
| F-07 | 目标流程未实现，且已有一条明确不一致 | 9 个未删除商品中有 2 个 `ON_SHELF` 商品库存大于 1，符合已确认的多库存需求，但现有订单无法按数量成交；另有 1 个 `SOLD` 商品库存仍为 6，状态与库存明确冲突。 |
| F-08 | 代码适用，无法由聚合数据证明已被利用 | 线上部署的 frontend 与本地相同，退出仍只清理浏览器状态；session 数量不能证明某个 refresh token 在用户退出后被再次使用。 |
| F-09 | 漂移已确认，当前运行被 AutoMigrate 暂时兜住 | 线上只有 `file_records`，且 `AUTO_MIGRATE=true`，所以当前上传链路没有因表名直接失效；关闭 AutoMigrate 或从 SQL migration 新建环境仍会出错。 |
| F-10 | 仓库问题已确认，非当前线上数据库 | `backend/app.db` 仍被 Git 跟踪并存在历史提交；生产实际使用 MySQL，因此该文件不是线上运行库，但仓库历史泄露风险不受影响。 |
| F-11 | schema 已确认，尚未触发 | 线上存在错误的三列唯一索引，但 `buyer_intents=0`，当前没有业务数据撞约束。 |
| F-12 | 线上已发生 | 生产微信登录明确为 mock，抖音漏配后也默认 mock；19 个现有买家全部为微信 mock 身份。 |
| F-13 | 线上已发生 | 生产为本地公开存储；在不输出 URL 或内容的前提下，现有营业执照匿名 GET 返回 HTTP 200、`image/png`。 |
| F-14 | 代码适用，动态利用未验证 | 线上有 2 个 revoked session，但中间件不查询 session；因未持有注销前 access token，本轮没有动态验证剩余 TTL 内的访问。 |
| F-15 | 代码适用，线上尚未使用 | 线上 `idempotency_records=0`，说明当前没有可供核对的幂等历史；并发竞态由控制流静态确认。 |
| F-16 | schema 已确认，当前数据未冲突 | 线上分类唯一索引只有 `name`；现有 20 个分类无重名，风险会在跨父级同名或切换 schema 管理方式时出现。 |

## 6. 部署环境新增发现

### D-01 [P1] 公网 8081 绕过 HTTPS 和宿主 Nginx

证据：

- Web 容器绑定 `0.0.0.0:8081` 和 `[::]:8081`。
- 从外部访问 `http://<公网 IP>:8081/healthz` 返回 200，且 `/api/v1/*` 由该容器继续代理到 API。
- 域名 HTTPS 入口本身可用，但 8081 直连不经过 TLS 和宿主 Nginx 的统一策略。

影响：

攻击者可以通过明文 HTTP 访问管理端和 API，绕过域名入口上的证书、日志、限速、安全头或后续 WAF 规则。即使用户通常访问域名，这个旁路仍扩大了攻击面。

建议：

变更前再次验证域名 HTTPS 首页、`/healthz` 和关键 `/api/v1` 请求均通过宿主 Nginx 正常工作，并保存当前端口映射作为回滚基线。然后把 Web 容器的 `8081` 只绑定到 `127.0.0.1`（供宿主 Nginx 反代）或改为等价的内部网络方案，同时在云安全组关闭公网 `8081`。变更后从域名和公网 IP 两侧复测；若域名入口异常，按已记录配置恢复端口映射，而不是临时长期开放公网旁路。

### D-02 [P1] 部署配置不可复现且敏感文件权限过宽

证据：

- 生效的 `docker-compose.yml`、Dockerfile、Nginx 配置和 `.env` 位于源码仓库外的 `deploy/` 目录，不能通过 Git 提交重建同一环境。
- compose 中 MySQL 普通用户和 root 口令都是 YAML 字面量；DB DSN 与 JWT secret 从 `.env` 注入。
- `docker-compose.yml` 和 `.env` 权限均为 `664`；父目录 `/home/yu` 为 `750`，因此当前并非所有本机用户都能穿越读取，但同组账号可读写，文件本身还保留了 other-read 位。生产 DB DSN 仍使用仓库示例数据库口令。

影响：

部署变更缺少审计和回滚基线；任何同组账号、已取得目录穿越能力的进程或后续放宽父目录权限的场景，都可能取得或改写数据库/JWT 配置。配置与源码版本分离也使事故恢复和环境复制依赖人工记忆。

建议：

当前止血范围是轮换实际在用的应用数据库口令和 root 口令、同步更新连接配置并验证应用重连，同时把 `.env` 和含秘密的 compose 权限收紧到 `600`。操作应保留可回滚的旧配置副本，但不得把秘密写入仓库或审计日志。

不含秘密的 compose、Dockerfile 和 Nginx 配置纳入版本控制属于后续可复现性治理；迁移到 secret 文件或云密钥服务可以分期完成，不应成为轮换示例口令的前置条件。

### D-03 [P2] 三层上传大小限制互相冲突

证据：

- 应用配置未显式设置 `FILE_UPLOAD_MAX_MB`，代码默认允许 40 MB。
- 宿主 Nginx `client_max_body_size` 为 20 MB。
- Web 容器 Nginx `client_max_body_size` 为 10 MB。
- README 和接口文档仍承诺原图超过 40 MB 才拒绝。

影响：

公网实际会在 10 MB 处被 Nginx 拒绝，请求甚至到不了应用的统一错误结构。客户端、文档和服务端对同一文件会给出不同预期，且 F-06 的应用层测试无法覆盖有效入口限制。

建议：

选定一个业务上限，并在客户端提示、两层 Nginx 和应用配置中使用同一值；在入口层返回可识别的 `413`，同时保留应用层二次校验。

### D-04 [P2] 未找到可确认的异机或离线备份

证据：

- 部署目录、上传目录和源码目录未找到备份产物；`yu` 用户没有 crontab，可见 systemd timer 中没有 MySQL/上传备份任务。
- MySQL binlog 已开启并保留 30 天，但与数据库位于同一服务器，不能抵御磁盘损坏、整机丢失或恶意删除。
- 当前权限无法查看 root cron 或阿里云快照策略。

影响：

数据库虽只有约 1.25 MB，上传目录也只有约 65.9 MB，但已经包含商家资质、商品图片和账号数据。若不存在云端快照，单机故障会造成不可恢复的数据丢失。

建议：

先由具备权限的人员核实阿里云磁盘快照、数据库备份策略和 root 用户 cron，并记录策略 ID、保留期及最近一次成功时间。只有确认这些位置也没有有效备份时，再补一个与当前数据规模相称的最小方案：定时导出 MySQL、同步 `/data/uploads` 到异机或对象存储，并至少做一次抽样恢复验证。后续再根据业务可接受的数据损失确定 RPO/RTO 和演练频率。

### D-05 [P2] 缺少容器级健康、资源和日志保护

证据：

- API 和 Web 容器没有 Docker healthcheck；只有 MySQL 有 healthcheck。
- 三个容器都没有内存或 PID 限制。
- 日志驱动为默认 `json-file`，没有 `max-size` / `max-file` 轮转参数。
- 当前三容器均为 0 次重启、无 OOM，说明现在稳定，但不能替代保护措施。

影响：

应用内部失活时 Docker 仍可能显示 `Up`；异常流量或图片处理可能挤占整机内存；长期访问日志可能无界占用磁盘。

建议：

为 API/Web 增加 healthcheck 和明确的重启策略，设置基于实际压测的内存/PID 限制，并为 `json-file` 配置日志轮转和外部采集。

### D-06 [P3] 运行镜像无法严格对应源码提交

证据：

- 运行镜像名为本地 compose 名称 `deploy-api` / `deploy-web`，没有不可变 release tag。
- 镜像和容器标签没有 Git commit；compose 配置 hash 只能证明 compose 配置，不能证明构建上下文源码。

影响：

故障或安全事件发生时，无法从容器元数据回答“线上到底运行哪一份源码”，也无法可靠地用同一镜像回滚或复现。

建议：

CI 构建不可变镜像，标签和 OCI label 同时写入 commit SHA、构建时间和版本；部署只引用镜像 digest，并在发布记录中保存 digest 与 commit 的对应关系。

### D-07 [P3] HTTPS 响应缺少基础安全响应头

证据：

- `https://market.meaningful.ink/` 返回管理端 HTML 200，但响应未包含 HSTS 和 CSP；`/healthz` 也没有这些响应头。
- 公网 8081 还允许明文 HTTP 直连，进一步削弱 HTTPS 强制策略。

影响：

浏览器没有长期 HTTPS 强制记忆，管理端也缺少 CSP 对脚本来源的额外约束。安全头不能替代修复 XSS 或关闭旁路，但应作为管理后台的纵深防护。

建议：

关闭 D-01 的公网旁路后，在宿主 Nginx 统一配置并验证 HSTS、CSP、`X-Content-Type-Options`、`Referrer-Policy` 等响应头；CSP 先以 report-only 观察再收紧。

## 7. 文档真实性矩阵

| 文档说法 | 源码事实 | 线上事实 | 判断 |
| --- | --- | --- | --- |
| `README.md:15`、`project-overview.md:4,36`：不含买家端/小程序 | 已有完整 `miniapp/`、buyer API、买家表和商家意向处理 | 线上有 19 个买家、51 个 session | **错误/陈旧**；同一 README 后文还列出买家登录变量，内部自相矛盾 |
| `project-overview.md:11,102`：Redis 辅助会话、黑名单和限流 | 会话存 MySQL，限流为单进程内存结构，仓库没有 Redis 接入 | 生产容器没有 Redis 服务 | **错误/未实现** |
| `project-overview.md:12,132`：MinIO/OSS/S3 对象存储抽象 | 当前 provider 只实现本地落盘和公开静态 URL；README 已较准确写成“当前支持 local” | 生产使用本地 `/data/uploads` | **overview 陈旧**，README 此处较准确 |
| `project-overview.md:52`、`dir-structure.md:86`、`release-readiness.md:6`：管理员由 bootstrap 脚本预置 | 脚本确实存在，但脚本和服务启动时的自动 seed 都写入相同固定密码；文档没有说明启动路径也会自动创建 | 线上有两个共享密码哈希的管理员，无法只读判断由哪条路径创建 | **部分正确但关键行为缺失**，且两条路径都有同一安全问题 |
| `specs.md:86,92-95`、`data-model.md:137,160`、`backend-api-checklist.md:145,151`：库存固定为 1 | DTO、handler 和 frontend 允许任意正库存，完成订单不扣库存而直接 SOLD | 3 个商品库存不为 1，其中一个 `SOLD` 后库存仍为 6 | **错误，且线上已出现不一致** |
| `specs.md:69`、`frontend-pages.md:87,96`：审核详情展示证照图片并支持预览 | API/UI 只返回和显示 `license_file_id` | 已有 APPROVED 商家和执照，但审核页不能预览 | **错误，核心验收项未实现** |
| `data-model.md:118,224`、SQL migration：分类按 `(parent_id,name)` 唯一、文件表为 `files` | GORM 生成全局 `name` 唯一和 `file_records` | 线上索引仅 `name`，表为 `file_records` | **错误，migration 与生效 schema 漂移** |
| `backend-api-checklist.md:46-47`：同一幂等键只执行一次 | 先执行业务再写记录且忽略写入错误 | 线上幂等记录为 0，未发生可核对样本 | **实现不满足文档保证**；线上是否已重复执行无法验证 |
| `project-overview.md:113`：JWT 支持主动失效 | logout 只撤销 refresh session，access 中间件不查 session | 线上已有 revoked session，但无旧 access token 可做只读验证 | **部分正确**；refresh 可失效，access 非即时失效 |
| `README.md:34,135`、`backend-api-checklist.md:190`：原图上限 40 MB | 应用默认 40 MB | 公网有效链路先经过 20 MB 和 10 MB Nginx 限制 | **文档对应用正确，对生效环境错误** |
| `release-readiness.md:13-18,45-46`：2026-03-10 回归通过且没有发布阻断 | 这些是历史执行记录，当前源码有多个未覆盖阻断项 | 服务已部署，但 mock 登录、公开执照等问题仍生效 | **历史快照，不可作为当前发布证明**；“无阻断”对当前版本错误 |
| `miniapp-release-readiness.md:39-53`：未启用真实微信登录前不可发布 | 代码支持 real，但默认 mock 且无生产启动保护 | 生产仍为 mock，19 个买家全部 mock | **文档判断正确，但发布门槛未被执行** |
| `README.md:66-88`：生产统一 `/api/v1` 并通过 HTTPS 域名访问 | 路由和代理前缀一致 | 域名 HTTPS/API 正常 | **主链路正确**；但公网 8081 旁路使“统一入口”实际上不成立 |
| `dir-structure.md:9-58`：仓库只有 frontend/backend/docs，内部采用 handler/service/repo/filesvc 分层 | 仓库已有 miniapp；后端主要集中在 `internal/app`，没有文档树中的分层目录 | 不影响运行，但会误导维护者和部署脚本 | **推荐稿未更新为现状** |

结论：文档并非整体不可用。接口前缀、restricted login、主要状态机和 miniapp “真实登录未验收则不可发布”的判断基本正确；问题在于早期目标文档、架构建议和历史发布快照没有标注状态，混在一起后容易被误当成当前事实。建议为文档增加 `status: current/design/historical` 和适用 commit，并把 README、数据模型与生产运行手册作为唯一现状入口。

## 8. 验证结果

### 后端

执行命令：

```bash
GOMODCACHE=/Users/huangyu/go/pkg/mod \
GOCACHE=/tmp/second-hand-market-go-build \
GOPROXY=off \
go test -v -timeout=120s ./...
```

结果：所有后端包和现有测试通过。

### miniapp

执行命令：

```bash
npm test
npm run build:weapp
npm run build:tt
```

结果：11 个测试文件、17 个测试用例通过；微信和抖音构建成功。

### frontend

尝试执行：

```bash
npm test
npm run build
```

结果：未取得可信的完成结果。全量 Vitest 停在 `RUN`，单文件单 worker 仍停在同一阶段；`tsc -b` 也长期无输出，相关进程几乎为零 CPU，均由审查者中止。前序诊断曾观察到依赖文件映射到 macOS FileProvider 的 `.../wharf/delete/...` 路径，本轮现象与依赖读取挂起一致，但现有证据不足以把 FileProvider 写成唯一根因。

该项属于环境阻塞，不代表 frontend 构建或测试通过，也不能排除额外的 TypeScript、测试或打包问题。

### 针对性数据库复现

除全量测试外，使用最小 SQLite 表分别复现订单和买家意向的唯一索引行为：

- 第一笔 inactive 订单存在时，第二笔 active 订单更新为 inactive，触发 `orders.product_id, orders.is_active` 唯一约束。
- 第一笔 closed 意向存在时，第二笔 open 意向更新为 closed，触发 `buyer_intents.buyer_id, buyer_intents.product_id, buyer_intents.is_open` 唯一约束。

两项均为确定性 schema 错误，不依赖并发或特定前端行为。

## 9. 两份审查的交叉复核结论

| Grok 条目 | 复核处置 | 本报告位置 |
| --- | --- | --- |
| R-01 ~ R-10 | 均由源码或实际复现确认；与原报告 F-01 ~ F-10 一致 | F-01 ~ F-10 |
| R-03 中的 BuyerIntent 同类风险 | 已实际复现，不再只作为附带风险 | F-11 |
| R-11 默认 mock 登录 | 确认；线上明确为 mock 且 19 个买家全部是 mock 身份，生产处置等级提升到 P1 | F-12、第 5 节 |
| R-12 资质文件公开 | 确认；现有营业执照匿名请求返回 200，不再只是静态推断，生产处置等级提升到 P1 | F-13、第 5 节 |
| R-13 文档漂移 | 确认；逐项对照源码与线上环境，不把设计稿或历史记录冒充当前事实 | 第 7 节 |
| R-14 质量门禁偏弱 | 确认；仓库无 CI/lint，frontend 测试深度有限 | 第 10、11 节 |
| R-15 意向页未注册 | 对本地 HEAD 不列为缺陷：测试明确要求隐藏；远程源码 `c5381d1` 仍注册意向页，但 SSH 无法确认平台实际发布的小程序包 | 第 4、10 节 |
| R-16 access JWT 不查 session | 确认；默认 2 小时吊销窗口对管理端不应只按 P3 观察项处理 | F-14 |
| R-17 分类、幂等、限流等混合项 | 分类和幂等分别确认并拆项；生产当前只有一个 API 容器，多实例限流绕过不是当前事实；其余按证据强度保留 | F-15、F-16、第 10 节 |

Grok 二次反馈没有推翻上述源码、schema 或线上核验事实；本轮据此修正的是当前小站的处置优先级和迁移范围，避免把所有成立的问题都表述为同一批 P1 发布阻断。

## 10. 工程质量与文档一致性

### 已确认的优点

- 商家 `onboarding/full` scope 和角色路由边界清楚。
- 商品、订单状态转换集中在 `stateflow`，比在 handler 中散落条件更易审计。
- 后端已有认证、权限、买家流程、上传和事务相关集成测试。
- 买家游客数据合并、收藏/历史 owner key、请求刷新单飞等设计有明确结构。
- 上传路径对目录穿越做了根目录约束，图片 MIME 和处理结果也有二次校验。

### 已确认的工程风险

- `README.md` 和 `docs/project-overview.md` 仍声明不含买家端，但仓库、线上 API 和生产数据均已有买家域。
- 设计文档写 Redis session/限流和对象存储；源码与生产实际是数据库 session、进程内限流和本地静态目录。管理员 bootstrap 脚本确实存在，但服务启动路径还会自动执行同样的固定密码 seed，文档没有披露这一行为。
- 仓库没有可见 CI workflow，也没有项目级 lint 命令。frontend 只有登录、路由守卫和错误码 3 个测试文件。
- `memoryRateLimiter` 仅在单进程内生效；生产当前只有一个 API 容器，所以多实例不共享不是当前漏洞，但进程重启仍会清空计数，扩容前必须替换或明确限制。
- 本地 HEAD 的 miniapp 意向页源代码存在但页面配置和测试都要求隐藏，属于产品策略切换后的残留代码；远程源码仍注册页面，实际发布包需要从小程序平台另行确认。
- `IdempotencyRecord`、auth session 和操作日志都需要明确数据保留与清理策略。

## 11. 测试覆盖缺口

现有测试通过不代表上述业务边界正确。至少缺少以下回归测试：

- 空配置生产启动必须失败
- 默认管理员初始化和首次改密策略
- 商品图片及营业执照的跨商家越权绑定
- 错误文件类型、扫描未通过文件和不存在文件的绑定
- 同一商品创建多笔 active 订单，并分别关闭或完成
- miniapp 请求层收到 HTTP 401 后刷新并重放请求
- 多个并发 401 只触发一次 refresh
- 匿名上传请求体上限、频率和未绑定文件清理
- 多数量订单的预占、释放、完成扣减和库存归零售罄
- 并发创建订单时预占总量不得超过可售库存
- frontend 退出登录调用服务端 logout
- 仅通过 SQL migrations 初始化空库后的文件上传流程
- 同一买家对同一商品连续创建并关闭两笔意向
- 生产配置拒绝微信或抖音 mock 登录模式
- 营业执照只能通过授权或短期签名地址访问
- logout 后旧 access token 的预期行为
- 相同 Idempotency-Key 的并发请求只执行一次业务副作用
- 幂等记录的过期与清理
- migration 与 AutoMigrate 生成相同的分类复合唯一索引
- CI 自动运行 backend、frontend 和 miniapp 的测试及构建矩阵

## 12. 修复顺序建议

### 24～48 小时止血

1. 验证 HTTPS 域名和 API 主入口后收口公网 `8081`，并完成内外两侧复测及回滚记录。
2. 轮换当前应用/root 数据库口令，核实并轮换两个管理员口令，把敏感部署文件权限收紧到 `600`；不等待完整密钥设施建设。
3. 限制继续创建新的 mock 买家身份，启动 19 个现有买家的数据盘点；不能直接裸切 `real`。
4. 先建立管理员受限预览，再隔离现有 1 份营业执照并封禁旧 URL；商品图片仍保持公开。
5. 多库存规则已经确认；在数量、预占和扣减逻辑发布前，冻结 3 个相关商品的后台建单入口，核对实际总库存，尤其处理 `SOLD` 但库存仍为 6 的记录，不把其余库存机械归一为 1。
6. 核实阿里云快照、数据库备份和 root cron。若确认均不存在，在当前运维窗口补最小 MySQL 与上传目录异机备份。

### 对应业务流程再次发生前

1. 完成 mock 账号重置或绑定/合并方案、小范围试迁移与回滚验证，再把生产登录切到 `real` 并增加生产配置保护。
2. 在商家后台开放多库存订单前，删除订单单 active 唯一约束并完成数量、预占库存和并发防超卖迁移；在恢复买家意向入口前迁移意向 open 唯一索引。
3. 在下一次商家入驻和资质审核前补文件归属/类型/扫描状态校验，以及营业执照鉴权预览。
4. 在下一次 miniapp 或网关版本中修复 401 刷新、匿名上传限制和三层上传大小不一致。

### 常规版本与运维治理

1. 完成 frontend logout、access token 吊销策略、幂等原子性、migration/AutoMigrate/分类 schema 一致性，以及 Git 中业务数据库的合规处置。
2. 将非敏感部署配置纳入版本控制，为镜像写入 commit/digest，并逐步增加 healthcheck、资源限制、日志轮转、安全响应头和可验证的备份演练。
3. 补齐 CI/lint，在干净依赖环境中执行 backend、frontend、miniapp 的测试、构建和入口级 smoke。

## 13. 发布判断

当前部署已经稳定运行约 3 至 4 个月，现有只读证据没有确认需要全站立即停机的 P0，也不建议把全部发现合并成一次无序重构。健康检查为 200 只能证明服务可响应，F-12 的 mock 身份、F-13 的公开营业执照、D-01 的明文旁路、示例数据库口令和 F-07 的 3 条库存异常仍需按上节止血。

收口 D-01、轮换示例数据库及管理员口令、限制并制定 F-12 身份迁移、隔离 F-13 现有执照，并修正或冻结 F-07 的 3 个异常商品后，当前小站可以在低流量和明确补偿控制下继续运行。这里的“继续运行”不等同于宣称已经通过完整生产验收。

F-03 和 F-07 按已确认的商家多库存模型合并治理，必须在后台开放多数量、多订单前完成；F-02 和 F-04 必须在下一次商家入驻/资质审核前补齐；F-11 必须在恢复意向入口前迁移。其余 P2/P3 条目按相关版本和运维窗口处理，不是当前停站条件。D-04 应先取得云快照/root 任务证据，不能在权限不足时把“未找到”写成“确定没有备份”。
