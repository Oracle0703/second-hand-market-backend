package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func buyerLogin(t *testing.T, srv interface{ Router() http.Handler }, code, deviceID string) apiResp {
	t.Helper()
	return requestJSON(t, srv.Router(), http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code":      code,
		"device_id": deviceID,
		"nickname":  "buyer-" + code,
	}, nil)
}

// adapter keeps helper ergonomic without changing existing test helpers.
type serverAdapter struct{ h http.Handler }

func (s serverAdapter) Router() http.Handler { return s.h }

func TestBuyerAuthRefreshLogout(t *testing.T) {
	srv := newTestServer(t)
	headers := map[string]string{"X-Device-Id": "dev-auth-001"}

	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "wx-auth-001", "device_id": "dev-auth-001", "nickname": "buyer-auth",
	}, nil)
	if login.Code != 0 {
		t.Fatalf("buyer login failed: %+v", login)
	}
	access := str(login.Data["access_token"])
	refresh := str(login.Data["refresh_token"])

	summary := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/me/summary", nil, map[string]string{"Authorization": "Bearer " + access, "X-Device-Id": "dev-auth-001"})
	if summary.Code != 0 {
		t.Fatalf("buyer summary failed: %+v", summary)
	}
	isLogin, _ := summary.Data["is_login"].(bool)
	if !isLogin {
		t.Fatalf("summary should be login state: %+v", summary)
	}

	refreshResp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/refresh", map[string]interface{}{"refresh_token": refresh}, headers)
	if refreshResp.Code != 0 {
		t.Fatalf("buyer refresh failed: %+v", refreshResp)
	}

	logout := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/logout", map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + access})
	if logout.Code != 0 {
		t.Fatalf("buyer logout failed: %+v", logout)
	}

	refreshAfterLogout := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/refresh", map[string]interface{}{"refresh_token": refresh}, headers)
	if refreshAfterLogout.Code != 10002 {
		t.Fatalf("refresh after logout should be unauthorized: %+v", refreshAfterLogout)
	}
}

func TestBuyerGuestProductsBrowse(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyerbrowse")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	headers := map[string]string{"X-Device-Id": "dev-browse-001"}
	list := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/products", nil, headers)
	if list.Code != 0 {
		t.Fatalf("buyer guest products list failed: %+v", list)
	}
	detail := requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/buyer/products/%d", productID), nil, headers)
	if detail.Code != 0 {
		t.Fatalf("buyer guest products detail failed: %+v", detail)
	}
}

func TestBuyerFavoritesCRUD(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyerfav")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	headers := map[string]string{"X-Device-Id": "dev-fav-001"}
	add := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/favorites", map[string]interface{}{"product_id": productID}, headers)
	if add.Code != 0 {
		t.Fatalf("favorite add failed: %+v", add)
	}

	list1 := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/favorites", nil, headers)
	if list1.Code != 0 {
		t.Fatalf("favorite list failed: %+v", list1)
	}
	items1, ok := list1.Data["items"].([]interface{})
	if !ok || len(items1) != 1 {
		t.Fatalf("favorite list should have one item: %+v", list1)
	}

	del := requestJSON(t, srv.Router, http.MethodDelete, fmt.Sprintf("/api/v1/buyer/favorites/%d", productID), nil, headers)
	if del.Code != 0 {
		t.Fatalf("favorite delete failed: %+v", del)
	}

	list2 := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/favorites", nil, headers)
	if list2.Code != 0 {
		t.Fatalf("favorite list2 failed: %+v", list2)
	}
	items2, _ := list2.Data["items"].([]interface{})
	if len(items2) != 0 {
		t.Fatalf("favorite list should be empty after delete: %+v", list2)
	}
}

