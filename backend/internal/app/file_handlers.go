package app

import (
	"fmt"
	"io"
	"net/http"
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
)

const maxUploadSizeBytes int64 = media.DefaultMaxOriginalBytes
const maxMultipartOverheadBytes int64 = 1 * 1024 * 1024

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
	if !allowedMIMEs[mimeType] || req.FileSize <= 0 || req.FileSize > s.uploadSizeLimit() {
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
	ext := media.MIMEExt(mimeType)
	if ext == "" {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	objectKey := fmt.Sprintf("%s/%s%s", strings.ToLower(bizType), common.BuildBizNo("F"), ext)
	recordMIME := mimeType
	if bizType == model.FileBizProductImage {
		objectKey = fmt.Sprintf("product_image/detail-v1/%s.jpg", common.BuildBizNo("F"))
		recordMIME = "image/jpeg"
	}
	file := model.FileRecord{
		BizType:      bizType,
		ObjectKey:    objectKey,
		URL:          "",
		MimeType:     recordMIME,
		SizeBytes:    req.FileSize,
		UploaderType: uploaderType,
		UploaderID:   uploaderID,
		ScanStatus:   model.FileScanPending,
	}
	if err := s.DB.Create(&file).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{
		"upload_url":       "/api/v1/files/upload",
		"upload_method":    "multipart/form-data",
		"storage_provider": strings.ToLower(strings.TrimSpace(s.cfg.FileStorageProvider)),
		"object_key":       objectKey,
		"file_id":          file.ID,
		"expire_at":        time.Now().Add(15 * time.Minute).Format(time.RFC3339),
	})
}

func (s *Server) handleUploadFile(c *gin.Context) {
	bodyLimit := s.uploadSizeLimit() + maxMultipartOverheadBytes
	if c.Request.ContentLength > bodyLimit {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		bodyLimit,
	)

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

	file, err := s.loadFileRecordAndAuthorize(c, fileID)
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
	processRequest := media.ProcessRequest{
		FileName:  formFile.Filename,
		InputMIME: file.MimeType,
		Content:   content,
	}
	if file.BizType == model.FileBizProductImage {
		processRequest.OutputProfile = media.DetailProfileVersion
	}
	processed, err := processor.Process(c.Request.Context(), processRequest)
	if err != nil {
		common.Fail(c, err)
		return
	}
	outputMIME := strings.ToLower(strings.TrimSpace(processed.OutputMIME))
	outputExt := media.MIMEExt(outputMIME)
	finalObjectKey := file.ObjectKey
	if file.BizType == model.FileBizProductImage {
		if len(processed.Content) == 0 ||
			outputMIME != "image/jpeg" ||
			strings.ToLower(strings.TrimSpace(processed.OutputExt)) != ".jpg" ||
			!media.IsDetailProductImageKey(file.ObjectKey) ||
			media.DetectImageMIME(processed.Content) != "image/jpeg" {
			common.Fail(c, common.ErrInvalidUpload)
			return
		}
		if _, _, err := media.ValidateDetailJPEG(media.DefaultDetailImagePolicy(), processed.Content); err != nil {
			common.Fail(c, err)
			return
		}
	} else {
		expectedOutputMIME := media.CanonicalImageMIME(file.MimeType)
		if len(processed.Content) == 0 ||
			int64(len(processed.Content)) > limit ||
			outputExt == "" ||
			outputMIME != expectedOutputMIME ||
			strings.ToLower(strings.TrimSpace(processed.OutputExt)) != outputExt ||
			media.DetectImageMIME(processed.Content) != outputMIME {
			common.Fail(c, common.ErrInvalidUpload)
			return
		}
		finalObjectKey, err = replaceObjectKeyExtension(file.ObjectKey, outputExt)
		if err != nil {
			common.Fail(c, common.ErrInvalidUpload)
			return
		}
	}
	dstPath, err := s.localUploadPath(finalObjectKey)
	if err != nil {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	if err := media.PublishObjectNoReplace(dstPath, processed.Content, 0o600); err != nil {
		common.Fail(c, err)
		return
	}

	url := s.publicFileURL(finalObjectKey)
	updates := map[string]interface{}{
		"object_key":  finalObjectKey,
		"url":         url,
		"scan_status": model.FileScanPass,
		"mime_type":   outputMIME,
		"size_bytes":  int64(len(processed.Content)),
	}
	if err := s.DB.Model(&model.FileRecord{}).Where("id = ?", file.ID).Updates(updates).Error; err != nil {
		_ = os.Remove(dstPath)
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"file_id": file.ID, "url": url, "object_key": finalObjectKey, "status": model.FileScanPass})
}

