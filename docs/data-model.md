# 数据模型设计（data-model）

## 默认假设
1. 数据库：MySQL 8.x，字符集 `utf8mb4`。
2. 通用字段：业务表默认包含 `id, created_at, updated_at, deleted_at`（软删除）。
3. 金额字段统一使用分（`*_cent`），避免浮点误差。
4. 枚举字段使用 `VARCHAR` + 业务层枚举校验（便于演进）。

## 1. 实体关系概览
1. `merchants`（商家主体）1:N `merchant_accounts`（商家账号）。
2. `merchants` 1:N `products`（商品）。
3. `categories`（分类字典）1:N `products`。
4. `products` 1:N `product_images`（商品图片）。
5. `products` 1:N `orders`（轻量订单）。
6. `orders` 1:N `order_events`（订单事件流）。
7. `merchants` 1:N `merchant_audit_logs`（审核日志）。
8. 所有关键动作写入 `operation_logs`。

## 2. 核心数据表

### 2.1 merchants（商家主体）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| merchant_no | varchar(32) unique | 商家编号 |
| merchant_name | varchar(128) | 商家名称 |
| contact_name | varchar(64) | 联系人 |
| contact_phone | varchar(20) | 联系电话 |
| contact_email | varchar(128) null | 邮箱 |
| license_no | varchar(64) null | 营业执照号 |
| license_file_id | bigint null | 营业执照文件 ID |
| review_status | varchar(16) | `PENDING/APPROVED/REJECTED/DISABLED` |
| reject_reason | varchar(255) null | 驳回原因 |
| reviewed_by | bigint null | 审核管理员 ID |
| reviewed_at | datetime null | 审核时间 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |
| deleted_at | datetime null | 软删除时间 |

索引建议：
1. `uk_merchant_no(merchant_no)`
2. `idx_review_status(review_status, created_at)`
3. `idx_contact_phone(contact_phone)`

### 2.2 merchant_accounts（商家账号）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| merchant_id | bigint | 商家 ID |
| username | varchar(64) unique | 登录账号 |
| password_hash | varchar(255) | 密码哈希 |
| role | varchar(16) | `OWNER/STAFF` |
| status | varchar(16) | `ACTIVE/DISABLED` |
| last_login_at | datetime null | 最后登录时间 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |
| deleted_at | datetime null | 软删除时间 |

索引建议：
1. `uk_username(username)`
2. `idx_merchant_role(merchant_id, role)`

### 2.3 admin_users（平台管理员）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| username | varchar(64) unique | 管理员账号 |
| password_hash | varchar(255) | 密码哈希 |
| display_name | varchar(64) | 显示名 |
| role | varchar(16) | `SUPER_ADMIN/ADMIN` |
| status | varchar(16) | `ACTIVE/DISABLED` |
| last_login_at | datetime null | 最后登录时间 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |
| deleted_at | datetime null | 软删除时间 |

初始化说明：
1. 本期管理员由初始化脚本导入。
2. 本期不提供管理员管理页面与相关业务接口。

### 2.4 merchant_audit_logs（商家审核日志）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| merchant_id | bigint | 商家 ID |
| action | varchar(32) | `SUBMIT/APPROVE/REJECT/REAPPLY` |
| from_status | varchar(16) | 变更前状态 |
| to_status | varchar(16) | 变更后状态 |
| reason | varchar(255) null | 原因 |
| operator_type | varchar(16) | `ADMIN/MERCHANT` |
| operator_id | bigint | 操作人 ID |
| created_at | datetime | 创建时间 |

索引建议：
1. `idx_merchant_created(merchant_id, created_at)`

### 2.5 categories（分类字典）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| parent_id | bigint null | 父分类 ID，一级分类为空 |
| level | tinyint | `1/2` |
| name | varchar(64) | 分类名称 |
| status | varchar(16) | `ENABLED/DISABLED` |
| sort | int | 排序值（越小越靠前） |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |
| deleted_at | datetime null | 软删除时间 |

