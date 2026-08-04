package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

type sessionTestTokens struct {
	access  string
	refresh string
}

type sessionLifecycleFixture struct {
	login         func(*testing.T, *app.Server) sessionTestTokens
	accessPath    string
	logoutPath    string
	refreshPath   string
	accessHeaders map[string]string
}

func TestSessionLogoutRevokesOnlyCurrentSession(t *testing.T) {
	tests := []struct {
		name    string
		fixture func(*testing.T, *app.Server) sessionLifecycleFixture
	}{
		{
			name: "admin",
			fixture: func(_ *testing.T, _ *app.Server) sessionLifecycleFixture {
				return sessionLifecycleFixture{
					login:       sessionAdminLogin,
					accessPath:  "/api/v1/admin/logs",
					logoutPath:  "/api/v1/auth/logout",
					refreshPath: "/api/v1/auth/refresh",
				}
			},
		},
		{
			name: "merchant",
			fixture: func(t *testing.T, srv *app.Server) sessionLifecycleFixture {
				adminToken := adminAccessToken(t, srv)
				merchantID, username, password := registerMerchant(t, srv, "session_lifecycle")
				approveMerchant(t, srv, adminToken, merchantID)
				return sessionLifecycleFixture{
					login: func(t *testing.T, srv *app.Server) sessionTestTokens {
						response := merchantLogin(t, srv, username, password)
						return sessionTokensFromResponse(t, response)
					},
					accessPath:  "/api/v1/merchant/account",
					logoutPath:  "/api/v1/auth/logout",
					refreshPath: "/api/v1/auth/refresh",
				}
			},
		},
		{
			name: "buyer",
			fixture: func(_ *testing.T, _ *app.Server) sessionLifecycleFixture {
				loginCount := 0
				return sessionLifecycleFixture{
					login: func(t *testing.T, srv *app.Server) sessionTestTokens {
						loginCount++
						response := requestJSON(
							t,
							srv.Router,
							http.MethodPost,
							"/api/v1/buyer/auth/wechat-login",
							map[string]interface{}{
								"code":      "session-lifecycle-buyer",
								"device_id": fmt.Sprintf("session-device-%d", loginCount),
								"nickname":  "session buyer",
							},
							nil,
						)
						return sessionTokensFromResponse(t, response)
					},
					accessPath:  "/api/v1/buyer/intents",
					logoutPath:  "/api/v1/buyer/auth/logout",
					refreshPath: "/api/v1/buyer/auth/refresh",
					accessHeaders: map[string]string{
						"X-Device-Id": "session-device-current",
					},
				}
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			srv := newTestServer(t)
			fixture := testCase.fixture(t, srv)
			current := fixture.login(t, srv)
			sibling := fixture.login(t, srv)

			assertSessionAccessCode(t, srv, fixture, current.access, common.CodeOK)
			assertSessionAccessCode(t, srv, fixture, sibling.access, common.CodeOK)

			logout := requestJSON(
				t,
				srv.Router,
				http.MethodPost,
				fixture.logoutPath,
				map[string]interface{}{},
				map[string]string{"Authorization": "Bearer " + current.access},
			)
			if logout.Code != common.CodeOK {
				t.Fatalf("logout failed: %+v", logout)
			}

			assertSessionAccessCode(t, srv, fixture, current.access, common.CodeUnauthorized)
			assertSessionRefreshCode(t, srv, fixture.refreshPath, current.refresh, common.CodeUnauthorized)
			assertSessionAccessCode(t, srv, fixture, sibling.access, common.CodeOK)
			assertSessionRefreshCode(t, srv, fixture.refreshPath, sibling.refresh, common.CodeOK)
		})
	}
}

