package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
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
	var account model.MerchantAccount
	var accountInfo interface{}
	if err := s.DB.Where("merchant_id = ? AND role = ?", id, model.AccountRoleOwner).First(&account).Error; err == nil {
		accountInfo = gin.H{"username": account.Username, "status": account.Status}
	} else if err != gorm.ErrRecordNotFound {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{
		"account": accountInfo,
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
