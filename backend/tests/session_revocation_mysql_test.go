package tests

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	mysqlcfg "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

const (
	sessionAcceptanceAccessSecret  = "session-revocation-test-access"
	sessionAcceptanceRefreshSecret = "session-revocation-test-refresh"
	sessionAcceptancePassword      = "F14-Test-Password-Only"
)

type sessionAcceptanceResponse struct {
	HTTPStatus int
	Body       apiResp
}

func sessionAcceptanceRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body interface{},
	headers map[string]string,
) sessionAcceptanceResponse {
	t.Helper()
	response, err := sessionAcceptanceRequestResult(handler, method, path, body, headers)
	if err != nil {
		t.Fatal("execute isolated acceptance request")
	}
	return response
}

func sessionAcceptanceRequestResult(
	handler http.Handler,
	method string,
	path string,
	body interface{},
	headers map[string]string,
) (sessionAcceptanceResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return sessionAcceptanceResponse{}, err
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var response apiResp
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		return sessionAcceptanceResponse{}, err
	}
	return sessionAcceptanceResponse{HTTPStatus: w.Code, Body: response}, nil
}

func requireSessionAcceptanceCode(t *testing.T, label string, response sessionAcceptanceResponse, wantHTTP, wantCode int) {
	t.Helper()
	if response.HTTPStatus != wantHTTP || response.Body.Code != wantCode {
		t.Fatalf("%s status/code = %d/%d, want %d/%d", label, response.HTTPStatus, response.Body.Code, wantHTTP, wantCode)
	}
	t.Logf("%s status/code = %d/%d", label, response.HTTPStatus, response.Body.Code)
}

func sessionAcceptanceTokenPair(t *testing.T, response sessionAcceptanceResponse) sessionTokenPair {
	t.Helper()
	requireSessionAcceptanceCode(t, "login", response, http.StatusOK, common.CodeOK)
	pair := sessionTokenPair{
		access:  str(response.Body.Data["access_token"]),
		refresh: str(response.Body.Data["refresh_token"]),
	}
	if pair.access == "" || pair.refresh == "" {
		t.Fatal("isolated acceptance login omitted token pair")
	}
	return pair
}

func sessionAcceptanceSessionID(t *testing.T, access string) uint64 {
	t.Helper()
	claims, err := auth.ParseAccessToken(sessionAcceptanceAccessSecret, access)
	if err != nil || claims.SessionID == 0 {
		t.Fatal("parse isolated acceptance access token")
	}
	return claims.SessionID
}

func createSessionAcceptanceAdmin(t *testing.T, srv *app.Server, suffix string) (string, string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(sessionAcceptancePassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal("hash isolated administrator password")
	}
	username := "f14_admin_" + suffix
	admin := model.AdminUser{
		Username: username, PasswordHash: string(hash), DisplayName: "F14 Admin",
		Role: model.AdminRoleAdmin, Status: model.AccountStatusActive,
	}
	if err := srv.DB.Create(&admin).Error; err != nil {
		t.Fatal("create isolated administrator")
	}
	return username, sessionAcceptancePassword
}

func createSessionAcceptanceMerchant(t *testing.T, srv *app.Server, suffix string) (string, string, uint64, uint64) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(sessionAcceptancePassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal("hash isolated merchant password")
	}
	merchant := model.Merchant{
		MerchantNo: "F14-M-" + suffix, MerchantName: "F14 Merchant",
		ContactName: "F14 Owner", ContactPhone: "13900000000",
		ReviewStatus: model.ReviewApproved,
	}
	if err := srv.DB.Create(&merchant).Error; err != nil {
		t.Fatal("create isolated merchant")
	}
	username := "f14_merchant_" + suffix
	account := model.MerchantAccount{
		MerchantID: merchant.ID, Username: username, PasswordHash: string(hash),
		Role: model.AccountRoleOwner, Status: model.AccountStatusActive,
	}
	if err := srv.DB.Create(&account).Error; err != nil {
		t.Fatal("create isolated merchant account")
	}
	return username, sessionAcceptancePassword, merchant.ID, account.ID
}