func TestBuyerHistoriesDedupAndClear(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyerhis")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	headers := map[string]string{"X-Device-Id": "dev-his-001"}
	v1 := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/histories/views", map[string]interface{}{"product_id": productID}, headers)
	if v1.Code != 0 || numToUint64(v1.Data["view_count"]) != 1 {
		t.Fatalf("first view failed: %+v", v1)
	}
	v2 := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/histories/views", map[string]interface{}{"product_id": productID}, headers)
	if v2.Code != 0 || numToUint64(v2.Data["view_count"]) != 1 {
		t.Fatalf("second view should be deduped in 30s: %+v", v2)
	}
	v3 := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/histories/views", map[string]interface{}{
		"product_id": productID,
		"viewed_at":  time.Now().Add(31 * time.Second).Format(time.RFC3339),
	}, headers)
	if v3.Code != 0 || numToUint64(v3.Data["view_count"]) != 2 {
		t.Fatalf("third view should increase count: %+v", v3)
	}

	list := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/histories", nil, headers)
	if list.Code != 0 {
		t.Fatalf("history list failed: %+v", list)
	}
	items, ok := list.Data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("history list should have one item: %+v", list)
	}
	first := items[0].(map[string]interface{})
	if numToUint64(first["view_count"]) != 2 {
		t.Fatalf("history view_count mismatch: %+v", list)
	}

	clear := requestJSON(t, srv.Router, http.MethodDelete, "/api/v1/buyer/histories", nil, headers)
	if clear.Code != 0 {
		t.Fatalf("clear histories failed: %+v", clear)
	}
	listAfter := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/histories", nil, headers)
	if listAfter.Code != 0 {
		t.Fatalf("history list after clear failed: %+v", listAfter)
	}
	itemsAfter, _ := listAfter.Data["items"].([]interface{})
	if len(itemsAfter) != 0 {
		t.Fatalf("histories should be empty after clear: %+v", listAfter)
	}
}

func TestBuyerGuestMergeIdempotent(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyermerge")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	guestHeaders := map[string]string{"X-Device-Id": "dev-merge-001"}
	fav := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/favorites", map[string]interface{}{"product_id": productID}, guestHeaders)
	if fav.Code != 0 {
		t.Fatalf("guest favorite failed: %+v", fav)
	}
	view := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/histories/views", map[string]interface{}{"product_id": productID}, guestHeaders)
	if view.Code != 0 {
		t.Fatalf("guest history failed: %+v", view)
	}

	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "wx-merge-001", "device_id": "dev-merge-001", "nickname": "buyer-merge",
	}, nil)
	if login.Code != 0 {
		t.Fatalf("buyer login failed: %+v", login)
	}
	buyerToken := str(login.Data["access_token"])

	merge1 := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/guest/merge", map[string]interface{}{"device_id": "dev-merge-001"}, map[string]string{"Authorization": "Bearer " + buyerToken})
	if merge1.Code != 0 {
		t.Fatalf("merge1 failed: %+v", merge1)
	}
	merged := merge1.Data["merged"].(map[string]interface{})
	if numToUint64(merged["favorites_count"]) != 1 || numToUint64(merged["histories_count"]) != 1 {
		t.Fatalf("merge1 counts mismatch: %+v", merge1)
	}

	merge2 := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/guest/merge", map[string]interface{}{"device_id": "dev-merge-001"}, map[string]string{"Authorization": "Bearer " + buyerToken})
	if merge2.Code != 0 {
		t.Fatalf("merge2 failed: %+v", merge2)
	}
	merged2 := merge2.Data["merged"].(map[string]interface{})
	if numToUint64(merged2["favorites_count"]) != 0 || numToUint64(merged2["histories_count"]) != 0 {
		t.Fatalf("merge should be idempotent: %+v", merge2)
	}
}

