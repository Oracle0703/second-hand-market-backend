package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func writeUploadFixture(t *testing.T, root, objectKey string, content []byte) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create upload fixture directory: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	return path
}

func TestPublicUploadAllowsOnlyProductImages(t *testing.T) {
	srv, uploadDir := newTestServerWithUploadDir(t)
	productBytes := []byte("public-product-image")
	licenseBytes := []byte("private-license-image")
	writeUploadFixture(t, uploadDir, "product_image/public.jpg", productBytes)
	writeUploadFixture(t, uploadDir, "merchant_license/private.jpg", licenseBytes)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "product image", path: "/uploads/product_image/public.jpg", wantStatus: http.StatusOK},
		{name: "merchant license", path: "/uploads/merchant_license/private.jpg", wantStatus: http.StatusNotFound},
		{name: "uppercase prefix", path: "/uploads/PRODUCT_IMAGE/public.jpg", wantStatus: http.StatusNotFound},
		{name: "unknown prefix", path: "/uploads/other/public.jpg", wantStatus: http.StatusNotFound},
		{name: "plain traversal", path: "/uploads/product_image/../merchant_license/private.jpg", wantStatus: http.StatusNotFound},
		{name: "encoded traversal", path: "/uploads/product_image/%2e%2e/merchant_license/private.jpg", wantStatus: http.StatusNotFound},
		{name: "missing object", path: "/uploads/product_image/", wantStatus: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			srv.Router.ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body=%q", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				if !bytes.Equal(w.Body.Bytes(), productBytes) {
					t.Fatalf("product bytes = %q, want %q", w.Body.Bytes(), productBytes)
				}
				if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
					t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
				}
				return
			}
			body := w.Body.String()
			if strings.Contains(body, uploadDir) || strings.Contains(body, "merchant_license/private.jpg") || bytes.Contains(w.Body.Bytes(), licenseBytes) {
				t.Fatalf("not-found response leaked private file details: %q", body)
			}
		})
	}
}

func TestPublicUploadRejectsSymlinkEscape(t *testing.T) {
	srv, uploadDir := newTestServerWithUploadDir(t)
	outsideDir := t.TempDir()
	outsideBytes := []byte("outside-upload-root")
	outsidePath := filepath.Join(outsideDir, "outside.jpg")
	if err := os.WriteFile(outsidePath, outsideBytes, 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	linkPath := filepath.Join(uploadDir, "product_image", "escape.jpg")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("create symlink directory: %v", err)
	}
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/uploads/product_image/escape.jpg", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if bytes.Contains(w.Body.Bytes(), outsideBytes) || strings.Contains(w.Body.String(), outsidePath) {
		t.Fatalf("response leaked outside file: %q", w.Body.String())
	}
}

type licensePrivacyFixture struct {
	srv        *app.Server
	uploadDir  string
	merchant   model.Merchant
	file       model.FileRecord
	fileBytes  []byte
	admin      model.AdminUser
	adminToken string
}

func newLicensePrivacyFixture(t *testing.T) licensePrivacyFixture {
	t.Helper()
	srv, uploadDir := newTestServerWithUploadDir(t)
	return newLicensePrivacyFixtureForServer(t, srv, uploadDir)
}

