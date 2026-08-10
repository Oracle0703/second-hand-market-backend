# 设计文档审核（代码核对版）：商品售罄与可恢复销售状态

| 项目 | 内容 |
|---|---|
| 日期 | 2026-08-09 |
| 审核对象 | `docs/superpowers/specs/2026-08-09-product-sold-out-state-design.md` |
| 分支 | `hotfix/hy/0000_product_sale_status` |
| 审核方式 | 逐条对照后端（Go/Gin）、商家前端（React）、买家小程序（Taro）工作区代码核实 |
| 结论 | 方案可进入实现；需先补齐 4 个业务决策、修正 2 处与现状不符的表述、纳入若干遗漏改动点 |

> 本文档与 `2026-08-09-product-sold-out-state-design-review.md` 互补：该评审侧重设计语义，本文侧重文档声明与实际代码的逐条核对。

## 1. 总体评价

文档质量较高：需求边界清晰、方案比较（A/B/C）选型合理、业务不变量（I1–I6）可验证、测试设计与验收标准完整、发布顺序考虑了数据迁移先于代码发布。文档对基线的声明（含本分支未提交的订单库存联动修复）经核实属实。核心设计可以成立。

## 2. 需要业务决策的漏洞（建议补充进文档）

### 2.1 0 库存商品永远无法进入 `SOLD`——语义闭环有缺口

- 文档 §5.1 `OFF_SHELF` 行写"库存大于 0 时设为售罄"，但 §8.1 矩阵对 `OFF_SHELF` 的 `MARK_SOLD` 是无条件"允许"，两处表述不一致。
- 实际约束在 DTO 层：`quantity` 校验 `gt=0`（`backend/internal/dto/dto.go:65`），`MARK_SOLD` 数量又不得超过库存。因此 `DECREASE` 到 0 的 `OFF_SHELF` 商品没有任何路径转为"售罄"——只能先补库存再设为售罄，很荒谬。
- 需要明确：这是有意为之（"0 库存的下架商品就不叫售罄"），还是允许 `stock=0` 时直接置 `SOLD`？建议文档显式说明。

### 2.2 `SOLD` 商品没有退出路径

§5.1/§9.2 中 `SOLD` 只允许"查看、补充库存"，不能编辑、不能删除。商家确认不再补货的商品将永远留在列表里（删除要求 `DRAFT/OFF_SHELF`，`backend/internal/app/product_handlers.go:399`）。现状如此、不是回退，但既然本次把 `SOLD` 从终态改为可恢复状态，建议顺带明确：是否允许 `SOLD` 删除或编辑？

### 2.3 小程序"我想要"的处理方式与覆盖范围

- §10 写"隐藏**或**禁用"，二选一未定。
- 文档只提详情页，但收藏页有同样问题：`miniapp/src/pages/favorite/index.tsx:45` 的"我想要"同样无条件可点。文档遗漏了这处入口。

### 2.4 历史 `product_close` 日志文案映射

`frontend/src/pages/merchant/logs/ListPage.tsx:58` 有 `product_close: '关闭商品'` 映射。后端移除 close 路由后历史日志仍存在，保留映射还是清理需决策。

## 3. 与现状不符的表述（建议修正措辞）

### 3.1 §8.3 事务声称过强

§8.3 称"库存、状态、库存流水和操作日志必须在同一事务中提交或回滚"，但操作日志实际是 best-effort：`server.go:335` 的 `writeOperationLog` Create 错误被 `_ =` 吞掉，不在事务内。§2 又说"只复用现有机制"，两处自相矛盾。建议改为"库存、状态、流水同事务；操作日志沿用现有 best-effort 机制"，否则实现者会误以为要改日志机制。

### 3.2 §10 低估了小程序侧工作量

"小程序详情页必须消费该字段，不再只显示原始状态码"的措辞暗示详情页已消费 `can_submit_intent` 只是展示不对；实际上：

- 详情页直接裸显 `product.status` 原始码（`miniapp/src/pages/product/detail/index.tsx:63`），"我想要"按钮无任何禁用条件（:107-109），`can_submit_intent` 从未被详情页消费。
- 唯一消费处 `intent/create/index.tsx:53,88` 所在页面未注册进 `app.config.ts`，线上不可达。
- 小程序目前没有任何商品状态文案映射工具（`utils/intent.ts` 只映射意向状态），"售罄"文案需要新建映射，不只是"改显示"。

后端 `can_submit_intent = status == ON_SHELF`（`backend/internal/app/buyer_handlers.go:374`）已符合要求，这部分没问题。

## 4. 遗漏的改动点（§7/§12 影响范围清单之外）

