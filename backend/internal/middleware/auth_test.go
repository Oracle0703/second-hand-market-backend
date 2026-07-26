package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func newAuthMiddlewareDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)",
		strings.ReplaceAll(t.Name(), "/", "_"),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open auth middleware database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.AuthSession{},
		&model.AdminUser{},
		&model.Merchant{},
		&model.MerchantAccount{},
		&model.BuyerUser{},
	); err != nil {
		t.Fatalf("migrate auth middleware database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get auth middleware sql database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func authMiddlewareRouter(db *gorm.DB, actor *common.Actor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if actor != nil {
		r.Use(func(c *gin.Context) {
			common.SetActor(c, *actor)
			c.Next()
		})
	}
	r.Use(RequireActiveSession(db))
	r.GET("/probe", func(c *gin.Context) {
		actor, _ := common.GetActor(c)
		common.Success(c, gin.H{
			"user_id":     actor.UserID,
			"user_type":   actor.UserType,
			"role":        actor.Role,
			"merchant_id": actor.MerchantID,
			"scope":       actor.Scope,
			"session_id":  actor.SessionID,
		})
	})
	return r
}

func serveAuthProbe(router http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

type authProbeResponse struct {
	Code int `json:"code"`
	Data struct {
		UserID     uint64 `json:"user_id"`
		UserType   string `json:"user_type"`
		Role       string `json:"role"`
		MerchantID uint64 `json:"merchant_id"`
		Scope      string `json:"scope"`
		SessionID  uint64 `json:"session_id"`
	} `json:"data"`
}

func decodeAuthProbe(t *testing.T, w *httptest.ResponseRecorder) authProbeResponse {
	t.Helper()
	var response authProbeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode probe response: %v", err)
	}
	return response
}

func activeTestActor() common.Actor {
	return common.Actor{
		UserType:  model.UserTypeMerchant,
		UserID:    11,
		SessionID: 22,
	}
}

func activeTestSession(now time.Time) model.AuthSession {
	return model.AuthSession{
		ID:        22,
		UserType:  model.UserTypeMerchant,
		UserID:    11,
		ExpiredAt: now.Add(time.Hour),
	}
}

func createActorSession(t *testing.T, db *gorm.DB, actor common.Actor) {
	t.Helper()
	session := model.AuthSession{
		ID:        actor.SessionID,
		UserType:  actor.UserType,
		UserID:    actor.UserID,
		ExpiredAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create actor session: %v", err)
	}
}

func seedAdminAuthorization(t *testing.T, db *gorm.DB, status, role string) common.Actor {
	t.Helper()
	actor := common.Actor{
		UserID: 101, UserType: model.UserTypeAdmin, Role: model.AdminRoleAdmin,
		MerchantID: 999, Scope: "onboarding", SessionID: 201,
	}
	admin := model.AdminUser{
		ID: 101, Username: "f14_admin", DisplayName: "F14 Admin",
		Status: status, Role: role,
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create administrator: %v", err)
	}
	createActorSession(t, db, actor)
	return actor
}

func seedMerchantAuthorization(t *testing.T, db *gorm.DB, accountStatus, role, reviewStatus string) common.Actor {
	t.Helper()
	actor := common.Actor{
		UserID: 102, UserType: model.UserTypeMerchant, Role: model.AccountRoleStaff,
		MerchantID: 999, Scope: "onboarding", SessionID: 202,
	}
	merchant := model.Merchant{
		ID: 302, MerchantNo: "F14-M-302", MerchantName: "F14 Current Merchant",
		ReviewStatus: reviewStatus,
	}
	account := model.MerchantAccount{
		ID: 102, MerchantID: merchant.ID, Username: "f14_merchant",
		Status: accountStatus, Role: role,
	}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create merchant account: %v", err)
	}
	createActorSession(t, db, actor)
	return actor
}

func seedBuyerAuthorization(t *testing.T, db *gorm.DB, status string) common.Actor {
	t.Helper()
	actor := common.Actor{
		UserID: 103, UserType: model.UserTypeBuyer, Role: "STALE",
		MerchantID: 999, Scope: "onboarding", SessionID: 203,
	}
	buyer := model.BuyerUser{
		ID: 103, BuyerNo: "F14-B-103", AuthProvider: "wechat",
		OpenID: "f14-openid-103", Status: status,
	}
	if err := db.Create(&buyer).Error; err != nil {
		t.Fatalf("create buyer: %v", err)
	}
	createActorSession(t, db, actor)
	return actor
}

