package tests

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/media"
)

func TestStrictImageVipsHTTPIntegration(t *testing.T) {
	if os.Getenv("STRICT_IMAGE_VIPS_INTEGRATION") != "1" {
		t.Skip("set STRICT_IMAGE_VIPS_INTEGRATION=1 to run the real libvips HTTP path")
	}
	binary := os.Getenv("IMAGE_PROCESSOR_BIN")
	if binary == "" {
		binary = "vips"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Fatalf("required vips binary is unavailable: %v", err)
	}

	cfg := newTestConfig(t, t.TempDir())
	cfg.ImageProcessorDriver = "vips"
	cfg.ImageProcessorBin = binary
	srv, err := app.NewServer(cfg)
	if err != nil {
		t.Fatalf("start vips-backed test server: %v", err)
	}

	formats := []struct {
		name        string
		fixtureMIME string
		claimedMIME string
		outputMIME  string
		outputExt   string
	}{
		{name: "jpeg", fixtureMIME: "image/jpeg", claimedMIME: "image/jpeg", outputMIME: "image/jpeg", outputExt: ".jpg"},
		{name: "webp", fixtureMIME: "image/webp", claimedMIME: "image/webp", outputMIME: "image/webp", outputExt: ".webp"},
		{name: "heif", fixtureMIME: "image/heif", claimedMIME: "image/heif", outputMIME: "image/heic", outputExt: ".heic"},
		{name: "heic_declared_heif", fixtureMIME: "image/heic", claimedMIME: "image/heif", outputMIME: "image/heic", outputExt: ".heic"},
		{name: "heif_declared_heic", fixtureMIME: "image/heif", claimedMIME: "image/heic", outputMIME: "image/heic", outputExt: ".heic"},
	}
	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			marker := []byte("<script>http-vips-trailer()</script>")
			fixture := vipsHTTPFixture(t, binary, tc.fixtureMIME)
			if got := media.DetectImageMIME(fixture); got != tc.fixtureMIME {
				t.Fatalf("fixture MIME mismatch: got=%q want=%q", got, tc.fixtureMIME)
			}
			original := append(fixture, marker...)
			presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
				"biz_type":  "MERCHANT_LICENSE",
				"file_name": "poc.html",
				"file_size": len(original),
				"mime_type": tc.claimedMIME,
			}, nil)
			if presign.Code != 0 {
				t.Fatalf("presign failed: %+v", presign)
			}
			fileID := numToUint64(presign.Data["file_id"])
			reservedObjectKey := str(presign.Data["object_key"])
			if strings.HasSuffix(reservedObjectKey, ".html") {
				t.Fatalf("vips path received unsafe object key: %q", reservedObjectKey)
			}

			upload := requestMultipart(
				t,
				srv.Router,
				http.MethodPost,
				"/api/v1/files/upload",
				map[string]string{
					"file_id":    fmt.Sprintf("%d", fileID),
					"object_key": reservedObjectKey,
				},
				"file",
				"poc.html",
				original,
				nil,
			)
			if upload.Code != 0 || upload.HTTPStatus != http.StatusOK {
				t.Fatalf("vips-backed HTTP upload failed: %+v", upload)
			}
			finalObjectKey := str(upload.Data["object_key"])
			if !strings.HasSuffix(finalObjectKey, tc.outputExt) {
				t.Fatalf("unexpected canonical output key: %q", finalObjectKey)
			}

			url := str(upload.Data["url"])
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			srv.Router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("download processed image: status=%d body=%s", w.Code, w.Body.String())
			}
			if w.Header().Get("Content-Type") != tc.outputMIME ||
				w.Header().Get("X-Content-Type-Options") != "nosniff" ||
				w.Header().Get("Content-Security-Policy") != "sandbox; default-src 'none'" {
				t.Fatalf("unsafe public response headers: %+v", w.Header())
			}
			if bytes.Equal(w.Body.Bytes(), original) ||
				bytes.Contains(w.Body.Bytes(), marker) ||
				media.DetectImageMIME(w.Body.Bytes()) != tc.outputMIME {
				t.Fatalf("HTTP path returned unsanitized or invalid %s bytes", tc.outputMIME)
			}

			confirm := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/confirm", map[string]interface{}{
				"file_id":    fileID,
				"object_key": finalObjectKey,
			}, nil)
			if confirm.Code != 0 {
				t.Fatalf("processed upload should confirm idempotently: %+v", confirm)
			}
		})
	}
}

func vipsHTTPFixture(t *testing.T, binary, mimeType string) []byte {
	t.Helper()
	if mimeType == "image/jpeg" {
		return encodedUploadImage(t, mimeType)
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.png")
	if err := os.WriteFile(inputPath, encodedUploadImage(t, "image/png"), 0o600); err != nil {
		t.Fatalf("write vips HTTP source fixture: %v", err)
	}
	outputExt := ".webp"
	if mimeType == "image/heif" {
		outputExt = ".heic"
	}
	outputPath := filepath.Join(tmpDir, "fixture"+outputExt)
	if output, err := exec.Command(binary, "copy", inputPath, outputPath+"[Q=82,strip]").CombinedOutput(); err != nil {
		t.Fatalf("create %s HTTP fixture with vips: %v\n%s", mimeType, err, output)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read %s HTTP fixture: %v", mimeType, err)
	}
	if mimeType == "image/heif" {
		if len(content) < 12 || !bytes.Equal(content[4:8], []byte("ftyp")) {
			t.Fatal("generated HEIC fixture has no ISO BMFF ftyp box")
		}
		content = append([]byte(nil), content...)
		copy(content[8:12], []byte("mif1"))
	}
	return content
}
