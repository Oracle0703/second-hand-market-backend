package tests

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/model"
)

func uniqueUsername(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func registerMerchant(t *testing.T, srv *app.Server, prefix string) (uint64, string, string) {
	t.Helper()
	username := uniqueUsername(prefix)
	password := "Passw0rd!2026"
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": 1000, "mime_type": "image/jpeg",
	}, nil)
	if presign.Code != 0 {
		t.Fatalf("license presign failed: %+v", presign)
	}
	register := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register", map[string]interface{}{
		"merchant_name":   "测试商家_" + prefix,
		"contact_name":    "张三",
		"phone":           fmt.Sprintf("139%08d", time.Now().UnixNano()%100000000),
		"username":        username,
		"password":        password,
		"license_file_id": numToUint64(presign.Data["file_id"]),
	}, nil)
	if register.Code != 0 {
		t.Fatalf("register failed: %+v", register)
	}
	return numToUint64(register.Data["merchant_id"]), username, password
}

func adminAccessToken(t *testing.T, srv *app.Server) string {
	t.Helper()
	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": "ADMIN", "username": "admin", "password": "Admin@123456",
	}, nil)
	if login.Code != 0 {
		t.Fatalf("admin login failed: %+v", login)
	}
	return str(login.Data["access_token"])
}

func merchantLogin(t *testing.T, srv *app.Server, username, password string) apiResp {
	t.Helper()
	return requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": "MERCHANT", "username": username, "password": password,
	}, nil)
}

func approveMerchant(t *testing.T, srv *app.Server, adminToken string, merchantID uint64) {
	t.Helper()
	resp := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/admin/merchants/%d/approve", merchantID), map[string]interface{}{"comment": "ok"}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.Code != 0 {
		t.Fatalf("approve failed: %+v", resp)
	}
}

func rejectMerchant(t *testing.T, srv *app.Server, adminToken string, merchantID uint64) {
	t.Helper()
	resp := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/admin/merchants/%d/reject", merchantID), map[string]interface{}{"reason": "资料需补充"}, map[string]string{"Authorization": "Bearer " + adminToken})
	if resp.Code != 0 {
		t.Fatalf("reject failed: %+v", resp)
	}
}

func productImageAndCategory(t *testing.T, srv *app.Server, merchantToken string) (uint64, uint64) {
	t.Helper()
	img := uploadProductImage(t, srv, merchantToken, encodedUploadImage(t, "image/jpeg"), "image/jpeg")
	cats := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/categories?level=2", nil, map[string]string{"Authorization": "Bearer " + merchantToken})
	if cats.Code != 0 {
		t.Fatalf("categories failed: %+v", cats)
	}
	items, ok := cats.Data["items"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("categories empty: %+v", cats)
	}
	row := items[0].(map[string]interface{})
	categoryID := numToUint64(row["id"])
	if categoryID == 0 {
		categoryID = numToUint64(row["ID"])
	}
	return img.ID, categoryID
}

func createAndOnShelfProduct(t *testing.T, srv *app.Server, merchantToken string) uint64 {
	t.Helper()
	imgID, categoryID := productImageAndCategory(t, srv, merchantToken)
	create := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/products", map[string]interface{}{
		"title": "测试商品", "description": "desc", "category_id": categoryID, "price_cent": 10000, "original_price_cent": 12000, "condition_level": "GOOD", "stock": 1, "image_file_ids": []uint64{imgID},
	}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if create.Code != 0 {
		t.Fatalf("create product failed: %+v", create)
	}
	productID := numToUint64(create.Data["product_id"])
	onShelf := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if onShelf.Code != 0 {
		t.Fatalf("on shelf failed: %+v", onShelf)
	}
	return productID
}

func createDraftProduct(t *testing.T, srv *app.Server, merchantToken string) uint64 {
	t.Helper()
	imgID, categoryID := productImageAndCategory(t, srv, merchantToken)
	create := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/products", map[string]interface{}{
		"title": "草稿商品", "description": "draft", "category_id": categoryID, "price_cent": 10000, "original_price_cent": 12000, "condition_level": "GOOD", "stock": 1, "image_file_ids": []uint64{imgID},
	}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if create.Code != 0 {
		t.Fatalf("create draft product failed: %+v", create)
	}
	return numToUint64(create.Data["product_id"])
}

