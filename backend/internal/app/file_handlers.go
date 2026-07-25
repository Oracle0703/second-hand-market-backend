package app

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/media"
	"second-hand-market-backend/backend/internal/model"

	"gorm.io/gorm"
)

const maxUploadSizeBytes int64 = media.DefaultMaxOriginalBytes

var allowedMIMEs = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/heic": true,
	"image/heif": true,
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
	mimeType := strings.ToLower(strings.TrimSpace(req.MIMEType))
	if !allowedMIMEs[mimeType] || req.FileSize > s.uploadSizeLimit() {
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
	now := time.Now()
	expiresAt := now.Add(fileCapabilityTTL)
	var ownerMerchantID *uint64
	var capabilityTokenHash *string
	var capabilityExpiresAt *time.Time
	fileToken := ""
	if actorPtr == nil {
		var tokenHash string
		var err error
		fileToken, tokenHash, expiresAt, err = newFileCapability(now)
		if err != nil {
			common.Fail(c, err)
			return
		}
		capabilityTokenHash = &tokenHash
		capabilityExpiresAt = &expiresAt
	} else if actor.UserType == model.UserTypeMerchant {
		ownerMerchantID = &actor.MerchantID
	}
	ext := strings.ToLower(filepath.Ext(req.FileName))
	if ext == "" {
		ext = mimeExt(mimeType)
	}
	if ext == "" {
		ext = ".bin"
	}
	objectKey := fmt.Sprintf("%s/%s%s", strings.ToLower(bizType), common.BuildBizNo("F"), ext)
	file := model.FileRecord{
		BizType:             bizType,
		ObjectKey:           objectKey,
		URL:                 "",
		MimeType:            mimeType,
		SizeBytes:           req.FileSize,
		UploaderType:        uploaderType,
		UploaderID:          uploaderID,
		ScanStatus:          model.FileScanPending,
		OwnerMerchantID:     ownerMerchantID,
		CapabilityTokenHash: capabilityTokenHash,
		CapabilityExpiresAt: capabilityExpiresAt,
	}
	if err := s.DB.Create(&file).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	response := gin.H{
		"upload_url":       "/api/v1/files/upload",
		"upload_method":    "multipart/form-data",
		"storage_provider": strings.ToLower(strings.TrimSpace(s.cfg.FileStorageProvider)),
		"object_key":       objectKey,
		"file_id":          file.ID,
		"expire_at":        expiresAt.Format(time.RFC3339),
	}
	if fileToken != "" {
		response["file_token"] = fileToken
	}
	common.Success(c, response)
}

