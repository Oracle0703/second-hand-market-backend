# 后端接口设计与 Checklist（backend-api-checklist）

## 默认假设
1. API 前缀统一为 `/api/v1`。
2. 认证方式为 `Authorization: Bearer <access_token>`。
3. 响应体统一结构：`{ code, message, request_id, data }`。
4. 写接口支持 `Idempotency-Key`（建议 UUID）用于防重复提交。

## 1. 通用接口规范

### 1.1 分页与筛选
- 请求参数：`page`（默认 1）、`page_size`（默认 20，最大 100）。
- 响应结构：`{ items: [], total, page, page_size }`。
- 筛选字段统一使用 query 参数，时间范围采用 `start_at`、`end_at`（ISO 8601）。

### 1.2 错误码建议

| 错误码 | 含义 |
| --- | --- |
| 0 | 成功 |
| 10001 | 参数校验失败 |
| 10002 | 未登录或 token 无效 |
| 10003 | 无权限访问 |
| 10004 | 资源不存在 |
| 10005 | 状态流转非法 |
| 10006 | 商家账号处于受限制登录态，仅可访问 onboarding 接口 |
| 10007 | 账号已禁用 |
| 10008 | 上传文件不合法 |
| 10009 | 频率限制 |
| 10010 | 并发冲突（如商品已被占用） |
| 10011 | 重复提交（幂等键冲突） |
| 20001 | 系统内部错误 |

### 1.3 权限标识
- `PUBLIC`：无需登录。
- `ADMIN`：平台管理员。
- `MERCHANT`：商家主账号（本期）。

### 1.3.1 merchant token_scope
- `full`：可访问全部商家经营能力。
- `onboarding`：仅可访问入驻流程能力（profile/reapply/资质上传）。

### 1.4 并发与幂等策略
1. 订单创建、订单完成、订单关闭、商品上架/下架/关闭属于关键写接口。
2. 后端规则：
   - 同一 `Idempotency-Key + operator_id + path` 只执行一次。
   - 重复请求返回首次执行结果；参数不同则返回 `10011`。
3. 并发控制：
   - 创建订单时对商品行加锁，并检查商品状态为 `ON_SHELF` 且无活动订单。
   - `orders` 使用 `uk_product_active(product_id, is_active)` 避免同商品并发 `CREATED`。

## 2. 认证模块（auth）

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/auth/register` | POST | 商家注册 | `merchant_name(R), contact_name(R), phone(R), username(R), password(R), license_file_id(R)` | `merchant_id, merchant_no, review_status` | PUBLIC |
| `/auth/login` | POST | 登录 | `login_type(R: ADMIN/MERCHANT), username(R), password(R)` | `access_token, refresh_token, expires_in, token_scope(full/onboarding), review_status, user{id,role,merchant_id?}` | PUBLIC |
| `/auth/refresh` | POST | 刷新令牌 | `refresh_token(R)` | `access_token, refresh_token, expires_in` | PUBLIC |
| `/auth/logout` | POST | 退出登录 | 无 | `success` | ADMIN/MERCHANT |

restricted login 规则：
1. 商家 `PENDING/REJECTED` 登录成功并返回 `token_scope=onboarding`。
2. 商家 `APPROVED` 登录成功并返回 `token_scope=full`。
3. `onboarding` token 访问经营接口（商品/订单/仪表盘/商家日志/账号设置）返回 `10006`。
4. 管理员或商家账号 `DISABLED` 登录返回 `10007`。

## 3. 商家主体资料与审核状态模块（merchant-onboarding）

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/profile` | GET | 获取当前商家资料与审核状态 | 无 | `merchant_info{id,name,contact,phone}, review_status, reject_reason` | MERCHANT(full/onboarding) |
| `/merchant/reapply` | POST | 驳回后重新提交资料 | `merchant_name(O), contact_name(O), phone(O), license_file_id(O)` | `review_status` | MERCHANT(onboarding) |

