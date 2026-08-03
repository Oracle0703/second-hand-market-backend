# 手动调整库存入口 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在商家后台商品列表和商品详情新增“调整库存”入口，并通过后端独立库存调整接口记录可审计流水。

**Architecture:** 后端新增 `product_stock_adjustments` 流水表和 `POST /api/v1/merchant/products/:id/stock-adjustments` 接口，在事务内锁定商品、计算库存与状态、写商品和流水。前端新增可复用 `StockAdjustmentModal`，商品列表和详情共享该弹窗；小程序不改代码，继续消费现有 `stock/status`。

**Tech Stack:** Go 1.22、Gin、GORM、MySQL/SQLite 测试库、React 18、TypeScript、Vite、Ant Design、TanStack Query、Vitest。

## Global Constraints

| 约束 | 内容 |
|---|---|
| 分支 | 当前分支必须保持 `feature/hy/10005_inventory_adjustment` |
| 生产环境 | 不部署、不修改正式环境 |
| 小程序 | 不要求小程序发版，不新增小程序登录依赖 |
| 订单模型 | 不引入 `reserved_stock`，不改订单数量，不支持同商品多个活动订单 |
| 权限 | 只允许 `MERCHANT full` 调整当前商家自己的商品 |
| 状态范围 | 只允许 `DRAFT`、`ON_SHELF`、`OFF_SHELF` 调整库存 |
| 幂等 | 写接口支持 `Idempotency-Key`，相同请求不能重复扣减 |
| TDD | 任何生产代码前先写失败测试，并确认失败原因是功能缺失 |
| 文档 | 修改后同步 `docs/backend-api-checklist.md`、`docs/frontend-pages.md`、`docs/data-model.md` |
| 执行方式 | 当前会话禁止主动派生子代理；实现时使用 `superpowers:executing-plans` 内联执行 |

---

## File Structure

| 文件 | 操作 | 职责 |
|---|---|---|
| `backend/internal/model/models.go` | 修改 | 新增库存调整类型常量和 `ProductStockAdjustment` 模型 |
| `backend/internal/dto/dto.go` | 修改 | 新增 `AdjustProductStockRequest` 请求 DTO |
| `backend/internal/app/database_operations.go` | 修改 | 将库存调整流水表加入 `MigrateSchema` |
| `backend/migrations/0006_product_stock_adjustments.up.sql` | 新增 | MySQL 流水表迁移 |
| `backend/internal/app/product_stock_adjustment_handlers.go` | 新增 | 库存调整接口、状态计算、事务、流水、操作日志 |
| `backend/internal/app/server.go` | 修改 | 注册 `POST /merchant/products/:id/stock-adjustments` 路由 |
| `backend/tests/product_stock_adjustment_test.go` | 新增 | 后端集成测试覆盖增加、减少、售出、非法状态、跨商家、幂等 |
| `backend/internal/app/database_operations_test.go` | 修改 | 验证 AutoMigrate 创建库存调整表 |
| `frontend/src/services/api.ts` | 修改 | 新增 `adjustProductStock` API 方法和请求/响应类型 |
| `frontend/src/pages/merchant/products/stock-adjustment.ts` | 新增 | 前端可测试的库存调整常量、状态判断、默认原因 |
| `frontend/src/pages/merchant/products/stock-adjustment.test.ts` | 新增 | 前端纯逻辑测试 |
| `frontend/src/pages/merchant/products/components/StockAdjustmentModal.tsx` | 新增 | 商品列表/详情复用的库存调整弹窗 |
| `frontend/src/pages/merchant/products/ListPage.tsx` | 修改 | 操作列新增“调整库存”按钮并刷新列表 |
| `frontend/src/pages/merchant/products/DetailPage.tsx` | 修改 | 顶部和卡片操作区新增“调整库存”按钮并刷新详情/列表缓存 |
| `docs/backend-api-checklist.md` | 修改 | 新增库存调整接口，修正库存固定 1 的旧说明 |
| `docs/frontend-pages.md` | 修改 | 新增商品列表/详情调整库存入口说明 |
| `docs/data-model.md` | 修改 | 新增库存调整流水表，修正 `products.stock` 描述 |

## Task 1: 后端库存调整数据结构与迁移

**Files:**

- Modify: `backend/internal/model/models.go`
- Modify: `backend/internal/app/database_operations.go`
- Modify: `backend/internal/app/database_operations_test.go`
- Create: `backend/migrations/0006_product_stock_adjustments.up.sql`

**Interfaces:**

- Produces constants:
  - `model.StockAdjustmentIncrease = "INCREASE"`
  - `model.StockAdjustmentDecrease = "DECREASE"`
  - `model.StockAdjustmentMarkSold = "MARK_SOLD"`
- Produces model:
  - `model.ProductStockAdjustment`
  - table name `product_stock_adjustments`

- [ ] **Step 1: Write the failing migration test**

Add this assertion block to `TestMigrateSchemaCreatesApplicationTables` in `backend/internal/app/database_operations_test.go`:

```go
if !db.Migrator().HasTable(&model.ProductStockAdjustment{}) {
	t.Fatalf("expected product_stock_adjustments table to be created")
}
```