func newLicensePrivacyFixtureForServer(t *testing.T, srv *app.Server, uploadDir string) licensePrivacyFixture {
	t.Helper()
	merchant := model.Merchant{
		MerchantNo:   fmt.Sprintf("M-PRIVACY-%d", time.Now().UnixNano()),
		MerchantName: "Privacy Fixture Merchant",
		ContactName:  "Fixture Owner",
		ContactPhone: fmt.Sprintf("137%08d", time.Now().UnixNano()%100000000),
		ReviewStatus: model.ReviewPending,
	}
	if err := srv.DB.Create(&merchant).Error; err != nil {
		t.Fatalf("create privacy merchant: %v", err)
	}
	ownerID := merchant.ID
	file := model.FileRecord{
		BizType:         model.FileBizMerchantLicense,
		ObjectKey:       fmt.Sprintf("merchant_license/privacy-%d.jpg", time.Now().UnixNano()),
		URL:             "",
		MimeType:        "image/jpeg",
		SizeBytes:       22,
		UploaderType:    model.UserTypePublic,
		ScanStatus:      model.FileScanPass,
		OwnerMerchantID: &ownerID,
	}
	if err := srv.DB.Create(&file).Error; err != nil {
		t.Fatalf("create privacy file: %v", err)
	}
	if err := srv.DB.Model(&merchant).Update("license_file_id", file.ID).Error; err != nil {
		t.Fatalf("bind privacy file: %v", err)
	}
	merchant.LicenseFileID = &file.ID
	fileBytes := minimalJPEG()
	writeUploadFixture(t, uploadDir, file.ObjectKey, fileBytes)
	adminToken := adminAccessToken(t, srv)
	var admin model.AdminUser
	if err := srv.DB.Where("username = ?", "admin").First(&admin).Error; err != nil {
		t.Fatalf("load fixture admin: %v", err)
	}
	return licensePrivacyFixture{
		srv:        srv,
		uploadDir:  uploadDir,
		merchant:   merchant,
		file:       file,
		fileBytes:  fileBytes,
		admin:      admin,
		adminToken: adminToken,
	}
}

func requestAdminLicenseContent(t *testing.T, fixture licensePrivacyFixture, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/admin/files/%d/content", fixture.file.ID), nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	fixture.srv.Router.ServeHTTP(w, req)
	return w
}

func requireAPIErrorResponse(t *testing.T, w *httptest.ResponseRecorder, wantHTTP, wantCode int, privateBytes []byte) {
	t.Helper()
	if w.Code != wantHTTP {
		t.Fatalf("HTTP status = %d, want %d, body=%q", w.Code, wantHTTP, w.Body.String())
	}
	var response common.APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode API error: %v, body=%q", err, w.Body.String())
	}
	if response.Code != wantCode {
		t.Fatalf("API code = %d, want %d, body=%q", response.Code, wantCode, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), privateBytes) {
		t.Fatalf("error response leaked private bytes: %q", w.Body.String())
	}
}

