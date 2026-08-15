# Merchant Category Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build merchant-owned first-level and second-level category management, lightweight default initialization, and buyer miniapp merchant-scoped category/product access.

**Architecture:** Add `merchant_id` to `categories`, keep legacy global rows unused, and route all runtime category/product validation through merchant-scoped helpers. The merchant backend gets CRUD endpoints and an Ant Design Pro management page; the miniapp sends `merchant_no` with buyer requests so one shared backend can serve separate merchant miniapp entries.

**Tech Stack:** Go 1.22, Gin, GORM, SQL migrations, React, TypeScript, Vite, Ant Design Pro, Taro, Vitest.

## Global Constraints

- The development branch is `codex/category-management`.
- Do not commit `deploy/acceptance/secrets/` or any credentials.
- Keep existing untracked files untouched unless a task explicitly creates or modifies that exact file.
- Use `gofmt` for Go changes.
- Keep TypeScript style consistent with the existing app: two-space indentation, single quotes, `@/` alias where already used.
- Category management supports exactly two levels: level 1 roots and level 2 children.
- Different merchants may reuse the same category names.
- Runtime merchant and buyer queries must ignore legacy global categories where `merchant_id IS NULL`.
- Buyer miniapp scoped requests use `merchant_no` query data.
- This plan does not add platform/admin template management or multiple category templates.

---

### Task 1: Category Model, Defaults, And Backfill Helpers

**Files:**
- Modify: `backend/internal/model/models.go`
- Modify: `backend/internal/model/models_test.go`
- Modify: `backend/internal/app/database_operations.go`
- Modify: `backend/internal/app/database_operations_test.go`

**Interfaces:**
- Produces: `model.Category.MerchantID *uint64`
- Produces: `func EnsureMerchantDefaultCategories(db *gorm.DB, merchantID uint64) error`
- Produces: `func BackfillMerchantCategories(db *gorm.DB) error`
- Produces: `func findOrCreateMerchantCategory(db *gorm.DB, merchantID uint64, parentID *uint64, level int8, name string, sort int) (model.Category, error)`
- Consumes: existing `defaultCategorySeeds`, `model.Merchant`, `model.Product`, and `gorm.DB`

- [ ] **Step 1: Write failing model metadata test**

Add this test to `backend/internal/model/models_test.go`:

```go
func TestCategoryModelMetadataSupportsMerchantOwnership(t *testing.T) {
	categorySchema, err := schema.Parse(&Category{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse category schema: %v", err)
	}

	assertFieldTagContains(t, categorySchema, "MerchantID",
		"index:idx_merchant_parent_sort,priority:1",
		"index:idx_merchant_level_status_sort,priority:1",
	)
	assertFieldTagContains(t, categorySchema, "ParentID",
		"index:idx_merchant_parent_sort,priority:2",
	)

	indexes := categorySchema.ParseIndexes()
	assertModelIndex(t, indexes, "idx_merchant_parent_sort", false, "MerchantID", "ParentID", "Sort")
	assertModelIndex(t, indexes, "idx_merchant_level_status_sort", false, "MerchantID", "Level", "Status", "Sort")
}
```

- [ ] **Step 2: Run model test and verify failure**

Run: `cd backend && go test ./internal/model -run TestCategoryModelMetadataSupportsMerchantOwnership -count=1`

Expected: FAIL because `Category.MerchantID` does not exist.

- [ ] **Step 3: Implement category model field and indexes**

In `backend/internal/model/models.go`, add `MerchantID *uint64` to `Category` and update index tags:

```go
type Category struct {
	ID         uint64         `gorm:"primaryKey" json:"id"`
	MerchantID *uint64        `gorm:"index:idx_merchant_parent_sort,priority:1;index:idx_merchant_level_status_sort,priority:1" json:"merchant_id"`
	ParentID   *uint64        `gorm:"index:idx_parent_sort,priority:1;index:idx_merchant_parent_sort,priority:2" json:"parent_id"`
	Level      int8           `gorm:"index:idx_level_status_sort,priority:1;index:idx_merchant_level_status_sort,priority:2" json:"level"`
	Name       string         `gorm:"size:64;uniqueIndex:uk_parent_name,priority:2" json:"name"`
	Status     string         `gorm:"size:16;index:idx_level_status_sort,priority:2;index:idx_merchant_level_status_sort,priority:3" json:"status"`
	Sort       int            `gorm:"index:idx_parent_sort,priority:2;index:idx_level_status_sort,priority:3;index:idx_merchant_parent_sort,priority:3;index:idx_merchant_level_status_sort,priority:4" json:"sort"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}
