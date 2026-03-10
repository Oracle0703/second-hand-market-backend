package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
	"second-hand-market-backend/backend/internal/stateflow"
)

func (s *Server) handleMerchantProfile(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var merchant model.Merchant
	if err := s.DB.Where("id = ?", actor.MerchantID).First(&merchant).Error; err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	common.Success(c, gin.H{
		"merchant_info": gin.H{"id": merchant.ID, "name": merchant.MerchantName, "contact": merchant.ContactName, "phone": merchant.ContactPhone},
		"review_status": merchant.ReviewStatus,
		"reject_reason": merchant.RejectReason,
	})
}

func (s *Server) handleMerchantReapply(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.ReapplyRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		var merchant model.Merchant
		if err := tx.Where("id = ?", actor.MerchantID).First(&merchant).Error; err != nil {
			return s.dbError(err)
		}
		if !stateflow.CanTransitionMerchant(merchant.ReviewStatus, model.ReviewPending) {
			return common.ErrInvalidTransition
		}
		if req.MerchantName != nil {
			merchant.MerchantName = *req.MerchantName
		}
		if req.ContactName != nil {
			merchant.ContactName = *req.ContactName
		}
		if req.Phone != nil {
			merchant.ContactPhone = *req.Phone
		}
		if req.LicenseFileID != nil {
			merchant.LicenseFileID = req.LicenseFileID
		}
		fromStatus := merchant.ReviewStatus
		merchant.ReviewStatus = model.ReviewPending
		merchant.RejectReason = nil
		merchant.ReviewedBy = nil
		merchant.ReviewedAt = nil
		if err := tx.Save(&merchant).Error; err != nil {
			return err
		}
		audit := model.MerchantAuditLog{MerchantID: merchant.ID, Action: "REAPPLY", FromStatus: fromStatus, ToStatus: model.ReviewPending, OperatorType: model.UserTypeMerchant, OperatorID: actor.UserID}
		if err := tx.Create(&audit).Error; err != nil {
			return err
		}
		from, to := fromStatus, model.ReviewPending
		s.writeOperationLog(c, tx, "merchant", merchant.ID, "merchant_reapply", &from, &to, common.CodeOK, &merchant.ID, nil)
		return nil
	}); err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, gin.H{"review_status": model.ReviewPending})
}

func (s *Server) handleMerchantAccount(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var acct model.MerchantAccount
	if err := s.DB.Where("id = ?", actor.UserID).First(&acct).Error; err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	var pwdUpdatedAt *time.Time
	common.Success(c, gin.H{
		"account":  gin.H{"id": acct.ID, "username": acct.Username, "role": acct.Role, "status": acct.Status, "last_login_at": acct.LastLoginAt},
		"security": gin.H{"password_updated_at": pwdUpdatedAt, "mfa_enabled": false},
	})
}

func (s *Server) handleMerchantChangePassword(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.UpdatePasswordRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	var acct model.MerchantAccount
	if err := s.DB.Where("id = ?", actor.UserID).First(&acct).Error; err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acct.PasswordHash), []byte(req.OldPassword)); err != nil {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	if len(req.NewPassword) < 8 {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	now := time.Now()
	if err := s.DB.Model(&model.MerchantAccount{}).Where("id = ?", acct.ID).Update("password_hash", string(hash)).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"success": true, "password_updated_at": now.Format(time.RFC3339)})
}

func (s *Server) handleCategories(c *gin.Context) {
	query := s.DB.Model(&model.Category{}).Where("status = ?", model.CategoryEnabled)
	if v := c.Query("level"); v != "" {
		query = query.Where("level = ?", v)
	}
	if v := c.Query("parent_id"); v != "" {
		query = query.Where("parent_id = ?", v)
	}
	var items []model.Category
	if err := query.Order("sort ASC, id ASC").Find(&items).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"items": items})
}

func (s *Server) handleDashboard(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	countByStatus := func(table, statusField string, statuses []string) map[string]int64 {
		res := map[string]int64{}
		for _, st := range statuses {
			var n int64
			_ = s.DB.Table(table).Where("merchant_id = ? AND "+statusField+" = ?", actor.MerchantID, st).Count(&n).Error
			res[strings.ToLower(st)] = n
		}
		return res
	}
	common.Success(c, gin.H{
		"product_stats": countByStatus("products", "status", []string{model.ProductDraft, model.ProductOnShelf, model.ProductLocked, model.ProductOffShelf, model.ProductSold, model.ProductClosed}),
		"order_stats":   countByStatus("orders", "status", []string{model.OrderCreated, model.OrderCompleted, model.OrderClosed}),
	})
}

func (s *Server) handleMerchantLogs(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Model(&model.OperationLog{}).Where("merchant_id = ?", actor.MerchantID)
	if v := c.Query("action"); v != "" {
		query = query.Where("action = ?", v)
	}
	if v := c.Query("resource_type"); v != "" {
		query = query.Where("resource_type = ?", v)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	var items []model.OperationLog
	if err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	type item struct {
		ID           uint64    `json:"id"`
		Action       string    `json:"action"`
		ResourceType string    `json:"resource_type"`
		ResourceID   uint64    `json:"resource_id"`
		FromStatus   *string   `json:"from_status"`
		ToStatus     *string   `json:"to_status"`
		ResultCode   int       `json:"result_code"`
		CreatedAt    time.Time `json:"created_at"`
		RequestID    string    `json:"request_id"`
	}
	out := make([]item, 0, len(items))
	for _, it := range items {
		out = append(out, item{
			ID:           it.ID,
			Action:       it.Action,
			ResourceType: it.ResourceType,
			ResourceID:   it.ResourceID,
			FromStatus:   it.FromStatus,
			ToStatus:     it.ToStatus,
			ResultCode:   it.ResultCode,
			CreatedAt:    it.CreatedAt,
			RequestID:    it.RequestID,
		})
	}
	common.Success(c, common.PageResult[item]{Items: out, Total: total, Page: page, PageSize: size})
}
