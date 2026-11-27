package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
)

func (s *Server) handleLogin(c *gin.Context) {
	var req dto.LoginRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	loginType := strings.ToUpper(req.LoginType)
	if loginType == model.UserTypeAdmin {
		s.adminLogin(c, req)
		return
	}
	s.merchantLogin(c, req)
}

func (s *Server) adminLogin(c *gin.Context, req dto.LoginRequest) {
	var data gin.H
	var admin model.AdminUser
	err := s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		// Serialize credential verification and session creation with self-service
		// password changes so an old-password login cannot escape revocation.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("username = ?", req.Username).First(&admin).Error; err != nil {
			return sessionLookupError(err)
		}
		if err := activeSessionAccount(admin.Status); err != nil {
			return err
		}
		if admin.Role != model.AdminRoleAdmin && admin.Role != model.AdminRoleSuper {
			return common.ErrUnauthorized
		}
		if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)) != nil {
			return common.ErrUnauthorized
		}
		var err error
		data, err = s.issueTokensWithDB(c, tx, model.UserTypeAdmin, admin.ID, admin.Role, 0, "full")
		if err != nil {
			return err
		}
		return tx.Model(&model.AdminUser{}).Where("id = ?", admin.ID).Update("last_login_at", time.Now()).Error
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	data["user"] = gin.H{"id": admin.ID, "role": admin.Role}
	common.Success(c, data)
}

func (s *Server) merchantLogin(c *gin.Context, req dto.LoginRequest) {
	var data gin.H
	var acct model.MerchantAccount
	var merchant model.Merchant
	scope := "full"
	err := s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		// Serialize credential verification and session creation with password
		// changes, resets and status changes so an old-password login cannot
		// escape their session revocation.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("username = ?", req.Username).First(&acct).Error; err != nil {
			return sessionLookupError(err)
		}
		if err := activeSessionAccount(acct.Status); err != nil {
			return err
		}
		if bcrypt.CompareHashAndPassword([]byte(acct.PasswordHash), []byte(req.Password)) != nil {
			return common.ErrUnauthorized
		}
		if err := tx.Where("id = ?", acct.MerchantID).First(&merchant).Error; err != nil {
			return sessionLookupError(err)
		}
		if merchant.ReviewStatus != model.ReviewApproved {
			scope = "onboarding"
		}
		var err error
		data, err = s.issueTokensWithDB(c, tx, model.UserTypeMerchant, acct.ID, acct.Role, merchant.ID, scope)
		if err != nil {
			return err
		}
		return tx.Model(&model.MerchantAccount{}).Where("id = ?", acct.ID).Update("last_login_at", time.Now()).Error
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	data["user"] = gin.H{"id": acct.ID, "role": acct.Role, "merchant_id": merchant.ID, "must_change_password": acct.MustChangePassword}
	data["token_scope"] = scope
	data["review_status"] = merchant.ReviewStatus
	common.Success(c, data)
}

func (s *Server) issueTokens(c *gin.Context, userType string, userID uint64, role string, merchantID uint64, scope string) (gin.H, error) {
	return s.issueTokensWithDB(c, s.DB, userType, userID, role, merchantID, scope)
}

func (s *Server) issueTokensWithDB(c *gin.Context, db *gorm.DB, userType string, userID uint64, role string, merchantID uint64, scope string) (gin.H, error) {
	session := model.AuthSession{UserType: userType, UserID: userID, ExpiredAt: time.Now().Add(s.cfg.RefreshTTL)}
	ip := c.ClientIP()
	session.IP = &ip
	if err := db.Create(&session).Error; err != nil {
		return nil, common.ErrInternal
	}
	refresh, refreshExp, err := auth.BuildRefreshToken(s.cfg.JWTRefreshSecret, auth.RefreshClaims{UserID: userID, UserType: userType, SessionID: session.ID}, s.cfg.RefreshTTL)
	if err != nil {
		return nil, common.ErrInternal
	}
	if err := db.Model(&model.AuthSession{}).Where("id = ?", session.ID).Updates(map[string]interface{}{"refresh_token_hash": common.SHA256(refresh), "expired_at": refreshExp}).Error; err != nil {
		return nil, common.ErrInternal
	}
	access, _, err := auth.BuildAccessToken(s.cfg.JWTAccessSecret, auth.AccessClaims{UserID: userID, UserType: userType, Role: role, MerchantID: merchantID, Scope: scope, SessionID: session.ID}, s.cfg.AccessTTL)
	if err != nil {
		return nil, common.ErrInternal
	}
	return gin.H{"access_token": access, "refresh_token": refresh, "expires_in": int(s.cfg.AccessTTL.Seconds())}, nil
}

func (s *Server) handleRefresh(c *gin.Context) {
	var req dto.RefreshRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	claims, err := auth.ParseRefreshToken(s.cfg.JWTRefreshSecret, req.RefreshToken)
	if err != nil {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	identity, err := s.resolveSessionIdentity(c.Request.Context(), claims.SessionID, claims.UserType, claims.UserID)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if identity.Session.RefreshTokenHash != common.SHA256(req.RefreshToken) {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	session, actor := identity.Session, identity.Actor

	newRefresh, refreshExp, err := auth.BuildRefreshToken(s.cfg.JWTRefreshSecret, auth.RefreshClaims{UserID: claims.UserID, UserType: claims.UserType, SessionID: session.ID}, s.cfg.RefreshTTL)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	access, _, err := auth.BuildAccessToken(s.cfg.JWTAccessSecret, auth.AccessClaims{UserID: claims.UserID, UserType: claims.UserType, Role: actor.Role, MerchantID: actor.MerchantID, Scope: actor.Scope, SessionID: session.ID}, s.cfg.AccessTTL)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	// Compare-and-swap on the presented hash so a refresh token rotates exactly once.
	result := s.DB.WithContext(c.Request.Context()).Model(&model.AuthSession{}).
		Where("id = ? AND refresh_token_hash = ? AND revoked_at IS NULL AND expired_at > ?", session.ID, session.RefreshTokenHash, time.Now()).
		Updates(map[string]interface{}{"refresh_token_hash": common.SHA256(newRefresh), "expired_at": refreshExp})
	if result.Error != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	if result.RowsAffected != 1 {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	common.Success(c, gin.H{"access_token": access, "refresh_token": newRefresh, "expires_in": int(s.cfg.AccessTTL.Seconds())})
}

func (s *Server) handleLogout(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	now := time.Now()
	result := s.DB.WithContext(c.Request.Context()).Model(&model.AuthSession{}).
		Where("id = ? AND user_type = ? AND user_id = ? AND revoked_at IS NULL AND expired_at > ?", actor.SessionID, actor.UserType, actor.UserID, now).
		Update("revoked_at", now)
	if result.Error != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	if result.RowsAffected != 1 {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	common.Success(c, gin.H{"success": true})
}
