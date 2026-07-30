package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

var sessionIdentityTestNow = time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)

func TestSessionIdentityResolverUsesCurrentAccountState(t *testing.T) {
	t.Run("admin", func(t *testing.T) {
		db := newSessionIdentityTestDB(t)
		admin := model.AdminUser{
			ID:           101,
			Username:     "identity-admin",
			PasswordHash: "unused",
			Role:         model.AdminRoleAdmin,
			Status:       model.AccountStatusActive,
		}
		mustCreateSessionIdentityRecord(t, db, &admin)
		session := newSessionIdentityTestSession(t, db, model.UserTypeAdmin, admin.ID)
		resolver := newSessionIdentityResolver(db, func() time.Time { return sessionIdentityTestNow })

		actor, err := resolver.ResolveAccess(context.Background(), &AccessClaims{
			UserID:     admin.ID,
			UserType:   model.UserTypeAdmin,
			Role:       model.AdminRoleSuper,
			MerchantID: 999,
			Scope:      "onboarding",
			SessionID:  session.ID,
		})
		if err != nil {
			t.Fatalf("resolve admin identity: %v", err)
		}
		assertSessionIdentityActor(t, actor, common.Actor{
			UserID:    admin.ID,
			UserType:  model.UserTypeAdmin,
			Role:      model.AdminRoleAdmin,
			Scope:     "full",
			SessionID: session.ID,
		})

		mustSessionIdentityUpdate(t, db.Model(&model.AdminUser{}).
			Where("id = ?", admin.ID).
			Update("role", model.AdminRoleSuper))
		actor, err = resolver.ResolveAccess(context.Background(), &AccessClaims{
			UserID:    admin.ID,
			UserType:  model.UserTypeAdmin,
			Role:      model.AdminRoleAdmin,
			Scope:     "stale",
			SessionID: session.ID,
		})
		if err != nil {
			t.Fatalf("resolve updated admin identity: %v", err)
		}
		if actor.Role != model.AdminRoleSuper {
			t.Fatalf("admin role came from stale JWT claims: %+v", actor)
		}
	})

	t.Run("merchant", func(t *testing.T) {
		db := newSessionIdentityTestDB(t)
		approved := model.Merchant{
			ID:           201,
			MerchantNo:   "M-IDENTITY-APPROVED",
			MerchantName: "approved",
			ReviewStatus: model.ReviewApproved,
			ContactName:  "contact",
			ContactPhone: "10000000001",
		}
		pending := model.Merchant{
			ID:           202,
			MerchantNo:   "M-IDENTITY-PENDING",
			MerchantName: "pending",
			ReviewStatus: model.ReviewPending,
			ContactName:  "contact",
			ContactPhone: "10000000002",
		}
		account := model.MerchantAccount{
			ID:           102,
			MerchantID:   approved.ID,
			Username:     "identity-merchant",
			PasswordHash: "unused",
			Role:         model.AccountRoleOwner,
			Status:       model.AccountStatusActive,
		}
		mustCreateSessionIdentityRecord(t, db, &approved)
		mustCreateSessionIdentityRecord(t, db, &pending)
		mustCreateSessionIdentityRecord(t, db, &account)
		session := newSessionIdentityTestSession(t, db, model.UserTypeMerchant, account.ID)
		resolver := newSessionIdentityResolver(db, func() time.Time { return sessionIdentityTestNow })

		actor, err := resolver.ResolveAccess(context.Background(), &AccessClaims{
			UserID:     account.ID,
			UserType:   model.UserTypeMerchant,
			Role:       model.AccountRoleStaff,
			MerchantID: 999,
			Scope:      "onboarding",
			SessionID:  session.ID,
		})
		if err != nil {
			t.Fatalf("resolve merchant identity: %v", err)
		}
		assertSessionIdentityActor(t, actor, common.Actor{
			UserID:     account.ID,
			UserType:   model.UserTypeMerchant,
			Role:       model.AccountRoleOwner,
			MerchantID: approved.ID,
			Scope:      "full",
			SessionID:  session.ID,
		})

		mustSessionIdentityUpdate(t, db.Model(&model.MerchantAccount{}).
			Where("id = ?", account.ID).
			Updates(map[string]interface{}{
				"role":        model.AccountRoleStaff,
				"merchant_id": pending.ID,
			}))
		actor, err = resolver.ResolveAccess(context.Background(), &AccessClaims{
			UserID:     account.ID,
			UserType:   model.UserTypeMerchant,
			Role:       model.AccountRoleOwner,
			MerchantID: approved.ID,
			Scope:      "full",
			SessionID:  session.ID,
		})
		if err != nil {
			t.Fatalf("resolve reassigned merchant identity: %v", err)
		}
		assertSessionIdentityActor(t, actor, common.Actor{
			UserID:     account.ID,
			UserType:   model.UserTypeMerchant,
			Role:       model.AccountRoleStaff,
			MerchantID: pending.ID,
			Scope:      "onboarding",
			SessionID:  session.ID,
		})

		mustSessionIdentityUpdate(t, db.Model(&model.Merchant{}).
			Where("id = ?", pending.ID).
			Update("review_status", model.ReviewApproved))
		actor, err = resolver.ResolveAccess(context.Background(), &AccessClaims{
			UserID:     account.ID,
			UserType:   model.UserTypeMerchant,
			Role:       "stale",
			MerchantID: approved.ID,
			Scope:      "stale",
			SessionID:  session.ID,
		})
		if err != nil {
			t.Fatalf("resolve approved reassigned merchant identity: %v", err)
		}
		if actor.Scope != "full" || actor.MerchantID != pending.ID || actor.Role != model.AccountRoleStaff {
			t.Fatalf("merchant identity was not rebuilt from current state: %+v", actor)
		}
	})

	t.Run("buyer", func(t *testing.T) {
		db := newSessionIdentityTestDB(t)
		buyer := model.BuyerUser{
			ID:           103,
			BuyerNo:      "B-IDENTITY",
			AuthProvider: "wechat",
			OpenID:       "identity-openid",
			Status:       model.BuyerStatusActive,
		}
		mustCreateSessionIdentityRecord(t, db, &buyer)
		session := newSessionIdentityTestSession(t, db, model.UserTypeBuyer, buyer.ID)
		resolver := newSessionIdentityResolver(db, func() time.Time { return sessionIdentityTestNow })

		actor, err := resolver.ResolveAccess(context.Background(), &AccessClaims{
			UserID:     buyer.ID,
			UserType:   model.UserTypeBuyer,
			Role:       model.AdminRoleSuper,
			MerchantID: 999,
			Scope:      "stale",
			SessionID:  session.ID,
		})
		if err != nil {
			t.Fatalf("resolve buyer identity: %v", err)
		}
		assertSessionIdentityActor(t, actor, common.Actor{
			UserID:    buyer.ID,
			UserType:  model.UserTypeBuyer,
			Role:      model.UserTypeBuyer,
			Scope:     "full",
			SessionID: session.ID,
		})
	})
}

