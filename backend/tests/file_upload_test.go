package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"second-hand-market-backend/backend/internal/app"
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
	resp.HTTPStatus = w.Code
	return resp
}

func encodedUploadImage(t *testing.T, mimeType string) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 5, 4))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, color.NRGBA{
				R: uint8(20 + x*35),
				G: uint8(40 + y*40),
				B: uint8(220 - x*25),
				A: 255,
			})
		}
	}

	var output bytes.Buffer
	var err error
	switch mimeType {
	case "image/jpeg":
		err = jpeg.Encode(&output, img, &jpeg.Options{Quality: 96})
	case "image/png":
		err = png.Encode(&output, img)
	default:
		t.Fatalf("unsupported test MIME: %s", mimeType)
	}
	if err != nil {
		t.Fatalf("encode upload fixture: %v", err)
	}
	return output.Bytes()
}

func assertRejectedUploadState(t *testing.T, srv *app.Server, uploadDir string, fileID uint64, objectKey string) {
	t.Helper()
	var record model.FileRecord
	if err := srv.DB.First(&record, fileID).Error; err != nil {
		t.Fatalf("load rejected file record: %v", err)
	}
	if record.ScanStatus != model.FileScanPending || record.URL != "" {
		t.Fatalf("rejected file was promoted: %+v", record)
	}

	finalPath := filepath.Join(uploadDir, filepath.FromSlash(objectKey))
	for _, path := range []string{finalPath, finalPath + ".tmp"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("rejected content was written to %s: err=%v", path, err)
		}
	}
}

func TestFileUploadReencodesAndServesCanonicalImages(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		ext      string
		format   string
	}{
		{name: "jpeg", mimeType: "image/jpeg", ext: ".jpg", format: "jpeg"},
		{name: "png", mimeType: "image/png", ext: ".png", format: "png"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			marker := []byte("<script>original-trailer()</script>")
			original := append(encodedUploadImage(t, tc.mimeType), marker...)

			presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
				"biz_type":  "MERCHANT_LICENSE",
				"file_name": "poc.html",
				"file_size": len(original),
				"mime_type": tc.mimeType,
			}, nil)
			if presign.Code != 0 {
				t.Fatalf("presign failed: %+v", presign)
			}
			fileID := numToUint64(presign.Data["file_id"])
			objectKey := str(presign.Data["object_key"])
			if fileID == 0 || !strings.HasSuffix(objectKey, tc.ext) || strings.HasSuffix(objectKey, ".html") {
				t.Fatalf("presign did not choose canonical extension: %+v", presign)
			}

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
				"poc.html",
				original,
				nil,
			)
			if upload.Code != 0 {
				t.Fatalf("upload failed: %+v", upload)
			}
			if got := str(upload.Data["object_key"]); got != objectKey {
				t.Fatalf("unexpected final object key: got=%q want=%q", got, objectKey)
			}

			url := str(upload.Data["url"])
			if !strings.HasPrefix(url, "/uploads/") || !strings.HasSuffix(url, tc.ext) {
				t.Fatalf("unexpected public URL: %s", url)
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			srv.Router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("download uploaded file failed: status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Content-Type"); got != tc.mimeType {
				t.Fatalf("unexpected content type: got=%q want=%q", got, tc.mimeType)
			}
			if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("missing nosniff header: %q", got)
			}
			if got := w.Header().Get("Content-Security-Policy"); got != "sandbox; default-src 'none'" {
				t.Fatalf("unexpected content security policy: %q", got)
			}
			if bytes.Equal(w.Body.Bytes(), original) || bytes.Contains(w.Body.Bytes(), marker) {
				t.Fatal("download returned unsanitized original bytes")
			}
			if _, format, err := image.Decode(bytes.NewReader(w.Body.Bytes())); err != nil || format != tc.format {
				t.Fatalf("download is not a decodable %s: format=%q err=%v", tc.format, format, err)
			}

			var record model.FileRecord
			if err := srv.DB.First(&record, fileID).Error; err != nil {
				t.Fatalf("load file record: %v", err)
			}
			if record.ScanStatus != model.FileScanPass ||
				record.MimeType != tc.mimeType ||
				record.ObjectKey != objectKey ||
				record.SizeBytes != int64(len(w.Body.Bytes())) {
				t.Fatalf("stored metadata does not describe processed output: %+v", record)
			}
		})
	}
}

