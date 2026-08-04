package auth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

type ResolvedSessionIdentity struct {
	Actor            common.Actor
	RefreshTokenHash string
	ExpiredAt        time.Time
}

type SessionIdentityResolver struct {
	db  *gorm.DB
	now func() time.Time
}

func NewSessionIdentityResolver(db *gorm.DB) *SessionIdentityResolver {
	return newSessionIdentityResolver(db, time.Now)
}

func newSessionIdentityResolver(db *gorm.DB, now func() time.Time) *SessionIdentityResolver {
	return &SessionIdentityResolver{db: db, now: now}
}

func (r *SessionIdentityResolver) ResolveAccess(ctx context.Context, claims *AccessClaims) (common.Actor, error) {
	if claims == nil {
		return common.Actor{}, common.ErrUnauthorized
	}
	resolved, err := r.Resolve(ctx, claims.SessionID, claims.UserType, claims.UserID)
	if err != nil {
		return common.Actor{}, err
	}
	return resolved.Actor, nil
}

func (r *SessionIdentityResolver) Resolve(
	ctx context.Context,
	sessionID uint64,
	userType string,
	userID uint64,
) (ResolvedSessionIdentity, error) {
	if r == nil || r.db == nil || r.now == nil {
		return ResolvedSessionIdentity{}, common.ErrInternal
	}
	if sessionID == 0 || userID == 0 {
		return ResolvedSessionIdentity{}, common.ErrUnauthorized
	}

	var session model.AuthSession
	if err := r.db.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error; err != nil {
		return ResolvedSessionIdentity{}, sessionIdentityLookupError(err)
	}
	now := r.now()
	if session.UserType != userType ||
		session.UserID != userID ||
		session.UserType == "" ||
		session.RefreshTokenHash == "" ||
		session.ExpiredAt.IsZero() ||
		!session.ExpiredAt.After(now) ||
		session.RevokedAt != nil {
		return ResolvedSessionIdentity{}, common.ErrUnauthorized
	}

	actor, err := r.resolveCurrentActor(ctx, session)
	if err != nil {
		return ResolvedSessionIdentity{}, err
	}
	return ResolvedSessionIdentity{
		Actor:            actor,
		RefreshTokenHash: session.RefreshTokenHash,
		ExpiredAt:        session.ExpiredAt,
	}, nil
}

func (r *SessionIdentityResolver) resolveCurrentActor(
	ctx context.Context,
	session model.AuthSession,
) (common.Actor, error) {
	actor := common.Actor{
		UserID:    session.UserID,
		UserType:  session.UserType,
		SessionID: session.ID,
	}

	switch session.UserType {
	case model.UserTypeAdmin:
		var admin model.AdminUser
		if err := r.db.WithContext(ctx).Where("id = ?", session.UserID).First(&admin).Error; err != nil {
			return common.Actor{}, sessionIdentityLookupError(err)
		}
		if err := activeAccountStatusError(admin.Status); err != nil {
			return common.Actor{}, err
		}
		if admin.Role != model.AdminRoleSuper && admin.Role != model.AdminRoleAdmin {
			return common.Actor{}, common.ErrUnauthorized
		}
		actor.Role = admin.Role
		actor.Scope = "full"
	case model.UserTypeMerchant:
		var account model.MerchantAccount
		if err := r.db.WithContext(ctx).Where("id = ?", session.UserID).First(&account).Error; err != nil {
			return common.Actor{}, sessionIdentityLookupError(err)
		}
		if err := activeAccountStatusError(account.Status); err != nil {
			return common.Actor{}, err
		}
		if account.Role != model.AccountRoleOwner && account.Role != model.AccountRoleStaff {
			return common.Actor{}, common.ErrUnauthorized
		}
		if account.MerchantID == 0 {
			return common.Actor{}, common.ErrUnauthorized
		}

		var merchant model.Merchant
		if err := r.db.WithContext(ctx).Where("id = ?", account.MerchantID).First(&merchant).Error; err != nil {
			return common.Actor{}, sessionIdentityLookupError(err)
		}
		actor.Role = account.Role
		actor.MerchantID = account.MerchantID
		switch merchant.ReviewStatus {
		case model.ReviewApproved:
			actor.Scope = "full"
		case model.ReviewPending, model.ReviewRejected:
			actor.Scope = "onboarding"
		case model.ReviewDisabled:
			return common.Actor{}, common.ErrReviewNotApproved
		default:
			return common.Actor{}, common.ErrUnauthorized
		}
	case model.UserTypeBuyer:
		var buyer model.BuyerUser
		if err := r.db.WithContext(ctx).Where("id = ?", session.UserID).First(&buyer).Error; err != nil {
			return common.Actor{}, sessionIdentityLookupError(err)
		}
		switch buyer.Status {
		case model.BuyerStatusActive:
		case model.BuyerStatusDisabled:
			return common.Actor{}, common.ErrAccountDisabled
		default:
			return common.Actor{}, common.ErrUnauthorized
		}
		actor.Role = model.UserTypeBuyer
		actor.Scope = "full"
	default:
		return common.Actor{}, common.ErrUnauthorized
	}

	return actor, nil
}

func activeAccountStatusError(status string) error {
	switch status {
	case model.AccountStatusActive:
		return nil
	case model.AccountStatusDisabled:
		return common.ErrAccountDisabled
	default:
		return common.ErrUnauthorized
	}
}

func sessionIdentityLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return common.ErrUnauthorized
	}
	return common.ErrInternal
}
