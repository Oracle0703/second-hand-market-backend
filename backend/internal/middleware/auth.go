package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

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