- [ ] **Step 2: Run test and verify it fails**

Run from `backend/`:

```powershell
go test ./internal/app -run TestMigrateSchemaCreatesApplicationTables -v
```

Expected: FAIL because `ProductStockAdjustment` does not exist or its table is not migrated.

- [ ] **Step 3: Add model constants and struct**

Add to `backend/internal/model/models.go`:

```go
const (
	StockAdjustmentIncrease = "INCREASE"
	StockAdjustmentDecrease = "DECREASE"
	StockAdjustmentMarkSold = "MARK_SOLD"
)

type ProductStockAdjustment struct {
	ID             uint64 `gorm:"primaryKey"`
	ProductID      uint64 `gorm:"index:idx_product_stock_adjustment_created,priority:1"`
	MerchantID     uint64 `gorm:"index:idx_merchant_stock_adjustment_created,priority:1"`
	AdjustmentType string `gorm:"size:32"`
	Quantity       int
	StockBefore    int
	StockAfter     int
	StatusBefore    string `gorm:"size:16"`
	StatusAfter     string `gorm:"size:16"`
	Reason         string `gorm:"size:255"`
	OperatorID     uint64
	CreatedAt      time.Time `gorm:"index:idx_product_stock_adjustment_created,priority:2;index:idx_merchant_stock_adjustment_created,priority:2"`
}
```

No custom `TableName` is required because GORM maps the struct to `product_stock_adjustments`.

- [ ] **Step 4: Add AutoMigrate target**

Add `&model.ProductStockAdjustment{},` to `MigrateSchema` immediately after `&model.ProductImage{},`:

```go
&model.Product{},
&model.ProductImage{},
&model.ProductStockAdjustment{},
&model.Order{},
```

- [ ] **Step 5: Add MySQL migration**

Create `backend/migrations/0006_product_stock_adjustments.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS product_stock_adjustments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  product_id BIGINT NOT NULL,
  merchant_id BIGINT NOT NULL,
  adjustment_type VARCHAR(32) NOT NULL,
  quantity INT NOT NULL,
  stock_before INT NOT NULL,
  stock_after INT NOT NULL,
  status_before VARCHAR(16) NOT NULL,
  status_after VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NOT NULL,
  operator_id BIGINT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_product_stock_adjustment_created (product_id, created_at),
  INDEX idx_merchant_stock_adjustment_created (merchant_id, created_at)
);
```

- [ ] **Step 6: Run test and verify it passes**

Run from `backend/`:

```powershell
go test ./internal/app -run TestMigrateSchemaCreatesApplicationTables -v
```

Expected: PASS.

- [ ] **Step 7: Commit Task 1**

```powershell
git add backend/internal/model/models.go backend/internal/app/database_operations.go backend/internal/app/database_operations_test.go backend/migrations/0006_product_stock_adjustments.up.sql
git commit -m "feat(inventory): add stock adjustment model"
```

## Task 2: 后端库存调整 API

**Files:**

- Modify: `backend/internal/dto/dto.go`
- Create: `backend/internal/app/product_stock_adjustment_handlers.go`
- Modify: `backend/internal/app/server.go`
- Create: `backend/tests/product_stock_adjustment_test.go`

**Interfaces:**

- Consumes:
  - `model.ProductStockAdjustment`
  - `model.StockAdjustmentIncrease`
  - `model.StockAdjustmentDecrease`
  - `model.StockAdjustmentMarkSold`
- Produces route:
  - `POST /api/v1/merchant/products/:id/stock-adjustments`
- Produces handler:
  - `func (s *Server) handleProductStockAdjustment(c *gin.Context)`
- Produces DTO:
  - `dto.AdjustProductStockRequest`

- [ ] **Step 1: Write failing integration test for increase and decrease**

Create `backend/tests/product_stock_adjustment_test.go` with:

```go
package tests

import (
	"fmt"
	"net/http"
	"testing"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/model"
)

func approvedMerchantToken(t *testing.T, srv *app.Server, prefix string) string {
	t.Helper()
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, prefix)
	approveMerchant(t, srv, adminToken, merchantID)
	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	return str(login.Data["access_token"])
}

func TestProductStockAdjustmentIncreaseAndDecrease(t *testing.T) {
	srv := newTestServer(t)
	token := approvedMerchantToken(t, srv, "stock_adjust")
	productID := createAndOnShelfProduct(t, srv, token)

	increase := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID),
		map[string]interface{}{"adjustment_type": "INCREASE", "quantity": 4, "reason": "盘点补录"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if increase.Code != 0 {
		t.Fatalf("increase stock failed: %+v", increase)
	}
	if numToUint64(increase.Data["stock_before"]) != 1 || numToUint64(increase.Data["stock_after"]) != 5 {
		t.Fatalf("increase stock response mismatch: %+v", increase)
	}

	decrease := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID),
		map[string]interface{}{"adjustment_type": "DECREASE", "quantity": 5, "reason": "盘点减少"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if decrease.Code != 0 {
		t.Fatalf("decrease stock failed: %+v", decrease)
	}
	if numToUint64(decrease.Data["stock_after"]) != 0 || str(decrease.Data["status_after"]) != model.ProductOffShelf {
		t.Fatalf("decrease to zero should off-shelf product: %+v", decrease)
	}

	var movements []model.ProductStockAdjustment
	if err := srv.DB.Where("product_id = ?", productID).Order("id ASC").Find(&movements).Error; err != nil {
		t.Fatalf("query stock movements failed: %v", err)
	}
	if len(movements) != 2 {
		t.Fatalf("expected 2 stock movements, got %d", len(movements))
	}
}
```