func TestSessionIdentityResolverRejectsInvalidSessions(t *testing.T) {
	db := newSessionIdentityTestDB(t)
	admin := model.AdminUser{
		ID:           301,
		Username:     "invalid-session-admin",
		PasswordHash: "unused",
		Role:         model.AdminRoleAdmin,
		Status:       model.AccountStatusActive,
	}
	mustCreateSessionIdentityRecord(t, db, &admin)
	resolver := newSessionIdentityResolver(db, func() time.Time { return sessionIdentityTestNow })
	revokedAt := sessionIdentityTestNow.Add(-time.Minute)

	tests := []struct {
		name        string
		session     model.AuthSession
		resolveID   uint64
		resolveType string
		resolveUser uint64
	}{
		{
			name:        "missing",
			resolveID:   9999,
			resolveType: model.UserTypeAdmin,
			resolveUser: admin.ID,
		},
		{
			name: "revoked",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID, RefreshTokenHash: "hash",
				ExpiredAt: sessionIdentityTestNow.Add(time.Hour), RevokedAt: &revokedAt,
			},
			resolveType: model.UserTypeAdmin,
			resolveUser: admin.ID,
		},
		{
			name: "expired",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID, RefreshTokenHash: "hash",
				ExpiredAt: sessionIdentityTestNow.Add(-time.Nanosecond),
			},
			resolveType: model.UserTypeAdmin,
			resolveUser: admin.ID,
		},
		{
			name: "expiry_boundary",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID, RefreshTokenHash: "hash",
				ExpiredAt: sessionIdentityTestNow,
			},
			resolveType: model.UserTypeAdmin,
			resolveUser: admin.ID,
		},
		{
			name: "user_type_mismatch_same_numeric_id",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID, RefreshTokenHash: "hash",
				ExpiredAt: sessionIdentityTestNow.Add(time.Hour),
			},
			resolveType: model.UserTypeMerchant,
			resolveUser: admin.ID,
		},
		{
			name: "user_id_mismatch",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID, RefreshTokenHash: "hash",
				ExpiredAt: sessionIdentityTestNow.Add(time.Hour),
			},
			resolveType: model.UserTypeAdmin,
			resolveUser: admin.ID + 1,
		},
		{
			name: "empty_refresh_hash",
			session: model.AuthSession{
				UserType: model.UserTypeAdmin, UserID: admin.ID,
				ExpiredAt: sessionIdentityTestNow.Add(time.Hour),
			},
			resolveType: model.UserTypeAdmin,
			resolveUser: admin.ID,
		},
		{
			name: "unknown_user_type",
			session: model.AuthSession{
				UserType: "UNKNOWN", UserID: admin.ID, RefreshTokenHash: "hash",
				ExpiredAt: sessionIdentityTestNow.Add(time.Hour),
			},
			resolveType: "UNKNOWN",
			resolveUser: admin.ID,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			resolveID := testCase.resolveID
			if testCase.session.UserType != "" {
				mustCreateSessionIdentityRecord(t, db, &testCase.session)
				resolveID = testCase.session.ID
			}
			_, err := resolver.Resolve(
				context.Background(),
				resolveID,
				testCase.resolveType,
				testCase.resolveUser,
			)
			if err != common.ErrUnauthorized {
				t.Fatalf("invalid session error = %v, want unauthorized", err)
			}
		})
	}

	if _, err := resolver.ResolveAccess(context.Background(), nil); err != common.ErrUnauthorized {
		t.Fatalf("nil access claims error = %v, want unauthorized", err)
	}
}

