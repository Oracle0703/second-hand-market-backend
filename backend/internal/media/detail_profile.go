package media

import (
	"bytes"
	"image"
	"path/filepath"
	"strings"
	"time"

	"second-hand-market-backend/backend/internal/common"
)

const (
	DetailProfileVersion       = "detail-v1"
	DetailMaxEdge             = 1280
	DetailTargetBytes   int64 = 300 * 1024
	DetailHardLimitBytes int64 = 500 * 1024
	DetailProcessTimeout       = 60 * time.Second
)

var (
	DetailEdges     = []int{1280, 1120, 960}
	DetailQualities = []int{82, 78, 74, 70, 66}
)

type DetailImagePolicy struct {
	MaxEdge        int
	TargetBytes    int64
	HardLimitBytes int64
	Edges          []int
	Qualities      []int
	Timeout        time.Duration
}

type DetailCandidate struct {
	Content     []byte
	LongestEdge int
	Quality     int
	SizeBytes   int64
}

func DefaultDetailImagePolicy() DetailImagePolicy {
	return DetailImagePolicy{
		MaxEdge:        DetailMaxEdge,
		TargetBytes:    DetailTargetBytes,
		HardLimitBytes: DetailHardLimitBytes,
		Edges:          append([]int(nil), DetailEdges...),
		Qualities:      append([]int(nil), DetailQualities...),
		Timeout:        DetailProcessTimeout,
	}
}

func (p DetailImagePolicy) normalized() DetailImagePolicy {
	if p.MaxEdge <= 0 {
		p.MaxEdge = DetailMaxEdge
	}
	if p.TargetBytes <= 0 {
		p.TargetBytes = DetailTargetBytes
	}
	if p.HardLimitBytes <= 0 {
		p.HardLimitBytes = DetailHardLimitBytes
	}
	if len(p.Edges) == 0 {
		p.Edges = append([]int(nil), DetailEdges...)
	}
	if len(p.Qualities) == 0 {
		p.Qualities = append([]int(nil), DetailQualities...)
	}
	if p.Timeout <= 0 {
		p.Timeout = DetailProcessTimeout
	}
	return p
}

func (p DetailImagePolicy) Select(candidates []DetailCandidate) (DetailCandidate, error) {
	p = p.normalized()
	var fallback DetailCandidate
	hasFallback := false
	for _, candidate := range candidates {
		if candidate.SizeBytes <= 0 {
			continue
		}
		if candidate.SizeBytes <= p.TargetBytes {
			return candidate, nil
		}
		if candidate.SizeBytes <= p.HardLimitBytes && !hasFallback {
			fallback = candidate
			hasFallback = true
		}
	}
	if hasFallback {
		return fallback, nil
	}
	return DetailCandidate{}, common.ErrInvalidUpload
}

func ValidateDetailJPEG(policy DetailImagePolicy, content []byte) (int, int, error) {
	policy = policy.normalized()
	if len(content) == 0 || int64(len(content)) > policy.HardLimitBytes {
		return 0, 0, common.ErrInvalidUpload
	}
	if DetectImageMIME(content) != "image/jpeg" {
		return 0, 0, common.ErrInvalidUpload
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, common.ErrInvalidUpload
	}
	if maxInt(cfg.Width, cfg.Height) > policy.MaxEdge {
		return 0, 0, common.ErrInvalidUpload
	}
	return cfg.Width, cfg.Height, nil
}

func IsDetailProductImageKey(key string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(key))
	if normalized == "" || strings.HasPrefix(normalized, "/") ||
		normalized == "." || normalized == ".." ||
		strings.Contains(normalized, "../") ||
		strings.Contains(normalized, "/../") ||
		strings.Contains(normalized, "/..") {
		return false
	}
	return strings.HasPrefix(normalized, "product_image/detail-v1/") &&
		strings.HasSuffix(strings.ToLower(normalized), ".jpg")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
