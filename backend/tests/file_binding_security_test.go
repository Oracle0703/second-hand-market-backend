package tests

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func registrationPayload(username string, fileID uint64, rawToken string) map[string]interface{} {
	return map[string]interface{}{
		"merchant_name":      "Claim Store",
		"contact_name":       "Owner",
		"phone":              "13800138000",
		"username":           username,
		"password":           "Passw0rd!2026",
		"license_file_id":    fileID,
		"license_file_token": rawToken,
	}
}

func loginApprovedMerchant(
	t *testing.T,
	srv *app.Server,
	adminToken string,
	prefix string,
) (uint64, string) {
	t.Helper()
	merchantID, username, password := registerMerchant(t, srv, prefix)
	approveMerchant(t, srv, adminToken, merchantID)
	login := merchantLogin(t, srv, username, password)
	if login.Code != common.CodeOK || str(login.Data["token_scope"]) != "full" {
		t.Fatalf("full merchant login: %+v", login)
	}
	return merchantID, str(login.Data["access_token"])
}

func productCreatePayload(categoryID uint64, fileIDs []uint64) map[string]interface{} {
	return map[string]interface{}{
		"title":               "Binding Product",
		"description":         "binding security regression",
		"category_id":         categoryID,
		"price_cent":          10000,
		"original_price_cent": 12000,
		"condition_level":     "GOOD",
		"stock":               1,
		"image_file_ids":      fileIDs,
	}
}

func TestProductCreateValidatesImageBindings(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantA, tokenA := loginApprovedMerchant(t, srv, adminToken, "product_bind_a")
	merchantB, _ := loginApprovedMerchant(t, srv, adminToken, "product_bind_b")
	validID, categoryID := productImageAndCategory(t, srv, tokenA)
	foreign := createReadyOwnedFile(t, srv, merchantB, model.FileBizProductImage)
	wrongType := createReadyOwnedFile(t, srv, merchantA, model.FileBizMerchantLicense)
	pending := createReadyOwnedFile(t, srv, merchantA, model.FileBizProductImage)
	if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", pending.ID).Updates(map[string]interface{}{
		"scan_status": model.FileScanPending,
		"url":         "",
	}).Error; err != nil {
		t.Fatalf("make pending image: %v", err)
	}
	blocked := createReadyOwnedFile(t, srv, merchantA, model.FileBizProductImage)
	if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", blocked.ID).Update("scan_status", model.FileScanBlocked).Error; err != nil {
		t.Fatalf("block image: %v", err)
	}
	emptyURL := createReadyOwnedFile(t, srv, merchantA, model.FileBizProductImage)
	if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", emptyURL.ID).Update("url", "").Error; err != nil {
		t.Fatalf("clear image URL: %v", err)
	}

	tests := []struct {
		name string
		ids  []uint64
	}{
		{name: "foreign", ids: []uint64{foreign.ID}},
		{name: "wrong type", ids: []uint64{wrongType.ID}},
		{name: "pending", ids: []uint64{pending.ID}},
		{name: "blocked", ids: []uint64{blocked.ID}},
		{name: "empty URL", ids: []uint64{emptyURL.ID}},
		{name: "missing", ids: []uint64{999999}},
		{name: "duplicate", ids: []uint64{validID, validID}},
		{name: "mixed valid and foreign", ids: []uint64{validID, foreign.ID}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var productsBefore int64
			var imagesBefore int64
			var logsBefore int64
			if err := srv.DB.Model(&model.Product{}).Count(&productsBefore).Error; err != nil {
				t.Fatal(err)
			}
			if err := srv.DB.Model(&model.ProductImage{}).Count(&imagesBefore).Error; err != nil {
				t.Fatal(err)
			}
			if err := srv.DB.Model(&model.OperationLog{}).Where("action = ?", "product_create").Count(&logsBefore).Error; err != nil {
				t.Fatal(err)
			}

			resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/products",
				productCreatePayload(categoryID, tt.ids),
				map[string]string{"Authorization": "Bearer " + tokenA})
			if resp.Code != common.CodeInvalidFileBinding {
				t.Fatalf("create response = %+v", resp)
			}

			var productsAfter int64
			var imagesAfter int64
			var logsAfter int64
			_ = srv.DB.Model(&model.Product{}).Count(&productsAfter).Error
			_ = srv.DB.Model(&model.ProductImage{}).Count(&imagesAfter).Error
			_ = srv.DB.Model(&model.OperationLog{}).Where("action = ?", "product_create").Count(&logsAfter).Error
			if productsAfter != productsBefore || imagesAfter != imagesBefore || logsAfter != logsBefore {
				t.Fatalf(
					"failed create persisted rows: products %d->%d images %d->%d logs %d->%d",
					productsBefore, productsAfter, imagesBefore, imagesAfter, logsBefore, logsAfter,
				)
			}
		})
	}
}

