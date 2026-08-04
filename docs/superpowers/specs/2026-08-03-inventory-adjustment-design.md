# 手动调整库存入口设计

| 项目 | 内容 |
|---|---|
| 日期 | 2026-08-03 |
| 分支 | `feature/hy/10005_inventory_adjustment` |
| 目标 | 商家后台在商品列表和商品详情中直接调整库存，支持补库存、减少库存、按线下售出扣减库存 |
| 非目标 | 不改订单数量模型、不引入 `reserved_stock`、不支持同一商品多个活动订单、不要求小程序发版 |

## 1. 当前状态

| 位置 | 当前行为 | 问题 |
|---|---|---|
| `frontend/src/pages/merchant/products/ListPage.tsx` | 商品列表展示库存；`DRAFT/OFF_SHELF` 显示“编辑/上架”，`ON_SHELF` 显示“下架/创建订单” | 在售商品没有编辑入口，也没有直接调整库存入口 |
| `frontend/src/pages/merchant/products/DetailPage.tsx` | 商品详情展示库存；操作按钮与列表类似 | 商家需要进入编辑页或绕状态流转才能处理库存 |
| `frontend/src/pages/merchant/products/EditPage.tsx` | `DRAFT/OFF_SHELF` 可编辑 `stock`，`ON_SHELF` 不可编辑库存 | 对客户不直观，且没有调整原因和流水 |
| `backend/internal/app/product_handlers.go` | `PUT /merchant/products/:id` 在允许编辑的状态下可直接覆盖 `stock` | 覆盖式修改无法区分“补库存、盘亏、线下售出” |
| `backend/internal/model/models.go` | `products.stock` 已是正整数库存字段 | 旧文档仍写“库存固定 1”，需要随本需求修正 |

## 2. 需求边界

| 类型 | 结论 |
|---|---|
| 用户 | 后端商家后台用户，当前系统内部运行，只有商家登录；小程序端无用户登录发版需求 |
| 核心需求 | 除下单交易外，商家可以手动调整库存；扣减库存可以记录为普通减少，也可以记录为线下售出 |
| 兼容要求 | 小程序不发版；继续读取现有商品 `status` 和 `stock` 即可 |
| 审计要求 | 每次调整要有流水，包含调整前后库存、调整类型、原因、操作人、时间 |
| 安全要求 | 只能调整当前商家自己的商品，沿用 `MERCHANT full` 权限 |

## 3. 方案比较

| 方案 | 做法 | 优点 | 缺点 | 结论 |
|---|---|---|---|---|
| A. 只在编辑页开放库存 | 继续复用 `PUT /merchant/products/:id` 覆盖 `stock` | 改动最小 | 没有原因、没有调整类型、在售商品仍不方便；无法证明“卖掉/减少”的差异 | 不采用 |
| B. 独立库存调整接口 + 调整流水 | 新增库存调整 API、流水表、前端弹窗入口 | 行为清晰，可审计，改动可控，不影响小程序 | 需要新增后端接口和前端弹窗 | 采用 |
| C. 完整库存预留/多订单模型 | 新增 `reserved_stock`、订单数量、库存预留与释放 | 长期库存模型完整 | 范围大，会触碰订单主流程和小程序；当前需求不需要 | 不采用 |

本次采用方案 B。它满足客户“方便修改库存”的直接诉求，同时避免把库存调整入口扩大成订单系统重构。

## 4. 目标行为

### 4.1 前端入口

| 页面 | 入口 | 显示条件 | 行为 |
|---|---|---|---|
| 商品列表 | 操作列新增“调整库存”按钮 | `DRAFT`、`ON_SHELF`、`OFF_SHELF` | 打开调整库存弹窗 |
| 商品详情 | 顶部操作区和按钮卡片新增“调整库存”按钮 | `DRAFT`、`ON_SHELF`、`OFF_SHELF` | 打开同一个调整库存弹窗 |

