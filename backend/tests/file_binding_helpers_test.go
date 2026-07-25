package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func minimalJPEG() []byte {
	return []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F',
		0x00, 0x01, 0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00,
		0xFF, 0xD9,
	}
}

func createReadyOwnedFile(
	t *testing.T,
	srv *app.Server,
	ownerMerchantID uint64,
	bizType string,
) model.FileRecord {
	t.Helper()
	file := model.FileRecord{
		BizType:         bizType,
		ObjectKey:       fmt.Sprintf("test/%d-%d.jpg", ownerMerchantID, time.Now().UnixNano()),
		URL:             "/uploads/test-owned.jpg",
		MimeType:        "image/jpeg",
		SizeBytes:       22,
		UploaderType:    model.UserTypeMerchant,
		ScanStatus:      model.FileScanPass,
		OwnerMerchantID: &ownerMerchantID,
	}
	if err := srv.DB.Create(&file).Error; err != nil {
		t.Fatalf("create owned file: %v", err)
	}
	return file
}

func createReadyOwnedFileForToken(
	t *testing.T,
	srv *app.Server,
	merchantToken string,
	bizType string,
) model.FileRecord {
	t.Helper()
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  bizType,
		"file_name": "owned.jpg",
		"file_size": 22,
		"mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if presign.Code != common.CodeOK {
		t.Fatalf("presign owned file: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Updates(map[string]interface{}{
		"url":         "/uploads/test-owned.jpg",
		"scan_status": model.FileScanPass,
	}).Error; err != nil {
		t.Fatalf("complete owned file: %v", err)
	}
	var file model.FileRecord
	if err := srv.DB.First(&file, fileID).Error; err != nil {
		t.Fatalf("load owned file: %v", err)
	}
	return file
}

func uploadReadyPublicLicense(t *testing.T, srv *app.Server) (uint64, string) {
	t.Helper()
	jpeg := minimalJPEG()
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  model.FileBizMerchantLicense,
		"file_name": "license.jpg",
		"file_size": len(jpeg),
		"mime_type": "image/jpeg",
	}, nil)
	if presign.Code != common.CodeOK {
		t.Fatalf("presign license: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	rawToken := str(presign.Data["file_token"])
	if fileID == 0 || objectKey == "" || rawToken == "" {
		t.Fatalf("invalid license presign: %+v", presign)
	}
	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey, "file_token": rawToken,
	}, "file", "license.jpg", jpeg, nil)
	if upload.Code != common.CodeOK {
		t.Fatalf("upload license: %+v", upload)
	}
	return fileID, rawToken
}
