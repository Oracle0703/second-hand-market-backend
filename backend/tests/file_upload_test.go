package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/media"
	"second-hand-market-backend/backend/internal/model"
)

type multipartTestResponse struct {
	apiResp
	HTTPStatus int
}

func buildMultipartBody(
	t *testing.T,
	fields map[string]string,
	fileField,
	fileName string,
	content []byte,
) (*bytes.Buffer, string) {
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
	return &body, writer.FormDataContentType()
}

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
) multipartTestResponse {
	t.Helper()
	body, contentType := buildMultipartBody(t, fields, fileField, fileName, content)

	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp apiResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v, raw=%s", err, w.Body.String())
	}
	return multipartTestResponse{apiResp: resp, HTTPStatus: w.Code}
}

func requestAnonymousPresignFromIP(t *testing.T, srv *app.Server, remoteAddr, forwardedFor string, fileSize int64) apiResp {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"biz_type":  model.FileBizMerchantLicense,
		"file_name": "license.jpg",
		"file_size": fileSize,
		"mime_type": "image/jpeg",
	})
	if err != nil {
		t.Fatalf("marshal presign request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/presign", bytes.NewReader(body))
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", "application/json")
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	out := httptest.NewRecorder()
	srv.Router.ServeHTTP(out, req)
	var resp apiResp
	if err := json.Unmarshal(out.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode presign response: %v, raw=%s", err, out.Body.String())
	}
	return resp
}

func TestFileUploadLocalPublicLicense(t *testing.T) {
	srv := newTestServer(t)
	jpeg := minimalJPEG()

	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": len(jpeg), "mime_type": "image/jpeg",
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

	if _, exists := upload.Data["url"]; exists {
		t.Fatalf("merchant license upload exposed a public url: %+v", upload)
	}
	if err := srv.DB.First(&record, fileID).Error; err != nil {
		t.Fatalf("reload uploaded license: %v", err)
	}
	if record.URL != "" || record.ObjectKey == "" || record.ScanStatus != model.FileScanPass {
		t.Fatalf("private license state = %+v", record)
	}
}

func TestProductImageUploadRemainsPublic(t *testing.T) {
	srv := newTestServer(t)
	adminToken := adminAccessToken(t, srv)
	jpeg := minimalJPEG()
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": model.FileBizProductImage, "file_name": "product.jpg", "file_size": len(jpeg), "mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if presign.Code != common.CodeOK {
		t.Fatalf("presign product image: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprint(fileID), "object_key": objectKey,
	}, "file", "product.jpg", jpeg, map[string]string{"Authorization": "Bearer " + adminToken})
	if upload.Code != common.CodeOK {
		t.Fatalf("upload product image: %+v", upload)
	}
	wantURL := "/uploads/" + objectKey
	if got := str(upload.Data["url"]); got != wantURL {
		t.Fatalf("product image url = %q, want %q", got, wantURL)
	}

	confirm := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/confirm", map[string]interface{}{
		"file_id": fileID, "object_key": objectKey,
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if confirm.Code != common.CodeOK || str(confirm.Data["url"]) != wantURL {
		t.Fatalf("confirm product image: %+v", confirm)
	}
	var record model.FileRecord
	if err := srv.DB.First(&record, fileID).Error; err != nil {
		t.Fatalf("load product image: %v", err)
	}
	if record.URL != wantURL {
		t.Fatalf("stored product image url = %q, want %q", record.URL, wantURL)
	}
}