禁止在 `LOCKED`、`SOLD`、`CLOSED` 商品上显示可执行入口。后端仍必须做相同校验，不能只依赖前端。

### 4.2 弹窗字段

| 字段 | 说明 |
|---|---|
| 当前库存 | 只读展示当前 `stock` |
| 调整类型 | `INCREASE` 补充库存、`DECREASE` 减少库存、`MARK_SOLD` 线下售出 |
| 调整数量 | 正整数；`DECREASE` 和 `MARK_SOLD` 不能超过当前库存 |
| 调整原因 | 必填，2 到 255 字，例如“盘点调整”“客户线下购买”“损耗” |

### 4.3 调整规则

| 调整类型 | 库存变化 | 状态变化 | 业务含义 |
|---|---|---|---|
| `INCREASE` | `stock_after = stock_before + quantity` | 状态保持不变 | 补货、盘盈、录入遗漏 |
| `DECREASE` | `stock_after = stock_before - quantity` | 如果 `ON_SHELF` 扣到 0，自动转 `OFF_SHELF`；其他允许状态保持不变 | 盘亏、损耗、非销售原因减少 |
| `MARK_SOLD` | `stock_after = stock_before - quantity` | 扣到 0 时转 `SOLD` 并写入 `sold_at`；未扣到 0 时状态保持不变 | 线下售出或其他非平台订单售出 |

`MARK_SOLD` 不创建订单、不计入平台销售额、不改变订单列表。它只表示库存来源上的“售出扣减”。后续如果要统计线下销售额，需要单独设计“线下订单/成交登记”。

## 5. 后端接口

### 5.1 API

```text
POST /api/v1/merchant/products/:id/stock-adjustments
```

权限：`MERCHANT full`。

幂等：支持 `Idempotency-Key`，复用现有 `runWithIdempotency` 机制。同一 `Idempotency-Key + operator_id + path` 的相同请求返回第一次执行结果；请求体不同返回重复提交错误。

### 5.2 请求

```json
{
  "adjustment_type": "INCREASE",
  "quantity": 3,
  "reason": "盘点补录"
}
```

| 字段 | 类型 | 约束 |
|---|---|---|
| `adjustment_type` | string | 必填，取值 `INCREASE`、`DECREASE`、`MARK_SOLD` |
| `quantity` | int | 必填，必须大于 0；扣减类不能超过当前库存 |
| `reason` | string | 必填，去掉首尾空格后长度 2 到 255 |

### 5.3 响应

```json
{
  "product_id": 123,
  "movement_id": 456,
  "adjustment_type": "INCREASE",
  "quantity": 3,
  "stock_before": 2,
  "stock_after": 5,
  "status_before": "ON_SHELF",
  "status_after": "ON_SHELF",
  "adjusted_at": "2026-08-03T18:30:00+08:00"
}
```

### 5.4 失败处理

| 场景 | 返回码 | 说明 |
|---|---|---|
| 参数缺失、类型非法、数量小于等于 0、原因过短或过长 | `10001` | `invalid argument` |
| 商品不存在 | `10004` | `resource not found` |
| 调整其他商家的商品 | `10003` | `forbidden` |
| `LOCKED`、`SOLD`、`CLOSED` 商品调整库存 | `10005` | `invalid status transition` |
| `DECREASE` / `MARK_SOLD` 数量大于当前库存 | `10005` | 避免库存变负 |
| 同一幂等键重复提交不同请求体 | `10011` | `duplicate submit` |

## 6. 数据模型