func TestBuyerIntentCreateConflictAndMerchantStatusFlow(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyerintent")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	buyerLoginResp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "wx-intent-001", "device_id": "dev-intent-001", "nickname": "buyer-intent",
	}, nil)
	if buyerLoginResp.Code != 0 {
		t.Fatalf("buyer login failed: %+v", buyerLoginResp)
	}
	buyerToken := str(buyerLoginResp.Data["access_token"])

	create1 := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/intents", map[string]interface{}{
		"product_id":    productID,
		"contact_phone": "13800138000",
		"message":       "有意向，方便联系",
	}, map[string]string{"Authorization": "Bearer " + buyerToken, "X-Device-Id": "dev-intent-001"})
	if create1.Code != 0 {
		t.Fatalf("create intent failed: %+v", create1)
	}
	intentID := numToUint64(create1.Data["intent_id"])

	createConflict := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/intents", map[string]interface{}{
		"product_id":    productID,
		"contact_phone": "13800138000",
	}, map[string]string{"Authorization": "Bearer " + buyerToken, "X-Device-Id": "dev-intent-001"})
	if createConflict.Code != 10010 {
		t.Fatalf("same buyer same product open intent should conflict: %+v", createConflict)
	}

	merchantList := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/intents", nil, map[string]string{"Authorization": "Bearer " + merchantToken})
	if merchantList.Code != 0 {
		t.Fatalf("merchant intent list failed: %+v", merchantList)
	}

	markContacted := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/intents/%d/contacted", intentID), map[string]interface{}{}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if markContacted.Code != 0 {
		t.Fatalf("merchant contacted failed: %+v", markContacted)
	}

	buyerList := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/intents", nil, map[string]string{"Authorization": "Bearer " + buyerToken})
	if buyerList.Code != 0 {
		t.Fatalf("buyer intent list failed: %+v", buyerList)
	}
	listItems := buyerList.Data["items"].([]interface{})
	if len(listItems) == 0 {
		t.Fatalf("buyer intent list should not be empty: %+v", buyerList)
	}
	if listItems[0].(map[string]interface{})["buyer_status_text"] != "已联系" {
		t.Fatalf("buyer status text should map CONTACTED to 已联系: %+v", buyerList)
	}

	closeIntent := requestJSON(t, srv.Router, http.MethodPost, fmt.Sprintf("/api/v1/merchant/intents/%d/close", intentID), map[string]interface{}{"reason": "NO_RESPONSE"}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if closeIntent.Code != 0 {
		t.Fatalf("merchant close intent failed: %+v", closeIntent)
	}

	buyerDetail := requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/buyer/intents/%d", intentID), nil, map[string]string{"Authorization": "Bearer " + buyerToken})
	if buyerDetail.Code != 0 {
		t.Fatalf("buyer intent detail failed: %+v", buyerDetail)
	}
	intentDetail := buyerDetail.Data["intent"].(map[string]interface{})
	if intentDetail["status"] != "CLOSED" || intentDetail["buyer_status_text"] != "已关闭" {
		t.Fatalf("buyer detail status mapping mismatch: %+v", buyerDetail)
	}

	createAfterClose := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/intents", map[string]interface{}{
		"product_id":     productID,
		"contact_wechat": "wx_after_close",
	}, map[string]string{"Authorization": "Bearer " + buyerToken, "X-Device-Id": "dev-intent-001"})
	if createAfterClose.Code != 0 {
		t.Fatalf("should allow create after previous intent closed: %+v", createAfterClose)
	}
}

func TestBuyerIntentInvalidProductStatusAndPrivilegeBoundary(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyerguard")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	draftProductID := createDraftProduct(t, srv, merchantToken)

	buyerLoginResp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "wx-guard-001", "device_id": "dev-guard-001", "nickname": "buyer-guard",
	}, nil)
	if buyerLoginResp.Code != 0 {
		t.Fatalf("buyer login failed: %+v", buyerLoginResp)
	}
	buyerToken := str(buyerLoginResp.Data["access_token"])

	invalidStatus := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/intents", map[string]interface{}{
		"product_id":    draftProductID,
		"contact_phone": "13900139000",
	}, map[string]string{"Authorization": "Bearer " + buyerToken, "X-Device-Id": "dev-guard-001"})
	if invalidStatus.Code != 10005 {
		t.Fatalf("intent on non ON_SHELF product should fail with 10005: %+v", invalidStatus)
	}

	buyerCallMerchant := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/products", nil, map[string]string{"Authorization": "Bearer " + buyerToken})
	if buyerCallMerchant.Code != 10003 {
		t.Fatalf("buyer token should not access merchant api: %+v", buyerCallMerchant)
	}
	buyerCallAdmin := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/admin/merchants", nil, map[string]string{"Authorization": "Bearer " + buyerToken})
	if buyerCallAdmin.Code != 10003 {
		t.Fatalf("buyer token should not access admin api: %+v", buyerCallAdmin)
	}
}
