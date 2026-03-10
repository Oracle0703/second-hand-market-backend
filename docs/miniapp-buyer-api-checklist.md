# 买家侧接口 Checklist（miniapp-buyer-api-checklist）

## 默认假设
1. API 前缀统一为 `/api/v1`。
2. 统一响应结构：`{ code, message, request_id, data }`。
3. 游客请求默认携带 `X-Device-Id`。
4. 本期购买意向必须登录。
5. 本文只覆盖买家侧与意向闭环必需接口，不扩展支付/售后/聊天。

---

## 1. 商品列表

| 项 | 内容 |
| --- | --- |
| method | `GET` |
| path | `/api/v1/buyer/products` |
| 请求参数 | `keyword(O), category_id(O), page(O), page_size(O), sort(O:latest/price_asc/price_desc)` |
| 响应字段 | `items[{id,title,price_cent,condition_level,cover_url,status,merchant_id,merchant_name,is_favorited}], total,page,page_size` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`120 req/min/device` |

规则：
1. 默认仅返回 `ON_SHELF`。
2. 不返回商家后台管理字段（如内部状态变更日志）。

---

## 2. 商品详情

| 项 | 内容 |
| --- | --- |
| method | `GET` |
| path | `/api/v1/buyer/products/:id` |
| 请求参数 | `id(path,R)` |
| 响应字段 | `product{id,title,description,price_cent,condition_level,status,images[],merchant{id,name},is_favorited,can_submit_intent}` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`120 req/min/device` |

规则：
1. `OFF_SHELF/SOLD/LOCKED` 可查看详情但 `can_submit_intent=false`。
2. `DRAFT/CLOSED` 返回 `10004`（对买家不可见）。

---

## 3. 分类

| 项 | 内容 |
| --- | --- |
| method | `GET` |
| path | `/api/v1/buyer/categories` |
| 请求参数 | `level(O:1/2), parent_id(O)` |
| 响应字段 | `items[{id,parent_id,level,name,sort}]` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`120 req/min/device` |

规则：
1. 仅返回 `ENABLED` 分类。

---

## 4. 微信登录与会话

### 4.1 微信登录

| 项 | 内容 |
| --- | --- |
| method | `POST` |
| path | `/api/v1/buyer/auth/wechat-login` |
| 请求参数 | `code(R), device_id(R), nickname(O), avatar_url(O)` |
| 响应字段 | `access_token, refresh_token, expires_in, user{id,buyer_no,nickname,avatar_url,phone?}` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`20 req/min/device` + `120 req/min/ip` |

### 4.2 刷新令牌

| 项 | 内容 |
| --- | --- |
| method | `POST` |
| path | `/api/v1/buyer/auth/refresh` |
| 请求参数 | `refresh_token(R)` |
| 响应字段 | `access_token, refresh_token, expires_in` |
| 游客可访问 | 否 |
| 是否必须登录 | 否（凭 refresh） |
| 是否需要限流 | 是，`60 req/min/device` |

### 4.3 退出登录

| 项 | 内容 |
| --- | --- |
| method | `POST` |
| path | `/api/v1/buyer/auth/logout` |
| 请求参数 | 无 |
| 响应字段 | `success` |
| 游客可访问 | 否 |
| 是否必须登录 | 是 |
| 是否需要限流 | 否 |

### 4.4 游客数据合并

| 项 | 内容 |
| --- | --- |
| method | `POST` |
| path | `/api/v1/buyer/guest/merge` |
| 请求参数 | `device_id(R)` |
| 响应字段 | `merged{favorites_count,histories_count}, merged_at` |
| 游客可访问 | 否 |
| 是否必须登录 | 是 |
| 是否需要限流 | 是，`10 req/min/buyer` |

规则：
1. 合并幂等，重复请求不重复计数。
2. 仅处理未合并游客数据。

---

## 5. 收藏

### 5.1 获取收藏列表

| 项 | 内容 |
| --- | --- |
| method | `GET` |
| path | `/api/v1/buyer/favorites` |
| 请求参数 | `page(O), page_size(O)` |
| 响应字段 | `items[{product_id,title,cover_url,price_cent,status,favorited_at}], total,page,page_size` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`120 req/min/owner` |

### 5.2 添加收藏

| 项 | 内容 |
| --- | --- |
| method | `POST` |
| path | `/api/v1/buyer/favorites` |
| 请求参数 | `product_id(R)` |
| 响应字段 | `product_id, is_favorited` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`60 req/min/owner` |

### 5.3 取消收藏

| 项 | 内容 |
| --- | --- |
| method | `DELETE` |
| path | `/api/v1/buyer/favorites/:product_id` |
| 请求参数 | `product_id(path,R)` |
| 响应字段 | `product_id, is_favorited` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`60 req/min/owner` |