- [ ] **Step 2: Run test and verify it fails**

Run from `backend/`:

```powershell
go test ./tests -run TestProductStockAdjustmentIncreaseAndDecrease -v
```

Expected: FAIL with route not found or missing handler/model compile errors before implementation.

- [ ] **Step 3: Add request DTO**

Add to `backend/internal/dto/dto.go`:

```go
type AdjustProductStockRequest struct {
	AdjustmentType string `json:"adjustment_type" binding:"required,oneof=INCREASE DECREASE MARK_SOLD"`
	Quantity       int    `json:"quantity" binding:"required,gt=0"`
	Reason         string `json:"reason" binding:"required,min=2,max=255"`
}
```

- [ ] **Step 4: Implement handler and helper functions**

Create `backend/internal/app/product_stock_adjustment_handlers.go`:

```go
package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
)

func canAdjustProductStock(status string) bool {
	return status == model.ProductDraft || status == model.ProductOnShelf || status == model.ProductOffShelf
}

func calculateStockAdjustment(product model.Product, adjustmentType string, quantity int) (int, string, error) {
	if !canAdjustProductStock(product.Status) || product.ActiveOrderID != nil {
		return 0, "", common.ErrInvalidTransition
	}
	if quantity <= 0 {
		return 0, "", common.ErrInvalidArgument
	}

	stockAfter := product.Stock
	statusAfter := product.Status
	switch adjustmentType {
	case model.StockAdjustmentIncrease:
		stockAfter = product.Stock + quantity
	case model.StockAdjustmentDecrease:
		if quantity > product.Stock {
			return 0, "", common.ErrInvalidTransition
		}
		stockAfter = product.Stock - quantity
		if product.Status == model.ProductOnShelf && stockAfter == 0 {
			statusAfter = model.ProductOffShelf
		}
	case model.StockAdjustmentMarkSold:
		if quantity > product.Stock {
			return 0, "", common.ErrInvalidTransition
		}
		stockAfter = product.Stock - quantity
		if stockAfter == 0 {
			statusAfter = model.ProductSold
		}
	default:
		return 0, "", common.ErrInvalidArgument
	}
	return stockAfter, statusAfter, nil
}

func (s *Server) loadOwnedProductForUpdate(tx *gorm.DB, productID uint64, merchantID uint64) (model.Product, error) {
	var product model.Product
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", productID).
		First(&product).Error
	if err != nil {
		return model.Product{}, s.dbError(err)
	}
	if product.MerchantID != merchantID {
		return model.Product{}, common.ErrForbidden
	}
	return product, nil
}

func (s *Server) handleProductStockAdjustment(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.AdjustProductStockRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) < 2 || len(req.Reason) > 255 {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}

	payload := gin.H{"id": id, "adjustment_type": req.AdjustmentType, "quantity": req.Quantity, "reason": req.Reason}
	data, err := s.runWithIdempotency(c, payload, func() (map[string]interface{}, error) {
		resp := map[string]interface{}{}
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			product, err := s.loadOwnedProductForUpdate(tx, id, actor.MerchantID)
			if err != nil {
				return err
			}
			stockBefore := product.Stock
			statusBefore := product.Status
			stockAfter, statusAfter, err := calculateStockAdjustment(product, req.AdjustmentType, req.Quantity)
			if err != nil {
				return err
			}

			now := time.Now()
			product.Stock = stockAfter
			product.Status = statusAfter
			product.UpdatedBy = actor.UserID
			product.Version++
			if statusAfter == model.ProductOffShelf && statusBefore != model.ProductOffShelf {
				product.OffShelfAt = &now
			}
			if statusAfter == model.ProductSold && statusBefore != model.ProductSold {
				product.SoldAt = &now
				product.ActiveOrderID = nil
			}
			if err := tx.Save(&product).Error; err != nil {
				return err
			}

			movement := model.ProductStockAdjustment{
				ProductID:      product.ID,
				MerchantID:     actor.MerchantID,
				AdjustmentType: req.AdjustmentType,
				Quantity:       req.Quantity,
				StockBefore:    stockBefore,
				StockAfter:     stockAfter,
				StatusBefore:    statusBefore,
				StatusAfter:     statusAfter,
				Reason:         req.Reason,
				OperatorID:     actor.UserID,
				CreatedAt:      now,
			}
			if err := tx.Create(&movement).Error; err != nil {
				return err
			}

			from, to := statusBefore, statusAfter
			s.writeOperationLog(c, tx, "product", product.ID, "product_stock_adjust", &from, &to, common.CodeOK, &actor.MerchantID, gin.H{
				"adjustment_type": req.AdjustmentType,
				"quantity":        req.Quantity,
				"stock_before":    stockBefore,
				"stock_after":     stockAfter,
				"reason":          req.Reason,
				"movement_id":     movement.ID,
			})

			resp["product_id"] = product.ID
			resp["movement_id"] = movement.ID
			resp["adjustment_type"] = req.AdjustmentType
			resp["quantity"] = req.Quantity
			resp["stock_before"] = stockBefore
			resp["stock_after"] = stockAfter
			resp["status_before"] = statusBefore
			resp["status_after"] = statusAfter
			resp["adjusted_at"] = now.Format(time.RFC3339)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return resp, nil
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, data)
}
```