```

- [ ] **Step 4: Run model test and verify pass**

Run: `cd backend && go test ./internal/model -run TestCategoryModelMetadataSupportsMerchantOwnership -count=1`

Expected: PASS.

- [ ] **Step 5: Write failing default initializer and backfill tests**

Add tests to `backend/internal/app/database_operations_test.go`:

```go
func TestEnsureMerchantDefaultCategoriesCreatesPrivateCopies(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.Merchant{}, &model.Category{}); err != nil {
		t.Fatalf("migrate merchant categories: %v", err)
	}
	merchantOne := model.Merchant{MerchantNo: "M1", MerchantName: "One", ContactName: "A", ContactPhone: "1", ReviewStatus: model.ReviewApproved}
	merchantTwo := model.Merchant{MerchantNo: "M2", MerchantName: "Two", ContactName: "B", ContactPhone: "2", ReviewStatus: model.ReviewApproved}
	if err := db.Create(&merchantOne).Error; err != nil {
		t.Fatalf("create merchant one: %v", err)
	}
	if err := db.Create(&merchantTwo).Error; err != nil {
		t.Fatalf("create merchant two: %v", err)
	}

	for run := 0; run < 2; run++ {
		if err := EnsureMerchantDefaultCategories(db, merchantOne.ID); err != nil {
			t.Fatalf("seed merchant one run %d: %v", run+1, err)
		}
	}
	if err := EnsureMerchantDefaultCategories(db, merchantTwo.ID); err != nil {
		t.Fatalf("seed merchant two: %v", err)
	}

	var count int64
	if err := db.Model(&model.Category{}).Where("merchant_id IN ?", []uint64{merchantOne.ID, merchantTwo.ID}).Count(&count).Error; err != nil {
		t.Fatalf("count merchant categories: %v", err)
	}
	if count != 40 {
		t.Fatalf("merchant category count = %d, want 40", count)
	}

	var roots []model.Category
	if err := db.Where("merchant_id = ? AND parent_id IS NULL", merchantOne.ID).Order("sort ASC").Find(&roots).Error; err != nil {
		t.Fatalf("load roots: %v", err)
	}
	if len(roots) != len(defaultCategorySeeds) {
		t.Fatalf("root count = %d, want %d", len(roots), len(defaultCategorySeeds))
	}
}