func TestAdminLicenseContentStreamsPrivateFileAndWritesSafeAudit(t *testing.T) {
	fixture := newLicensePrivacyFixture(t)
	w := requestAdminLicenseContent(t, fixture, fixture.adminToken)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200, body=%q", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), fixture.fileBytes) {
		t.Fatalf("license bytes = %q, want %q", w.Body.Bytes(), fixture.fileBytes)
	}
	for name, want := range map[string]string{
		"Content-Type":           "image/jpeg",
		"Content-Disposition":    "inline",
		"Cache-Control":          "private, no-store",
		"Pragma":                 "no-cache",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := w.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	var logs []model.OperationLog
	if err := fixture.srv.DB.Where("action = ?", "admin_file_read").Find(&logs).Error; err != nil {
		t.Fatalf("load admin file read log: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("admin_file_read log count = %d, want 1", len(logs))
	}
	log := logs[0]
	if log.OperatorType != model.UserTypeAdmin || log.OperatorID != fixture.admin.ID ||
		log.MerchantID == nil || *log.MerchantID != fixture.merchant.ID ||
		log.ResourceType != "file" || log.ResourceID != fixture.file.ID ||
		log.ResultCode != common.CodeOK {
		t.Fatalf("unexpected admin_file_read log: %+v", log)
	}
	var detail map[string]string
	if err := json.Unmarshal(log.DetailJSON, &detail); err != nil {
		t.Fatalf("decode audit detail: %v", err)
	}
	if len(detail) != 2 || detail["biz_type"] != model.FileBizMerchantLicense || detail["scan_status"] != model.FileScanPass {
		t.Fatalf("unexpected audit detail: %v", detail)
	}
	detailText := string(log.DetailJSON)
	for _, forbidden := range []string{"object_key", "uploads", "token", string(fixture.fileBytes)} {
		if strings.Contains(detailText, forbidden) {
			t.Fatalf("audit detail contains %q: %s", forbidden, detailText)
		}
	}
}

func TestAdminLicenseContentReturnsUniformNotFoundForUndisclosableFiles(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *licensePrivacyFixture)
	}{
		{name: "missing record", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := f.srv.DB.Delete(&f.file).Error; err != nil {
				t.Fatalf("delete file record: %v", err)
			}
		}},
		{name: "wrong type", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := f.srv.DB.Model(&f.file).Update("biz_type", model.FileBizProductImage).Error; err != nil {
				t.Fatalf("set wrong type: %v", err)
			}
		}},
		{name: "non pass", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := f.srv.DB.Model(&f.file).Update("scan_status", model.FileScanPending).Error; err != nil {
				t.Fatalf("set pending scan: %v", err)
			}
		}},
		{name: "empty object key", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := f.srv.DB.Model(&f.file).Update("object_key", "").Error; err != nil {
				t.Fatalf("clear object key: %v", err)
			}
		}},
		{name: "invalid mime", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := f.srv.DB.Model(&f.file).Update("mime_type", "application/pdf").Error; err != nil {
				t.Fatalf("set invalid mime: %v", err)
			}
		}},
		{name: "noncanonical mime", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := f.srv.DB.Model(&f.file).Update("mime_type", " image/jpeg ").Error; err != nil {
				t.Fatalf("set noncanonical mime: %v", err)
			}
		}},
		{name: "public object key prefix", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			publicKey := fmt.Sprintf("product_image/license-%d.jpg", time.Now().UnixNano())
			if err := f.srv.DB.Model(&f.file).Update("object_key", publicKey).Error; err != nil {
				t.Fatalf("set public object key: %v", err)
			}
			writeUploadFixture(t, f.uploadDir, publicKey, f.fileBytes)
		}},
		{name: "owner missing", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := f.srv.DB.Model(&f.file).Update("owner_merchant_id", nil).Error; err != nil {
				t.Fatalf("clear owner: %v", err)
			}
		}},
		{name: "merchant reference missing", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := f.srv.DB.Model(&f.merchant).Update("license_file_id", nil).Error; err != nil {
				t.Fatalf("clear merchant reference: %v", err)
			}
		}},
		{name: "merchant reference mismatch", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			otherOwner := f.merchant.ID + 1000
			if err := f.srv.DB.Model(&f.file).Update("owner_merchant_id", otherOwner).Error; err != nil {
				t.Fatalf("set mismatched owner: %v", err)
			}
		}},
		{name: "physical file missing", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := os.Remove(filepath.Join(f.uploadDir, filepath.FromSlash(f.file.ObjectKey))); err != nil {
				t.Fatalf("remove physical file: %v", err)
			}
		}},
		{name: "traversal object key", mutate: func(t *testing.T, f *licensePrivacyFixture) {
			if err := f.srv.DB.Model(&f.file).Update("object_key", "merchant_license/../outside.jpg").Error; err != nil {
				t.Fatalf("set traversal object key: %v", err)
			}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newLicensePrivacyFixture(t)
			tc.mutate(t, &fixture)
			w := requestAdminLicenseContent(t, fixture, fixture.adminToken)
			requireAPIErrorResponse(t, w, http.StatusNotFound, common.CodeNotFound, fixture.fileBytes)
			var count int64
			if err := fixture.srv.DB.Model(&model.OperationLog{}).Where("action = ?", "admin_file_read").Count(&count).Error; err != nil {
				t.Fatalf("count read logs: %v", err)
			}
			if count != 0 {
				t.Fatalf("undisclosable file created %d read logs", count)
			}
		})
	}
}