索引建议：
1. `idx_parent(parent_id, sort)`
2. `idx_level_status(level, status, sort)`
3. `uk_parent_name(parent_id, name)`

约束说明：
1. 商品 `category_id` 必须引用二级分类（`level=2`）。
2. 本期分类数据由初始化脚本维护，不通过后台页面维护。

### 2.6 products（商品）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| product_no | varchar(32) unique | 商品编号 |
| merchant_id | bigint | 商家 ID |
| title | varchar(128) | 标题 |
| description | text | 描述 |
| category_id | bigint | 二级分类 ID |
| price_cent | int | 售价（分） |
| original_price_cent | int null | 原价（分） |
| condition_level | varchar(16) | `LIKE_NEW/GOOD/FAIR/POOR` |
| stock | int | 尚未售出的实物总库存 |
| reserved_stock | int | 未完成订单已预占库存，默认 0 |
| cover_file_id | bigint null | 封面图文件 ID |
| status | varchar(16) | `DRAFT/ON_SHELF/LOCKED/OFF_SHELF/SOLD/CLOSED` |
| active_order_id | bigint null | 首版回滚兼容列；新订单主流程不读写 |
| locked_at | datetime null | 锁定时间 |
| shelf_at | datetime null | 上架时间 |
| off_shelf_at | datetime null | 下架时间 |
| sold_at | datetime null | 成交时间 |
| closed_at | datetime null | 关闭时间 |
| created_by | bigint | 创建账号 ID |
| updated_by | bigint | 更新账号 ID |
| version | int | 乐观锁版本 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |
| deleted_at | datetime null | 软删除时间 |

索引建议：
1. `uk_product_no(product_no)`
2. `idx_merchant_status(merchant_id, status, updated_at)`
3. `idx_merchant_title(merchant_id, title)`
4. `idx_active_order(active_order_id)`

约束说明：
1. `stock >= 0`、`reserved_stock >= 0` 且 `reserved_stock <= stock`。
2. `available_stock=stock-reserved_stock` 为可继续创建订单的数量，不单独持久化。

### 2.7 product_images（商品图片）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| product_id | bigint | 商品 ID |
| file_id | bigint | 文件 ID |
| sort_order | int | 排序 |
| created_at | datetime | 创建时间 |

索引建议：
1. `idx_product_sort(product_id, sort_order)`

### 2.8 orders（轻量订单）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| order_no | varchar(32) unique | 订单号 |
| merchant_id | bigint | 商家 ID |
| product_id | bigint | 商品 ID |
| quantity | int | 购买数量，正整数，历史订单回填为 1 |
| deal_price_cent | int | 单件成交价（分） |
| buyer_contact_masked | varchar(64) null | 买家联系方式（脱敏） |
| status | varchar(16) | `CREATED/COMPLETED/CLOSED` |
| is_active | tinyint | `1=CREATED`，`0=非CREATED`，用于快速筛选 |
| close_reason | varchar(255) null | 关闭原因 |
| created_by | bigint | 创建账号 ID |
| completed_at | datetime null | 完成时间 |
| closed_at | datetime null | 关闭时间 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |
| deleted_at | datetime null | 软删除时间 |

索引建议：
1. `uk_order_no(order_no)`
2. `idx_merchant_status(merchant_id, status, created_at)`
3. `idx_product_id(product_id)`
4. `idx_order_product_active(product_id, is_active)`（普通查询索引）

实现说明：
1. 创建订单时写入 `status=CREATED`、`is_active=1`。
2. 完成/关闭订单时更新 `is_active=0`。
3. 同一商品允许多笔 active 和历史订单；创建订单使用条件更新预占库存，完成/关闭使用订单行锁与订单状态 CAS。
4. 整单总价由 `quantity * deal_price_cent` 使用 64 位整数派生，不单独持久化。