func loadProductImageIDs(t *testing.T, srv *app.Server, productID uint64) []uint64 {
	t.Helper()
	var images []model.ProductImage
	if err := srv.DB.Where("product_id = ?", productID).Order("sort_order ASC").Find(&images).Error; err != nil {
		t.Fatalf("load product images: %v", err)
	}
	ids := make([]uint64, 0, len(images))
	for _, image := range images {
		ids = append(ids, image.FileID)
	}
	return ids
}

func TestProductUpdatePreservesImagesWhenBindingFails(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantA, tokenA := loginApprovedMerchant(t, srv, adminToken, "product_update_a")
	merchantB, _ := loginApprovedMerchant(t, srv, adminToken, "product_update_b")
	firstID, categoryID := productImageAndCategory(t, srv, tokenA)
	second := createReadyOwnedFileForToken(t, srv, tokenA, model.FileBizProductImage)
	create := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/products",
		productCreatePayload(categoryID, []uint64{firstID, second.ID}),
		map[string]string{"Authorization": "Bearer " + tokenA})
	if create.Code != common.CodeOK {
		t.Fatalf("create product: %+v", create)
	}
	productID := numToUint64(create.Data["product_id"])
	beforeIDs := loadProductImageIDs(t, srv, productID)
	var before model.Product
	if err := srv.DB.First(&before, productID).Error; err != nil {
		t.Fatalf("load product before update: %v", err)
	}
	foreign := createReadyOwnedFile(t, srv, merchantB, model.FileBizProductImage)

	resp := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/merchant/products/"+fmt.Sprint(productID), map[string]interface{}{
		"image_file_ids": []uint64{foreign.ID},
	}, map[string]string{"Authorization": "Bearer " + tokenA})
	if resp.Code != common.CodeInvalidFileBinding {
		t.Fatalf("update response = %+v", resp)
	}
	afterIDs := loadProductImageIDs(t, srv, productID)
	var after model.Product
	if err := srv.DB.First(&after, productID).Error; err != nil {
		t.Fatalf("load product after update: %v", err)
	}
	if !reflect.DeepEqual(afterIDs, beforeIDs) {
		t.Fatalf("images changed: before=%v after=%v", beforeIDs, afterIDs)
	}
	if before.CoverFileID == nil || after.CoverFileID == nil || *after.CoverFileID != *before.CoverFileID {
		t.Fatalf("cover changed: before=%v after=%v", before.CoverFileID, after.CoverFileID)
	}
	if after.MerchantID != merchantA {
		t.Fatalf("product owner changed: got %d want %d", after.MerchantID, merchantA)
	}
}

func countRegistrationRows(t *testing.T, srv *app.Server) (int64, int64, int64) {
	t.Helper()
	var merchants int64
	var accounts int64
	var audits int64
	if err := srv.DB.Model(&model.Merchant{}).Count(&merchants).Error; err != nil {
		t.Fatalf("count merchants: %v", err)
	}
	if err := srv.DB.Model(&model.MerchantAccount{}).Count(&accounts).Error; err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if err := srv.DB.Model(&model.MerchantAuditLog{}).Count(&audits).Error; err != nil {
		t.Fatalf("count audits: %v", err)
	}
	return merchants, accounts, audits
}

