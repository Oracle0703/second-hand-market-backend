package app

import (
	"time"

	"github.com/gin-gonic/gin"

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
		reject := func() { common.Fail(c, common.ErrUnauthorized); c.Abort() }
		var session model.AuthSession
		if err := s.DB.Where("id = ? AND user_id = ? AND user_type = ? AND revoked_at IS NULL AND expired_at > ?", actor.SessionID, actor.UserID, actor.UserType, time.Now()).First(&session).Error; err != nil {
			reject()
			return
		}
		switch actor.UserType {
		case model.UserTypeAdmin:
			var account model.AdminUser
			if err := s.DB.First(&account, actor.UserID).Error; err != nil || account.Status != model.AccountStatusActive {
				reject()
				return
			}
			actor.Role = account.Role
		case model.UserTypeMerchant:
			var account model.MerchantAccount
			if err := s.DB.First(&account, actor.UserID).Error; err != nil || account.Status != model.AccountStatusActive || account.MerchantID != actor.MerchantID {
				reject()
				return
			}
			actor.Role = account.Role
			var merchant model.Merchant
			if err := s.DB.First(&merchant, account.MerchantID).Error; err != nil {
				reject()
				return
			}
			actor.Scope = "onboarding"
			if merchant.ReviewStatus == model.ReviewApproved {
				actor.Scope = "full"
			}
			if account.MustChangePassword {
				path := c.Request.URL.Path
				if path != "/api/v1/merchant/account" && path != "/api/v1/merchant/account/password" && path != "/api/v1/auth/logout" && path != "/api/v1/auth/refresh" {
					common.Fail(c, common.ErrForbidden)
					c.Abort()
					return
				}
			}
		}
		common.SetActor(c, actor)
		c.Next()
	}
}
