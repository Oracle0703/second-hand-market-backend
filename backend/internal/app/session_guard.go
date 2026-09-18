package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

// Reload account authority rather than trusting stale role/status claims.
func (s *Server) sessionGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := common.GetActor(c)
		if !ok {
			c.Next()
			return
		}
		identity, err := s.resolveSessionIdentity(c.Request.Context(), actor.SessionID, actor.UserType, actor.UserID)
		if err != nil {
			// Preserve access-token invalidation semantics for inactive identities.
			if err != common.ErrInternal {
				err = common.ErrUnauthorized
			}
			common.Fail(c, err)
			c.Abort()
			return
		}
		if identity.MustChangePassword {
			path := c.Request.URL.Path
			if path != "/api/v1/merchant/account" && path != "/api/v1/merchant/account/password" && path != "/api/v1/auth/logout" && path != "/api/v1/auth/refresh" {
				common.Fail(c, common.ErrForbidden)
				c.Abort()
				return
			}
		}
		common.SetActor(c, identity.Actor)
		c.Next()
	}
}

type sessionIdentity struct {
	Actor              common.Actor
	Session            model.AuthSession
	MustChangePassword bool
}

// Access and refresh must use the same live session and account authority.
// Roles, merchant ownership and review scope come from the database, not JWT claims.
func (s *Server) resolveSessionIdentity(ctx context.Context, sessionID uint64, userType string, userID uint64) (sessionIdentity, error) {
	identity := sessionIdentity{}
	if sessionID == 0 || userID == 0 {
		return identity, common.ErrUnauthorized
	}
	if userType != model.UserTypeAdmin && userType != model.UserTypeMerchant && userType != model.UserTypeBuyer {
		return identity, common.ErrUnauthorized
	}
	db := s.DB.WithContext(ctx)
	if err := db.Where("id = ? AND user_id = ? AND user_type = ? AND revoked_at IS NULL AND expired_at > ?", sessionID, userID, userType, time.Now()).First(&identity.Session).Error; err != nil {
		return identity, sessionLookupError(err)
	}
	if strings.TrimSpace(identity.Session.RefreshTokenHash) == "" {
		return identity, common.ErrUnauthorized
	}
	actor := common.Actor{UserID: userID, UserType: userType, SessionID: sessionID, Scope: "full"}
	switch userType {
	case model.UserTypeAdmin:
		var account model.AdminUser
		if err := db.First(&account, userID).Error; err != nil {
			return identity, sessionLookupError(err)
		}
		if err := activeSessionAccount(account.Status); err != nil {
			return identity, err
		}
		if account.Role != model.AdminRoleAdmin && account.Role != model.AdminRoleSuper {
			return identity, common.ErrUnauthorized
		}
		actor.Role = account.Role
	case model.UserTypeMerchant:
		var account model.MerchantAccount
		if err := db.First(&account, userID).Error; err != nil {
			return identity, sessionLookupError(err)
		}
		if err := activeSessionAccount(account.Status); err != nil {
			return identity, err
		}
		if account.MerchantID == 0 || (account.Role != model.AccountRoleOwner && account.Role != model.AccountRoleStaff) {
			return identity, common.ErrUnauthorized
		}
		var merchant model.Merchant
		if err := db.First(&merchant, account.MerchantID).Error; err != nil {
			return identity, sessionLookupError(err)
		}
		switch merchant.ReviewStatus {
		case model.ReviewApproved:
		case model.ReviewPending, model.ReviewRejected:
			actor.Scope = "onboarding"
		case model.ReviewDisabled:
			return identity, common.ErrReviewNotApproved
		default:
			return identity, common.ErrUnauthorized
		}
		actor.Role, actor.MerchantID = account.Role, account.MerchantID
		identity.MustChangePassword = account.MustChangePassword
	case model.UserTypeBuyer:
		var buyer model.BuyerUser
		if err := db.First(&buyer, userID).Error; err != nil {
			return identity, sessionLookupError(err)
		}
		if err := activeSessionAccount(buyer.Status); err != nil {
			return identity, err
		}
		actor.Role = model.UserTypeBuyer
	}
	identity.Actor = actor
	return identity, nil
}

func activeSessionAccount(status string) error {
	if status == model.AccountStatusDisabled {
		return common.ErrAccountDisabled
	}
	if status != model.AccountStatusActive {
		return common.ErrUnauthorized
	}
	return nil
}

func sessionLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return common.ErrUnauthorized
	}
	return common.ErrInternal
}
