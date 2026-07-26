package media

import "testing"

func TestPolicyAllowsExact10MiB(t *testing.T) {
	policy := DefaultUploadPolicy()
	if policy.MaxOriginalBytes != 10*1024*1024 {
		t.Fatalf("default original limit = %d", policy.MaxOriginalBytes)
	}
	if err := policy.ValidateOriginalSize(10 * 1024 * 1024); err != nil {
		t.Fatalf("expected 10 MiB to be accepted: %v", err)
	}
}

func TestPolicyRejectsOneByteOver10MiB(t *testing.T) {
	policy := DefaultUploadPolicy()
	if err := policy.ValidateOriginalSize(10*1024*1024 + 1); err == nil {
		t.Fatal("expected one byte over 10 MiB to be rejected")
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
