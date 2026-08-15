package tests

import (
	"fmt"
	"net/http"
	"testing"
)

func TestMerchantCategoryCRUDIsScopedToMerchant(t *testing.T) {
	srv := newTestServer(t)
	merchantOneID, merchantOneUser, merchantOnePassword := registerMerchant(t, srv, "cat_owner_1")
	merchantTwoID, merchantTwoUser, merchantTwoPassword := registerMerchant(t, srv, "cat_owner_2")
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
	rootID := numToUint64(createRoot.Data["id"])
	if rootID == 0 {
		t.Fatalf("create root returned invalid id: %+v", createRoot.Data)
	}

	createChild := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/categories", map[string]interface{}{
		"level": 2, "parent_id": rootID, "name": "自定义二级", "sort": 1,
	}, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if createChild.Code != 0 {
		t.Fatalf("create child code = %d data=%v", createChild.Code, createChild.Data)
	}
	childID := numToUint64(createChild.Data["id"])
	if childID == 0 {
		t.Fatalf("create child returned invalid id: %+v", createChild.Data)
	}

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

	updateChild := requestJSON(t, srv.Router, http.MethodPut, fmt.Sprintf("/api/v1/merchant/categories/%d", childID), map[string]interface{}{
		"name": "自定义二级改名", "sort": 3, "status": "DISABLED",
	}, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if updateChild.Code != 0 {
		t.Fatalf("update child failed: %+v", updateChild)
	}
}

func TestProductRejectsCrossMerchantCategory(t *testing.T) {
	srv := newTestServer(t)
	merchantOneID, merchantOneUser, merchantOnePassword := registerMerchant(t, srv, "cat_product_1")
	merchantTwoID, merchantTwoUser, merchantTwoPassword := registerMerchant(t, srv, "cat_product_2")
	adminToken := adminAccessToken(t, srv)
	approveMerchant(t, srv, adminToken, merchantOneID)
	approveMerchant(t, srv, adminToken, merchantTwoID)
	loginOne := merchantLogin(t, srv, merchantOneUser, merchantOnePassword)
	loginTwo := merchantLogin(t, srv, merchantTwoUser, merchantTwoPassword)
	if loginOne.Code != 0 || loginTwo.Code != 0 {
		t.Fatalf("merchant login failed: one=%+v two=%+v", loginOne, loginTwo)
	}
	merchantOneToken := str(loginOne.Data["access_token"])
	merchantTwoToken := str(loginTwo.Data["access_token"])

	cats := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/categories?level=2", nil, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if cats.Code != 0 {
		t.Fatalf("merchant one categories failed: %+v", cats)
	}
	items := cats.Data["items"].([]interface{})
	merchantOneCategoryID := numToUint64(items[0].(map[string]interface{})["id"])
	img := uploadProductImage(t, srv, merchantTwoToken, encodedUploadImage(t, "image/jpeg"), "image/jpeg")

	create := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/products", map[string]interface{}{
		"title": "跨商户分类商品", "description": "desc", "category_id": merchantOneCategoryID,
		"price_cent": 10000, "original_price_cent": 12000, "condition_level": "GOOD",
		"stock": 1, "image_file_ids": []uint64{img.ID},
	}, map[string]string{"Authorization": "Bearer " + merchantTwoToken})
	if create.Code == 0 {
		t.Fatal("product creation accepted another merchant's category")
	}
}
