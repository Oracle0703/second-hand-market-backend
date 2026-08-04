//go:build mysqlacceptance

package auth_test

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

const (
	mysqlSessionAccessSecret  = "f14-isolated-access-secret"
	mysqlSessionRefreshSecret = "f14-isolated-refresh-secret"
	mysqlSessionAdminPassword = "F14-Admin-Password-2026!"
)

type mysqlSessionAcceptanceFixture struct {
	server         *app.Server
	database       string
	expectedUser   string
	expectedUUID   string
	expectedNonce  string
	accessSecret   string
	refreshSecret  string
	refreshTTL     time.Duration
	accessTTL      time.Duration
	databaseClosed bool
}

type mysqlSessionAPIResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

type mysqlSessionTokens struct {
	access  string
	refresh string
}

func TestSessionAccessRevocationMySQLAcceptance(t *testing.T) {
	fixture := newMySQLSessionAcceptanceFixture(t)

	if !t.Run("isolated_mysql_8_4_fixture", func(t *testing.T) {
		fixture.verifyIdentity(t)
	}) {
		t.FailNow()
	}
	t.Run("stale_jwt_uses_current_identity", func(t *testing.T) {
		fixture.cleanSessionData(t)
		fixture.assertStaleJWTUsesCurrentIdentity(t)
	})
	t.Run("invalid_sessions_fail_closed", func(t *testing.T) {
		fixture.cleanSessionData(t)
		fixture.assertInvalidSessionsFailClosed(t)
	})
	t.Run("logout_revokes_only_current_session", func(t *testing.T) {
		fixture.cleanSessionData(t)
		fixture.assertLogoutRevokesOnlyCurrentSession(t)
	})
	t.Run("immediate_refresh_succeeds", func(t *testing.T) {
		fixture.cleanSessionData(t)
		fixture.assertImmediateRefreshSucceeds(t)
	})
	t.Run("database_errors_are_redacted", func(t *testing.T) {
		fixture.cleanSessionData(t)
		fixture.assertDatabaseErrorsAreRedacted(t)
	})
}