func (s *Server) handleUploadFile(c *gin.Context) {
	fileID, err := strconv.ParseUint(strings.TrimSpace(c.PostForm("file_id")), 10, 64)
	if err != nil || fileID == 0 {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	objectKey := strings.TrimSpace(c.PostForm("object_key"))
	if objectKey == "" {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	formFile, err := c.FormFile("file")
	limit := s.uploadSizeLimit()
	if err != nil || formFile == nil || formFile.Size <= 0 || formFile.Size > limit {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}

	file, err := s.loadFileRecordAndAuthorize(c, fileID, c.PostForm("file_token"))
	if err != nil {
		common.Fail(c, err)
		return
	}
	if file.ObjectKey != objectKey {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}

	src, err := formFile.Open()
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	defer src.Close()

	content, err := io.ReadAll(io.LimitReader(src, limit+1))
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	if int64(len(content)) == 0 || int64(len(content)) > limit {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	processor := s.imageProcessor
	if processor == nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	processed, err := processor.Process(c.Request.Context(), media.ProcessRequest{
		FileName:  formFile.Filename,
		InputMIME: file.MimeType,
		Content:   content,
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	if len(processed.Content) == 0 || !media.IsAllowedImageMIME(processed.OutputMIME) {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}

	dstPath, err := s.localUploadPath(file.ObjectKey)
	if err != nil {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	tmpPath := dstPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	writeAndClose := func() error {
		defer out.Close()
		if _, err := out.Write(processed.Content); err != nil {
			return err
		}
		return nil
	}
	if err := writeAndClose(); err != nil {
		_ = os.Remove(tmpPath)
		if err == common.ErrInvalidUpload {
			common.Fail(c, err)
			return
		}
		common.Fail(c, common.ErrInternal)
		return
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		common.Fail(c, common.ErrInternal)
		return
	}

	url := s.publicFileURL(file.ObjectKey)
	updates := map[string]interface{}{
		"url":         url,
		"scan_status": model.FileScanPass,
		"mime_type":   processed.OutputMIME,
		"size_bytes":  int64(len(processed.Content)),
	}
	if err := s.DB.Model(&model.FileRecord{}).Where("id = ?", file.ID).Updates(updates).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"file_id": file.ID, "url": url, "object_key": file.ObjectKey, "status": model.FileScanPass})
}

func (s *Server) handleConfirmUpload(c *gin.Context) {
	var req dto.ConfirmUploadRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	file, err := s.loadFileRecordAndAuthorize(c, req.FileID, req.FileToken)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if file.ObjectKey != req.ObjectKey {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	if strings.EqualFold(s.cfg.FileStorageProvider, "local") {
		path, pathErr := s.localUploadPath(file.ObjectKey)
		if pathErr != nil {
			common.Fail(c, common.ErrInvalidUpload)
			return
		}
		if stat, statErr := os.Stat(path); statErr != nil || stat.IsDir() {
			common.Fail(c, common.ErrInvalidUpload)
			return
		}
	}
	url := s.publicFileURL(req.ObjectKey)
	if err := s.DB.Model(&model.FileRecord{}).Where("id = ?", file.ID).Updates(map[string]interface{}{"url": url, "scan_status": model.FileScanPass}).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"file_id": file.ID, "url": url, "status": model.FileScanPass})
}

func (s *Server) loadFileRecordAndAuthorize(c *gin.Context, fileID uint64, rawToken string) (*model.FileRecord, error) {
	actor, hasActor := common.GetActor(c)
	var file model.FileRecord
	if err := s.DB.Where("id = ?", fileID).First(&file).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if !hasActor {
				return nil, common.ErrInvalidFileBinding
			}
			return nil, common.ErrNotFound
		}
		return nil, err
	}
	if !hasActor {
		if file.UploaderType != model.UserTypePublic {
			return nil, common.ErrInvalidFileBinding
		}
		if strings.TrimSpace(rawToken) == "" ||
			file.CapabilityTokenHash == nil ||
			file.CapabilityExpiresAt == nil ||
			!file.CapabilityExpiresAt.After(time.Now()) {
			return nil, common.ErrInvalidFileBinding
		}
		computedHash := fileCapabilityHash(rawToken)
		if subtle.ConstantTimeCompare([]byte(*file.CapabilityTokenHash), []byte(computedHash)) != 1 {
			return nil, common.ErrInvalidFileBinding
		}
	} else {
		if err := allowUploadBiz(&actor, file.BizType); err != nil {
			return nil, err
		}
		if actor.UserType != model.UserTypeAdmin {
			if file.UploaderType != actor.UserType || file.UploaderID == nil || *file.UploaderID != actor.UserID {
				return nil, common.ErrForbidden
			}
		}
	}
	return &file, nil
}

func (s *Server) localUploadPath(objectKey string) (string, error) {
	root, err := filepath.Abs(s.cfg.FileUploadLocalDir)
	if err != nil {
		return "", err
	}
	cleanKey := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+objectKey)), "/")
	target := filepath.Join(root, filepath.FromSlash(cleanKey))
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", common.ErrInvalidUpload
	}
	return target, nil
}

func (s *Server) publicFileURL(objectKey string) string {
	base := strings.TrimSpace(s.cfg.FilePublicBaseURL)
	cleanKey := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+objectKey)), "/")
	if base != "" {
		return strings.TrimRight(base, "/") + "/" + cleanKey
	}
	return "/uploads/" + cleanKey
}

func mimeExt(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/heic":
		return ".heic"
	case "image/heif":
		return ".heif"
	default:
		return ""
	}
}

func (s *Server) uploadSizeLimit() int64 {
	if s.cfg.FileUploadMaxBytes > 0 {
		return s.cfg.FileUploadMaxBytes
	}
	return maxUploadSizeBytes
}
