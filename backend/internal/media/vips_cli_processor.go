package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"second-hand-market-backend/backend/internal/common"
)

type VipsCLIProcessor struct {
	Binary string
	Policy UploadPolicy
	runner func(ctx context.Context, binary string, args ...string) error
}

func NewVipsCLIProcessor(binary string, policy UploadPolicy) VipsCLIProcessor {
	if policy.MaxOriginalBytes <= 0 {
		policy = DefaultUploadPolicy()
	}
	return VipsCLIProcessor{
		Binary: defaultProcessorBinary(binary),
		Policy: policy,
	}
}

func (p VipsCLIProcessor) Process(ctx context.Context, req ProcessRequest) (ProcessResult, error) {
	if err := p.Policy.ValidateOriginalSize(int64(len(req.Content))); err != nil {
		return ProcessResult{}, err
	}
	if req.OutputProfile == DetailProfileVersion {
		return p.processDetail(ctx, req)
	}

	detected := DetectImageMIME(req.Content)
	if !IsAllowedImageMIME(detected) || !ImageMIMEMatchesClaim(detected, req.InputMIME) {
		return ProcessResult{}, common.ErrInvalidUpload
	}
	outputMIME := CanonicalImageMIME(detected)
	if outputMIME == "" {
		return ProcessResult{}, common.ErrInvalidUpload
	}

	resultTemplate := ProcessResult{
		OutputMIME: outputMIME,
		OutputExt:  MIMEExt(outputMIME),
	}

	tmpDir, err := os.MkdirTemp("", "shm-vips-*")
	if err != nil {
		return ProcessResult{}, common.ErrInternal
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input"+resultTemplate.OutputExt)
	if err := os.WriteFile(inputPath, req.Content, 0o600); err != nil {
		return ProcessResult{}, common.ErrInternal
	}

	normalizedPath := filepath.Join(tmpDir, "normalized"+resultTemplate.OutputExt)
	normalizeArgs := []string{"autorot", inputPath, buildOutputSpec(normalizedPath, outputMIME, 82)}
	if err := p.run(ctx, normalizeArgs...); err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) || os.IsNotExist(err) || ctx.Err() != nil {
			return ProcessResult{}, common.ErrInternal
		}
		return ProcessResult{}, common.ErrInvalidUpload
	}
	normalized, err := os.ReadFile(normalizedPath)
	if err != nil || len(normalized) == 0 || DetectImageMIME(normalized) != outputMIME {
		return ProcessResult{}, common.ErrInvalidUpload
	}
	best := resultTemplate
	best.Content = normalized
	if p.Policy.TargetBytes <= 0 || int64(len(normalized)) <= p.Policy.TargetBytes {
		return best, nil
	}

	qualities := []int{75, 68, 60, 55}
	scales := []float64{1, 0.85, 0.7, 0.55}
	var lastErr error
	for _, scale := range scales {
		for _, quality := range qualities {
			outPath := filepath.Join(tmpDir, fmt.Sprintf("out-%.2f-%d%s", scale, quality, resultTemplate.OutputExt))
			args := buildVipsArgs(normalizedPath, outPath, outputMIME, quality, scale)
			if err := p.run(ctx, args...); err != nil {
				lastErr = err
				continue
			}
			output, err := os.ReadFile(outPath)
			if err != nil || len(output) == 0 {
				if err != nil {
					lastErr = err
				}
				continue
			}
			if detectedOutputMIME := DetectImageMIME(output); detectedOutputMIME != outputMIME {
				lastErr = fmt.Errorf("vips output MIME mismatch: got %q want %q", detectedOutputMIME, outputMIME)
				continue
			}

			candidate := resultTemplate
			candidate.Content = output
			if len(best.Content) == 0 || len(candidate.Content) < len(best.Content) {
				best = candidate
			}
			if p.Policy.TargetBytes <= 0 || int64(len(candidate.Content)) <= p.Policy.TargetBytes {
				return candidate, nil
			}
		}
	}

	if len(best.Content) > 0 {
		return best, nil
	}
	if lastErr != nil {
		var execErr *exec.Error
		if errors.As(lastErr, &execErr) || os.IsNotExist(lastErr) || ctx.Err() != nil {
			return ProcessResult{}, common.ErrInternal
		}
		return ProcessResult{}, common.ErrInvalidUpload
	}
	return ProcessResult{}, common.ErrInvalidUpload
}

