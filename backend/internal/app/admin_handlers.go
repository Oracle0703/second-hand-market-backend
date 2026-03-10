package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
	"second-hand-market-backend/backend/internal/stateflow"
)

func (s *Server) handleAdminMerchantList(c *gin.Context) {
	page, size := parsePage(c)
	query := s.DB.Model(&model.Merchant{})
	if v := c.Query("status"); v != "" {
		query = query.Where("review_status = ?", v)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		query = query.Where("merchant_name LIKE ? OR contact_name LIKE ? OR contact_phone LIKE ?", like, like, like)
	}
	if st := c.Query("start_at"); st != "" {
		query = query.Where("created_at >= ?", st)
	}
	if et := c.Query("end_at"); et != "" {
		query = query.Where("created_at <= ?", et)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	var items []model.Merchant
	if err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	type item struct {
		ID           uint64    `json:"id"`
		MerchantNo   string    `json:"merchant_no"`
		MerchantName string    `json:"merchant_name"`
		ContactName  string    `json:"contact_name"`
		ContactPhone string    `json:"contact_phone"`
		ReviewStatus string    `json:"review_status"`
		CreatedAt    time.Time `json:"created_at"`
	}
	out := make([]item, 0, len(items))
	for _, it := range items {
		out = append(out, item{ID: it.ID, MerchantNo: it.MerchantNo, MerchantName: it.MerchantName, ContactName: it.ContactName, ContactPhone: it.ContactPhone, ReviewStatus: it.ReviewStatus, CreatedAt: it.CreatedAt})
	}
	common.Success(c, common.PageResult[item]{Items: out, Total: total, Page: page, PageSize: size})
}

func (s *Server) handleAdminMerchantDetail(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	var merchant model.Merchant
	if err := s.DB.Where("id = ?", id).First(&merchant).Error; err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	var logs []model.MerchantAuditLog
	if err := s.DB.Where("merchant_id = ?", id).Order("id DESC").Find(&logs).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	type merchantDetail struct {
		ID            uint64     `json:"id"`
		MerchantNo    string     `json:"merchant_no"`
		MerchantName  string     `json:"merchant_name"`
		ContactName   string     `json:"contact_name"`
		ContactPhone  string     `json:"contact_phone"`
		LicenseFileID *uint64    `json:"license_file_id"`
		ReviewStatus  string     `json:"review_status"`
		RejectReason  *string    `json:"reject_reason"`
		ReviewedBy    *uint64    `json:"reviewed_by"`
		ReviewedAt    *time.Time `json:"reviewed_at"`
		CreatedAt     time.Time  `json:"created_at"`
		UpdatedAt     time.Time  `json:"updated_at"`
	}
	type auditLog struct {
		ID           uint64    `json:"id"`
		MerchantID   uint64    `json:"merchant_id"`
		Action       string    `json:"action"`
		FromStatus   string    `json:"from_status"`
		ToStatus     string    `json:"to_status"`
		Reason       *string   `json:"reason"`
		OperatorType string    `json:"operator_type"`
		OperatorID   uint64    `json:"operator_id"`
		CreatedAt    time.Time `json:"created_at"`
	}
	logItems := make([]auditLog, 0, len(logs))
	for _, item := range logs {
		logItems = append(logItems, auditLog{
			ID:           item.ID,
			MerchantID:   item.MerchantID,
			Action:       item.Action,
			FromStatus:   item.FromStatus,
			ToStatus:     item.ToStatus,
			Reason:       item.Reason,
			OperatorType: item.OperatorType,
			OperatorID:   item.OperatorID,
			CreatedAt:    item.CreatedAt,
		})
	}
	common.Success(c, gin.H{
		"merchant_detail": merchantDetail{
			ID:            merchant.ID,
			MerchantNo:    merchant.MerchantNo,
			MerchantName:  merchant.MerchantName,
			ContactName:   merchant.ContactName,
			ContactPhone:  merchant.ContactPhone,
			LicenseFileID: merchant.LicenseFileID,
			ReviewStatus:  merchant.ReviewStatus,
			RejectReason:  merchant.RejectReason,
			ReviewedBy:    merchant.ReviewedBy,
			ReviewedAt:    merchant.ReviewedAt,
			CreatedAt:     merchant.CreatedAt,
			UpdatedAt:     merchant.UpdatedAt,
		},
		"audit_logs": logItems,
	})
}

func (s *Server) handleAdminMerchantApprove(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.MerchantReviewApproveRequest
	if c.Request.ContentLength > 0 {
		if err := bindJSON(c, &req); err != nil {
			common.Fail(c, err)
			return
		}
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var merchant model.Merchant
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&merchant).Error; err != nil {
			return s.dbError(err)
		}
		if !stateflow.CanTransitionMerchant(merchant.ReviewStatus, model.ReviewApproved) {
			return common.ErrInvalidTransition
		}
		fromStatus := merchant.ReviewStatus
		now := time.Now()
		merchant.ReviewStatus = model.ReviewApproved
		merchant.RejectReason = nil
		merchant.ReviewedAt = &now
		merchant.ReviewedBy = &actor.UserID
		if err := tx.Save(&merchant).Error; err != nil {
			return err
		}
		a := model.MerchantAuditLog{MerchantID: merchant.ID, Action: "APPROVE", FromStatus: fromStatus, ToStatus: model.ReviewApproved, OperatorType: model.UserTypeAdmin, OperatorID: actor.UserID, Reason: req.Comment}
		if err := tx.Create(&a).Error; err != nil {
			return err
		}
		from, to := fromStatus, model.ReviewApproved
		s.writeOperationLog(c, tx, "merchant", merchant.ID, "merchant_approve", &from, &to, common.CodeOK, &merchant.ID, nil)
		return nil
	}); err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, gin.H{"merchant_id": merchant.ID, "review_status": merchant.ReviewStatus, "reviewed_at": merchant.ReviewedAt, "reviewed_by": merchant.ReviewedBy})
}

