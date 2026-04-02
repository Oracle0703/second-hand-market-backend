package media

import "testing"

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