func TestRestrictedLoginScope(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "restricted")

	pendingLogin := merchantLogin(t, srv, username, password)
	if pendingLogin.Code != 0 || str(pendingLogin.Data["token_scope"]) != "onboarding" {
		t.Fatalf("pending restricted login failed: %+v", pendingLogin)
	}
	pendingToken := str(pendingLogin.Data["access_token"])

	profile := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/profile", nil, map[string]string{"Authorization": "Bearer " + pendingToken})
	if profile.Code != 0 {
		t.Fatalf("pending profile should be allowed: %+v", profile)
	}
	productsDenied := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/products", nil, map[string]string{"Authorization": "Bearer " + pendingToken})
	if productsDenied.Code != 10006 {
		t.Fatalf("pending products should be denied by scope: %+v", productsDenied)
	}
	ordersDenied := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/orders", nil, map[string]string{"Authorization": "Bearer " + pendingToken})
	if ordersDenied.Code != 10006 {
		t.Fatalf("pending orders should be denied by scope: %+v", ordersDenied)
	}
	uploadDenied := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "PRODUCT_IMAGE", "file_name": "p.jpg", "file_size": 1000, "mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + pendingToken})
	if uploadDenied.Code != 10006 {
		t.Fatalf("pending product image upload should be denied: %+v", uploadDenied)
	}

	adminToken := adminAccessToken(t, srv)
	rejectMerchant(t, srv, adminToken, merchantID)

	rejectedLogin := merchantLogin(t, srv, username, password)
	if rejectedLogin.Code != 0 || str(rejectedLogin.Data["token_scope"]) != "onboarding" || str(rejectedLogin.Data["review_status"]) != "REJECTED" {
		t.Fatalf("rejected restricted login failed: %+v", rejectedLogin)
	}
	rejectedToken := str(rejectedLogin.Data["access_token"])

	reapply := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/reapply", map[string]interface{}{"contact_name": "李四"}, map[string]string{"Authorization": "Bearer " + rejectedToken})
	if reapply.Code != 0 {
		t.Fatalf("reapply should be allowed: %+v", reapply)
	}

	approveMerchant(t, srv, adminToken, merchantID)
	fullLogin := merchantLogin(t, srv, username, password)
	if fullLogin.Code != 0 || str(fullLogin.Data["token_scope"]) != "full" {
		t.Fatalf("approved full login failed: %+v", fullLogin)
	}
	fullToken := str(fullLogin.Data["access_token"])
	products := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/products", nil, map[string]string{"Authorization": "Bearer " + fullToken})
	if products.Code != 0 {
		t.Fatalf("approved products should be allowed: %+v", products)
	}
}

func TestCrossMerchantForbidden(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	m1ID, u1, p1 := registerMerchant(t, srv, "m1")
	m2ID, u2, p2 := registerMerchant(t, srv, "m2")
	approveMerchant(t, srv, adminToken, m1ID)
	approveMerchant(t, srv, adminToken, m2ID)

	m1 := merchantLogin(t, srv, u1, p1)
	m2 := merchantLogin(t, srv, u2, p2)
	m1Token := str(m1.Data["access_token"])
	m2Token := str(m2.Data["access_token"])

	productID := createAndOnShelfProduct(t, srv, m1Token)

	productDetail := requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/merchant/products/%d", productID), nil, map[string]string{"Authorization": "Bearer " + m2Token})
	if productDetail.Code != 10003 {
		t.Fatalf("cross merchant product detail should be forbidden: %+v", productDetail)
	}

	createOrder := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/orders", map[string]interface{}{"product_id": productID, "deal_price_cent": 9900}, map[string]string{"Authorization": "Bearer " + m1Token})
	if createOrder.Code != 0 {
		t.Fatalf("m1 create order failed: %+v", createOrder)
	}
	orderID := numToUint64(createOrder.Data["order_id"])

	orderDetail := requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/merchant/orders/%d", orderID), nil, map[string]string{"Authorization": "Bearer " + m2Token})
	if orderDetail.Code != 10003 {
		t.Fatalf("cross merchant order detail should be forbidden: %+v", orderDetail)
	}
}

func TestOrderConflictAndInvalidTransition(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "conflict")
	approveMerchant(t, srv, adminToken, merchantID)

	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])
	productID := createAndOnShelfProduct(t, srv, token)

	create1 := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/orders", map[string]interface{}{"product_id": productID, "deal_price_cent": 9800}, map[string]string{"Authorization": "Bearer " + token})
	if create1.Code != 0 {
		t.Fatalf("first create order failed: %+v", create1)
	}
	create2 := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/orders", map[string]interface{}{"product_id": productID, "deal_price_cent": 9700}, map[string]string{"Authorization": "Bearer " + token})
	if create2.Code != 10010 {
		t.Fatalf("second create order should conflict: %+v", create2)
	}

	orderID := numToUint64(create1.Data["order_id"])
	complete := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/complete", orderID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if complete.Code != 0 {
		t.Fatalf("complete order failed: %+v", complete)
	}
	closeAgain := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/close", orderID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if closeAgain.Code != 10005 {
		t.Fatalf("close completed order should be invalid transition: %+v", closeAgain)
	}

	onShelfAgain := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if onShelfAgain.Code != 10005 {
		t.Fatalf("sold product should not re-on-shelf: %+v", onShelfAgain)
	}
}

