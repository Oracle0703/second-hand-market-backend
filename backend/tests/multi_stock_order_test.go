package tests

import (
	"fmt"
	"net/http"
	"testing"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func createMerchantOrder(t *testing.T, token string, productID uint64, quantity, unitPriceCent int, srvRouter http.Handler) apiResp {
	t.Helper()
	return requestJSON(t, srvRouter, http.MethodPost, "/api/v1/merchant/orders", map[string]interface{}{
		"product_id":      productID,
		"quantity":        quantity,
		"deal_price_cent": unitPriceCent,
	}, map[string]string{"Authorization": "Bearer " + token})
}

func TestMultiStockOrderReservationLifecycle(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "multi_stock")
	approveMerchant(t, srv, adminToken, merchantID)
	login := merchantLogin(t, srv, username, password)
	token := str(login.Data["access_token"])
	productID := createAndOnShelfProductWithStock(t, srv, token, 5)

	first := createMerchantOrder(t, token, productID, 2, 1200, srv.Router)
	if first.Code != common.CodeOK || numToUint64(first.Data["quantity"]) != 2 || numToUint64(first.Data["total_deal_price_cent"]) != 2400 {
		t.Fatalf("create first multi-stock order: %+v", first)
	}
	second := createMerchantOrder(t, token, productID, 1, 1500, srv.Router)
	if second.Code != common.CodeOK || numToUint64(second.Data["available_stock"]) != 2 {
		t.Fatalf("create second active order: %+v", second)
	}

	var product model.Product
	if err := srv.DB.First(&product, productID).Error; err != nil {
		t.Fatalf("load reserved product: %v", err)
	}
	if product.Status != model.ProductOnShelf || product.Stock != 5 || product.ReservedStock != 3 || product.ActiveOrderID != nil {
		t.Fatalf("unexpected reserved product: %+v", product)
	}
	var activeOrders int64
	if err := srv.DB.Model(&model.Order{}).Where("product_id = ? AND is_active = ?", productID, true).Count(&activeOrders).Error; err != nil {
		t.Fatalf("count active orders: %v", err)
	}
	if activeOrders != 2 {
		t.Fatalf("expected two active orders, got %d", activeOrders)
	}

	buyerDetail := requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/buyer/products/%d", productID), nil, map[string]string{"X-Device-Id": "multi-stock-device"})
	buyerProduct, _ := buyerDetail.Data["product"].(map[string]interface{})
	if buyerDetail.Code != common.CodeOK || numToUint64(buyerProduct["stock"]) != 2 {
		t.Fatalf("buyer stock must expose available inventory: %+v", buyerDetail)
	}

	offShelf := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/off-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if offShelf.Code != common.CodeOK {
		t.Fatalf("off shelf with active orders: %+v", offShelf)
	}
	blockedCreate := createMerchantOrder(t, token, productID, 1, 1300, srv.Router)
	if blockedCreate.Code != common.CodeInvalidTransition {
		t.Fatalf("off-shelf product must reject new orders: %+v", blockedCreate)
	}

	firstID := numToUint64(first.Data["order_id"])
	complete := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/complete", firstID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if complete.Code != common.CodeOK || str(complete.Data["product_status"]) != model.ProductOffShelf || numToUint64(complete.Data["stock"]) != 3 || numToUint64(complete.Data["reserved_stock"]) != 1 {
		t.Fatalf("complete first order: %+v", complete)
	}

	secondID := numToUint64(second.Data["order_id"])
	closeResp := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/close", secondID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if closeResp.Code != common.CodeOK || str(closeResp.Data["product_status"]) != model.ProductOffShelf || numToUint64(closeResp.Data["stock"]) != 3 || numToUint64(closeResp.Data["reserved_stock"]) != 0 {
		t.Fatalf("close second order: %+v", closeResp)
	}

	onShelf := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if onShelf.Code != common.CodeOK {
		t.Fatalf("re-on-shelf product: %+v", onShelf)
	}
	finalOrder := createMerchantOrder(t, token, productID, 3, 1100, srv.Router)
	if finalOrder.Code != common.CodeOK {
		t.Fatalf("create final order: %+v", finalOrder)
	}
	finalComplete := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/complete", numToUint64(finalOrder.Data["order_id"])), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if finalComplete.Code != common.CodeOK || str(finalComplete.Data["product_status"]) != model.ProductSold || numToUint64(finalComplete.Data["stock"]) != 0 {
		t.Fatalf("final completion should sell out product: %+v", finalComplete)
	}
}

