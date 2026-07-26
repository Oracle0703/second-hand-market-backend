package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/model"
)

const uploadCleanupCandidatePredicate = `
	uploader_type = ?
	AND owner_merchant_id IS NULL
	AND cleanup_after IS NOT NULL
	AND cleanup_after <= ?
	AND (cleanup_claimed_at IS NULL OR cleanup_claimed_at <= ?)`

var (
	errUploadCleanupUnsafePath          = errors.New("unsafe upload cleanup path")
	errUploadCleanupUnsupportedProvider = errors.New("unsupported upload cleanup provider")
	errUploadCleanupDeleteFailed        = errors.New("upload cleanup delete failed")
	errUploadCleanupClaimLost           = errors.New("upload cleanup claim lost")
	errUploadCleanupDatabase            = errors.New("upload cleanup database failure")
)

type uploadCleanupSummary struct {
	Claimed           int
	Deleted           int
	Failed            int
	FailureCategories map[string]int
}

func (s *Server) runUploadCleanupBatch(ctx context.Context, now time.Time) (uploadCleanupSummary, error) {
	summary := uploadCleanupSummary{FailureCategories: make(map[string]int)}
	claimed, err := s.claimUploadCleanupCandidates(ctx, now)
	if err != nil {
		summary.Failed++
		summary.FailureCategories["database_error"]++
		s.logUploadCleanupSummary(summary)
		return summary, err
	}
	summary.Claimed = len(claimed)
	for _, file := range claimed {
		if err := s.processUploadCleanupClaim(ctx, file); err != nil {
			summary.Failed++
			summary.FailureCategories[uploadCleanupFailureCategory(err)]++
			if errors.Is(err, errUploadCleanupDatabase) {
				s.logUploadCleanupSummary(summary)
				return summary, err
			}
			continue
		}
		summary.Deleted++
	}
	s.logUploadCleanupSummary(summary)
	return summary, nil
}

func (s *Server) claimUploadCleanupCandidates(ctx context.Context, now time.Time) ([]model.FileRecord, error) {
	if s.cfg.FileUploadCleanupBatchSize <= 0 || s.cfg.FileUploadCleanupClaimTTL <= 0 {
		return nil, errUploadCleanupDatabase
	}
	token, err := newUploadCleanupClaimToken()
	if err != nil {
		return nil, errUploadCleanupDatabase
	}
	staleBefore := now.Add(-s.cfg.FileUploadCleanupClaimTTL)
	claimed := make([]model.FileRecord, 0, s.cfg.FileUploadCleanupBatchSize)
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidates []model.FileRecord
		query := tx.Where(
			uploadCleanupCandidatePredicate,
			model.UserTypePublic,
			now,
			staleBefore,
		).Order("cleanup_after ASC").Order("id ASC").Limit(s.cfg.FileUploadCleanupBatchSize)
		if tx.Dialector.Name() == "mysql" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := query.Find(&candidates).Error; err != nil {
			return err
		}
		if len(candidates) == 0 {
			return nil
		}
		ids := make([]uint64, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.ID)
		}
		if err := tx.Model(&model.FileRecord{}).
			Where("id IN ?", ids).
			Where(
				uploadCleanupCandidatePredicate,
				model.UserTypePublic,
				now,
				staleBefore,
			).
			Updates(map[string]interface{}{
				"cleanup_claim_token": token,
				"cleanup_claimed_at":  now,
				"cleanup_attempts":    gorm.Expr("cleanup_attempts + 1"),
			}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ? AND cleanup_claim_token = ? AND uploader_type = ? AND owner_merchant_id IS NULL", ids, token, model.UserTypePublic).
			Order("cleanup_after ASC").Order("id ASC").
			Find(&claimed).Error
	})
	if err != nil {
		return nil, fmt.Errorf("%w", errUploadCleanupDatabase)
	}
	return claimed, nil
}