func sessionAcceptanceAdminLogin(t *testing.T, srv *app.Server, username, password string) sessionTokenPair {
	t.Helper()
	return sessionAcceptanceTokenPair(t, sessionAcceptanceRequest(t, srv.Router, http.MethodPost,
		"/api/v1/auth/login", map[string]interface{}{
			"login_type": model.UserTypeAdmin, "username": username, "password": password,
		}, nil))
}

func sessionAcceptanceMerchantLogin(t *testing.T, srv *app.Server, username, password string) sessionTokenPair {
	t.Helper()
	return sessionAcceptanceTokenPair(t, sessionAcceptanceRequest(t, srv.Router, http.MethodPost,
		"/api/v1/auth/login", map[string]interface{}{
			"login_type": model.UserTypeMerchant, "username": username, "password": password,
		}, nil))
}

func sessionAcceptanceBuyerLogin(t *testing.T, srv *app.Server, code, device string) sessionTokenPair {
	t.Helper()
	return sessionAcceptanceTokenPair(t, sessionAcceptanceRequest(t, srv.Router, http.MethodPost,
		"/api/v1/buyer/auth/wechat-login", map[string]interface{}{
			"code": code, "device_id": device, "nickname": "F14 Buyer",
		}, nil))
}

func exerciseSessionAcceptanceRevocation(
	t *testing.T,
	srv *app.Server,
	label string,
	first sessionTokenPair,
	second sessionTokenPair,
	accessPath string,
	refreshPath string,
) {
	t.Helper()
	firstID := sessionAcceptanceSessionID(t, first.access)
	secondID := sessionAcceptanceSessionID(t, second.access)
	if firstID == secondID {
		t.Fatalf("%s two logins reused one session", label)
	}
	access := func(token string) sessionAcceptanceResponse {
		return sessionAcceptanceRequest(t, srv.Router, http.MethodGet, accessPath, nil,
			map[string]string{"Authorization": "Bearer " + token})
	}
	refresh := func(token string) sessionAcceptanceResponse {
		return sessionAcceptanceRequest(t, srv.Router, http.MethodPost, refreshPath,
			map[string]interface{}{"refresh_token": token}, nil)
	}
	requireSessionAcceptanceCode(t, label+" first access before logout", access(first.access), http.StatusOK, common.CodeOK)
	requireSessionAcceptanceCode(t, label+" second access before logout", access(second.access), http.StatusOK, common.CodeOK)
	logout := sessionAcceptanceRequest(t, srv.Router, http.MethodPost, "/api/v1/auth/logout", nil,
		map[string]string{"Authorization": "Bearer " + first.access})
	requireSessionAcceptanceCode(t, label+" logout", logout, http.StatusOK, common.CodeOK)
	requireSessionAcceptanceCode(t, label+" old access", access(first.access), http.StatusUnauthorized, common.CodeUnauthorized)
	requireSessionAcceptanceCode(t, label+" old refresh", refresh(first.refresh), http.StatusUnauthorized, common.CodeUnauthorized)
	requireSessionAcceptanceCode(t, label+" unrelated access", access(second.access), http.StatusOK, common.CodeOK)
	requireSessionAcceptanceCode(t, label+" unrelated refresh", refresh(second.refresh), http.StatusOK, common.CodeOK)

	var sessions []model.AuthSession
	if err := srv.DB.Where("id IN ?", []uint64{firstID, secondID}).Find(&sessions).Error; err != nil {
		t.Fatalf("%s load session rows", label)
	}
	if len(sessions) != 2 {
		t.Fatalf("%s session row count = %d, want 2", label, len(sessions))
	}
	state := map[uint64]bool{}
	for _, session := range sessions {
		state[session.ID] = session.RevokedAt != nil
	}
	if !state[firstID] || state[secondID] {
		t.Fatalf("%s first/second revoked = %t/%t", label, state[firstID], state[secondID])
	}
}