规则：
1. owner = 登录买家或 device_id 游客。
2. 仅允许对买家可见商品操作收藏。

---

## 6. 浏览记录

### 6.1 浏览上报

| 项 | 内容 |
| --- | --- |
| method | `POST` |
| path | `/api/v1/buyer/histories/views` |
| 请求参数 | `product_id(R), viewed_at(O)` |
| 响应字段 | `product_id, last_viewed_at, view_count` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`120 req/min/owner` + `同商品30秒去重` |

### 6.2 获取浏览记录

| 项 | 内容 |
| --- | --- |
| method | `GET` |
| path | `/api/v1/buyer/histories` |
| 请求参数 | `page(O), page_size(O)` |
| 响应字段 | `items[{product_id,title,cover_url,price_cent,status,last_viewed_at,view_count}], total,page,page_size` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`120 req/min/owner` |

### 6.3 清空浏览记录

| 项 | 内容 |
| --- | --- |
| method | `DELETE` |
| path | `/api/v1/buyer/histories` |
| 请求参数 | `product_id(O)`（为空表示清空全部） |
| 响应字段 | `success` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`30 req/min/owner` |

---

## 7. 购买意向

### 7.1 创建购买意向

| 项 | 内容 |
| --- | --- |
| method | `POST` |
| path | `/api/v1/buyer/intents` |
| 请求参数 | `product_id(R), contact_name(O), contact_phone(O), contact_wechat(O), message(O)` |
| 响应字段 | `intent_id, intent_no, status, created_at` |
| 游客可访问 | 否 |
| 是否必须登录 | 是 |
| 是否需要限流 | 是，`5 req/min/buyer` + `20 req/day/buyer` + `30 req/day/device` |

规则：
1. `contact_phone/contact_wechat` 至少一个必填。
2. 商品必须为 `ON_SHELF`。
3. 同买家同商品仅允许 1 条未关闭意向（冲突返回 `10010`）。

### 7.2 我的意向列表

| 项 | 内容 |
| --- | --- |
| method | `GET` |
| path | `/api/v1/buyer/intents` |
| 请求参数 | `status(O:NEW/CONTACTED/CLOSED), page(O), page_size(O)` |
| 响应字段 | `items[{id,intent_no,product{id,title,cover_url},status,buyer_status_text,created_at,updated_at}], total,page,page_size` |
| 游客可访问 | 否 |
| 是否必须登录 | 是 |
| 是否需要限流 | 是，`120 req/min/buyer` |

### 7.3 意向详情

| 项 | 内容 |
| --- | --- |
| method | `GET` |
| path | `/api/v1/buyer/intents/:id` |
| 请求参数 | `id(path,R)` |
| 响应字段 | `intent{id,intent_no,status,buyer_status_text,product,contact_masked,message,created_at,updated_at}` |
| 游客可访问 | 否 |
| 是否必须登录 | 是 |
| 是否需要限流 | 是，`120 req/min/buyer` |

规则：
1. 买家仅可查看自己的意向。
2. 不返回商家内部备注和处理人信息。

---

## 8. 我的汇总

| 项 | 内容 |
| --- | --- |
| method | `GET` |
| path | `/api/v1/buyer/me/summary` |
| 请求参数 | 无 |
| 响应字段 | `is_login, profile{buyer_id?,nickname?,avatar_url?}, counters{favorites,histories,intents_open}` |
| 游客可访问 | 是 |
| 是否必须登录 | 否 |
| 是否需要限流 | 是，`120 req/min/owner` |

---

## 9. 配套商家端接口（意向闭环依赖）

说明：以下接口不属于买家端调用，但属于本期闭环必须新增。

| method | path | 用途 | 是否需要商家登录 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/merchant/intents` | 意向线索列表 | 是（MERCHANT full） |
| `GET` | `/api/v1/merchant/intents/:id` | 意向线索详情 | 是（MERCHANT full） |
| `POST` | `/api/v1/merchant/intents/:id/contacted` | 标记已联系 | 是（MERCHANT full） |
| `POST` | `/api/v1/merchant/intents/:id/close` | 关闭线索 | 是（MERCHANT full） |

状态流转：
1. `NEW -> CONTACTED`
2. `NEW/CONTACTED -> CLOSED`

---

## 10. 错误与幂等约定
1. 统一沿用现有错误码体系：
- `10001` 参数错误
- `10002` 未登录
- `10003` 无权限
- `10004` 资源不存在
- `10005` 非法状态
- `10009` 频率限制
- `10010` 冲突（重复未关闭意向等）
2. 意向创建建议支持 `Idempotency-Key`，避免重复点击提交。
3. 频控命中返回 `10009`，前端统一提示“操作过于频繁，请稍后重试”。
