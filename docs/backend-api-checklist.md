# 后端接口设计与 Checklist（backend-api-checklist）

## 默认假设
1. API 前缀统一为 `/api/v1`。
2. 认证方式为 `Authorization: Bearer <access_token>`。
3. 响应体统一结构：`{ code, message, request_id, data }`。
4. 错误码由服务端维护字典并同步前端。

## 1. 通用接口规范

## 1.1 分页与筛选
- 请求参数：`page`（默认 1）、`page_size`（默认 20，最大 100）。
- 响应结构：`{ items: [], total, page, page_size }`。
- 筛选字段统一使用 query 参数，时间范围采用 `start_at`、`end_at`（ISO 8601）。

## 1.2 错误码建议

| 错误码 | 含义 |
| --- | --- |
| 0 | 成功 |
| 10001 | 参数校验失败 |
| 10002 | 未登录或 token 无效 |
| 10003 | 无权限访问 |
| 10004 | 资源不存在 |
| 10005 | 状态流转非法 |
| 10006 | 账号审核未通过 |
| 10007 | 账号已禁用 |
| 10008 | 上传文件不合法 |
| 10009 | 频率限制 |
| 20001 | 系统内部错误 |

## 1.3 权限标识
- `PUBLIC`：无需登录。
- `ADMIN`：平台管理员。
- `MERCHANT`：商家主账号（本期）。

## 2. 认证模块（auth）

| 路径 | 方法 | 用途 | 请求参数（关键） | 响应字段（关键） | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/auth/register` | POST | 商家注册 | `merchant_name, contact_name, phone, username, password, license_file_id` | `merchant_id, review_status` | PUBLIC |
| `/auth/login` | POST | 登录 | `username, password` | `access_token, refresh_token, expires_in, user{role,id,merchant_id}` | PUBLIC |
| `/auth/refresh` | POST | 刷新令牌 | `refresh_token` | `access_token, refresh_token, expires_in` | PUBLIC |
| `/auth/logout` | POST | 退出登录 | 无（从 token 获取会话） | `success` | ADMIN/MERCHANT |

补充规则：
1. `PENDING`/`REJECTED` 商家登录返回 `10006`。
2. 密码存储必须为安全哈希，不可明文。

## 3. 商家侧资料与审核状态模块（merchant-onboarding）

| 路径 | 方法 | 用途 | 请求参数（关键） | 响应字段（关键） | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/profile` | GET | 获取当前商家资料与审核状态 | 无 | `merchant_info, review_status, reject_reason` | MERCHANT |
| `/merchant/reapply` | POST | 驳回后重新提交资料 | `merchant_name?, contact_name?, phone?, license_file_id?` | `review_status` | MERCHANT |

补充规则：
1. 仅 `REJECTED` 允许 `reapply`。
2. `reapply` 成功后状态变更为 `PENDING` 并写审核日志。

## 4. 管理员审核模块（admin-audit）

| 路径 | 方法 | 用途 | 请求参数（关键） | 响应字段（关键） | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/admin/merchants` | GET | 商家审核列表 | `status?, keyword?, start_at?, end_at?, page, page_size` | `items[{id,name,contact,phone,status,created_at}], total` | ADMIN |
| `/admin/merchants/:id` | GET | 商家审核详情 | `id(path)` | `merchant_detail, audit_logs[]` | ADMIN |
| `/admin/merchants/:id/approve` | POST | 审核通过 | `id(path), comment?` | `status=APPROVED, reviewed_at` | ADMIN |
| `/admin/merchants/:id/reject` | POST | 审核驳回 | `id(path), reason` | `status=REJECTED, reviewed_at` | ADMIN |
| `/admin/merchants/:id/freeze` | POST | 冻结商家（可选） | `id(path), reason` | `status=DISABLED` | ADMIN |

补充规则：
1. 仅 `PENDING` 允许 approve/reject。
2. reject 必填原因，长度限制建议 200。
3. 所有审核动作必须写 `merchant_audit_logs` 与 `operation_logs`。

## 5. 商品管理模块（products）

| 路径 | 方法 | 用途 | 请求参数（关键） | 响应字段（关键） | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/products` | POST | 创建商品 | `title, description, category_id, price_cent, condition_level, stock, image_file_ids[]` | `product_id, status` | MERCHANT |
| `/merchant/products/:id` | PUT | 编辑商品 | `title?, description?, price_cent?, condition_level?, stock?, image_file_ids?` | `product_id, status, updated_at` | MERCHANT |
| `/merchant/products/:id` | GET | 商品详情 | `id(path)` | `product_detail` | MERCHANT |
| `/merchant/products` | GET | 商品列表 | `status?, keyword?, start_at?, end_at?, page, page_size` | `items[], total` | MERCHANT |
| `/merchant/products/:id/on-shelf` | POST | 上架 | `id(path)` | `status=ON_SHELF` | MERCHANT |
| `/merchant/products/:id/off-shelf` | POST | 下架 | `id(path)` | `status=OFF_SHELF` | MERCHANT |
| `/merchant/products/:id/close` | POST | 关闭商品 | `id(path), reason?` | `status=CLOSED` | MERCHANT |
| `/merchant/products/:id/mark-sold` | POST | 标记成交（保底接口） | `id(path), deal_price_cent, note?` | `status=SOLD, order_id` | MERCHANT |