func TestSessionIdentityResolverRejectsAccountInvariants(t *testing.T) {
	tests := []struct {
		name     string
		userType string
		userID   uint64
		prepare  func(*testing.T, *gorm.DB)
		wantErr  error
	}{
		{
			name:     "missing_account",
			userType: model.UserTypeAdmin,
			userID:   401,
			prepare:  func(*testing.T, *gorm.DB) {},
			wantErr:  common.ErrUnauthorized,
		},
		{
			name:     "disabled_admin",
			userType: model.UserTypeAdmin,
			userID:   402,
			prepare: func(t *testing.T, db *gorm.DB) {
				mustCreateSessionIdentityRecord(t, db, &model.AdminUser{
					ID: 402, Username: "disabled-admin", PasswordHash: "unused",
					Role: model.AdminRoleAdmin, Status: model.AccountStatusDisabled,
				})
			},
			wantErr: common.ErrAccountDisabled,
		},
		{
			name:     "unknown_admin_role",
			userType: model.UserTypeAdmin,
			userID:   403,
			prepare: func(t *testing.T, db *gorm.DB) {
				mustCreateSessionIdentityRecord(t, db, &model.AdminUser{
					ID: 403, Username: "unknown-role-admin", PasswordHash: "unused",
					Role: "ROOT", Status: model.AccountStatusActive,
				})
			},
			wantErr: common.ErrUnauthorized,
		},
		{
			name:     "unknown_account_status",
			userType: model.UserTypeAdmin,
			userID:   404,
			prepare: func(t *testing.T, db *gorm.DB) {
				mustCreateSessionIdentityRecord(t, db, &model.AdminUser{
					ID: 404, Username: "unknown-status-admin", PasswordHash: "unused",
					Role: model.AdminRoleAdmin, Status: "LOCKED",
				})
			},
			wantErr: common.ErrUnauthorized,
		},
		{
			name:     "soft_deleted_admin",
			userType: model.UserTypeAdmin,
			userID:   408,
			prepare: func(t *testing.T, db *gorm.DB) {
				admin := model.AdminUser{
					ID: 408, Username: "deleted-admin", PasswordHash: "unused",
					Role: model.AdminRoleAdmin, Status: model.AccountStatusActive,
				}
				mustCreateSessionIdentityRecord(t, db, &admin)
				if result := db.Delete(&admin); result.Error != nil || result.RowsAffected != 1 {
					t.Fatal("soft delete isolated admin")
				}
			},
			wantErr: common.ErrUnauthorized,
		},
		{
			name:     "merchant_missing_membership",
			userType: model.UserTypeMerchant,
			userID:   405,
			prepare: func(t *testing.T, db *gorm.DB) {
				mustCreateSessionIdentityRecord(t, db, &model.MerchantAccount{
					ID: 405, Username: "missing-membership", PasswordHash: "unused",
					Role: model.AccountRoleOwner, Status: model.AccountStatusActive,
				})
			},
			wantErr: common.ErrUnauthorized,
		},
		{
			name:     "merchant_record_missing",
			userType: model.UserTypeMerchant,
			userID:   409,
			prepare: func(t *testing.T, db *gorm.DB) {
				mustCreateSessionIdentityRecord(t, db, &model.MerchantAccount{
					ID: 409, MerchantID: 509, Username: "missing-merchant",
					PasswordHash: "unused", Role: model.AccountRoleOwner,
					Status: model.AccountStatusActive,
				})
			},
			wantErr: common.ErrUnauthorized,
		},
		{
			name:     "merchant_role_invalid",
			userType: model.UserTypeMerchant,
			userID:   410,
			prepare: func(t *testing.T, db *gorm.DB) {
				merchant := model.Merchant{
					ID: 510, MerchantNo: "M-ROLE-INVALID", MerchantName: "invalid role",
					ContactName: "contact", ContactPhone: "10000000510",
					ReviewStatus: model.ReviewApproved,
				}
				mustCreateSessionIdentityRecord(t, db, &merchant)
				mustCreateSessionIdentityRecord(t, db, &model.MerchantAccount{
					ID: 410, MerchantID: merchant.ID, Username: "invalid-role",
					PasswordHash: "unused", Role: "ROOT",
					Status: model.AccountStatusActive,
				})
			},
			wantErr: common.ErrUnauthorized,
		},
		{
			name:     "merchant_review_disabled",
			userType: model.UserTypeMerchant,
			userID:   406,
			prepare: func(t *testing.T, db *gorm.DB) {
				merchant := model.Merchant{
					ID: 506, MerchantNo: "M-DISABLED", MerchantName: "disabled",
					ContactName: "contact", ContactPhone: "10000000506",
					ReviewStatus: model.ReviewDisabled,
				}
				mustCreateSessionIdentityRecord(t, db, &merchant)
				mustCreateSessionIdentityRecord(t, db, &model.MerchantAccount{
					ID: 406, MerchantID: merchant.ID, Username: "review-disabled",
					PasswordHash: "unused", Role: model.AccountRoleOwner,
					Status: model.AccountStatusActive,
				})
			},
			wantErr: common.ErrReviewNotApproved,
		},
		{
			name:     "merchant_review_unknown",
			userType: model.UserTypeMerchant,
			userID:   411,
			prepare: func(t *testing.T, db *gorm.DB) {
				merchant := model.Merchant{
					ID: 511, MerchantNo: "M-REVIEW-UNKNOWN", MerchantName: "unknown review",
					ContactName: "contact", ContactPhone: "10000000511",
					ReviewStatus: "ARCHIVED",
				}
				mustCreateSessionIdentityRecord(t, db, &merchant)
				mustCreateSessionIdentityRecord(t, db, &model.MerchantAccount{
					ID: 411, MerchantID: merchant.ID, Username: "unknown-review",
					PasswordHash: "unused", Role: model.AccountRoleOwner,
					Status: model.AccountStatusActive,
				})
			},
			wantErr: common.ErrUnauthorized,
		},
		{
			name:     "buyer_disabled",
			userType: model.UserTypeBuyer,
			userID:   407,
			prepare: func(t *testing.T, db *gorm.DB) {
				mustCreateSessionIdentityRecord(t, db, &model.BuyerUser{
					ID: 407, BuyerNo: "B-DISABLED", AuthProvider: "wechat",
					OpenID: "disabled-buyer", Status: model.BuyerStatusDisabled,
				})
			},
			wantErr: common.ErrAccountDisabled,
		},
		{
			name:     "buyer_status_unknown",
			userType: model.UserTypeBuyer,
			userID:   412,
			prepare: func(t *testing.T, db *gorm.DB) {
				mustCreateSessionIdentityRecord(t, db, &model.BuyerUser{
					ID: 412, BuyerNo: "B-UNKNOWN", AuthProvider: "wechat",
					OpenID: "unknown-buyer", Status: "LOCKED",
				})
			},
			wantErr: common.ErrUnauthorized,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := newSessionIdentityTestDB(t)
			testCase.prepare(t, db)
			session := newSessionIdentityTestSession(t, db, testCase.userType, testCase.userID)
			resolver := newSessionIdentityResolver(db, func() time.Time { return sessionIdentityTestNow })
			_, err := resolver.Resolve(
				context.Background(),
				session.ID,
				testCase.userType,
				testCase.userID,
			)
			if err != testCase.wantErr {
				t.Fatalf("account invariant error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestSessionIdentityResolverRedactsDatabaseErrors(t *testing.T) {
	t.Run("session_query", func(t *testing.T) {
		db := newSessionIdentityTestDB(t)
		admin := model.AdminUser{
			ID: 501, Username: "database-error-admin", PasswordHash: "unused",
			Role: model.AdminRoleAdmin, Status: model.AccountStatusActive,
		}
		mustCreateSessionIdentityRecord(t, db, &admin)
		session := newSessionIdentityTestSession(t, db, model.UserTypeAdmin, admin.ID)
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal("open test SQL database")
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatal("close test SQL database")
		}

		resolver := newSessionIdentityResolver(db, func() time.Time { return sessionIdentityTestNow })
		if _, err := resolver.Resolve(
			context.Background(),
			session.ID,
			model.UserTypeAdmin,
			admin.ID,
		); err != common.ErrInternal {
			t.Fatalf("session database error = %v, want fixed internal error", err)
		}
	})

	t.Run("account_query", func(t *testing.T) {
		db := newSessionIdentityTestDB(t)
		admin := model.AdminUser{
			ID: 502, Username: "account-query-error-admin", PasswordHash: "unused",
			Role: model.AdminRoleAdmin, Status: model.AccountStatusActive,
		}
		mustCreateSessionIdentityRecord(t, db, &admin)
		session := newSessionIdentityTestSession(t, db, model.UserTypeAdmin, admin.ID)
		if err := db.Migrator().DropTable(&model.AdminUser{}); err != nil {
			t.Fatal("remove isolated admin table")
		}
		resolver := newSessionIdentityResolver(db, func() time.Time { return sessionIdentityTestNow })
		if _, err := resolver.Resolve(
			context.Background(),
			session.ID,
			model.UserTypeAdmin,
			admin.ID,
		); err != common.ErrInternal {
			t.Fatalf("account database error = %v, want fixed internal error", err)
		}
	})

	var nilResolver *SessionIdentityResolver
	if _, err := nilResolver.Resolve(
		context.Background(),
		1,
		model.UserTypeAdmin,
		1,
	); err != common.ErrInternal {
		t.Fatalf("nil resolver error = %v, want fixed internal error", err)
	}
}

func newSessionIdentityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:session_identity_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal("open isolated session identity database")
	}
	if err := db.AutoMigrate(
		&model.AuthSession{},
		&model.AdminUser{},
		&model.Merchant{},
		&model.MerchantAccount{},
		&model.BuyerUser{},
	); err != nil {
		t.Fatal("migrate isolated session identity database")
	}
	return db
}

func newSessionIdentityTestSession(
	t *testing.T,
	db *gorm.DB,
	userType string,
	userID uint64,
) model.AuthSession {
	t.Helper()
	session := model.AuthSession{
		UserType:         userType,
		UserID:           userID,
		RefreshTokenHash: "session-identity-refresh-hash",
		ExpiredAt:        sessionIdentityTestNow.Add(time.Hour),
	}
	mustCreateSessionIdentityRecord(t, db, &session)
	return session
}

func mustCreateSessionIdentityRecord(t *testing.T, db *gorm.DB, value interface{}) {
	t.Helper()
	if err := db.Create(value).Error; err != nil {
		t.Fatal("create isolated session identity fixture")
	}
}

func mustSessionIdentityUpdate(t *testing.T, result *gorm.DB) {
	t.Helper()
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatal("update isolated session identity fixture")
	}
}

func assertSessionIdentityActor(t *testing.T, got, want common.Actor) {
	t.Helper()
	if got != want {
		t.Fatalf("actor = %+v, want %+v", got, want)
	}
}
