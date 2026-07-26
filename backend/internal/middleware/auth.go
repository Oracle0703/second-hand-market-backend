package middleware

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func OptionalAuth(accessSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		head := c.GetHeader("Authorization")
		if head == "" {
			c.Next()
			return
		}
		parts := strings.SplitN(head, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			common.Fail(c, common.ErrUnauthorized)
			c.Abort()
			return
		}
		claims, err := auth.ParseAccessToken(accessSecret, parts[1])
		if err != nil {
			common.Fail(c, common.ErrUnauthorized)
			c.Abort()
			return
		}
		common.SetActor(c, common.Actor{UserID: claims.UserID, UserType: claims.UserType, Role: claims.Role, MerchantID: claims.MerchantID, Scope: claims.Scope, SessionID: claims.SessionID})
		c.Next()
	}
}

func RequireActiveSession(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := common.GetActor(c)
		if !ok {
			c.Next()
			return
		}
		if err := requireActiveSession(db, actor, time.Now()); err != nil {
			common.Fail(c, err)
			c.Abort()
			return
		}
		current, err := loadAuthoritativeActor(db, actor)
		if err != nil {
			common.Fail(c, err)
			c.Abort()
			return
		}
		common.SetActor(c, current)
		c.Next()
	}
}

func requireActiveSession(db *gorm.DB, actor common.Actor, now time.Time) error {
	if actor.SessionID == 0 {
		return common.ErrUnauthorized
	}
	var session model.AuthSession
	if err := db.Select("id", "user_type", "user_id", "expired_at", "revoked_at").
		Where("id = ?", actor.SessionID).Take(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return common.ErrUnauthorized
		}
		return common.ErrInternal
	}
	if session.UserType != actor.UserType || session.UserID != actor.UserID ||
		session.RevokedAt != nil || !session.ExpiredAt.After(now) {
		return common.ErrUnauthorized
	}
	return nil
}

type accountAuthorizationState struct {
	Status string
	Role   string
}

type merchantAuthorizationState struct {
	AccountStatus string `gorm:"column:account_status"`
	Role          string
	MerchantID    uint64
	ReviewStatus  string
}

func authorizationQueryError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return common.ErrUnauthorized
	}
	return common.ErrInternal
}

func loadAuthoritativeActor(db *gorm.DB, actor common.Actor) (common.Actor, error) {
	switch actor.UserType {
	case model.UserTypeAdmin:
		var state accountAuthorizationState
		if err := db.Model(&model.AdminUser{}).Select("status", "role").
			Where("id = ?", actor.UserID).Take(&state).Error; err != nil {
			return common.Actor{}, authorizationQueryError(err)
		}
		if state.Status == model.AccountStatusDisabled {
			return common.Actor{}, common.ErrAccountDisabled
		}
		if state.Status != model.AccountStatusActive ||
			(state.Role != model.AdminRoleSuper && state.Role != model.AdminRoleAdmin) {
			return common.Actor{}, common.ErrInternal
		}
		actor.Role = state.Role
		actor.MerchantID = 0
		actor.Scope = "full"
		return actor, nil

	case model.UserTypeMerchant:
		var state merchantAuthorizationState
		err := db.Table("merchant_accounts AS account").
			Select("account.status AS account_status, account.role, account.merchant_id, merchant.review_status").
			Joins("JOIN merchants AS merchant ON merchant.id = account.merchant_id AND merchant.deleted_at IS NULL").
			Where("account.id = ? AND account.deleted_at IS NULL", actor.UserID).
			Take(&state).Error
		if err != nil {
			return common.Actor{}, authorizationQueryError(err)
		}
		if state.AccountStatus == model.AccountStatusDisabled || state.ReviewStatus == model.ReviewDisabled {
			return common.Actor{}, common.ErrAccountDisabled
		}
		if state.AccountStatus != model.AccountStatusActive ||
			(state.Role != model.AccountRoleOwner && state.Role != model.AccountRoleStaff) {
			return common.Actor{}, common.ErrInternal
		}
		switch state.ReviewStatus {
		case model.ReviewApproved:
			actor.Scope = "full"
		case model.ReviewPending, model.ReviewRejected:
			actor.Scope = "onboarding"
		default:
			return common.Actor{}, common.ErrInternal
		}
		actor.Role = state.Role
		actor.MerchantID = state.MerchantID
		return actor, nil

	case model.UserTypeBuyer:
		var state struct{ Status string }
		if err := db.Model(&model.BuyerUser{}).Select("status").
			Where("id = ?", actor.UserID).Take(&state).Error; err != nil {
			return common.Actor{}, authorizationQueryError(err)
		}
		if state.Status == model.BuyerStatusDisabled {
			return common.Actor{}, common.ErrAccountDisabled
		}
		if state.Status != model.BuyerStatusActive {
			return common.Actor{}, common.ErrInternal
		}
		actor.Role = model.UserTypeBuyer
		actor.MerchantID = 0
		actor.Scope = "full"
		return actor, nil

	default:
		return common.Actor{}, common.ErrUnauthorized
	}
}

func RequireAuth(userTypes ...string) gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, t := range userTypes {
		allowed[t] = true
	}
	return func(c *gin.Context) {
		actor, ok := common.GetActor(c)
		if !ok {
			common.Fail(c, common.ErrUnauthorized)
			c.Abort()
			return
		}
		if len(allowed) > 0 && !allowed[actor.UserType] {
			common.Fail(c, common.ErrForbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireFullMerchantScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := common.GetActor(c)
		if !ok || actor.UserType != model.UserTypeMerchant {
			common.Fail(c, common.ErrUnauthorized)
			c.Abort()
			return
		}
		if actor.Scope != "full" {
			common.Fail(c, common.ErrReviewNotApproved)
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireMerchantScope(allowed ...string) gin.HandlerFunc {
	allow := map[string]bool{}
	for _, v := range allowed {
		allow[v] = true
	}
	return func(c *gin.Context) {
		actor, ok := common.GetActor(c)
		if !ok || actor.UserType != model.UserTypeMerchant {
			common.Fail(c, common.ErrUnauthorized)
			c.Abort()
			return
		}
		if len(allow) > 0 && !allow[actor.Scope] {
			common.Fail(c, common.ErrForbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}