- [ ] **Step 5: Register route**

Add to merchant product routes in `backend/internal/app/server.go`:

```go
merchant.POST("/products/:id/stock-adjustments", middleware.RequireFullMerchantScope(), s.handleProductStockAdjustment)
```

Place it near the existing product state transition routes.

- [ ] **Step 6: Run increase/decrease test and verify it passes**

Run from `backend/`:

```powershell
go test ./tests -run TestProductStockAdjustmentIncreaseAndDecrease -v
```

Expected: PASS.

- [ ] **Step 7: Add failing tests for sold, invalid state, cross-merchant, idempotency**

Append these tests to `backend/tests/product_stock_adjustment_test.go`:

```go
func TestProductStockAdjustmentMarkSold(t *testing.T) {
	srv := newTestServer(t)
	token := approvedMerchantToken(t, srv, "stock_sold")
	productID := createAndOnShelfProduct(t, srv, token)

	sold := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID),
		map[string]interface{}{"adjustment_type": "MARK_SOLD", "quantity": 1, "reason": "客户线下购买"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if sold.Code != 0 {
		t.Fatalf("mark sold failed: %+v", sold)
	}
	if numToUint64(sold.Data["stock_after"]) != 0 || str(sold.Data["status_after"]) != model.ProductSold {
		t.Fatalf("mark sold response mismatch: %+v", sold)
	}

	var product model.Product
	if err := srv.DB.Where("id = ?", productID).First(&product).Error; err != nil {
		t.Fatalf("load product failed: %v", err)
	}
	if product.Status != model.ProductSold || product.Stock != 0 || product.SoldAt == nil {
		t.Fatalf("product should be sold with zero stock: status=%s stock=%d sold_at=%v", product.Status, product.Stock, product.SoldAt)
	}
}

func TestProductStockAdjustmentRejectsInvalidStatesAndInsufficientStock(t *testing.T) {
	srv := newTestServer(t)
	token := approvedMerchantToken(t, srv, "stock_invalid")
	productID := createAndOnShelfProduct(t, srv, token)

	tooMany := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID),
		map[string]interface{}{"adjustment_type": "DECREASE", "quantity": 2, "reason": "超过库存"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if tooMany.Code != 10005 {
		t.Fatalf("decrease below zero should be rejected: %+v", tooMany)
	}

	createOrder := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/orders", map[string]interface{}{"product_id": productID, "deal_price_cent": 9900}, map[string]string{"Authorization": "Bearer " + token})
	if createOrder.Code != 0 {
		t.Fatalf("create order failed: %+v", createOrder)
	}

	locked := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID),
		map[string]interface{}{"adjustment_type": "INCREASE", "quantity": 1, "reason": "锁定商品"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if locked.Code != 10005 {
		t.Fatalf("locked product adjustment should be rejected: %+v", locked)
	}
}

func TestProductStockAdjustmentScopeAndIdempotency(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	m1ID, u1, p1 := registerMerchant(t, srv, "stock_m1")
	m2ID, u2, p2 := registerMerchant(t, srv, "stock_m2")
	approveMerchant(t, srv, adminToken, m1ID)
	approveMerchant(t, srv, adminToken, m2ID)
	m1Login := merchantLogin(t, srv, u1, p1)
	m2Login := merchantLogin(t, srv, u2, p2)
	m1Token := str(m1Login.Data["access_token"])
	m2Token := str(m2Login.Data["access_token"])
	productID := createAndOnShelfProduct(t, srv, m1Token)

	crossMerchant := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID),
		map[string]interface{}{"adjustment_type": "INCREASE", "quantity": 1, "reason": "越权调整"},
		map[string]string{"Authorization": "Bearer " + m2Token},
	)
	if crossMerchant.Code != 10003 {
		t.Fatalf("cross merchant adjustment should be forbidden: %+v", crossMerchant)
	}

	headers := map[string]string{"Authorization": "Bearer " + m1Token, "Idempotency-Key": "stock-adjust-once"}
	body := map[string]interface{}{"adjustment_type": "INCREASE", "quantity": 2, "reason": "幂等补录"}
	first := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID), body, headers)
	second := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID), body, headers)
	if first.Code != 0 || second.Code != 0 {
		t.Fatalf("idempotent requests should both succeed: first=%+v second=%+v", first, second)
	}
	if v, ok := second.Data["idempotent"].(bool); !ok || !v {
		t.Fatalf("second response should be idempotent: %+v", second)
	}

	var product model.Product
	if err := srv.DB.Where("id = ?", productID).First(&product).Error; err != nil {
		t.Fatalf("load product failed: %v", err)
	}
	if product.Stock != 3 {
		t.Fatalf("stock should only increase once, got %d", product.Stock)
	}
}
```

