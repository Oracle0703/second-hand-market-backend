package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	if fileID == 0 || objectKey == "" {
		t.Fatalf("invalid presign response: %+v", presign)
	}

	// Minimal JPEG bytes for MIME detection.
	jpeg := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F',
		0x00, 0x01, 0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00,
		0xFF, 0xD9,
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
