# 商品售罄与可恢复销售状态实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 移除商品 `CLOSED`，将 `SOLD` 实现为库存归零且可补库存恢复的“售罄”状态，并保持订单关闭语义独立。

**Architecture:** 通用 `productTransitions` 只保留商品 API 与创建订单校验需要的边；订单完成/关闭和库存调整在各自事务中执行领域迁移。商家端与小程序消费同一五状态契约，历史数据通过注册的 0007 前向迁移归一化。

**Tech Stack:** Go 1.22+、Gin、GORM、React 18、TypeScript、Vite/Vitest、Taro 3.6、MySQL SQL migration。

## Global Constraints

- 分支固定为 `hotfix/hy/0000_product_sale_status`，保留当前未提交的订单库存联动修复。
- 商品状态码继续使用 `SOLD`，展示文案统一为“售罄”；订单和意向 `CLOSED` 不变。
- 不新增 `SOLD_OUT`、多活动订单、支付、销售额、商品 `SOLD` 直接删除或额外安全治理。
- 库存、状态和库存流水同事务；操作日志继续使用现有 best-effort 机制。
- 2026-08-10 已获得本地提交授权；仍未获得推送或 PR 授权。

---

### Task 1: 后端商品状态与库存领域规则

**Files:**
- Modify: `backend/internal/stateflow/stateflow.go`
- Modify: `backend/internal/stateflow/stateflow_test.go`
- Modify: `backend/internal/dto/dto.go`
- Modify: `backend/internal/app/product_stock_adjustment_handlers.go`
- Modify: `backend/tests/product_stock_adjustment_test.go`

**Interfaces:**
- Consumes: 当前 `Product.Stock/ReservedStock/ActiveOrderID` 和 `ProductStockAdjustment` 流水模型。
- Produces: `AdjustProductStockRequest.AllRemaining bool`；`MARK_SOLD + all_remaining=true` 返回实际 `quantity`；`SOLD + INCREASE -> OFF_SHELF`。

- [x] **Step 1: 写失败测试**

覆盖以下字面行为：`DRAFT + MARK_SOLD` 返回 `10005`；部分 `MARK_SOLD` 保持原状态；快捷售罄实际归零；`SOLD` 只允许 `INCREASE` 并转 `OFF_SHELF`；`SOLD` 的其他调整返回 `10005`。

- [x] **Step 2: 运行 RED**

```powershell
go test -p 1 ./internal/stateflow ./tests -run 'TestProductTransition|TestProductStockAdjustment' -count=1
```

预期：新增售罄恢复和 `all_remaining` 断言因功能缺失而失败。

- [x] **Step 3: 最小实现**

```go
type AdjustProductStockRequest struct {
    AdjustmentType string `json:"adjustment_type" binding:"required,oneof=INCREASE DECREASE MARK_SOLD"`
    Quantity       int    `json:"quantity" binding:"omitempty,gt=0"`
    AllRemaining   bool   `json:"all_remaining"`
    Reason         string `json:"reason" binding:"required,min=2,max=255"`
}
```

库存计算必须返回后端实际应用数量；快捷售罄仅允许 `ON_SHELF/OFF_SHELF`、`stock>0`、`reserved_stock=0`、无活动订单。

- [x] **Step 4: 运行 GREEN**

重复 Step 2 命令，要求相关测试通过。

### Task 2: 后端商品入口、统计与历史迁移

**Files:**
- Modify: `backend/internal/model/models.go`
- Modify: `backend/internal/stateflow/stateflow.go`
- Modify: `backend/internal/stateflow/stateflow_test.go`
- Modify: `backend/internal/app/product_handlers.go`
- Modify: `backend/internal/app/server.go`
- Modify: `backend/internal/app/merchant_handlers.go`
- Modify: `backend/tests/restricted_and_security_test.go`
- Create: `backend/migrations/0007_product_sold_out_state.up.sql`
- Modify: `backend/scripts/migrate/main.go`
- Modify: `backend/scripts/migrate/main_test.go`

**Interfaces:**
- Consumes: Task 1 的五状态和库存恢复规则。
- Produces: 无商品 close 路由；上架完整校验；无商品 `closed` 统计；注册的 0007 迁移。

- [x] **Step 1: 写失败测试**

覆盖商品 close 路由不存在、订单 close 仍存在、零可售库存不能上架、`LOCKED/SOLD` 不能通过通用商品 API 离开、Dashboard 商品统计无 `closed`。

- [x] **Step 2: 运行 RED**

```powershell
go test -p 1 ./internal/app ./tests -run 'Test.*Product.*(Close|Shelf|Dashboard|Order)' -count=1
```

- [x] **Step 3: 最小实现**

删除 `ProductClosed` 常量、状态边、路由、handler 和统计项；把图片、`stock-reserved_stock>0`、`active_order_id=nil` 校验放入上架事务。0007 将 `CLOSED` 和 `SOLD AND stock>0` 转为 `OFF_SHELF` 并注册 SHA256。

- [x] **Step 4: 运行 GREEN 与迁移测试**

```powershell
go test -p 1 ./internal/app ./internal/stateflow ./tests ./scripts/migrate -run 'Test.*(Product|Dashboard|MigrationCatalog)' -count=1
```

### Task 3: 商家后台五状态与操作入口