func newMySQLSessionAcceptanceFixture(t *testing.T) *mysqlSessionAcceptanceFixture {
	t.Helper()
	dsn := readSecureMySQLSessionFixtureFile(t, "F14_MYSQL_DSN_FILE")
	database := readSecureMySQLSessionFixtureFile(t, "F14_MYSQL_DATABASE_FILE")
	expectedUser := readSecureMySQLSessionFixtureFile(t, "F14_MYSQL_USER_FILE")
	expectedUUID := readSecureMySQLSessionFixtureFile(t, "F14_MYSQL_SERVER_UUID_FILE")
	expectedNonce := readSecureMySQLSessionFixtureFile(t, "F14_MYSQL_NONCE_FILE")

	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("isolated MySQL DSN is invalid")
	}
	host, port, err := net.SplitHostPort(parsed.Addr)
	if err != nil ||
		parsed.Net != "tcp" ||
		host != "127.0.0.1" ||
		port == "" ||
		parsed.DBName != database ||
		!strings.HasPrefix(database, "f14_session_access_") ||
		!strings.HasPrefix(parsed.User, "f14_app_") ||
		!parsed.ParseTime ||
		parsed.Loc == nil ||
		parsed.Loc.String() != "UTC" ||
		parsed.Timeout <= 0 ||
		parsed.ReadTimeout <= 0 ||
		parsed.WriteTimeout <= 0 ||
		parsed.MultiStatements ||
		parsed.AllowAllFiles ||
		parsed.AllowCleartextPasswords ||
		parsed.TLSConfig != "" {
		t.Fatal("isolated MySQL DSN violates the F-14 safety boundary")
	}

	cfg := app.Config{
		AppEnv:                   "test",
		Addr:                     ":0",
		DBTarget:                 "local",
		DBDriver:                 "mysql",
		DBDSN:                    dsn,
		JWTAccessSecret:          mysqlSessionAccessSecret,
		JWTRefreshSecret:         mysqlSessionRefreshSecret,
		AccessTTL:                2 * time.Hour,
		RefreshTTL:               7 * 24 * time.Hour,
		AutoMigrate:              false,
		SeedDefaults:             false,
		FileStorageProvider:      "local",
		FileUploadLocalDir:       t.TempDir(),
		ImageProcessorDriver:     "passthrough",
		BuyerWechatLoginMode:     "mock",
		BuyerDouyinLoginMode:     "mock",
		FileUploadMaxBytes:       40 * 1024 * 1024,
		ImageCompressTargetBytes: 20 * 1024 * 1024,
	}
	server, err := app.NewServer(cfg)
	if err != nil {
		t.Fatal("open isolated MySQL F-14 server")
	}

	fixture := &mysqlSessionAcceptanceFixture{
		server:        server,
		database:      database,
		expectedUser:  expectedUser,
		expectedUUID:  expectedUUID,
		expectedNonce: expectedNonce,
		accessSecret:  cfg.JWTAccessSecret,
		refreshSecret: cfg.JWTRefreshSecret,
		refreshTTL:    cfg.RefreshTTL,
		accessTTL:     cfg.AccessTTL,
	}
	t.Cleanup(func() {
		if fixture.databaseClosed {
			return
		}
		sqlDB, dbErr := server.DB.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return fixture
}

func (f *mysqlSessionAcceptanceFixture) verifyIdentity(t *testing.T) {
	t.Helper()
	var version, database, currentUser, serverUUID, nonce string
	if err := f.server.DB.Raw(
		"SELECT VERSION(), DATABASE(), CURRENT_USER(), @@server_uuid",
	).Row().Scan(&version, &database, &currentUser, &serverUUID); err != nil {
		t.Fatal("read isolated MySQL identity")
	}
	if !strings.HasPrefix(version, "8.4.") ||
		database != f.database ||
		currentUser != f.expectedUser ||
		serverUUID != f.expectedUUID {
		t.Fatal("isolated MySQL identity does not match the F-14 fixture")
	}
	if err := f.server.DB.Raw(
		"SELECT marker FROM f14_acceptance_guard WHERE id = 1",
	).Row().Scan(&nonce); err != nil {
		t.Fatal("read isolated MySQL F-14 nonce")
	}
	if nonce != f.expectedNonce {
		t.Fatal("isolated MySQL F-14 nonce mismatch")
	}
	for _, table := range []interface{}{
		&model.AuthSession{},
		&model.AdminUser{},
		&model.Merchant{},
		&model.MerchantAccount{},
		&model.BuyerUser{},
	} {
		if !f.server.DB.Migrator().HasTable(table) {
			t.Fatal("isolated MySQL fixture is missing an authentication table")
		}
	}
}

func (f *mysqlSessionAcceptanceFixture) cleanSessionData(t *testing.T) {
	t.Helper()
	for _, table := range []string{
		"auth_sessions",
		"merchant_accounts",
		"buyer_users",
		"admin_users",
		"merchants",
	} {
		if err := f.server.DB.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatal("clean isolated MySQL F-14 fixture")
		}
	}
}