func TestConcurrentOrderCreateSingleActive(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "concurrent")
	approveMerchant(t, srv, adminToken, merchantID)

	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])
	productID := createAndOnShelfProduct(t, srv, token)

	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	codes := make(chan int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			resp := requestJSON(
				t,
				srv.Router,
				http.MethodPost,
				"/api/v1/merchant/orders",
				map[string]interface{}{"product_id": productID, "deal_price_cent": 9000 + idx},
				map[string]string{"Authorization": "Bearer " + token},
			)
			codes <- resp.Code
		}(i)
	}
	close(start)
	wg.Wait()
	close(codes)

	success, conflicts, transitions, locked := 0, 0, 0, 0
	for code := range codes {
		switch code {
		case 0:
			success++
		case 10010:
			conflicts++
		case 10005:
			transitions++
		case 20001:
			locked++
		default:
			t.Fatalf("unexpected response code in concurrency create: %d", code)
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly 1 success, got success=%d conflict=%d transition=%d locked=%d", success, conflicts, transitions, locked)
	}

	var activeCount int64
	if err := srv.DB.Model(&model.Order{}).Where("product_id = ? AND status = ? AND is_active = ?", productID, model.OrderCreated, true).Count(&activeCount).Error; err != nil {
		t.Fatalf("count active orders failed: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active order in db, got %d", activeCount)
	}

	var product model.Product
	if err := srv.DB.Where("id = ?", productID).First(&product).Error; err != nil {
		t.Fatalf("load product failed: %v", err)
	}
	if product.Status != model.ProductLocked || product.ActiveOrderID == nil {
		t.Fatalf("expected product locked with active order, got status=%s active_order_id=%v", product.Status, product.ActiveOrderID)
	}
}

func TestOrderProductTransactionConsistency(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "tx")
	approveMerchant(t, srv, adminToken, merchantID)

	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])

	productForComplete := createAndOnShelfProduct(t, srv, token)
	createCompleteOrder := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/orders", map[string]interface{}{"product_id": productForComplete, "deal_price_cent": 10000}, map[string]string{"Authorization": "Bearer " + token})
	if createCompleteOrder.Code != 0 {
		t.Fatalf("create order for complete failed: %+v", createCompleteOrder)
	}
	orderForComplete := numToUint64(createCompleteOrder.Data["order_id"])
	complete := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/complete", orderForComplete), map[string]interface{}{"note": "ok"}, map[string]string{"Authorization": "Bearer " + token})
	if complete.Code != 0 {
		t.Fatalf("complete failed: %+v", complete)
	}
	var completedOrder model.Order
	if err := srv.DB.Where("id = ?", orderForComplete).First(&completedOrder).Error; err != nil {
		t.Fatalf("load completed order failed: %v", err)
	}
	var completedProduct model.Product
	if err := srv.DB.Where("id = ?", productForComplete).First(&completedProduct).Error; err != nil {
		t.Fatalf("load completed product failed: %v", err)
	}
	if completedOrder.Status != model.OrderCompleted || completedOrder.IsActive {
		t.Fatalf("completed order mismatch: status=%s is_active=%v", completedOrder.Status, completedOrder.IsActive)
	}
	if completedProduct.Status != model.ProductSold || completedProduct.ActiveOrderID != nil {
		t.Fatalf("completed product mismatch: status=%s active_order_id=%v", completedProduct.Status, completedProduct.ActiveOrderID)
	}

	productForClose := createAndOnShelfProduct(t, srv, token)
	createCloseOrder := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/orders", map[string]interface{}{"product_id": productForClose, "deal_price_cent": 9900}, map[string]string{"Authorization": "Bearer " + token})
	if createCloseOrder.Code != 0 {
		t.Fatalf("create order for close failed: %+v", createCloseOrder)
	}
	orderForClose := numToUint64(createCloseOrder.Data["order_id"])
	closeResp := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/close", orderForClose), map[string]interface{}{"reason": "cancel"}, map[string]string{"Authorization": "Bearer " + token})
	if closeResp.Code != 0 {
		t.Fatalf("close failed: %+v", closeResp)
	}
	var closedOrder model.Order
	if err := srv.DB.Where("id = ?", orderForClose).First(&closedOrder).Error; err != nil {
		t.Fatalf("load closed order failed: %v", err)
	}
	var closedProduct model.Product
	if err := srv.DB.Where("id = ?", productForClose).First(&closedProduct).Error; err != nil {
		t.Fatalf("load closed product failed: %v", err)
	}
	if closedOrder.Status != model.OrderClosed || closedOrder.IsActive {
		t.Fatalf("closed order mismatch: status=%s is_active=%v", closedOrder.Status, closedOrder.IsActive)
	}
	if closedProduct.Status != model.ProductOffShelf || closedProduct.ActiveOrderID != nil {
		t.Fatalf("closed product mismatch: status=%s active_order_id=%v", closedProduct.Status, closedProduct.ActiveOrderID)
	}
}

