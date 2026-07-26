# 生产加固与商家多库存修复方案

- 日期：2026-07-24
- 基线：`master` / `1bde7d9`
- 目标分支：`codex/reconcile-code-reviews`
- 依据：`docs/full-project-code-review-2026-07-24.md`（问题台账）、`docs/deep-code-review-2026-07-24.md`（底稿）与生产环境只读核验
- 角色：本文是当前小修施工单，并单列后续业务门槛；后续门槛不得反向阻塞当前范围
- Status: local implementation and production-clone isolated acceptance completed; production migration/deployment/account rotation still pending.
- F-12, F-13, license governance, miniapp ordering, MySQL root rotation, and the Vitest investigation remain outside this release.

## 0. 生产执行记录

### 0.0 Checkpoint A 进度总表

| 分项 | 内容 | 状态 | 与当前版本的关系 |
| --- | --- | --- | --- |
| A1 网络 | Web `8081` 仅 loopback；云安全组关闭公网 8081 | **部分完成** | 主机侧已完成；只差阿里云侧确认，不阻塞本地代码开发 |
| A2a 应用库口令 | MySQL 业务账号口令轮换 + `.env`/`compose` 权限 `600` | **已完成** | 见 §0.2 |
| A2b MySQL root 口令 | 保留 root 账号并轮换口令 | **待独立维护窗** | 不与管理员改密或库存发布混做 |
| A2c 管理员口令 | 保留两个管理员并逐个改密；移除固定密码 seed；不得动 `yaner` | **本地实现完成，待生产轮换** | 改密、session 吊销、安全 bootstrap 与生产默认值保护已完成；尚未改现网口令 |
| A3 备份证据 | 核实云快照 / DB 备份 / root cron；确无则补最小异机备份 | **待办** | 是生产数据库迁移前置条件 |
| C 商家多库存 | 数量、单价、自动总价、预占和多 active 订单 | **本地实现和生产数据克隆隔离验收完成，待迁移部署** | MySQL 8.4.8 迁移、索引、CHECK、AutoMigrate、并发及浏览器验收已通过；生产迁移仍待执行 |
| G1 资质文件 | 执照私有化与管理员预览 | **延后** | 真实商家入驻/资质审核前完成，不阻塞 C |
| G2 miniapp 身份 | 先迁移两名体验用户，再关闭 mock | **后续独立迁移** | 不阻塞 C；不得先切断现有体验用户登录 |

各分项独立验收。不得因为 G1/G2 尚未实施而阻塞商家后台多库存，也不得把 A2a 已完成扩写为 D-02/F-01 已全部关闭。

### 0.1 2026-07-24：公网 `8081` 收口（A1 主机侧）

- Web 容器端口映射已从公网监听改为只监听 `127.0.0.1:8081`，宿主 Nginx 继续通过 loopback 反向代理。
- 只重建了 Web 容器；API 和 MySQL 未重启。
- 两轮复测中，HTTPS 域名首页、`/healthz` 和买家商品接口均返回 200，公网直连 `8081` 失败，Web 容器 `RestartCount=0`。
- 原 compose 备份位于 `/home/yu/services/secondhand-market/deploy/docker-compose.yml.bak-codex-20260724-8081-loopback`，权限已收紧为 `600`。
- 云安全组是否已删除公网 `8081` 规则仍需以阿里云侧配置为准；应用主机侧已经不可通过该端口直连。

### 0.2 2026-07-24：应用数据库凭据轮换（A2a）

- 保留现有 MySQL 应用账号名和原有库级授权，不删除、不锁定账号，也不修改管理员、商家或买家账号。
- 使用 MySQL 8.4 双密码能力完成平滑切换：先验证新旧密码均可连接，再只重建 API 容器，确认 API 已读取新配置后废止旧应用密码。
- 新凭据完成只读查询和事务内零行更新校验；旧应用密码已被拒绝，账号不再保留第二密码。
- MySQL、Web 和宿主 Nginx 未重启；API 重建后 `RestartCount=0`，启动日志未发现 error、fatal 或 panic。
- `.env`、含秘密的 compose 及旧配置备份权限已收紧为 `600`；备份目录权限为 `700`。
- 变更前逻辑备份位于 `/home/yu/services/secondhand-market/deploy/backups/credential-rotation-20260724/mysql-before.sql.gz`，包含 18 张表并通过 gzip 完整性校验。
- MySQL root 口令和两个后台管理员口令未在本次应用账号切换中修改，仍是 A2b / A2c 待办；不能据此把 D-02/F-01 整体标记为完成。

### 0.3 现网账号保护约束

- 经生产数据库只读核对，`yaner` 是 `merchant_accounts` 中正在使用的 `ACTIVE / OWNER` 商家账号，不是后台管理员账号。
- `yaner` 必须原账号保留。未经账号实际使用者单独确认，不得删除、软删除、禁用、重命名、重新初始化、重置密码、改变角色或商家归属，也不得批量注销其 session。
- 后续两个后台管理员口令的轮换只能明确作用于 `admin_users` 表中的目标管理员，不得按相似用户名或全账号范围更新 `merchant_accounts`。
- 公网 `8081` 收口和应用数据库凭据轮换均未修改 `yaner` 的密码哈希、状态、角色、商家归属、session 或业务数据。
- 商家多库存迁移发布前后都要确认 `yaner` 仍能登录，并能访问其原有商品和订单；验证不得创建不可回滚的真实订单。

### 0.4 2026-07-24：本地修复实施记录

- 已移除服务启动时的固定管理员口令 seed；安全 bootstrap 不内置或回显密码，不覆盖现有账号，也不自行执行迁移。
- 已增加管理员自助改密和仅针对目标管理员的 session 吊销；历史管理员 access/refresh token 会立即失效，商家与买家 session 不受影响。
- 已增加 `APP_ENV=production` 默认值保护，生产示例中的 JWT/数据库占位值在替换前会被拒绝。
- 已实现商家后台多库存订单、数量/单件成交价/自动总价、原子预占、完成双减、关闭释放和多 active 订单；新主路径不再写入 `LOCKED` 或 `active_order_id`。
- 买家列表、详情、收藏和历史继续读取字段 `stock`，其值由 API 映射为可售库存；小程序页面未增加下单功能。
- 本地后端、前端构建和 miniapp 测试均已通过；`frontend npm test` 在 Ant Design 模块初始化阶段挂起，未完成，不是绿色证据。三条 smoke 脚本已通过语法检查，但未对现网执行会产生业务数据的 smoke。
- 本轮没有执行生产 SQL、部署、管理员改密或任何 `yaner` 数据变更。MySQL 8.4.8 的迁移、索引、CHECK、AutoMigrate 兼容性、并发、管理员安全及桌面/移动浏览器验收已在生产数据克隆隔离环境通过。

