package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/model"
)

func buyerLogin(t *testing.T, srv interface{ Router() http.Handler }, code, deviceID string) apiResp {
	t.Helper()
	return requestJSON(t, srv.Router(), http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code":      code,
		"device_id": deviceID,
		"nickname":  "buyer-" + code,
	}, nil)
}

func buyerMiniappLogin(t *testing.T, srv interface{ Router() http.Handler }, provider, code, deviceID string) apiResp {
	t.Helper()
	return requestJSON(t, srv.Router(), http.MethodPost, "/api/v1/buyer/auth/miniapp-login", map[string]interface{}{
		"provider":  provider,
		"code":      code,
		"device_id": deviceID,
		"nickname":  provider + "-" + code,
	}, nil)
}

// adapter keeps helper ergonomic without changing existing test helpers.
type serverAdapter struct{ h http.Handler }

func (s serverAdapter) Router() http.Handler { return s.h }

func merchantNoByID(t *testing.T, srv *app.Server, merchantID uint64) string {
	t.Helper()
	var merchant model.Merchant
	if err := srv.DB.First(&merchant, merchantID).Error; err != nil {
		t.Fatalf("load merchant number: %v", err)
	}
	return merchant.MerchantNo
}

func withMerchantNo(path string, merchantNo string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "merchant_no=" + merchantNo
}

func TestBuyerAuthRefreshLogout(t *testing.T) {
	srv := newTestServer(t)
	merchantID, _, _ := registerMerchant(t, srv, "buyer-auth-summary")
	merchantNo := merchantNoByID(t, srv, merchantID)
	headers := map[string]string{"X-Device-Id": "dev-auth-001"}

	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "wx-auth-001", "device_id": "dev-auth-001", "nickname": "buyer-auth",
	}, nil)
	if login.Code != 0 {
		t.Fatalf("buyer login failed: %+v", login)
	}
	access := str(login.Data["access_token"])
	refresh := str(login.Data["refresh_token"])

	summary := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/me/summary", merchantNo), nil, map[string]string{"Authorization": "Bearer " + access, "X-Device-Id": "dev-auth-001"})
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

func TestBuyerMiniappLoginSupportsDouyinAndProviderIsolation(t *testing.T) {
	srv := newTestServer(t)

	douyinLogin := buyerMiniappLogin(t, serverAdapter{h: srv.Router}, "douyin", "shared-code-001", "dev-tt-001")
	if douyinLogin.Code != 0 {
		t.Fatalf("douyin miniapp login failed: %+v", douyinLogin)
	}
	douyinUser, _ := douyinLogin.Data["user"].(map[string]interface{})
	if str(douyinUser["auth_provider"]) != "douyin" {
		t.Fatalf("douyin auth_provider mismatch: %+v", douyinLogin)
	}

	douyinAgain := buyerMiniappLogin(t, serverAdapter{h: srv.Router}, "douyin", "shared-code-001", "dev-tt-002")
	if douyinAgain.Code != 0 {
		t.Fatalf("repeat douyin miniapp login failed: %+v", douyinAgain)
	}
	if numToUint64(douyinUser["id"]) != numToUint64(douyinAgain.Data["user"].(map[string]interface{})["id"]) {
		t.Fatalf("same douyin code should map to same buyer: first=%+v second=%+v", douyinLogin, douyinAgain)
	}

	wechatLogin := buyerMiniappLogin(t, serverAdapter{h: srv.Router}, "wechat", "shared-code-001", "dev-wx-001")
	if wechatLogin.Code != 0 {
		t.Fatalf("wechat miniapp login failed: %+v", wechatLogin)
	}
	wechatUser, _ := wechatLogin.Data["user"].(map[string]interface{})
	if str(wechatUser["auth_provider"]) != "wechat" {
		t.Fatalf("wechat auth_provider mismatch: %+v", wechatLogin)
	}
	if numToUint64(douyinUser["id"]) == numToUint64(wechatUser["id"]) {
		t.Fatalf("same code across providers should not reuse buyer: douyin=%+v wechat=%+v", douyinLogin, wechatLogin)
	}
}