- [ ] **Step 8: Run new tests and verify they fail before any additional production edits**

Run from `backend/`:

```powershell
go test ./tests -run TestProductStockAdjustment -v
```

Expected: the first test passes and at least one newly added test fails until missing edge behavior is implemented. If all pass immediately, record that the earlier implementation already satisfied the new tests and do not change code unnecessarily.

- [ ] **Step 9: Complete minimal backend behavior**

If Step 8 reveals failures, adjust only `product_stock_adjustment_handlers.go` so:

```go
// MARK_SOLD with stock_after == 0 sets SOLD and sold_at.
// LOCKED/SOLD/CLOSED or active_order_id != nil returns ErrInvalidTransition.
// Cross-merchant returns ErrForbidden before any stock mutation.
// Idempotency retry returns the cached result and does not create a second ProductStockAdjustment.
```

- [ ] **Step 10: Run full backend stock adjustment tests**

Run from `backend/`:

```powershell
go test ./tests -run TestProductStockAdjustment -v
```

Expected: PASS.

- [ ] **Step 11: Commit Task 2**

```powershell
git add backend/internal/dto/dto.go backend/internal/app/product_stock_adjustment_handlers.go backend/internal/app/server.go backend/tests/product_stock_adjustment_test.go
git commit -m "feat(inventory): add stock adjustment api"
```

## Task 3: 前端 API 与可测试库存调整逻辑

**Files:**

- Modify: `frontend/src/services/api.ts`
- Create: `frontend/src/pages/merchant/products/stock-adjustment.ts`
- Create: `frontend/src/pages/merchant/products/stock-adjustment.test.ts`

**Interfaces:**

- Produces API:
  - `api.adjustProductStock(productId, payload)`
- Produces helper:
  - `canAdjustProductStock(status: ProductStatus): boolean`
  - `STOCK_ADJUSTMENT_TYPE_OPTIONS`

- [ ] **Step 1: Write failing front-end logic test**

Create `frontend/src/pages/merchant/products/stock-adjustment.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { ProductStatus } from '@/constants/status'
import { canAdjustProductStock, STOCK_ADJUSTMENT_TYPE_OPTIONS } from './stock-adjustment'

describe('stock adjustment helpers', () => {
  it('allows only draft, on-shelf, and off-shelf products to be adjusted', () => {
    const allowed: ProductStatus[] = ['DRAFT', 'ON_SHELF', 'OFF_SHELF']
    const denied: ProductStatus[] = ['LOCKED', 'SOLD', 'CLOSED']

    for (const status of allowed) {
      expect(canAdjustProductStock(status)).toBe(true)
    }
    for (const status of denied) {
      expect(canAdjustProductStock(status)).toBe(false)
    }
  })

  it('exposes the three supported stock adjustment types', () => {
    expect(STOCK_ADJUSTMENT_TYPE_OPTIONS.map((item) => item.value)).toEqual(['INCREASE', 'DECREASE', 'MARK_SOLD'])
  })
})
```

- [ ] **Step 2: Run test and verify it fails**

Run from `frontend/`:

```powershell
npm test -- src/pages/merchant/products/stock-adjustment.test.ts
```

Expected: FAIL because `stock-adjustment.ts` does not exist.

- [ ] **Step 3: Add helper module**

Create `frontend/src/pages/merchant/products/stock-adjustment.ts`:

```ts
import type { ProductStatus } from '@/constants/status'

export type StockAdjustmentType = 'INCREASE' | 'DECREASE' | 'MARK_SOLD'

export const STOCK_ADJUSTMENT_TYPE_OPTIONS: Array<{ label: string; value: StockAdjustmentType }> = [
  { label: '补充库存', value: 'INCREASE' },
  { label: '减少库存', value: 'DECREASE' },
  { label: '线下售出', value: 'MARK_SOLD' }
]

export function canAdjustProductStock(status: ProductStatus) {
  return status === 'DRAFT' || status === 'ON_SHELF' || status === 'OFF_SHELF'
}
```

- [ ] **Step 4: Add API types and method**

Modify `frontend/src/services/api.ts`:

```ts
export type AdjustProductStockPayload = {
  adjustment_type: 'INCREASE' | 'DECREASE' | 'MARK_SOLD'
  quantity: number
  reason: string
}

export type AdjustProductStockResponse = {
  product_id: number
  movement_id: number
  adjustment_type: AdjustProductStockPayload['adjustment_type']
  quantity: number
  stock_before: number
  stock_after: number
  status_before: string
  status_after: string
  adjusted_at: string
  idempotent?: boolean
}
```

Add inside `api`:

```ts
adjustProductStock(productId: string | number, payload: AdjustProductStockPayload) {
  return http.post<APIResponse<AdjustProductStockResponse>>(`/merchant/products/${productId}/stock-adjustments`, payload)
}
```

- [ ] **Step 5: Run test and typecheck through build**

