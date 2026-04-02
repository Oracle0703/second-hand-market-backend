package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"second-hand-market-backend/backend/internal/common"
)

type VipsCLIProcessor struct {
	Binary string
	Policy UploadPolicy
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

	detected := DetectImageMIME(req.Content, req.InputMIME, req.FileName)
	if !IsAllowedImageMIME(detected) {
		return ProcessResult{}, common.ErrInvalidUpload
	}

	best := ProcessResult{
		OutputMIME: detected,
		OutputExt:  MIMEExt(detected, req.FileName),
		Content:    append([]byte(nil), req.Content...),
	}

	tmpDir, err := os.MkdirTemp("", "shm-vips-*")
	if err != nil {
		return ProcessResult{}, common.ErrInternal
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input"+best.OutputExt)
	if err := os.WriteFile(inputPath, req.Content, 0o600); err != nil {
		return ProcessResult{}, common.ErrInternal
	}

	qualities := []int{82, 75, 68, 60, 55}
	scales := []float64{1, 0.85, 0.7, 0.55}
	var lastErr error
	for _, scale := range scales {
		for _, quality := range qualities {
			outPath := filepath.Join(tmpDir, fmt.Sprintf("out-%.2f-%d%s", scale, quality, best.OutputExt))
			args := buildVipsArgs(inputPath, outPath, detected, quality, scale)
			if err := exec.CommandContext(ctx, p.Binary, args...).Run(); err != nil {
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
			if len(output) < len(best.Content) {
				best.Content = output
			}
			if int64(len(best.Content)) <= p.Policy.TargetBytes {
				return best, nil
			}
		}
	}

	if len(best.Content) > 0 {
		return best, nil
	}
	if lastErr != nil {
		return ProcessResult{}, common.ErrInvalidUpload
	}
	return ProcessResult{}, common.ErrInternal
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

func buildVipsArgs(inputPath, outPath, mimeType string, quality int, scale float64) []string {
	outSpec := buildOutputSpec(outPath, mimeType, quality)
	if scale >= 0.999 {
		return []string{"copy", inputPath, outSpec}
	}
	return []string{"resize", inputPath, outSpec, fmt.Sprintf("%.4f", scale)}
}
