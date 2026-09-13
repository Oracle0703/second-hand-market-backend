package app

import (
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

var merchantUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)

type createMerchantRequest struct {
	MerchantName string `json:"merchant_name" binding:"required,max=128"`
	ContactName  string `json:"contact_name" binding:"required,max=64"`
	Phone        string `json:"phone" binding:"required,max=20"`
	Username     string `json:"username" binding:"required,min=3,max=64"`
	Password     string `json:"password" binding:"required"`
}

func (s *Server) handleAdminCreateMerchant(c *gin.Context) {
	var req createMerchantRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	if !common.ValidPassword(req.Password) || !merchantUsernamePattern.MatchString(req.Username) {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	now := time.Now()
	merchant := model.Merchant{MerchantNo: common.BuildBizNo("M"), MerchantName: req.MerchantName, ContactName: req.ContactName, ContactPhone: req.Phone, ReviewStatus: model.ReviewApproved, ReviewedBy: &actor.UserID, ReviewedAt: &now}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.MerchantAccount{}).Where("username = ?", req.Username).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return common.ErrInvalidArgument
		}
		if err := tx.Create(&merchant).Error; err != nil {
			return err
		}
		account := model.MerchantAccount{MerchantID: merchant.ID, Username: req.Username, PasswordHash: string(hash), Role: model.AccountRoleOwner, Status: model.AccountStatusActive, MustChangePassword: true}
		if err := tx.Create(&account).Error; err != nil {
			return err
		}
		if err := EnsureMerchantDefaultCategories(tx, merchant.ID); err != nil {
			return err
		}
		return tx.Create(&model.MerchantAuditLog{MerchantID: merchant.ID, Action: "CREATE_ACCOUNT", ToStatus: model.ReviewApproved, OperatorType: model.UserTypeAdmin, OperatorID: actor.UserID}).Error
	})
	if err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	common.Success(c, gin.H{"merchant_id": merchant.ID})
}

func (s *Server) handleAdminResetMerchantPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	if !common.ValidPassword(req.Password) {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	s.updateMerchantAccount(c, map[string]interface{}{"password_hash": string(hash), "must_change_password": true}, "RESET_PASSWORD")
}

func (s *Server) handleAdminSetMerchantStatus(c *gin.Context) {
	var req struct {
		Status string `json:"status" binding:"required,oneof=ACTIVE DISABLED"`
	}
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	s.updateMerchantAccount(c, map[string]interface{}{"status": req.Status}, "SET_ACCOUNT_STATUS")
}

func (s *Server) updateMerchantAccount(c *gin.Context, values map[string]interface{}, action string) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		var account model.MerchantAccount
		if err := tx.Where("merchant_id = ? AND role = ?", id, model.AccountRoleOwner).First(&account).Error; err != nil {
			return err
		}
		fromStatus := account.Status
		if err := tx.Model(&account).Updates(values).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AuthSession{}).Where("user_type = ? AND user_id = ?", model.UserTypeMerchant, account.ID).Update("revoked_at", time.Now()).Error; err != nil {
			return err
		}
		toStatus := account.Status
		if status, ok := values["status"].(string); ok {
			toStatus = status
		}
		return tx.Create(&model.MerchantAuditLog{MerchantID: id, Action: action, FromStatus: fromStatus, ToStatus: toStatus, OperatorType: model.UserTypeAdmin, OperatorID: actor.UserID}).Error
	})
	if err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	common.Success(c, gin.H{"success": true})
}