func TestAdminLicenseContentRequiresActiveAdminSession(t *testing.T) {
	fixture := newLicensePrivacyFixture(t)

	unauthenticated := requestAdminLicenseContent(t, fixture, "")
	requireAPIErrorResponse(t, unauthenticated, http.StatusUnauthorized, common.CodeUnauthorized, fixture.fileBytes)

	merchantID, username, password := registerMerchant(t, fixture.srv, "license_content_auth")
	_ = merchantID
	merchantLoginResponse := merchantLogin(t, fixture.srv, username, password)
	merchant := requestAdminLicenseContent(t, fixture, str(merchantLoginResponse.Data["access_token"]))
	requireAPIErrorResponse(t, merchant, http.StatusForbidden, common.CodeForbidden, fixture.fileBytes)

	buyerLoginResponse := requestJSON(t, fixture.srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "license-content-buyer", "device_id": "license-content-device",
	}, nil)
	if buyerLoginResponse.Code != common.CodeOK {
		t.Fatalf("buyer login: %+v", buyerLoginResponse)
	}
	buyer := requestAdminLicenseContent(t, fixture, str(buyerLoginResponse.Data["access_token"]))
	requireAPIErrorResponse(t, buyer, http.StatusForbidden, common.CodeForbidden, fixture.fileBytes)

	now := time.Now()
	if err := fixture.srv.DB.Model(&model.AuthSession{}).
		Where("user_type = ? AND user_id = ? AND revoked_at IS NULL", model.UserTypeAdmin, fixture.admin.ID).
		Update("revoked_at", &now).Error; err != nil {
		t.Fatalf("revoke admin session: %v", err)
	}
	revoked := requestAdminLicenseContent(t, fixture, fixture.adminToken)
	requireAPIErrorResponse(t, revoked, http.StatusUnauthorized, common.CodeUnauthorized, fixture.fileBytes)

	fresh := requestAdminLicenseContent(t, fixture, adminAccessToken(t, fixture.srv))
	if fresh.Code != http.StatusOK || !bytes.Equal(fresh.Body.Bytes(), fixture.fileBytes) {
		t.Fatalf("fresh active admin response: status=%d body=%q", fresh.Code, fresh.Body.String())
	}
}

func TestAdminLicenseContentFailsClosedForNonLocalStorage(t *testing.T) {
	srv, uploadDir := newTestServerWithStorage(t, "remote")
	fixture := newLicensePrivacyFixtureForServer(t, srv, uploadDir)
	w := requestAdminLicenseContent(t, fixture, fixture.adminToken)
	requireAPIErrorResponse(t, w, http.StatusInternalServerError, common.CodeInternal, fixture.fileBytes)
}

func TestAdminLicenseContentReturnsInternalWithoutBytesWhenAuditFails(t *testing.T) {
	fixture := newLicensePrivacyFixture(t)
	callbackName := fmt.Sprintf("test:fail_admin_file_read_%d", time.Now().UnixNano())
	err := fixture.srv.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "operation_logs" {
			tx.AddError(errors.New("forced operation log failure"))
		}
	})
	if err != nil {
		t.Fatalf("register audit failure callback: %v", err)
	}
	t.Cleanup(func() {
		_ = fixture.srv.DB.Callback().Create().Remove(callbackName)
	})

	w := requestAdminLicenseContent(t, fixture, fixture.adminToken)
	requireAPIErrorResponse(t, w, http.StatusInternalServerError, common.CodeInternal, fixture.fileBytes)
	for _, name := range []string{"Content-Disposition", "Cache-Control", "Pragma", "X-Content-Type-Options"} {
		if got := w.Header().Get(name); got != "" {
			t.Errorf("%s must be absent on audit failure, got %q", name, got)
		}
	}
	var count int64
	if err := fixture.srv.DB.Model(&model.OperationLog{}).Where("action = ?", "admin_file_read").Count(&count).Error; err != nil {
		t.Fatalf("count failed read logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed audit persisted %d read logs", count)
	}
}