func TestRequireActiveSessionSkipsAnonymousRequest(t *testing.T) {
	db := newAuthMiddlewareDB(t)
	w := serveAuthProbe(authMiddlewareRouter(db, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("anonymous request status = %d", w.Code)
	}
}

func TestRequireActiveSessionAcceptsMatchingActiveSession(t *testing.T) {
	db := newAuthMiddlewareDB(t)
	now := time.Now()
	merchant := model.Merchant{
		ID:           31,
		MerchantNo:   "F14-M-31",
		MerchantName: "F14 Merchant",
		ReviewStatus: model.ReviewApproved,
	}
	account := model.MerchantAccount{
		ID:         11,
		MerchantID: merchant.ID,
		Username:   "f14_active_merchant",
		Role:       model.AccountRoleOwner,
		Status:     model.AccountStatusActive,
	}
	session := activeTestSession(now)
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create merchant account: %v", err)
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create active session: %v", err)
	}
	actor := activeTestActor()
	w := serveAuthProbe(authMiddlewareRouter(db, &actor))
	if w.Code != http.StatusOK {
		t.Fatalf("active session status = %d", w.Code)
	}
}

func TestRequireActiveSessionRejectsInvalidSessionState(t *testing.T) {
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		actor  common.Actor
		mutate func(*model.AuthSession)
		omit   bool
	}{
		{name: "zero sid", actor: common.Actor{UserType: model.UserTypeMerchant, UserID: 11}},
		{name: "missing", actor: common.Actor{UserType: model.UserTypeMerchant, UserID: 11, SessionID: 999}, omit: true},
		{name: "expired", actor: activeTestActor(), mutate: func(s *model.AuthSession) { s.ExpiredAt = now }},
		{name: "revoked", actor: activeTestActor(), mutate: func(s *model.AuthSession) { s.RevokedAt = &now }},
		{name: "user mismatch", actor: activeTestActor(), mutate: func(s *model.AuthSession) { s.UserID++ }},
		{name: "type mismatch", actor: activeTestActor(), mutate: func(s *model.AuthSession) { s.UserType = model.UserTypeBuyer }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newAuthMiddlewareDB(t)
			session := activeTestSession(now)
			if tc.mutate != nil {
				tc.mutate(&session)
			}
			if !tc.omit {
				if err := db.Create(&session).Error; err != nil {
					t.Fatalf("create session: %v", err)
				}
			}
			err := requireActiveSession(db, tc.actor, now)
			if !errors.Is(err, common.ErrUnauthorized) {
				t.Fatalf("error = %v, want unauthorized", err)
			}
		})
	}
}

