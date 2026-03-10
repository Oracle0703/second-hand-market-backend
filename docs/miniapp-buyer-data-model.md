# 买家侧数据模型设计（miniapp-buyer-data-model）

## 默认假设
1. 数据库沿用当前项目关系型数据库（MySQL 8.x / sqlite dev）。
2. 买家侧支持游客行为归属（`device_id`）与登录归属（`buyer_id`）双轨并存。
3. 本期购买意向必须登录后提交。
4. 本期不引入支付订单模型，`buyer_intents` 仅表示线索，不驱动商品锁定。

---

## 1. 实体关系概览
1. `buyer_users` 1:N `buyer_device_bindings`。
2. `buyer_users` 1:N `buyer_favorites`（登录态收藏）。
3. `buyer_users` 1:N `buyer_histories`（登录态浏览记录）。
4. `buyer_users` 1:N `buyer_intents`（购买意向）。
5. `device_id` 1:N `buyer_favorites` / `buyer_histories`（游客态）。
6. `products` 1:N `buyer_favorites` / `buyer_histories` / `buyer_intents`。
7. `merchants` 1:N `buyer_intents`。

---

## 2. 核心数据表

### 2.1 buyer_users（买家账号）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| buyer_no | varchar(32) unique | 买家编号 |
| openid | varchar(64) unique | 微信 openid |
| unionid | varchar(64) null | 微信 unionid（可空） |
| nickname | varchar(64) null | 昵称 |
| avatar_url | varchar(500) null | 头像 |
| phone | varchar(20) null | 手机号（本期不强制） |
| status | varchar(16) | `ACTIVE/DISABLED` |
| last_login_at | datetime null | 最近登录时间 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |
| deleted_at | datetime null | 软删除 |

索引建议：
1. `uk_openid(openid)`
2. `uk_buyer_no(buyer_no)`
3. `idx_unionid(unionid)`
4. `idx_status_created(status, created_at)`

---

### 2.2 buyer_device_bindings（设备与买家绑定关系）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| device_id | varchar(64) | 设备唯一标识 |
| buyer_id | bigint | 买家 ID |
| first_bind_at | datetime | 首次绑定时间 |
| last_bind_at | datetime | 最近绑定时间 |
| last_merge_at | datetime null | 最近一次合并时间 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |

索引建议：
1. `uk_device_buyer(device_id, buyer_id)`
2. `idx_buyer_last_bind(buyer_id, last_bind_at)`
3. `idx_device_last_bind(device_id, last_bind_at)`

用途：
1. 支撑“一台设备多账号/一个账号多设备”行为追踪。
2. 支撑游客数据合并后的幂等判断与追溯。

---

### 2.3 buyer_favorites（收藏）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| owner_type | varchar(16) | `BUYER/DEVICE` |
| owner_key | varchar(96) | `B:{buyer_id}` 或 `D:{device_id}` |
| buyer_id | bigint null | 登录态归属（可空） |
| device_id | varchar(64) null | 游客态归属（可空） |
| product_id | bigint | 商品 ID |
| merchant_id | bigint | 商家 ID（冗余便于统计） |
| is_active | tinyint(1) | 1=有效收藏，0=已取消或已合并失活 |
| merge_target_buyer_id | bigint null | 合并目标买家 |
| merged_at | datetime null | 合并时间 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |

唯一约束：
1. `uk_owner_product(owner_key, product_id)`

索引建议：
1. `idx_buyer_active_created(buyer_id, is_active, created_at)`
2. `idx_device_active_created(device_id, is_active, created_at)`
3. `idx_product_active(product_id, is_active, created_at)`

说明：
1. 收藏取消不物理删除，更新 `is_active=0`，保留行为追踪。
2. 合并后原游客记录可置 `is_active=0` 并写入 `merge_target_buyer_id/merged_at`。

---

### 2.4 buyer_histories（浏览记录）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| owner_type | varchar(16) | `BUYER/DEVICE` |
| owner_key | varchar(96) | `B:{buyer_id}` 或 `D:{device_id}` |
| buyer_id | bigint null | 登录态归属（可空） |
| device_id | varchar(64) null | 游客态归属（可空） |
| product_id | bigint | 商品 ID |
| merchant_id | bigint | 商家 ID（冗余） |
| first_viewed_at | datetime | 首次浏览时间 |
| last_viewed_at | datetime | 最近浏览时间 |
| view_count | int | 浏览次数 |
| is_active | tinyint(1) | 1=有效记录，0=已合并失活 |
| merge_target_buyer_id | bigint null | 合并目标买家 |
| merged_at | datetime null | 合并时间 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |

唯一约束：
1. `uk_owner_product(owner_key, product_id)`

索引建议：
1. `idx_owner_last_view(owner_key, is_active, last_viewed_at)`
2. `idx_buyer_last_view(buyer_id, is_active, last_viewed_at)`
3. `idx_device_last_view(device_id, is_active, last_viewed_at)`
4. `idx_product_last_view(product_id, last_viewed_at)`

说明：
1. 同一 owner 同一商品重复浏览时，更新 `last_viewed_at` 与 `view_count`。
2. 合并时浏览次数累加，最近浏览时间取最大值。

---

### 2.5 buyer_intents（购买意向）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | bigint PK | 主键 |
| intent_no | varchar(32) unique | 意向单号 |
| buyer_id | bigint | 买家 ID（必填，登录后提交） |
| source_device_id | varchar(64) null | 提交设备 ID |
| product_id | bigint | 商品 ID |
| merchant_id | bigint | 商家 ID |
| status | varchar(16) | `NEW/CONTACTED/CLOSED` |
| is_open | tinyint(1) | 1=未关闭，0=已关闭 |
| contact_name | varchar(64) null | 联系人 |
| contact_phone | varchar(20) null | 手机号 |
| contact_wechat | varchar(64) null | 微信号 |
| message | varchar(500) null | 买家留言 |
| handled_by | bigint null | 处理人（商家账号 ID） |
| handled_at | datetime null | 处理时间 |
| closed_at | datetime null | 关闭时间 |
| close_reason | varchar(32) null | `DEAL_DONE/NO_RESPONSE/NOT_INTERESTED/INVALID` |
| merchant_note | varchar(255) null | 商家内部备注（不返回买家） |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |

唯一约束：
1. `uk_buyer_product_open(buyer_id, product_id, is_open)`

索引建议：
1. `idx_merchant_status_created(merchant_id, status, created_at)`
2. `idx_buyer_created(buyer_id, created_at)`
3. `idx_product_open(product_id, is_open)`
4. `idx_source_device_created(source_device_id, created_at)`

字段校验建议：
1. `contact_phone` 与 `contact_wechat` 至少一个非空。
2. `status=CLOSED` 时要求 `is_open=0` 且 `closed_at` 非空。
3. `status in (NEW, CONTACTED)` 时要求 `is_open=1`。

---

## 3. 状态枚举

### 3.1 买家账号状态
1. `ACTIVE`
2. `DISABLED`

### 3.2 owner_type
1. `BUYER`
2. `DEVICE`

### 3.3 意向状态
1. `NEW`：新提交，待商家处理。
2. `CONTACTED`：商家已联系。
3. `CLOSED`：线索关闭。

### 3.4 买家可见状态映射
1. `NEW` => 处理中
2. `CONTACTED` => 已联系
3. `CLOSED` => 已关闭

---

## 4. 一致性与事务规则
1. 游客合并必须事务化处理 `buyer_favorites` 与 `buyer_histories`。
2. 合并过程必须幂等：重复调用不会重复插入。
3. 意向创建前检查商品状态必须为 `ON_SHELF`。
4. 意向关闭时同步更新 `status/is_open/closed_at/close_reason`。
5. 商家处理动作必须记录操作日志（复用 `operation_logs`）。

---

## 5. 合并策略（device_id -> buyer_id）
1. 合并触发：登录成功后调用合并接口。
2. 收藏合并：
- 按 `(buyer owner_key, product_id)` upsert。
- 游客记录写 `merge_target_buyer_id` 与 `merged_at`，并 `is_active=0`。
3. 浏览合并：
- 按 `(buyer owner_key, product_id)` upsert。
- `last_viewed_at` 取更大值，`view_count` 累加。
- 游客记录打合并标记并失活。
4. 一台设备多账号：仅处理当前登录时仍未合并的数据。
5. 一个账号多设备：允许多次合并，靠唯一约束去重。

---

## 6. 预留扩展
1. `buyer_users.phone` 已预留后续强制手机号绑定能力。
2. `buyer_intents` 可扩展到预约到店时间、议价历史等。
3. 如后续引入支付订单，`buyer_intents` 可作为订单前置线索池，不需推翻本表结构。