func (f *mysqlSessionAcceptanceFixture) assertStaleJWTUsesCurrentIdentity(t *testing.T) {
	merchant := model.Merchant{
		MerchantNo:   "M-F14-MYSQL-CURRENT",
		MerchantName: "current identity",
		ContactName:  "contact",
		ContactPhone: "10000000001",
		ReviewStatus: model.ReviewApproved,
	}
	if err := f.server.DB.Create(&merchant).Error; err != nil {
		t.Fatal("create isolated MySQL merchant")
	}
	account := model.MerchantAccount{
		MerchantID:   merchant.ID,
		Username:     "f14_mysql_current_merchant",
		PasswordHash: "unused",
		Role:         model.AccountRoleStaff,
		Status:       model.AccountStatusActive,
	}
	if err := f.server.DB.Create(&account).Error; err != nil {
		t.Fatal("create isolated MySQL merchant account")
	}
	session := f.createSession(t, model.UserTypeMerchant, account.ID)
	token, _, err := auth.BuildAccessToken(f.accessSecret, auth.AccessClaims{
		UserID:     account.ID,
		UserType:   model.UserTypeMerchant,
		Role:       model.AdminRoleSuper,
		MerchantID: merchant.ID + 1000,
		Scope:      "onboarding",
		SessionID:  session.ID,
	}, f.accessTTL)
	if err != nil {
		t.Fatal("build isolated MySQL stale access token")
	}

	response, _ := f.request(
		t,
		http.MethodGet,
		"/api/v1/merchant/account",
		nil,
		map[string]string{"Authorization": "Bearer " + token},
	)
	if response.Code != common.CodeOK {
		t.Fatal("current merchant identity did not override stale JWT claims")
	}

	result := f.server.DB.Model(&model.Merchant{}).
		Where("id = ?", merchant.ID).
		Update("review_status", model.ReviewPending)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatal("downgrade isolated MySQL merchant review state")
	}
	response, _ = f.request(
		t,
		http.MethodGet,
		"/api/v1/merchant/account",
		nil,
		map[string]string{"Authorization": "Bearer " + token},
	)
	if response.Code != common.CodeReviewNotApproved {
		t.Fatal("stale JWT retained full merchant scope after review downgrade")
	}
}

func (f *mysqlSessionAcceptanceFixture) assertInvalidSessionsFailClosed(t *testing.T) {
	admin := model.AdminUser{
		Username: "f14_mysql_invalid_admin", PasswordHash: "unused",
		Role: model.AdminRoleAdmin, Status: model.AccountStatusActive,
	}
	if err := f.server.DB.Create(&admin).Error; err != nil {
		t.Fatal("create isolated MySQL invalid-session admin")
	}
	revokedAt := time.Now().UTC()
	tests := []struct {
		name      string
		session   model.AuthSession
		sessionID uint64
		userType  string
		userID    uint64
	}{
		{
			name: "missing", sessionID: uint64(1 << 62),
			userType: model.UserTypeAdmin, userID: admin.ID,
		},
		{
			name: "revoked",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID,
				RefreshTokenHash: "hash", ExpiredAt: time.Now().Add(time.Hour),
				RevokedAt: &revokedAt,
			},
			userType: model.UserTypeAdmin, userID: admin.ID,
		},
		{
			name: "expired",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID,
				RefreshTokenHash: "hash", ExpiredAt: time.Now().Add(-time.Second),
			},
			userType: model.UserTypeAdmin, userID: admin.ID,
		},
		{
			name: "user_type_mismatch",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID,
				RefreshTokenHash: "hash", ExpiredAt: time.Now().Add(time.Hour),
			},
			userType: model.UserTypeMerchant, userID: admin.ID,
		},
		{
			name: "user_id_mismatch",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID,
				RefreshTokenHash: "hash", ExpiredAt: time.Now().Add(time.Hour),
			},
			userType: model.UserTypeAdmin, userID: admin.ID + 1,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			sessionID := testCase.sessionID
			if testCase.session.UserType != "" {
				if err := f.server.DB.Create(&testCase.session).Error; err != nil {
					t.Fatal("create isolated MySQL invalid session")
				}
				sessionID = testCase.session.ID
			}
			token, _, err := auth.BuildAccessToken(f.accessSecret, auth.AccessClaims{
				UserID:    testCase.userID,
				UserType:  testCase.userType,
				Role:      model.AdminRoleAdmin,
				Scope:     "full",
				SessionID: sessionID,
			}, f.accessTTL)
			if err != nil {
				t.Fatal("build isolated MySQL invalid-session token")
			}
			response, _ := f.request(
				t,
				http.MethodGet,
				"/api/v1/admin/merchants",
				nil,
				map[string]string{"Authorization": "Bearer " + token},
			)
			if response.Code != common.CodeUnauthorized {
				t.Fatal("invalid isolated MySQL session did not fail closed")
			}
		})
	}
}

