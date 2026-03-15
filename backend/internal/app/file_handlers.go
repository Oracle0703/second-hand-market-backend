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
	"second-hand-market-backend/backend/internal/model"
)

const maxUploadSizeBytes int64 = 5 * 1024 * 1024

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
	if !allowedMIMEs[mimeType] || req.FileSize > maxUploadSizeBytes {
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
	ext := strings.ToLower(filepath.Ext(req.FileName))
	if ext == "" {
		ext = mimeExt(mimeType)
	}
	if ext == "" {
		ext = ".bin"
	}
	objectKey := fmt.Sprintf("%s/%s%s", strings.ToLower(bizType), common.BuildBizNo("F"), ext)
	file := model.FileRecord{
		BizType:      bizType,
		ObjectKey:    objectKey,
		URL:          "",
		MimeType:     mimeType,
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
	if err != nil || formFile == nil || formFile.Size <= 0 || formFile.Size > maxUploadSizeBytes {
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

	head := make([]byte, 512)
	n, readErr := io.ReadFull(src, head)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		common.Fail(c, common.ErrInvalidUpload)
		return
	}
	detected := strings.ToLower(http.DetectContentType(head[:n]))
	if !allowedMIMEs[detected] {
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
	var written int64
	writeAndClose := func() error {
		defer out.Close()
		if n > 0 {
			m, werr := out.Write(head[:n])
			if werr != nil {
				return werr
			}
			written += int64(m)
		}
		limit := maxUploadSizeBytes - written + 1
		if limit <= 0 {
			return common.ErrInvalidUpload
		}
		copied, cerr := io.Copy(out, io.LimitReader(src, limit))
		if cerr != nil {
			return cerr
		}
		written += copied
		if written > maxUploadSizeBytes {
			return common.ErrInvalidUpload
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
		"mime_type":   detected,
		"size_bytes":  written,
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
	file, err := s.loadFileRecordAndAuthorize(c, req.FileID)
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