func TestRequireActiveSessionMapsSessionQueryFailureToInternal(t *testing.T) {
	db := newAuthMiddlewareDB(t)
	errSynthetic := errors.New("synthetic auth session query failure")
	if err := db.Callback().Query().Before("gorm:query").
		Register("test:fail_auth_session_query", func(tx *gorm.DB) {
			if tx.Statement.Table == "auth_sessions" {
				tx.AddError(errSynthetic)
			}
		}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	err := requireActiveSession(db, activeTestActor(), time.Now())
	if !errors.Is(err, common.ErrInternal) {
		t.Fatalf("error = %v, want internal", err)
	}
}

func TestRequireActiveSessionReloadsAuthoritativeActor(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*testing.T, *gorm.DB) common.Actor
		want  common.Actor
	}{
		{
			name: "administrator role",
			setup: func(t *testing.T, db *gorm.DB) common.Actor {
				return seedAdminAuthorization(t, db, model.AccountStatusActive, model.AdminRoleSuper)
			},
			want: common.Actor{
				UserID: 101, UserType: model.UserTypeAdmin, Role: model.AdminRoleSuper,
				Scope: "full", SessionID: 201,
			},
		},
		{
			name: "merchant relationship and scope",
			setup: func(t *testing.T, db *gorm.DB) common.Actor {
				return seedMerchantAuthorization(t, db, model.AccountStatusActive, model.AccountRoleOwner, model.ReviewApproved)
			},
			want: common.Actor{
				UserID: 102, UserType: model.UserTypeMerchant, Role: model.AccountRoleOwner,
				MerchantID: 302, Scope: "full", SessionID: 202,
			},
		},
		{
			name: "buyer role and scope",
			setup: func(t *testing.T, db *gorm.DB) common.Actor {
				return seedBuyerAuthorization(t, db, model.BuyerStatusActive)
			},
			want: common.Actor{
				UserID: 103, UserType: model.UserTypeBuyer, Role: model.UserTypeBuyer,
				Scope: "full", SessionID: 203,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newAuthMiddlewareDB(t)
			actor := tc.setup(t, db)
			w := serveAuthProbe(authMiddlewareRouter(db, &actor))
			response := decodeAuthProbe(t, w)
			if w.Code != http.StatusOK || response.Code != common.CodeOK {
				t.Fatalf("probe status/code = %d/%d", w.Code, response.Code)
			}
			got := common.Actor{
				UserID: response.Data.UserID, UserType: response.Data.UserType,
				Role: response.Data.Role, MerchantID: response.Data.MerchantID,
				Scope: response.Data.Scope, SessionID: response.Data.SessionID,
			}
			if got != tc.want {
				t.Fatalf("authoritative actor = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRequireActiveSessionAccountStateMatrix(t *testing.T) {
	type setupFunc func(*testing.T, *gorm.DB) common.Actor
	missingActor := func(userType string, userID, sessionID uint64) setupFunc {
		return func(t *testing.T, db *gorm.DB) common.Actor {
			actor := common.Actor{UserID: userID, UserType: userType, SessionID: sessionID}
			createActorSession(t, db, actor)
			return actor
		}
	}
	cases := []struct {
		name       string
		setup      setupFunc
		wantStatus int
		wantCode   int
	}{
		{name: "missing administrator", setup: missingActor(model.UserTypeAdmin, 101, 201), wantStatus: http.StatusUnauthorized, wantCode: common.CodeUnauthorized},
		{
			name: "soft deleted administrator",
			setup: func(t *testing.T, db *gorm.DB) common.Actor {
				actor := seedAdminAuthorization(t, db, model.AccountStatusActive, model.AdminRoleAdmin)
				if err := db.Delete(&model.AdminUser{}, actor.UserID).Error; err != nil {
					t.Fatalf("soft delete administrator: %v", err)
				}
				return actor
			},
			wantStatus: http.StatusUnauthorized, wantCode: common.CodeUnauthorized,
		},
		{name: "disabled administrator", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedAdminAuthorization(t, db, model.AccountStatusDisabled, model.AdminRoleAdmin)
		}, wantStatus: http.StatusForbidden, wantCode: common.CodeAccountDisabled},
		{name: "unknown administrator status", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedAdminAuthorization(t, db, "LOCKED", model.AdminRoleAdmin)
		}, wantStatus: http.StatusInternalServerError, wantCode: common.CodeInternal},
		{name: "unknown administrator role", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedAdminAuthorization(t, db, model.AccountStatusActive, "AUDITOR")
		}, wantStatus: http.StatusInternalServerError, wantCode: common.CodeInternal},
		{name: "missing merchant account", setup: missingActor(model.UserTypeMerchant, 102, 202), wantStatus: http.StatusUnauthorized, wantCode: common.CodeUnauthorized},
		{
			name: "soft deleted merchant account",
			setup: func(t *testing.T, db *gorm.DB) common.Actor {
				actor := seedMerchantAuthorization(t, db, model.AccountStatusActive, model.AccountRoleOwner, model.ReviewApproved)
				if err := db.Delete(&model.MerchantAccount{}, actor.UserID).Error; err != nil {
					t.Fatalf("soft delete merchant account: %v", err)
				}
				return actor
			},
			wantStatus: http.StatusUnauthorized, wantCode: common.CodeUnauthorized,
		},
		{
			name: "soft deleted merchant relationship",
			setup: func(t *testing.T, db *gorm.DB) common.Actor {
				actor := seedMerchantAuthorization(t, db, model.AccountStatusActive, model.AccountRoleOwner, model.ReviewApproved)
				if err := db.Delete(&model.Merchant{}, uint64(302)).Error; err != nil {
					t.Fatalf("soft delete merchant: %v", err)
				}
				return actor
			},
			wantStatus: http.StatusUnauthorized, wantCode: common.CodeUnauthorized,
		},
		{name: "disabled merchant account", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedMerchantAuthorization(t, db, model.AccountStatusDisabled, model.AccountRoleOwner, model.ReviewApproved)
		}, wantStatus: http.StatusForbidden, wantCode: common.CodeAccountDisabled},
		{name: "disabled merchant review", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedMerchantAuthorization(t, db, model.AccountStatusActive, model.AccountRoleOwner, model.ReviewDisabled)
		}, wantStatus: http.StatusForbidden, wantCode: common.CodeAccountDisabled},
		{name: "unknown merchant account status", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedMerchantAuthorization(t, db, "LOCKED", model.AccountRoleOwner, model.ReviewApproved)
		}, wantStatus: http.StatusInternalServerError, wantCode: common.CodeInternal},
		{name: "unknown merchant role", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedMerchantAuthorization(t, db, model.AccountStatusActive, "AUDITOR", model.ReviewApproved)
		}, wantStatus: http.StatusInternalServerError, wantCode: common.CodeInternal},
		{name: "unknown merchant review", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedMerchantAuthorization(t, db, model.AccountStatusActive, model.AccountRoleOwner, "SUSPENDED")
		}, wantStatus: http.StatusInternalServerError, wantCode: common.CodeInternal},
		{name: "missing buyer", setup: missingActor(model.UserTypeBuyer, 103, 203), wantStatus: http.StatusUnauthorized, wantCode: common.CodeUnauthorized},
		{
			name: "soft deleted buyer",
			setup: func(t *testing.T, db *gorm.DB) common.Actor {
				actor := seedBuyerAuthorization(t, db, model.BuyerStatusActive)
				if err := db.Delete(&model.BuyerUser{}, actor.UserID).Error; err != nil {
					t.Fatalf("soft delete buyer: %v", err)
				}
				return actor
			},
			wantStatus: http.StatusUnauthorized, wantCode: common.CodeUnauthorized,
		},
		{name: "disabled buyer", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedBuyerAuthorization(t, db, model.BuyerStatusDisabled)
		}, wantStatus: http.StatusForbidden, wantCode: common.CodeAccountDisabled},
		{name: "unknown buyer status", setup: func(t *testing.T, db *gorm.DB) common.Actor {
			return seedBuyerAuthorization(t, db, "LOCKED")
		}, wantStatus: http.StatusInternalServerError, wantCode: common.CodeInternal},
		{name: "unsupported public actor", setup: missingActor(model.UserTypePublic, 104, 204), wantStatus: http.StatusUnauthorized, wantCode: common.CodeUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newAuthMiddlewareDB(t)
			actor := tc.setup(t, db)
			w := serveAuthProbe(authMiddlewareRouter(db, &actor))
			response := decodeAuthProbe(t, w)
			if w.Code != tc.wantStatus || response.Code != tc.wantCode {
				t.Fatalf("status/code = %d/%d, want %d/%d", w.Code, response.Code, tc.wantStatus, tc.wantCode)
			}
		})
	}
}

