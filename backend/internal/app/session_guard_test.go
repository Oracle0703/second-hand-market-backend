package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/middleware"
	"second-hand-market-backend/backend/internal/model"
)

type authorityTokens struct{ access, refresh string }
type authorityResponse struct {
	Code int                    `json:"code"`
	Data map[string]interface{} `json:"data"`
}

func TestSessionAuthority(t *testing.T) {
	runSessionAuthoritySuite(t, func(t *testing.T) *Server {
		cfg := authorityTestConfig(t)
		cfg.DBDriver = "sqlite"
		cfg.DBDSN = filepath.Join(t.TempDir(), "authority.sqlite")
		return newAuthorityTestServer(t, cfg)
	})
}

func runSessionAuthoritySuite(t *testing.T, newServer func(*testing.T) *Server) {
	userTypes := []string{model.UserTypeAdmin, model.UserTypeMerchant, model.UserTypeBuyer}
	for _, userType := range userTypes {
		t.Run(userType, func(t *testing.T) {
			for _, mutation := range []string{"revoked", "expired", "missing", "empty_hash", "wrong_user", "wrong_type"} {
				t.Run(mutation, func(t *testing.T) {
					srv := newServer(t)
					tokens := issueAuthorityTokens(t, srv, userType)
					claims, _ := auth.ParseRefreshToken(srv.cfg.JWTRefreshSecret, tokens.refresh)
					query := srv.DB.Model(&model.AuthSession{}).Where("id = ?", claims.SessionID)
					var result *gorm.DB
					switch mutation {
					case "revoked":
						result = query.Update("revoked_at", time.Now())
					case "expired":
						result = query.Update("expired_at", time.Now().Add(-time.Minute))
					case "missing":
						result = query.Delete(&model.AuthSession{})
					case "empty_hash":
						result = query.Update("refresh_token_hash", "")
					case "wrong_user":
						result = query.Update("user_id", 999)
					case "wrong_type":
						result = query.Update("user_type", model.UserTypePublic)
					}
					mustAuthorityWrite(t, result)
					assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", tokens.access, nil, common.CodeUnauthorized)
					assertAuthorityRefresh(t, srv, tokens.refresh, common.CodeUnauthorized)
				})
			}
			for _, mutation := range []string{"disabled", "unknown_status", "deleted"} {
				t.Run(mutation, func(t *testing.T) {
					srv := newServer(t)
					tokens := issueAuthorityTokens(t, srv, userType)
					var account interface{}
					switch userType {
					case model.UserTypeAdmin:
						account = &model.AdminUser{}
					case model.UserTypeMerchant:
						account = &model.MerchantAccount{}
					case model.UserTypeBuyer:
						account = &model.BuyerUser{}
					}
					query := srv.DB.Model(account).Where("id = ?", 1)
					refreshCode := common.CodeUnauthorized
					switch mutation {
					case "disabled":
						mustAuthorityWrite(t, query.Update("status", model.AccountStatusDisabled))
						refreshCode = common.CodeAccountDisabled
					case "unknown_status":
						mustAuthorityWrite(t, query.Update("status", "UNKNOWN"))
					case "deleted":
						mustAuthorityWrite(t, query.Delete(account))
					}
					assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", tokens.access, nil, common.CodeUnauthorized)
					assertAuthorityRefresh(t, srv, tokens.refresh, refreshCode)
					// Accounts in the other tables intentionally have the same numeric ID.
					for _, other := range userTypes {
						if other != userType {
							active := issueAuthorityTokens(t, srv, other)
							assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", active.access, nil, common.CodeOK)
							assertAuthorityRefresh(t, srv, active.refresh, common.CodeOK)
						}
					}
				})
			}
			t.Run("logout_preserves_sibling_session", func(t *testing.T) {
				srv := newServer(t)
				current, sibling := issueAuthorityTokens(t, srv, userType), issueAuthorityTokens(t, srv, userType)
				assertAuthorityCode(t, srv, http.MethodPost, "/api/v1/auth/logout", current.access, map[string]interface{}{}, common.CodeOK)
				assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", current.access, nil, common.CodeUnauthorized)
				assertAuthorityRefresh(t, srv, current.refresh, common.CodeUnauthorized)
				assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", sibling.access, nil, common.CodeOK)
				assertAuthorityRefresh(t, srv, sibling.refresh, common.CodeOK)
			})
		})
	}

	t.Run("merchant_authority_and_review_are_current", func(t *testing.T) {
		srv := newServer(t)
		tokens := issueAuthorityTokens(t, srv, model.UserTypeMerchant)
		mustAuthorityWrite(t, srv.DB.Model(&model.MerchantAccount{}).Where("id = 1").Updates(map[string]interface{}{"merchant_id": 2, "role": model.AccountRoleStaff}))
		response := assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", tokens.access, nil, common.CodeOK)
		if response.Data["merchant_id"] != float64(2) || response.Data["role"] != model.AccountRoleStaff || response.Data["scope"] != "onboarding" {
			t.Fatal("access retained stale merchant ownership, role or scope")
		}
		assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-merchant-test", tokens.access, nil, common.CodeReviewNotApproved)
		response = assertAuthorityRefresh(t, srv, tokens.refresh, common.CodeOK)
		claims, err := auth.ParseAccessToken(srv.cfg.JWTAccessSecret, response.Data["access_token"].(string))
		if err != nil || claims.MerchantID != 2 || claims.Role != model.AccountRoleStaff || claims.Scope != "onboarding" {
			t.Fatal("refresh retained stale merchant authority")
		}
		mustAuthorityWrite(t, srv.DB.Model(&model.Merchant{}).Where("id = 2").Update("review_status", model.ReviewDisabled))
		assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", tokens.access, nil, common.CodeUnauthorized)
		assertAuthorityRefresh(t, srv, response.Data["refresh_token"].(string), common.CodeReviewNotApproved)
	})
	t.Run("admin_role_is_current_and_invalid_roles_fail_closed", func(t *testing.T) {
		srv := newServer(t)
		tokens := issueAuthorityTokens(t, srv, model.UserTypeAdmin)
		mustAuthorityWrite(t, srv.DB.Model(&model.AdminUser{}).Where("id = 1").Update("role", model.AdminRoleAdmin))
		response := assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", tokens.access, nil, common.CodeOK)
		if response.Data["role"] != model.AdminRoleAdmin {
			t.Fatal("access retained stale admin role")
		}
		response = assertAuthorityRefresh(t, srv, tokens.refresh, common.CodeOK)
		claims, err := auth.ParseAccessToken(srv.cfg.JWTAccessSecret, response.Data["access_token"].(string))
		if err != nil || claims.Role != model.AdminRoleAdmin {
			t.Fatal("refresh retained stale admin role")
		}
		mustAuthorityWrite(t, srv.DB.Model(&model.AdminUser{}).Where("id = 1").Update("role", "UNKNOWN"))
		assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", tokens.access, nil, common.CodeUnauthorized)
		assertAuthorityRefresh(t, srv, response.Data["refresh_token"].(string), common.CodeUnauthorized)
	})
	t.Run("initial_password_restriction_survives_refresh", func(t *testing.T) {
		srv := newServer(t)
		tokens := issueAuthorityTokens(t, srv, model.UserTypeMerchant)
		mustAuthorityWrite(t, srv.DB.Model(&model.MerchantAccount{}).Where("id = 1").Update("must_change_password", true))
		assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", tokens.access, nil, common.CodeForbidden)
		assertAuthorityCode(t, srv, http.MethodGet, "/api/v1/merchant/account", tokens.access, nil, common.CodeOK)
		response := assertAuthorityRefresh(t, srv, tokens.refresh, common.CodeOK)
		assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", response.Data["access_token"].(string), nil, common.CodeForbidden)
	})
	t.Run("unknown_token_identity_fails_closed", func(t *testing.T) {
		srv := newServer(t)
		tokens := issueAuthorityTokens(t, srv, model.UserTypePublic)
		assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", tokens.access, nil, common.CodeUnauthorized)
		assertAuthorityRefresh(t, srv, tokens.refresh, common.CodeUnauthorized)
	})
	t.Run("database_failure_is_redacted_and_anonymous_skips_queries", func(t *testing.T) {
		srv := newServer(t)
		tokens := issueAuthorityTokens(t, srv, model.UserTypeBuyer)
		queries := 0
		if err := srv.DB.Callback().Query().Before("gorm:query").Register("session-authority-failure", func(db *gorm.DB) {
			queries++
			db.AddError(errors.New("session-authority-secret-sentinel"))
		}); err != nil {
			t.Fatal(err)
		}
		assertAuthorityCode(t, srv, http.MethodGet, "/healthz", "", nil, common.CodeOK)
		if queries != 0 {
			t.Fatal("anonymous health request queried session storage")
		}
		assertAuthorityCode(t, srv, http.MethodGet, "/session-authority-test", tokens.access, nil, common.CodeInternal)
		assertAuthorityRefresh(t, srv, tokens.refresh, common.CodeInternal)
		if queries != 2 {
			t.Fatalf("expected both auth paths to fail at the database, got %d queries", queries)
		}
	})
}

