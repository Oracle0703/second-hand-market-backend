package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"second-hand-market-backend/backend/internal/common"
)

func encodedTestImage(t *testing.T, mimeType string) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, color.NRGBA{
				R: uint8(30 + x*40),
				G: uint8(20 + y*60),
				B: uint8(200 - x*20),
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
		t.Fatalf("encode test image: %v", err)
	}
	return output.Bytes()
}

func TestPolicyAllowsUploadWhenOriginalWithin40MB(t *testing.T) {
	policy := UploadPolicy{
		MaxOriginalBytes: 40 * 1024 * 1024,
		TargetBytes:      20 * 1024 * 1024,
	}

	if err := policy.ValidateOriginalSize(40 * 1024 * 1024); err != nil {
		t.Fatalf("expected 40MB to be accepted: %v", err)
	}
}

func TestPolicyRejectsOriginalOver40MB(t *testing.T) {
	policy := UploadPolicy{
		MaxOriginalBytes: 40 * 1024 * 1024,
		TargetBytes:      20 * 1024 * 1024,
	}

	if err := policy.ValidateOriginalSize(40*1024*1024 + 1); err == nil {
		t.Fatal("expected oversize original file to be rejected")
	}
}

func TestAllowedImageMIMERejectsQuickTime(t *testing.T) {
	if IsAllowedImageMIME("video/quicktime") {
		t.Fatal("quicktime video must not be treated as image")
	}
}

func TestDetectImageMIMEIgnoresClaimedTypeAndFilename(t *testing.T) {
	html := []byte("<!doctype html><script>window.pwned=true</script>")
	if got := DetectImageMIME(html, "image/jpeg", "poc.html"); got != "" {
		t.Fatalf("client metadata must not turn HTML into an image: %q", got)
	}
}

func TestCanonicalImageExtensionsIgnoreClientFilename(t *testing.T) {
	if got := MIMEExt("image/jpeg", "poc.html"); got != ".jpg" {
		t.Fatalf("unexpected canonical JPEG extension: %q", got)
	}
	if got := MIMEForExt(".html"); got != "" {
		t.Fatalf("executable extension must not map to an image MIME: %q", got)
	}
	if got := MIMEForExt(".JPEG"); got != "image/jpeg" {
		t.Fatalf("legacy JPEG extension should remain readable: %q", got)
	}
}

func TestCanonicalImageMIMENormalizesGenericHEIF(t *testing.T) {
	if got := CanonicalImageMIME("image/heif"); got != "image/heic" {
		t.Fatalf("generic HEIF must have a stable encoded form: %q", got)
	}
	if got := CanonicalImageMIME("image/webp"); got != "image/webp" {
		t.Fatalf("WebP should retain its MIME: %q", got)
	}
	if got := CanonicalImageMIME("text/html"); got != "" {
		t.Fatalf("unsupported MIME must not get a canonical image form: %q", got)
	}
}

func TestDetailImagePolicyUsesFixedTraversalAndHardLimit(t *testing.T) {
	policy := DefaultDetailImagePolicy()
	if policy.MaxEdge != 1280 || policy.TargetBytes != 300*1024 || policy.HardLimitBytes != 500*1024 {
		t.Fatalf("unexpected detail policy constants: %+v", policy)
	}

	candidate, err := policy.Select([]DetailCandidate{
		{LongestEdge: 1280, Quality: 82, SizeBytes: 360 * 1024},
		{LongestEdge: 1280, Quality: 78, SizeBytes: 299 * 1024},
		{LongestEdge: 1120, Quality: 82, SizeBytes: 250 * 1024},
	})
	if err != nil {
		t.Fatalf("select target candidate: %v", err)
	}
	if candidate.LongestEdge != 1280 || candidate.Quality != 78 {
		t.Fatalf("selected candidate = %+v", candidate)
	}

	fallback, err := policy.Select([]DetailCandidate{
		{LongestEdge: 1280, Quality: 82, SizeBytes: 360 * 1024},
		{LongestEdge: 1120, Quality: 82, SizeBytes: 340 * 1024},
	})
	if err != nil {
		t.Fatalf("select hard-limit fallback: %v", err)
	}
	if fallback.LongestEdge != 1280 || fallback.Quality != 82 {
		t.Fatalf("fallback candidate = %+v", fallback)
	}

	_, err = policy.Select([]DetailCandidate{{
		LongestEdge: 960,
		Quality:     66,
		SizeBytes:   500*1024 + 1,
	}})
	if err != common.ErrInvalidUpload {
		t.Fatalf("oversized candidate error = %v", err)
	}
}