func TestFilePresignUsesCanonicalExtension(t *testing.T) {
	srv := newTestServer(t)
	tests := []struct {
		fileName string
		mimeType string
		wantExt  string
	}{
		{fileName: "poc.html", mimeType: "image/jpeg", wantExt: ".jpg"},
		{fileName: "poc.svg", mimeType: "image/png", wantExt: ".png"},
		{fileName: "wrong.jpg", mimeType: "image/webp", wantExt: ".webp"},
	}
	for _, tc := range tests {
		resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
			"biz_type":  "MERCHANT_LICENSE",
			"file_name": tc.fileName,
			"file_size": 1024,
			"mime_type": tc.mimeType,
		}, nil)
		if resp.Code != 0 {
			t.Fatalf("presign failed for %+v: %+v", tc, resp)
		}
		if objectKey := str(resp.Data["object_key"]); !strings.HasSuffix(objectKey, tc.wantExt) {
			t.Fatalf("unsafe object key for %+v: %q", tc, objectKey)
		}
	}
}

func TestFileUploadRejectsDisguisedAndMalformedImages(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{name: "html_declared_as_jpeg", content: []byte("<!doctype html><script>window.pwned=true</script>")},
		{name: "truncated_jpeg", content: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uploadDir := t.TempDir()
			srv := newTestServerWithUploadDir(t, uploadDir)
			presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
				"biz_type":  "MERCHANT_LICENSE",
				"file_name": "poc.html",
				"file_size": len(tc.content),
				"mime_type": "image/jpeg",
			}, nil)
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
				"poc.html",
				tc.content,
				nil,
			)
			if upload.Code != 10008 {
				t.Fatalf("unsafe upload should be rejected: %+v", upload)
			}
			if upload.HTTPStatus != http.StatusBadRequest {
				t.Fatalf("unsafe upload returned HTTP %d, want %d", upload.HTTPStatus, http.StatusBadRequest)
			}
			assertRejectedUploadState(t, srv, uploadDir, fileID, objectKey)
		})
	}
}

func TestFileUploadRejectsReservedMIMEMismatch(t *testing.T) {
	uploadDir := t.TempDir()
	srv := newTestServerWithUploadDir(t, uploadDir)
	content := encodedUploadImage(t, "image/png")
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "photo.jpg",
		"file_size": len(content),
		"mime_type": "image/jpeg",
	}, nil)
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
		"photo.jpg",
		content,
		nil,
	)
	if upload.Code != 10008 {
		t.Fatalf("MIME mismatch should be rejected: %+v", upload)
	}
	if upload.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("MIME mismatch returned HTTP %d, want %d", upload.HTTPStatus, http.StatusBadRequest)
	}
	assertRejectedUploadState(t, srv, uploadDir, fileID, objectKey)
}

func TestPublicUploadHandlerBlocksExecutableAndMismatchedFiles(t *testing.T) {
	uploadDir := t.TempDir()
	srv := newTestServerWithUploadDir(t, uploadDir)
	productDir := filepath.Join(uploadDir, "product_image")
	if err := os.MkdirAll(productDir, 0o755); err != nil {
		t.Fatalf("create product directory: %v", err)
	}
	html := []byte("<!doctype html><script>window.pwned=true</script>")
	for _, name := range []string{"poc.html", "poc.jpg"} {
		if err := os.WriteFile(filepath.Join(productDir, name), html, 0o600); err != nil {
			t.Fatalf("write legacy unsafe file: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/uploads/product_image/"+name, nil)
		w := httptest.NewRecorder()
		srv.Router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("unsafe legacy file %s should be hidden: status=%d body=%q", name, w.Code, w.Body.Bytes())
		}
	}
}

