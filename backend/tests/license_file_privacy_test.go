package tests

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUploadFixture(t *testing.T, root, objectKey string, content []byte) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create upload fixture directory: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	return path
}

func TestPublicUploadAllowsOnlyProductImages(t *testing.T) {
	srv, uploadDir := newTestServerWithUploadDir(t)
	productBytes := []byte("public-product-image")
	licenseBytes := []byte("private-license-image")
	writeUploadFixture(t, uploadDir, "product_image/public.jpg", productBytes)
	writeUploadFixture(t, uploadDir, "merchant_license/private.jpg", licenseBytes)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "product image", path: "/uploads/product_image/public.jpg", wantStatus: http.StatusOK},
		{name: "merchant license", path: "/uploads/merchant_license/private.jpg", wantStatus: http.StatusNotFound},
		{name: "uppercase prefix", path: "/uploads/PRODUCT_IMAGE/public.jpg", wantStatus: http.StatusNotFound},
		{name: "unknown prefix", path: "/uploads/other/public.jpg", wantStatus: http.StatusNotFound},
		{name: "plain traversal", path: "/uploads/product_image/../merchant_license/private.jpg", wantStatus: http.StatusNotFound},
		{name: "encoded traversal", path: "/uploads/product_image/%2e%2e/merchant_license/private.jpg", wantStatus: http.StatusNotFound},
		{name: "missing object", path: "/uploads/product_image/", wantStatus: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			srv.Router.ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body=%q", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				if !bytes.Equal(w.Body.Bytes(), productBytes) {
					t.Fatalf("product bytes = %q, want %q", w.Body.Bytes(), productBytes)
				}
				if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
					t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
				}
				return
			}
			body := w.Body.String()
			if strings.Contains(body, uploadDir) || strings.Contains(body, "merchant_license/private.jpg") || bytes.Contains(w.Body.Bytes(), licenseBytes) {
				t.Fatalf("not-found response leaked private file details: %q", body)
			}
		})
	}
}

func TestPublicUploadRejectsSymlinkEscape(t *testing.T) {
	srv, uploadDir := newTestServerWithUploadDir(t)
	outsideDir := t.TempDir()
	outsideBytes := []byte("outside-upload-root")
	outsidePath := filepath.Join(outsideDir, "outside.jpg")
	if err := os.WriteFile(outsidePath, outsideBytes, 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	linkPath := filepath.Join(uploadDir, "product_image", "escape.jpg")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("create symlink directory: %v", err)
	}
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/uploads/product_image/escape.jpg", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if bytes.Contains(w.Body.Bytes(), outsideBytes) || strings.Contains(w.Body.String(), outsidePath) {
		t.Fatalf("response leaked outside file: %q", w.Body.String())
	}
}