func (s *Server) handleAdminMerchantReject(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.MerchantReviewRejectRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var merchant model.Merchant
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&merchant).Error; err != nil {
			return s.dbError(err)
		}
		if !stateflow.CanTransitionMerchant(merchant.ReviewStatus, model.ReviewRejected) {
			return common.ErrInvalidTransition
		}
		fromStatus := merchant.ReviewStatus
		now := time.Now()
		merchant.ReviewStatus = model.ReviewRejected
		merchant.RejectReason = &req.Reason
		merchant.ReviewedAt = &now
		merchant.ReviewedBy = &actor.UserID
		if err := tx.Save(&merchant).Error; err != nil {
			return err
		}
		a := model.MerchantAuditLog{MerchantID: merchant.ID, Action: "REJECT", FromStatus: fromStatus, ToStatus: model.ReviewRejected, OperatorType: model.UserTypeAdmin, OperatorID: actor.UserID, Reason: &req.Reason}
		if err := tx.Create(&a).Error; err != nil {
			return err
		}
		from, to := fromStatus, model.ReviewRejected
		s.writeOperationLog(c, tx, "merchant", merchant.ID, "merchant_reject", &from, &to, common.CodeOK, &merchant.ID, nil)
		return nil
	}); err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, gin.H{"merchant_id": merchant.ID, "review_status": merchant.ReviewStatus, "reviewed_at": merchant.ReviewedAt, "reviewed_by": merchant.ReviewedBy, "reject_reason": merchant.RejectReason})
}

func (s *Server) handleAdminLogs(c *gin.Context) {
	page, size := parsePage(c)
	query := s.DB.Model(&model.OperationLog{})
	if v := c.Query("operator_type"); v != "" {
		query = query.Where("operator_type = ?", v)
	}
	if v := c.Query("action"); v != "" {
		query = query.Where("action = ?", v)
	}
	if v := c.Query("resource_type"); v != "" {
		query = query.Where("resource_type = ?", v)
	}
	if st := c.Query("start_at"); st != "" {
		query = query.Where("created_at >= ?", st)
	}
	if et := c.Query("end_at"); et != "" {
		query = query.Where("created_at <= ?", et)
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
	common.Success(c, common.PageResult[model.OperationLog]{Items: items, Total: total, Page: page, PageSize: size})
}