Run from `frontend/`:

```powershell
npm test -- src/pages/merchant/products/stock-adjustment.test.ts
npm run build
```

Expected: both PASS.

- [ ] **Step 6: Commit Task 3**

```powershell
git add frontend/src/services/api.ts frontend/src/pages/merchant/products/stock-adjustment.ts frontend/src/pages/merchant/products/stock-adjustment.test.ts
git commit -m "feat(inventory): add stock adjustment frontend api"
```

## Task 4: 前端调整库存弹窗

**Files:**

- Create: `frontend/src/pages/merchant/products/components/StockAdjustmentModal.tsx`
- Create or Modify: `frontend/vitest.config.ts` only if the existing config does not run React component tests in `jsdom`
- Create: `frontend/src/pages/merchant/products/components/StockAdjustmentModal.test.tsx`

**Interfaces:**

- Consumes:
  - `api.adjustProductStock`
  - `STOCK_ADJUSTMENT_TYPE_OPTIONS`
  - `canAdjustProductStock`
- Produces component:
  - `StockAdjustmentModal`

- [ ] **Step 1: Write failing component test**

Create `frontend/src/pages/merchant/products/components/StockAdjustmentModal.test.tsx`:

```tsx
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StockAdjustmentModal } from './StockAdjustmentModal'

vi.mock('@/services/api', () => ({
  api: {
    adjustProductStock: vi.fn(async () => ({
      data: {
        data: {
          product_id: 1,
          movement_id: 9,
          adjustment_type: 'INCREASE',
          quantity: 2,
          stock_before: 1,
          stock_after: 3,
          status_before: 'ON_SHELF',
          status_after: 'ON_SHELF',
          adjusted_at: '2026-08-03T18:30:00+08:00'
        }
      }
    }))
  }
}))

describe('StockAdjustmentModal', () => {
  it('shows current stock and submits an increase adjustment', async () => {
    const onSuccess = vi.fn()
    render(
      <StockAdjustmentModal
        open
        product={{ id: 1, title: '测试商品', status: 'ON_SHELF', stock: 1 }}
        onCancel={() => undefined}
        onSuccess={onSuccess}
      />
    )

    expect(screen.getByText('当前库存：1')).toBeInTheDocument()
    fireEvent.mouseDown(screen.getByLabelText('调整类型'))
    fireEvent.click(screen.getByText('补充库存'))
    fireEvent.change(screen.getByLabelText('调整数量'), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText('调整原因'), { target: { value: '盘点补录' } })
    fireEvent.click(screen.getByRole('button', { name: '确认调整' }))

    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
  })
})
```

If this test fails because `toBeInTheDocument` is not registered, add `import '@testing-library/jest-dom/vitest'` at the top of the test file.

- [ ] **Step 2: Run test and verify it fails**

Run from `frontend/`:

```powershell
npm test -- src/pages/merchant/products/components/StockAdjustmentModal.test.tsx
```

Expected: FAIL because `StockAdjustmentModal` does not exist.

- [ ] **Step 3: Implement modal**

Create `frontend/src/pages/merchant/products/components/StockAdjustmentModal.tsx`:

```tsx
import { Alert, Form, Input, InputNumber, Modal, Select, message } from 'antd'
import type { ProductStatus } from '@/constants/status'
import { api, type AdjustProductStockPayload } from '@/services/api'
import { STOCK_ADJUSTMENT_TYPE_OPTIONS, type StockAdjustmentType } from '../stock-adjustment'

export type StockAdjustmentProduct = {
  id: number
  title: string
  status: ProductStatus
  stock: number
}

type StockAdjustmentFormValues = {
  adjustment_type: StockAdjustmentType
  quantity: number
  reason: string
}

type StockAdjustmentModalProps = {
  open: boolean
  product: StockAdjustmentProduct | null
  onCancel: () => void
  onSuccess: () => void | Promise<void>
}

export function StockAdjustmentModal({ open, product, onCancel, onSuccess }: StockAdjustmentModalProps) {
  const [form] = Form.useForm<StockAdjustmentFormValues>()
  const currentStock = Number(product?.stock ?? 0)

  const submit = async () => {
    if (!product) return
    const values = await form.validateFields()
    const payload: AdjustProductStockPayload = {
      adjustment_type: values.adjustment_type,
      quantity: Math.floor(Number(values.quantity)),
      reason: values.reason.trim()
    }
    await api.adjustProductStock(product.id, payload)
    message.success('库存调整成功')
    form.resetFields()
    await onSuccess()
  }

  return (
    <Modal
      title={product ? `调整库存 - ${product.title}` : '调整库存'}
      open={open}
      okText="确认调整"
      cancelText="取消"
      onCancel={onCancel}
      onOk={() => void submit()}
      destroyOnClose
    >
      <Alert type="info" showIcon message={`当前库存：${currentStock}`} style={{ marginBottom: 16 }} />
      <Form<StockAdjustmentFormValues>
        form={form}
        layout="vertical"
        initialValues={{ adjustment_type: 'INCREASE', quantity: 1, reason: '' }}
      >
        <Form.Item name="adjustment_type" label="调整类型" rules={[{ required: true, message: '请选择调整类型' }]}>
          <Select options={STOCK_ADJUSTMENT_TYPE_OPTIONS} />
        </Form.Item>
        <Form.Item
          name="quantity"
          label="调整数量"
          rules={[
            { required: true, message: '请输入调整数量' },
            {
              validator: async (_, value) => {
                const quantity = Math.floor(Number(value))
                const type = form.getFieldValue('adjustment_type')
                if (!Number.isFinite(quantity) || quantity <= 0) throw new Error('调整数量必须大于 0')
                if ((type === 'DECREASE' || type === 'MARK_SOLD') && quantity > currentStock) throw new Error('调整数量不能超过当前库存')
              }
            }
          ]}
        >
          <InputNumber min={1} precision={0} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item
          name="reason"
          label="调整原因"
          rules={[
            { required: true, message: '请输入调整原因' },
            {
              validator: async (_, value) => {
                const reason = String(value ?? '').trim()
                if (reason.length < 2 || reason.length > 255) throw new Error('调整原因需为 2 到 255 个字符')
              }
            }
          ]}
        >
          <Input.TextArea rows={3} maxLength={255} showCount placeholder="例如：盘点补录、盘点减少、客户线下购买" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
```

