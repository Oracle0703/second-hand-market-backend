package tests

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/model"
)

const securityOldPassword = "ExistingSafePass123!"
const securityNewPassword = "UpdatedSecurePass456!"

func securityAdminLogin(t *testing.T, s *app.Server, username, password string) apiResp {
	t.Helper()
	return requestJSON(t, s.Router, http.MethodPost, "/api/v1/auth/login", map[string]string{"login_type": "ADMIN", "username": username, "password": password}, nil)
}

func securityAdmin(t *testing.T, s *app.Server, role string) (uint64, apiResp) {
	t.Helper()
	if err := app.BootstrapAdmin(s.DB, app.AdminBootstrap{Username: "security_admin", DisplayName: "Security Admin", Role: role, Password: securityOldPassword}); err != nil {
		t.Fatal(err)
	}
	login := securityAdminLogin(t, s, "security_admin", securityOldPassword)
	if login.Code != 0 {
		t.Fatal("fixture login failed")
	}
	return numToUint64(login.Data["user"].(map[string]interface{})["id"]), login
}

func securityChange(t *testing.T, s *app.Server, token, oldPassword, newPassword string) apiResp {
	t.Helper()
	return requestJSON(t, s.Router, http.MethodPut, "/api/v1/admin/account/password", map[string]string{"old_password": oldPassword, "new_password": newPassword}, map[string]string{"Authorization": "Bearer " + token})
}

func TestAdminPasswordLifecycleAndIdentityIsolation(t *testing.T) {
	for _, role := range []string{model.AdminRoleAdmin, model.AdminRoleSuper} {
		t.Run(role, func(t *testing.T) {
			s := newTestServer(t)
			id, first := securityAdmin(t, s, role)
			second := securityAdminLogin(t, s, "security_admin", securityOldPassword)
			otherToken := adminAccessToken(t, s)
			merchant := model.Merchant{ID: 100, MerchantNo: "M-security", MerchantName: "Protected merchant", ReviewStatus: model.ReviewApproved}
			if err := s.DB.Create(&merchant).Error; err != nil {
				t.Fatal(err)
			}
			hash, _ := bcrypt.GenerateFromPassword([]byte("ProtectedOwner123!"), bcrypt.MinCost)
			protected := model.MerchantAccount{ID: id, MerchantID: merchant.ID, Username: "yaner", PasswordHash: string(hash), Role: model.AccountRoleOwner, Status: model.AccountStatusActive}
			if err := s.DB.Create(&protected).Error; err != nil {
				t.Fatal(err)
			}
			buyer := model.BuyerUser{ID: id, OpenID: "protected-buyer", Status: model.BuyerStatusActive}
			if err := s.DB.Create(&buyer).Error; err != nil {
				t.Fatal(err)
			}
			var protectedSessions []model.AuthSession
			for _, kind := range []string{model.UserTypeMerchant, model.UserTypeBuyer} {
				session := model.AuthSession{UserType: kind, UserID: id, RefreshTokenHash: "protected-refresh-hash", ExpiredAt: time.Now().Add(time.Hour)}
				if err := s.DB.Create(&session).Error; err != nil {
					t.Fatal(err)
				}
				protectedSessions = append(protectedSessions, session)
				token, _, err := auth.BuildAccessToken("test-access", auth.AccessClaims{UserID: id, UserType: kind, SessionID: session.ID, Role: protected.Role, MerchantID: merchant.ID, Scope: "full"}, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				if r := securityChange(t, s, token, securityOldPassword, securityNewPassword); r.HTTPStatus != http.StatusForbidden {
					t.Fatalf("non-admin status=%d", r.HTTPStatus)
				}
			}
			if r := securityChange(t, s, "", securityOldPassword, securityNewPassword); r.HTTPStatus != http.StatusUnauthorized {
				t.Fatal("anonymous request accepted")
			}
			var accountsBefore []model.AdminUser
			s.DB.Where("id <> ?", id).Order("id").Find(&accountsBefore)
			var sessionsBefore []model.AuthSession
			s.DB.Where("user_type <> ? OR user_id <> ?", model.UserTypeAdmin, id).Order("id").Find(&sessionsBefore)
			protectedBefore, _ := json.Marshal(protected)
			accountsSnapshot, _ := json.Marshal(accountsBefore)
			sessionsSnapshot, _ := json.Marshal(sessionsBefore)
			token := str(first.Data["access_token"])
			info := requestJSON(t, s.Router, http.MethodGet, "/api/v1/admin/account", nil, map[string]string{"Authorization": "Bearer " + token})
			encoded, _ := json.Marshal(info.Data)
			if info.Code != 0 || strings.Contains(string(encoded), "password") {
				t.Fatal("account response invalid or contains password data")
			}
			// Client-supplied target fields cannot select another administrator.
			result := requestJSON(t, s.Router, http.MethodPut, "/api/v1/admin/account/password", map[string]interface{}{"old_password": securityOldPassword, "new_password": securityNewPassword, "id": 1, "username": "superadmin"}, map[string]string{"Authorization": "Bearer " + token})
			if result.Code != 0 || result.Data["success"] != true {
				t.Fatalf("change failed: code=%d", result.Code)
			}
			for _, login := range []apiResp{first, second} {
				if r := requestJSON(t, s.Router, http.MethodGet, "/api/v1/admin/account", nil, map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])}); r.HTTPStatus != http.StatusUnauthorized {
					t.Fatal("old access token survived")
				}
				if r := requestJSON(t, s.Router, http.MethodPost, "/api/v1/auth/refresh", map[string]string{"refresh_token": str(login.Data["refresh_token"])}, nil); r.HTTPStatus != http.StatusUnauthorized {
					t.Fatal("old refresh token survived")
				}
			}
			if securityAdminLogin(t, s, "security_admin", securityOldPassword).Code == 0 {
				t.Fatal("old password still works")
			}
			if securityAdminLogin(t, s, "security_admin", securityNewPassword).Code != 0 {
				t.Fatal("new password failed")
			}
			if r := requestJSON(t, s.Router, http.MethodGet, "/api/v1/admin/account", nil, map[string]string{"Authorization": "Bearer " + otherToken}); r.Code != 0 {
				t.Fatal("another administrator was revoked")
			}
			var after model.MerchantAccount
			s.DB.First(&after, id)
			afterJSON, _ := json.Marshal(after)
			if string(afterJSON) != string(protectedBefore) {
				t.Fatal("protected merchant changed")
			}
			var accountsAfter []model.AdminUser
			s.DB.Where("id <> ?", id).Order("id").Find(&accountsAfter)
			afterJSON, _ = json.Marshal(accountsAfter)
			if string(afterJSON) != string(accountsSnapshot) {
				t.Fatal("another admin changed")
			}
			var sessionsAfter []model.AuthSession
			s.DB.Where("user_type <> ? OR user_id <> ?", model.UserTypeAdmin, id).Order("id").Find(&sessionsAfter)
			afterJSON, _ = json.Marshal(sessionsAfter)
			if string(afterJSON) != string(sessionsSnapshot) {
				t.Fatal("unrelated sessions changed")
			}
			var logs []model.OperationLog
			if err := s.DB.Where("action = ?", "admin_password_change").Find(&logs).Error; err != nil || len(logs) != 1 {
				t.Fatal("missing audit event")
			}
			logJSON, _ := json.Marshal(logs)
			for _, secret := range []string{securityOldPassword, securityNewPassword, "password_hash", "$2a$", "$2b$"} {
				if strings.Contains(string(logJSON), secret) {
					t.Fatal("password material in audit event")
				}
			}
		})
	}
}