### 0.5 2026-07-26：F-06 匿名上传资源治理代码侧记录

```text
代码侧状态：已修复
测试服务器状态：未审核
生产状态：未执行 0008、未部署、未修改生产数据或文件
```

- 书面设计和规格已批准，实施提交范围为 `39aed02..c598f38`。业务文件上限统一为 10 MiB，multipart transport 上限为 11 MiB；代理侧 413 与应用侧均使用 code `10008`。
- 新增数据库串行的匿名频率/活跃配额、商家配额和全局配额，错误 code 为 `10009` 或 `10013`；来源 IP 仅保存 HMAC-SHA256，不记录原始值。
- 新增 `0008` 三段迁移与 InnoDB fail-closed 门禁；历史行治理字段保持 NULL，`0007` 执照隐私和商品图 URL 不变量是 `0008` 的强前置条件。
- 新增有界匿名孤儿 claim/清理；`cleanup_after` 取 capability 过期宽限与 `created_at+1 hour` 的较晚值，保证滚动限频证据不会提前删除。
- 本地 `make test`、`go vet ./...`、frontend 12 files / 25 tests、frontend build 和 smoke 脚本语法均通过。现有构建 warning 未扩大为 F-06 行为失败。
- F-06 的测试服务器审核必须另行授权固定路径 `/home/yu/services/secondhand-upload-governance-acceptance-20260726` 和 Compose project `secondhand-upload-governance-acceptance`。此前 F-02/F-04/F-05 的授权不复用。

## 1. 目标与原则

本方案不做停站重构。目标是在保持现有低流量服务可用的前提下，先关闭已经生效的安全暴露，再完善商家管理后台的多库存订单能力，最后按业务入口处理其余问题。

实施原则：

1. 每个发布检查点必须可以单独验证，避免把安全、库存、身份迁移和运维治理绑成一次发布。
2. 所有数据库变更先备份、预检、迁移、验证，再开放对应入口。
3. 生产操作必须有回滚步骤，不在代码提交或日志中记录任何真实秘密。
4. 小程序本轮不增加下单能力；真实微信登录迁移单独设计。
5. 线上现有数据只在人工核对后修正，不根据代码默认值自动覆盖。
6. `yaner` 作为现网在用商家账号列入发布保护清单，任何账号或数据迁移都必须显式排除未经确认的账号变更。
7. 现网生产默认 `AUTO_MIGRATE=true`：结构变更必须以「GORM 模型标签与手工/SQL 迁移一致」为准，禁止只改一边导致唯一索引被重建。
8. 营业执照与 miniapp 身份是独立业务门槛，不与商家后台库存版本捆绑发布。

## 2. 已确认的产品规则

### 2.1 订单入口

- 本轮只有商家管理后台可以创建、完成和关闭订单。
- 小程序继续提供商品浏览、收藏、历史和意向，不增加购物车、下单、支付或买家订单页面。
- 后端库存逻辑保持可复用，但在客户确认小程序交易需求前不暴露买家下单接口。

### 2.2 价格

- `deal_price_cent` 表示单件成交价。
- 订单增加 `quantity`，必须为正整数。
- 整单总价为 `deal_price_cent * quantity`，由服务端使用 64 位整数计算并返回，不单独持久化可推导总价。
- 历史订单迁移后 `quantity=1`，原成交价语义保持不变。

### 2.3 库存与不变量

- `products.stock`：尚未售出的实物总库存。
- `products.reserved_stock`：未完成订单已预占的库存。
- `available_stock = stock - reserved_stock`：当前可继续创建订单的库存。
- 创建订单增加 `reserved_stock`；关闭订单释放预占；完成订单同时减少 `stock` 和 `reserved_stock`。
- `stock=0` 时商品才进入 `SOLD`。
- 商家主动下架只阻止新订单，不影响已有订单继续完成或关闭。
- 商品存在预占库存时不得永久关闭；编辑库存时新值不得小于 `reserved_stock`。
- 一笔订单整体完成或整体关闭。本轮不做部分发货、拆单、退款和完成后退库。

运行时与迁移后均须满足：

```text
stock >= 0
reserved_stock >= 0
reserved_stock <= stock
available_stock = stock - reserved_stock
```

完成库存变更必须使用 `reserved_stock >= qty AND stock >= qty` 的单条条件更新；关闭订单至少使用 `reserved_stock >= qty` 的单条条件更新。两者都禁止只减一边或先读后写无条件更新。

### 2.4 商品状态机：废弃 `LOCKED` 主路径（旧 → 新）

现网订单创建会把商品打成 `LOCKED`，完成/关闭依赖「商品必须为 LOCKED」。多库存下同一商品可有多笔 active 订单，**`LOCKED` 不再作为主路径**。

| 场景 | 旧行为 | 新行为（本阶段强制） |
| --- | --- | --- |
| 创建订单成功 | 商品 → `LOCKED`，写入 `active_order_id` | 只允许 `ON_SHELF` 商品建单；成功后仍为 `ON_SHELF`，不写 `active_order_id`，只增加 `reserved_stock` |
| 存在 active 订单 | 通常整商品锁定，难并存多单 | 允许多笔 `is_active=true` 订单；可售为 0 时禁止新单，状态仍可为 `ON_SHELF` |
| 关闭订单 | 订单 inactive，商品 → `OFF_SHELF` | 订单 inactive，**释放预占**；商品**保持**当前上架/下架，**不**自动下架、不自动上架 |
| 完成订单 | 订单 inactive，商品 → `SOLD` | 双减库存；**仅当 `stock=0`** 时 → `SOLD`；仍有库存则保持当前上架/下架 |
| `available_stock=0` 且 `stock>0` | 少见（整单锁定） | 全部预占：禁止新单；买家侧按 §2.5 展示 |
| `active_order_id` | 读写 | **停止读写**；迁移时置空；列首发保留便于回滚 |
| `stateflow` | `ON_SHELF→LOCKED`，`LOCKED→SOLD/OFF_SHELF` | 主路径改为围绕 `ON_SHELF` / `OFF_SHELF` / `SOLD` / `CLOSED`；订单不再驱动进入 `LOCKED` |
| 历史 `LOCKED` 行 | 可能存在 | 预检统计；若无 active 订单，维护窗将遗留 `LOCKED` **人工确认后**迁回 `ON_SHELF` 或 `OFF_SHELF`（默认：无预占 → `ON_SHELF` 需商家确认，或稳妥起见先 `OFF_SHELF` 再由商家上架）。**有 active 订单的 LOCKED 必须先处理完订单再迁状态** |

