package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"second-hand-market-backend/backend/internal/model"
)

func TestProductUpdateRejectsInvalidFields(t *testing.T) {
	srv := newTestServer(t)
	token := approvedMerchantToken(t, srv, "update_validation")
	image := uploadProductImage(t, srv, token, encodedUploadImage(t, "image/jpeg"), "image/jpeg")
	create := createProductWithImages(t, srv, token, []uint64{image.ID})
	if create.Code != 0 {
		t.Fatalf("create product failed: %+v", create)
	}
	productID := numToUint64(create.Data["product_id"])
	path := fmt.Sprintf("/api/v1/merchant/products/%d", productID)
	headers := map[string]string{"Authorization": "Bearer " + token}

	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{"empty title", map[string]interface{}{"title": ""}},
		{"short title", map[string]interface{}{"title": "a"}},
		{"long title", map[string]interface{}{"title": strings.Repeat("a", 129)}},
		{"empty description", map[string]interface{}{"description": ""}},
		{"short description", map[string]interface{}{"description": "a"}},
		{"long description", map[string]interface{}{"description": strings.Repeat("a", 5001)}},
		{"empty condition level", map[string]interface{}{"condition_level": ""}},
		{"unknown condition level", map[string]interface{}{"condition_level": "BROKEN"}},
	}
	for _, tc := range cases {
		resp := requestJSON(t, srv.Router, http.MethodPut, path, tc.body, headers)
		if resp.Code != 10001 || resp.HTTPStatus != http.StatusBadRequest {
			t.Fatalf("%s must be rejected: %+v", tc.name, resp)
		}
	}

	var product model.Product
	if err := srv.DB.First(&product, productID).Error; err != nil {
		t.Fatalf("load product: %v", err)
	}
	if product.Title != "图片校验商品" || product.Description != "desc" || product.ConditionLevel != "GOOD" {
		t.Fatalf("rejected updates must not change product: %+v", product)
	}

	valid := requestJSON(t, srv.Router, http.MethodPut, path, map[string]interface{}{
		"title":           "更新后的商品",
		"description":     "更新后的描述",
		"condition_level": "LIKE_NEW",
	}, headers)
	if valid.Code != 0 {
		t.Fatalf("valid update failed: %+v", valid)
	}
	if err := srv.DB.First(&product, productID).Error; err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if product.Title != "更新后的商品" || product.Description != "更新后的描述" || product.ConditionLevel != "LIKE_NEW" {
		t.Fatalf("valid update not persisted: %+v", product)
	}
}
