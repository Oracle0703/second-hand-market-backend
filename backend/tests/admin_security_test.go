package tests

import (
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestAdminChangePasswordRevokesOnlyTargetAdminSessions(t *testing.T) {
	srv := newTestServer(t)

	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": 1000, "mime_type": "image/jpeg",
	}, nil)
	register := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register", map[string]interface{}{
		"merchant_name": "Yaner Store", "contact_name": "Yaner", "phone": "13800138001", "username": "yaner", "password": "YanerPass@2026", "license_file_id": numToUint64(presign.Data["file_id"]),
	}, nil)
	if register.Code != common.CodeOK {
		t.Fatalf("register yaner: %+v", register)
	}
	adminToken := adminAccessToken(t, srv)
	approveMerchant(t, srv, adminToken, numToUint64(register.Data["merchant_id"]))
	yanerLogin := merchantLogin(t, srv, "yaner", "YanerPass@2026")
	if yanerLogin.Code != common.CodeOK {
		t.Fatalf("login yaner: %+v", yanerLogin)
	}
	yanerToken := str(yanerLogin.Data["access_token"])
	var yaner model.MerchantAccount
	if err := srv.DB.Where("username = ?", "yaner").First(&yaner).Error; err != nil {
		t.Fatalf("load yaner: %v", err)
	}
	yanerHash := yaner.PasswordHash

	superLogin := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": "ADMIN", "username": "superadmin", "password": testAdminPassword,
	}, nil)
	if superLogin.Code != common.CodeOK {
		t.Fatalf("login superadmin: %+v", superLogin)
	}
	oldAccess := str(superLogin.Data["access_token"])
	oldRefresh := str(superLogin.Data["refresh_token"])
	newPassword := "DifferentAdmin@2026"
	change := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/admin/account/password", map[string]interface{}{
		"current_password": testAdminPassword,
		"new_password":     newPassword,
	}, map[string]string{"Authorization": "Bearer " + oldAccess})
	if change.Code != common.CodeOK {
		t.Fatalf("change password: %+v", change)
	}

	oldAccessResp := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/admin/logs", nil, map[string]string{"Authorization": "Bearer " + oldAccess})
	if oldAccessResp.Code != common.CodeUnauthorized {
		t.Fatalf("old access token should be rejected: %+v", oldAccessResp)
	}
	oldRefreshResp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/refresh", map[string]interface{}{"refresh_token": oldRefresh}, nil)
	if oldRefreshResp.Code != common.CodeUnauthorized {
		t.Fatalf("old refresh token should be rejected: %+v", oldRefreshResp)
	}
	oldPasswordLogin := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": "ADMIN", "username": "superadmin", "password": testAdminPassword,
	}, nil)
	if oldPasswordLogin.Code != common.CodeUnauthorized {
		t.Fatalf("old password should be rejected: %+v", oldPasswordLogin)
	}
	newPasswordLogin := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": "ADMIN", "username": "superadmin", "password": newPassword,
	}, nil)
	if newPasswordLogin.Code != common.CodeOK {
		t.Fatalf("new password login failed: %+v", newPasswordLogin)
	}

	adminStillWorks := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/admin/logs", nil, map[string]string{"Authorization": "Bearer " + adminToken})
	if adminStillWorks.Code != common.CodeOK {
		t.Fatalf("other administrator session was revoked: %+v", adminStillWorks)
	}
	yanerStillWorks := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/profile", nil, map[string]string{"Authorization": "Bearer " + yanerToken})
	if yanerStillWorks.Code != common.CodeOK {
		t.Fatalf("yaner session was affected: %+v", yanerStillWorks)
	}
	if err := srv.DB.Where("username = ?", "yaner").First(&yaner).Error; err != nil {
		t.Fatalf("reload yaner: %v", err)
	}
	if yaner.PasswordHash != yanerHash || bcrypt.CompareHashAndPassword([]byte(yaner.PasswordHash), []byte("YanerPass@2026")) != nil {
		t.Fatal("yaner password hash changed")
	}
}

func TestAdminChangePasswordValidation(t *testing.T) {
	srv := newTestServer(t)
	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": "ADMIN", "username": "admin", "password": testAdminPassword,
	}, nil)
	token := str(login.Data["access_token"])

	wrongCurrent := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/admin/account/password", map[string]interface{}{
		"current_password": "wrong-password",
		"new_password":     "AnotherAdmin@2026",
	}, map[string]string{"Authorization": "Bearer " + token})
	if wrongCurrent.Code != common.CodeInvalidArgument {
		t.Fatalf("wrong current password: %+v", wrongCurrent)
	}
	weak := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/admin/account/password", map[string]interface{}{
		"current_password": testAdminPassword,
		"new_password":     "short",
	}, map[string]string{"Authorization": "Bearer " + token})
	if weak.Code != common.CodeInvalidArgument {
		t.Fatalf("weak password: %+v", weak)
	}
	same := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/admin/account/password", map[string]interface{}{
		"current_password": testAdminPassword,
		"new_password":     testAdminPassword,
	}, map[string]string{"Authorization": "Bearer " + token})
	if same.Code != common.CodeInvalidArgument {
		t.Fatalf("same password: %+v", same)
	}
	legacyDefault := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/admin/account/password", map[string]interface{}{
		"current_password": testAdminPassword,
		"new_password":     "Admin@123456",
	}, map[string]string{"Authorization": "Bearer " + token})
	if legacyDefault.Code != common.CodeInvalidArgument {
		t.Fatalf("known default password: %+v", legacyDefault)
	}
}

func TestAdminChangePasswordAuthorization(t *testing.T) {
	srv := newTestServer(t)
	payload := map[string]interface{}{
		"current_password": testAdminPassword,
		"new_password":     "DifferentAdmin@2026",
	}

	unauthenticated := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/admin/account/password", payload, nil)
	if unauthenticated.Code != common.CodeUnauthorized {
		t.Fatalf("unauthenticated password change should be rejected: %+v", unauthenticated)
	}

	merchantID, username, password := registerMerchant(t, srv, "admin_password_auth")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	merchant := merchantLogin(t, srv, username, password)
	merchantAttempt := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/admin/account/password", payload, map[string]string{"Authorization": "Bearer " + str(merchant.Data["access_token"])})
	if merchantAttempt.Code != common.CodeForbidden {
		t.Fatalf("merchant password change should be forbidden: %+v", merchantAttempt)
	}

	buyer := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": "admin-password-authorization-buyer", "device_id": "admin-password-authorization-device",
	}, nil)
	if buyer.Code != common.CodeOK {
		t.Fatalf("buyer login: %+v", buyer)
	}
	buyerAttempt := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/admin/account/password", payload, map[string]string{"Authorization": "Bearer " + str(buyer.Data["access_token"])})
	if buyerAttempt.Code != common.CodeForbidden {
		t.Fatalf("buyer password change should be forbidden: %+v", buyerAttempt)
	}
}

func TestAdminSessionDatabaseFailureReturnsInternalError(t *testing.T) {
	srv := newTestServer(t)
	token := adminAccessToken(t, srv)
	sqlDB, err := srv.DB.DB()
	if err != nil {
		t.Fatalf("load database handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database handle: %v", err)
	}

	resp := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/admin/logs", nil, map[string]string{"Authorization": "Bearer " + token})
	if resp.Code != common.CodeInternal {
		t.Fatalf("database failure must not be reported as a missing session: %+v", resp)
	}
}