func (p VipsCLIProcessor) processDetail(ctx context.Context, req ProcessRequest) (ProcessResult, error) {
	detected := DetectImageMIME(req.Content)
	if !IsAllowedImageMIME(detected) {
		return ProcessResult{}, common.ErrInvalidUpload
	}
	sourceExt := MIMEExt(detected)
	if sourceExt == "" {
		return ProcessResult{}, common.ErrInvalidUpload
	}

	policy := DefaultDetailImagePolicy()
	timeout := policy.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "shm-vips-detail-*")
	if err != nil {
		return ProcessResult{}, common.ErrInternal
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input"+sourceExt)
	if err := os.WriteFile(inputPath, req.Content, 0o600); err != nil {
		return ProcessResult{}, common.ErrInternal
	}

	var (
		candidates []DetailCandidate
		lastErr    error
	)
	for _, edge := range policy.Edges {
		for _, quality := range policy.Qualities {
			outPath := filepath.Join(tmpDir, fmt.Sprintf("detail-%d-%d.jpg", edge, quality))
			if err := p.run(ctx, buildDetailVipsArgs(inputPath, outPath, edge, quality)...); err != nil {
				if isProcessorRuntimeError(ctx, err) {
					return ProcessResult{}, common.ErrInternal
				}
				lastErr = err
				continue
			}
			output, err := os.ReadFile(outPath)
			if err != nil {
				lastErr = err
				continue
			}
			width, height, err := ValidateDetailJPEG(policy, output)
			if err != nil {
				lastErr = err
				continue
			}
			candidate := DetailCandidate{
				Content:     output,
				LongestEdge: maxInt(width, height),
				Quality:     quality,
				SizeBytes:   int64(len(output)),
			}
			candidates = append(candidates, candidate)
			if candidate.SizeBytes <= policy.TargetBytes {
				return detailResult(candidate, width, height), nil
			}
		}
	}

	candidate, err := policy.Select(candidates)
	if err != nil {
		if lastErr != nil && isProcessorRuntimeError(ctx, lastErr) {
			return ProcessResult{}, common.ErrInternal
		}
		return ProcessResult{}, err
	}
	width, height, err := ValidateDetailJPEG(policy, candidate.Content)
	if err != nil {
		return ProcessResult{}, err
	}
	return detailResult(candidate, width, height), nil
}

func detailResult(candidate DetailCandidate, width, height int) ProcessResult {
	return ProcessResult{
		OutputMIME: "image/jpeg",
		OutputExt:  ".jpg",
		Content:    candidate.Content,
		Width:      width,
		Height:     height,
	}
}

func isProcessorRuntimeError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	var execErr *exec.Error
	return errors.As(err, &execErr) || os.IsNotExist(err) || ctx.Err() != nil
}

func (p VipsCLIProcessor) run(ctx context.Context, args ...string) error {
	if p.runner != nil {
		return p.runner(ctx, p.Binary, args...)
	}
	return exec.CommandContext(ctx, p.Binary, args...).Run()
}

func defaultProcessorBinary(binary string) string {
	if binary == "" {
		return "vips"
	}
	return binary
}

func buildOutputSpec(path, mimeType string, quality int) string {
	switch normalizeMIME(mimeType) {
	case "image/png":
		return path + "[compression=9,strip]"
	case "image/jpeg", "image/webp", "image/heic", "image/heif":
		return path + "[Q=" + strconv.Itoa(quality) + ",strip]"
	default:
		return path
	}
}

func buildDetailVipsArgs(inputPath, outPath string, edge, quality int) []string {
	return []string{
		"thumbnail",
		inputPath,
		buildDetailOutputSpec(outPath, quality),
		strconv.Itoa(edge),
		"--height",
		strconv.Itoa(edge),
		"--size",
		"down",
		"--auto-rotate",
	}
}

func buildDetailOutputSpec(path string, quality int) string {
	return path + "[Q=" + strconv.Itoa(quality) + ",strip]"
}

func buildVipsArgs(inputPath, outPath, mimeType string, quality int, scale float64) []string {
	outSpec := buildOutputSpec(outPath, mimeType, quality)
	if scale >= 0.999 {
		return []string{"copy", inputPath, outSpec}
	}
	return []string{"resize", inputPath, outSpec, fmt.Sprintf("%.4f", scale)}
}