func TestLocalFileResponsesIgnorePersistedExternalURL(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "legacy_file_url")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 || str(login.Data["token_scope"]) != "full" {
		t.Fatalf("approved merchant login failed: %+v", login)
	}
	merchantToken := str(login.Data["access_token"])
	productID := createAndOnShelfProduct(t, srv, merchantToken)

	var product model.Product
	if err := srv.DB.First(&product, productID).Error; err != nil {
		t.Fatalf("load product: %v", err)
	}
	if product.CoverFileID == nil {
		t.Fatal("test product has no cover file")
	}
	var file model.FileRecord
	if err := srv.DB.First(&file, *product.CoverFileID).Error; err != nil {
		t.Fatalf("load cover file: %v", err)
	}
	legacyURL := "https://legacy-cdn.example.test/poc.html"
	if err := srv.DB.Model(&file).Update("url", legacyURL).Error; err != nil {
		t.Fatalf("store legacy external URL: %v", err)
	}
	guardedURL := "/uploads/" + file.ObjectKey

	responses := []apiResp{
		requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/merchant/products/%d", productID), nil, map[string]string{
			"Authorization": "Bearer " + merchantToken,
		}),
		requestJSON(t, srv.Router, http.MethodGet, "/api/v1/buyer/products", nil, map[string]string{
			"X-Device-Id": "legacy-file-url-list",
		}),
		requestJSON(t, srv.Router, http.MethodGet, fmt.Sprintf("/api/v1/buyer/products/%d", productID), nil, map[string]string{
			"X-Device-Id": "legacy-file-url-detail",
		}),
	}
	for index, resp := range responses {
		if resp.Code != 0 || resp.HTTPStatus != http.StatusOK {
			t.Fatalf("response %d failed: %+v", index, resp)
		}
		payload, err := json.Marshal(resp.Data)
		if err != nil {
			t.Fatalf("marshal response %d: %v", index, err)
		}
		if bytes.Contains(payload, []byte(legacyURL)) {
			t.Fatalf("response %d exposed persisted external URL: %s", index, payload)
		}
		if !bytes.Contains(payload, []byte(guardedURL)) {
			t.Fatalf("response %d did not use guarded URL %q: %s", index, guardedURL, payload)
		}
	}
}

func TestConfirmCannotPromotePendingFile(t *testing.T) {
	srv := newTestServer(t)
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "license.jpg",
		"file_size": 1024,
		"mime_type": "image/jpeg",
	}, nil)
	if presign.Code != 0 {
		t.Fatalf("presign failed: %+v", presign)
	}

	confirm := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/confirm", map[string]interface{}{
		"file_id":    numToUint64(presign.Data["file_id"]),
		"object_key": str(presign.Data["object_key"]),
	}, nil)
	if confirm.Code != 10008 {
		t.Fatalf("pending file must not be confirmed without processing: %+v", confirm)
	}
}

func TestServerRejectsUnknownImageProcessorDriver(t *testing.T) {
	cfg := newTestConfig(t, t.TempDir())
	cfg.ImageProcessorDriver = "passthrough-typo"
	if _, err := app.NewServer(cfg); err == nil {
		t.Fatal("unknown image processor driver must fail closed")
	}
}

func TestServerRejectsExternalPublicBaseURLForLocalStorage(t *testing.T) {
	cfg := newTestConfig(t, t.TempDir())
	cfg.FilePublicBaseURL = "https://static.example.test/uploads"
	if _, err := app.NewServer(cfg); err == nil {
		t.Fatal("local uploads must not bypass the guarded /uploads handler")
	}
}

func TestServerRejectsUnsupportedFileStorageProvider(t *testing.T) {
	cfg := newTestConfig(t, t.TempDir())
	cfg.FileStorageProvider = "s3"
	if _, err := app.NewServer(cfg); err == nil {
		t.Fatal("unimplemented storage providers must fail closed")
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

func TestFilePresignRejectsInvalidSize(t *testing.T) {
	srv := newTestServer(t)
	tests := []struct {
		size     int
		wantCode int
	}{
		{size: 0, wantCode: 10001},
		{size: 40*1024*1024 + 1, wantCode: 10008},
	}
	for _, tc := range tests {
		resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
			"biz_type":  "MERCHANT_LICENSE",
			"file_name": "license.jpg",
			"file_size": tc.size,
			"mime_type": "image/jpeg",
		}, nil)
		if resp.Code != tc.wantCode {
			t.Fatalf("presign should reject size %d with code %d: %+v", tc.size, tc.wantCode, resp)
		}
	}
}