补充规则：
1. 跨商家访问返回 `10003`。
2. 状态流转非法返回 `10005`。
3. 上架前校验必填字段与图片数量。

## 6. 轻量订单模块（orders-lite）

| 路径 | 方法 | 用途 | 请求参数（关键） | 响应字段（关键） | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/orders` | POST | 创建订单 | `product_id, deal_price_cent, buyer_contact_masked?, remark?` | `order_id, order_no, status=CREATED` | MERCHANT |
| `/merchant/orders` | GET | 订单列表 | `status?, keyword?, page, page_size` | `items[], total` | MERCHANT |
| `/merchant/orders/:id` | GET | 订单详情 | `id(path)` | `order_detail, events[]` | MERCHANT |
| `/merchant/orders/:id/complete` | POST | 完成订单 | `id(path), note?` | `status=COMPLETED, completed_at` | MERCHANT |
| `/merchant/orders/:id/close` | POST | 关闭订单 | `id(path), reason?` | `status=CLOSED, closed_at` | MERCHANT |

补充规则：
1. `complete` 时要求订单处于 `CREATED` 且商品处于可成交状态。
2. `complete` 成功后商品状态更新为 `SOLD`（事务内执行）。
3. `close` 不自动恢复商品状态，由商家自行处理商品上下架。

## 7. 文件上传模块（uploads）

| 路径 | 方法 | 用途 | 请求参数（关键） | 响应字段（关键） | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/files/presign` | POST | 获取上传凭证 | `biz_type, file_name, file_size, mime_type` | `upload_url, object_key, file_id, expire_at` | ADMIN/MERCHANT/PUBLIC(注册场景可放开) |
| `/files/confirm` | POST | 上传完成确认 | `file_id, object_key` | `file_id, url, status` | ADMIN/MERCHANT/PUBLIC(注册场景可放开) |

补充规则：
1. 限制 MIME 与文件大小。
2. 上传回调/确认后写 `files` 元数据。

## 8. 审计日志模块（operation-logs）

| 路径 | 方法 | 用途 | 请求参数（关键） | 响应字段（关键） | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/admin/logs` | GET | 管理员查看全局日志 | `operator_type?, action?, resource_type?, start_at?, end_at?, page, page_size` | `items[], total` | ADMIN |
| `/merchant/logs` | GET | 商家查看本商家日志 | `action?, resource_type?, start_at?, end_at?, page, page_size` | `items[], total` | MERCHANT |

## 9. Dashboard 模块（可选但建议）

| 路径 | 方法 | 用途 | 请求参数（关键） | 响应字段（关键） | 权限 |
| --- | --- | --- | --- | --- | --- |
| `/merchant/dashboard` | GET | 商家仪表盘统计 | 无 | `product_stats, order_stats, todo_count` | MERCHANT |

## 10. 后端交付 Checklist

### 10.1 接口实现检查
1. 所有接口均有 DTO 参数校验（必填、长度、枚举、范围）。
2. 所有写接口具备权限拦截与资源归属校验。
3. 所有状态变更接口统一走状态机校验函数。
4. 所有关键写操作在事务中保证主数据与日志一致性。
5. 所有列表接口支持统一分页结构。

### 10.2 安全与合规检查
1. 密码哈希与敏感字段脱敏输出已完成。
2. 登录与上传接口具备限流策略。
3. request_id 全链路透传，日志可检索。
4. 关键操作已写审计日志，且包含前后状态。

### 10.3 文档与测试检查
1. OpenAPI 文档与实际 handler 路径一致。
2. 单元测试覆盖状态机与权限判断核心逻辑。
3. 集成测试覆盖主流程：注册-审核-登录-商品-订单闭环。
4. 错误码表与前端映射文档同步。