func TestSessionAccessUsesCurrentAccountState(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	merchantID, username, password := registerMerchant(t, srv, "session_current_state")
	approveMerchant(t, srv, adminToken, merchantID)
	login := merchantLogin(t, srv, username, password)
	tokens := sessionTokensFromResponse(t, login)

	claims, err := auth.ParseAccessToken("test-access", tokens.access)
	if err != nil {
		t.Fatal("parse isolated access token")
	}
	staleToken, _, err := auth.BuildAccessToken("test-access", auth.AccessClaims{
		UserID:     claims.UserID,
		UserType:   claims.UserType,
		Role:       "STALE_ROLE",
		MerchantID: merchantID + 999,
		Scope:      "onboarding",
		SessionID:  claims.SessionID,
	}, time.Hour)
	if err != nil {
		t.Fatal("build stale isolated access token")
	}

	allowed := requestJSON(
		t,
		srv.Router,
		http.MethodGet,
		"/api/v1/merchant/products",
		nil,
		map[string]string{"Authorization": "Bearer " + staleToken},
	)
	if allowed.Code != common.CodeOK {
		t.Fatalf("current approved state did not override stale claims: %+v", allowed)
	}

	if result := srv.DB.Model(&model.Merchant{}).
		Where("id = ?", merchantID).
		Update("review_status", model.ReviewRejected); result.Error != nil || result.RowsAffected != 1 {
		t.Fatal("downgrade isolated merchant review state")
	}
	downgraded := requestJSON(
		t,
		srv.Router,
		http.MethodGet,
		"/api/v1/merchant/products",
		nil,
		map[string]string{"Authorization": "Bearer " + staleToken},
	)
	if downgraded.Code != common.CodeReviewNotApproved {
		t.Fatalf("stale full scope survived current review downgrade: %+v", downgraded)
	}

	var account model.MerchantAccount
	if err := srv.DB.Where("id = ?", claims.UserID).First(&account).Error; err != nil {
		t.Fatal("load isolated merchant account")
	}
	if result := srv.DB.Model(&model.MerchantAccount{}).
		Where("id = ?", account.ID).
		Update("status", model.AccountStatusDisabled); result.Error != nil || result.RowsAffected != 1 {
		t.Fatal("disable isolated merchant account")
	}
	disabled := requestJSON(
		t,
		srv.Router,
		http.MethodGet,
		"/api/v1/merchant/profile",
		nil,
		map[string]string{"Authorization": "Bearer " + staleToken},
	)
	if disabled.Code != common.CodeAccountDisabled {
		t.Fatalf("disabled account retained access through stale claims: %+v", disabled)
	}
}

func TestSessionIdentityMismatchFailsClosed(t *testing.T) {
	srv := newTestServer(t)
	tokens := sessionAdminLogin(t, srv)
	claims, err := auth.ParseAccessToken("test-access", tokens.access)
	if err != nil {
		t.Fatal("parse isolated admin access token")
	}

	mismatchedTypeToken, _, err := auth.BuildAccessToken("test-access", auth.AccessClaims{
		UserID:     claims.UserID,
		UserType:   model.UserTypeMerchant,
		Role:       model.AccountRoleOwner,
		MerchantID: claims.UserID,
		Scope:      "full",
		SessionID:  claims.SessionID,
	}, time.Hour)
	if err != nil {
		t.Fatal("build mismatched user type token")
	}
	typeMismatch := requestJSON(
		t,
		srv.Router,
		http.MethodGet,
		"/api/v1/merchant/profile",
		nil,
		map[string]string{"Authorization": "Bearer " + mismatchedTypeToken},
	)
	if typeMismatch.Code != common.CodeUnauthorized {
		t.Fatalf("same numeric ID crossed user types: %+v", typeMismatch)
	}

	mismatchedUserToken, _, err := auth.BuildAccessToken("test-access", auth.AccessClaims{
		UserID:    claims.UserID + 1,
		UserType:  claims.UserType,
		Role:      claims.Role,
		Scope:     claims.Scope,
		SessionID: claims.SessionID,
	}, time.Hour)
	if err != nil {
		t.Fatal("build mismatched user ID token")
	}
	userMismatch := requestJSON(
		t,
		srv.Router,
		http.MethodGet,
		"/api/v1/admin/logs",
		nil,
		map[string]string{"Authorization": "Bearer " + mismatchedUserToken},
	)
	if userMismatch.Code != common.CodeUnauthorized {
		t.Fatalf("session accepted mismatched user ID: %+v", userMismatch)
	}

	if result := srv.DB.Model(&model.AuthSession{}).
		Where("id = ?", claims.SessionID).
		Update("user_type", model.UserTypeMerchant); result.Error != nil || result.RowsAffected != 1 {
		t.Fatal("mutate isolated session identity")
	}
	accessAfterSessionDrift := requestJSON(
		t,
		srv.Router,
		http.MethodGet,
		"/api/v1/admin/logs",
		nil,
		map[string]string{"Authorization": "Bearer " + tokens.access},
	)
	if accessAfterSessionDrift.Code != common.CodeUnauthorized {
		t.Fatalf("access survived session identity drift: %+v", accessAfterSessionDrift)
	}
	assertSessionRefreshCode(t, srv, "/api/v1/auth/refresh", tokens.refresh, common.CodeUnauthorized)
}