func TestBackfillMerchantCategoriesRemapsProductsToMerchantCopies(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.Merchant{}, &model.Category{}, &model.Product{}); err != nil {
		t.Fatalf("migrate backfill schema: %v", err)
	}
	if err := SeedDefaultCategories(db); err != nil {
		t.Fatalf("seed legacy categories: %v", err)
	}
	merchant := model.Merchant{MerchantNo: "M1", MerchantName: "One", ContactName: "A", ContactPhone: "1", ReviewStatus: model.ReviewApproved}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	var legacyChild model.Category
	if err := db.Where("merchant_id IS NULL AND level = ? AND name = ?", 2, "家具").First(&legacyChild).Error; err != nil {
		t.Fatalf("load legacy child: %v", err)
	}
	product := model.Product{
		ProductNo: "P1", MerchantID: merchant.ID, Title: "Desk", Description: "Desk",
		CategoryID: legacyChild.ID, PriceCent: 100, ConditionLevel: "GOOD",
		Stock: 1, Status: model.ProductDraft, CreatedBy: 1, UpdatedBy: 1,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := BackfillMerchantCategories(db); err != nil {
		t.Fatalf("backfill merchant categories: %v", err)
	}

	var updated model.Product
	if err := db.First(&updated, product.ID).Error; err != nil {
		t.Fatalf("load product: %v", err)
	}
	if updated.CategoryID == legacyChild.ID {
		t.Fatal("product still points at legacy global category")
	}
	var ownedCategory model.Category
	if err := db.First(&ownedCategory, updated.CategoryID).Error; err != nil {
		t.Fatalf("load owned category: %v", err)
	}
	if ownedCategory.MerchantID == nil || *ownedCategory.MerchantID != merchant.ID || ownedCategory.Name != legacyChild.Name {
		t.Fatalf("product category not remapped to merchant copy: %+v", ownedCategory)
	}
}
```

- [ ] **Step 6: Run initializer tests and verify failure**

Run: `cd backend && go test ./internal/app -run 'TestEnsureMerchantDefaultCategoriesCreatesPrivateCopies|TestBackfillMerchantCategoriesRemapsProductsToMerchantCopies' -count=1`

Expected: FAIL because `EnsureMerchantDefaultCategories` and `BackfillMerchantCategories` do not exist.

- [ ] **Step 7: Implement merchant category initialization and backfill**

In `backend/internal/app/database_operations.go`, keep `SeedDefaultCategories` for legacy/global seeding, add merchant-specific helpers:

```go
func EnsureMerchantDefaultCategories(db *gorm.DB, merchantID uint64) error {
	if merchantID == 0 {
		return errors.New("merchant_id is required")
	}
	for i, seed := range defaultCategorySeeds {
		root, err := findOrCreateMerchantCategory(db, merchantID, nil, 1, seed.Name, i+1)
		if err != nil {
			return err
		}
		seen := map[string]struct{}{}
		sortOrder := 1
		for _, childName := range seed.Children {
			name := strings.TrimSpace(childName)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			if _, err := findOrCreateMerchantCategory(db, merchantID, &root.ID, 2, name, sortOrder); err != nil {
				return err
			}
			sortOrder++
		}
	}
	return nil
}
```

Implement `findOrCreateMerchantCategory` using `Unscoped()` with `merchant_id`, `parent_id`, `level`, and `name`. If the matching row is soft-deleted, revive it by clearing `deleted_at`, updating `status = ENABLED`, and setting the requested `sort`. Implement `BackfillMerchantCategories` in a transaction: load merchants, call `EnsureMerchantDefaultCategories(tx, merchant.ID)`, then update products whose current category is a legacy global level-2 category to the matching merchant-owned child by root and child names. Return an error when a product category cannot be matched.

- [ ] **Step 8: Run initializer tests and verify pass**

Run: `cd backend && go test ./internal/app -run 'TestEnsureMerchantDefaultCategoriesCreatesPrivateCopies|TestBackfillMerchantCategoriesRemapsProductsToMerchantCopies' -count=1`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/model/models.go backend/internal/model/models_test.go backend/internal/app/database_operations.go backend/internal/app/database_operations_test.go
git commit -m "feat(category): add merchant category defaults"
```

### Task 2: SQL Migration And Registration Initialization

**Files:**
- Create: `backend/migrations/0008_merchant_categories.up.sql`
- Create: `backend/migrations/0008_merchant_categories.down.sql`
- Create: `backend/scripts/backfill_merchant_categories/main.go`
- Create: `backend/scripts/backfill_merchant_categories/main_test.go`
- Modify: `backend/internal/app/auth_handlers.go`
- Create: `backend/tests/merchant_category_registration_test.go`

**Interfaces:**
- Consumes: `EnsureMerchantDefaultCategories(db *gorm.DB, merchantID uint64) error`
- Produces: registration creates default categories for new merchants

- [ ] **Step 1: Write failing integration test for registration defaults**

Create `backend/tests/merchant_category_registration_test.go`:

```go
package tests

import (
	"net/http"
	"testing"
)

func TestMerchantRegistrationCreatesDefaultCategories(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "category_registration")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": "MERCHANT", "username": username, "password": password,
	}, nil)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	merchantToken := str(login.Data["access_token"])

	cats := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/categories?level=1", nil, map[string]string{"Authorization": "Bearer " + merchantToken})
	if cats.Code != 0 {
		t.Fatalf("merchant categories code = %d data=%v", cats.Code, cats.Data)
	}
	items := cats.Data["items"].([]interface{})
	if len(items) != 3 {
		t.Fatalf("registered merchant root categories = %d, want 3", len(items))
	}
}
```

- [ ] **Step 2: Run integration test and verify failure**

Run: `cd backend && go test ./tests -run TestMerchantRegistrationCreatesDefaultCategories -count=1`

