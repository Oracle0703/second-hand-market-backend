package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
		common.Success(c, gin.H{"reached": true})
	})
	return r
}

func serveAuthProbe(router http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
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