### 2.9 order_events（订单事件）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| order_id | bigint | 订单 ID |
| event_type | varchar(32) | `CREATE/COMPLETE/CLOSE` |
| from_status | varchar(16) null | 变更前状态 |
| to_status | varchar(16) | 变更后状态 |
| operator_type | varchar(16) | `MERCHANT/ADMIN/SYSTEM` |
| operator_id | bigint | 操作人 ID |
| note | varchar(255) null | 备注 |
| created_at | datetime | 创建时间 |

索引建议：
1. `idx_order_created(order_id, created_at)`

### 2.10 file_records（文件元数据）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| biz_type | varchar(32) | `MERCHANT_LICENSE/PRODUCT_IMAGE/OTHER` |
| object_key | varchar(255) | 对象存储路径 |
| url | varchar(500) | 访问地址 |
| mime_type | varchar(64) | MIME 类型 |
| size_bytes | bigint | 文件大小 |
| uploader_type | varchar(16) | `ADMIN/MERCHANT/PUBLIC` |
| uploader_id | bigint null | 上传者 ID |
| scan_status | varchar(16) | `PENDING/PASS/BLOCKED` |
| owner_merchant_id | bigint unsigned null | 文件绑定的商家 ID；已绑定商品图和营业执照必须有值 |
| capability_token_hash | char(64) null | PUBLIC 匿名上传的一次性 capability token SHA-256；绑定成功后清空 |
| capability_expires_at | datetime(3) null | capability token 失效时间；绑定成功后清空 |
| source_ip_hash | char(64) null | 可信来源 IP 规范化字节的 HMAC-SHA256 小写 hex；仅迁移后匿名 presign 写入 |
| cleanup_after | datetime(3) null | 匿名孤儿最早清理时间；NULL 永不参加自动清理 |
| cleanup_claimed_at | datetime(3) null | 清理 claim 时间 |
| cleanup_claim_token | char(64) null | 清理批次随机 claim token |
| cleanup_attempts | int unsigned | 清理 claim 次数，默认 0 |
| created_at | datetime | 创建时间 |

索引建议：
1. `uk_object_key(object_key)`
2. `idx_biz_type_created(biz_type, created_at)`
3. `idx_file_owner_biz_scan(owner_merchant_id, biz_type, scan_status)`
4. `uk_file_capability_token(capability_token_hash)`
5. `idx_file_capability_expires(capability_expires_at)`
6. `idx_file_source_created(source_ip_hash, created_at)`
7. `idx_file_cleanup_candidate(uploader_type, owner_merchant_id, cleanup_after, cleanup_claimed_at)`

表名以完整 SQL migration 链和 `FileRecord.TableName()` 的
`file_records` 为准；历史 `0001` 中的 `files` 由 `0005` 兼容迁移收敛。
`0006` 为文件绑定增加商家归属和匿名一次性 capability。商品图片只能绑定
到 `owner_merchant_id` 相同、`biz_type=PRODUCT_IMAGE`、`scan_status=PASS`
且 URL 非空的文件；营业执照对应 `MERCHANT_LICENSE`。PUBLIC 营业执照在
注册事务内使用原始 `file_token` 原子认领，成功后写入商家归属并清空 token
hash/失效时间；同一 capability 只能成功认领一次。

`0008_anonymous_upload_governance` 不回填任何历史行的治理字段。迁移后匿名记录的清理时间为：

```text
cleanup_after = max(
  capability_expires_at + FILE_UPLOAD_CLEANUP_GRACE_SECONDS,
  created_at + 1 hour
)
```

第二项保证清理不会在滚动一小时限频窗口结束前删除计数证据。活跃匿名配额统计要求 `PUBLIC + owner_merchant_id IS NULL + source_ip_hash 匹配 + cleanup_after IS NOT NULL`，即 capability 过期但尚未清理的记录仍占用配额。绑定成功后保留 `source_ip_hash` 供滚动限频使用，但 owner 非空使记录永不进入清理候选。