func (f *mysqlSessionAcceptanceFixture) assertLogoutRevokesOnlyCurrentSession(t *testing.T) {
	username := "f14_mysql_logout_admin"
	f.bootstrapAdmin(t, username)
	current := f.loginAdmin(t, username)
	sibling := f.loginAdmin(t, username)

	f.assertAdminAccessCode(t, current.access, common.CodeOK)
	f.assertAdminAccessCode(t, sibling.access, common.CodeOK)
	response, _ := f.request(
		t,
		http.MethodPost,
		"/api/v1/auth/logout",
		map[string]interface{}{},
		map[string]string{"Authorization": "Bearer " + current.access},
	)
	if response.Code != common.CodeOK {
		t.Fatal("isolated MySQL logout failed")
	}
	f.assertAdminAccessCode(t, current.access, common.CodeUnauthorized)
	f.assertRefreshCode(t, current.refresh, common.CodeUnauthorized)
	f.assertAdminAccessCode(t, sibling.access, common.CodeOK)
	f.assertRefreshCode(t, sibling.refresh, common.CodeOK)
}

func (f *mysqlSessionAcceptanceFixture) assertImmediateRefreshSucceeds(t *testing.T) {
	username := "f14_mysql_immediate_admin"
	f.bootstrapAdmin(t, username)
	var admin model.AdminUser
	if err := f.server.DB.Where("username = ?", username).First(&admin).Error; err != nil {
		t.Fatal("load isolated MySQL immediate-refresh admin")
	}

	exercisedNoOpUpdate := false
	for attempt := 0; attempt < 5; attempt++ {
		session := f.createSession(t, model.UserTypeAdmin, admin.ID)
		waitForFreshJWTSecond(t)
		refresh, refreshExp, err := auth.BuildRefreshToken(
			f.refreshSecret,
			auth.RefreshClaims{
				UserID:    admin.ID,
				UserType:  model.UserTypeAdmin,
				SessionID: session.ID,
			},
			f.refreshTTL,
		)
		if err != nil {
			t.Fatal("build isolated MySQL immediate refresh token")
		}
		result := f.server.DB.Model(&model.AuthSession{}).
			Where("id = ?", session.ID).
			Updates(map[string]interface{}{
				"refresh_token_hash": common.SHA256(refresh),
				"expired_at":         refreshExp,
			})
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatal("prepare isolated MySQL immediate refresh session")
		}

		response, _ := f.request(
			t,
			http.MethodPost,
			"/api/v1/auth/refresh",
			map[string]interface{}{"refresh_token": refresh},
			nil,
		)
		if response.Code != common.CodeOK {
			t.Fatal("immediate isolated MySQL refresh failed")
		}
		if mysqlSessionString(response.Data["refresh_token"]) == refresh {
			exercisedNoOpUpdate = true
			break
		}
	}
	if !exercisedNoOpUpdate {
		t.Fatal("isolated MySQL fixture did not exercise a same-token no-op refresh")
	}
}

func waitForFreshJWTSecond(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		now := time.Now()
		if time.Duration(now.Nanosecond()) < 100*time.Millisecond {
			return
		}
		wait := time.Second - time.Duration(now.Nanosecond()) + 5*time.Millisecond
		if now.Add(wait).After(deadline) {
			t.Fatal("could not enter a fresh JWT second")
		}
		time.Sleep(wait)
	}
}

func (f *mysqlSessionAcceptanceFixture) assertDatabaseErrorsAreRedacted(t *testing.T) {
	username := "f14_mysql_database_error_admin"
	f.bootstrapAdmin(t, username)
	tokens := f.loginAdmin(t, username)
	sqlDB, err := f.server.DB.DB()
	if err != nil {
		t.Fatal("open isolated MySQL SQL pool")
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal("close isolated MySQL SQL pool")
	}
	f.databaseClosed = true

	response, raw := f.request(
		t,
		http.MethodGet,
		"/api/v1/admin/merchants",
		nil,
		map[string]string{"Authorization": "Bearer " + tokens.access},
	)
	if response.Code != common.CodeInternal || response.Message != common.ErrInternal.Message {
		t.Fatal("isolated MySQL database error did not fail closed")
	}
	lower := strings.ToLower(raw)
	for _, forbidden := range []string{
		"sql", "mysql", "dsn", "password", f.database,
		strings.ToLower(f.expectedUser), strings.ToLower(f.expectedUUID),
	} {
		if forbidden != "" && strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatal("isolated MySQL database error leaked internal detail")
		}
	}
}