职责边界：
1. `merchant/profile` 仅面向“商家主体 + 审核状态”，不返回账号安全信息。
2. 账号层信息（用户名、密码修改、安全设置）统一由 `merchant/account` 域接口提供。

失败场景：
1. 当前非 `REJECTED` 调用 `reapply` 返回 `10005`。

onboarding scope 白名单：
1. `GET /merchant/profile`
2. `POST /merchant/reapply`
3. `POST /files/presign`、`POST /files/confirm`（仅允许 `biz_type=MERCHANT_LICENSE`）

onboarding scope 黑名单：
1. `/merchant/account*`
2. `/merchant/dashboard`
3. `/merchant/categories`
4. `/merchant/products*`
5. `/merchant/orders*`
6. `/merchant/logs`

## 4. 商家账号与安全设置模块（merchant-account）

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/account` | GET | 获取当前登录账号资料与安全设置 | 无 | `account{id,username,role,status,last_login_at}, security{password_updated_at,mfa_enabled}` | MERCHANT(full) |
| `/merchant/account/password` | PUT | 修改当前登录账号密码 | `old_password(R), new_password(R)` | `success, password_updated_at` | MERCHANT(full) |

失败场景：
1. `old_password` 错误返回 `10001`（或业务子码）。
2. `new_password` 不满足强度策略返回 `10001`。
3. 账号状态为 `DISABLED` 返回 `10007`。

## 5. 管理员审核模块（admin-audit）

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/admin/merchants` | GET | 商家审核列表 | `status(O), keyword(O), start_at(O), end_at(O), page(O), page_size(O)` | `items[{id,merchant_no,merchant_name,contact_name,contact_phone,review_status,created_at}], total,page,page_size` | ADMIN |
| `/admin/merchants/:id` | GET | 商家审核详情 | `id(path,R)` | `merchant_detail, audit_logs[]` | ADMIN |
| `/admin/merchants/:id/approve` | POST | 审核通过 | `id(path,R), comment(O)` | `merchant_id, review_status, reviewed_at, reviewed_by` | ADMIN |
| `/admin/merchants/:id/reject` | POST | 审核驳回 | `id(path,R), reason(R)` | `merchant_id, review_status, reviewed_at, reviewed_by, reject_reason` | ADMIN |

失败场景：
1. 非 `PENDING` 执行 `approve/reject` 返回 `10005`。
2. `reject` 未传 `reason` 返回 `10001`。

## 6. 分类字典模块（categories）

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/categories` | GET | 查询分类字典 | `level(O:1/2), parent_id(O), status(O)` | `items[{id,parent_id,level,name,status,sort}]` | MERCHANT(full) |

说明：
1. 新建/编辑商品页面先请求一级分类，再根据 `parent_id` 请求二级分类。
2. 本期不提供分类增删改接口。

## 7. 商品管理模块（products）

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/products` | POST | 创建商品 | `title(R), description(R), category_id(R), price_cent(R), original_price_cent(R), condition_level(R), stock(R,>0), image_file_ids(R[])` | `product_id, product_no, status, stock, created_at` | MERCHANT(full) |
| `/merchant/products/:id` | PUT | 编辑商品 | `id(path,R), title(O), description(O), category_id(O), price_cent(O), original_price_cent(O), condition_level(O), stock(O), image_file_ids(O[])` | `product_id, status, stock, updated_at` | MERCHANT(full) |
| `/merchant/products/:id` | GET | 商品详情 | `id(path,R)` | `product{id,title,status,category,price_cent,condition_level,stock,images[],active_order_id}` | MERCHANT(full) |
| `/merchant/products` | GET | 商品列表 | `status(O), keyword(O), start_at(O), end_at(O), page(O), page_size(O)` | `items[{id,title,status,price_cent,stock,updated_at}], total,page,page_size` | MERCHANT(full) |
| `/merchant/products/:id/on-shelf` | POST | 上架 | `id(path,R)` | `product_id, from_status, to_status, changed_at` | MERCHANT(full) |
| `/merchant/products/:id/off-shelf` | POST | 下架 | `id(path,R)` | `product_id, from_status, to_status, changed_at` | MERCHANT(full) |
| `/merchant/products/:id/close` | POST | 关闭商品 | `id(path,R), reason(O)` | `product_id, from_status, to_status, changed_at` | MERCHANT(full) |
| `/merchant/products/:id/stock-adjustments` | POST | 调整库存 | `id(path,R), adjustment_type(R:INCREASE/DECREASE/MARK_SOLD), quantity(R,>0), reason(R,2-255)` | `product_id, movement_id, adjustment_type, quantity, stock_before, stock_after, status_before, status_after, adjusted_at` | MERCHANT(full) |