func TestAnonymousFileUploadRequiresMatchingCapability(t *testing.T) {
	srv := newTestServer(t)
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": len(jpeg), "mime_type": "image/jpeg",
	}, nil)
	if presign.Code != common.CodeOK {
		t.Fatalf("presign failed: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	fileToken := str(presign.Data["file_token"])
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
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg", "file_size": len(jpeg), "mime_type": "image/jpeg",
	}, nil)
	if presign.Code != common.CodeOK {
		t.Fatalf("presign failed: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	fileToken := str(presign.Data["file_token"])
	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey, "file_token": fileToken,
	}, "file", "license.jpg", jpeg, nil)
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
			if tc.name == "matching" {
				if _, exists := resp.Data["url"]; exists {
					t.Fatalf("merchant license confirm exposed a public url: %+v", resp)
				}
				if str(resp.Data["object_key"]) != objectKey {
					t.Fatalf("confirm object key = %q, want %q", str(resp.Data["object_key"]), objectKey)
				}
			}
		})
	}
	var record model.FileRecord
	if err := srv.DB.First(&record, fileID).Error; err != nil {
		t.Fatalf("load confirmed license: %v", err)
	}
	if record.URL != "" {
		t.Fatalf("confirmed license url = %q, want empty", record.URL)
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

func TestAnonymousPresignPersistsHMACAndRateSafeCleanupAfter(t *testing.T) {
	srv := newTestServer(t)
	const rawIP = "192.0.2.10"
	const spoofedIP = "198.51.100.7"
	resp := requestAnonymousPresignFromIP(t, srv, rawIP+":12345", spoofedIP, 22)
	if resp.Code != common.CodeOK {
		t.Fatalf("anonymous presign: %+v", resp)
	}
	var file model.FileRecord
	if err := srv.DB.First(&file, numToUint64(resp.Data["file_id"])).Error; err != nil {
		t.Fatalf("load anonymous presign: %v", err)
	}
	const expectedHMAC = "db73c546bca8da58498b32a1de02e529633a74d2d803507cb7f11e0fcea1a598"
	if file.SourceIPHash == nil || *file.SourceIPHash != expectedHMAC {
		t.Fatalf("source hash = %v", file.SourceIPHash)
	}
	if file.CapabilityExpiresAt == nil || file.CleanupAfter == nil ||
		!file.CleanupAfter.Equal(file.CreatedAt.Add(time.Hour)) ||
		file.CleanupAfter.Before(file.CapabilityExpiresAt.Add(30*time.Minute)) {
		t.Fatalf("capability/cleanup timestamps = %v/%v", file.CapabilityExpiresAt, file.CleanupAfter)
	}
	if dump := fmt.Sprintf("%+v", file); strings.Contains(dump, rawIP) || strings.Contains(dump, spoofedIP) {
		t.Fatalf("file row disclosed source IP: %s", dump)
	}
}

func TestAnonymousPresignRatePersistsAcrossServerInstances(t *testing.T) {
	cfg := newTestAppConfig(t, "local")
	cfg.FileUploadAnonPresignPerHour = 1
	first, err := app.NewServer(cfg)
	if err != nil {
		t.Fatalf("first server: %v", err)
	}
	if resp := requestAnonymousPresignFromIP(t, first, "192.0.2.20:1000", "", 1); resp.Code != common.CodeOK {
		t.Fatalf("first presign: %+v", resp)
	}
	second, err := app.NewServer(cfg)
	if err != nil {
		t.Fatalf("second server: %v", err)
	}
	resp := requestAnonymousPresignFromIP(t, second, "192.0.2.20:2000", "", 1)
	if resp.Code != common.CodeRateLimit {
		t.Fatalf("second presign = %+v", resp)
	}
	var count int64
	if err := second.DB.Model(&model.FileRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("file count = %d, err=%v", count, err)
	}
}

func TestAnonymousPresignRejectsActiveFileAndByteQuotaWithoutCreatingRows(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*app.Config)
		firstSize  int64
		secondSize int64
	}{
		{
			name:       "active file count",
			configure:  func(cfg *app.Config) { cfg.FileUploadAnonActiveFiles = 1 },
			firstSize:  1,
			secondSize: 1,
		},
		{
			name:       "active bytes",
			configure:  func(cfg *app.Config) { cfg.FileUploadAnonActiveBytes = 2 },
			firstSize:  2,
			secondSize: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestAppConfig(t, "local")
			tc.configure(&cfg)
			srv := newTestServerFromConfig(t, cfg)
			if resp := requestAnonymousPresignFromIP(t, srv, "192.0.2.30:1000", "", tc.firstSize); resp.Code != common.CodeOK {
				t.Fatalf("first presign: %+v", resp)
			}
			resp := requestAnonymousPresignFromIP(t, srv, "192.0.2.30:2000", "", tc.secondSize)
			if resp.Code != common.CodeUploadQuotaExceeded {
				t.Fatalf("quota response = %+v", resp)
			}
			var count int64
			if err := srv.DB.Model(&model.FileRecord{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("file count = %d, err=%v", count, err)
			}
		})
	}
}

