package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/media"
	"second-hand-market-backend/backend/internal/model"
)

func requestMultipart(
	t *testing.T,
	h http.Handler,
	method,
	path string,
	fields map[string]string,
	fileField,
	fileName string,
	content []byte,
	headers map[string]string,
) apiResp {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatalf("write field failed: %v", err)
		}
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write file content failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp apiResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v, raw=%s", err, w.Body.String())
	}
	return resp
}

func TestFileUploadLocalPublicLicense(t *testing.T) {
	srv := newTestServer(t)

	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": 32, "mime_type": "image/jpeg",
	}, nil)
	if presign.Code != 0 {
		t.Fatalf("presign failed: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	fileToken := str(presign.Data["file_token"])
	if fileID == 0 || objectKey == "" || fileToken == "" {
		t.Fatalf("invalid presign response: %+v", presign)
	}
	var record model.FileRecord
	if err := srv.DB.First(&record, fileID).Error; err != nil {
		t.Fatalf("load presigned record: %v", err)
	}
	wantHash := sha256.Sum256([]byte(fileToken))
	if record.CapabilityTokenHash == nil || *record.CapabilityTokenHash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("capability hash was not persisted: %+v", record)
	}
	if *record.CapabilityTokenHash == fileToken {
		t.Fatal("raw file token must not be persisted")
	}

	jpeg := minimalJPEG()
	upload := requestMultipart(
		t,
		srv.Router,
		http.MethodPost,
		"/api/v1/files/upload",
		map[string]string{
			"file_id":    fmt.Sprintf("%d", fileID),
			"object_key": objectKey,
			"file_token": fileToken,
		},
		"file",
		"license.jpg",
		jpeg,
		nil,
	)
	if upload.Code != 0 {
		t.Fatalf("upload failed: %+v", upload)
	}

	url := str(upload.Data["url"])
	if !strings.HasPrefix(url, "/uploads/") {
		t.Fatalf("unexpected public url: %s", url)
	}

	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("download uploaded file failed: status=%d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), jpeg) {
		t.Fatalf("downloaded file mismatch: got=%d want=%d", len(w.Body.Bytes()), len(jpeg))
	}
}

func TestAnonymousFileUploadRequiresMatchingCapability(t *testing.T) {
	srv := newTestServer(t)
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": 22, "mime_type": "image/jpeg",
	}, nil)
	if presign.Code != common.CodeOK {
		t.Fatalf("presign failed: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	fileToken := str(presign.Data["file_token"])
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}

	for _, tc := range []struct {
		name  string
		token string
	}{
		{name: "missing"},
		{name: "wrong", token: "wrong-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
				"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey, "file_token": tc.token,
			}, "file", "license.jpg", jpeg, nil)
			if resp.Code != common.CodeInvalidFileBinding {
				t.Fatalf("response = %+v", resp)
			}
		})
	}

	expired := time.Now().Add(-time.Minute)
	if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Update("capability_expires_at", expired).Error; err != nil {
		t.Fatalf("expire capability: %v", err)
	}
	expiredResp := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey, "file_token": fileToken,
	}, "file", "license.jpg", jpeg, nil)
	if expiredResp.Code != common.CodeInvalidFileBinding {
		t.Fatalf("expired capability response = %+v", expiredResp)
	}
}

func TestAnonymousFileConfirmRequiresMatchingCapability(t *testing.T) {
	srv := newTestServer(t)
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": 22, "mime_type": "image/jpeg",
	}, nil)
	if presign.Code != common.CodeOK {
		t.Fatalf("presign failed: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	fileToken := str(presign.Data["file_token"])
	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey, "file_token": fileToken,
	}, "file", "license.jpg", []byte{0xFF, 0xD8, 0xFF, 0xD9}, nil)
	if upload.Code != common.CodeOK {
		t.Fatalf("upload failed: %+v", upload)
	}

	for _, tc := range []struct {
		name  string
		token string
		code  int
	}{
		{name: "missing", code: common.CodeInvalidFileBinding},
		{name: "wrong", token: "wrong-token", code: common.CodeInvalidFileBinding},
		{name: "matching", token: fileToken, code: common.CodeOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/confirm", map[string]interface{}{
				"file_id": fileID, "object_key": objectKey, "file_token": tc.token,
			}, nil)
			if resp.Code != tc.code {
				t.Fatalf("response = %+v", resp)
			}
		})
	}
}

