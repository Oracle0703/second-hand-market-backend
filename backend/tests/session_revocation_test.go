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

func requestJSONCode(handler http.Handler, method, path string, body interface{}, headers map[string]string) (int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
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
		return 0, err
	}
	return response.Code, nil
}

func TestSessionRevocationAllowsOnlyOneConcurrentLogout(t *testing.T) {
	srv := newTestServer(t)
	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": model.UserTypeAdmin,
		"username":   "admin",
		"password":   testAdminPassword,
	}, nil)
	pair := sessionTokenPairFromResponse(t, login)

	start := make(chan struct{})
	results := make(chan int, 2)
	errorsFound := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			code, err := requestJSONCode(srv.Router, http.MethodPost, "/api/v1/auth/logout", nil,
				map[string]string{"Authorization": "Bearer " + pair.access})
			if err != nil {
				errorsFound <- err
				return
			}
			results <- code
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
	var codes []int
	for code := range results {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	if len(codes) != 2 || codes[0] != common.CodeOK || codes[1] != common.CodeUnauthorized {
		t.Fatalf("concurrent logout codes = %v, want [%d %d]", codes, common.CodeOK, common.CodeUnauthorized)
	}
}