func TestProductLifecycleStatusTransitions(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "product_lifecycle")
	approveMerchant(t, srv, adminToken, merchantID)

	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])
	productID := createDraftProduct(t, srv, token)

	onShelf := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if onShelf.Code != 0 || str(onShelf.Data["to_status"]) != model.ProductOnShelf {
		t.Fatalf("on shelf failed: %+v", onShelf)
	}

	offShelf := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/off-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if offShelf.Code != 0 || str(offShelf.Data["to_status"]) != model.ProductOffShelf {
		t.Fatalf("off shelf failed: %+v", offShelf)
	}

	onShelfAgain := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if onShelfAgain.Code != 0 || str(onShelfAgain.Data["to_status"]) != model.ProductOnShelf {
		t.Fatalf("on shelf again failed: %+v", onShelfAgain)
	}

	closeResp := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/close", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if closeResp.Code != 0 || str(closeResp.Data["to_status"]) != model.ProductClosed {
		t.Fatalf("close product failed: %+v", closeResp)
	}

	reOnShelf := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if reOnShelf.Code != 10005 {
		t.Fatalf("closed product should not re-on-shelf: %+v", reOnShelf)
	}
}

func TestProductEditRulesByStatus(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "product_edit_rules")
	approveMerchant(t, srv, adminToken, merchantID)

	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])
	productID := createDraftProduct(t, srv, token)

	updateDraft := requestJSON(
		t,
		srv.Router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/merchant/products/%d", productID),
		map[string]interface{}{"title": "草稿改名", "price_cent": 12000},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if updateDraft.Code != 0 {
		t.Fatalf("draft update should be allowed: %+v", updateDraft)
	}

	onShelf := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if onShelf.Code != 0 {
		t.Fatalf("on shelf failed: %+v", onShelf)
	}

	updateOnShelfPrice := requestJSON(
		t,
		srv.Router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/merchant/products/%d", productID),
		map[string]interface{}{"price_cent": 13000},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if updateOnShelfPrice.Code != 10005 {
		t.Fatalf("on-shelf price update should be denied: %+v", updateOnShelfPrice)
	}

	updateOnShelfDescription := requestJSON(
		t,
		srv.Router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/merchant/products/%d", productID),
		map[string]interface{}{"description": "on shelf desc"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if updateOnShelfDescription.Code != 0 {
		t.Fatalf("on-shelf description update should be allowed: %+v", updateOnShelfDescription)
	}

	closeResp := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/close", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + token})
	if closeResp.Code != 0 {
		t.Fatalf("close product failed: %+v", closeResp)
	}

	updateClosed := requestJSON(
		t,
		srv.Router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/merchant/products/%d", productID),
		map[string]interface{}{"description": "closed desc"},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if updateClosed.Code != 10005 {
		t.Fatalf("closed product update should be denied: %+v", updateClosed)
	}
}

func TestProductDeleteRemovesImageRecords(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "product_delete")
	approveMerchant(t, srv, adminToken, merchantID)

	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])
	productID := createDraftProduct(t, srv, token)

	var productImages []model.ProductImage
	if err := srv.DB.Where("product_id = ?", productID).Find(&productImages).Error; err != nil {
		t.Fatalf("query product images failed: %v", err)
	}
	if len(productImages) == 0 {
		t.Fatalf("expected product images for product %d", productID)
	}
	fileID := productImages[0].FileID

	del := requestJSON(
		t,
		srv.Router,
		http.MethodDelete,
		fmt.Sprintf("/api/v1/merchant/products/%d", productID),
		nil,
		map[string]string{"Authorization": "Bearer " + token},
	)
	if del.Code != 0 {
		t.Fatalf("delete product failed: %+v", del)
	}

	detail := requestJSON(
		t,
		srv.Router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/merchant/products/%d", productID),
		nil,
		map[string]string{"Authorization": "Bearer " + token},
	)
	if detail.Code != 10004 {
		t.Fatalf("deleted product detail should be not found: %+v", detail)
	}

	var deletedProduct model.Product
	if err := srv.DB.Unscoped().Where("id = ?", productID).First(&deletedProduct).Error; err != nil {
		t.Fatalf("load deleted product failed: %v", err)
	}
	if !deletedProduct.DeletedAt.Valid {
		t.Fatalf("expected deleted_at set for product %d", productID)
	}

	var imageCount int64
	if err := srv.DB.Model(&model.ProductImage{}).Where("product_id = ?", productID).Count(&imageCount).Error; err != nil {
		t.Fatalf("count product images failed: %v", err)
	}
	if imageCount != 0 {
		t.Fatalf("expected product images removed, got %d", imageCount)
	}

	var fileCount int64
	if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Count(&fileCount).Error; err != nil {
		t.Fatalf("count file records failed: %v", err)
	}
	if fileCount != 0 {
		t.Fatalf("expected file record removed, got %d", fileCount)
	}
}

func TestProductDeleteDeniedWhenOnShelf(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "product_delete_deny")
	approveMerchant(t, srv, adminToken, merchantID)

	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])
	productID := createDraftProduct(t, srv, token)

	onShelf := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", productID),
		map[string]interface{}{},
		map[string]string{"Authorization": "Bearer " + token},
	)
	if onShelf.Code != 0 {
		t.Fatalf("on shelf failed: %+v", onShelf)
	}

	del := requestJSON(
		t,
		srv.Router,
		http.MethodDelete,
		fmt.Sprintf("/api/v1/merchant/products/%d", productID),
		nil,
		map[string]string{"Authorization": "Bearer " + token},
	)
	if del.Code != 10005 {
		t.Fatalf("on shelf product delete should be denied: %+v", del)
	}
}

