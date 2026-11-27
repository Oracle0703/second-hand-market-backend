package tests

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/model"
)

func firstMerchantCategoryID(t *testing.T, srv *app.Server, token string) uint64 {
	t.Helper()
	cats := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/categories?level=2", nil, map[string]string{"Authorization": "Bearer " + token})
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
	return categoryID
}

func createProductWithImages(t *testing.T, srv *app.Server, token string, imageFileIDs []uint64) apiResp {
	t.Helper()
	return requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/products", map[string]interface{}{
		"title":               "图片校验商品",
		"description":         "desc",
		"category_id":         firstMerchantCategoryID(t, srv, token),
		"price_cent":          10000,
		"original_price_cent": 12000,
		"condition_level":     "GOOD",
		"stock":               1,
		"image_file_ids":      imageFileIDs,
	}, map[string]string{"Authorization": "Bearer " + token})
}

func createLegacyPassProductImageRecord(t *testing.T, srv *app.Server, username string) uint64 {
	t.Helper()
	var account model.MerchantAccount
	if err := srv.DB.Where("username = ?", username).First(&account).Error; err != nil {
		t.Fatalf("load merchant account: %v", err)
	}
	file := model.FileRecord{
		BizType:      model.FileBizProductImage,
		ObjectKey:    fmt.Sprintf("product_image/Flegacy%d.jpg", time.Now().UnixNano()),
		MimeType:     "image/jpeg",
		SizeBytes:    1024,
		UploaderType: model.UserTypeMerchant,
		UploaderID:   &account.ID,
		ScanStatus:   model.FileScanPass,
	}
	if err := srv.DB.Create(&file).Error; err != nil {
		t.Fatalf("create legacy product image record: %v", err)
	}
	return file.ID
}

func TestProductCreateRejectsPendingProductImage(t *testing.T) {
	srv := newTestServer(t)
	token := approvedMerchantToken(t, srv, "pending_product_image")
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "PRODUCT_IMAGE",
		"file_name": "pending.jpg",
		"file_size": 1024,
		"mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + token})
	if presign.Code != 0 {
		t.Fatalf("product image presign failed: %+v", presign)
	}

	create := createProductWithImages(t, srv, token, []uint64{numToUint64(presign.Data["file_id"])})
	if create.Code != 10008 {
		t.Fatalf("pending product image must be rejected: %+v", create)
	}
}

func TestProductCreateAllowsLegacyPassImageWhenStrictSwitchIsFalse(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "legacy_allowed")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	login := merchantLogin(t, srv, username, password)
	token := str(login.Data["access_token"])
	fileID := createLegacyPassProductImageRecord(t, srv, username)

	create := createProductWithImages(t, srv, token, []uint64{fileID})
	if create.Code != 0 {
		t.Fatalf("legacy PASS image should be allowed while strict switch is false: %+v", create)
	}
}

func TestProductCreateRejectsLegacyPassImageWhenStrictSwitchIsTrue(t *testing.T) {
	cfg := newTestConfig(t, t.TempDir())
	cfg.RequireDetailV1ProductImages = true
	srv := newTestServerWithConfig(t, cfg)
	merchantID, username, password := registerMerchant(t, srv, "legacy_rejected")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	login := merchantLogin(t, srv, username, password)
	token := str(login.Data["access_token"])
	fileID := createLegacyPassProductImageRecord(t, srv, username)

	create := createProductWithImages(t, srv, token, []uint64{fileID})
	if create.Code != 10008 {
		t.Fatalf("legacy image must be rejected while strict switch is true: %+v", create)
	}
}

func TestProductCreateRejectsCrossMerchantProductImage(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	m1ID, m1User, m1Password := registerMerchant(t, srv, "image_owner")
	m2ID, m2User, m2Password := registerMerchant(t, srv, "image_editor")
	approveMerchant(t, srv, adminToken, m1ID)
	approveMerchant(t, srv, adminToken, m2ID)
	m1Token := str(merchantLogin(t, srv, m1User, m1Password).Data["access_token"])
	m2Token := str(merchantLogin(t, srv, m2User, m2Password).Data["access_token"])
	uploaded := uploadProductImage(t, srv, m1Token, encodedUploadImage(t, "image/jpeg"), "image/jpeg")

	create := createProductWithImages(t, srv, m2Token, []uint64{uploaded.ID})
	if create.Code != 10008 {
		t.Fatalf("cross merchant product image must be rejected: %+v", create)
	}
}