文档 `specs` / `data-model` 中「关单一律 OFF_SHELF」「有单即 LOCKED」的表述在本阶段发布后视为**过时**，应在同一发布说明中标注，避免运营按旧规则操作。

### 2.5 买家侧库存展示（本阶段已定）

- 本阶段**小程序不改页面代码**（无下单）。
- 现有小程序商品卡片、详情、收藏和历史页面都直接展示响应中的 `stock`；因此只新增一个小程序不读取的 `available_stock` 字段不能解决展示偏差。
- 为保持小程序页面代码和现有响应契约不变，买家只读 API 继续返回字段 `stock`，但其值映射为 **`available_stock`（可售库存）**；本阶段不要求买家页面读取或依赖新增字段，也不得向买家接口暴露预占明细。
- 商家 API 中 `stock` 继续表示未售实物总库存，并另外返回 `reserved_stock`、`available_stock`。买家和商家必须使用各自的响应 DTO，不能直接序列化同一个商品模型；字段语义须在 API 文档与测试中明确。
- `available_stock=0` 时买家端显示 0 件或无货；本轮不增加买家下单能力，也不因该字段变化修改 miniapp 页面。
- 商家后台必须展示 `stock` / `reserved_stock` / `available_stock` 三件套。

## 3. 范围

### 3.1 本轮实现

1. 完成管理员自助改密、active-session 校验和生产默认值保护，移除固定密码 seed；保留并逐个轮换两个后台管理员，严格排除 `yaner`。
2. MySQL root 口令在单独维护窗轮换，不与管理员或库存发布混做。
3. 商家后台订单增加数量、单价和自动总价展示。
4. 商品增加预占库存，允许同一商品存在多笔 active 订单；**废弃 LOCKED 主路径**（§2.4）。
5. 删除错误的订单唯一索引，保留普通查询索引；GORM 模型同步去掉 unique 标签。
6. 提供生产迁移（含 AutoMigrate 双轨处置）、数据核对、发布和回滚步骤。
7. 完成阿里云安全组 `8081` 关闭确认，并在库存迁移前取得可恢复备份证据。

### 3.2 本轮不实现

- 小程序直接下单、购物车、支付、物流和买家订单中心。
- `mock -> real` 的直接切换或 19 个现有买家账号的自动合并。
- 通过 Nginx、`disabled` 模式或单独增加“禁止创建 mock 账号”的开关来直接封禁 miniapp 登录；当前 mock OpenID 随临时登录 code 变化，这些做法都不能保证现有两个实际体验用户重新登录。
- 营业执照完整私有化、管理员执照预览和商品/执照上传归属绑定；这些改为真实商家入驻/资质审核前的独立门槛。
- 全局轮换商品图片 URL 或迁移全部上传文件。
- 部分完成、部分取消、退款和售后库存回补。
- 一次性引入完整密钥平台、对象存储或大型库存子系统。

## 4. 发布检查点

### 4.1 Checkpoint A：线上运维止血

这些操作多数不依赖应用业务代码，但必须逐项执行；进度以 §0.0 为准。

#### 4.1.1 A1 网络

1. 再次验证 HTTPS 域名首页、`/healthz` 和关键 `/api/v1` 均走宿主 Nginx。
2. Web 容器 `8081` 仅绑定 `127.0.0.1`（主机侧 **已完成**）。
3. 在阿里云安全组删除公网 `8081` 入站；变更后从公网 IP 复测应失败，域名侧应仍为 200。
4. 保留 compose 备份路径与回滚步骤（已有备份文件）。

#### 4.1.2 A2 凭据

1. **A2a 应用库**：**已完成**（§0.2）。
2. **A2c 管理员（下一小修）**：
   - 保留现有 `admin`、`superadmin` 的账号名、角色和业务记录，不删除、不禁用、不合并；本项只改变其密码哈希和对应管理员 session。`yaner` 属于 `merchant_accounts`，不在本项更新范围。
   - 新增管理员自助改密接口和后台安全设置入口；必须校验当前密码，新密码长度至少 12 位，且不得继续使用公开初始化口令。
   - 只更新当前 `admin_users.id` 对应行；路由和 SQL 都不得写入 `merchant_accounts`，不得匹配或修改 `yaner`。
   - 在同一事务中更新密码哈希，并以 `user_type='ADMIN' AND user_id=?` 将该管理员所有未注销 `auth_sessions` 标记为 revoked；前端收到成功响应后立即清除本地 access/refresh token 并跳回登录页。
   - 当前 access token 是无状态 JWT，原中间件不查询 session。为确保改密后立即失效，在 `OptionalAuth` 解析 token 后增加仅针对 `ADMIN` actor 的 active-session 校验：session 不存在、已 revoked、已过期，或 session 的 `user_type/user_id` 与 token 不一致时返回 401；商家和买家 actor 直接通过，不影响 `yaner`。
   - 删除服务启动时“空库自动创建两个固定密码管理员”的分支；保留分类等非敏感 seed。`APP_ENV=production` 且空库没有管理员时应带明确错误停止启动，提示先执行一次性 bootstrap，不能静默写入仓库公开口令。
   - 修改 `backend/scripts/bootstrap_admin`：不得内置或回显密码，必须由交互式隐藏输入或受控 secret 文件显式提供；不覆盖已有管理员，也不在生产中自行执行 AutoMigrate。测试使用测试夹具显式创建管理员，不依赖生产 seed。
   - 增加 `APP_ENV=production` 配置保护：生产启动拒绝仓库已知的默认 JWT secret、示例 DB DSN/口令和任何固定管理员初始化口令；mock 登录暂按 §9.1 独立迁移，不在本项直接拒绝。
   - `admin` 与 `superadmin` 使用不同的新密码，逐个轮换；第一个新密码登录验证成功后再处理第二个，始终保留一个可登录管理员。
   - 记录旧密码登录失败、新密码登录成功、历史 refresh 失败，但不在 Git、终端回显或审计文档中记录任何口令。
