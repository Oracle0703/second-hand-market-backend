package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
)

var allowedMIMEs = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

func allowUploadBiz(actor *common.Actor, bizType string) error {
	if actor == nil {
		if bizType == model.FileBizMerchantLicense {
			return nil
		}
		return common.ErrForbidden
	}
	switch actor.UserType {
	case model.UserTypeMerchant:
		if actor.Scope == "onboarding" && bizType != model.FileBizMerchantLicense {
			return common.ErrReviewNotApproved
		}
		if bizType == model.FileBizMerchantLicense || bizType == model.FileBizProductImage {
			return nil
		}
		return common.ErrForbidden
	case model.UserTypeAdmin:
		return nil
	default:
		return common.ErrForbidden
	}
}

func (s *Server) handlePresign(c *gin.Context) {
	var req dto.PresignRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	bizType := strings.ToUpper(req.BizType)
	if !allowedMIMEs[strings.ToLower(req.MIMEType)] || req.FileSize > 5*1024*1024 {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	actor, ok := common.GetActor(c)
	uploaderType := model.UserTypePublic
	var uploaderID *uint64
	var actorPtr *common.Actor
	if ok {
		actorPtr = &actor
		uploaderType = actor.UserType
		uploaderID = &actor.UserID
	}
	if err := allowUploadBiz(actorPtr, bizType); err != nil {
		common.Fail(c, err)
		return
	}
	ext := filepath.Ext(req.FileName)
	objectKey := fmt.Sprintf("%s/%s%s", strings.ToLower(bizType), common.BuildBizNo("F"), ext)
	file := model.FileRecord{
		BizType:      bizType,
		ObjectKey:    objectKey,
		URL:          "",
		MimeType:     strings.ToLower(req.MIMEType),
		SizeBytes:    req.FileSize,
		UploaderType: uploaderType,
		UploaderID:   uploaderID,
		ScanStatus:   model.FileScanPending,
	}
	if err := s.DB.Create(&file).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"upload_url": "https://upload.local/" + objectKey, "object_key": objectKey, "file_id": file.ID, "expire_at": time.Now().Add(15 * time.Minute).Format(time.RFC3339)})
}

func (s *Server) handleConfirmUpload(c *gin.Context) {
	var req dto.ConfirmUploadRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	var file model.FileRecord
	if err := s.DB.Where("id = ?", req.FileID).First(&file).Error; err != nil {
		common.Fail(c, common.ErrNotFound)
		return
	}
	actor, ok := common.GetActor(c)
	if !ok {
		if file.UploaderType != model.UserTypePublic {
			common.Fail(c, common.ErrForbidden)
			return
		}
	} else {
		if err := allowUploadBiz(&actor, file.BizType); err != nil {
			common.Fail(c, err)
			return
		}
		if actor.UserType != model.UserTypeAdmin {
			if file.UploaderType != actor.UserType || file.UploaderID == nil || *file.UploaderID != actor.UserID {
				common.Fail(c, common.ErrForbidden)
				return
			}
		}
	}
	if file.ObjectKey != req.ObjectKey {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	url := "https://cdn.local/" + req.ObjectKey
	if err := s.DB.Model(&model.FileRecord{}).Where("id = ?", file.ID).Updates(map[string]interface{}{"url": url, "scan_status": model.FileScanPass}).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"file_id": file.ID, "url": url, "status": model.FileScanPass})
}