func TestBuyerGuestProductsBrowse(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyerbrowse")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	merchantNo := merchantNoByID(t, srv, merchantID)
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	headers := map[string]string{"X-Device-Id": "dev-browse-001"}
	list := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/products", merchantNo), nil, headers)
	if list.Code != 0 {
		t.Fatalf("buyer guest products list failed: %+v", list)
	}
	listItems, ok := list.Data["items"].([]interface{})
	if !ok || len(listItems) == 0 {
		t.Fatalf("buyer guest products list should not be empty: %+v", list)
	}
	firstListItem := listItems[0].(map[string]interface{})
	if numToUint64(firstListItem["stock"]) == 0 {
		t.Fatalf("buyer products list stock should be returned: %+v", list)
	}
	if numToUint64(firstListItem["original_price_cent"]) == 0 {
		t.Fatalf("buyer products list original_price_cent should be returned: %+v", list)
	}
	if str(firstListItem["cover_url"]) == "" {
		t.Fatalf("buyer products list cover_url should be returned: %+v", list)
	}
	detail := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo(fmt.Sprintf("/api/v1/buyer/products/%d", productID), merchantNo), nil, headers)
	if detail.Code != 0 {
		t.Fatalf("buyer guest products detail failed: %+v", detail)
	}
	detailProduct, ok := detail.Data["product"].(map[string]interface{})
	if !ok {
		t.Fatalf("buyer product detail missing product: %+v", detail)
	}
	if numToUint64(detailProduct["stock"]) == 0 {
		t.Fatalf("buyer product detail stock should be returned: %+v", detail)
	}
	if numToUint64(detailProduct["original_price_cent"]) == 0 {
		t.Fatalf("buyer product detail original_price_cent should be returned: %+v", detail)
	}
}

func TestBuyerProductsAndCategoriesRequireMerchantNoAndStayScoped(t *testing.T) {
	srv := newTestServer(t)
	merchantOneID, merchantOneUser, merchantOnePassword := registerMerchant(t, srv, "buyer_scope_1")
	merchantTwoID, merchantTwoUser, merchantTwoPassword := registerMerchant(t, srv, "buyer_scope_2")
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

	productOneID := createAndOnShelfProduct(t, srv, tokenOne)
	productTwoID := createAndOnShelfProduct(t, srv, tokenTwo)

	missing := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/products", nil, nil)
	if missing.Code == 0 {
		t.Fatal("buyer products without merchant_no succeeded")
	}

	listOne := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/products?merchant_no="+merchantOneNo, nil, nil)
	if listOne.Code != 0 {
		t.Fatalf("buyer scoped list code = %d data=%v", listOne.Code, listOne.Data)
	}
	items := listOne.Data["items"].([]interface{})
	if len(items) != 1 || numToUint64(items[0].(map[string]interface{})["id"]) != productOneID {
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
	categoryItems := categories.Data["items"].([]interface{})
	if len(categoryItems) != 3 {
		t.Fatalf("merchant two categories = %d, want 3", len(categoryItems))
	}
	for _, item := range categoryItems {
		row := item.(map[string]interface{})
		if got := numToUint64(row["merchant_id"]); got != merchantTwoID {
			t.Fatalf("buyer category merchant_id = %d, want %d row=%+v", got, merchantTwoID, row)
		}
	}
}

func TestBuyerCategoriesUseLowercaseFields(t *testing.T) {
	srv := newTestServer(t)
	merchantID, _, _ := registerMerchant(t, srv, "buyer_categories")
	merchantNo := merchantNoByID(t, srv, merchantID)
	resp := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/categories?level=1", merchantNo), nil, map[string]string{"X-Device-Id": "dev-cat-001"})
	if resp.Code != 0 {
		t.Fatalf("buyer categories failed: %+v", resp)
	}
	items, ok := resp.Data["items"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("buyer categories should not be empty: %+v", resp)
	}
	first, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("buyer categories item invalid: %+v", resp)
	}
	if _, exists := first["id"]; !exists {
		t.Fatalf("buyer categories should use lowercase id: %+v", resp)
	}
	if _, exists := first["name"]; !exists {
		t.Fatalf("buyer categories should use lowercase name: %+v", resp)
	}
	if _, exists := first["level"]; !exists {
		t.Fatalf("buyer categories should use lowercase level: %+v", resp)
	}
}

