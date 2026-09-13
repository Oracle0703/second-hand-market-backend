package tests

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
	"testing"
	"time"
)

func TestAdminProvisioningAndPasswordLifecycle(t *testing.T) {
	srv := newTestServer(t)
	admin := map[string]string{"Authorization": "Bearer " + adminAccessToken(t, srv)}
	payload := map[string]interface{}{"merchant_name": "Custom merchant", "contact_name": "Owner", "phone": "13800138000", "username": "custom_owner", "password": "InitialPassword!2026"}
	if got := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/admin/merchants", payload, nil); got.Code != common.CodeUnauthorized {
		t.Fatalf("anonymous provisioning: %+v", got)
	}
	payload["password"] = "weakpassword"
	if got := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/admin/merchants", payload, admin); got.Code != common.CodeInvalidArgument {
		t.Fatalf("weak password: %+v", got)
	}
	payload["password"] = "InitialPassword!2026"
	created := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/admin/merchants", payload, admin)
	if created.Code != 0 {
		t.Fatalf("create: %+v", created)
	}
	id := numToUint64(created.Data["merchant_id"])
	var merchant model.Merchant
	if err := srv.DB.First(&merchant, id).Error; err != nil {
		t.Fatal(err)
	}
	if merchant.LicenseFileID != nil || merchant.ReviewStatus != model.ReviewApproved {
		t.Fatalf("unexpected merchant: %+v", merchant)
	}
	login := merchantLogin(t, srv, "custom_owner", "InitialPassword!2026")
	if login.Code != 0 || login.Data["user"].(map[string]interface{})["must_change_password"] != true {
		t.Fatalf("initial login: %+v", login)
	}
	initial := map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])}
	if got := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/products", nil, initial); got.Code != common.CodeForbidden {
		t.Fatalf("initial password bypass: %+v", got)
	}
	changed := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/merchant/account/password", map[string]interface{}{"old_password": "InitialPassword!2026", "new_password": "ReplacementPass!2026"}, initial)
	if changed.Code != 0 {
		t.Fatalf("change: %+v", changed)
	}
	if got := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/account", nil, initial); got.Code != common.CodeUnauthorized {
		t.Fatalf("old access survives: %+v", got)
	}
	if got := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/refresh", map[string]interface{}{"refresh_token": login.Data["refresh_token"]}, nil); got.Code != common.CodeUnauthorized {
		t.Fatalf("old refresh survives: %+v", got)
	}
	login = merchantLogin(t, srv, "custom_owner", "ReplacementPass!2026")
	active := map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])}
	if got := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/products", nil, active); got.Code != 0 {
		t.Fatalf("business access: %+v", got)
	}
	if got := requestJSON(t, srv.Router, http.MethodPut, fmt.Sprintf("/api/v1/admin/merchants/%d/password", id), map[string]interface{}{"password": "ResetPassword!2026"}, active); got.Code != common.CodeForbidden {
		t.Fatalf("merchant reset admin bypass: %+v", got)
	}
	reset := requestJSON(t, srv.Router, http.MethodPut, fmt.Sprintf("/api/v1/admin/merchants/%d/password", id), map[string]interface{}{"password": "ResetPassword!2026"}, admin)
	if reset.Code != 0 {
		t.Fatalf("reset: %+v", reset)
	}
	if got := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/account", nil, active); got.Code != common.CodeUnauthorized {
		t.Fatalf("reset access survives: %+v", got)
	}
	login = merchantLogin(t, srv, "custom_owner", "ResetPassword!2026")
	if login.Code != 0 || login.Data["user"].(map[string]interface{})["must_change_password"] != true {
		t.Fatalf("reset login: %+v", login)
	}
	disabled := requestJSON(t, srv.Router, http.MethodPut, fmt.Sprintf("/api/v1/admin/merchants/%d/status", id), map[string]interface{}{"status": "DISABLED"}, admin)
	if disabled.Code != 0 {
		t.Fatalf("disable: %+v", disabled)
	}
	if got := merchantLogin(t, srv, "custom_owner", "ResetPassword!2026"); got.Code != common.CodeAccountDisabled {
		t.Fatalf("disabled login: %+v", got)
	}
	var logs []model.MerchantAuditLog
	if err := srv.DB.Where("merchant_id = ?", id).Find(&logs).Error; err != nil || len(logs) < 3 {
		t.Fatalf("audit missing: %v %+v", err, logs)
	}
}

func TestRetiredRegistrationAndLicenseUploads(t *testing.T) {
	uploadDir := t.TempDir()
	srv := newTestServerWithUploadDir(t, uploadDir)
	for _, path := range []string{"/api/v1/auth/register", "/api/v1/merchant/reapply"} {
		w := httptest.NewRecorder()
		srv.Router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("retired route %s: %d", path, w.Code)
		}
	}
	for _, headers := range []map[string]string{nil, {"Authorization": "Bearer " + adminAccessToken(t, srv)}} {
		response := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": 1000, "mime_type": "image/jpeg"}, headers)
		if response.Code != common.CodeForbidden {
			t.Fatalf("license collection still open: %+v", response)
		}
	}
	// Old pending public records cannot be confirmed or uploaded after retirement.
	file := model.FileRecord{BizType: model.FileBizMerchantLicense, ObjectKey: "merchant_license/old.jpg", MimeType: "image/jpeg", UploaderType: model.UserTypePublic, ScanStatus: model.FileScanPending, CreatedAt: time.Now()}
	if err := srv.DB.Create(&file).Error; err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/confirm", map[string]interface{}{"file_id": file.ID, "object_key": file.ObjectKey}, nil)
	if response.Code != common.CodeForbidden {
		t.Fatalf("old license confirm: %+v", response)
	}
	// Verify the actual historical bytes are accessible only to administrators.
	path := filepath.Join(uploadDir, file.ObjectKey)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encodedUploadImage(t, "image/jpeg"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		token  string
		status int
	}{{"", http.StatusNotFound}, {adminAccessToken(t, srv), http.StatusOK}} {
		req := httptest.NewRequest(http.MethodGet, "/uploads/"+file.ObjectKey, nil)
		if test.token != "" {
			req.Header.Set("Authorization", "Bearer "+test.token)
		}
		w := httptest.NewRecorder()
		srv.Router.ServeHTTP(w, req)
		if w.Code != test.status {
			t.Fatalf("historical license access status %d, want %d", w.Code, test.status)
		}
	}

}

func TestJSONBodyLimitWithoutContentLength(t *testing.T) {
	srv := newTestServer(t)
	body := append([]byte(`{"username":"`), bytes.Repeat([]byte("a"), 3<<20)...)
	body = append(body, []byte(`","password":"x","login_type":"ADMIN"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	if w.Code < 400 {
		t.Fatalf("unbounded JSON accepted: %d", w.Code)
	}
}