func TestSessionRevocationMySQLAcceptance(t *testing.T) {
	if os.Getenv("SESSION_REVOCATION_MYSQL_TEST") != "1" {
		t.Skip("set SESSION_REVOCATION_MYSQL_TEST=1 only in the isolated session revocation project")
	}
	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	parsed, err := mysqlcfg.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" ||
		parsed.DBName != "second_hand_market_acceptance" {
		t.Fatal("DB_DSN must target isolated mysql:3306/second_hand_market_acceptance")
	}
	cfg := app.Config{
		AppEnv:                   "test",
		Addr:                     ":0",
		DBDriver:                 "mysql",
		DBDSN:                    dsn,
		JWTAccessSecret:          sessionAcceptanceAccessSecret,
		JWTRefreshSecret:         sessionAcceptanceRefreshSecret,
		AccessTTL:                time.Hour,
		RefreshTTL:               24 * time.Hour,
		AutoMigrate:              strings.EqualFold(os.Getenv("AUTO_MIGRATE"), "true"),
		FileStorageProvider:      "local",
		FileUploadLocalDir:       t.TempDir(),
		ImageCompressTargetBytes: 10 * 1024 * 1024,
		ImageProcessorDriver:     "passthrough",
		BuyerWechatLoginMode:     "mock",
		BuyerDouyinLoginMode:     "mock",
		BuyerWechatHTTPTimeout:   5 * time.Second,
		BuyerDouyinHTTPTimeout:   5 * time.Second,
	}
	configureTestUploadGovernance(&cfg)
	srv, err := app.NewServer(cfg)
	if err != nil {
		t.Fatal("start isolated session revocation server")
	}
	t.Cleanup(func() {
		sqlDB, sqlErr := srv.DB.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	t.Run("all actors revoke only one session", func(t *testing.T) {
		adminUsername, adminPassword := createSessionAcceptanceAdmin(t, srv, suffix+"a")
		exerciseSessionAcceptanceRevocation(t, srv, "administrator",
			sessionAcceptanceAdminLogin(t, srv, adminUsername, adminPassword),
			sessionAcceptanceAdminLogin(t, srv, adminUsername, adminPassword),
			"/api/v1/admin/logs", "/api/v1/auth/refresh")

		merchantUsername, merchantPassword, _, _ := createSessionAcceptanceMerchant(t, srv, suffix+"m")
		exerciseSessionAcceptanceRevocation(t, srv, "merchant",
			sessionAcceptanceMerchantLogin(t, srv, merchantUsername, merchantPassword),
			sessionAcceptanceMerchantLogin(t, srv, merchantUsername, merchantPassword),
			"/api/v1/merchant/profile", "/api/v1/auth/refresh")

		exerciseSessionAcceptanceRevocation(t, srv, "buyer",
			sessionAcceptanceBuyerLogin(t, srv, "f14-buyer-"+suffix, "f14-device-a-"+suffix),
			sessionAcceptanceBuyerLogin(t, srv, "f14-buyer-"+suffix, "f14-device-b-"+suffix),
			"/api/v1/buyer/intents", "/api/v1/buyer/auth/refresh")
	})

	t.Run("concurrent logout has one winner", func(t *testing.T) {
		username, password := createSessionAcceptanceAdmin(t, srv, suffix+"c")
		pair := sessionAcceptanceAdminLogin(t, srv, username, password)
		start := make(chan struct{})
		results := make(chan int, 2)
		errorsFound := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				response, err := sessionAcceptanceRequestResult(srv.Router, http.MethodPost, "/api/v1/auth/logout", nil,
					map[string]string{"Authorization": "Bearer " + pair.access})
				if err != nil {
					errorsFound <- err
					return
				}
				results <- response.Body.Code
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		close(errorsFound)
		for err := range errorsFound {
			if err != nil {
				t.Fatal("concurrent logout response failed")
			}
		}
		var codes []int
		for code := range results {
			codes = append(codes, code)
		}
		sort.Ints(codes)
		if len(codes) != 2 || codes[0] != common.CodeOK || codes[1] != common.CodeUnauthorized {
			t.Fatalf("concurrent logout codes = %v", codes)
		}
	})

	t.Run("explicit disablement fails closed", func(t *testing.T) {
		testSessionAcceptanceDisablement(t, srv, suffix)
	})

	t.Run("merchant review downgrade is immediate", func(t *testing.T) {
		testSessionAcceptanceMerchantDowngrade(t, srv, suffix)
	})

	t.Run("invalid session states are unauthorized", func(t *testing.T) {
		testSessionAcceptanceInvalidSessions(t, srv, suffix)
	})

	t.Run("authorization queries use primary keys", func(t *testing.T) {
		testSessionAcceptanceExplainPlans(t, srv, suffix)
	})

	t.Run("database failure fails closed without breaking anonymous health", func(t *testing.T) {
		testSessionAcceptanceDatabaseFailure(t, srv, suffix)
	})
}

func sessionAcceptanceAuthenticatedGET(t *testing.T, srv *app.Server, path, access string) sessionAcceptanceResponse {
	t.Helper()
	return sessionAcceptanceRequest(t, srv.Router, http.MethodGet, path, nil,
		map[string]string{"Authorization": "Bearer " + access})
}

func testSessionAcceptanceDisablement(t *testing.T, srv *app.Server, suffix string) {
	adminUsername, adminPassword := createSessionAcceptanceAdmin(t, srv, suffix+"da")
	adminPair := sessionAcceptanceAdminLogin(t, srv, adminUsername, adminPassword)
	adminClaims, err := auth.ParseAccessToken(sessionAcceptanceAccessSecret, adminPair.access)
	if err != nil {
		t.Fatal("parse administrator disablement token")
	}
	if err := srv.DB.Model(&model.AdminUser{}).Where("id = ?", adminClaims.UserID).
		Update("status", model.AccountStatusDisabled).Error; err != nil {
		t.Fatal("disable isolated administrator")
	}
	requireSessionAcceptanceCode(t, "disabled administrator",
		sessionAcceptanceAuthenticatedGET(t, srv, "/api/v1/admin/logs", adminPair.access),
		http.StatusForbidden, common.CodeAccountDisabled)

	merchantUsername, merchantPassword, _, accountID := createSessionAcceptanceMerchant(t, srv, suffix+"dm")
	merchantPair := sessionAcceptanceMerchantLogin(t, srv, merchantUsername, merchantPassword)
	if err := srv.DB.Model(&model.MerchantAccount{}).Where("id = ?", accountID).
		Update("status", model.AccountStatusDisabled).Error; err != nil {
		t.Fatal("disable isolated merchant account")
	}
	requireSessionAcceptanceCode(t, "disabled merchant account",
		sessionAcceptanceAuthenticatedGET(t, srv, "/api/v1/merchant/profile", merchantPair.access),
		http.StatusForbidden, common.CodeAccountDisabled)

	reviewUsername, reviewPassword, reviewMerchantID, _ := createSessionAcceptanceMerchant(t, srv, suffix+"dr")
	reviewPair := sessionAcceptanceMerchantLogin(t, srv, reviewUsername, reviewPassword)
	if err := srv.DB.Model(&model.Merchant{}).Where("id = ?", reviewMerchantID).
		Update("review_status", model.ReviewDisabled).Error; err != nil {
		t.Fatal("disable isolated merchant review")
	}
	requireSessionAcceptanceCode(t, "disabled merchant review",
		sessionAcceptanceAuthenticatedGET(t, srv, "/api/v1/merchant/profile", reviewPair.access),
		http.StatusForbidden, common.CodeAccountDisabled)

	buyerPair := sessionAcceptanceBuyerLogin(t, srv, "f14-disabled-buyer-"+suffix, "f14-disabled-device-"+suffix)
	buyerClaims, err := auth.ParseAccessToken(sessionAcceptanceAccessSecret, buyerPair.access)
	if err != nil {
		t.Fatal("parse buyer disablement token")
	}
	if err := srv.DB.Model(&model.BuyerUser{}).Where("id = ?", buyerClaims.UserID).
		Update("status", model.BuyerStatusDisabled).Error; err != nil {
		t.Fatal("disable isolated buyer")
	}
	requireSessionAcceptanceCode(t, "disabled buyer",
		sessionAcceptanceAuthenticatedGET(t, srv, "/api/v1/buyer/intents", buyerPair.access),
		http.StatusForbidden, common.CodeAccountDisabled)
}

func testSessionAcceptanceMerchantDowngrade(t *testing.T, srv *app.Server, suffix string) {
	username, password, merchantID, _ := createSessionAcceptanceMerchant(t, srv, suffix+"scope")
	pair := sessionAcceptanceMerchantLogin(t, srv, username, password)
	if err := srv.DB.Model(&model.Merchant{}).Where("id = ?", merchantID).
		Update("review_status", model.ReviewRejected).Error; err != nil {
		t.Fatal("downgrade isolated merchant review")
	}
	requireSessionAcceptanceCode(t, "merchant full route after downgrade",
		sessionAcceptanceAuthenticatedGET(t, srv, "/api/v1/merchant/products", pair.access),
		http.StatusForbidden, common.CodeReviewNotApproved)
	requireSessionAcceptanceCode(t, "merchant onboarding route after downgrade",
		sessionAcceptanceAuthenticatedGET(t, srv, "/api/v1/merchant/profile", pair.access),
		http.StatusOK, common.CodeOK)
}

func testSessionAcceptanceInvalidSessions(t *testing.T, srv *app.Server, suffix string) {
	type mutation func(*testing.T, *app.Server, uint64)
	cases := []struct {
		name   string
		mutate mutation
	}{
		{name: "missing", mutate: func(t *testing.T, srv *app.Server, sessionID uint64) {
			if err := srv.DB.Delete(&model.AuthSession{}, sessionID).Error; err != nil {
				t.Fatal("delete isolated session")
			}
		}},
		{name: "expired", mutate: func(t *testing.T, srv *app.Server, sessionID uint64) {
			if err := srv.DB.Model(&model.AuthSession{}).Where("id = ?", sessionID).
				Update("expired_at", time.Now().Add(-time.Minute)).Error; err != nil {
				t.Fatal("expire isolated session")
			}
		}},
		{name: "revoked", mutate: func(t *testing.T, srv *app.Server, sessionID uint64) {
			now := time.Now()
			if err := srv.DB.Model(&model.AuthSession{}).Where("id = ?", sessionID).
				Update("revoked_at", &now).Error; err != nil {
				t.Fatal("revoke isolated session")
			}
		}},
		{name: "identity mismatch", mutate: func(t *testing.T, srv *app.Server, sessionID uint64) {
			if err := srv.DB.Model(&model.AuthSession{}).Where("id = ?", sessionID).
				Update("user_id", gorm.Expr("user_id + 1")).Error; err != nil {
				t.Fatal("mismatch isolated session identity")
			}
		}},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			username, password := createSessionAcceptanceAdmin(t, srv, fmt.Sprintf("%s_s%d", suffix, index))
			pair := sessionAcceptanceAdminLogin(t, srv, username, password)
			sessionID := sessionAcceptanceSessionID(t, pair.access)
			tc.mutate(t, srv, sessionID)
			requireSessionAcceptanceCode(t, tc.name,
				sessionAcceptanceAuthenticatedGET(t, srv, "/api/v1/admin/logs", pair.access),
				http.StatusUnauthorized, common.CodeUnauthorized)
		})
	}
}