func (f *mysqlSessionAcceptanceFixture) createSession(
	t *testing.T,
	userType string,
	userID uint64,
) model.AuthSession {
	t.Helper()
	session := model.AuthSession{
		UserType:         userType,
		UserID:           userID,
		RefreshTokenHash: "f14-isolated-session-hash",
		ExpiredAt:        time.Now().Add(f.refreshTTL),
	}
	if err := f.server.DB.Create(&session).Error; err != nil {
		t.Fatal("create isolated MySQL auth session")
	}
	return session
}

func (f *mysqlSessionAcceptanceFixture) bootstrapAdmin(t *testing.T, username string) {
	t.Helper()
	if err := app.BootstrapAdmin(f.server.DB, app.AdminBootstrap{
		Username: username, DisplayName: "F-14 MySQL Admin",
		Role: model.AdminRoleAdmin, Password: mysqlSessionAdminPassword,
	}); err != nil {
		t.Fatal("bootstrap isolated MySQL admin")
	}
}

func (f *mysqlSessionAcceptanceFixture) loginAdmin(
	t *testing.T,
	username string,
) mysqlSessionTokens {
	t.Helper()
	response, _ := f.request(
		t,
		http.MethodPost,
		"/api/v1/auth/login",
		map[string]interface{}{
			"login_type": model.UserTypeAdmin,
			"username":   username,
			"password":   mysqlSessionAdminPassword,
		},
		nil,
	)
	if response.Code != common.CodeOK {
		t.Fatal("isolated MySQL admin login failed")
	}
	tokens := mysqlSessionTokens{
		access:  mysqlSessionString(response.Data["access_token"]),
		refresh: mysqlSessionString(response.Data["refresh_token"]),
	}
	if tokens.access == "" || tokens.refresh == "" {
		t.Fatal("isolated MySQL admin login returned incomplete tokens")
	}
	return tokens
}

func (f *mysqlSessionAcceptanceFixture) assertAdminAccessCode(
	t *testing.T,
	accessToken string,
	wantCode int,
) {
	t.Helper()
	response, _ := f.request(
		t,
		http.MethodGet,
		"/api/v1/admin/merchants",
		nil,
		map[string]string{"Authorization": "Bearer " + accessToken},
	)
	if response.Code != wantCode {
		t.Fatal("isolated MySQL admin access code mismatch")
	}
}

func (f *mysqlSessionAcceptanceFixture) assertRefreshCode(
	t *testing.T,
	refreshToken string,
	wantCode int,
) {
	t.Helper()
	response, _ := f.request(
		t,
		http.MethodPost,
		"/api/v1/auth/refresh",
		map[string]interface{}{"refresh_token": refreshToken},
		nil,
	)
	if response.Code != wantCode {
		t.Fatal("isolated MySQL refresh code mismatch")
	}
}

func (f *mysqlSessionAcceptanceFixture) request(
	t *testing.T,
	method string,
	path string,
	body interface{},
	headers map[string]string,
) (mysqlSessionAPIResponse, string) {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal("encode isolated MySQL request")
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	f.server.Router.ServeHTTP(recorder, request)
	var response mysqlSessionAPIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal("decode isolated MySQL response")
	}
	return response, recorder.Body.String()
}

func readSecureMySQLSessionFixtureFile(t *testing.T, envName string) string {
	t.Helper()
	path := os.Getenv(envName)
	if !filepath.IsAbs(path) {
		t.Fatal("isolated MySQL fixture path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil ||
		!info.Mode().IsRegular() ||
		info.Mode().Perm() != 0o600 {
		t.Fatal("isolated MySQL fixture file must be a regular 0600 file")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read isolated MySQL fixture file")
	}
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || strings.ContainsAny(trimmed, "\r\n") {
		t.Fatal("isolated MySQL fixture value is invalid")
	}
	return trimmed
}

func mysqlSessionString(value interface{}) string {
	result, _ := value.(string)
	return result
}