**Files:**
- Modify: `frontend/src/constants/status.ts`
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/pages/merchant/products/stock-adjustment.ts`
- Modify: `frontend/src/pages/merchant/products/stock-adjustment.test.ts`
- Modify: `frontend/src/pages/merchant/products/components/StockAdjustmentModal.tsx`
- Modify: `frontend/src/pages/merchant/products/components/StockAdjustmentModal.test.tsx`
- Modify: `frontend/src/pages/merchant/products/ListPage.tsx`
- Modify: `frontend/src/pages/merchant/products/DetailPage.tsx`
- Modify: `frontend/src/pages/merchant/dashboard/DashboardPage.tsx`

**Interfaces:**
- Consumes: 后端 `all_remaining`、五商品状态和移除后的 Dashboard 契约。
- Produces: `getStockAdjustmentTypeOptions(status)`、售罄快捷操作和售罄补货 UI。

- [x] **Step 1: 写失败测试**

断言 `SOLD` 可打开库存弹窗且只有 `INCREASE`；`DRAFT` 不含 `MARK_SOLD`；`ON_SHELF/OFF_SHELF` 保留三类调整；快捷请求发送 `all_remaining=true`。

- [x] **Step 2: 运行 RED**

```powershell
cmd.exe /c npx vitest run src/pages/merchant/products/stock-adjustment.test.ts src/pages/merchant/products/components/StockAdjustmentModal.test.tsx
```

- [x] **Step 3: 最小实现**

移除商品 `CLOSED` 类型、筛选、关闭按钮、删除条件和 API；`SOLD` 文案改为“售罄”；弹窗按状态派生调整类型；列表和详情在 `ON_SHELF/OFF_SHELF && stock>0` 时展示“设为售罄”；Dashboard 删除商品 `closed`。

- [x] **Step 4: 运行 GREEN 与构建**

```powershell
cmd.exe /c npx vitest run src/pages/merchant/products/stock-adjustment.test.ts src/pages/merchant/products/components/StockAdjustmentModal.test.tsx
cmd.exe /c npm run build
```

### Task 4: 小程序售罄展示与不可购买入口

**Files:**
- Create: `miniapp/src/utils/product-status.ts`
- Create: `miniapp/tests/product-sale-status.test.ts`
- Modify: `miniapp/src/components/ProductCard.tsx`
- Modify: `miniapp/src/pages/product/detail/index.tsx`
- Modify: `miniapp/src/pages/favorite/index.tsx`

**Interfaces:**
- Consumes: 详情 `can_submit_intent` 和收藏项 `status`。
- Produces: `getProductStatusText(status)` 与 `canContactForProduct(status, canSubmitIntent?)`。

- [x] **Step 1: 写失败测试**

断言 `SOLD -> 售罄`；只有 `ON_SHELF` 可联系购买；后端明确返回 `can_submit_intent=false` 时不可联系。

- [x] **Step 2: 运行 RED**

```powershell
cmd.exe /c npx vitest run tests/product-sale-status.test.ts
```

- [x] **Step 3: 最小实现**

详情和通用卡片使用状态文案映射；详情按 `can_submit_intent` 隐藏“我想要”；收藏按 `status` 隐藏按钮并显示状态文案；不注册或恢复意向页面。

- [x] **Step 4: 运行 GREEN 与构建**

```powershell
cmd.exe /c npx vitest run tests/product-sale-status.test.ts tests/intent-hidden-and-contact-centralized.test.ts
cmd.exe /c npm run build:weapp
```

### Task 5: 集成审查与最终验证

**Files:**
- Modify: `docs/delivery/2026-08-09-product-sold-out-state/delivery-status.md`
- Modify: `docs/delivery/2026-08-09-product-sold-out-state/plan.md`
- Modify: `docs/delivery/2026-08-09-product-sold-out-state/test-review.md`
- Modify: `docs/delivery/2026-08-09-product-sold-out-state/delivery-report.md`

- [x] **Step 1: 规格符合性审查**

逐条映射 SC-1 至 SC-7，确认没有新增 `SOLD_OUT`、商品直接删除或隐藏意向页恢复。

- [x] **Step 2: 代码质量审查**

检查状态边是否可被通用路由绕过、库存流水数量是否为实际值、前后端状态契约是否一致。

- [x] **Step 3: 最终验证**

```powershell
go test -p 1 ./internal/app ./internal/stateflow ./tests ./scripts/migrate -count=1
cmd.exe /c npm test
cmd.exe /c npm run build
cmd.exe /c npm test
cmd.exe /c npm run build:weapp
git diff --check
```

Go 命令在 `backend`，前两个 Node 命令在 `frontend`，后两个 Node 命令在 `miniapp` 运行。Windows Go 首次锁文件失败时使用仓库内独立 `GOCACHE/GOTMPDIR`、`-p 1 -count=1` 重跑相同范围。

验证结果：后端商品/订单/统计/迁移精确四包通过；未过滤的四包命令仅被既有 Linux-only 小程序验收合同阻塞。前端 10 文件 29/29、小程序 16 文件 157/157 通过；两个生产构建退出码均为 0。最终差异检查和环境偏差见交付报告。

- [x] **Step 4: 更新交付证据**

把每条命令的真实退出码、通过数、环境阻塞和残余限制写入 `test-review.md` 与 `delivery-report.md`。本地提交由用户在交付后单独授权，推送与 PR 仍不执行。