Expected: FAIL because registration does not create merchant-owned default categories and the read endpoint still reads global categories.

- [ ] **Step 3: Create migration files**

Create `backend/migrations/0008_merchant_categories.up.sql` with these operations:

```sql
ALTER TABLE categories
  ADD COLUMN merchant_id BIGINT NULL AFTER id,
  ADD INDEX idx_merchant_parent_sort (merchant_id, parent_id, sort),
  ADD INDEX idx_merchant_level_status_sort (merchant_id, level, status, sort);
```

Because production runtime guardrails reject automatic seed writes, keep data backfill in an explicit maintenance command rather than service startup.

Create `backend/migrations/0008_merchant_categories.down.sql`:

```sql
ALTER TABLE categories
  DROP INDEX idx_merchant_parent_sort,
  DROP INDEX idx_merchant_level_status_sort,
  DROP COLUMN merchant_id;
```

- [ ] **Step 4: Initialize categories during registration**

In `backend/internal/app/auth_handlers.go`, inside the registration transaction after account creation and before audit log creation, call:

```go
if err := EnsureMerchantDefaultCategories(tx, merchant.ID); err != nil {
	return err
}
```

- [ ] **Step 5: Add explicit backfill maintenance command**

Create `backend/scripts/backfill_merchant_categories/main.go` with a small CLI that loads config, opens the app DB, and calls `app.BackfillMerchantCategories(db)`. The command must reject extra positional arguments and must not print DSNs or secrets in errors.

Create `backend/scripts/backfill_merchant_categories/main_test.go` with tests for:

- unknown argument returns an error
- successful run calls a stubbed backfill function once
- error output does not include a sentinel DSN string

- [ ] **Step 6: Run registration test and verify pass after Task 3 read endpoint is scoped**

Run this after Task 3 implements the scoped read endpoint: `cd backend && go test ./tests -run TestMerchantRegistrationCreatesDefaultCategories -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/migrations/0008_merchant_categories.up.sql backend/migrations/0008_merchant_categories.down.sql backend/scripts/backfill_merchant_categories/main.go backend/scripts/backfill_merchant_categories/main_test.go backend/internal/app/auth_handlers.go backend/tests/merchant_category_registration_test.go
git commit -m "feat(category): initialize categories for new merchants"
```

### Task 3: Merchant Category CRUD And Product Ownership Validation

**Files:**
- Modify: `backend/internal/dto/dto.go`
- Create: `backend/internal/app/category_handlers.go`
- Modify: `backend/internal/app/server.go`
- Modify: `backend/internal/app/product_handlers.go`
- Create: `backend/tests/merchant_category_test.go`

**Interfaces:**
- Produces: `CreateCategoryRequest`, `UpdateCategoryRequest`
- Produces: `GET/POST/PUT/DELETE /api/v1/merchant/categories`
- Produces: `func (s *Server) ensureMerchantLevel2Category(db *gorm.DB, merchantID uint64, categoryID uint64) error`
- Consumes: `actor.MerchantID`

- [ ] **Step 1: Write failing backend API tests**

Create `backend/tests/merchant_category_test.go` with tests that:

```go
func TestMerchantCategoryCRUDIsScopedToMerchant(t *testing.T) {
	srv := newTestServer(t)
	merchantOneID, merchantOneUser, merchantOnePassword := registerMerchant(t, srv, "cat-owner-1")
	merchantTwoID, merchantTwoUser, merchantTwoPassword := registerMerchant(t, srv, "cat-owner-2")
	adminToken := adminAccessToken(t, srv)
	approveMerchant(t, srv, adminToken, merchantOneID)
	approveMerchant(t, srv, adminToken, merchantTwoID)
	merchantOneLogin := merchantLogin(t, srv, merchantOneUser, merchantOnePassword)
	merchantTwoLogin := merchantLogin(t, srv, merchantTwoUser, merchantTwoPassword)
	if merchantOneLogin.Code != 0 || merchantTwoLogin.Code != 0 {
		t.Fatalf("merchant login failed: one=%+v two=%+v", merchantOneLogin, merchantTwoLogin)
	}
	merchantOneToken := str(merchantOneLogin.Data["access_token"])
	merchantTwoToken := str(merchantTwoLogin.Data["access_token"])

	createRoot := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/categories", map[string]interface{}{
		"level": 1, "name": "自定义一级", "sort": 9,
	}, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if createRoot.Code != 0 {
		t.Fatalf("create root code = %d data=%v", createRoot.Code, createRoot.Data)
	}
	rootID := uint64(createRoot.Data["id"].(float64))

	createChild := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/categories", map[string]interface{}{
		"level": 2, "parent_id": rootID, "name": "自定义二级", "sort": 1,
	}, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if createChild.Code != 0 {
		t.Fatalf("create child code = %d data=%v", createChild.Code, createChild.Data)
	}
	childID := uint64(createChild.Data["id"].(float64))

	otherUpdate := requestJSON(t, srv.Router, http.MethodPut, fmt.Sprintf("/api/v1/merchant/categories/%d", childID), map[string]interface{}{
		"name": "越权修改",
	}, map[string]string{"Authorization": "Bearer " + merchantTwoToken})
	if otherUpdate.Code == 0 {
		t.Fatal("cross-merchant category update succeeded")
	}

	deleteRootWithChild := requestJSON(t, srv.Router, http.MethodDelete, fmt.Sprintf("/api/v1/merchant/categories/%d", rootID), nil, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if deleteRootWithChild.Code == 0 {
		t.Fatal("deleted root category that still has children")
	}
}
```

Add a second test that creates a merchant one child category and verifies merchant two cannot create a product using that child category.

- [ ] **Step 2: Run category API tests and verify failure**

Run: `cd backend && go test ./tests -run 'TestMerchantCategory|TestProductRejectsCrossMerchantCategory' -count=1`

Expected: FAIL because routes and handlers do not exist.

- [ ] **Step 3: Add DTOs**

In `backend/internal/dto/dto.go` add:

```go
type CreateCategoryRequest struct {
	Level    int8    `json:"level" binding:"required"`
	ParentID *uint64 `json:"parent_id"`
	Name     string  `json:"name" binding:"required"`
	Sort     int     `json:"sort"`
}

type UpdateCategoryRequest struct {
	Name   *string `json:"name"`
	Sort   *int    `json:"sort"`
	Status *string `json:"status"`
}
```

- [ ] **Step 4: Implement category handlers**

Create `backend/internal/app/category_handlers.go` with merchant-scoped helper functions:

```go
func (s *Server) handleCategories(c *gin.Context)
func (s *Server) handleCreateCategory(c *gin.Context)
func (s *Server) handleUpdateCategory(c *gin.Context)
func (s *Server) handleDeleteCategory(c *gin.Context)
func (s *Server) loadOwnedCategory(db *gorm.DB, categoryID uint64, merchantID uint64) (model.Category, error)
func (s *Server) ensureMerchantLevel2Category(db *gorm.DB, merchantID uint64, categoryID uint64) error
```

Validation rules:

- trim `name`; reject empty names.
- create level 1 only with nil `parent_id`.
- create level 2 only with a parent owned by the same merchant and `level = 1`.
- duplicate check uses `merchant_id`, `parent_id`, `name`, and `deleted_at IS NULL`.
- update allows `name`, `sort`, and `status` only; accepted statuses are `ENABLED` and `DISABLED`.
- delete checks product references for level 2 and child references for level 1 before soft-delete.

- [ ] **Step 5: Register routes**

In `backend/internal/app/server.go`, under merchant full-scope routes:

```go
merchant.GET("/categories", middleware.RequireFullMerchantScope(), s.handleCategories)
merchant.POST("/categories", middleware.RequireFullMerchantScope(), s.handleCreateCategory)
merchant.PUT("/categories/:id", middleware.RequireFullMerchantScope(), s.handleUpdateCategory)
merchant.DELETE("/categories/:id", middleware.RequireFullMerchantScope(), s.handleDeleteCategory)
```

- [ ] **Step 6: Replace product category validation**

In `backend/internal/app/product_handlers.go`, replace `ensureLevel2Category(categoryID)` calls with:

```go
if err := s.ensureMerchantLevel2Category(s.DB, actor.MerchantID, req.CategoryID); err != nil {
	common.Fail(c, err)
	return
}
```

Inside transactions, pass `tx` instead of `s.DB`.

- [ ] **Step 7: Run backend category tests and verify pass**