func TestDetailProductImageKeyRequiresVersionedJPEGPrefix(t *testing.T) {
	valid := "product_image/detail-v1/F123.jpg"
	if !IsDetailProductImageKey(valid) {
		t.Fatalf("valid detail key rejected: %q", valid)
	}
	for _, key := range []string{
		"product_image/F123.jpg",
		"product_image/detail-v1/F123.png",
		"merchant_license/detail-v1/F123.jpg",
		"../product_image/detail-v1/F123.jpg",
	} {
		if IsDetailProductImageKey(key) {
			t.Fatalf("invalid detail key accepted: %q", key)
		}
	}
}

func TestPassthroughDetailProfileIgnoresOutputMIMEClaimAndEmitsJPEG(t *testing.T) {
	processor := NewPassthroughProcessor(DefaultUploadPolicy())

	result, err := processor.Process(context.Background(), ProcessRequest{
		FileName:      "photo.png",
		InputMIME:     "image/jpeg",
		OutputProfile: DetailProfileVersion,
		Content:       encodedTestImage(t, "image/png"),
	})
	if err != nil {
		t.Fatalf("process detail PNG with output claim: %v", err)
	}
	if result.OutputMIME != "image/jpeg" || result.OutputExt != ".jpg" {
		t.Fatalf("unexpected detail metadata: %+v", result)
	}
	if got := DetectImageMIME(result.Content); got != "image/jpeg" {
		t.Fatalf("detail output MIME = %q", got)
	}
	if _, format, err := image.Decode(bytes.NewReader(result.Content)); err != nil || format != "jpeg" {
		t.Fatalf("detail output is not decodable JPEG: format=%q err=%v", format, err)
	}
}

func TestPassthroughDetailProfileRejectsImagesItCannotResize(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1281, 1))
	var content bytes.Buffer
	if err := png.Encode(&content, img); err != nil {
		t.Fatalf("encode oversized fixture: %v", err)
	}

	result, err := NewPassthroughProcessor(DefaultUploadPolicy()).Process(context.Background(), ProcessRequest{
		InputMIME:     "image/png",
		OutputProfile: DetailProfileVersion,
		Content:       content.Bytes(),
	})
	if err != common.ErrInvalidUpload {
		t.Fatalf("oversized detail passthrough result=%+v err=%v", result, err)
	}
}

func TestImageMIMEMatchesClaimAllowsHEIFAliasesOnly(t *testing.T) {
	if !ImageMIMEMatchesClaim("image/heic", "image/heif") ||
		!ImageMIMEMatchesClaim("image/heif", "image/heic") {
		t.Fatal("HEIC and generic HEIF should be accepted as safe aliases")
	}
	if ImageMIMEMatchesClaim("image/png", "image/jpeg") ||
		ImageMIMEMatchesClaim("text/html", "image/jpeg") {
		t.Fatal("unrelated or non-image MIME claims must not match")
	}
}

func TestPassthroughProcessorReencodesJPEG(t *testing.T) {
	original := append(encodedTestImage(t, "image/jpeg"), []byte("<script>trailer()</script>")...)
	processor := NewPassthroughProcessor(DefaultUploadPolicy())

	result, err := processor.Process(context.Background(), ProcessRequest{
		FileName:  "poc.html",
		InputMIME: "image/jpeg",
		Content:   original,
	})
	if err != nil {
		t.Fatalf("process JPEG: %v", err)
	}
	if result.OutputMIME != "image/jpeg" || result.OutputExt != ".jpg" {
		t.Fatalf("unexpected JPEG metadata: %+v", result)
	}
	if bytes.Equal(result.Content, original) {
		t.Fatal("processor returned original upload bytes")
	}
	if bytes.Contains(result.Content, []byte("<script>")) {
		t.Fatal("re-encoded JPEG retained appended script bytes")
	}
	if _, format, err := image.Decode(bytes.NewReader(result.Content)); err != nil || format != "jpeg" {
		t.Fatalf("output is not a decodable JPEG: format=%q err=%v", format, err)
	}
	if result.Width != 4 || result.Height != 3 {
		t.Fatalf("unexpected dimensions: %dx%d", result.Width, result.Height)
	}
}