编辑约束：
1. `DRAFT/OFF_SHELF`：允许业务字段编辑（标题、描述、分类、价格、成色、图片），兼容保留 `stock` 编辑；需要审计原因的库存变化应使用库存调整接口。
2. `ON_SHELF`：仅允许 `description,image_file_ids`。
3. `LOCKED/SOLD/CLOSED`：禁止编辑。
4. `stock` 是当前可用库存，必须为正整数；手动增加、减少、线下售出扣减使用 `/stock-adjustments` 记录流水。

库存调整规则：
1. `INCREASE`：库存增加，商品状态不变。
2. `DECREASE`：库存减少，不能扣成负数；`ON_SHELF` 扣到 `0` 时自动转 `OFF_SHELF`。
3. `MARK_SOLD`：按线下售出扣减库存；扣到 `0` 时商品转 `SOLD`，不创建订单，不计入订单销售额。
4. `LOCKED/SOLD/CLOSED` 商品不允许库存调整。

失败场景：
1. 跨商家访问返回 `10003`。
2. 非法状态流转返回 `10005`。
3. 分类不存在或非二级分类返回 `10001`。
4. 创建商品或调整库存时数量小于等于 `0` 返回 `10001`。
5. 库存扣减数量大于当前库存返回 `10005`。

幂等说明：
1. `on-shelf/off-shelf/close` 重复请求且目标状态已达成时返回成功（`code=0`，`idempotent=true`）。
2. `stock-adjustments` 支持 `Idempotency-Key`；相同幂等键和相同请求体重复提交只执行一次。

## 8. 轻量订单模块（orders-lite）

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/orders` | POST | 创建订单 | `product_id(R), deal_price_cent(R), buyer_contact_masked(O), remark(O)` | `order_id, order_no, status, product_status` | MERCHANT(full) |
| `/merchant/orders` | GET | 订单列表 | `status(O), keyword(O), page(O), page_size(O)` | `items[{id,order_no,product_id,product_title,status,deal_price_cent,created_at}], total,page,page_size` | MERCHANT(full) |
| `/merchant/orders/:id` | GET | 订单详情 | `id(path,R)` | `order_detail{id,order_no,status,deal_price_cent,product}, events[]` | MERCHANT(full) |
| `/merchant/orders/:id/complete` | POST | 完成订单 | `id(path,R), note(O)` | `order_id, from_status, to_status, product_status, completed_at` | MERCHANT(full) |
| `/merchant/orders/:id/close` | POST | 关闭订单 | `id(path,R), reason(O)` | `order_id, from_status, to_status, product_status, closed_at` | MERCHANT(full) |

联动规则：
1. 创建订单：`product ON_SHELF -> LOCKED`。
2. 完成订单：`order CREATED -> COMPLETED` + `product LOCKED -> SOLD`。
3. 关闭订单：`order CREATED -> CLOSED` + `product LOCKED -> OFF_SHELF`。

失败场景：
1. 商品非 `ON_SHELF` 创建订单返回 `10005`。
2. 商品已有活动订单（`CREATED`）返回 `10010`。
3. 订单非 `CREATED` 调用 `complete/close` 返回 `10005`。

幂等说明：
1. `complete/close` 重复请求如果订单已到目标状态返回成功并标记 `idempotent=true`。
2. 若已到另一个终态（如已 `CLOSED` 再调 `complete`）返回 `10005`。

## 9. 文件上传模块（uploads）

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/files/presign` | POST | 获取上传凭证 | `biz_type(R), file_name(R), file_size(R), mime_type(R)` | `upload_url, object_key, file_id, expire_at` | ADMIN/MERCHANT/PUBLIC(注册场景) |
| `/files/upload` | POST | 上传图片二进制内容 | `file_id(R), object_key(R), file(binary,R)` | `file_id, url, object_key, status` | ADMIN/MERCHANT/PUBLIC(注册场景) |
| `/files/confirm` | POST | 上传完成确认 | `file_id(R), object_key(R)` | `file_id, url, status` | ADMIN/MERCHANT/PUBLIC(注册场景) |