func TestRegisterClaimsUploadedLicense(t *testing.T) {
	srv := newTestServer(t)
	fileID, rawToken := uploadReadyPublicLicense(t, srv)
	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register",
		registrationPayload(uniqueUsername("claim"), fileID, rawToken), nil)
	if resp.Code != common.CodeOK {
		t.Fatalf("register: %+v", resp)
	}
	merchantID := numToUint64(resp.Data["merchant_id"])
	var file model.FileRecord
	if err := srv.DB.First(&file, fileID).Error; err != nil {
		t.Fatalf("load claimed license: %v", err)
	}
	if file.OwnerMerchantID == nil || *file.OwnerMerchantID != merchantID ||
		file.CapabilityTokenHash != nil || file.CapabilityExpiresAt != nil {
		t.Fatalf("license was not consumed atomically: %+v", file)
	}
	var merchant model.Merchant
	if err := srv.DB.First(&merchant, merchantID).Error; err != nil {
		t.Fatalf("load merchant: %v", err)
	}
	if merchant.LicenseFileID == nil || *merchant.LicenseFileID != fileID {
		t.Fatalf("merchant license = %v, want %d", merchant.LicenseFileID, fileID)
	}
}

func TestRegisterRejectsInvalidLicenseBinding(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *app.Server) (uint64, string)
	}{
		{name: "missing file", prepare: func(_ *testing.T, _ *app.Server) (uint64, string) {
			return 999999, "missing-file-token"
		}},
		{name: "missing token", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, _ := uploadReadyPublicLicense(t, srv)
			return fileID, ""
		}},
		{name: "wrong token", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, _ := uploadReadyPublicLicense(t, srv)
			return fileID, "wrong-token"
		}},
		{name: "expired token", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, rawToken := uploadReadyPublicLicense(t, srv)
			expired := time.Now().Add(-time.Minute)
			if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Update("capability_expires_at", expired).Error; err != nil {
				t.Fatalf("expire token: %v", err)
			}
			return fileID, rawToken
		}},
		{name: "wrong biz type", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, rawToken := uploadReadyPublicLicense(t, srv)
			if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Update("biz_type", model.FileBizProductImage).Error; err != nil {
				t.Fatalf("change biz type: %v", err)
			}
			return fileID, rawToken
		}},
		{name: "pending", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
				"biz_type": model.FileBizMerchantLicense, "file_name": "pending.jpg", "file_size": 22, "mime_type": "image/jpeg",
			}, nil)
			return numToUint64(presign.Data["file_id"]), str(presign.Data["file_token"])
		}},
		{name: "already owned", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, rawToken := uploadReadyPublicLicense(t, srv)
			owner := uint64(42)
			if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Update("owner_merchant_id", owner).Error; err != nil {
				t.Fatalf("set owner: %v", err)
			}
			return fileID, rawToken
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			fileID, rawToken := tt.prepare(t, srv)
			resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register",
				registrationPayload(uniqueUsername("invalid"), fileID, rawToken), nil)
			if resp.Code != common.CodeInvalidFileBinding {
				t.Fatalf("register response = %+v", resp)
			}
			merchants, accounts, audits := countRegistrationRows(t, srv)
			if merchants != 0 || accounts != 0 || audits != 0 {
				t.Fatalf("partial registration persisted: merchants=%d accounts=%d audits=%d", merchants, accounts, audits)
			}
		})
	}
}

func TestRegisterLicenseCapabilityCannotBeReplayed(t *testing.T) {
	srv := newTestServer(t)
	fileID, rawToken := uploadReadyPublicLicense(t, srv)
	first := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register",
		registrationPayload(uniqueUsername("first"), fileID, rawToken), nil)
	if first.Code != common.CodeOK {
		t.Fatalf("first registration: %+v", first)
	}
	second := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register",
		registrationPayload(uniqueUsername("second"), fileID, rawToken), nil)
	if second.Code != common.CodeInvalidFileBinding {
		t.Fatalf("replayed registration = %+v", second)
	}
	merchants, accounts, audits := countRegistrationRows(t, srv)
	if merchants != 1 || accounts != 1 || audits != 1 {
		t.Fatalf("replay changed registration rows: merchants=%d accounts=%d audits=%d", merchants, accounts, audits)
	}
}