func TestPassthroughProcessorReencodesPNG(t *testing.T) {
	original := append(encodedTestImage(t, "image/png"), []byte("<script>trailer()</script>")...)
	processor := NewPassthroughProcessor(DefaultUploadPolicy())

	result, err := processor.Process(context.Background(), ProcessRequest{
		FileName:  "photo.png",
		InputMIME: "image/png",
		Content:   original,
	})
	if err != nil {
		t.Fatalf("process PNG: %v", err)
	}
	if result.OutputMIME != "image/png" || result.OutputExt != ".png" {
		t.Fatalf("unexpected PNG metadata: %+v", result)
	}
	if bytes.Equal(result.Content, original) || bytes.Contains(result.Content, []byte("<script>")) {
		t.Fatal("PNG was not sanitized by re-encoding")
	}
	if _, format, err := image.Decode(bytes.NewReader(result.Content)); err != nil || format != "png" {
		t.Fatalf("output is not a decodable PNG: format=%q err=%v", format, err)
	}
}

func TestPassthroughProcessorRejectsHTMLDeclaredAsJPEG(t *testing.T) {
	processor := NewPassthroughProcessor(DefaultUploadPolicy())
	result, err := processor.Process(context.Background(), ProcessRequest{
		FileName:  "poc.html",
		InputMIME: "image/jpeg",
		Content:   []byte("<!doctype html><script>window.pwned=true</script>"),
	})
	if err != common.ErrInvalidUpload {
		t.Fatalf("expected invalid upload, got result=%+v err=%v", result, err)
	}
	if len(result.Content) != 0 {
		t.Fatal("rejected upload returned content")
	}
}

func TestPassthroughProcessorRejectsMIMEClaimMismatch(t *testing.T) {
	processor := NewPassthroughProcessor(DefaultUploadPolicy())
	result, err := processor.Process(context.Background(), ProcessRequest{
		FileName:  "photo.jpg",
		InputMIME: "image/jpeg",
		Content:   encodedTestImage(t, "image/png"),
	})
	if err != common.ErrInvalidUpload {
		t.Fatalf("expected mismatch rejection, got result=%+v err=%v", result, err)
	}
}

func TestPassthroughProcessorRejectsTruncatedJPEG(t *testing.T) {
	processor := NewPassthroughProcessor(DefaultUploadPolicy())
	result, err := processor.Process(context.Background(), ProcessRequest{
		FileName:  "broken.jpg",
		InputMIME: "image/jpeg",
		Content:   []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'},
	})
	if err != common.ErrInvalidUpload {
		t.Fatalf("expected truncated JPEG rejection, got result=%+v err=%v", result, err)
	}
}

func TestVipsProcessorNeverFallsBackToOriginalBytes(t *testing.T) {
	original := encodedTestImage(t, "image/jpeg")
	processor := NewVipsCLIProcessor(filepath.Join(t.TempDir(), "missing-vips"), DefaultUploadPolicy())

	result, err := processor.Process(context.Background(), ProcessRequest{
		FileName:  "photo.jpg",
		InputMIME: "image/jpeg",
		Content:   original,
	})
	if err != common.ErrInternal {
		t.Fatalf("expected processor configuration error, got result=%+v err=%v", result, err)
	}
	if len(result.Content) != 0 {
		t.Fatal("failed vips execution returned original content")
	}
}

func TestVipsDetailProfileIgnoresOutputMIMEClaimAndStopsAtTarget(t *testing.T) {
	output := encodedTestImage(t, "image/jpeg")
	processor := NewVipsCLIProcessor("test-vips", DefaultUploadPolicy())
	commandCount := 0
	processor.runner = func(_ context.Context, _ string, args ...string) error {
		commandCount++
		if len(args) < 3 || args[0] != "thumbnail" {
			t.Fatalf("unexpected detail vips args: %v", args)
		}
		outputPath := strings.SplitN(args[2], "[", 2)[0]
		return os.WriteFile(outputPath, output, 0o600)
	}

	result, err := processor.Process(context.Background(), ProcessRequest{
		FileName:      "photo.png",
		InputMIME:     "image/jpeg",
		OutputProfile: DetailProfileVersion,
		Content:       encodedTestImage(t, "image/png"),
	})
	if err != nil {
		t.Fatalf("process detail through vips runner: %v", err)
	}
	if commandCount != 1 {
		t.Fatalf("target-sized candidate should short-circuit after one command, got %d", commandCount)
	}
	if result.OutputMIME != "image/jpeg" || result.OutputExt != ".jpg" || !bytes.Equal(result.Content, output) {
		t.Fatalf("unexpected detail result: %+v", result)
	}
}

