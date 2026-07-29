package media

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"second-hand-market-backend/backend/internal/common"
)

func TestVipsCLIProcessorIntegration(t *testing.T) {
	if os.Getenv("STRICT_IMAGE_VIPS_INTEGRATION") != "1" {
		t.Skip("set STRICT_IMAGE_VIPS_INTEGRATION=1 to run against real libvips codecs")
	}
	binary := os.Getenv("IMAGE_PROCESSOR_BIN")
	if binary == "" {
		binary = "vips"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Fatalf("required vips binary is unavailable: %v", err)
	}

	policy := DefaultUploadPolicy()
	policy.TargetBytes = policy.MaxOriginalBytes
	processor := NewVipsCLIProcessor(binary, policy)
	formats := []struct {
		name        string
		fixtureMIME string
		claimedMIME string
		outputMIME  string
	}{
		{name: "jpeg", fixtureMIME: "image/jpeg", claimedMIME: "image/jpeg", outputMIME: "image/jpeg"},
		{name: "png", fixtureMIME: "image/png", claimedMIME: "image/png", outputMIME: "image/png"},
		{name: "webp", fixtureMIME: "image/webp", claimedMIME: "image/webp", outputMIME: "image/webp"},
		{name: "heic", fixtureMIME: "image/heic", claimedMIME: "image/heic", outputMIME: "image/heic"},
		{name: "heif", fixtureMIME: "image/heif", claimedMIME: "image/heif", outputMIME: "image/heic"},
		{name: "heic_declared_heif", fixtureMIME: "image/heic", claimedMIME: "image/heif", outputMIME: "image/heic"},
		{name: "heif_declared_heic", fixtureMIME: "image/heif", claimedMIME: "image/heic", outputMIME: "image/heic"},
	}
	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			fixture := vipsIntegrationFixture(t, binary, tc.fixtureMIME)
			detected := DetectImageMIME(fixture)
			if detected != tc.fixtureMIME {
				t.Fatalf("fixture does not exercise requested MIME: requested=%s detected=%s", tc.fixtureMIME, detected)
			}
			marker := []byte("<script>vips-trailer()</script>")
			original := append(fixture, marker...)

			result, err := processor.Process(context.Background(), ProcessRequest{
				FileName:  "poc.html",
				InputMIME: tc.claimedMIME,
				Content:   original,
			})
			if err != nil {
				t.Fatalf("real vips processing failed: %v", err)
			}
			if result.OutputMIME != tc.outputMIME || result.OutputExt != MIMEExt(tc.outputMIME) {
				t.Fatalf("unexpected output metadata: %+v", result)
			}
			if bytes.Equal(result.Content, original) || bytes.Contains(result.Content, marker) {
				t.Fatal("vips returned unsanitized original bytes")
			}
			if got := DetectImageMIME(result.Content); got != tc.outputMIME {
				t.Fatalf("unexpected output bytes MIME: got=%q want=%q", got, tc.outputMIME)
			}
		})
	}

	t.Run("html_rejected", func(t *testing.T) {
		result, err := processor.Process(context.Background(), ProcessRequest{
			FileName:  "poc.html",
			InputMIME: "image/jpeg",
			Content:   []byte("<!doctype html><script>window.pwned=true</script>"),
		})
		if err != common.ErrInvalidUpload || len(result.Content) != 0 {
			t.Fatalf("HTML was not rejected: result=%+v err=%v", result, err)
		}
	})

	t.Run("orientation", func(t *testing.T) {
		input := orientedJPEGFixture(t)
		result, err := processor.Process(context.Background(), ProcessRequest{
			FileName:  "portrait.jpg",
			InputMIME: "image/jpeg",
			Content:   input,
		})
		if err != nil {
			t.Fatalf("process oriented JPEG: %v", err)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(result.Content))
		if err != nil || format != "jpeg" {
			t.Fatalf("decode oriented output: format=%q err=%v", format, err)
		}
		if cfg.Width != 2 || cfg.Height != 3 {
			t.Fatalf("EXIF orientation was not applied: got=%dx%d want=2x3", cfg.Width, cfg.Height)
		}
		if bytes.Contains(result.Content, []byte("Exif\x00\x00")) {
			t.Fatal("EXIF metadata was not stripped from the normalized output")
		}
	})
}

func vipsIntegrationFixture(t *testing.T, binary, mimeType string) []byte {
	t.Helper()
	if mimeType == "image/jpeg" || mimeType == "image/png" {
		return encodedTestImage(t, mimeType)
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.png")
	if err := os.WriteFile(inputPath, encodedTestImage(t, "image/png"), 0o600); err != nil {
		t.Fatalf("write source fixture: %v", err)
	}
	outputMIME := mimeType
	if mimeType == "image/heif" {
		outputMIME = "image/heic"
	}
	outputPath := filepath.Join(tmpDir, "fixture"+MIMEExt(outputMIME))
	outputSpec := buildOutputSpec(outputPath, outputMIME, 82)
	if output, err := exec.Command(binary, "copy", inputPath, outputSpec).CombinedOutput(); err != nil {
		t.Fatalf("create %s fixture with vips: %v\n%s", mimeType, err, output)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated %s fixture: %v", mimeType, err)
	}
	if mimeType == "image/heif" {
		if len(content) < 12 || !bytes.Equal(content[4:8], []byte("ftyp")) {
			t.Fatalf("generated HEIC fixture has no ISO BMFF ftyp box")
		}
		content = append([]byte(nil), content...)
		copy(content[8:12], []byte("mif1"))
	}
	return content
}

func orientedJPEGFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(40 + x*80), G: uint8(30 + y*150), B: 80, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode oriented JPEG base: %v", err)
	}
	base := encoded.Bytes()
	if len(base) < 2 || base[0] != 0xff || base[1] != 0xd8 {
		t.Fatal("generated JPEG has no SOI marker")
	}

	// Big-endian TIFF with one Orientation=6 (rotate 90° clockwise) entry.
	exif := []byte{
		0xff, 0xe1, 0x00, 0x22,
		'E', 'x', 'i', 'f', 0x00, 0x00,
		'M', 'M', 0x00, 0x2a, 0x00, 0x00, 0x00, 0x08,
		0x00, 0x01,
		0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01, 0x00, 0x06, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	result := make([]byte, 0, len(base)+len(exif))
	result = append(result, base[:2]...)
	result = append(result, exif...)
	result = append(result, base[2:]...)
	return result
}