func TestMerchantProductListReturnsFirstImageCoverURL(t *testing.T) {
	srv := newTestServer(t)
	token := approvedMerchantToken(t, srv, "list_cover")
	firstImage := uploadProductImage(t, srv, token, encodedUploadImage(t, "image/jpeg"), "image/jpeg")
	secondImage := uploadProductImage(t, srv, token, encodedUploadImage(t, "image/jpeg"), "image/jpeg")
	create := createProductWithImages(t, srv, token, []uint64{firstImage.ID, secondImage.ID})
	if create.Code != 0 {
		t.Fatalf("create product failed: %+v", create)
	}
	productID := numToUint64(create.Data["product_id"])

	list := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/products", nil, map[string]string{"Authorization": "Bearer " + token})
	if list.Code != 0 {
		t.Fatalf("product list failed: %+v", list)
	}
	items, ok := list.Data["items"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("product list items invalid: %+v", list)
	}
	for _, raw := range items {
		row, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if numToUint64(row["id"]) != productID {
			continue
		}
		if got := str(row["cover_url"]); got != firstImage.URL {
			t.Fatalf("product list cover_url mismatch: got=%q want=%q row=%+v", got, firstImage.URL, row)
		}
		return
	}
	t.Fatalf("created product not found in list: product_id=%d list=%+v", productID, list)
}

func TestProductDeleteByStaffRemovesOwnerUploadedImageRecord(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, ownerUser, ownerPassword := registerMerchant(t, srv, "staff_delete_owner_image")
	approveMerchant(t, srv, adminToken, merchantID)
	ownerToken := str(merchantLogin(t, srv, ownerUser, ownerPassword).Data["access_token"])
	productID := createDraftProduct(t, srv, ownerToken)

	var productImages []model.ProductImage
	if err := srv.DB.Where("product_id = ?", productID).Find(&productImages).Error; err != nil {
		t.Fatalf("load product images: %v", err)
	}
	if len(productImages) == 0 {
		t.Fatal("draft product has no images")
	}
	fileID := productImages[0].FileID
	staffUser, staffPassword := createStaffMerchantAccount(t, srv, merchantID, "staff_delete_owner_image")
	staffToken := str(merchantLogin(t, srv, staffUser, staffPassword).Data["access_token"])

	deleteResp := requestJSON(
		t,
		srv.Router,
		http.MethodDelete,
		fmt.Sprintf("/api/v1/merchant/products/%d", productID),
		nil,
		map[string]string{"Authorization": "Bearer " + staffToken},
	)
	if deleteResp.Code != 0 {
		t.Fatalf("staff delete product failed: %+v", deleteResp)
	}
	var fileCount int64
	if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Count(&fileCount).Error; err != nil {
		t.Fatalf("count deleted image record: %v", err)
	}
	if fileCount != 0 {
		t.Fatalf("staff delete should remove owner uploaded file record, got count=%d", fileCount)
	}
}

func TestProductDeleteKeepsOtherCoverAndGalleryReferences(t *testing.T) {
	uploadDir := t.TempDir()
	srv := newTestServerWithConfig(t, newTestConfig(t, uploadDir))
	token := approvedMerchantToken(t, srv, "shared_images")
	cover := uploadProductImage(t, srv, token, encodedUploadImage(t, "image/jpeg"), "image/jpeg")
	gallery := uploadProductImage(t, srv, token, encodedUploadImage(t, "image/jpeg"), "image/jpeg")
	ids := []uint64{cover.ID, gallery.ID}
	source := createProductWithImages(t, srv, token, ids)
	target := createProductWithImages(t, srv, token, ids)
	if source.Code != 0 || target.Code != 0 {
		t.Fatalf("failed to create products: %+v %+v", source, target)
	}
	// Older records may have a cover without a corresponding gallery row.
	if err := srv.DB.Where("product_id = ? AND file_id = ?", numToUint64(target.Data["product_id"]), cover.ID).Delete(&model.ProductImage{}).Error; err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + token}
	for _, product := range []apiResp{source, target} {
		result := requestJSON(t, srv.Router, http.MethodDelete, fmt.Sprintf("/api/v1/merchant/products/%d", numToUint64(product.Data["product_id"])), nil, headers)
		if result.Code != 0 {
			t.Fatalf("delete failed: %+v", result)
		}
		var count int64
		if err := srv.DB.Model(&model.FileRecord{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		shared := product.Data["product_id"] == source.Data["product_id"]
		if (shared && count != 2) || (!shared && count != 0) {
			t.Fatalf("shared=%t, remaining file records=%d", shared, count)
		}
		for _, image := range []uploadedFile{cover, gallery} {
			_, err := os.Stat(filepath.Join(uploadDir, filepath.FromSlash(image.ObjectKey)))
			if (shared && err != nil) || (!shared && !os.IsNotExist(err)) {
				t.Fatalf("shared=%t file=%d stat=%v", shared, image.ID, err)
			}
		}
	}
}

func createStaffMerchantAccount(t *testing.T, srv *app.Server, merchantID uint64, prefix string) (string, string) {
	t.Helper()
	username := uniqueUsername(prefix + "_staff")
	password := "Passw0rd!2026"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash staff password: %v", err)
	}
	account := model.MerchantAccount{
		MerchantID:   merchantID,
		Username:     username,
		PasswordHash: string(hash),
		Role:         model.AccountRoleStaff,
		Status:       model.AccountStatusActive,
	}
	if err := srv.DB.Create(&account).Error; err != nil {
		t.Fatalf("create staff account: %v", err)
	}
	return username, password
}
