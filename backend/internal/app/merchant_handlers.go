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
		if req.LicenseFileID != nil {
			if err := validateMerchantFilesForBinding(
				tx,
				actor.MerchantID,
				[]uint64{*req.LicenseFileID},
				model.FileBizMerchantLicense,
			); err != nil {
				return err
			}
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
	allowedRootNames := make([]string, 0, len(defaultCategorySeeds))
	for _, seed := range defaultCategorySeeds {
		name := strings.TrimSpace(seed.Name)
		if name != "" {
			allowedRootNames = append(allowedRootNames, name)
		}
	}

	query := s.DB.Model(&model.Category{}).Where("categories.status = ?", model.CategoryEnabled)
	level := strings.TrimSpace(c.Query("level"))
	parentID := strings.TrimSpace(c.Query("parent_id"))
	if level != "" {
		if level == "1" {
			query = query.Where("categories.name IN ?", allowedRootNames)
		}
		if level == "2" && parentID == "" {
			query = query.Joins("JOIN categories AS p ON p.id = categories.parent_id").Where("p.name IN ?", allowedRootNames)
		}
		query = query.Where("categories.level = ?", level)
	}
	if parentID != "" {
		query = query.Where("categories.parent_id = ?", parentID)
	}
	var items []model.Category
	if err := query.Order("categories.sort ASC, categories.id ASC").Find(&items).Error; err != nil {
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
			_ = s.DB.Table(table).Where("merchant_id = ? AND "+statusField+" = ? AND deleted_at IS NULL", actor.MerchantID, st).Count(&n).Error
			res[strings.ToLower(st)] = n
		}
		return res
	}

	var onShelfTotalAmountCent int64
	if err := s.DB.Table("products").
		Select("COALESCE(SUM(price_cent * (stock - reserved_stock)), 0)").
		Where("merchant_id = ? AND status = ? AND deleted_at IS NULL", actor.MerchantID, model.ProductOnShelf).
		Scan(&onShelfTotalAmountCent).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}

	common.Success(c, gin.H{
		"product_stats":              countByStatus("products", "status", []string{model.ProductDraft, model.ProductOnShelf, model.ProductLocked, model.ProductOffShelf, model.ProductSold, model.ProductClosed}),
		"order_stats":                countByStatus("orders", "status", []string{model.OrderCreated, model.OrderCompleted, model.OrderClosed}),
		"on_shelf_total_amount_cent": onShelfTotalAmountCent,
	})
}

func (s *Server) handleMerchantLogs(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Table("operation_logs AS l").
		Joins("LEFT JOIN orders AS o ON l.resource_type = 'order' AND o.id = l.resource_id").
		Joins("LEFT JOIN buyer_intents AS i ON l.resource_type = 'intent' AND i.id = l.resource_id").
		Joins("LEFT JOIN products AS p ON (l.resource_type = 'product' AND p.id = l.resource_id) OR (l.resource_type = 'order' AND p.id = o.product_id) OR (l.resource_type = 'intent' AND p.id = i.product_id)").
		Joins("LEFT JOIN categories AS c2 ON c2.id = p.category_id").
		Joins("LEFT JOIN categories AS c1 ON c1.id = c2.parent_id").
		Where("l.merchant_id = ?", actor.MerchantID)
	if v := strings.TrimSpace(c.Query("action")); v != "" {
		query = query.Where("l.action = ?", v)
	}
	if v := strings.TrimSpace(c.Query("resource_type")); v != "" {
		query = query.Where("l.resource_type = ?", v)
	}
	if lv1 := strings.TrimSpace(c.Query("category_level1_id")); lv1 != "" {
		query = query.Where("c1.id = ?", lv1)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	type item struct {
		ID                 uint64    `json:"id"`
		Action             string    `json:"action"`
		ResourceType       string    `json:"resource_type"`
		ResourceID         uint64    `json:"resource_id"`
		FromStatus         *string   `json:"from_status"`
		ToStatus           *string   `json:"to_status"`
		ResultCode         int       `json:"result_code"`
		CreatedAt          time.Time `json:"created_at"`
		RequestID          string    `json:"request_id"`
		CategoryLevel1ID   *uint64   `json:"category_level1_id"`
		CategoryLevel1Name *string   `json:"category_level1_name"`
		CategoryLevel2ID   *uint64   `json:"category_level2_id"`
		CategoryLevel2Name *string   `json:"category_level2_name"`
	}
	out := make([]item, 0, size)
	if err := query.Select(
		"l.id, l.action, l.resource_type, l.resource_id, l.from_status, l.to_status, l.result_code, l.created_at, l.request_id, c1.id AS category_level1_id, c1.name AS category_level1_name, c2.id AS category_level2_id, c2.name AS category_level2_name",
	).Order("l.id DESC").Offset((page - 1) * size).Limit(size).Find(&out).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, common.PageResult[item]{Items: out, Total: total, Page: page, PageSize: size})
}
