package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func (s *Server) handleAdminFileContent(c *gin.Context) {
	fileID, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(s.cfg.FileStorageProvider), "local") {
		common.Fail(c, common.ErrInternal)
		return
	}

	var fileRecord model.FileRecord
	if err := s.DB.Where("id = ?", fileID).First(&fileRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.Fail(c, common.ErrNotFound)
		} else {
			common.Fail(c, common.ErrInternal)
		}
		return
	}
	objectKey, objectKeyErr := normalizeObjectKey(fileRecord.ObjectKey)
	mimeType := strings.ToLower(strings.TrimSpace(fileRecord.MimeType))
	if fileRecord.BizType != model.FileBizMerchantLicense ||
		fileRecord.ScanStatus != model.FileScanPass ||
		objectKeyErr != nil ||
		!strings.HasPrefix(objectKey, "merchant_license/") ||
		fileRecord.MimeType != mimeType ||
		!allowedMIMEs[mimeType] ||
		fileRecord.OwnerMerchantID == nil {
		common.Fail(c, common.ErrNotFound)
		return
	}

	var merchant model.Merchant
	if err := s.DB.Where("id = ? AND license_file_id = ?", *fileRecord.OwnerMerchantID, fileRecord.ID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.Fail(c, common.ErrNotFound)
		} else {
			common.Fail(c, common.ErrInternal)
		}
		return
	}

	openedFile, stat, err := s.openLocalRegularFile(objectKey)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.Fail(c, common.ErrNotFound)
		} else {
			common.Fail(c, common.ErrInternal)
		}
		return
	}
	defer openedFile.Close()

	logItem := s.buildOperationLog(
		c,
		"file",
		fileRecord.ID,
		"admin_file_read",
		nil,
		nil,
		common.CodeOK,
		fileRecord.OwnerMerchantID,
		map[string]interface{}{
			"biz_type":    model.FileBizMerchantLicense,
			"scan_status": model.FileScanPass,
		},
	)
	if err := s.insertOperationLog(nil, &logItem); err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}

	c.Header("Content-Disposition", "inline")
	c.Header("Cache-Control", "private, no-store")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, stat.Size(), mimeType, openedFile, nil)
}