### 2.10.1 file_quota_guards（文件配额串行 guard）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | tinyint unsigned PK | 固定为 1 |
| guard_name | varchar(32) unique | 固定为 `file_records` |
| created_at | datetime(3) | guard 创建时间，默认 `CURRENT_TIMESTAMP(3)` |

该表与 `file_records` 都必须使用 InnoDB。所有增加配额占用的事务在 READ COMMITTED 下先执行 `SELECT ... FOR UPDATE` 锁定固定行，再读取全局、商家或匿名聚合并创建/绑定记录。表、引擎或固定行漂移时 migration fail closed；应用不得降级绕过配额。

### 2.11 operation_logs（操作审计日志）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| request_id | varchar(64) | 请求链路 ID |
| operator_type | varchar(16) | `ADMIN/MERCHANT/SYSTEM` |
| operator_id | bigint | 操作人 ID |
| merchant_id | bigint null | 商家 ID（管理员操作可为空） |
| action | varchar(64) | 操作类型 |
| resource_type | varchar(32) | 资源类型（merchant/product/order） |
| resource_id | bigint | 资源 ID |
| from_status | varchar(16) null | 变更前状态 |
| to_status | varchar(16) null | 变更后状态 |
| method | varchar(8) | HTTP 方法 |
| path | varchar(255) | 请求路径 |
| ip | varchar(64) | 来源 IP |
| user_agent | varchar(255) | UA |
| result_code | int | 结果码 |
| detail_json | json | 额外上下文 |
| created_at | datetime | 创建时间 |

索引建议：
1. `idx_operator_created(operator_type, operator_id, created_at)`
2. `idx_resource(resource_type, resource_id, created_at)`
3. `idx_merchant_created(merchant_id, created_at)`

### 2.12 auth_sessions（登录会话）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| user_type | varchar(16) | `ADMIN/MERCHANT` |
| user_id | bigint | 用户 ID |
| refresh_token_hash | varchar(255) | refresh token 哈希 |
| device_info | varchar(255) null | 设备信息 |
| ip | varchar(64) null | 登录 IP |
| expired_at | datetime | 过期时间 |
| revoked_at | datetime null | 失效时间 |
| created_at | datetime | 创建时间 |

索引建议：
1. `idx_user_expired(user_type, user_id, expired_at)`
2. `idx_token_hash(refresh_token_hash)`

## 3. 关键枚举定义

### 3.1 商家审核状态
- `PENDING`：待审核
- `APPROVED`：审核通过
- `REJECTED`：审核驳回
- `DISABLED`：平台冻结

### 3.2 商品状态
- `DRAFT`
- `ON_SHELF`
- `LOCKED`
- `OFF_SHELF`
- `SOLD`
- `CLOSED`

### 3.3 订单状态
- `CREATED`
- `COMPLETED`
- `CLOSED`

### 3.4 管理员角色
- `SUPER_ADMIN`
- `ADMIN`

## 4. 一致性与事务规则
1. 创建订单必须在同一事务中以 `stock-reserved_stock >= quantity` 为条件增加预占并写入订单。
2. 完成订单必须在同一事务中锁定订单行，同时按数量减少 `stock` 与 `reserved_stock`；仅 `stock=0` 时置为 `SOLD`。
3. 关闭订单必须在同一事务中锁定订单行并按数量释放 `reserved_stock`，不改变商品上下架状态。
4. 任何状态变更必须同时写 `operation_logs`。
5. 非法状态变更统一返回业务错误码 `10005`。
6. 删除策略采用软删除；审核/审计相关表禁止物理删除。
7. `LOCKED` 与 `active_order_id` 仅保留兼容；迁移后主路径不再写入。

## 5. 预留扩展（子账号与 RBAC）
1. `merchant_accounts.role` 已支持 `STAFF`。
2. 后续可新增：
   - `merchant_roles`
   - `merchant_permissions`
   - `merchant_account_roles`
3. 本期不生成子账号管理接口，仅保证表结构兼容。