3. **A2b root（独立维护窗）**：
   - 保留现有 `root@localhost` 和 `root@%` 账号，不删除账号；先完成可恢复备份并记录回滚步骤。
   - 使用 MySQL 8.4 双密码能力为两个 root host 逐一加入新密码，分别验证新旧密码在过渡期可用。
   - 同步更新含秘密的 compose 字段；将 healthcheck 改为不携带 root 口令的纯存活探测。`mysqladmin ping` 使用错误密码也可能返回成功，不能作为口令轮换验收证据。
   - 在短维护窗重建 MySQL 容器以应用 compose/healthcheck 变化，确认数据卷、表数量、API 数据库连接和域名入口正常后，再废止两个 root 账号的旧密码。
   - 必须用真实 SQL 连接分别证明新密码可用、旧密码被拒绝；容器 `healthy` 只能证明进程存活。
4. 敏感文件保持 `600`，备份目录保持 `700`；root/admin 密码不得写入本仓库。

#### 4.1.3 A3 备份

1. 有权限人员核实：阿里云磁盘快照、是否有 DB 自动备份、root cron/timer。
2. 记录：策略 ID 或「无」、保留期、最近成功时间（可写在运维笔记，不进 Git 秘密）。
3. 仅当确认均无效时，补最小方案：定期 `mysqldump`（或等价）+ `/data/uploads` 异机/对象存储，并做一次抽样恢复。

#### 4.1.4 A 完成标准（分项）

- A1：域名正常 + 公网 8081 不可达（含安全组确认）
- A2a：应用库已轮换，旧应用密码失效
- A2c：两个管理员已逐个改密并完成登录/session 验证；固定密码 seed 已移除，生产默认值保护生效，`yaner` 未被修改
- A2b：两个 root host 的新密码可用、旧密码失效，MySQL/API 复测正常
- A3：备份证据已记录，或最小异机备份已就绪

### 4.2 Gate G1：真实商家入驻前的资质文件私有化

本 Gate **不属于当前小修，也不阻塞 Checkpoint C**。现阶段只有两个实际体验用户，现有一份营业执照是人工测试数据；F-04、F-13 以及 F-02 中涉及营业执照的归属/类型校验，在接入真实商家、真实执照或恢复正式资质审核前完成。

当前阶段不修改或删除这份测试记录和文件，也不把它的处置列为库存版本前置条件。若在 G1 前另行决定临时降低暴露，只能在核对实际 object key 后，对精确的 `merchant_license/` 前缀增加匿名拒绝规则并回归商品图片；删除记录或文件必须另经数据所有者确认。

后端：

- 将 `/uploads` 改为受控静态处理，只允许商品图片等公开业务类型；`merchant_license/` 路径始终拒绝匿名访问。
- 新增管理员鉴权文件内容接口 `GET /api/v1/admin/files/:id/content`。
- 接口只允许读取存在、`biz_type=MERCHANT_LICENSE`、扫描状态为 `PASS` 的记录；**拒绝**用该接口读商品图或其他类型。
- 本地文件路径继续使用根目录约束，响应设置正确 MIME、`Content-Disposition: inline`、`Cache-Control: private, no-store` 和 `X-Content-Type-Options: nosniff`。
- 新营业执照不再保存永久公开 URL；管理员接口根据文件 ID 构造访问路径。
- 商品图片的公开 URL 和现有买家展示保持不变。
- 建议写操作日志 `admin_file_read`（可选，不挡验收）。

前端：

- 审核详情页使用带管理员 token 的 Blob 请求获取执照。
- 使用 `URL.createObjectURL` 展示图片，并在切换记录或卸载页面时释放对象 URL。
- 提供加载中、无执照、鉴权失败和文件损坏状态。

生产数据：

- 只处理现有 1 份营业执照记录：清除公开 URL 或改为受限访问路径，并验证旧 URL 已失效。
- 不轮换商品图片 URL，不批量移动无关文件。

完成标准：

1. 匿名请求执照为 404/403；正式环境推荐应用层和入口层同时限制。
2. 管理员可以预览。
3. 商家和买家调用管理员文件接口为 401/403。
4. 商品图片匿名仍为 200。
5. **不得**将 F-02 的商品图绑定校验或其他未实施部分标为本 Gate 完成项。

### 4.3 Checkpoint C：商家多库存订单

#### 数据模型

`products`：

- 新增 `reserved_stock INT NOT NULL DEFAULT 0`。
- 保留 `stock` 表示未售实物库存。
- 第一版保留但停止使用 `active_order_id`，待稳定版本再删除，降低首发回滚成本。

`orders`：

- 新增 `quantity INT NOT NULL DEFAULT 1`。
- `deal_price_cent` 明确为单件成交价。
- 删除唯一索引 `uk_product_active(product_id, is_active)`。
- 新增普通索引 `idx_order_product_active(product_id, is_active)`。
- 保留 `is_active` 作为快速筛选 CREATED 订单的标记。

不新增 `total_deal_price_cent` 持久化列，避免数量或单价变更后出现派生字段漂移。

GORM：

- `Order` 模型去掉 `uniqueIndex:uk_product_active`。
- 增加 `Quantity`、`Product.ReservedStock` 字段标签与迁移一致。
- **禁止**在模型上保留 unique 标签却只靠 SQL 删索引（AutoMigrate 可能重建）。

#### 创建订单

请求增加：

```json
{
  "product_id": 123,
  "quantity": 5,
  "deal_price_cent": 1200,
  "buyer_contact_masked": "可选",
  "remark": "可选"
}
```

服务端在同一事务内：