func TestFileUploadRejectsOversizedMultipartBeforeParsing(t *testing.T) {
	uploadDir := t.TempDir()
	multipartTmpDir := t.TempDir()
	t.Setenv("TMPDIR", multipartTmpDir)

	cfg := newTestConfig(t, uploadDir)
	cfg.FileUploadMaxBytes = 1024
	srv, err := app.NewServer(cfg)
	if err != nil {
		t.Fatalf("new size-limited server: %v", err)
	}
	srv.Router.MaxMultipartMemory = 64

	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "license.jpg",
		"file_size": 1024,
		"mime_type": "image/jpeg",
	}, nil)
	if presign.Code != 0 {
		t.Fatalf("presign failed: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	oversized := make([]byte, 2*1024*1024)
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
		"oversized.jpg",
		oversized,
		nil,
	)
	if upload.Code != 10008 || upload.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("oversized multipart should be rejected before parsing: %+v", upload)
	}
	assertRejectedUploadState(t, srv, uploadDir, fileID, objectKey)
	entries, err := os.ReadDir(multipartTmpDir)
	if err != nil {
		t.Fatalf("read multipart temp directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("oversized request created multipart temp files: %v", entries)
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

func TestFileUploadRejectsInvalidProcessorContract(t *testing.T) {
	tests := []struct {
		name   string
		result media.ProcessResult
	}{
		{
			name: "unsafe_output_extension",
			result: media.ProcessResult{
				OutputMIME: "image/jpeg",
				OutputExt:  ".html",
			},
		},
		{
			name: "output_MIME_differs_from_reservation",
			result: media.ProcessResult{
				OutputMIME: "image/png",
				OutputExt:  ".png",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uploadDir := t.TempDir()
			content := encodedUploadImage(t, "image/jpeg")
			tc.result.Content = content
			if tc.result.OutputMIME == "image/png" {
				tc.result.Content = encodedUploadImage(t, "image/png")
			}
			srv := newTestServerWithUploadDir(t, uploadDir)
			srv.SetImageProcessor(fakeProcessor{result: tc.result})
			presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
				"biz_type":  "MERCHANT_LICENSE",
				"file_name": "license.jpg",
				"file_size": len(content),
				"mime_type": "image/jpeg",
			}, nil)
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
				"license.jpg",
				content,
				nil,
			)
			if upload.Code != 10008 {
				t.Fatalf("invalid processor contract should be rejected: %+v", upload)
			}
			if upload.HTTPStatus != http.StatusBadRequest {
				t.Fatalf("invalid processor contract returned HTTP %d, want %d", upload.HTTPStatus, http.StatusBadRequest)
			}
			assertRejectedUploadState(t, srv, uploadDir, fileID, objectKey)
		})
	}
}

func TestApprovedMerchantProductImageRejectsHTMLDeclaredAsJPEG(t *testing.T) {
	uploadDir := t.TempDir()
	srv := newTestServerWithUploadDir(t, uploadDir)
	merchantID, username, password := registerMerchant(t, srv, "product_image_xss")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 || str(login.Data["token_scope"]) != "full" {
		t.Fatalf("approved merchant login failed: %+v", login)
	}
	merchantToken := str(login.Data["access_token"])

	html := []byte("<!doctype html><script>localStorage.getItem('access_token')</script>")
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "PRODUCT_IMAGE",
		"file_name": "poc.html",
		"file_size": len(html),
		"mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if presign.Code != 0 {
		t.Fatalf("product image presign failed: %+v", presign)
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
		"poc.html",
		html,
		map[string]string{"Authorization": "Bearer " + merchantToken},
	)
	if upload.Code != 10008 || upload.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("approved merchant HTML product image should be rejected: %+v", upload)
	}
	assertRejectedUploadState(t, srv, uploadDir, fileID, objectKey)
}
