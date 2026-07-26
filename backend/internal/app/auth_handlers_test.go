package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func newAuthHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)",
		strings.ReplaceAll(t.Name(), "/", "_"),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open auth handler database: %v", err)
	}
	if err := db.AutoMigrate(&model.AuthSession{}); err != nil {
		t.Fatalf("migrate auth handler database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get auth handler sql database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func createRevocationTestSession(t *testing.T, db *gorm.DB, now time.Time) model.AuthSession {
	t.Helper()
	session := model.AuthSession{
		UserType:  model.UserTypeMerchant,
		UserID:    41,
		ExpiredAt: now.Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	return session
}

func TestRevokeCurrentSessionRequiresExactActiveIdentity(t *testing.T) {
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		actor func(model.AuthSession) common.Actor
	}{
		{
			name: "wrong user id",
			actor: func(session model.AuthSession) common.Actor {
				return common.Actor{UserType: session.UserType, UserID: session.UserID + 1, SessionID: session.ID}
			},
		},
		{
			name: "wrong user type",
			actor: func(session model.AuthSession) common.Actor {
				return common.Actor{UserType: model.UserTypeBuyer, UserID: session.UserID, SessionID: session.ID}
			},
		},
		{
			name: "wrong session id",
			actor: func(session model.AuthSession) common.Actor {
				return common.Actor{UserType: session.UserType, UserID: session.UserID, SessionID: session.ID + 1}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newAuthHandlerTestDB(t)
			session := createRevocationTestSession(t, db, now)
			if err := revokeCurrentSession(db, tc.actor(session), now); !errors.Is(err, common.ErrUnauthorized) {
				t.Fatalf("identity mismatch error = %v", err)
			}
			var unchanged model.AuthSession
			if err := db.First(&unchanged, session.ID).Error; err != nil {
				t.Fatalf("reload session: %v", err)
			}
			if unchanged.RevokedAt != nil {
				t.Fatal("identity mismatch changed session")
			}
		})
	}

	db := newAuthHandlerTestDB(t)
	session := createRevocationTestSession(t, db, now)
	exact := common.Actor{UserType: session.UserType, UserID: session.UserID, SessionID: session.ID}
	if err := revokeCurrentSession(db, exact, now); err != nil {
		t.Fatalf("exact revoke: %v", err)
	}
	if err := revokeCurrentSession(db, exact, now); !errors.Is(err, common.ErrUnauthorized) {
		t.Fatalf("second revoke error = %v", err)
	}
}

func TestRevokeCurrentSessionMapsDatabaseFailureToInternal(t *testing.T) {
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	db := newAuthHandlerTestDB(t)
	session := createRevocationTestSession(t, db, now)
	errSynthetic := errors.New("synthetic session update failure")
	if err := db.Callback().Update().Before("gorm:update").
		Register("test:fail_session_revocation", func(tx *gorm.DB) {
			if tx.Statement.Table == "auth_sessions" {
				tx.AddError(errSynthetic)
			}
		}); err != nil {
		t.Fatalf("register update callback: %v", err)
	}
	actor := common.Actor{UserType: session.UserType, UserID: session.UserID, SessionID: session.ID}
	if err := revokeCurrentSession(db, actor, now); !errors.Is(err, common.ErrInternal) {
		t.Fatalf("database failure error = %v", err)
	}
}