失败场景：
1. 仅允许 `jpeg/png/webp/heic/heif` 静态图片；`mov` 等视频返回 `10008`。
2. 原图超过 `40MB` 返回 `10008`。
3. 服务端会统一压缩图片，压缩目标为 `20MB`，但不是最终拒绝门槛。
4. `onboarding` token 上传 `PRODUCT_IMAGE` 返回 `10006`。
5. `PUBLIC` 身份上传非资质类文件返回 `10003`。

## 10. 审计日志模块（operation-logs）

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/admin/logs` | GET | 管理员查看全局日志 | `operator_type(O), action(O), resource_type(O), start_at(O), end_at(O), page(O), page_size(O)` | `items[{id,operator,action,resource_type,resource_id,from_status,to_status,result_code,created_at}], total,page,page_size` | ADMIN |
| `/merchant/logs` | GET | 商家查看本商家日志 | `action(O), resource_type(O), start_at(O), end_at(O), page(O), page_size(O)` | `items[{id,action,resource_type,resource_id,from_status,to_status,result_code,created_at}], total,page,page_size` | MERCHANT(full) |

## 11. Dashboard 模块

| 路径 | 方法 | 用途 | 请求参数 | 响应字段 | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/dashboard` | GET | 商家仪表盘统计 | 无 | `product_stats{draft,on_shelf,locked,off_shelf,sold,closed}, order_stats{created,completed,closed}` | MERCHANT(full) |

## 12. 关键接口 JSON 示例

### 11.1 注册（`POST /api/v1/auth/register`）

```json
{
  "merchant_name": "上海数码回收店",
  "contact_name": "张三",
  "phone": "13800138000",
  "username": "merchant_zhangsan",
  "password": "Passw0rd!2026",
  "license_file_id": 10001
}
```

```json
{
  "code": 0,
  "message": "OK",
  "request_id": "req_01HRX",
  "data": {
    "merchant_id": 9001,
    "merchant_no": "M202603100001",
    "review_status": "PENDING"
  }
}
```

### 11.2 登录（`POST /api/v1/auth/login`）

```json
{
  "login_type": "MERCHANT",
  "username": "merchant_zhangsan",
  "password": "Passw0rd!2026"
}
```

```json
{
  "code": 0,
  "message": "OK",
  "request_id": "req_01HRY",
  "data": {
    "access_token": "eyJhbGciOi...",
    "refresh_token": "eyJhbGciOi...",
    "expires_in": 7200,
    "token_scope": "onboarding",
    "review_status": "PENDING",
    "user": {
      "id": 7001,
      "role": "OWNER",
      "merchant_id": 9001
    }
  }
}
```

### 11.3 审核通过（`POST /api/v1/admin/merchants/:id/approve`）

```json
{
  "comment": "资料齐全，审核通过"
}
```

```json
{
  "code": 0,
  "message": "OK",
  "request_id": "req_01HRZ",
  "data": {
    "merchant_id": 9001,
    "review_status": "APPROVED",
    "reviewed_at": "2026-03-10T09:30:00+08:00",
    "reviewed_by": 1
  }
}
```

### 11.4 审核驳回（`POST /api/v1/admin/merchants/:id/reject`）

```json
{
  "reason": "营业执照照片不清晰，请重新上传"
}
```