### 4.1 既有测试必须改写

| 文件 | 问题 |
|---|---|
| `backend/tests/restricted_and_security_test.go:561-569` | 断言 `POST /products/:id/close` 成功且 CLOSED 后不可上架——删路由后必然挂，B8 应点名此用例 |
| `backend/internal/stateflow/stateflow_test.go:16` | 断言 `SOLD→ON_SHELF` 不可——若按 §12 在 `stateflow.go` 加 `SOLD→OFF_SHELF` 边，需同步 |
| `frontend/src/pages/merchant/products/stock-adjustment.test.ts:8,23` | 把 `'CLOSED'` 放进 `ProductStatus[]`，类型删除后直接编译失败；且当前断言 `SOLD` 不能调库存，与 F3/F4 相反 |

### 4.2 前端具体位置

- `ListPage.tsx:284` 删除按钮条件含 `status === 'CLOSED'`，§7"商品删除规则"只提了后端。
- 前端 `MARK_SOLD` 现状叫"线下售出"（`frontend/src/pages/merchant/products/stock-adjustment.ts:3-9`），弹窗不按状态过滤调整类型（`StockAdjustmentModal.tsx:61`），`SOLD` 目前连库存调整入口都没有（`stock-adjustment.ts:11-13` 排除 SOLD）。§6.4"设为售罄"快捷操作与现有"线下售出"入口的关系（并存？文案区分？）建议写清楚。

### 4.3 数据修复 SQL 的执行形式

§11 的 `UPDATE products ... WHERE status='CLOSED'` 没有对应迁移文件；发布顺序第 1 步靠它，但文档没说以迁移脚本还是手工 SQL 形式执行，建议明确。

### 4.4 冒烟脚本

`scripts/smoke-flow.mjs:144,189` 现有断言（完成订单→SOLD、关闭订单→OFF_SHELF）与新设计一致可保留，但 B4/B5（设为售罄、SOLD 补库存）在 scripts/ 下无对应冒烟用例，如需要应新增。

### 4.5 后端列表 status 筛选无白名单

`product_handlers.go:345` 把 query 参数直接拼进 SQL：移除 CLOSED 后传 `status=CLOSED` 静默返回空，可接受但前端筛选项必须删干净（文档已覆盖前端筛选）。

## 5. 确认无误的关键声明（抽样核对结果）

| 文档声明 | 证据 | 结果 |
|---|---|---|
| 商品六状态、订单三状态、意向 `CLOSED` | `backend/internal/model/models.go:33-42,58-60` | 属实 |
| `SOLD` 当前是终态、三条 `→ CLOSED` 边存在 | `backend/internal/stateflow/stateflow.go:5-22` | 属实 |
| 订单预占/扣减/释放联动（本分支未提交修复） | `backend/internal/app/order_handlers.go:37-232` | 属实 |
| `MARK_SOLD` 部分扣减、归零才进 `SOLD` | `backend/internal/app/product_stock_adjustment_handlers.go:41-48` | 属实（与 §6.4 末段一致） |
| `status` 无枚举约束、`sold_at/closed_at` 已存在、无需改表结构 | `backend/migrations/0001_init.up.sql:88-94` | 属实 |
| 仪表盘商品统计含 `closed`、订单统计含 `closed` | `backend/internal/app/merchant_handlers.go:201`、`frontend/src/pages/merchant/dashboard/DashboardPage.tsx:23` | 属实 |
| 错误码 10005=非法状态流转、10001=参数非法 | `backend/internal/common/errors.go:7,11` | 属实 |
| 前端 `SOLD` 当前文案是"已成交"而非"售罄" | `frontend/src/constants/status.ts:17` | 属实（文案变更是真实改动） |
| §8.1 矩阵的两处行为变化 | 当前 `SOLD` 不允许 `INCREASE`、`DRAFT` 允许 `MARK_SOLD` 归零进 `SOLD`（`product_stock_adjustment_handlers.go:16-18`） | 文档已正确识别为待实现变更 |

## 6. 结论

方案本身（保留 `SOLD` 状态码、补库存转 `OFF_SHELF`、显式再上架）合理且改动面可控，可以进入实现，但建议先补三件事：

1. 决策并写明：0 库存商品能否设为售罄、`SOLD` 是否允许删除、"我想要"隐藏还是禁用、收藏页是否一并处理、历史 `product_close` 日志映射去留。
2. 修正 §8.3 事务表述与 §10 小程序现状描述。
3. 把第 4 节列出的具体文件/测试纳入 §7 和 §12 的影响范围，尤其是三个会因改动直接失败的既有测试。