- [ ] **Step 4: Run component test and fix only test-revealed issues**

Run from `frontend/`:

```powershell
npm test -- src/pages/merchant/products/components/StockAdjustmentModal.test.tsx
```

Expected: PASS after the component exists. If jsdom APIs such as `matchMedia` are missing, define the minimal test shim in the test file rather than adding unrelated app code.

- [ ] **Step 5: Commit Task 4**

```powershell
git add frontend/src/pages/merchant/products/components/StockAdjustmentModal.tsx frontend/src/pages/merchant/products/components/StockAdjustmentModal.test.tsx frontend/vitest.config.ts
git commit -m "feat(inventory): add stock adjustment modal"
```

If `frontend/vitest.config.ts` was not changed, omit it from `git add`.

## Task 5: 商品列表和详情接入调整库存入口

**Files:**

- Modify: `frontend/src/pages/merchant/products/ListPage.tsx`
- Modify: `frontend/src/pages/merchant/products/DetailPage.tsx`

**Interfaces:**

- Consumes:
  - `StockAdjustmentModal`
  - `canAdjustProductStock`
- Produces:
  - 列表操作列“调整库存”按钮
  - 详情页操作区“调整库存”按钮

- [ ] **Step 1: Write failing integration-level UI tests**

Extend `frontend/src/pages/merchant/products/stock-adjustment.test.ts` with a pure behavior assertion that locks in the visible entry rule:

```ts
it('keeps locked, sold, and closed products out of the stock adjustment entry', () => {
  expect(['LOCKED', 'SOLD', 'CLOSED'].every((status) => !canAdjustProductStock(status as ProductStatus))).toBe(true)
})
```

This test should pass if Task 3 is complete. It protects the entry visibility rule before editing pages.

- [ ] **Step 2: Add list page state and button**

In `ListPage.tsx`:

```tsx
import { StockAdjustmentModal, type StockAdjustmentProduct } from './components/StockAdjustmentModal'
import { canAdjustProductStock } from './stock-adjustment'
```

Add state:

```tsx
const [stockAdjustProduct, setStockAdjustProduct] = useState<StockAdjustmentProduct | null>(null)
```

Add action button before “关闭”:

```tsx
{canAdjustProductStock(row.status) && (
  <Button type="link" onClick={() => setStockAdjustProduct({ id: row.id, title: row.title, status: row.status, stock: row.stock })}>
    调整库存
  </Button>
)}
```

Render modal near the existing image modal:

```tsx
<StockAdjustmentModal
  open={Boolean(stockAdjustProduct)}
  product={stockAdjustProduct}
  onCancel={() => setStockAdjustProduct(null)}
  onSuccess={async () => {
    setStockAdjustProduct(null)
    actionRef.current?.reload()
  }}
/>
```

- [ ] **Step 3: Add detail page state and button**

In `DetailPage.tsx`:

```tsx
import { StockAdjustmentModal } from './components/StockAdjustmentModal'
import { canAdjustProductStock } from './stock-adjustment'
```

Add state:

```tsx
const [stockAdjustOpen, setStockAdjustOpen] = useState(false)
```

Add action button in `actionButtons`:

```tsx
canAdjustProductStock(product.status) && (
  <Button key="adjust-stock" onClick={() => setStockAdjustOpen(true)}>
    调整库存
  </Button>
)
```

Render modal:

```tsx
<StockAdjustmentModal
  open={stockAdjustOpen}
  product={{ id: product.id, title: product.title, status: product.status, stock: product.stock }}
  onCancel={() => setStockAdjustOpen(false)}
  onSuccess={async () => {
    setStockAdjustOpen(false)
    await queryClient.invalidateQueries({ queryKey: ['product-detail', productId] })
    await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
  }}
/>
```

- [ ] **Step 4: Run frontend tests and build**

Run from `frontend/`:

```powershell
npm test -- src/pages/merchant/products/stock-adjustment.test.ts src/pages/merchant/products/components/StockAdjustmentModal.test.tsx
npm run build
```

