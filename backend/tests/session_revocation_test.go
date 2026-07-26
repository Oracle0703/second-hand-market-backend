package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

type sessionTokenPair struct {
	access  string
	refresh string
}

type sessionHTTPResponse struct {
	HTTPStatus int
	Code       int
}

func sessionTokenPairFromResponse(t *testing.T, response apiResp) sessionTokenPair {
	t.Helper()
	if response.Code != common.CodeOK {
		t.Fatalf("login code = %d", response.Code)
	}
	pair := sessionTokenPair{
		access:  str(response.Data["access_token"]),
		refresh: str(response.Data["refresh_token"]),
	}
	if pair.access == "" || pair.refresh == "" {
		t.Fatal("login response omitted token pair")
	}
	return pair
}

func sessionIDFromAccess(t *testing.T, access string) uint64 {
	t.Helper()
	claims, err := auth.ParseAccessToken("test-access", access)
	if err != nil {
		t.Fatal("parse test access token")
	}
	if claims.SessionID == 0 {
		t.Fatal("test access token has zero session id")
	}
	return claims.SessionID
}

func assertSessionAccessCode(t *testing.T, srv *app.Server, path, access string, want int) {
	t.Helper()
	response := requestJSON(t, srv.Router, http.MethodGet, path, nil,
		map[string]string{"Authorization": "Bearer " + access})
	if response.Code != want {
		t.Fatalf("access code = %d, want %d", response.Code, want)
	}
}

func assertRefreshCode(t *testing.T, srv *app.Server, path, refresh string, want int) {
	t.Helper()
	response := requestJSON(t, srv.Router, http.MethodPost, path,
		map[string]interface{}{"refresh_token": refresh}, nil)
	if response.Code != want {
		t.Fatalf("refresh code = %d, want %d", response.Code, want)
	}
}

func assertOnlyFirstSessionRevoked(t *testing.T, srv *app.Server, firstID, secondID uint64) {
	t.Helper()
	var sessions []model.AuthSession
	if err := srv.DB.Where("id IN ?", []uint64{firstID, secondID}).Order("id ASC").Find(&sessions).Error; err != nil {
		t.Fatalf("load session revocation state: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("session row count = %d, want 2", len(sessions))
	}
	state := make(map[uint64]bool, len(sessions))
	for _, session := range sessions {
		state[session.ID] = session.RevokedAt != nil
	}
	if !state[firstID] || state[secondID] {
		t.Fatalf("revocation state first/second = %t/%t, want true/false", state[firstID], state[secondID])
	}
}

func requireSessionHTTPResponse(t *testing.T, label string, response sessionHTTPResponse, wantHTTP, wantCode int) {
	t.Helper()
	if response.HTTPStatus != wantHTTP || response.Code != wantCode {
		t.Fatalf("%s status/code = %d/%d, want %d/%d", label, response.HTTPStatus, response.Code, wantHTTP, wantCode)
	}
}

func verifySessionRevocationContract(
	t *testing.T,
	srv *app.Server,
	first sessionTokenPair,
	second sessionTokenPair,
	accessPath string,
	refreshPath string,
) {
	t.Helper()
	firstID := sessionIDFromAccess(t, first.access)
	secondID := sessionIDFromAccess(t, second.access)
	if firstID == secondID {
		t.Fatal("two logins reused one session")
	}

	assertSessionAccessCode(t, srv, accessPath, first.access, common.CodeOK)
	assertSessionAccessCode(t, srv, accessPath, second.access, common.CodeOK)
	logout := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/logout", nil,
		map[string]string{"Authorization": "Bearer " + first.access})
	if logout.Code != common.CodeOK {
		t.Fatalf("logout code = %d", logout.Code)
	}

	assertSessionAccessCode(t, srv, accessPath, first.access, common.CodeUnauthorized)
	assertRefreshCode(t, srv, refreshPath, first.refresh, common.CodeUnauthorized)
	assertSessionAccessCode(t, srv, accessPath, second.access, common.CodeOK)
	assertRefreshCode(t, srv, refreshPath, second.refresh, common.CodeOK)
	assertOnlyFirstSessionRevoked(t, srv, firstID, secondID)
}

func TestSessionRevocationForEveryActor(t *testing.T) {
	t.Run("administrator", func(t *testing.T) {
		srv := newTestServer(t)
		login := func() sessionTokenPair {
			return sessionTokenPairFromResponse(t, requestJSON(t, srv.Router, http.MethodPost,
				"/api/v1/auth/login", map[string]interface{}{
					"login_type": model.UserTypeAdmin,
					"username":   "admin",
					"password":   testAdminPassword,
				}, nil))
		}
		verifySessionRevocationContract(t, srv, login(), login(),
			"/api/v1/admin/logs", "/api/v1/auth/refresh")
	})

	t.Run("merchant", func(t *testing.T) {
		srv := newTestServer(t)
		merchantID, username, password := registerMerchant(t, srv, "f14_revoke")
		approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
		login := func() sessionTokenPair {
			return sessionTokenPairFromResponse(t, merchantLogin(t, srv, username, password))
		}
		verifySessionRevocationContract(t, srv, login(), login(),
			"/api/v1/merchant/profile", "/api/v1/auth/refresh")
	})

	t.Run("buyer", func(t *testing.T) {
		srv := newTestServer(t)
		login := func(device string) sessionTokenPair {
			return sessionTokenPairFromResponse(t, requestJSON(t, srv.Router, http.MethodPost,
				"/api/v1/buyer/auth/wechat-login", map[string]interface{}{
					"code": "f14-revoke-buyer", "device_id": device, "nickname": "F14 Buyer",
				}, nil))
		}
		verifySessionRevocationContract(t, srv, login("f14-device-1"), login("f14-device-2"),
			"/api/v1/buyer/intents", "/api/v1/buyer/auth/refresh")
	})
}

