package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

const anonymousPresignRateWindow = time.Hour

func anonymousUploadCleanupAfter(createdAt, capabilityExpiresAt time.Time, grace time.Duration) time.Time {
	cleanupAfter := capabilityExpiresAt.Add(grace)
	rateWindowEnd := createdAt.Add(anonymousPresignRateWindow)
	if cleanupAfter.Before(rateWindowEnd) {
		return rateWindowEnd
	}
	return cleanupAfter
}

func (s *Server) anonymousSourceHash(rawIP string) (string, error) {
	if strings.TrimSpace(s.cfg.FileUploadIPHashSecret) == "" {
		return "", common.ErrInternal
	}
	parsed := net.ParseIP(strings.TrimSpace(rawIP))
	if parsed == nil {
		return "", common.ErrInternal
	}
	canonical := parsed.To16()
	if v4 := parsed.To4(); v4 != nil {
		canonical = v4
	}
	if canonical == nil {
		return "", common.ErrInternal
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.FileUploadIPHashSecret))
	_, _ = mac.Write(canonical)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func quotaTransactionOptions(dialect string) *sql.TxOptions {
	if strings.EqualFold(strings.TrimSpace(dialect), "mysql") {
		return &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	return nil
}

func (s *Server) withQuotaTransaction(fn func(*gorm.DB) error) error {
	options := quotaTransactionOptions(s.DB.Dialector.Name())
	if options == nil {
		return s.DB.Transaction(fn)
	}
	return s.DB.Transaction(fn, options)
}

func lockFileQuotaGuard(tx *gorm.DB) error {
	query := tx.Model(&model.FileQuotaGuard{}).
		Select("id", "guard_name").
		Where("id = ?", 1)
	if tx.Dialector.Name() == "mysql" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var guard model.FileQuotaGuard
	if err := query.Take(&guard).Error; err != nil {
		return common.ErrInternal
	}
	if guard.ID != 1 || guard.GuardName != "file_records" {
		return common.ErrInternal
	}
	return nil
}

func (s *Server) reserveFileRecord(file *model.FileRecord, now time.Time) error {
	if file == nil || file.SizeBytes <= 0 || now.IsZero() {
		return common.ErrInternal
	}
	if file.UploaderType == model.UserTypePublic {
		if file.SourceIPHash == nil || len(*file.SourceIPHash) != sha256.Size*2 ||
			file.CleanupAfter == nil || file.OwnerMerchantID != nil {
			return common.ErrInternal
		}
	}
	file.CreatedAt = now

	return s.withQuotaTransaction(func(tx *gorm.DB) error {
		if err := lockFileQuotaGuard(tx); err != nil {
			return err
		}
		var negativeCount int64
		if err := tx.Model(&model.FileRecord{}).Where("size_bytes < 0").Count(&negativeCount).Error; err != nil {
			return common.ErrInternal
		}
		if negativeCount != 0 {
			return common.ErrInternal
		}

		globalBytes, err := sumFileBytes(tx.Model(&model.FileRecord{}))
		if err != nil {
			return common.ErrInternal
		}
		if quotaWouldExceed(globalBytes, file.SizeBytes, s.cfg.FileUploadGlobalQuotaBytes) {
			return common.ErrUploadQuotaExceeded
		}

		if file.UploaderType == model.UserTypePublic {
			if err := s.checkAnonymousReservation(tx, file, now); err != nil {
				return err
			}
		}
		if file.OwnerMerchantID != nil {
			merchantBytes, err := sumFileBytes(
				tx.Model(&model.FileRecord{}).Where("owner_merchant_id = ?", *file.OwnerMerchantID),
			)
			if err != nil {
				return common.ErrInternal
			}
			if quotaWouldExceed(merchantBytes, file.SizeBytes, s.cfg.FileUploadMerchantQuotaBytes) {
				return common.ErrUploadQuotaExceeded
			}
		}

		if err := tx.Create(file).Error; err != nil {
			return common.ErrInternal
		}
		return nil
	})
}

func (s *Server) checkAnonymousReservation(tx *gorm.DB, file *model.FileRecord, now time.Time) error {
	base := tx.Model(&model.FileRecord{}).
		Where("uploader_type = ? AND source_ip_hash = ?", model.UserTypePublic, *file.SourceIPHash)
	var recentCount int64
	if err := base.Where("created_at > ?", now.Add(-anonymousPresignRateWindow)).Count(&recentCount).Error; err != nil {
		return common.ErrInternal
	}
	if recentCount >= s.cfg.FileUploadAnonPresignPerHour {
		return common.ErrRateLimit
	}

	active := tx.Model(&model.FileRecord{}).Where(
		"uploader_type = ? AND owner_merchant_id IS NULL AND source_ip_hash = ? AND cleanup_after IS NOT NULL",
		model.UserTypePublic,
		*file.SourceIPHash,
	)
	var activeCount int64
	if err := active.Count(&activeCount).Error; err != nil {
		return common.ErrInternal
	}
	if activeCount >= s.cfg.FileUploadAnonActiveFiles {
		return common.ErrUploadQuotaExceeded
	}
	activeBytes, err := sumFileBytes(active)
	if err != nil {
		return common.ErrInternal
	}
	if quotaWouldExceed(activeBytes, file.SizeBytes, s.cfg.FileUploadAnonActiveBytes) {
		return common.ErrUploadQuotaExceeded
	}
	return nil
}

func sumFileBytes(query *gorm.DB) (int64, error) {
	var total int64
	if err := query.Select("COALESCE(SUM(size_bytes), 0)").Scan(&total).Error; err != nil {
		return 0, err
	}
	if total < 0 {
		return 0, common.ErrInternal
	}
	return total, nil
}

func quotaWouldExceed(current, addition, limit int64) bool {
	if current < 0 || addition <= 0 || limit <= 0 {
		return true
	}
	return addition > limit || current > limit-addition
}