新增表：`product_stock_adjustments`。

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGINT PK | 调整流水 ID |
| `product_id` | BIGINT | 商品 ID |
| `merchant_id` | BIGINT | 商家 ID |
| `adjustment_type` | VARCHAR(32) | `INCREASE`、`DECREASE`、`MARK_SOLD` |
| `quantity` | INT | 本次调整数量 |
| `stock_before` | INT | 调整前库存 |
| `stock_after` | INT | 调整后库存 |
| `status_before` | VARCHAR(16) | 调整前商品状态 |
| `status_after` | VARCHAR(16) | 调整后商品状态 |
| `reason` | VARCHAR(255) | 调整原因 |
| `operator_id` | BIGINT | 商家账号 ID |
| `created_at` | DATETIME | 调整时间 |

索引：

| 索引 | 字段 | 用途 |
|---|---|---|
| `idx_product_stock_adjustment_created` | `product_id, created_at` | 商品维度流水查询 |
| `idx_merchant_stock_adjustment_created` | `merchant_id, created_at` | 商家维度审计查询 |

## 7. 并发与一致性

| 规则 | 设计 |
|---|---|
| 原子性 | 库存更新和调整流水写入放在同一个数据库事务中 |
| 防负库存 | 事务内读取当前库存后校验扣减数量，保存时以计算后的 `stock_after` 为准 |
| 并发调整 | 使用行级锁或等价的事务串行化方式读取商品，避免两个扣减请求同时基于同一个旧库存计算 |
| 操作日志 | 除专用流水外，继续写 `operation_logs`，`action = product_stock_adjust` |
| 失败回滚 | 任一校验或写入失败时，商品库存和流水都不落库 |

## 8. 小程序兼容

| 项目 | 结论 |
|---|---|
| 是否需要小程序发版 | 不需要 |
| 买家列表 | 继续读取后端 `GET /buyer/products` 返回的 `stock` 和 `status` |
| 买家详情 | 继续读取后端 `GET /buyer/products/:id` 返回的 `stock` 和 `status` |
| 扣到 0 后的可见性 | `DECREASE` 扣到 0 的在售商品自动下架；`MARK_SOLD` 扣到 0 的商品转售出；小程序不会继续把它作为可购买商品展示 |

## 9. 文档同步

| 文件 | 调整 |
|---|---|
| `docs/backend-api-checklist.md` | 增加库存调整接口；修正“库存固定 1”的旧描述 |
| `docs/frontend-pages.md` | 增加商品列表/详情的调整库存入口；修正库存不允许编辑的旧描述 |
| `docs/data-model.md` | 增加 `product_stock_adjustments` 表；修正 `products.stock` 说明 |

## 10. 验收标准

| 编号 | 验收项 |
|---|---|
| AC1 | 商家在商品列表可看到 `DRAFT/ON_SHELF/OFF_SHELF` 商品的“调整库存”按钮 |
| AC2 | 商家在商品详情可看到 `DRAFT/ON_SHELF/OFF_SHELF` 商品的“调整库存”按钮 |
| AC3 | `INCREASE` 能增加库存，并记录调整流水 |
| AC4 | `DECREASE` 能减少库存，不能减到负数；`ON_SHELF` 扣到 0 后自动 `OFF_SHELF` |
| AC5 | `MARK_SOLD` 能按售出扣减库存；扣到 0 后商品状态为 `SOLD` |
| AC6 | `LOCKED/SOLD/CLOSED` 商品不能调整库存 |
| AC7 | 商家不能调整其他商家的商品 |
| AC8 | 相同 `Idempotency-Key` 重试不会重复扣减库存 |
| AC9 | 小程序无需发版，仍可正确展示库存和商品状态 |
| AC10 | 后端测试、前端构建通过，测试环境部署后可通过商家后台验证入口 |

## 11. 自审结果

| 检查项 | 结果 |
|---|---|
| 完整性检查 | 未留下空白章节或未完成内容 |
| 范围检查 | 未包含 `reserved_stock`、订单数量、多活动订单、小程序发版 |
| 一致性检查 | API、状态规则、数据库流水、前端入口一致 |
| 歧义检查 | 明确了 `DECREASE` 与 `MARK_SOLD` 的区别：前者是非销售减少，后者是售出扣减 |