func TestMerchantAndGlobalPresignQuotaRejectWithoutCreatingRows(t *testing.T) {
	t.Run("merchant", func(t *testing.T) {
		cfg := newTestAppConfig(t, "local")
		cfg.FileUploadMerchantQuotaBytes = int64(len(minimalJPEG()))
		srv := newTestServerFromConfig(t, cfg)
		_, username, password := registerMerchant(t, srv, "merchant_quota")
		login := merchantLogin(t, srv, username, password)
		if login.Code != common.CodeOK {
			t.Fatalf("merchant login: %+v", login)
		}
		var before int64
		_ = srv.DB.Model(&model.FileRecord{}).Count(&before).Error
		resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
			"biz_type": model.FileBizMerchantLicense, "file_name": "extra.jpg", "file_size": 1, "mime_type": "image/jpeg",
		}, map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])})
		if resp.Code != common.CodeUploadQuotaExceeded {
			t.Fatalf("merchant quota response: %+v", resp)
		}
		var after int64
		_ = srv.DB.Model(&model.FileRecord{}).Count(&after).Error
		if after != before {
			t.Fatalf("merchant rejection created row: %d -> %d", before, after)
		}
	})

	t.Run("global", func(t *testing.T) {
		cfg := newTestAppConfig(t, "local")
		cfg.FileUploadGlobalQuotaBytes = int64(len(minimalJPEG()) + 1)
		srv := newTestServerFromConfig(t, cfg)
		registerMerchant(t, srv, "global_quota")
		var before int64
		_ = srv.DB.Model(&model.FileRecord{}).Count(&before).Error
		resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
			"biz_type": model.FileBizProductImage, "file_name": "admin.jpg", "file_size": 2, "mime_type": "image/jpeg",
		}, map[string]string{"Authorization": "Bearer " + adminAccessToken(t, srv)})
		if resp.Code != common.CodeUploadQuotaExceeded {
			t.Fatalf("global quota response: %+v", resp)
		}
		var after int64
		_ = srv.DB.Model(&model.FileRecord{}).Count(&after).Error
		if after != before {
			t.Fatalf("global rejection created row: %d -> %d", before, after)
		}
	})
}

func TestAuthenticatedPresignDoesNotPersistAnonymousGovernanceFields(t *testing.T) {
	srv := newTestServer(t)
	_, username, password := registerMerchant(t, srv, "governance_fields")
	login := merchantLogin(t, srv, username, password)
	if login.Code != common.CodeOK {
		t.Fatalf("merchant login: %+v", login)
	}
	merchantResp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": model.FileBizMerchantLicense, "file_name": "merchant.jpg", "file_size": 1, "mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])})
	adminResp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": model.FileBizProductImage, "file_name": "admin.jpg", "file_size": 1, "mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + adminAccessToken(t, srv)})
	for name, resp := range map[string]apiResp{"merchant": merchantResp, "admin": adminResp} {
		if resp.Code != common.CodeOK {
			t.Fatalf("%s presign: %+v", name, resp)
		}
		var file model.FileRecord
		if err := srv.DB.First(&file, numToUint64(resp.Data["file_id"])).Error; err != nil {
			t.Fatalf("load %s file: %v", name, err)
		}
		if file.SourceIPHash != nil || file.CleanupAfter != nil || file.CleanupClaimedAt != nil ||
			file.CleanupClaimToken != nil || file.CleanupAttempts != 0 {
			t.Fatalf("%s file has anonymous governance fields: %+v", name, file)
		}
	}
}

func TestFilePresignAllowsImageAt10MiB(t *testing.T) {
	srv := newTestServer(t)

	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "license.heic",
		"file_size": 10 * 1024 * 1024,
		"mime_type": "image/heic",
	}, nil)
	if resp.Code != 0 {
		t.Fatalf("presign should allow a 10 MiB image: %+v", resp)
	}
}

