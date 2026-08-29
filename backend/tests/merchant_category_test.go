package tests

import (
	"fmt"
	"net/http"
	"testing"

	"second-hand-market-backend/backend/internal/model"
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

	createSecondChild := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/categories", map[string]interface{}{
		"level": 2, "parent_id": rootID, "name": "自定义二级二", "sort": 2,
	}, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if createSecondChild.Code != 0 {
		t.Fatalf("create second child code = %d data=%v", createSecondChild.Code, createSecondChild.Data)
	}
	secondChildID := numToUint64(createSecondChild.Data["id"])
	if secondChildID == 0 {
		t.Fatalf("create second child returned invalid id: %+v", createSecondChild)
	}

	otherUpdate := requestJSON(t, srv.Router, http.MethodPut, fmt.Sprintf("/api/v1/merchant/categories/%d", childID), map[string]interface{}{
		"name": "越权修改",
	}, map[string]string{"Authorization": "Bearer " + merchantTwoToken})
	if otherUpdate.Code == 0 {
		t.Fatal("cross-merchant category update succeeded")
	}

	updateChild := requestJSON(t, srv.Router, http.MethodPut, fmt.Sprintf("/api/v1/merchant/categories/%d", childID), map[string]interface{}{
		"name": "自定义二级改名", "sort": 3, "status": "DISABLED",
	}, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if updateChild.Code != 0 {
		t.Fatalf("update child failed: %+v", updateChild)
	}

	enabledOnly := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/categories?level=2", nil, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if enabledOnly.Code != 0 {
		t.Fatalf("enabled category list failed: %+v", enabledOnly)
	}
	for _, item := range enabledOnly.Data["items"].([]interface{}) {
		row := item.(map[string]interface{})
		if numToUint64(row["id"]) == childID {
			t.Fatalf("disabled category leaked into default enabled list: %+v", enabledOnly)
		}
	}

	allStatuses := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/categories?level=2&status=ALL", nil, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if allStatuses.Code != 0 {
		t.Fatalf("all-status category list failed: %+v", allStatuses)
	}
	foundDisabled := false
	for _, item := range allStatuses.Data["items"].([]interface{}) {
		row := item.(map[string]interface{})
		if numToUint64(row["id"]) == childID {
			foundDisabled = row["status"] == "DISABLED"
		}
	}
	if !foundDisabled {
		t.Fatalf("disabled category should stay visible in management all-status list: %+v", allStatuses)
	}

	deleteRootWithChild := requestJSON(t, srv.Router, http.MethodDelete, fmt.Sprintf("/api/v1/merchant/categories/%d", rootID), nil, map[string]string{"Authorization": "Bearer " + merchantOneToken})
	if deleteRootWithChild.Code != 0 {
		t.Fatalf("delete root with children failed: %+v", deleteRootWithChild)
	}
	var deletedRoot, deletedChild, deletedSecondChild model.Category
	if err := srv.DB.Unscoped().First(&deletedRoot, rootID).Error; err != nil {
		t.Fatalf("load deleted root: %v", err)
	}
	if err := srv.DB.Unscoped().First(&deletedChild, childID).Error; err != nil {
		t.Fatalf("load deleted child: %v", err)
	}
	if err := srv.DB.Unscoped().First(&deletedSecondChild, secondChildID).Error; err != nil {
		t.Fatalf("load deleted second child: %v", err)
	}
	if !deletedRoot.DeletedAt.Valid || !deletedChild.DeletedAt.Valid || !deletedSecondChild.DeletedAt.Valid {
		t.Fatalf("deleting root should soft-delete root and all children: root=%+v child=%+v second_child=%+v", deletedRoot, deletedChild, deletedSecondChild)
	}
}

func TestMerchantCategoryDeleteParentIsAtomicWhenChildHasProduct(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "cat_delete_atomic")
	adminToken := adminAccessToken(t, srv)
	approveMerchant(t, srv, adminToken, merchantID)
	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	token := str(login.Data["access_token"])

	root := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/categories", map[string]interface{}{
		"level": 1, "name": "带商品一级", "sort": 1,
	}, map[string]string{"Authorization": "Bearer " + token})
	if root.Code != 0 {
		t.Fatalf("create root failed: %+v", root)
	}
	rootID := numToUint64(root.Data["id"])
	child := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/categories", map[string]interface{}{
		"level": 2, "parent_id": rootID, "name": "带商品二级", "sort": 1,
	}, map[string]string{"Authorization": "Bearer " + token})
	if child.Code != 0 {
		t.Fatalf("create child failed: %+v", child)
	}
	childID := numToUint64(child.Data["id"])
	if err := srv.DB.Create(&model.Product{
		ProductNo:      "category-delete-product",
		MerchantID:     merchantID,
		Title:          "引用分类的商品",
		Description:    "商品描述",
		CategoryID:     childID,
		PriceCent:      100,
		ConditionLevel: "GOOD",
		Stock:          1,
		Status:         model.ProductDraft,
	}).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}

	deleted := requestJSON(t, srv.Router, http.MethodDelete, fmt.Sprintf("/api/v1/merchant/categories/%d", rootID), nil, map[string]string{"Authorization": "Bearer " + token})
	if deleted.Code == 0 {
		t.Fatal("deleted root category that contains a referenced child")
	}
	var storedRoot, storedChild model.Category
	if err := srv.DB.Unscoped().First(&storedRoot, rootID).Error; err != nil {
		t.Fatalf("load root after rejected delete: %v", err)
	}
	if err := srv.DB.Unscoped().First(&storedChild, childID).Error; err != nil {
		t.Fatalf("load child after rejected delete: %v", err)
	}
	if storedRoot.DeletedAt.Valid || storedChild.DeletedAt.Valid {
		t.Fatalf("rejected cascade delete must be atomic: root=%+v child=%+v", storedRoot, storedChild)
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
