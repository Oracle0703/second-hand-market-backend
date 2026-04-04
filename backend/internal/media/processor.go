package media

import (
	"bytes"
	"context"
	"path/filepath"
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
	detected := DetectImageMIME(req.Content, req.InputMIME, req.FileName)
	if !IsAllowedImageMIME(detected) {
		return ProcessResult{}, common.ErrInvalidUpload
	}
	content := append([]byte(nil), req.Content...)
	return ProcessResult{
		OutputMIME: detected,
		OutputExt:  MIMEExt(detected, req.FileName),
		Content:    content,
	}, nil
}

func IsAllowedImageMIME(mimeType string) bool {
	return allowedImageMIMEs[normalizeMIME(mimeType)]
}

func DetectImageMIME(content []byte, inputMIME, fileName string) string {
	if len(content) > 0 {
		if mt, err := mimetype.DetectReader(bytes.NewReader(content)); err == nil && mt != nil {
			normalized := normalizeMIME(mt.String())
			if IsAllowedImageMIME(normalized) {
				return normalized
			}
		}
	}

	if normalized := normalizeMIME(inputMIME); IsAllowedImageMIME(normalized) {
		return normalized
	}

	switch strings.ToLower(filepath.Ext(strings.TrimSpace(fileName))) {
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

func MIMEExt(mimeType, fileName string) string {
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
	}
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	if ext != "" {
		return ext
	}
	return ""
}

func normalizeMIME(mimeType string) string {
	return strings.ToLower(strings.TrimSpace(mimeType))
}
