# 设计规格

## 摘要

| 字段 | 内容 |
|---|---|
| 需求来源 | `requirements.md` |
| 交付目标 | 实现商品售罄可补货恢复，并移除商品关闭状态 |
| 主要验收标准 | SC-1 至 SC-7 |

完整业务设计以 `docs/superpowers/specs/2026-08-09-product-sold-out-state-design.md` 为准。

## 当前状态

| 区域 | 当前行为 / 证据 |
|---|---|
| 后端 | 商品六状态；`SOLD` 不可补货；上架只校验图片；商品 close 路由存在 |
| 商家端 | `SOLD=已成交`、`CLOSED=已关闭`；库存弹窗不支持 `SOLD` |
| 小程序 | 详情裸显状态码；详情和收藏页始终显示电话 CTA“我想要” |
| 数据 | `products.status` 为字符串；存在显式 SQL 迁移工具和迁移目录 |

## 目标行为

| ID | 行为 | 用户 / 系统影响 |
|---|---|---|
| TB-1 | 商品只保留五种状态 | 商品关闭入口和统计消失 |
| TB-2 | 快捷售罄锁内归零并记录实际流水数量 | 售罄状态与库存一致 |
| TB-3 | `SOLD + INCREASE -> OFF_SHELF` | 补货后可再次显式上架 |
| TB-4 | 上架要求图片、可销售库存和无活动订单 | 零库存商品不会进入在售 |
| TB-5 | 小程序非在售商品隐藏电话 CTA | 售罄详情可看但不可联系购买 |
| TB-6 | 0007 迁移归一化历史数据 | 新代码不遇到商品 `CLOSED` 或矛盾 `SOLD` |

## 接口

| 接口 | 变更 | 兼容性说明 |
|---|---|---|
| `POST /merchant/products/:id/stock-adjustments` | 增加可选 `all_remaining`; `MARK_SOLD + all_remaining=true` 锁内归零 | 原有 `quantity` 部分调整继续有效 |
| `POST /merchant/products/:id/close` | 删除 | 订单与意向 close 路由不变 |
| Dashboard `product_stats` | 删除 `closed` | `order_stats.closed` 保留 |
| 买家详情 | 状态仍返回 `SOLD`，`can_submit_intent=false` | 不新增状态码 |

## 数据契约

| 字段 / Payload / 格式 | 必需规则 | 兼容性 / 消费方说明 |
|---|---|---|
| `all_remaining` | 仅与 `MARK_SOLD` 组合；为 true 时 `quantity` 可省略 | 商家快捷售罄使用 |
| `quantity` | 普通调整必需且大于 0；快捷售罄响应返回后端实际值 | 原有消费者不变 |
| `SOLD` | `stock=0,reserved_stock=0,active_order_id=NULL` | 商家端与小程序显示“售罄” |

## 数据流

| 步骤 | 输入 | 处理 | 输出 |
|---|---|---|---|
| 1 | 商家点击设为售罄 | 行锁内检查状态、预占和活动订单，读取实际库存 | 库存归零、状态 `SOLD`、写流水 |
| 2 | 商家为 `SOLD` 补库存 | `INCREASE` 同事务更新库存 | 状态 `OFF_SHELF` |
| 3 | 商家重新上架 | 校验图片、可销售库存、活动订单 | 状态 `ON_SHELF` |

## 失败模式

| 场景 | 预期处理 | 需要的证据 |
|---|---|---|
| `DRAFT/LOCKED/SOLD` 快捷售罄 | `10005` | Go 测试 |
| `SOLD` 执行非 `INCREASE` | `10005` | Go 测试 |
| 0 库存上架或快捷售罄 | `10005` | Go 测试 |
| 无图片上架 | `10001` | 既有及新增 Go 测试 |
| 日志写入失败 | 不改变核心事务既有策略 | 代码审查 |

## 验收标准映射

| 成功标准 | 规格覆盖 | 验证方式 |
|---|---|---|
| SC-1 | TB-1 | 路由、状态、统计和前端测试 |
| SC-2 | TB-2 | 库存与订单 Go 测试 |
| SC-3 | TB-3/TB-4 | Go 与前端测试 |
| SC-4 | TB-2/TB-4 | Go 非法流转测试 |
| SC-5 | TB-1/TB-2/TB-3 | 前端 Vitest 与 build |
| SC-6 | TB-5 | 小程序 Vitest 与 build |
| SC-7 | TB-6 | 迁移目录测试和 SQL 审查 |