func TestRequireActiveSessionMapsAccountQueryFailureToInternal(t *testing.T) {
	db := newAuthMiddlewareDB(t)
	actor := seedAdminAuthorization(t, db, model.AccountStatusActive, model.AdminRoleAdmin)
	errSynthetic := errors.New("synthetic administrator query failure")
	if err := db.Callback().Query().Before("gorm:query").
		Register("test:fail_admin_authorization_query", func(tx *gorm.DB) {
			if tx.Statement.Table == "admin_users" {
				tx.AddError(errSynthetic)
			}
		}); err != nil {
		t.Fatalf("register account query callback: %v", err)
	}
	w := serveAuthProbe(authMiddlewareRouter(db, &actor))
	response := decodeAuthProbe(t, w)
	if w.Code != http.StatusInternalServerError || response.Code != common.CodeInternal {
		t.Fatalf("status/code = %d/%d, want %d/%d", w.Code, response.Code, http.StatusInternalServerError, common.CodeInternal)
	}
}

func TestRequireActiveSessionUsesExactAuthorizationQueryCount(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*testing.T, *gorm.DB) *common.Actor
		want  int64
	}{
		{name: "anonymous", setup: func(*testing.T, *gorm.DB) *common.Actor { return nil }, want: 0},
		{name: "administrator", setup: func(t *testing.T, db *gorm.DB) *common.Actor {
			actor := seedAdminAuthorization(t, db, model.AccountStatusActive, model.AdminRoleAdmin)
			return &actor
		}, want: 2},
		{name: "merchant join", setup: func(t *testing.T, db *gorm.DB) *common.Actor {
			actor := seedMerchantAuthorization(t, db, model.AccountStatusActive, model.AccountRoleOwner, model.ReviewApproved)
			return &actor
		}, want: 2},
		{name: "buyer", setup: func(t *testing.T, db *gorm.DB) *common.Actor {
			actor := seedBuyerAuthorization(t, db, model.BuyerStatusActive)
			return &actor
		}, want: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newAuthMiddlewareDB(t)
			actor := tc.setup(t, db)
			var count atomic.Int64
			if err := db.Callback().Query().Before("gorm:query").
				Register("test:count_authorization_queries", func(*gorm.DB) {
					count.Add(1)
				}); err != nil {
				t.Fatalf("register query counter: %v", err)
			}
			w := serveAuthProbe(authMiddlewareRouter(db, actor))
			if w.Code != http.StatusOK {
				t.Fatalf("probe status = %d", w.Code)
			}
			if got := count.Load(); got != tc.want {
				t.Fatalf("authorization query count = %d, want %d", got, tc.want)
			}
		})
	}
}