func TestAnonymousRequestSkipsSessionLookupAndDatabaseErrorsAreRedacted(t *testing.T) {
	srv := newTestServer(t)
	tokens := sessionAdminLogin(t, srv)
	if err := srv.DB.Migrator().DropTable(&model.AuthSession{}); err != nil {
		t.Fatal("remove isolated session table")
	}

	anonymous := requestJSON(t, srv.Router, http.MethodGet, "/healthz", nil, nil)
	if anonymous.Code != common.CodeOK || anonymous.HTTPStatus != http.StatusOK {
		t.Fatalf("anonymous request queried auth sessions: %+v", anonymous)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs", nil)
	request.Header.Set("Authorization", "Bearer "+tokens.access)
	recorder := httptest.NewRecorder()
	srv.Router.ServeHTTP(recorder, request)
	var response common.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal("decode database failure response")
	}
	if recorder.Code != http.StatusInternalServerError ||
		response.Code != common.CodeInternal ||
		response.Message != common.ErrInternal.Message {
		t.Fatalf("database failure did not fail closed: status=%d response=%+v", recorder.Code, response)
	}
	lowerBody := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{"no such table", "auth_sessions", "gorm", "sql"} {
		if strings.Contains(lowerBody, forbidden) {
			t.Fatalf("database failure leaked internal detail: %s", recorder.Body.String())
		}
	}
}

func TestSessionImmediateRefreshSucceeds(t *testing.T) {
	srv := newTestServer(t)
	for _, testCase := range []struct {
		name        string
		login       func(*testing.T, *app.Server) sessionTestTokens
		refreshPath string
	}{
		{name: "admin", login: sessionAdminLogin, refreshPath: "/api/v1/auth/refresh"},
		{
			name: "buyer",
			login: func(t *testing.T, srv *app.Server) sessionTestTokens {
				response := requestJSON(
					t,
					srv.Router,
					http.MethodPost,
					"/api/v1/buyer/auth/wechat-login",
					map[string]interface{}{
						"code": "immediate-refresh", "device_id": "immediate-device",
					},
					nil,
				)
				return sessionTokensFromResponse(t, response)
			},
			refreshPath: "/api/v1/buyer/auth/refresh",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tokens := testCase.login(t, srv)
			assertSessionRefreshCode(t, srv, testCase.refreshPath, tokens.refresh, common.CodeOK)
		})
	}
}

func sessionAdminLogin(t *testing.T, srv *app.Server) sessionTestTokens {
	t.Helper()
	response := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		"/api/v1/auth/login",
		map[string]interface{}{
			"login_type": model.UserTypeAdmin,
			"username":   "admin",
			"password":   "Admin@123456",
		},
		nil,
	)
	return sessionTokensFromResponse(t, response)
}

func sessionTokensFromResponse(t *testing.T, response apiResp) sessionTestTokens {
	t.Helper()
	if response.Code != common.CodeOK {
		t.Fatalf("session login failed: %+v", response)
	}
	tokens := sessionTestTokens{
		access:  str(response.Data["access_token"]),
		refresh: str(response.Data["refresh_token"]),
	}
	if tokens.access == "" || tokens.refresh == "" {
		t.Fatalf("session login returned incomplete tokens: %+v", response)
	}
	return tokens
}

func assertSessionAccessCode(
	t *testing.T,
	srv *app.Server,
	fixture sessionLifecycleFixture,
	accessToken string,
	wantCode int,
) {
	t.Helper()
	headers := map[string]string{"Authorization": "Bearer " + accessToken}
	for key, value := range fixture.accessHeaders {
		headers[key] = value
	}
	response := requestJSON(t, srv.Router, http.MethodGet, fixture.accessPath, nil, headers)
	if response.Code != wantCode {
		t.Fatalf("access code = %d, want %d: %+v", response.Code, wantCode, response)
	}
}

func assertSessionRefreshCode(
	t *testing.T,
	srv *app.Server,
	refreshPath string,
	refreshToken string,
	wantCode int,
) {
	t.Helper()
	response := requestJSON(
		t,
		srv.Router,
		http.MethodPost,
		refreshPath,
		map[string]interface{}{"refresh_token": refreshToken},
		nil,
	)
	if response.Code != wantCode {
		t.Fatalf("refresh code = %d, want %d: %+v", response.Code, wantCode, response)
	}
}