func TestFilePresignRejectsImageOver10MiBWithoutRow(t *testing.T) {
	srv := newTestServer(t)
	var before int64
	if err := srv.DB.Model(&model.FileRecord{}).Count(&before).Error; err != nil {
		t.Fatalf("count files before presign: %v", err)
	}

	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "huge.jpg",
		"file_size": 10*1024*1024 + 1,
		"mime_type": "image/jpeg",
	}, nil)
	if resp.Code != 10008 {
		t.Fatalf("presign should reject an image over 10 MiB: %+v", resp)
	}
	var after int64
	if err := srv.DB.Model(&model.FileRecord{}).Count(&after).Error; err != nil {
		t.Fatalf("count files after presign: %v", err)
	}
	if after != before {
		t.Fatalf("rejected presign created rows: before=%d after=%d", before, after)
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

type countingProcessor struct {
	calls  int
	result media.ProcessResult
	err    error
}

func (p *countingProcessor) Process(_ context.Context, _ media.ProcessRequest) (media.ProcessResult, error) {
	p.calls++
	if p.err != nil {
		return media.ProcessResult{}, p.err
	}
	return p.result, nil
}

func requireNoUploadArtifacts(t *testing.T, uploadDir string) {
	t.Helper()
	if err := filepath.WalkDir(uploadDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != uploadDir && !entry.IsDir() {
			t.Errorf("rejected upload wrote file %q", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("inspect upload directory: %v", err)
	}
}

func decodeMultipartResponse(t *testing.T, out *httptest.ResponseRecorder) multipartTestResponse {
	t.Helper()
	var resp apiResp
	if err := json.Unmarshal(out.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v, raw=%s", err, out.Body.String())
	}
	return multipartTestResponse{apiResp: resp, HTTPStatus: out.Code}
}

func anonymousUploadReservation(t *testing.T, srv *app.Server, fileSize int64) (uint64, string, string) {
	t.Helper()
	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": model.FileBizMerchantLicense, "file_name": "license.jpg", "file_size": fileSize, "mime_type": "image/jpeg",
	}, nil)
	if resp.Code != common.CodeOK {
		t.Fatalf("presign upload: %+v", resp)
	}
	return numToUint64(resp.Data["file_id"]), str(resp.Data["object_key"]), str(resp.Data["file_token"])
}

func TestFileUploadRejectsContentLengthOver11MiBBeforeMultipartParsing(t *testing.T) {
	processor := &countingProcessor{}
	srv, uploadDir := newTestServerWithUploadDir(t)
	srv.SetImageProcessor(processor)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=upload-boundary")
	req.ContentLength = 11*1024*1024 + 1
	out := httptest.NewRecorder()

	srv.Router.ServeHTTP(out, req)

	resp := decodeMultipartResponse(t, out)
	if resp.HTTPStatus != http.StatusRequestEntityTooLarge || resp.Code != common.CodeInvalidUpload {
		t.Fatalf("response = status %d, body %+v", resp.HTTPStatus, resp.apiResp)
	}
	if processor.calls != 0 {
		t.Fatalf("processor calls = %d, want 0", processor.calls)
	}
	requireNoUploadArtifacts(t, uploadDir)
}

func TestFileUploadRejectsChunkedBodyOver11MiBWithJSON413(t *testing.T) {
	processor := &countingProcessor{}
	srv, uploadDir := newTestServerWithUploadDir(t)
	srv.SetImageProcessor(processor)
	body, contentType := buildMultipartBody(t, nil, "file", "huge.jpg", make([]byte, 11*1024*1024))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = -1
	out := httptest.NewRecorder()

	srv.Router.ServeHTTP(out, req)

	resp := decodeMultipartResponse(t, out)
	if resp.HTTPStatus != http.StatusRequestEntityTooLarge || resp.Code != common.CodeInvalidUpload {
		t.Fatalf("response = status %d, body %+v", resp.HTTPStatus, resp.apiResp)
	}
	if processor.calls != 0 {
		t.Fatalf("processor calls = %d, want 0", processor.calls)
	}
	requireNoUploadArtifacts(t, uploadDir)
}

func TestFileUploadRejectsFileOver10MiBWithJSON413(t *testing.T) {
	processor := &countingProcessor{}
	srv, uploadDir := newTestServerWithUploadDir(t)
	srv.SetImageProcessor(processor)
	fileID, objectKey, fileToken := anonymousUploadReservation(t, srv, 10*1024*1024)

	resp := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprint(fileID), "object_key": objectKey, "file_token": fileToken,
	}, "file", "huge.jpg", make([]byte, 10*1024*1024+1), nil)

	if resp.HTTPStatus != http.StatusRequestEntityTooLarge || resp.Code != common.CodeInvalidUpload {
		t.Fatalf("response = status %d, body %+v", resp.HTTPStatus, resp.apiResp)
	}
	if processor.calls != 0 {
		t.Fatalf("processor calls = %d, want 0", processor.calls)
	}
	requireNoUploadArtifacts(t, uploadDir)
}

