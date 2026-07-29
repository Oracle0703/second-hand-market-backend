package media

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"strings"

	"github.com/gabriel-vasile/mimetype"

	"second-hand-market-backend/backend/internal/common"
)

const (
	DefaultMaxOriginalBytes int64 = 40 * 1024 * 1024
	DefaultTargetBytes      int64 = 20 * 1024 * 1024
)

var allowedImageMIMEs = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/heic": true,
	"image/heif": true,
}

type UploadPolicy struct {
	MaxOriginalBytes int64
	TargetBytes      int64
	MinWidth         int
	MinHeight        int
	MinQuality       int
}

func DefaultUploadPolicy() UploadPolicy {
	return UploadPolicy{
		MaxOriginalBytes: DefaultMaxOriginalBytes,
		TargetBytes:      DefaultTargetBytes,
		MinWidth:         720,
		MinHeight:        720,
		MinQuality:       55,
	}
}

func (p UploadPolicy) ValidateOriginalSize(size int64) error {
	limit := p.MaxOriginalBytes
	if limit <= 0 {
		limit = DefaultMaxOriginalBytes
	}
	if size <= 0 || size > limit {
		return common.ErrInvalidUpload
	}
	return nil
}

type ProcessRequest struct {
	FileName  string
	InputMIME string
	Content   []byte
}

type ProcessResult struct {
	OutputMIME string
	OutputExt  string
	Content    []byte
	Width      int
	Height     int
}

type Processor interface {
	Process(ctx context.Context, req ProcessRequest) (ProcessResult, error)
}

// PassthroughProcessor keeps its historical name for configuration
// compatibility. It never passes bytes through: JPEG and PNG inputs are fully
// decoded and re-encoded, while formats that need libvips fail closed.
type PassthroughProcessor struct {
	policy UploadPolicy
}

func NewPassthroughProcessor(policy UploadPolicy) PassthroughProcessor {
	if policy.MaxOriginalBytes <= 0 {
		policy = DefaultUploadPolicy()
	}
	return PassthroughProcessor{policy: policy}
}

func (p PassthroughProcessor) Process(_ context.Context, req ProcessRequest) (ProcessResult, error) {
	if err := p.policy.ValidateOriginalSize(int64(len(req.Content))); err != nil {
		return ProcessResult{}, err
	}

	detected := DetectImageMIME(req.Content)
	if !ImageMIMEMatchesClaim(detected, req.InputMIME) ||
		(detected != "image/jpeg" && detected != "image/png") {
		return ProcessResult{}, common.ErrInvalidUpload
	}

	decoded, format, err := image.Decode(bytes.NewReader(req.Content))
	if err != nil || decoded.Bounds().Dx() <= 0 || decoded.Bounds().Dy() <= 0 {
		return ProcessResult{}, common.ErrInvalidUpload
	}

	var output bytes.Buffer
	switch detected {
	case "image/jpeg":
		if format != "jpeg" {
			return ProcessResult{}, common.ErrInvalidUpload
		}
		if err := jpeg.Encode(&output, decoded, &jpeg.Options{Quality: 82}); err != nil {
			return ProcessResult{}, common.ErrInvalidUpload
		}
	case "image/png":
		if format != "png" {
			return ProcessResult{}, common.ErrInvalidUpload
		}
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&output, decoded); err != nil {
			return ProcessResult{}, common.ErrInvalidUpload
		}
	}
	if output.Len() == 0 {
		return ProcessResult{}, common.ErrInvalidUpload
	}

	return ProcessResult{
		OutputMIME: detected,
		OutputExt:  MIMEExt(detected),
		Content:    output.Bytes(),
		Width:      decoded.Bounds().Dx(),
		Height:     decoded.Bounds().Dy(),
	}, nil
}

func IsAllowedImageMIME(mimeType string) bool {
	return allowedImageMIMEs[normalizeMIME(mimeType)]
}

// CanonicalImageMIME returns the MIME emitted by the processing pipeline.
// Generic HEIF input is normalized to HEIC because libvips/libheif otherwise
// commonly writes an HEIC bitstream even when the input used the mif1 brand.
func CanonicalImageMIME(mimeType string) string {
	normalized := normalizeMIME(mimeType)
	if !IsAllowedImageMIME(normalized) {
		return ""
	}
	if normalized == "image/heif" {
		return "image/heic"
	}
	return normalized
}

// ImageMIMEMatchesClaim treats HEIC and generic HEIF as safe aliases while
// keeping every other declared MIME bound to the detected byte format.
func ImageMIMEMatchesClaim(detectedMIME, claimedMIME string) bool {
	detectedCanonical := CanonicalImageMIME(detectedMIME)
	claimedCanonical := CanonicalImageMIME(claimedMIME)
	return detectedCanonical != "" && detectedCanonical == claimedCanonical
}

// DetectImageMIME deliberately trusts only the uploaded bytes. The optional
// arguments are retained for source compatibility with older callers, but a
// client-provided MIME type or filename must never turn non-image content into
// an accepted image.
func DetectImageMIME(content []byte, _ ...string) string {
	if len(content) == 0 {
		return ""
	}
	mt, err := mimetype.DetectReader(bytes.NewReader(content))
	if err != nil || mt == nil {
		return ""
	}
	detected := normalizeMIME(mt.String())
	if !IsAllowedImageMIME(detected) {
		return ""
	}
	return detected
}

// MIMEExt returns a server-controlled extension for an allowed image MIME.
// Optional filename arguments are intentionally ignored.
func MIMEExt(mimeType string, _ ...string) string {
	switch normalizeMIME(mimeType) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/heic":
		return ".heic"
	case "image/heif":
		return ".heif"
	default:
		return ""
	}
}

// MIMEForExt returns the image MIME that the server will send for a stored
// object. Unknown and executable extensions are rejected by returning "".
func MIMEForExt(ext string) string {
	switch normalizeMIME(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	default:
		return ""
	}
}

func normalizeMIME(mimeType string) string {
	return strings.ToLower(strings.TrimSpace(mimeType))
}