1. 校验商品属于当前商家、状态为 `ON_SHELF`、数量和单价为正数，且请求值不超过数据库字段范围；**不**要求/不执行进入 `LOCKED`。
2. 使用带条件的原子更新预占库存：只有 `stock - reserved_stock >= quantity` 时才能增加 `reserved_stock`。
3. 创建一笔包含 `quantity` 的订单（`is_active=true`），**不是**按数量拆成多笔订单；不写入 `product.active_order_id`，并记录事件。
4. 使用 64 位整数计算总价，并在乘法前验证 `deal_price_cent <= MaxInt64 / quantity`；溢出时拒绝请求。
5. 返回数量、成交单价、总价、总库存、预占库存和可售库存。

条件更新影响行数为 0 时返回库存不足冲突，不依赖“先读后写”，防止并发超卖。

#### 关闭订单

在同一事务内：

1. 按 `order.id + merchant_id` 使用 `SELECT ... FOR UPDATE`（GORM `clause.Locking{Strength: "UPDATE"}`）锁住订单行后再读取状态和数量。
2. 已是 `CLOSED` 时返回幂等成功；已是其他终态时拒绝；只允许 `CREATED -> CLOSED`。
3. 原子条件减少 `reserved_stock`（`reserved_stock >= quantity`），减少量等于订单 `quantity`。
4. 以 `WHERE id=? AND status='CREATED' AND is_active=true` 更新订单为 inactive/CLOSED，并要求 `RowsAffected=1`；失败则整笔事务回滚。
5. 保留商品当前的 `ON_SHELF` 或 `OFF_SHELF` 状态，**不**自动改为 `OFF_SHELF`，不自动重新上架。
6. **不**再要求商品处于 `LOCKED`。

重复或并发关闭不能重复释放库存。领域正确性依赖订单行锁/条件更新，不依赖当前非原子的 idempotency 记录层。

#### 完成订单

在同一事务内：

1. 按 `order.id + merchant_id` 使用 `SELECT ... FOR UPDATE` 锁住订单行后再读取状态和数量。
2. 已是 `COMPLETED` 时返回幂等成功；已是其他终态时拒绝；只允许 `CREATED -> COMPLETED`。
3. 单条条件更新：`stock -= quantity` 且 `reserved_stock -= quantity`（两边都足够才成功）。
4. 以 `WHERE id=? AND status='CREATED' AND is_active=true` 更新订单为 inactive/COMPLETED，并要求 `RowsAffected=1`；失败则整笔事务回滚。
5. 若更新后 `stock=0` 则设置 `SOLD`；仍有库存时保留商品当前上架/下架状态。
6. **不**再要求商品处于 `LOCKED`。

重复完成、并发双完成以及完成/关闭竞争都只能有一个状态转换成功，不能再次扣减或释放库存。

#### 商品管理规则

- 商品详情和列表展示总库存、预占库存和可售库存。
- 商家订单创建表单填写数量和单件成交价，实时显示总价。
- 创建按钮在可售库存为 0 时禁用。
- 商品下架允许存在 active 订单，但下架后不能创建新订单。
- 商品永久关闭和删除要求 `reserved_stock=0` 且不存在 active 订单。
- 库存编辑仍限定在草稿或已下架状态，新库存不得小于预占库存。
- 商家仪表盘“在售货值”改为 `price_cent * available_stock`。
- 列表/详情/日志不再依赖 `active_order_id` 或「整单锁定」文案。

#### `yaner` 的 3 个受保护商品

生产只读核对确认，这 3 个商品都归属于 `yaner`，但性质不同：

1. 2 个 `ON_SHELF / stock>1` 商品符合已经确认的多库存需求，**不是数据异常**，不得归一为 1、下架或修改库存。由于旧订单逻辑首次成交会错误整件售罄，在 Checkpoint C 发布前由 `yaner` 使用者避免为这两个商品创建旧式订单；需要技术门禁时另行确认，不直接改商品数据。
2. 1 个 `SOLD / stock>0` 商品存在状态与库存冲突。保持现状并禁止继续建单，取得 `yaner` 使用者对真实库存和成交历史的确认后，再单独修正状态或库存；修正前后留不含隐私的记录。
3. C 迁移和验收不得把三者批量当作异常数据修复。迁移后用前两个合法多库存商品验证读取与展示；真实订单测试使用专门测试商品或可完整回滚的数据。

完成标准：同一商品可以并发创建多笔订单，但成功预占总量不超过库存；关闭正确释放，完成正确扣减，库存归零才售罄；主路径无 `LOCKED`；旧唯一索引不存在且 AutoMigrate 不会重建。

## 5. 数据库迁移与现网 AutoMigrate 双轨

新增 `backend/migrations/0004_merchant_multi_stock.up.sql` 和对应 down 文件。
生产当前 **`AUTO_MIGRATE=true`** 且表名为 GORM 默认（如 `file_records`），必须按下述顺序操作，不能「只丢 SQL 文件期望自动发生」。

### 5.0 现网推荐执行顺序（维护窗）

§8.1 是唯一的生产维护窗顺序。本节只补充其迁移步骤：暂停所有订单写操作；对受保护 `yaner` 账号和商品采集指纹；只执行一次 `backend/migrations/0004_merchant_multi_stock.preflight.sql`；预检通过后只执行一次 `backend/migrations/0004_merchant_multi_stock.up.sql`；再执行 `backend/migrations/0004_merchant_multi_stock.postflight.sql`。任一门禁失败或 §5.1 停止条件命中即停止生产窗口。

在 SQL 迁移完成前不得启动新 API。新后端模型必须已去掉 `uk_product_active` unique 标签；迁移成功后 AutoMigrate 只作兼容性检查，不能重建唯一索引。发布 API 和商家前端后，按 §8.1 完成 health/auth/read、受控专用测试商品写验证、`yaner` 指纹比较和 30-60 分钟观察，才恢复订单入口。

**禁止**：模型仍带 `uniqueIndex:uk_product_active` 时依赖「人手 drop index」——进程重启/AutoMigrate 可能加回唯一约束并在第二笔历史单时再次炸库。

### 5.1 迁移前预检

