package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const fileCapabilityTTL = 15 * time.Minute

func validateMerchantFilesForBinding(
	tx *gorm.DB,
	merchantID uint64,
	fileIDs []uint64,
	wantBizType string,
) error {
	if merchantID == 0 || len(fileIDs) == 0 {
		return common.ErrInvalidFileBinding
	}

	seen := make(map[uint64]struct{}, len(fileIDs))
	for _, id := range fileIDs {
		if id == 0 {
			return common.ErrInvalidFileBinding
		}
		if _, exists := seen[id]; exists {
			return common.ErrInvalidFileBinding
		}
		seen[id] = struct{}{}
	}

	var files []model.FileRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ?", fileIDs).
		Find(&files).Error; err != nil {
		return err
	}
	if len(files) != len(fileIDs) {
		return common.ErrInvalidFileBinding
	}

	for _, file := range files {
		if file.BizType != wantBizType ||
			file.ScanStatus != model.FileScanPass ||
			!fileHasCompletedStorage(file) ||
			file.OwnerMerchantID == nil ||
			*file.OwnerMerchantID != merchantID {
			return common.ErrInvalidFileBinding
		}
	}
	return nil
}

func fileHasCompletedStorage(file model.FileRecord) bool {
	switch file.BizType {
	case model.FileBizProductImage:
		return strings.TrimSpace(file.URL) != ""
	case model.FileBizMerchantLicense:
		return strings.TrimSpace(file.ObjectKey) != ""
	default:
		return false
	}
}

func fileCapabilityHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newFileCapability(now time.Time) (string, string, time.Time, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", "", time.Time{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(random)
	return raw, fileCapabilityHash(raw), now.Add(fileCapabilityTTL), nil
}

func claimPublicMerchantLicense(
	tx *gorm.DB,
	fileID uint64,
	rawToken string,
	merchantID uint64,
	now time.Time,
) error {
	if fileID == 0 || merchantID == 0 || strings.TrimSpace(rawToken) == "" {
		return common.ErrInvalidFileBinding
	}

	result := tx.Model(&model.FileRecord{}).
		Where(
			"id = ? AND biz_type = ? AND scan_status = ? AND object_key <> '' AND uploader_type = ? AND owner_merchant_id IS NULL AND capability_token_hash = ? AND capability_expires_at > ?",
			fileID,
			model.FileBizMerchantLicense,
			model.FileScanPass,
			model.UserTypePublic,
			fileCapabilityHash(rawToken),
			now,
		).
		Updates(map[string]interface{}{
			"owner_merchant_id":     merchantID,
			"capability_token_hash": nil,
			"capability_expires_at": nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.ErrInvalidFileBinding
	}
	return nil
}