Expected: PASS.

- [ ] **Step 5: Commit Task 5**

```powershell
git add frontend/src/pages/merchant/products/ListPage.tsx frontend/src/pages/merchant/products/DetailPage.tsx
git commit -m "feat(inventory): expose stock adjustment entry"
```

## Task 6: 文档同步

**Files:**

- Modify: `docs/backend-api-checklist.md`
- Modify: `docs/frontend-pages.md`
- Modify: `docs/data-model.md`

**Interfaces:**

- Consumes:
  - 已实现接口与状态规则
- Produces:
  - 与当前代码一致的 API、页面、数据模型说明

- [ ] **Step 1: Update backend API checklist**

In `docs/backend-api-checklist.md`:

```markdown
| `/merchant/products/:id/stock-adjustments` | POST | 调整库存 | `id(path,R), adjustment_type(R:INCREASE/DECREASE/MARK_SOLD), quantity(R,>0), reason(R,2-255)` | `product_id,movement_id,adjustment_type,quantity,stock_before,stock_after,status_before,status_after,adjusted_at` | MERCHANT(full) |
```

Also replace old statements that say `stock` is fixed to `1` with current behavior: `stock` is an integer inventory field; manual changes should use the stock adjustment endpoint so movements are auditable.

- [ ] **Step 2: Update frontend page document**

In `docs/frontend-pages.md`, update product list/detail sections:

```markdown
- 商品列表和商品详情对 `DRAFT/ON_SHELF/OFF_SHELF` 商品展示“调整库存”入口。
- 调整库存弹窗包含当前库存、调整类型、调整数量、调整原因。
- `LOCKED/SOLD/CLOSED` 商品不展示可执行调整入口。
```

- [ ] **Step 3: Update data model document**

In `docs/data-model.md`, add `product_stock_adjustments` and update `products.stock`:

```markdown
| stock | int | 当前可用库存，允许大于等于 0；手动调整通过 `product_stock_adjustments` 记录流水 |
```

Add a new subsection with fields from the design document.

- [ ] **Step 4: Scan docs for obsolete fixed-stock text**

Run from repo root:

```powershell
rg -n "库存固定|固定为 1|仅允许1|仅允许为 `1`|stock.*固定" docs
```

Expected: no remaining product-management statements that contradict the implemented stock adjustment behavior. Historical docs can remain only if they clearly describe old scope; current checklist and page docs must be corrected.

- [ ] **Step 5: Commit Task 6**

```powershell
git add docs/backend-api-checklist.md docs/frontend-pages.md docs/data-model.md
git commit -m "docs(inventory): document stock adjustment flow"
```

## Task 7: Final verification and handoff

**Files:**

- Modify: `docs/superpowers/plans/2026-08-03-inventory-adjustment.md` only to mark completed checkboxes during execution

**Acceptance Evidence:**

| 验收项 | 证据 |
|---|---|
| AC1/AC2 | `frontend` build succeeds and code inspection confirms list/detail按钮只在允许状态显示 |
| AC3/AC4/AC5 | `go test ./tests -run TestProductStockAdjustment -v` passes |
| AC6/AC7 | 后端非法状态和跨商家测试 passes |
| AC8 | 后端幂等测试 passes，最终库存只变更一次 |
| AC9 | 未修改 `miniapp/`，小程序仍使用现有 `stock/status` |
| AC10 | 全量后端测试和前端构建通过 |

- [ ] **Step 1: Run backend targeted tests**

Run from `backend/`:

```powershell
go test ./tests -run TestProductStockAdjustment -v
```

Expected: PASS.

- [ ] **Step 2: Run backend full tests**

Run from `backend/`:

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run frontend tests**

Run from `frontend/`:

```powershell
npm test -- src/pages/merchant/products/stock-adjustment.test.ts src/pages/merchant/products/components/StockAdjustmentModal.test.tsx
```

Expected: PASS.

- [ ] **Step 4: Run frontend build**

Run from `frontend/`:

```powershell
npm run build
```

Expected: PASS.

- [ ] **Step 5: Confirm miniapp compatibility by diff**

Run from repo root:

```powershell
git diff --name-only HEAD
```

Expected: no files under `miniapp/`.

- [ ] **Step 6: Final commit if plan status was updated**

If only the plan checkbox status changed after implementation:

```powershell
git add docs/superpowers/plans/2026-08-03-inventory-adjustment.md
git commit -m "docs(inventory): update stock adjustment execution status"
```

If the plan was not updated after the last task commit, skip this commit.

## Self-Review

| 检查项 | 结果 |
|---|---|
| Spec coverage | 计划覆盖设计文档中的 API、流水、状态规则、幂等、权限、前端入口、文档同步、小程序兼容 |
| Placeholder scan | 没有未完成章节或空白任务 |
| Type consistency | 后端使用 `AdjustProductStockRequest`、`ProductStockAdjustment`、`StockAdjustment*` 常量；前端使用 `AdjustProductStockPayload` 和 `StockAdjustmentType` |
| Execution path | 由于当前会话不使用子代理，后续实现走内联执行方式 |