func (s *Server) processUploadCleanupClaim(ctx context.Context, claimed model.FileRecord) error {
	if claimed.CleanupClaimToken == nil || *claimed.CleanupClaimToken == "" {
		return errUploadCleanupClaimLost
	}
	token := *claimed.CleanupClaimToken
	var current model.FileRecord
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND cleanup_claim_token = ? AND uploader_type = ? AND owner_merchant_id IS NULL", claimed.ID, token, model.UserTypePublic).
		Take(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errUploadCleanupClaimLost
		}
		return errUploadCleanupDatabase
	}
	if err := s.removeManagedLocalFile(current.ObjectKey); err != nil {
		if releaseErr := s.releaseUploadCleanupClaim(ctx, current.ID, token); releaseErr != nil {
			return errUploadCleanupDatabase
		}
		return err
	}
	result := s.DB.WithContext(ctx).
		Where("id = ? AND cleanup_claim_token = ? AND uploader_type = ? AND owner_merchant_id IS NULL", current.ID, token, model.UserTypePublic).
		Delete(&model.FileRecord{})
	if result.Error != nil {
		_ = s.releaseUploadCleanupClaim(ctx, current.ID, token)
		return errUploadCleanupDatabase
	}
	if result.RowsAffected != 1 {
		return errUploadCleanupClaimLost
	}
	return nil
}

func (s *Server) releaseUploadCleanupClaim(ctx context.Context, id uint64, token string) error {
	result := s.DB.WithContext(ctx).Model(&model.FileRecord{}).
		Where("id = ? AND cleanup_claim_token = ? AND uploader_type = ? AND owner_merchant_id IS NULL", id, token, model.UserTypePublic).
		Updates(map[string]interface{}{
			"cleanup_claim_token": nil,
			"cleanup_claimed_at":  nil,
		})
	if result.Error != nil {
		return errUploadCleanupDatabase
	}
	return nil
}

func (s *Server) removeManagedLocalFile(objectKey string) error {
	if !strings.EqualFold(strings.TrimSpace(s.cfg.FileStorageProvider), "local") {
		return errUploadCleanupUnsupportedProvider
	}
	key, err := normalizeObjectKey(objectKey)
	if err != nil {
		return errUploadCleanupUnsafePath
	}
	root, err := filepath.Abs(s.cfg.FileUploadLocalDir)
	if err != nil {
		return errUploadCleanupDeleteFailed
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return errUploadCleanupDeleteFailed
	}
	rootInfo, err := os.Lstat(realRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return errUploadCleanupUnsafePath
	}
	parts := strings.Split(key, "/")
	current := realRoot
	for _, segment := range parts[:len(parts)-1] {
		current = filepath.Join(current, filepath.FromSlash(segment))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return errUploadCleanupDeleteFailed
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errUploadCleanupUnsafePath
		}
	}
	target := filepath.Join(current, filepath.FromSlash(parts[len(parts)-1]))
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errUploadCleanupDeleteFailed
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errUploadCleanupUnsafePath
	}
	if err := os.Remove(target); err != nil {
		return errUploadCleanupDeleteFailed
	}
	return nil
}

func (s *Server) runUploadCleanupLoop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	_, _ = s.runUploadCleanupBatch(ctx, time.Now())
	ticker := time.NewTicker(s.cfg.FileUploadCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			_, _ = s.runUploadCleanupBatch(ctx, now)
		}
	}
}

func newUploadCleanupClaimToken() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

func uploadCleanupFailureCategory(err error) string {
	switch {
	case errors.Is(err, errUploadCleanupUnsupportedProvider):
		return "unsupported_provider"
	case errors.Is(err, errUploadCleanupUnsafePath):
		return "unsafe_path"
	case errors.Is(err, errUploadCleanupClaimLost):
		return "claim_lost"
	case errors.Is(err, errUploadCleanupDatabase):
		return "database_error"
	default:
		return "delete_failed"
	}
}

func (s *Server) logUploadCleanupSummary(summary uploadCleanupSummary) {
	keys := make([]string, 0, len(summary.FailureCategories))
	for category := range summary.FailureCategories {
		keys = append(keys, category)
	}
	sort.Strings(keys)
	categories := make([]string, 0, len(keys))
	for _, category := range keys {
		categories = append(categories, fmt.Sprintf("%s=%d", category, summary.FailureCategories[category]))
	}
	if len(categories) == 0 {
		categories = append(categories, "none")
	}
	logf := s.cleanupLogf
	if logf == nil {
		logf = func(format string, args ...interface{}) {}
	}
	logf(
		"upload cleanup batch claimed=%d deleted=%d failed=%d failure_categories=%s",
		summary.Claimed,
		summary.Deleted,
		summary.Failed,
		strings.Join(categories, ","),
	)
}