func TestDashboardStatsExcludeDeletedProducts(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "dashboard_deleted")
	approveMerchant(t, srv, adminToken, merchantID)

	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])

	keepProductID := createDraftProduct(t, srv, token)
	deleteProductID := createDraftProduct(t, srv, token)

	del := requestJSON(
		t,
		srv.Router,
		http.MethodDelete,
		fmt.Sprintf("/api/v1/merchant/products/%d", deleteProductID),
		nil,
		map[string]string{"Authorization": "Bearer " + token},
	)
	if del.Code != 0 {
		t.Fatalf("delete product failed: %+v", del)
	}

	dashboard := requestJSON(
		t,
		srv.Router,
		http.MethodGet,
		"/api/v1/merchant/dashboard",
		nil,
		map[string]string{"Authorization": "Bearer " + token},
	)
	if dashboard.Code != 0 {
		t.Fatalf("dashboard request failed: %+v", dashboard)
	}

	productStats, ok := dashboard.Data["product_stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("dashboard product_stats format invalid: %+v", dashboard.Data["product_stats"])
	}
	draftCnt, ok := productStats["draft"].(float64)
	if !ok {
		t.Fatalf("dashboard draft count missing: %+v", productStats)
	}
	if int64(draftCnt) != 1 {
		t.Fatalf("deleted products should be excluded from dashboard stats, expected draft=1 got=%v (keep=%d deleted=%d)", draftCnt, keepProductID, deleteProductID)
	}
}

func TestDashboardOnShelfTotalAmount(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "dashboard_amount")
	approveMerchant(t, srv, adminToken, merchantID)

	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])

	_ = createAndOnShelfProduct(t, srv, token)
	_ = createAndOnShelfProduct(t, srv, token)
	_ = createDraftProduct(t, srv, token)

	dashboard := requestJSON(
		t,
		srv.Router,
		http.MethodGet,
		"/api/v1/merchant/dashboard",
		nil,
		map[string]string{"Authorization": "Bearer " + token},
	)
	if dashboard.Code != 0 {
		t.Fatalf("dashboard request failed: %+v", dashboard)
	}

	onShelfAmount, ok := dashboard.Data["on_shelf_total_amount_cent"].(float64)
	if !ok {
		t.Fatalf("dashboard on_shelf_total_amount_cent missing: %+v", dashboard.Data)
	}
	if int64(onShelfAmount) != 20000 {
		t.Fatalf("on_shelf_total_amount_cent mismatch: expected=20000 got=%v", onShelfAmount)
	}
}