func authorityTestConfig(t *testing.T) Config {
	t.Helper()
	return Config{AppEnv: "test", DBTarget: "local", Addr: ":0", JWTAccessSecret: "authority-test-access", JWTRefreshSecret: "authority-test-refresh", AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour, FileStorageProvider: "local", FileUploadLocalDir: t.TempDir(), ImageProcessorDriver: "passthrough", BuyerWechatLoginMode: "disabled", BuyerDouyinLoginMode: "disabled"}
}

func authorityModels() []interface{} {
	return []interface{}{&model.AuthSession{}, &model.AdminUser{}, &model.Merchant{}, &model.MerchantAccount{}, &model.BuyerUser{}}
}

func newAuthorityTestServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatal("open isolated authority server")
	}
	t.Cleanup(func() { closeDatabase(srv.DB) })
	// MySQL acceptance verifies an empty, explicitly opted-in fixture before DDL.
	if cfg.DBDriver == "mysql" {
		var tables int64
		if err := srv.DB.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables).Error; err != nil || tables != 0 {
			t.Fatal("authority MySQL fixture must start empty")
		}
		t.Cleanup(func() {
			if err := srv.DB.Migrator().DropTable(authorityModels()...); err != nil {
				t.Error("clean up authority fixture tables")
			}
		})
	}
	if err := srv.DB.AutoMigrate(authorityModels()...); err != nil {
		t.Fatal("create isolated authority schema")
	}
	for _, row := range []interface{}{
		&model.AdminUser{ID: 1, Username: "authority-admin", Role: model.AdminRoleSuper, Status: model.AccountStatusActive},
		&model.Merchant{ID: 1, MerchantNo: "AUTHORITY-1", ReviewStatus: model.ReviewApproved},
		&model.Merchant{ID: 2, MerchantNo: "AUTHORITY-2", ReviewStatus: model.ReviewPending},
		&model.MerchantAccount{ID: 1, MerchantID: 1, Username: "authority-merchant", Role: model.AccountRoleOwner, Status: model.AccountStatusActive},
		&model.BuyerUser{ID: 1, BuyerNo: "AUTHORITY-BUYER", AuthProvider: "wechat", OpenID: "synthetic-authority-buyer", Status: model.BuyerStatusActive},
	} {
		mustAuthorityWrite(t, srv.DB.Create(row))
	}
	showActor := func(c *gin.Context) {
		actor, _ := common.GetActor(c)
		common.Success(c, gin.H{"user_id": actor.UserID, "user_type": actor.UserType, "merchant_id": actor.MerchantID, "role": actor.Role, "scope": actor.Scope})
	}
	srv.Router.GET("/session-authority-test", middleware.RequireAuth(), showActor)
	srv.Router.GET("/session-authority-merchant-test", middleware.RequireFullMerchantScope(), showActor)
	return srv
}