func TestVipsDetailProfileUsesFirstHardLimitFallback(t *testing.T) {
	output := paddedJPEG(t, 340*1024)
	processor := NewVipsCLIProcessor("test-vips", DefaultUploadPolicy())
	commandCount := 0
	processor.runner = func(_ context.Context, _ string, args ...string) error {
		commandCount++
		outputPath := strings.SplitN(args[2], "[", 2)[0]
		return os.WriteFile(outputPath, output, 0o600)
	}

	result, err := processor.Process(context.Background(), ProcessRequest{
		InputMIME:     "image/jpeg",
		OutputProfile: DetailProfileVersion,
		Content:       encodedTestImage(t, "image/jpeg"),
	})
	if err != nil {
		t.Fatalf("process fallback detail candidate: %v", err)
	}
	if commandCount != len(DetailEdges)*len(DetailQualities) {
		t.Fatalf("fallback search commands = %d", commandCount)
	}
	if !bytes.Equal(result.Content, output) || int64(len(result.Content)) != 340*1024 {
		t.Fatalf("fallback result size = %d", len(result.Content))
	}
}

func TestVipsDetailProfileRejectsCandidatesOverHardLimit(t *testing.T) {
	output := paddedJPEG(t, int(DetailHardLimitBytes)+1)
	processor := NewVipsCLIProcessor("test-vips", DefaultUploadPolicy())
	processor.runner = func(_ context.Context, _ string, args ...string) error {
		outputPath := strings.SplitN(args[2], "[", 2)[0]
		return os.WriteFile(outputPath, output, 0o600)
	}

	result, err := processor.Process(context.Background(), ProcessRequest{
		InputMIME:     "image/jpeg",
		OutputProfile: DetailProfileVersion,
		Content:       encodedTestImage(t, "image/jpeg"),
	})
	if err != common.ErrInvalidUpload {
		t.Fatalf("oversized detail candidates result=%+v err=%v", result, err)
	}
}

func paddedJPEG(t *testing.T, size int) []byte {
	t.Helper()
	content := encodedTestImage(t, "image/jpeg")
	if len(content) > size {
		t.Fatalf("base JPEG size %d exceeds requested size %d", len(content), size)
	}
	padding := make([]byte, size-len(content))
	return append(content, padding...)
}

func TestVipsProcessorKeepsSanitizedResultWhenCompressionFails(t *testing.T) {
	marker := []byte("<script>original-trailer()</script>")
	original := append(encodedTestImage(t, "image/jpeg"), marker...)
	sanitized, err := NewPassthroughProcessor(DefaultUploadPolicy()).Process(
		context.Background(),
		ProcessRequest{InputMIME: "image/jpeg", Content: original},
	)
	if err != nil {
		t.Fatalf("build sanitized fixture: %v", err)
	}

	policy := DefaultUploadPolicy()
	policy.TargetBytes = 1
	processor := NewVipsCLIProcessor("test-vips", policy)
	commandCount := 0
	processor.runner = func(_ context.Context, _ string, args ...string) error {
		commandCount++
		if len(args) >= 3 && args[0] == "autorot" {
			outputPath := strings.SplitN(args[2], "[", 2)[0]
			return os.WriteFile(outputPath, sanitized.Content, 0o600)
		}
		return errors.New("forced compression failure")
	}

	result, err := processor.Process(context.Background(), ProcessRequest{
		FileName:  "photo.jpg",
		InputMIME: "image/jpeg",
		Content:   original,
	})
	if err != nil {
		t.Fatalf("safe normalized output should survive compression failure: %v", err)
	}
	if commandCount < 2 {
		t.Fatalf("compression retry branch was not exercised: commands=%d", commandCount)
	}
	if !bytes.Equal(result.Content, sanitized.Content) ||
		bytes.Equal(result.Content, original) ||
		bytes.Contains(result.Content, marker) {
		t.Fatal("compression failure did not return the sanitized normalized image")
	}
}

func TestDefaultProcessorBinaryFallsBackToVips(t *testing.T) {
	if got := defaultProcessorBinary(""); got != "vips" {
		t.Fatalf("unexpected default processor binary: %s", got)
	}
}

func TestBuildOutputSpecIncludesEncoderOptions(t *testing.T) {
	jpegSpec := buildOutputSpec("/tmp/test.jpg", "image/jpeg", 75)
	if jpegSpec != "/tmp/test.jpg[Q=75,strip]" {
		t.Fatalf("unexpected jpeg output spec: %s", jpegSpec)
	}

	pngSpec := buildOutputSpec("/tmp/test.png", "image/png", 75)
	if pngSpec != "/tmp/test.png[compression=9,strip]" {
		t.Fatalf("unexpected png output spec: %s", pngSpec)
	}
}
