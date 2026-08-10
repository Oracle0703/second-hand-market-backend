package tests

import (
	"fmt"
	"net/http"
	"testing"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/model"
)

func approvedMerchantTokenForStockAdjustment(t *testing.T, srv *app.Server, prefix string) string {
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
	token := approvedMerchantTokenForStockAdjustment(t, srv, "stock_adjust")
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

func TestProductStockAdjustmentMarkSold(t *testing.T) {
	srv := newTestServer(t)
	token := approvedMerchantTokenForStockAdjustment(t, srv, "stock_sold")
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

func TestProductStockAdjustmentMarkSoldAndSoldRecoveryRules(t *testing.T) {
	srv := newTestServer(t)
	token := approvedMerchantTokenForStockAdjustment(t, srv, "stock_sold_rules")

	draftProductID := createDraftProduct(t, srv, token)
	draftMarkSold := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", draftProductID),
		map[string]interface{}{"adjustment_type": "MARK_SOLD", "quantity": 1, "reason": "草稿售罄"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if draftMarkSold.Code != 10005 {
		t.Fatalf("draft product mark sold should be rejected: %+v", draftMarkSold)
	}

	productID := createAndOnShelfProductWithStock(t, srv, token, 4)
	partialMarkSold := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID),
		map[string]interface{}{"adjustment_type": "MARK_SOLD", "quantity": 1, "reason": "部分售出"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if partialMarkSold.Code != 0 || numToUint64(partialMarkSold.Data["stock_after"]) != 3 || str(partialMarkSold.Data["status_after"]) != model.ProductOnShelf {
		t.Fatalf("partial mark sold should keep on-shelf status: %+v", partialMarkSold)
	}

	allRemaining := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID),
		map[string]interface{}{"adjustment_type": "MARK_SOLD", "all_remaining": true, "reason": "全部售出"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if allRemaining.Code != 0 || numToUint64(allRemaining.Data["quantity"]) != 3 || numToUint64(allRemaining.Data["stock_after"]) != 0 || str(allRemaining.Data["status_after"]) != model.ProductSold {
		t.Fatalf("all remaining mark sold should apply actual remaining stock: %+v", allRemaining)
	}
	var allRemainingMovement model.ProductStockAdjustment
	if err := srv.DB.Where("id = ?", numToUint64(allRemaining.Data["movement_id"])).First(&allRemainingMovement).Error; err != nil {
		t.Fatalf("load all remaining stock movement failed: %v", err)
	}
	if allRemainingMovement.AdjustmentType != "MARK_SOLD" || allRemainingMovement.Quantity != 3 || allRemainingMovement.StockBefore != 3 || allRemainingMovement.StockAfter != 0 || allRemainingMovement.StatusAfter != model.ProductSold {
		t.Fatalf("all remaining stock movement mismatch: %+v", allRemainingMovement)
	}

	restock := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", productID),
		map[string]interface{}{"adjustment_type": "INCREASE", "quantity": 2, "reason": "补货恢复"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if restock.Code != 0 || numToUint64(restock.Data["stock_after"]) != 2 || str(restock.Data["status_after"]) != model.ProductOffShelf {
		t.Fatalf("sold product increase should restore it to off-shelf: %+v", restock)
	}

	soldProductID := createAndOnShelfProduct(t, srv, token)
	markSold := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", soldProductID),
		map[string]interface{}{"adjustment_type": "MARK_SOLD", "quantity": 1, "reason": "售罄测试"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if markSold.Code != 0 {
		t.Fatalf("prepare sold product failed: %+v", markSold)
	}
	for _, adjustmentType := range []string{"DECREASE", "MARK_SOLD"} {
		adjustment := requestJSON(
			t,
			srv.Router,
			http.MethodPost,
			fmt.Sprintf("/api/v1/merchant/products/%d/stock-adjustments", soldProductID),
			map[string]interface{}{"adjustment_type": adjustmentType, "quantity": 1, "reason": "售罄后调整"},
			map[string]string{"Authorization": "Bearer " + token},
		)
		if adjustment.Code != 10005 {
			t.Fatalf("sold product %s should be rejected: %+v", adjustmentType, adjustment)
		}
	}
}

func TestProductStockAdjustmentRejectsInvalidStatesAndInsufficientStock(t *testing.T) {
	srv := newTestServer(t)
	token := approvedMerchantTokenForStockAdjustment(t, srv, "stock_invalid")
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