Run: `cd backend && go test ./tests -run 'TestMerchantCategory|TestProductRejectsCrossMerchantCategory' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/dto/dto.go backend/internal/app/category_handlers.go backend/internal/app/server.go backend/internal/app/product_handlers.go backend/tests/merchant_category_test.go
git commit -m "feat(category): add merchant category management api"
```

### Task 4: Buyer Merchant Scope

**Files:**
- Modify: `backend/internal/app/buyer_handlers.go`
- Modify: `backend/tests/buyer_flow_test.go`

**Interfaces:**
- Produces: `func (s *Server) resolveBuyerMerchantScope(c *gin.Context) (model.Merchant, error)`
- Consumes: `merchant_no` query parameter

- [ ] **Step 1: Write failing buyer scope tests**

Add tests to `backend/tests/buyer_flow_test.go`:

```go
func TestBuyerProductsAndCategoriesRequireMerchantNoAndStayScoped(t *testing.T) {
	srv := newTestServer(t)
	merchantOneID, merchantOneUser, merchantOnePassword := registerMerchant(t, srv, "buyer-scope-1")
	merchantTwoID, merchantTwoUser, merchantTwoPassword := registerMerchant(t, srv, "buyer-scope-2")
	adminToken := adminAccessToken(t, srv)
	approveMerchant(t, srv, adminToken, merchantOneID)
	approveMerchant(t, srv, adminToken, merchantTwoID)
	loginOne := merchantLogin(t, srv, merchantOneUser, merchantOnePassword)
	loginTwo := merchantLogin(t, srv, merchantTwoUser, merchantTwoPassword)
	if loginOne.Code != 0 || loginTwo.Code != 0 {
		t.Fatalf("merchant login failed: one=%+v two=%+v", loginOne, loginTwo)
	}
	tokenOne := str(loginOne.Data["access_token"])
	tokenTwo := str(loginTwo.Data["access_token"])
	merchantOneNo := merchantNoByID(t, srv, merchantOneID)
	merchantTwoNo := merchantNoByID(t, srv, merchantTwoID)

	productOneID := createOnShelfProductForMerchant(t, srv, tokenOne)
	productTwoID := createOnShelfProductForMerchant(t, srv, tokenTwo)

	missing := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/products", nil, nil)
	if missing.Code == 0 {
		t.Fatal("buyer products without merchant_no succeeded")
	}

	listOne := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/products?merchant_no="+merchantOneNo, nil, nil)
	if listOne.Code != 0 {
		t.Fatalf("buyer scoped list code = %d data=%v", listOne.Code, listOne.Data)
	}
	items := listOne.Data["items"].([]interface{})
	if len(items) != 1 || uint64(items[0].(map[string]interface{})["id"].(float64)) != productOneID {
		t.Fatalf("merchant one list leaked or missed products: %+v, product two=%d", items, productTwoID)
	}

	crossDetail := requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/buyer/products/%d?merchant_no=%s", productTwoID, merchantOneNo), nil, nil)
	if crossDetail.Code == 0 {
		t.Fatal("buyer detail leaked product from another merchant")
	}

	categories := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/categories?level=1&merchant_no="+merchantTwoNo, nil, nil)
	if categories.Code != 0 {
		t.Fatalf("buyer categories code = %d data=%v", categories.Code, categories.Data)
	}
}
```

Add this helper in `backend/tests/buyer_flow_test.go`:

```go
func merchantNoByID(t *testing.T, srv *app.Server, merchantID uint64) string {
	t.Helper()
	var merchant model.Merchant
	if err := srv.DB.First(&merchant, merchantID).Error; err != nil {
		t.Fatalf("load merchant number: %v", err)
	}
	return merchant.MerchantNo
}
```

- [ ] **Step 2: Run buyer tests and verify failure**

Run: `cd backend && go test ./tests -run TestBuyerProductsAndCategoriesRequireMerchantNoAndStayScoped -count=1`

Expected: FAIL because buyer handlers do not require or apply `merchant_no`.

- [ ] **Step 3: Implement merchant scope resolver**

In `backend/internal/app/buyer_handlers.go`, add:

```go
func (s *Server) resolveBuyerMerchantScope(c *gin.Context) (model.Merchant, error) {
	merchantNo := strings.TrimSpace(c.Query("merchant_no"))
	if merchantNo == "" {
		return model.Merchant{}, common.ErrInvalidArgument
	}
	var merchant model.Merchant
	if err := s.DB.Where("merchant_no = ?", merchantNo).First(&merchant).Error; err != nil {
		return model.Merchant{}, s.dbError(err)
	}
	return merchant, nil
}
```

- [ ] **Step 4: Scope buyer read handlers**

Apply `merchant.ID` filters:

- `handleBuyerCategories`: `merchant_id = merchant.ID`
- `handleBuyerProducts`: `merchant_id = merchant.ID`
- `handleBuyerProductDetail`: `id = ? AND merchant_id = ?`

- [ ] **Step 5: Scope buyer user-state handlers**

In favorite, history, and intent handlers:

- list queries must filter rows by `merchant_id = scopedMerchant.ID`.
- add favorite/report history/create intent must load target products with `merchant_id = scopedMerchant.ID`.
- detail endpoints must verify the stored row's `merchant_id` matches `scopedMerchant.ID`.

- [ ] **Step 6: Run buyer scope tests and verify pass**

Run: `cd backend && go test ./tests -run TestBuyerProductsAndCategoriesRequireMerchantNoAndStayScoped -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/app/buyer_handlers.go backend/tests/buyer_flow_test.go
git commit -m "feat(buyer): scope miniapp data by merchant entry"
```

### Task 5: Merchant Admin Category UI

**Files:**
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/app/Layout.tsx`
- Modify: `frontend/src/app/App.tsx`
- Create: `frontend/src/pages/merchant/categories/ListPage.tsx`
- Modify: `frontend/src/app/Layout.test.tsx`
- Create: `frontend/src/pages/merchant/categories/ListPage.test.tsx`

**Interfaces:**
- Produces: `api.createCategory`, `api.updateCategory`, `api.deleteCategory`
- Produces: route `/merchant/categories`

- [ ] **Step 1: Write failing layout test**

In `frontend/src/app/Layout.test.tsx`, add an assertion that merchant menus include `商品分类` and `/merchant/categories`.

- [ ] **Step 2: Write failing category page test**

Create `frontend/src/pages/merchant/categories/ListPage.test.tsx` to mock `api.categories`, render `ListPage`, and assert that root and child categories render. Mock create/update/delete calls and assert the page invokes `api.createCategory`, `api.updateCategory`, and `api.deleteCategory` from button/form interactions.

- [ ] **Step 3: Run frontend tests and verify failure**

Run: `cd frontend && npm run test -- Layout.test.tsx ListPage.test.tsx --run`

Expected: FAIL because menu, route, API methods, and page do not exist.

- [ ] **Step 4: Extend frontend API client**

In `frontend/src/services/api.ts`, add:

```ts
createCategory(payload: { level: 1 | 2; parent_id?: number; name: string; sort?: number }) {
  return http.post('/merchant/categories', payload)
},
updateCategory(categoryId: string | number, payload: Partial<{ name: string; sort: number; status: string }>) {
  return http.put(`/merchant/categories/${categoryId}`, payload)
},
deleteCategory(categoryId: string | number) {
  return http.delete(`/merchant/categories/${categoryId}`)
},
```

- [ ] **Step 5: Add menu and route**

In `frontend/src/app/Layout.tsx`, import `TagsOutlined` and add:

```tsx
{ path: '/merchant/categories', name: '商品分类', icon: <TagsOutlined /> },
```

In `frontend/src/app/App.tsx`, lazy-load `@/pages/merchant/categories/ListPage` and add route:

```tsx
<Route path="/merchant/categories" element={loadable(<CategoryListPage />)} />
```

- [ ] **Step 6: Implement category page**

Create `frontend/src/pages/merchant/categories/ListPage.tsx` using existing Ant Design patterns:

- fetch roots with `api.categories(1)`
- fetch all children with `api.categories(2)`
- display a `Table` with expandable child rows or nested child lists
- use `ModalForm` or `Modal` + `Form` for create/edit
- use `Popconfirm` for delete
- use `Switch` or action button for `ENABLED`/`DISABLED`
- call `queryClient.invalidateQueries({ queryKey: ['merchant-categories'] })` after mutations

- [ ] **Step 7: Run frontend tests and build**

Run:

```bash
cd frontend && npm run test -- Layout.test.tsx ListPage.test.tsx --run
cd frontend && npm run build
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/services/api.ts frontend/src/app/Layout.tsx frontend/src/app/App.tsx frontend/src/app/Layout.test.tsx frontend/src/pages/merchant/categories/ListPage.tsx frontend/src/pages/merchant/categories/ListPage.test.tsx
git commit -m "feat(frontend): add merchant category management page"
```

### Task 6: Miniapp Merchant Number Propagation

**Files:**
- Modify: `miniapp/src/services/request.ts`
- Modify: `miniapp/src/services/buyer.ts`
- Create: `miniapp/tests/merchant-scope.test.ts`

**Interfaces:**
- Produces: `declare const __MERCHANT_NO__: string` in `miniapp/src/services/buyer.ts`
- Produces: buyer API requests include `merchant_no`

- [ ] **Step 1: Write failing miniapp service test**

Create `miniapp/tests/merchant-scope.test.ts` that stubs the global merchant constant and verifies `fetchBuyerProducts`, `fetchBuyerCategories`, `fetchBuyerProductDetail`, `addFavorite`, `reportView`, `createIntent`, `listFavorites`, `listHistories`, and `listIntents` pass `merchant_no` in request data.

- [ ] **Step 2: Run miniapp test and verify failure**

Run: `cd miniapp && npm test -- merchant-scope.test.ts --run`

Expected: FAIL because buyer API calls do not attach `merchant_no`.

- [ ] **Step 3: Add merchant number env constant**

In `miniapp/config/index.ts`, define `__MERCHANT_NO__` from `process.env.TARO_APP_MERCHANT_NO || ''` using the same `defineConstants` style as `__API_BASE_URL__`.

In `miniapp/src/services/buyer.ts`, add the compile-time constant declaration directly:

```ts
declare const __MERCHANT_NO__: string
```

- [ ] **Step 4: Attach merchant_no in buyer service**

In `miniapp/src/services/buyer.ts`, add:

```ts
declare const __MERCHANT_NO__: string