1. 记录订单、商品、active 订单、`LOCKED` 商品和真正违反状态/库存不变量的商品数量。
2. 验证不存在 `stock < 0`、重复订单号或无法关联商品的订单。
3. 查询 active 订单；当前只读核验未发现 active 订单，但上线前必须重新确认。生产首发要求该数量为 0；若发现 active 订单，停止生产窗口并重新开始最终预检。
4. 列出所有 `status=LOCKED` 商品。任何 `LOCKED > 0` 都停止生产窗口；报告每个受影响行 ID 及其 active-order 计数，取得明确业务批准后才可逐行处置。禁止批量把所有受影响商品状态改写为同一值。
5. 分开记录两个合法 `ON_SHELF / stock>1` 商品和一个 `SOLD / stock>0` 冲突商品；迁移不得改动前两个，也不得自动修正第三个。
6. 完成数据库快照或可恢复备份。
7. 确认旧索引形态是预期的唯一 `(product_id,is_active)` 定义；不一致即停止生产窗口。确认将被删除的索引名（名称以现网为准，脚本勿写死错误名）。
8. 确认 `yaner` 恰有一个受保护账号且可采集发布前指纹；缺失、重复或无法采集均停止生产窗口。
9. 缺少可恢复备份证据即停止生产窗口。

### 5.2 结构与回填

1. 添加 `orders.quantity`，默认值为 1，并回填历史行。
2. 添加 `products.reserved_stock`，默认值为 0。
3. `reserved_stock` 首发回填为 0，并再次断言不存在 active 订单；本生产方案不在结构迁移中猜测或接管旧 active 订单。
4. 删除 `uk_product_active`（或现网实际 unique 名），创建普通复合索引 `idx_order_product_active(product_id, is_active)`。
5. 第一版不删除 `active_order_id`，只停止读写并置空已迁移关联。
6. 只处理已按 §5.1 取得明确业务批准的 `LOCKED` 行；不得批量状态改写。
7. 加入数据库 CHECK（MySQL 8.4 可用）或迁移后 SQL 验证：`quantity > 0`、`reserved_stock >= 0`、`reserved_stock <= stock`。

### 5.3 迁移后验证

- 历史订单全部为 `quantity=1`。
- 没有 `reserved_stock > stock`。
- active 订单预占汇总与商品 `reserved_stock` 一致。
- **唯一**索引 `uk_product_active` 已删除；普通索引存在。
- GORM 模型与 `SHOW CREATE TABLE orders` 均无该 unique。
- `LOCKED` 商品数量为 0；无法确认归属状态的遗留商品不得带入开放订单阶段。
- 商品和订单聚合金额与迁移前历史数据一致。
- 买家 API 的兼容字段 `stock` 等于可售库存；商家 API 同时返回总库存、预占库存和可售库存。

### 5.4 Down migration 边界

down migration 只用于尚未按新语义创建订单的开发/预发布环境。旧索引同时限制每个商品最多一笔 `is_active=true` 和一笔 `is_active=false` 订单；因此同商品出现多笔 active，或仅出现两笔已完成/已关闭历史订单，都会导致旧唯一索引无法恢复。生产一旦开放新订单，应停止写入并优先向前修复，不直接执行 down 或恢复旧代码。

## 6. 代码修改范围

后端预计修改：

- `backend/internal/model/models.go`（字段 + **去掉** order unique 标签）
- `backend/internal/dto/dto.go`（订单数量、管理员改密请求）
- `backend/internal/app/admin_handlers.go`（管理员自助改密）
- `backend/internal/app/auth_handlers.go` 或独立 session helper（撤销目标管理员 session）
- `backend/internal/middleware/auth.go` 或管理员专用中间件（校验 active admin session，使旧 access token 立即失效）
- `backend/internal/app/config.go`、`backend/internal/app/server.go`（生产默认值保护、移除固定管理员 seed、注册管理员改密路由）
- `backend/scripts/bootstrap_admin/main.go`（显式安全输入，不再内置固定密码）
- `backend/internal/app/product_handlers.go`
- `backend/internal/app/order_handlers.go`（去 LOCKED、预占、订单行锁/条件更新）
- `backend/internal/app/merchant_handlers.go`（仪表盘货值）
- `backend/internal/app/buyer_handlers.go`（买家兼容字段 `stock=available_stock`，§2.5）
- `backend/internal/stateflow/stateflow.go`（废弃订单驱动的 LOCKED 主路径）
- `backend/migrations/0004_merchant_multi_stock.up.sql`
- `backend/migrations/0004_merchant_multi_stock.down.sql`
- 相关 `backend/tests/*`（管理员改密/session、订单流、并发状态转换、锁定假设）

前端预计修改：

- `frontend/src/services/api.ts`
- 管理员安全设置页与路由（当前密码、新密码、确认密码；成功后清理 token 并回登录页）
- `frontend/src/pages/merchant/products/DetailPage.tsx`（将当前一键创建改为数量、单件成交价和自动总价表单）
- `frontend/src/pages/merchant/products/ListPage.tsx`（同样替换列表中的一键创建入口）
- `frontend/src/pages/merchant/products/CreatePage.tsx` / `EditPage.tsx`（库存与预占校验文案，如需要）
- `frontend/src/pages/merchant/orders/ListPage.tsx`
- `frontend/src/pages/merchant/orders/DetailPage.tsx`
- 可复用的订单创建表单/弹窗组件（若按现有结构抽取确有必要）
- 与以上页面同目录或现有测试目录中的 Vitest 测试

小程序目录：本阶段**不改页面功能**。现有页面继续读取 `stock`，由买家 API 将该兼容字段映射为可售库存；运行 miniapp 测试确认接口契约没有破坏。

文档同步（与对应代码同版完成，不延后）：

- `README.md`、`docs/acceptance-checklist.md`、`docs/project-overview.md` 和 `docs/dir-structure.md`：删除“公开固定口令即可初始化/验收”的生产指引，改为显式安全 bootstrap 与首次改密流程。
- `docs/specs.md`、`docs/data-model.md`、`docs/backend-api-checklist.md` 和 `docs/frontend-pages.md`：删除“库存固定为 1”和旧 `LOCKED` 单订单规则，补充数量、单件成交价、派生总价、预占库存及买家兼容字段语义。
- 两份代码审核报告保留为审查时点记录，不回写成“当初未发现”；只在本方案和后续修复记录中标注实际关闭状态。

## 7. 测试方案

### 7.1 后端

管理员改密：