func TestAdminPasswordValidationAndInactiveAccounts(t *testing.T) {
	s := newTestServer(t)
	id, login := securityAdmin(t, s, model.AdminRoleAdmin)
	token := str(login.Data["access_token"])
	for _, tc := range []struct{ name, old, new string }{
		{"wrong_old", "WrongExisting123!", securityNewPassword},
		{"empty_old", "", securityNewPassword},
		{"short", securityOldPassword, "Short12!"},
		{"no_digit", securityOldPassword, "OnlyLettersHere"},
		{"whitespace", securityOldPassword, "Password With123"},
		{"unicode", securityOldPassword, "密码Password12345"},
		{"over_bcrypt_limit", securityOldPassword, strings.Repeat("a", 72) + "1"},
		{"reuse", securityOldPassword, securityOldPassword},
		{"public_initial", securityOldPassword, "Admin@123456"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if r := securityChange(t, s, token, tc.old, tc.new); r.HTTPStatus != http.StatusBadRequest {
				t.Fatalf("status=%d", r.HTTPStatus)
			}
		})
	}
	if r := requestJSON(t, s.Router, http.MethodGet, "/api/v1/admin/account", nil, map[string]string{"Authorization": "Bearer " + token}); r.Code != 0 {
		t.Fatal("invalid changes revoked session")
	}
	if err := s.DB.Model(&model.AdminUser{}).Where("id = ?", id).Update("status", model.AccountStatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if r := securityChange(t, s, token, securityOldPassword, securityNewPassword); r.HTTPStatus != http.StatusUnauthorized {
		t.Fatal("disabled admin changed password")
	}
}

func TestAdminPasswordTransactionRollsBack(t *testing.T) {
	for _, failure := range []string{"session_revoke", "audit"} {
		t.Run(failure, func(t *testing.T) {
			s := newTestServer(t)
			id, login := securityAdmin(t, s, model.AdminRoleAdmin)
			callback := "test:admin_password_rollback"
			if failure == "session_revoke" {
				s.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "AuthSession" {
						tx.AddError(errors.New("simulated session write failure"))
					}
				})
				defer s.DB.Callback().Update().Remove(callback)
			} else {
				s.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "OperationLog" {
						tx.AddError(errors.New("simulated audit failure"))
					}
				})
				defer s.DB.Callback().Create().Remove(callback)
			}
			if r := securityChange(t, s, str(login.Data["access_token"]), securityOldPassword, securityNewPassword); r.HTTPStatus != http.StatusInternalServerError {
				t.Fatalf("status=%d", r.HTTPStatus)
			}
			var account model.AdminUser
			s.DB.First(&account, id)
			if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(securityOldPassword)) != nil {
				t.Fatal("password committed despite failure")
			}
			var sessions []model.AuthSession
			s.DB.Where("user_type = ? AND user_id = ?", model.UserTypeAdmin, id).Find(&sessions)
			for _, session := range sessions {
				if session.RevokedAt != nil {
					t.Fatal("session committed despite failure")
				}
			}
			if r := requestJSON(t, s.Router, http.MethodGet, "/api/v1/admin/account", nil, map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])}); r.Code != 0 {
				t.Fatal("old access no longer works after rollback")
			}
		})
	}
}