func TestBuyerFavoritesCRUD(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyerfav")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	merchantNo := merchantNoByID(t, srv, merchantID)
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	headers := map[string]string{"X-Device-Id": "dev-fav-001"}
	add := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/favorites", merchantNo), map[string]interface{}{"product_id": productID}, headers)
	if add.Code != 0 {
		t.Fatalf("favorite add failed: %+v", add)
	}

	list1 := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/favorites", merchantNo), nil, headers)
	if list1.Code != 0 {
		t.Fatalf("favorite list failed: %+v", list1)
	}
	items1, ok := list1.Data["items"].([]interface{})
	if !ok || len(items1) != 1 {
		t.Fatalf("favorite list should have one item: %+v", list1)
	}
	firstFavorite := items1[0].(map[string]interface{})
	if numToUint64(firstFavorite["stock"]) == 0 {
		t.Fatalf("favorite list stock should be returned: %+v", list1)
	}
	if numToUint64(firstFavorite["original_price_cent"]) == 0 {
		t.Fatalf("favorite list original_price_cent should be returned: %+v", list1)
	}

	del := requestJSON(t, srv.Router, http.MethodDelete, withMerchantNo(fmt.Sprintf("/api/v1/buyer/favorites/%d", productID), merchantNo), nil, headers)
	if del.Code != 0 {
		t.Fatalf("favorite delete failed: %+v", del)
	}

	list2 := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/favorites", merchantNo), nil, headers)
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
	merchantNo := merchantNoByID(t, srv, merchantID)
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	headers := map[string]string{"X-Device-Id": "dev-his-001"}
	v1 := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/histories/views", merchantNo), map[string]interface{}{"product_id": productID}, headers)
	if v1.Code != 0 || numToUint64(v1.Data["view_count"]) != 1 {
		t.Fatalf("first view failed: %+v", v1)
	}
	v2 := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/histories/views", merchantNo), map[string]interface{}{"product_id": productID}, headers)
	if v2.Code != 0 || numToUint64(v2.Data["view_count"]) != 1 {
		t.Fatalf("second view should be deduped in 30s: %+v", v2)
	}
	v3 := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/histories/views", merchantNo), map[string]interface{}{
		"product_id": productID,
		"viewed_at":  time.Now().Add(31 * time.Second).Format(time.RFC3339),
	}, headers)
	if v3.Code != 0 || numToUint64(v3.Data["view_count"]) != 2 {
		t.Fatalf("third view should increase count: %+v", v3)
	}

	list := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/histories", merchantNo), nil, headers)
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
	if numToUint64(first["stock"]) == 0 {
		t.Fatalf("history stock should be returned: %+v", list)
	}
	if numToUint64(first["original_price_cent"]) == 0 {
		t.Fatalf("history original_price_cent should be returned: %+v", list)
	}

	clear := requestJSON(t, srv.Router, http.MethodDelete, withMerchantNo("/api/v1/buyer/histories", merchantNo), nil, headers)
	if clear.Code != 0 {
		t.Fatalf("clear histories failed: %+v", clear)
	}
	listAfter := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/histories", merchantNo), nil, headers)
	if listAfter.Code != 0 {
		t.Fatalf("history list after clear failed: %+v", listAfter)
	}
	itemsAfter, _ := listAfter.Data["items"].([]interface{})
	if len(itemsAfter) != 0 {
		t.Fatalf("histories should be empty after clear: %+v", listAfter)
	}
}