- 未登录、商家和买家不能调用管理员改密接口。
- 当前密码错误、弱密码、与旧密码相同均被拒绝。
- 只更新当前 `admin_users.id`，`merchant_accounts` 行数和 `yaner` 密码哈希保持不变。
- 改密成功后只有 `user_type=ADMIN + user_id` 匹配的 session 被 revoke；新密码可登录、旧密码失败、旧 refresh 失败，相同数值 ID 的商家/买家 session 不受影响。
- 改密前签发的管理员 access token 在下一次任意鉴权请求立即返回 401；商家和 `yaner` token 行为不变。
- 两个管理员逐个轮换时，另一个账号始终可登录；测试不记录任何明文密码。
- 空库启动不会再自动生成固定密码管理员；安全 bootstrap 不覆盖已有 `admin` / `superadmin`，生产环境使用已知默认 JWT/DB 值时启动失败。

库存订单与状态机：

- 数量 0、负数、总价乘法溢出和超过可售库存被拒绝。
- 同一商品可创建多笔 active 订单；**创建后商品不为 `LOCKED`**。
- 同一商品完成/关闭第一笔订单后可以再创建并结束第二笔订单，证明多笔 inactive 历史不会再撞旧唯一索引。
- 多笔订单的预占总量不超过库存。
- 并发请求只允许库存范围内的订单成功，不发生负库存或超卖（例：stock=5，10 并发 qty=1 → 恰好 5 成功）。
- 关闭订单只释放一次库存；商品不因关闭被强制 `OFF_SHELF`。
- 完成订单只扣减一次库存；幂等重复完成不二次扣减。
- 同一订单使用不同 idempotency key 并发完成两次，只允许一次扣减；并发关闭两次只允许一次释放。
- 同一订单并发“完成 vs 关闭”只能有一个终态成功，库存与预占保持对应；测试必须覆盖商品还有其他订单预占的场景。
- 有剩余库存时商品不进入 SOLD；库存归零时才进入 SOLD。
- 下架后拒绝新订单，但既有订单可完成或关闭。
- 有预占库存时拒绝永久关闭、删除或把库存改到预占量以下。
- 历史 `quantity=1` 订单列表和详情保持兼容。
- 买家列表、详情、收藏和历史响应中的 `stock` 等于可售库存；商家响应中的 `stock` 仍为未售总库存。
- 旧测试若断言 `LOCKED` / 单 active unique，必须改写为新语义。
- 行锁、并发状态竞争、迁移 SQL 和索引断言必须在隔离的 MySQL 8.x 测试库执行；SQLite 回归可以保留，但不能单独作为这些生产语义的通过证据。

### 7.2 前端

- 管理员安全设置页校验当前密码、新密码和确认密码；成功后清理 token 并要求重新登录。
- 创建订单表单校验数量和单价，并显示正确总价。
- 可售库存不足时按钮和错误状态明确。
- 商品与订单列表、详情显示数量、单价、总价及三种库存。
- 文案不再暗示「整单锁定唯一订单」。

### 7.3 验证命令

```bash
make test
cd frontend && npm test
cd frontend && npm run build
cd miniapp && npm test
```

另用不含生产数据和生产凭据的 MySQL 8.x 临时库运行迁移、订单并发与索引回归；测试结束后核对库存不变量及 `SHOW INDEX FROM orders`。连接信息只通过测试环境 secret 注入，不写入命令记录或仓库。

`frontend npm test` 曾在 Ant Design 模块初始化阶段挂起，未取得完成结果；它不是绿色证据，Vitest 调查留在本次发布范围外。

小程序虽然不修改功能，仍运行测试确认无误改共享契约。发布前另执行：

- 域名入口 smoke
- `smoke-mysql-concurrency.mjs` 只在隔离 MySQL 环境运行，绝不指向生产
- 生产写验证只使用小数量的专用测试商家/商品，执行创建 -> 关闭及创建 -> 完成；不得使用 `yaner` 数据

## 8. 发布与回滚

### 8.1 发布顺序

生产维护窗只按以下顺序进行；任何步骤失败或停止条件命中都停止，不跳过或重排：

```text
recoverable backup evidence
-> protected yaner fingerprint
-> 0004 preflight
-> 0004 up migration exactly once
-> 0004 postflight
-> deploy API and admin frontend together
-> health/auth/read checks
-> controlled dedicated test product create/close/complete
-> protected yaner fingerprint comparison
-> 30-60 minute observation
```

唯一允许执行的 `0004` 文件路径为：

- `backend/migrations/0004_merchant_multi_stock.preflight.sql`
- `backend/migrations/0004_merchant_multi_stock.up.sql`
- `backend/migrations/0004_merchant_multi_stock.postflight.sql`

生产不得运行 `smoke-mysql-concurrency.mjs`。生产写验证只使用小数量的专用测试商家/商品，执行创建 -> 关闭及创建 -> 完成；不得使用 `yaner` 数据，也不得仅为测试轮换现有管理员密码。

Gate G1 资质私有化和 G2 miniapp 身份限制按业务触发条件独立安排，不插入上述库存发布链路。

### 8.2 回滚边界

- 管理员改密功能回滚时不得恢复公开初始化密码或旧密码哈希；已经轮换的密码继续有效，必要时使用受控的一次性重置流程。
- root 维护失败时可在双密码尚未废止的窗口恢复旧 compose 并重建 MySQL；新配置完全验证后才废止旧密码。
- 多库存功能开放前，可以回滚应用并保留新增列。
- 多库存功能开放后，只要已经按新语义创建过订单，就不得直接回滚旧代码；同商品存在多笔 active 或多笔 inactive 历史时均不得恢复旧唯一索引。生产回滚只允许前向修复。
- 数据不变量异常时立即关闭商家创建订单入口，保留完成/关闭已有订单的处置通道，优先向前修复。
- 回滚不得修改、禁用、重置或删除 `yaner`；两个合法多库存商品不得在回滚时机械改成 1。
- 回滚记录不得包含密码、token、openid、完整文件 URL。

## 9. 后续独立工作

### 9.1 身份迁移（F-12 完整闭环，后续独立实施）