func TestFileUploadRejectsDeclaredActualSizeMismatchBeforeProcessor(t *testing.T) {
	processor := &countingProcessor{}
	srv, uploadDir := newTestServerWithUploadDir(t)
	srv.SetImageProcessor(processor)
	content := minimalJPEG()
	fileID, objectKey, fileToken := anonymousUploadReservation(t, srv, int64(len(content)+1))

	resp := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprint(fileID), "object_key": objectKey, "file_token": fileToken,
	}, "file", "license.jpg", content, nil)

	if resp.HTTPStatus != http.StatusBadRequest || resp.Code != common.CodeInvalidUpload {
		t.Fatalf("response = status %d, body %+v", resp.HTTPStatus, resp.apiResp)
	}
	if processor.calls != 0 {
		t.Fatalf("processor calls = %d, want 0", processor.calls)
	}
	requireNoUploadArtifacts(t, uploadDir)
}

func TestFileUploadRejectsProcessorExpansionWithoutWrite(t *testing.T) {
	content := minimalJPEG()
	processor := &countingProcessor{result: media.ProcessResult{
		OutputMIME: "image/jpeg",
		OutputExt:  ".jpg",
		Content:    append(append([]byte(nil), content...), 0),
	}}
	srv, uploadDir := newTestServerWithUploadDir(t)
	srv.SetImageProcessor(processor)
	fileID, objectKey, fileToken := anonymousUploadReservation(t, srv, int64(len(content)))

	resp := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprint(fileID), "object_key": objectKey, "file_token": fileToken,
	}, "file", "license.jpg", content, nil)

	if resp.HTTPStatus != http.StatusBadRequest || resp.Code != common.CodeInvalidUpload {
		t.Fatalf("response = status %d, body %+v", resp.HTTPStatus, resp.apiResp)
	}
	if processor.calls != 1 {
		t.Fatalf("processor calls = %d, want 1", processor.calls)
	}
	requireNoUploadArtifacts(t, uploadDir)
}

func TestFileUploadAcceptsExact10MiB(t *testing.T) {
	content := make([]byte, 10*1024*1024)
	copy(content, minimalJPEG())
	processor := &countingProcessor{result: media.ProcessResult{
		OutputMIME: "image/jpeg",
		OutputExt:  ".jpg",
		Content:    content,
	}}
	srv, uploadDir := newTestServerWithUploadDir(t)
	srv.SetImageProcessor(processor)
	fileID, objectKey, fileToken := anonymousUploadReservation(t, srv, int64(len(content)))

	resp := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprint(fileID), "object_key": objectKey, "file_token": fileToken,
	}, "file", "license.jpg", content, nil)

	if resp.HTTPStatus != http.StatusOK || resp.Code != common.CodeOK {
		t.Fatalf("response = status %d, body %+v", resp.HTTPStatus, resp.apiResp)
	}
	if processor.calls != 1 {
		t.Fatalf("processor calls = %d, want 1", processor.calls)
	}
	finalPath := filepath.Join(uploadDir, filepath.FromSlash(objectKey))
	if stat, err := os.Stat(finalPath); err != nil || stat.Size() != int64(len(content)) {
		t.Fatalf("stored file = size %v, err %v", func() interface{} {
			if stat == nil {
				return nil
			}
			return stat.Size()
		}(), err)
	}
	if _, err := os.Stat(finalPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains: %v", err)
	}
}

func (p fakeProcessor) Process(_ context.Context, _ media.ProcessRequest) (media.ProcessResult, error) {
	if p.err != nil {
		return media.ProcessResult{}, p.err
	}
	return p.result, nil
}

func TestFileUploadStoresProcessedMetadata(t *testing.T) {
	processed := []byte("processed-image-content")
	original := []byte("original-image-content-with-padding")
	srv := newTestServerWithProcessor(t, fakeProcessor{
		result: media.ProcessResult{
			OutputMIME: "image/heic",
			OutputExt:  ".heic",
			Content:    processed,
		},
	})
	adminToken := adminAccessToken(t, srv)

	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "PRODUCT_IMAGE", "file_name": "product.heic", "file_size": len(original), "mime_type": "image/heic",
	}, map[string]string{"Authorization": "Bearer " + adminToken})
	if presign.Code != 0 {
		t.Fatalf("presign failed: %+v", presign)
	}

	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	upload := requestMultipart(
		t,
		srv.Router,
		http.MethodPost,
		"/api/v1/files/upload",
		map[string]string{
			"file_id":    fmt.Sprintf("%d", fileID),
			"object_key": objectKey,
		},
		"file",
		"product.heic",
		original,
		map[string]string{"Authorization": "Bearer " + adminToken},
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