func TestBuyerHistoryClearSingleProductKeepsOtherMerchantScopedRecords(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyerhisone")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	merchantNo := merchantNoByID(t, srv, merchantID)
	productOneID := createAndOnShelfProduct(t, srv, merchantToken)
	productTwoID := createAndOnShelfProduct(t, srv, merchantToken)

	headers := map[string]string{"X-Device-Id": "dev-his-single-001"}
	for _, productID := range []uint64{productOneID, productTwoID} {
		view := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/histories/views", merchantNo), map[string]interface{}{"product_id": productID}, headers)
		if view.Code != 0 {
			t.Fatalf("history view failed: %+v", view)
		}
	}

	clearOne := requestJSON(t, srv.Router, http.MethodDelete, withMerchantNo(fmt.Sprintf("/api/v1/buyer/histories?product_id=%d", productOneID), merchantNo), nil, headers)
	if clearOne.Code != 0 {
		t.Fatalf("clear single history failed: %+v", clearOne)
	}
	list := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/histories", merchantNo), nil, headers)
	if list.Code != 0 {
		t.Fatalf("history list failed: %+v", list)
	}
	items := list.Data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("history list should keep one item after clearing a single product: %+v", list)
	}
	if got := numToUint64(items[0].(map[string]interface{})["product_id"]); got != productTwoID {
		t.Fatalf("remaining history product_id = %d, want %d: %+v", got, productTwoID, list)
	}
}

func TestBuyerSummaryRequiresMerchantNoAndCountsCurrentMerchant(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantOneID, merchantOneUser, merchantOnePassword := registerMerchant(t, srv, "buyersummary1")
	merchantTwoID, merchantTwoUser, merchantTwoPassword := registerMerchant(t, srv, "buyersummary2")
	approveMerchant(t, srv, adminToken, merchantOneID)
	approveMerchant(t, srv, adminToken, merchantTwoID)
	merchantOne := merchantLogin(t, srv, merchantOneUser, merchantOnePassword)
	merchantTwo := merchantLogin(t, srv, merchantTwoUser, merchantTwoPassword)
	merchantOneToken := str(merchantOne.Data["access_token"])
	merchantTwoToken := str(merchantTwo.Data["access_token"])
	merchantOneNo := merchantNoByID(t, srv, merchantOneID)
	merchantTwoNo := merchantNoByID(t, srv, merchantTwoID)
	productOneID := createAndOnShelfProduct(t, srv, merchantOneToken)
	productTwoID := createAndOnShelfProduct(t, srv, merchantTwoToken)

	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "wx-summary-001", "device_id": "dev-summary-001", "nickname": "buyer-summary",
	}, nil)
	if login.Code != 0 {
		t.Fatalf("buyer login failed: %+v", login)
	}
	headers := map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"]), "X-Device-Id": "dev-summary-001"}

	for _, scoped := range []struct {
		merchantNo string
		productID  uint64
		wechat     string
	}{
		{merchantNo: merchantOneNo, productID: productOneID, wechat: "wx_summary_one"},
		{merchantNo: merchantTwoNo, productID: productTwoID, wechat: "wx_summary_two"},
	} {
		if resp := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/favorites", scoped.merchantNo), map[string]interface{}{"product_id": scoped.productID}, headers); resp.Code != 0 {
			t.Fatalf("favorite add failed: %+v", resp)
		}
		if resp := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/histories/views", scoped.merchantNo), map[string]interface{}{"product_id": scoped.productID}, headers); resp.Code != 0 {
			t.Fatalf("history view failed: %+v", resp)
		}
		if resp := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/intents", scoped.merchantNo), map[string]interface{}{
			"product_id":     scoped.productID,
			"contact_wechat": scoped.wechat,
		}, headers); resp.Code != 0 {
			t.Fatalf("intent create failed: %+v", resp)
		}
	}

	missing := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/me/summary", nil, headers)
	if missing.Code == 0 {
		t.Fatal("buyer summary without merchant_no succeeded")
	}

	for _, merchantNo := range []string{merchantOneNo, merchantTwoNo} {
		summary := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/me/summary", merchantNo), nil, headers)
		if summary.Code != 0 {
			t.Fatalf("buyer summary failed: %+v", summary)
		}
		counters := summary.Data["counters"].(map[string]interface{})
		if numToUint64(counters["favorites"]) != 1 || numToUint64(counters["histories"]) != 1 || numToUint64(counters["intents_open"]) != 1 {
			t.Fatalf("summary counters should be scoped to merchant %s: %+v", merchantNo, summary)
		}
	}
}