- 当前库中有 19 条 mock 买家记录，但实际体验用户只有 2 人；文档和迁移不得把“记录数”写成“实际用户数”。
- 现有 mock 实现使用 `mock_wx_ + Taro.login()` 临时 code 作为 OpenID。同一体验用户重新登录时 code 会变化，因此**不能**只增加 `BUYER_MOCK_LOGIN_ALLOW_CREATE=false` 后声称旧账号仍可登录；该做法会在 token/session 失效后把真实体验用户一起挡住。
- 在身份迁移完成前，不通过 Nginx 全封登录，不切 `disabled`，不批量 revoke 买家 session，也不裸切 `real`。当前 mock 风险继续作为已知未关闭项记录，并由现有限流承担临时滥用控制；此项不阻塞 Checkpoint C。
- 先在非生产环境验证微信 AppID、AppSecret、合法域名和真实 `code2session`，再设计受控的账号绑定/合并流程：持有有效旧 session 的用户提交真实平台 code，服务端解析真实 OpenID，并将其绑定到人工确认的 canonical buyer。
- 盘点 19 条记录的收藏、历史、意向、设备绑定和 session，分别确认两名体验用户要保留的 canonical buyer；合并时使用事务处理唯一键冲突，并在变更前备份、变更后核对聚合数量。
- 若旧 session 已失效，只能在人工核验实际使用者后走一次性恢复/迁移流程；客户端提交的 `device_id` 不能单独作为账号认领凭证。
- 两名体验用户逐一完成真实登录、原数据可见和回滚验证后，生产才切换到 `real`，增加生产环境拒绝 mock 的启动校验，并停止创建新的 mock 身份。
- `/api/v1/buyer/auth/wechat-login` 和 `/api/v1/buyer/auth/miniapp-login` 共用同一创建/登录逻辑，迁移测试必须同时覆盖两个入口和 refresh/session 延续行为。

### 9.2 下一相关版本

- miniapp HTTP 401 刷新（F-05）。
- ~~匿名上传限流、配额和未绑定文件清理（F-06）~~：**代码侧已修复，测试服务器未审核，生产未执行 `0008` 且未部署**。专用 MySQL 8.4/代理验收和后续生产维护窗仍是独立门禁。
- 商品/执照 **file_id 归属与类型绑定**（F-02 全量）。
- ~~frontend logout 调用服务端（F-08）~~ **本分支已修复（2026-07-26）**：`api.logout()` + Layout 失败容忍退出；设计见 `docs/superpowers/specs/2026-07-26-frontend-server-logout-design.md`。随下次 frontend 发布上线。
- access token 吊销策略（F-14）和幂等原子性（F-15）。
- 买家意向 open 唯一索引迁移（F-11），仅在恢复意向入口前实施。
- migration、AutoMigrate 和文件表名一致性（F-09）：**本地修复并通过隔离 MySQL 8.4 测试服务器审核；生产未执行 0005**。`file_records` 为唯一契约；`0005` preflight/up/postflight 已入库（up 在 rename/no-op 前重复列与索引校验）；完整八态矩阵与文件流通过。现网已是 `file_records`，维护窗预期 no-op。
- 分类 schema 一致性（F-16）：**本地修复并通过同一隔离矩阵的 AutoMigrate RED→GREEN 审核；生产未部署**。模型与 SQL 均为 `(parent_id,name)`，API 与独立 seed 按父级查找且不再移动身份字段。
- 在统一 `file_records` schema 后，为匿名营业执照上传增加高熵一次性绑定凭证；注册时校验凭证并原子绑定到新商家账号，商家重提时继续按登录账号校验归属。

### 9.3 常规运维治理

- 非敏感部署配置版本化、镜像 commit 标签（D-06）。
- 容器 healthcheck、资源限制和日志轮转（D-05）。
- 安全响应头（D-07）和备份恢复演练（D-04 深化）。
- CI、lint 和发布 smoke 门禁。

## 10. 分项验收与整体关闭

以下是独立验收台账，不是一次发布的捆绑阻断清单。Checkpoint C 的硬前置只有 A3 可恢复备份及 C 自身门槛；A1、A2b、A2c 可以按各自窗口验收，未完成时保持对应状态为待办，但不因此扩大库存版本范围。

### 10.1 Checkpoint A

1. **A1**：公网 `8081` 已收口（含安全组确认或等价证明）。
2. **A2a**：应用库口令已轮换且旧密码失效。
3. **A2c**：两个管理员已逐个改密并通过新旧密码/session 验证；固定密码 seed 已移除，生产默认值保护生效；`admin`、`superadmin` 和 `yaner` 均未被删除、禁用或改名。
4. **A2b**：两个 root host 已轮换，真实 SQL 连接证明新密码可用、旧密码失效；MySQL/API 复测正常。
5. **A3**：备份证据可追溯，或最小异机备份已就绪。

### 10.2 Checkpoint C

1. A3 已通过，迁移前备份可恢复。
2. 商家后台支持多数量、多 active 订单和自动总价；主路径无 `LOCKED`。
3. 并发订单不会超卖；同订单双完成、双关闭和完成/关闭竞争不会重复释放或扣减，库存不变量持续成立。
4. 买家 API 的兼容 `stock` 字段返回可售库存，现有 miniapp 页面无需改动即可显示正确数量。
5. `uk_product_active` 已删除，普通索引存在，GORM 模型不会经 AutoMigrate 重建唯一索引。
6. 两个合法 `ON_SHELF / stock>1` 商品未被误改；一个 `SOLD / stock>0` 商品已由业务确认修正，或保持冻结并有明确跟踪项。
7. 数据库迁移记录、后端测试、miniapp 测试与前端构建已通过；`frontend npm test` 未完成且不是绿色证据。多库存相关文档已与新行为同步。
8. 发布和回滚记录不包含密码、token、openid、文件完整 URL 或其他敏感值。
9. **`yaner` 账号、密码、角色、归属和业务数据未被误改**，由实际使用者完成登录与只读访问验证。

### 10.3 整体状态

只有 A1、A2a、A2b、A2c、A3 和 C 各自通过后，才把“生产凭据收口 + 商家多库存”整体计划标记完成；单个分项可以更早独立完成。

Gate G1 资质文件私有化只在真实商家入驻/资质审核前验收；G2 mock 身份限制与真实身份迁移独立验收。二者未完成时不得宣称对应 F-13/F-12 已关闭，但不影响 Checkpoint C 的完成判断。
