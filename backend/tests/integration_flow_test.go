package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
)

type apiResp struct {
	Code int                    `json:"code"`
	Data map[string]interface{} `json:"data"`
}

func newTestServer(t *testing.T) *app.Server {
	t.Helper()
	cfg := app.Config{
		Addr:                ":0",
		DBDriver:            "sqlite",
		DBDSN:               fmt.Sprintf("file:test_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		JWTAccessSecret:     "test-access",
		JWTRefreshSecret:    "test-refresh",
		AccessTTL:           app.LoadConfig().AccessTTL,
		RefreshTTL:          app.LoadConfig().RefreshTTL,
		AutoMigrate:         true,
		FileStorageProvider: "local",
		FileUploadLocalDir:  t.TempDir(),
	}
	srv, err := app.NewServer(cfg)
	if err != nil {
		t.Fatalf("new server error: %v", err)
	}
	return srv
}

func requestJSON(t *testing.T, h http.Handler, method, path string, body interface{}, headers map[string]string) apiResp {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp apiResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v, raw=%s", err, w.Body.String())
	}
	return resp
}

func str(v interface{}) string {
	s, _ := v.(string)
	return s
}

func numToUint64(v interface{}) uint64 {
	f, _ := v.(float64)
	return uint64(f)
}

func TestMainFlow_RegisterApproveLoginProductOrder(t *testing.T) {
	srv := newTestServer(t)

	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": 1000, "mime_type": "image/jpeg",
	}, nil)
	if presign.Code != 0 {
		t.Fatalf("presign failed: %+v", presign)
	}
	licenseFileID := numToUint64(presign.Data["file_id"])

	register := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register", map[string]interface{}{
		"merchant_name": "测试商家", "contact_name": "张三", "phone": "13800138000", "username": "merchant1", "password": "Passw0rd!2026", "license_file_id": licenseFileID,
	}, nil)
	if register.Code != 0 {
		t.Fatalf("register failed: %+v", register)
	}
	merchantID := numToUint64(register.Data["merchant_id"])

	loginPending := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{"login_type": "MERCHANT", "username": "merchant1", "password": "Passw0rd!2026"}, nil)
	if loginPending.Code != 0 || str(loginPending.Data["token_scope"]) != "onboarding" {
		t.Fatalf("pending login scope mismatch: %+v", loginPending)
	}
	pendingToken := str(loginPending.Data["access_token"])

	profile := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/profile", nil, map[string]string{"Authorization": "Bearer " + pendingToken})
	if profile.Code != 0 || str(profile.Data["review_status"]) != "PENDING" {
		t.Fatalf("profile pending mismatch: %+v", profile)
	}

	adminLogin := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{"login_type": "ADMIN", "username": "admin", "password": "Admin@123456"}, nil)
	if adminLogin.Code != 0 {
		t.Fatalf("admin login failed: %+v", adminLogin)
	}
	adminToken := str(adminLogin.Data["access_token"])

	approve := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/admin/merchants/%d/approve", merchantID), map[string]interface{}{"comment": "ok"}, map[string]string{"Authorization": "Bearer " + adminToken})
	if approve.Code != 0 {
		t.Fatalf("approve failed: %+v", approve)
	}

	loginFull := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{"login_type": "MERCHANT", "username": "merchant1", "password": "Passw0rd!2026"}, nil)
	if loginFull.Code != 0 || str(loginFull.Data["token_scope"]) != "full" {
		t.Fatalf("full login failed: %+v", loginFull)
	}
	merchantToken := str(loginFull.Data["access_token"])

	imgPresign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "PRODUCT_IMAGE", "file_name": "p1.jpg", "file_size": 1000, "mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if imgPresign.Code != 0 {
		t.Fatalf("img presign failed: %+v", imgPresign)
	}
	imgID := numToUint64(imgPresign.Data["file_id"])

	categories := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/categories?level=2", nil, map[string]string{"Authorization": "Bearer " + merchantToken})
	if categories.Code != 0 {
		t.Fatalf("categories failed: %+v", categories)
	}
	items, ok := categories.Data["items"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("category items empty: %+v", categories)
	}
	first := items[0].(map[string]interface{})
	categoryID := numToUint64(first["id"])
	if categoryID == 0 {
		categoryID = numToUint64(first["ID"])
	}

	product := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/products", map[string]interface{}{
		"title": "iPhone 14", "description": "正常", "category_id": categoryID, "price_cent": 320000, "condition_level": "GOOD", "stock": 1, "image_file_ids": []uint64{imgID},
	}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if product.Code != 0 {
		t.Fatalf("create product failed: %+v", product)
	}
	productID := numToUint64(product.Data["product_id"])

	onShelf := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", productID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + merchantToken, "Idempotency-Key": "k1"})
	if onShelf.Code != 0 || str(onShelf.Data["to_status"]) != "ON_SHELF" {
		t.Fatalf("on shelf failed: %+v", onShelf)
	}

	createOrder := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/orders", map[string]interface{}{
		"product_id": productID, "deal_price_cent": 315000,
	}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if createOrder.Code != 0 || str(createOrder.Data["product_status"]) != "LOCKED" {
		t.Fatalf("create order failed: %+v", createOrder)
	}
	orderID := numToUint64(createOrder.Data["order_id"])

	complete := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/complete", orderID), map[string]interface{}{"note": "done"}, map[string]string{"Authorization": "Bearer " + merchantToken, "Idempotency-Key": "k2"})
	if complete.Code != 0 || str(complete.Data["to_status"]) != "COMPLETED" || str(complete.Data["product_status"]) != "SOLD" {
		t.Fatalf("complete order failed: %+v", complete)
	}

	completeAgain := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/orders/%d/complete", orderID), map[string]interface{}{"note": "done"}, map[string]string{"Authorization": "Bearer " + merchantToken, "Idempotency-Key": "k2"})
	if completeAgain.Code != 0 {
		t.Fatalf("idempotent complete failed: %+v", completeAgain)
	}
	if v, ok := completeAgain.Data["idempotent"].(bool); !ok || !v {
		t.Fatalf("idempotent flag missing: %+v", completeAgain)
	}

	merchantLogs := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/logs", nil, map[string]string{"Authorization": "Bearer " + merchantToken})
	if merchantLogs.Code != 0 {
		t.Fatalf("merchant logs failed: %+v", merchantLogs)
	}
}