func TestMerchantReapplyValidatesLicenseBinding(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, *app.Server, uint64, uint64) model.FileRecord
		wantCode int
	}{
		{
			name: "foreign license",
			prepare: func(t *testing.T, srv *app.Server, _ uint64, merchantB uint64) model.FileRecord {
				return createReadyOwnedFile(t, srv, merchantB, model.FileBizMerchantLicense)
			},
			wantCode: common.CodeInvalidFileBinding,
		},
		{
			name: "wrong type",
			prepare: func(t *testing.T, srv *app.Server, merchantA uint64, _ uint64) model.FileRecord {
				return createReadyOwnedFile(t, srv, merchantA, model.FileBizProductImage)
			},
			wantCode: common.CodeInvalidFileBinding,
		},
		{
			name: "pending",
			prepare: func(t *testing.T, srv *app.Server, merchantA uint64, _ uint64) model.FileRecord {
				file := createReadyOwnedFile(t, srv, merchantA, model.FileBizMerchantLicense)
				if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", file.ID).Updates(map[string]interface{}{
					"scan_status": model.FileScanPending,
					"url":         "",
				}).Error; err != nil {
					t.Fatalf("make pending file: %v", err)
				}
				return file
			},
			wantCode: common.CodeInvalidFileBinding,
		},
		{
			name: "valid own license",
			prepare: func(t *testing.T, srv *app.Server, merchantA uint64, _ uint64) model.FileRecord {
				return createReadyOwnedFile(t, srv, merchantA, model.FileBizMerchantLicense)
			},
			wantCode: common.CodeOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			merchantA, usernameA, passwordA := registerMerchant(t, srv, "reapply_a")
			merchantB, _, _ := registerMerchant(t, srv, "reapply_b")
			adminToken := adminAccessToken(t, srv)
			rejectMerchant(t, srv, adminToken, merchantA)
			rejectMerchant(t, srv, adminToken, merchantB)

			login := merchantLogin(t, srv, usernameA, passwordA)
			if login.Code != common.CodeOK || str(login.Data["token_scope"]) != "onboarding" {
				t.Fatalf("onboarding login: %+v", login)
			}
			var before model.Merchant
			if err := srv.DB.First(&before, merchantA).Error; err != nil {
				t.Fatalf("load merchant before reapply: %v", err)
			}
			var auditBefore int64
			if err := srv.DB.Model(&model.MerchantAuditLog{}).
				Where("merchant_id = ? AND action = ?", merchantA, "REAPPLY").
				Count(&auditBefore).Error; err != nil {
				t.Fatalf("count reapply audits: %v", err)
			}

			file := tt.prepare(t, srv, merchantA, merchantB)
			resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/reapply", map[string]interface{}{
				"license_file_id": file.ID,
			}, map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])})
			if resp.Code != tt.wantCode {
				t.Fatalf("reapply response = %+v", resp)
			}

			var after model.Merchant
			if err := srv.DB.First(&after, merchantA).Error; err != nil {
				t.Fatalf("load merchant after reapply: %v", err)
			}
			var auditAfter int64
			if err := srv.DB.Model(&model.MerchantAuditLog{}).
				Where("merchant_id = ? AND action = ?", merchantA, "REAPPLY").
				Count(&auditAfter).Error; err != nil {
				t.Fatalf("count reapply audits after: %v", err)
			}

			if tt.wantCode == common.CodeOK {
				if after.ReviewStatus != model.ReviewPending || after.LicenseFileID == nil || *after.LicenseFileID != file.ID {
					t.Fatalf("valid reapply not persisted: %+v", after)
				}
				if auditAfter != auditBefore+1 {
					t.Fatalf("reapply audit count = %d, want %d", auditAfter, auditBefore+1)
				}
				return
			}

			if after.ReviewStatus != model.ReviewRejected || before.LicenseFileID == nil || after.LicenseFileID == nil ||
				*after.LicenseFileID != *before.LicenseFileID {
				t.Fatalf("invalid reapply changed merchant: before=%+v after=%+v", before, after)
			}
			if auditAfter != auditBefore {
				t.Fatalf("invalid reapply added audit: before=%d after=%d", auditBefore, auditAfter)
			}
		})
	}
}