func issueAuthorityTokens(t *testing.T, srv *Server, userType string) authorityTokens {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	role, merchantID := model.AdminRoleSuper, uint64(0)
	if userType == model.UserTypeMerchant {
		role, merchantID = model.AccountRoleOwner, 1
	}
	if userType == model.UserTypeBuyer {
		role = model.UserTypeBuyer
	}
	data, err := srv.issueTokens(c, userType, 1, role, merchantID, "full")
	if err != nil {
		t.Fatal("issue synthetic authority tokens")
	}
	return authorityTokens{data["access_token"].(string), data["refresh_token"].(string)}
}

func mustAuthorityWrite(t *testing.T, result *gorm.DB) {
	t.Helper()
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatal("write isolated authority fixture")
	}
}

func assertAuthorityRefresh(t *testing.T, srv *Server, refresh string, code int) authorityResponse {
	t.Helper()
	return assertAuthorityCode(t, srv, http.MethodPost, "/api/v1/auth/refresh", "", map[string]interface{}{"refresh_token": refresh}, code)
}

func assertAuthorityCode(t *testing.T, srv *Server, method, path, access string, body interface{}, code int) authorityResponse {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	if strings.Contains(w.Body.String(), "session-authority-secret-sentinel") {
		t.Fatal("database error leaked into response")
	}
	var response authorityResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal("decode authority response")
	}
	if response.Code != code {
		t.Fatalf("%s %s: code=%d, want %d (HTTP %d)", method, path, response.Code, code, w.Code)
	}
	if code == common.CodeUnauthorized && w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized HTTP status = %d", w.Code)
	}
	if code == common.CodeOK && w.Code != http.StatusOK {
		t.Fatalf("successful HTTP status = %d", w.Code)
	}
	return response
}
