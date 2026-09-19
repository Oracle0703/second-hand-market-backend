package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

type adminPasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,max=72"`
	NewPassword string `json:"new_password" binding:"required"`
}

func (s *Server) handleAdminAccount(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var account model.AdminUser
	if err := s.DB.WithContext(c.Request.Context()).First(&account, actor.UserID).Error; err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	common.Success(c, gin.H{"account": gin.H{
		"id": account.ID, "username": account.Username, "display_name": account.DisplayName,
		"role": account.Role, "status": account.Status, "last_login_at": account.LastLoginAt,
	}})
}

func (s *Server) handleAdminChangePassword(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	// Use the authenticated identity only; no account ID or username is accepted.
	if actor.UserType != model.UserTypeAdmin {
		common.Fail(c, common.ErrForbidden)
		return
	}
	var req adminPasswordRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	if !common.ValidPassword(req.NewPassword) || req.NewPassword == req.OldPassword || strings.EqualFold(req.NewPassword, "Admin@123456") {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	now := time.Now()
	err = s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var account model.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, actor.UserID).Error; err != nil {
			return sessionLookupError(err)
		}
		if err := activeSessionAccount(account.Status); err != nil {
			return err
		}
		if account.Role != model.AdminRoleAdmin && account.Role != model.AdminRoleSuper {
			return common.ErrForbidden
		}
		// A revocation between middleware and this transaction must also be honored.
		var active int64
		if err := tx.Model(&model.AuthSession{}).Where("id = ? AND user_type = ? AND user_id = ? AND revoked_at IS NULL AND expired_at > ?", actor.SessionID, model.UserTypeAdmin, account.ID, now).Count(&active).Error; err != nil {
			return err
		}
		if active != 1 {
			return common.ErrUnauthorized
		}
		if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(req.OldPassword)) != nil {
			return common.ErrInvalidArgument
		}
		result := tx.Model(&model.AdminUser{}).Where("id = ? AND password_hash = ? AND status = ?", account.ID, account.PasswordHash, model.AccountStatusActive).Update("password_hash", string(hash))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return common.ErrConflict
		}
		if err := tx.Model(&model.AuthSession{}).Where("user_type = ? AND user_id = ? AND revoked_at IS NULL", model.UserTypeAdmin, account.ID).Update("revoked_at", now).Error; err != nil {
			return err
		}
		return s.persistOperationLog(c, tx, "admin_user", account.ID, "admin_password_change", nil, nil, common.CodeOK, nil, nil)
	})
	if err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	common.Success(c, gin.H{"success": true})
}