function withMerchantScope(params: Record<string, unknown> = {}) {
  return { ...params, merchant_no: __MERCHANT_NO__ }
}
```

Wrap buyer-scoped request `data` values with `withMerchantScope(...)`.

- [ ] **Step 5: Run miniapp tests**

Run:

```bash
cd miniapp && npm test -- merchant-scope.test.ts --run
cd miniapp && npm test -- --run
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add miniapp/config/index.ts miniapp/src/services/buyer.ts miniapp/tests/merchant-scope.test.ts
git commit -m "feat(miniapp): send merchant scope with buyer requests"
```

### Task 7: Final Verification And Documentation Touchups

**Files:**
- Modify: `docs/backend-api-checklist.md`
- Modify: `docs/frontend-pages.md`
- Modify: `docs/miniapp-buyer-api-checklist.md`

**Interfaces:**
- Consumes: all previous task outputs
- Produces: verified branch ready for review

- [ ] **Step 1: Update docs for endpoint contracts**

Document merchant category CRUD endpoints in `docs/backend-api-checklist.md`, add the merchant category page in `docs/frontend-pages.md`, and document required `merchant_no` buyer scope in `docs/miniapp-buyer-api-checklist.md`.

- [ ] **Step 2: Run focused backend verification**

Run:

```bash
cd backend && go test ./internal/model ./internal/app ./tests -count=1
```

Expected: PASS.

- [ ] **Step 3: Run frontend verification**

Run:

```bash
cd frontend && npm run test -- --run
cd frontend && npm run build
```

Expected: PASS.

- [ ] **Step 4: Run miniapp verification**

Run:

```bash
cd miniapp && npm test -- --run
cd miniapp && npm run build:weapp
```

Expected: PASS.

- [ ] **Step 5: Run repository-level verification**

Run:

```bash
make test
git status --short --branch
```

Expected: backend tests pass and the only remaining changes are intentional tracked changes or unrelated pre-existing untracked files.

- [ ] **Step 6: Commit final docs**

```bash
git add docs/backend-api-checklist.md docs/frontend-pages.md docs/miniapp-buyer-api-checklist.md
git commit -m "docs: update category management contracts"
```