func requestSessionHTTPResponse(handler http.Handler, method, path string, body interface{}, headers map[string]string) (sessionHTTPResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return sessionHTTPResponse{}, err
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var response struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		return sessionHTTPResponse{}, err
	}
	return sessionHTTPResponse{HTTPStatus: w.Code, Code: response.Code}, nil
}

func TestSessionRevocationRejectsRevokedTokenOnPublicRoute(t *testing.T) {
	srv := newTestServer(t)
	login := func() sessionTokenPair {
		return sessionTokenPairFromResponse(t, requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
			"login_type": model.UserTypeAdmin,
			"username":   "admin",
			"password":   testAdminPassword,
		}, nil))
	}
	target := login()
	control := login()
	targetID := sessionIDFromAccess(t, target.access)
	controlID := sessionIDFromAccess(t, control.access)

	active, err := requestSessionHTTPResponse(srv.Router, http.MethodGet, "/healthz", nil,
		map[string]string{"Authorization": "Bearer " + target.access})
	if err != nil {
		t.Fatal("request active public route")
	}
	requireSessionHTTPResponse(t, "active public route", active, http.StatusOK, common.CodeOK)

	logout := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/logout", nil,
		map[string]string{"Authorization": "Bearer " + target.access})
	if logout.Code != common.CodeOK {
		t.Fatalf("logout code = %d, want %d", logout.Code, common.CodeOK)
	}
	revoked, err := requestSessionHTTPResponse(srv.Router, http.MethodGet, "/healthz", nil,
		map[string]string{"Authorization": "Bearer " + target.access})
	if err != nil {
		t.Fatal("request revoked public route")
	}
	requireSessionHTTPResponse(t, "revoked public route", revoked, http.StatusUnauthorized, common.CodeUnauthorized)
	assertOnlyFirstSessionRevoked(t, srv, targetID, controlID)

	sqlDB, err := srv.DB.DB()
	if err != nil {
		t.Fatal("load test database handle")
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal("close test database pool")
	}
	anonymous, err := requestSessionHTTPResponse(srv.Router, http.MethodGet, "/healthz", nil, nil)
	if err != nil {
		t.Fatal("request anonymous public route after database close")
	}
	requireSessionHTTPResponse(t, "anonymous public route after database close", anonymous, http.StatusOK, common.CodeOK)
}

func TestSessionRevocationAllowsOnlyOneConcurrentLogout(t *testing.T) {
	srv := newTestServer(t)
	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": model.UserTypeAdmin,
		"username":   "admin",
		"password":   testAdminPassword,
	}, nil)
	pair := sessionTokenPairFromResponse(t, login)
	controlLogin := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": model.UserTypeAdmin,
		"username":   "admin",
		"password":   testAdminPassword,
	}, nil)
	control := sessionTokenPairFromResponse(t, controlLogin)
	targetID := sessionIDFromAccess(t, pair.access)
	controlID := sessionIDFromAccess(t, control.access)

	start := make(chan struct{})
	results := make(chan sessionHTTPResponse, 2)
	errorsFound := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			response, err := requestSessionHTTPResponse(srv.Router, http.MethodPost, "/api/v1/auth/logout", nil,
				map[string]string{"Authorization": "Bearer " + pair.access})
			if err != nil {
				errorsFound <- err
				return
			}
			results <- response
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatalf("concurrent logout request failed: %v", err)
		}
	}
	var responses []sessionHTTPResponse
	for response := range results {
		responses = append(responses, response)
	}
	sort.Slice(responses, func(i, j int) bool { return responses[i].Code < responses[j].Code })
	if len(responses) != 2 ||
		responses[0] != (sessionHTTPResponse{HTTPStatus: http.StatusOK, Code: common.CodeOK}) ||
		responses[1] != (sessionHTTPResponse{HTTPStatus: http.StatusUnauthorized, Code: common.CodeUnauthorized}) {
		t.Fatalf("concurrent logout status/codes = %v, want [%d/%d %d/%d]", responses,
			http.StatusOK, common.CodeOK, http.StatusUnauthorized, common.CodeUnauthorized)
	}
	assertOnlyFirstSessionRevoked(t, srv, targetID, controlID)
}