func TestBuyerGuestMergeIdempotent(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "buyermerge")
	approveMerchant(t, srv, adminToken, merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantToken := str(merchant.Data["access_token"])
	merchantNo := merchantNoByID(t, srv, merchantID)
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	guestHeaders := map[string]string{"X-Device-Id": "dev-merge-001"}
	fav := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/favorites", merchantNo), map[string]interface{}{"product_id": productID}, guestHeaders)
	if fav.Code != 0 {
		t.Fatalf("guest favorite failed: %+v", fav)
	}
	view := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/histories/views", merchantNo), map[string]interface{}{"product_id": productID}, guestHeaders)
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
	merchantNo := merchantNoByID(t, srv, merchantID)
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	buyerLoginResp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "wx-intent-001", "device_id": "dev-intent-001", "nickname": "buyer-intent",
	}, nil)
	if buyerLoginResp.Code != 0 {
		t.Fatalf("buyer login failed: %+v", buyerLoginResp)
	}
	buyerToken := str(buyerLoginResp.Data["access_token"])

	create1 := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/intents", merchantNo), map[string]interface{}{
		"product_id":    productID,
		"contact_phone": "13800138000",
		"message":       "有意向，方便联系",
	}, map[string]string{"Authorization": "Bearer " + buyerToken, "X-Device-Id": "dev-intent-001"})
	if create1.Code != 0 {
		t.Fatalf("create intent failed: %+v", create1)
	}
	intentID := numToUint64(create1.Data["intent_id"])

	createConflict := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/intents", merchantNo), map[string]interface{}{
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

	buyerList := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo("/api/v1/buyer/intents", merchantNo), nil, map[string]string{"Authorization": "Bearer " + buyerToken})
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

	buyerDetail := requestJSON(t, srv.Router, http.MethodGet, withMerchantNo(fmt.Sprintf("/api/v1/buyer/intents/%d", intentID), merchantNo), nil, map[string]string{"Authorization": "Bearer " + buyerToken})
	if buyerDetail.Code != 0 {
		t.Fatalf("buyer intent detail failed: %+v", buyerDetail)
	}
	intentDetail := buyerDetail.Data["intent"].(map[string]interface{})
	if intentDetail["status"] != "CLOSED" || intentDetail["buyer_status_text"] != "已关闭" {
		t.Fatalf("buyer detail status mapping mismatch: %+v", buyerDetail)
	}

	createAfterClose := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/intents", merchantNo), map[string]interface{}{
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
	merchantNo := merchantNoByID(t, srv, merchantID)
	draftProductID := createDraftProduct(t, srv, merchantToken)

	buyerLoginResp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "wx-guard-001", "device_id": "dev-guard-001", "nickname": "buyer-guard",
	}, nil)
	if buyerLoginResp.Code != 0 {
		t.Fatalf("buyer login failed: %+v", buyerLoginResp)
	}
	buyerToken := str(buyerLoginResp.Data["access_token"])

	invalidStatus := requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/intents", merchantNo), map[string]interface{}{
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
