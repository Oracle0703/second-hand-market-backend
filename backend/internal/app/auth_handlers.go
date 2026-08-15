package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
)

func (s *Server) handleRegister(c *gin.Context) {
	var req dto.RegisterRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	var cnt int64
	if err := s.DB.Model(&model.MerchantAccount{}).Where("username = ?", req.Username).Count(&cnt).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	if cnt > 0 {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}

	merchant := model.Merchant{
		MerchantNo:   common.BuildBizNo("M"),
		MerchantName: req.MerchantName,
		ContactName:  req.ContactName,
		ContactPhone: req.Phone,
		ReviewStatus: model.ReviewPending,
	}
	merchant.LicenseFileID = &req.LicenseFileID
	acct := model.MerchantAccount{
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         model.AccountRoleOwner,
		Status:       model.AccountStatusActive,
	}

	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&merchant).Error; err != nil {
			return err
		}
		acct.MerchantID = merchant.ID
		if err := tx.Create(&acct).Error; err != nil {
			return err
		}
		if err := EnsureMerchantDefaultCategories(tx, merchant.ID); err != nil {
			return err
		}
		logItem := model.MerchantAuditLog{MerchantID: merchant.ID, Action: "SUBMIT", FromStatus: "", ToStatus: model.ReviewPending, OperatorType: model.UserTypeMerchant, OperatorID: acct.ID}
		return tx.Create(&logItem).Error
	}); err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"merchant_id": merchant.ID, "merchant_no": merchant.MerchantNo, "review_status": merchant.ReviewStatus})
}

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
	var admin model.AdminUser
	if err := s.DB.Where("username = ?", req.Username).First(&admin).Error; err != nil {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	if admin.Status == model.AccountStatusDisabled {
		common.Fail(c, common.ErrAccountDisabled)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	data, err := s.issueTokens(c, model.UserTypeAdmin, admin.ID, admin.Role, 0, "full")
	if err != nil {
		common.Fail(c, err)
		return
	}
	now := time.Now()
	_ = s.DB.Model(&model.AdminUser{}).Where("id = ?", admin.ID).Update("last_login_at", &now).Error
	data["user"] = gin.H{"id": admin.ID, "role": admin.Role}
	common.Success(c, data)
}

func (s *Server) merchantLogin(c *gin.Context, req dto.LoginRequest) {
	var acct model.MerchantAccount
	if err := s.DB.Where("username = ?", req.Username).First(&acct).Error; err != nil {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	if acct.Status == model.AccountStatusDisabled {
		common.Fail(c, common.ErrAccountDisabled)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acct.PasswordHash), []byte(req.Password)); err != nil {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	var merchant model.Merchant
	if err := s.DB.Where("id = ?", acct.MerchantID).First(&merchant).Error; err != nil {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	scope := "full"
	if merchant.ReviewStatus != model.ReviewApproved {
		scope = "onboarding"
	}
	data, err := s.issueTokens(c, model.UserTypeMerchant, acct.ID, acct.Role, merchant.ID, scope)
	if err != nil {
		common.Fail(c, err)
		return
	}
	now := time.Now()
	_ = s.DB.Model(&model.MerchantAccount{}).Where("id = ?", acct.ID).Update("last_login_at", &now).Error
	data["user"] = gin.H{"id": acct.ID, "role": acct.Role, "merchant_id": merchant.ID}
	data["token_scope"] = scope
	data["review_status"] = merchant.ReviewStatus
	common.Success(c, data)
}

func (s *Server) issueTokens(c *gin.Context, userType string, userID uint64, role string, merchantID uint64, scope string) (gin.H, error) {
	session := model.AuthSession{UserType: userType, UserID: userID, ExpiredAt: time.Now().Add(s.cfg.RefreshTTL)}
	ip := c.ClientIP()
	session.IP = &ip
	if err := s.DB.Create(&session).Error; err != nil {
		return nil, common.ErrInternal
	}
	refresh, refreshExp, err := auth.BuildRefreshToken(s.cfg.JWTRefreshSecret, auth.RefreshClaims{UserID: userID, UserType: userType, SessionID: session.ID}, s.cfg.RefreshTTL)
	if err != nil {
		return nil, common.ErrInternal
	}
	if err := s.DB.Model(&model.AuthSession{}).Where("id = ?", session.ID).Updates(map[string]interface{}{"refresh_token_hash": common.SHA256(refresh), "expired_at": refreshExp}).Error; err != nil {
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
	var session model.AuthSession
	if err := s.DB.Where("id = ?", claims.SessionID).First(&session).Error; err != nil {
		common.Fail(c, common.ErrUnauthorized)
		return
	}
	if session.RevokedAt != nil || session.ExpiredAt.Before(time.Now()) || session.RefreshTokenHash != common.SHA256(req.RefreshToken) {
		common.Fail(c, common.ErrUnauthorized)
		return
	}

	role := ""
	merchantID := uint64(0)
	scope := "full"
	switch claims.UserType {
	case model.UserTypeAdmin:
		var admin model.AdminUser
		if err := s.DB.Where("id = ?", claims.UserID).First(&admin).Error; err != nil {
			common.Fail(c, common.ErrUnauthorized)
			return
		}
		if admin.Status == model.AccountStatusDisabled {
			common.Fail(c, common.ErrAccountDisabled)
			return
		}
		role = admin.Role
	case model.UserTypeMerchant:
		var acct model.MerchantAccount
		if err := s.DB.Where("id = ?", claims.UserID).First(&acct).Error; err != nil {
			common.Fail(c, common.ErrUnauthorized)
			return
		}
		if acct.Status == model.AccountStatusDisabled {
			common.Fail(c, common.ErrAccountDisabled)
			return
		}
		role = acct.Role
		merchantID = acct.MerchantID
		var merchant model.Merchant
		if err := s.DB.Where("id = ?", acct.MerchantID).First(&merchant).Error; err != nil {
			common.Fail(c, common.ErrUnauthorized)
			return
		}
		if merchant.ReviewStatus != model.ReviewApproved {
			scope = "onboarding"
		}
	case model.UserTypeBuyer:
		var buyer model.BuyerUser
		if err := s.DB.Where("id = ?", claims.UserID).First(&buyer).Error; err != nil {
			common.Fail(c, common.ErrUnauthorized)
			return
		}
		if buyer.Status == model.BuyerStatusDisabled {
			common.Fail(c, common.ErrAccountDisabled)
			return
		}
		role = model.UserTypeBuyer
	default:
		common.Fail(c, common.ErrUnauthorized)
		return
	}

	newRefresh, refreshExp, err := auth.BuildRefreshToken(s.cfg.JWTRefreshSecret, auth.RefreshClaims{UserID: claims.UserID, UserType: claims.UserType, SessionID: session.ID}, s.cfg.RefreshTTL)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	access, _, err := auth.BuildAccessToken(s.cfg.JWTAccessSecret, auth.AccessClaims{UserID: claims.UserID, UserType: claims.UserType, Role: role, MerchantID: merchantID, Scope: scope, SessionID: session.ID}, s.cfg.AccessTTL)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	if err := s.DB.Model(&model.AuthSession{}).Where("id = ?", session.ID).Updates(map[string]interface{}{"refresh_token_hash": common.SHA256(newRefresh), "expired_at": refreshExp}).Error; err != nil {
		common.Fail(c, common.ErrInternal)
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
	if err := s.DB.Model(&model.AuthSession{}).Where("id = ?", actor.SessionID).Update("revoked_at", &now).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"success": true})
}