type sessionAcceptanceExplainRow struct {
	Table string
	Type  string
	Key   sql.NullString
}

func requirePrimaryKeyExplain(t *testing.T, label string, rows []sessionAcceptanceExplainRow, wantRows int) {
	t.Helper()
	if len(rows) != wantRows {
		t.Fatalf("%s EXPLAIN row count = %d, want %d", label, len(rows), wantRows)
	}
	for _, row := range rows {
		if !row.Key.Valid || row.Key.String != "PRIMARY" || strings.EqualFold(row.Type, "ALL") {
			t.Fatalf("%s EXPLAIN access/key = %s/%s", label, row.Type, row.Key.String)
		}
		t.Logf("%s EXPLAIN access/key = %s/%s", label, row.Type, row.Key.String)
	}
}

func testSessionAcceptanceExplainPlans(t *testing.T, srv *app.Server, suffix string) {
	adminUsername, adminPassword := createSessionAcceptanceAdmin(t, srv, suffix+"ea")
	adminPair := sessionAcceptanceAdminLogin(t, srv, adminUsername, adminPassword)
	adminClaims, err := auth.ParseAccessToken(sessionAcceptanceAccessSecret, adminPair.access)
	if err != nil {
		t.Fatal("parse administrator EXPLAIN token")
	}
	buyerPair := sessionAcceptanceBuyerLogin(t, srv, "f14-explain-buyer-"+suffix, "f14-explain-device-"+suffix)
	buyerClaims, err := auth.ParseAccessToken(sessionAcceptanceAccessSecret, buyerPair.access)
	if err != nil {
		t.Fatal("parse buyer EXPLAIN token")
	}
	merchantUsername, merchantPassword, _, accountID := createSessionAcceptanceMerchant(t, srv, suffix+"em")
	merchantPair := sessionAcceptanceMerchantLogin(t, srv, merchantUsername, merchantPassword)
	merchantSessionID := sessionAcceptanceSessionID(t, merchantPair.access)

	cases := []struct {
		name     string
		query    string
		args     []interface{}
		wantRows int
	}{
		{name: "session", query: "EXPLAIN SELECT id,user_type,user_id,expired_at,revoked_at FROM auth_sessions WHERE id = ? LIMIT 1", args: []interface{}{merchantSessionID}, wantRows: 1},
		{name: "administrator", query: "EXPLAIN SELECT status,role FROM admin_users WHERE id = ? AND deleted_at IS NULL LIMIT 1", args: []interface{}{adminClaims.UserID}, wantRows: 1},
		{name: "buyer", query: "EXPLAIN SELECT status FROM buyer_users WHERE id = ? AND deleted_at IS NULL LIMIT 1", args: []interface{}{buyerClaims.UserID}, wantRows: 1},
		{name: "merchant join", query: "EXPLAIN SELECT account.status,account.role,account.merchant_id,merchant.review_status FROM merchant_accounts AS account JOIN merchants AS merchant ON merchant.id=account.merchant_id AND merchant.deleted_at IS NULL WHERE account.id=? AND account.deleted_at IS NULL LIMIT 1", args: []interface{}{accountID}, wantRows: 2},
	}
	for _, tc := range cases {
		var rows []sessionAcceptanceExplainRow
		if err := srv.DB.Raw(tc.query, tc.args...).Scan(&rows).Error; err != nil {
			t.Fatalf("%s EXPLAIN query failed", tc.name)
		}
		requirePrimaryKeyExplain(t, tc.name, rows, tc.wantRows)
	}
}

func testSessionAcceptanceDatabaseFailure(t *testing.T, srv *app.Server, suffix string) {
	username, password := createSessionAcceptanceAdmin(t, srv, suffix+"db")
	pair := sessionAcceptanceAdminLogin(t, srv, username, password)
	sqlDB, err := srv.DB.DB()
	if err != nil {
		t.Fatal("get isolated SQL connection pool")
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal("close isolated SQL connection pool")
	}
	anonymous := sessionAcceptanceRequest(t, srv.Router, http.MethodGet, "/healthz", nil, nil)
	requireSessionAcceptanceCode(t, "anonymous health after database close", anonymous, http.StatusOK, common.CodeOK)
	authenticated := sessionAcceptanceAuthenticatedGET(t, srv, "/api/v1/admin/logs", pair.access)
	requireSessionAcceptanceCode(t, "authenticated access after database close", authenticated, http.StatusInternalServerError, common.CodeInternal)
}