func TestMerchantAndAdminPresignOwnership(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "presign_owner")
	login := merchantLogin(t, srv, username, password)
	if login.Code != common.CodeOK {
		t.Fatalf("merchant login failed: %+v", login)
	}
	merchantPresign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "owned.jpg", "file_size": 22, "mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])})
	if merchantPresign.Code != common.CodeOK {
		t.Fatalf("merchant presign failed: %+v", merchantPresign)
	}
	if str(merchantPresign.Data["file_token"]) != "" {
		t.Fatal("authenticated merchant presign returned a public capability")
	}
	var merchantFile model.FileRecord
	if err := srv.DB.First(&merchantFile, numToUint64(merchantPresign.Data["file_id"])).Error; err != nil {
		t.Fatalf("load merchant file: %v", err)
	}
	if merchantFile.OwnerMerchantID == nil || *merchantFile.OwnerMerchantID != merchantID {
		t.Fatalf("merchant owner = %v, want %d", merchantFile.OwnerMerchantID, merchantID)
	}

	adminPresign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "PRODUCT_IMAGE", "file_name": "admin.jpg", "file_size": 22, "mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + adminAccessToken(t, srv)})
	if adminPresign.Code != common.CodeOK {
		t.Fatalf("admin presign failed: %+v", adminPresign)
	}
	if str(adminPresign.Data["file_token"]) != "" {
		t.Fatal("admin presign returned a public capability")
	}
	var adminFile model.FileRecord
	if err := srv.DB.First(&adminFile, numToUint64(adminPresign.Data["file_id"])).Error; err != nil {
		t.Fatalf("load admin file: %v", err)
	}
	if adminFile.OwnerMerchantID != nil {
		t.Fatalf("admin file must remain unowned: %+v", adminFile)
	}
}

func TestFilePresignAllowsImageUpTo40MB(t *testing.T) {
	srv := newTestServer(t)

	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "license.heic",
		"file_size": 40 * 1024 * 1024,
		"mime_type": "image/heic",
	}, nil)
	if resp.Code != 0 {
		t.Fatalf("presign should allow 40MB image: %+v", resp)
	}
}

func TestFilePresignRejectsImageOver40MB(t *testing.T) {
	srv := newTestServer(t)

	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "huge.jpg",
		"file_size": 40*1024*1024 + 1,
		"mime_type": "image/jpeg",
	}, nil)
	if resp.Code != 10008 {
		t.Fatalf("presign should reject image > 40MB: %+v", resp)
	}
}

func TestFilePresignRejectsLivePhotoVideo(t *testing.T) {
	srv := newTestServer(t)

	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "live.mov",
		"file_size": 1024,
		"mime_type": "video/quicktime",
	}, nil)
	if resp.Code != 10008 {
		t.Fatalf("live photo video should be rejected: %+v", resp)
	}
}

type fakeProcessor struct {
	result media.ProcessResult
	err    error
}

func (p fakeProcessor) Process(_ context.Context, _ media.ProcessRequest) (media.ProcessResult, error) {
	if p.err != nil {
		return media.ProcessResult{}, p.err
	}
	return p.result, nil
}

func TestFileUploadStoresProcessedMetadata(t *testing.T) {
	processed := []byte("processed-image-content")
	srv := newTestServerWithProcessor(t, fakeProcessor{
		result: media.ProcessResult{
			OutputMIME: "image/heic",
			OutputExt:  ".heic",
			Content:    processed,
		},
	})

	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.heic", "file_size": 2048, "mime_type": "image/heic",
	}, nil)
	if presign.Code != 0 {
		t.Fatalf("presign failed: %+v", presign)
	}

	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	fileToken := str(presign.Data["file_token"])
	upload := requestMultipart(
		t,
		srv.Router,
		http.MethodPost,
		"/api/v1/files/upload",
		map[string]string{
			"file_id":    fmt.Sprintf("%d", fileID),
			"object_key": objectKey,
			"file_token": fileToken,
		},
		"file",
		"license.heic",
		[]byte("original-image-content"),
		nil,
	)
	if upload.Code != 0 {
		t.Fatalf("upload failed: %+v", upload)
	}

	req := httptest.NewRequest(http.MethodGet, str(upload.Data["url"]), nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("download uploaded file failed: status=%d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), processed) {
		t.Fatalf("stored bytes mismatch: got=%q want=%q", w.Body.Bytes(), processed)
	}
}
