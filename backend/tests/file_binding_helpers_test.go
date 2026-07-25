package tests

import (
	"fmt"
	"net/http"
	"testing"

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