func TestReservedStockProtectsProductEditsAndClose(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "reserved_rules")
	approveMerchant(t, srv, adminToken, merchantID)
	login := merchantLogin(t, srv, username, password)
	token := str(login.Data["access_token"])
	productID := createAndOnShelfProductWithStock(t, srv, token, 5)
	order := createMerchantOrder(t, token, productID, 3, 1000, srv.Router)
	if order.Code != common.CodeOK {
		t.Fatalf("create reserved order: %+v", order)
	}

	offShelf := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/off-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if offShelf.Code != common.CodeOK {
		t.Fatalf("off shelf: %+v", offShelf)
	}
	tooLow := requestJSON(t, srv.Router, http.MethodPut, fmt.Sprintf("/api/v1/merchant/products/%d", productID), map[string]interface{}{"stock": 2}, map[string]string{"Authorization": "Bearer " + token})
	if tooLow.Code != common.CodeConflict {
		t.Fatalf("stock below reservation must conflict: %+v", tooLow)
	}
	closeProduct := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/close", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if closeProduct.Code != common.CodeInvalidTransition {
		t.Fatalf("reserved product must not close permanently: %+v", closeProduct)
	}

	closeOrder := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/close", numToUint64(order.Data["order_id"])), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if closeOrder.Code != common.CodeOK {
		t.Fatalf("release reservation: %+v", closeOrder)
	}
	closeProduct = requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/close", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if closeProduct.Code != common.CodeOK {
		t.Fatalf("close product after releasing reservation: %+v", closeProduct)
	}
}

func TestBuyerStockCompatibilityUsesAvailableInventoryEverywhere(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyer_available_stock")
	approveMerchant(t, srv, adminToken, merchantID)
	login := merchantLogin(t, srv, username, password)
	token := str(login.Data["access_token"])
	productID := createAndOnShelfProductWithStock(t, srv, token, 5)
	order := createMerchantOrder(t, token, productID, 3, 1000, srv.Router)
	if order.Code != common.CodeOK {
		t.Fatalf("create reserved order: %+v", order)
	}

	merchantDetail := requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/merchant/products/%d", productID), nil, map[string]string{"Authorization": "Bearer " + token})
	merchantProduct, _ := merchantDetail.Data["product"].(map[string]interface{})
	if merchantDetail.Code != common.CodeOK || numToUint64(merchantProduct["stock"]) != 5 || numToUint64(merchantProduct["reserved_stock"]) != 3 || numToUint64(merchantProduct["available_stock"]) != 2 {
		t.Fatalf("merchant inventory semantics changed: %+v", merchantDetail)
	}

	buyerHeaders := map[string]string{"X-Device-Id": "buyer-available-stock-device"}
	list := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/products?page=1&page_size=20", nil, buyerHeaders)
	assertBuyerItemStock(t, list, "id", productID, 2, "buyer product list")

	detail := requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/buyer/products/%d", productID), nil, buyerHeaders)
	detailProduct, _ := detail.Data["product"].(map[string]interface{})
	assertBuyerStockOnly(t, detail.Code, detailProduct, 2, "buyer product detail")

	favorite := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/favorites", map[string]interface{}{"product_id": productID}, buyerHeaders)
	if favorite.Code != common.CodeOK {
		t.Fatalf("favorite product: %+v", favorite)
	}
	favorites := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/favorites", nil, buyerHeaders)
	assertBuyerItemStock(t, favorites, "product_id", productID, 2, "buyer favorites")

	history := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/histories/views", map[string]interface{}{"product_id": productID}, buyerHeaders)
	if history.Code != common.CodeOK {
		t.Fatalf("record history: %+v", history)
	}
	histories := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/histories", nil, buyerHeaders)
	assertBuyerItemStock(t, histories, "product_id", productID, 2, "buyer histories")
}

func assertBuyerItemStock(t *testing.T, resp apiResp, idField string, productID uint64, wantStock uint64, context string) {
	t.Helper()
	if resp.Code != common.CodeOK {
		t.Fatalf("%s request failed: %+v", context, resp)
	}
	items, _ := resp.Data["items"].([]interface{})
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		if numToUint64(item[idField]) == productID {
			assertBuyerStockOnly(t, resp.Code, item, wantStock, context)
			return
		}
	}
	t.Fatalf("%s did not contain product %d: %+v", context, productID, resp)
}

func assertBuyerStockOnly(t *testing.T, code int, item map[string]interface{}, wantStock uint64, context string) {
	t.Helper()
	if code != common.CodeOK || numToUint64(item["stock"]) != wantStock {
		t.Fatalf("%s stock must be available inventory %d: %+v", context, wantStock, item)
	}
	if _, exists := item["reserved_stock"]; exists {
		t.Fatalf("%s must not expose reserved inventory: %+v", context, item)
	}
}