func (s *Server) handleConfirmUpload(c *gin.Context) {
	var req dto.ConfirmUploadRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	file, err := s.loadFileRecordAndAuthorize(c, req.FileID)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if file.ObjectKey != req.ObjectKey {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	if !strings.EqualFold(s.cfg.FileStorageProvider, "local") || file.ScanStatus != model.FileScanPass {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	if media.MIMEForExt(filepath.Ext(file.ObjectKey)) != strings.ToLower(strings.TrimSpace(file.MimeType)) {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	path, pathErr := s.localUploadPath(file.ObjectKey)
	if pathErr != nil {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	if stat, statErr := os.Stat(path); statErr != nil || stat.IsDir() {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	url := s.publicFileURL(file.ObjectKey)
	common.Success(c, gin.H{"file_id": file.ID, "url": url, "status": model.FileScanPass})
}

func (s *Server) loadFileRecordAndAuthorize(c *gin.Context, fileID uint64) (*model.FileRecord, error) {
	var file model.FileRecord
	if err := s.DB.Where("id = ?", fileID).First(&file).Error; err != nil {
		return nil, common.ErrNotFound
	}
	actor, ok := common.GetActor(c)
	if !ok {
		if file.UploaderType != model.UserTypePublic {
			return nil, common.ErrForbidden
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
	return media.LocalObjectPath(s.cfg.FileUploadLocalDir, objectKey)
}

func replaceObjectKeyExtension(objectKey, ext string) (string, error) {
	if media.MIMEForExt(ext) == "" {
		return "", common.ErrInvalidUpload
	}
	currentExt := filepath.Ext(objectKey)
	base := strings.TrimSuffix(objectKey, currentExt)
	if currentExt == "" || base == "" {
		return "", common.ErrInvalidUpload
	}
	return base + strings.ToLower(ext), nil
}

func (s *Server) handlePublicUpload(c *gin.Context) {
	objectKey := strings.TrimPrefix(c.Param("object_key"), "/")
	mimeType := media.MIMEForExt(filepath.Ext(objectKey))
	if mimeType == "" {
		c.Status(http.StatusNotFound)
		return
	}
	path, err := s.localUploadPath(objectKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		c.Status(http.StatusNotFound)
		return
	}
	header := make([]byte, 3072)
	n, readErr := file.Read(header)
	if readErr != nil && readErr != io.EOF {
		c.Status(http.StatusNotFound)
		return
	}
	if media.DetectImageMIME(header[:n]) != mimeType {
		c.Status(http.StatusNotFound)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	c.Header("Content-Type", mimeType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	if media.IsDetailProductImageKey(objectKey) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(c.Writer, c.Request, filepath.Base(objectKey), stat.ModTime(), file)
}

func (s *Server) publicFileURL(objectKey string) string {
	cleanKey := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+objectKey)), "/")
	return "/uploads/" + cleanKey
}

func (s *Server) publicFileRecordURL(file model.FileRecord) string {
	if strings.TrimSpace(file.ObjectKey) == "" {
		return ""
	}
	return s.publicFileURL(file.ObjectKey)
}

func (s *Server) uploadSizeLimit() int64 {
	if s.cfg.FileUploadMaxBytes > 0 {
		return s.cfg.FileUploadMaxBytes
	}
	return maxUploadSizeBytes
}