```json
{
  "code": 0,
  "message": "OK",
  "request_id": "req_01HRA",
  "data": {
    "merchant_id": 9001,
    "review_status": "REJECTED",
    "reviewed_at": "2026-03-10T09:40:00+08:00",
    "reviewed_by": 1,
    "reject_reason": "营业执照照片不清晰，请重新上传"
  }
}
```

### 11.5 商品创建（`POST /api/v1/merchant/products`）

```json
{
  "title": "iPhone 14 128G",
  "description": "正常使用，无拆修",
  "category_id": 2102,
  "price_cent": 329900,
  "condition_level": "GOOD",
  "stock": 1,
  "image_file_ids": [3001, 3002, 3003]
}
```

```json
{
  "code": 0,
  "message": "OK",
  "request_id": "req_01HRB",
  "data": {
    "product_id": 12001,
    "product_no": "P202603100001",
    "status": "DRAFT",
    "created_at": "2026-03-10T10:00:00+08:00"
  }
}
```

### 11.6 商品上架（`POST /api/v1/merchant/products/:id/on-shelf`）

```json
{}
```

```json
{
  "code": 0,
  "message": "OK",
  "request_id": "req_01HRC",
  "data": {
    "product_id": 12001,
    "from_status": "DRAFT",
    "to_status": "ON_SHELF",
    "changed_at": "2026-03-10T10:05:00+08:00",
    "idempotent": false
  }
}
```

### 11.7 调整库存（`POST /api/v1/merchant/products/:id/stock-adjustments`）

```json
{
  "adjustment_type": "MARK_SOLD",
  "quantity": 1,
  "reason": "客户线下购买"
}
```

```json
{
  "code": 0,
  "message": "OK",
  "request_id": "req_01HRC2",
  "data": {
    "product_id": 12001,
    "movement_id": 81001,
    "adjustment_type": "MARK_SOLD",
    "quantity": 1,
    "stock_before": 1,
    "stock_after": 0,
    "status_before": "ON_SHELF",
    "status_after": "SOLD",
    "adjusted_at": "2026-03-10T10:10:00+08:00"
  }
}
```

### 11.8 创建订单（`POST /api/v1/merchant/orders`）

```json
{
  "product_id": 12001,
  "deal_price_cent": 320000,
  "buyer_contact_masked": "13****8000",
  "remark": "线下当面交易"
}
```

```json
{
  "code": 0,
  "message": "OK",
  "request_id": "req_01HRD",
  "data": {
    "order_id": 50001,
    "order_no": "O202603100001",
    "status": "CREATED",
    "product_status": "LOCKED"
  }
}
```

### 11.9 完成订单（`POST /api/v1/merchant/orders/:id/complete`）

```json
{
  "note": "已完成交付"
}
```

```json
{
  "code": 0,
  "message": "OK",
  "request_id": "req_01HRE",
  "data": {
    "order_id": 50001,
    "from_status": "CREATED",
    "to_status": "COMPLETED",
    "product_status": "SOLD",
    "completed_at": "2026-03-10T10:20:00+08:00",
    "idempotent": false
  }
}
```

## 13. 后端交付 Checklist

### 12.1 接口实现检查
1. 所有接口均有 DTO 参数校验（必填、长度、枚举、范围）。
2. 所有写接口具备权限拦截与资源归属校验。
3. 所有状态变更接口统一走状态机校验函数。
4. 订单创建/完成/关闭在事务内同时更新商品与订单。
5. 关键写接口支持幂等键，重复请求行为可预测。
6. 所有列表接口支持统一分页结构。

### 12.2 安全与合规检查
1. 密码哈希与敏感字段脱敏输出已完成。
2. 登录与上传接口具备限流策略。
3. request_id 全链路透传，日志可检索。
4. 关键操作已写审计日志，且包含前后状态。

### 12.3 文档与测试检查
1. OpenAPI 文档与实际 handler 路径一致。
2. 单元测试覆盖状态机、权限判断、幂等逻辑。
3. 集成测试覆盖主流程：注册-审核-登录-商品-订单闭环。
4. 错误码表与前端映射文档同步。
