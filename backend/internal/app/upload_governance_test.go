package app

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func governanceUnitServer(t *testing.T) *Server {
	t.Helper()
	cfg := securityTestConfig(t)
	cfg.FileUploadAnonPresignPerHour = 2
	cfg.FileUploadAnonActiveFiles = 2
	cfg.FileUploadAnonActiveBytes = 10
	cfg.FileUploadMerchantQuotaBytes = 20
	cfg.FileUploadGlobalQuotaBytes = 30
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func TestAnonymousSourceHashCanonicalizesAndUsesHMAC(t *testing.T) {
	srv := governanceUnitServer(t)
	a, err := srv.anonymousSourceHash("2001:db8::1")
	if err != nil {
		t.Fatalf("hash canonical IPv6: %v", err)
	}
	b, err := srv.anonymousSourceHash("2001:0db8:0:0:0:0:0:1")
	if err != nil {
		t.Fatalf("hash expanded IPv6: %v", err)
	}
	const expected = "05772f756734da6957f941942ad5ba44d2ec64d3864221138ec1d04d60cf8434"
	if a != expected || b != expected || len(a) != 64 || a == common.SHA256("2001:db8::1") {
		t.Fatalf("unsafe/noncanonical hashes: %q %q", a, b)
	}

	v4, err := srv.anonymousSourceHash("192.0.2.1")
	if err != nil {
		t.Fatalf("hash IPv4: %v", err)
	}
	mapped, err := srv.anonymousSourceHash("::ffff:192.0.2.1")
	if err != nil {
		t.Fatalf("hash mapped IPv4: %v", err)
	}
	if v4 != mapped {
		t.Fatalf("IPv4 hashes differ: %q %q", v4, mapped)
	}
}

func TestAnonymousSourceHashRejectsInvalidInputWithoutDisclosure(t *testing.T) {
	srv := governanceUnitServer(t)
	const invalidIP = "not-an-ip-address"
	if _, err := srv.anonymousSourceHash(invalidIP); err != common.ErrInternal || strings.Contains(err.Error(), invalidIP) {
		t.Fatalf("invalid IP error = %v", err)
	}
	srv.cfg.FileUploadIPHashSecret = ""
	if _, err := srv.anonymousSourceHash("192.0.2.1"); err != common.ErrInternal {
		t.Fatalf("empty secret error = %v", err)
	}
}

func TestReserveFileRecordRollsBackAtEachQuotaBoundary(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	sourceHash := strings.Repeat("a", 64)
	cleanupAfter := now.Add(time.Hour)
	expiredAt := now.Add(-time.Minute)
	merchantID := uint64(42)
	expiredCountOne := governanceFile("count-1", 1, now.Add(-2*time.Hour), &sourceHash, &cleanupAfter, nil)
	expiredCountOne.CapabilityExpiresAt = &expiredAt
	expiredCountTwo := governanceFile("count-2", 1, now.Add(-2*time.Hour), &sourceHash, &cleanupAfter, nil)
	expiredCountTwo.CapabilityExpiresAt = &expiredAt

	tests := []struct {
		name      string
		seed      []model.FileRecord
		candidate model.FileRecord
		wantErr   error
	}{
		{
			name: "rolling anonymous rate includes bound records",
			seed: []model.FileRecord{
				governanceFile("rate-bound", 1, now.Add(-30*time.Minute), &sourceHash, nil, &merchantID),
				governanceFile("rate-active", 1, now.Add(-20*time.Minute), &sourceHash, &cleanupAfter, nil),
			},
			candidate: governanceFile("rate-new", 1, now, &sourceHash, &cleanupAfter, nil),
			wantErr:   common.ErrRateLimit,
		},
		{
			name: "active anonymous file count includes expired capabilities",
			seed: []model.FileRecord{
				expiredCountOne,
				expiredCountTwo,
			},
			candidate: governanceFile("count-new", 1, now, &sourceHash, &cleanupAfter, nil),
			wantErr:   common.ErrUploadQuotaExceeded,
		},
		{
			name: "active anonymous bytes",
			seed: []model.FileRecord{
				governanceFile("bytes-existing", 9, now.Add(-2*time.Hour), &sourceHash, &cleanupAfter, nil),
			},
			candidate: governanceFile("bytes-new", 2, now, &sourceHash, &cleanupAfter, nil),
			wantErr:   common.ErrUploadQuotaExceeded,
		},
		{
			name: "merchant bytes",
			seed: []model.FileRecord{
				governanceFile("merchant-existing", 19, now.Add(-2*time.Hour), nil, nil, &merchantID),
			},
			candidate: governanceFile("merchant-new", 2, now, nil, nil, &merchantID),
			wantErr:   common.ErrUploadQuotaExceeded,
		},
		{
			name: "global bytes",
			seed: []model.FileRecord{
				governanceFile("global-existing", 29, now.Add(-2*time.Hour), nil, nil, nil),
			},
			candidate: governanceFile("global-new", 2, now, nil, nil, nil),
			wantErr:   common.ErrUploadQuotaExceeded,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := governanceUnitServer(t)
			if err := srv.DB.Create(&tc.seed).Error; err != nil {
				t.Fatalf("seed quota rows: %v", err)
			}
			assertReservationRejectedWithoutRow(t, srv, &tc.candidate, now, tc.wantErr)
		})
	}
}

func TestReserveFileRecordAllowsExactQuotaBoundaries(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	sourceHash := strings.Repeat("c", 64)
	cleanupAfter := now.Add(time.Hour)
	merchantID := uint64(42)
	cutoff := now.Add(-time.Hour)

	tests := []struct {
		name      string
		seed      []model.FileRecord
		candidate model.FileRecord
	}{
		{
			name: "rolling rate excludes exact one-hour cutoff",
			seed: []model.FileRecord{
				governanceFile("rate-cutoff", 1, cutoff, &sourceHash, nil, &merchantID),
				governanceFile("rate-inside", 1, now.Add(-time.Minute), &sourceHash, nil, &merchantID),
			},
			candidate: governanceFile("rate-boundary", 1, now, &sourceHash, &cleanupAfter, nil),
		},
		{
			name: "active file count equals limit",
			seed: []model.FileRecord{
				governanceFile("count-boundary-existing", 1, cutoff, &sourceHash, &cleanupAfter, nil),
			},
			candidate: governanceFile("count-boundary-new", 1, now, &sourceHash, &cleanupAfter, nil),
		},
		{
			name: "active bytes equal limit",
			seed: []model.FileRecord{
				governanceFile("bytes-boundary-existing", 8, cutoff, &sourceHash, &cleanupAfter, nil),
			},
			candidate: governanceFile("bytes-boundary-new", 2, now, &sourceHash, &cleanupAfter, nil),
		},
		{
			name: "merchant bytes equal limit",
			seed: []model.FileRecord{
				governanceFile("merchant-boundary-existing", 18, cutoff, nil, nil, &merchantID),
			},
			candidate: governanceFile("merchant-boundary-new", 2, now, nil, nil, &merchantID),
		},
		{
			name: "global bytes equal limit",
			seed: []model.FileRecord{
				governanceFile("global-boundary-existing", 28, cutoff, nil, nil, nil),
			},
			candidate: governanceFile("global-boundary-new", 2, now, nil, nil, nil),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := governanceUnitServer(t)
			if err := srv.DB.Create(&tc.seed).Error; err != nil {
				t.Fatalf("seed boundary rows: %v", err)
			}
			if err := srv.reserveFileRecord(&tc.candidate, now); err != nil {
				t.Fatalf("reserve at exact boundary: %v", err)
			}
			if tc.candidate.ID == 0 {
				t.Fatal("exact-boundary reservation was not persisted")
			}
		})
	}
}

func TestReserveFileRecordUsesOverflowSafeQuotaComparison(t *testing.T) {
	srv := governanceUnitServer(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	existing := governanceFile("huge-existing", math.MaxInt64, now.Add(-time.Hour), nil, nil, nil)
	if err := srv.DB.Create(&existing).Error; err != nil {
		t.Fatalf("seed huge row: %v", err)
	}
	candidate := governanceFile("overflow-new", 2, now, nil, nil, nil)
	assertReservationRejectedWithoutRow(t, srv, &candidate, now, common.ErrUploadQuotaExceeded)
}

func TestReserveFileRecordRejectsNegativeStoredSize(t *testing.T) {
	srv := governanceUnitServer(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	corrupt := governanceFile("negative-existing", -1, now.Add(-time.Hour), nil, nil, nil)
	if err := srv.DB.Create(&corrupt).Error; err != nil {
		t.Fatalf("seed corrupt row: %v", err)
	}
	candidate := governanceFile("negative-new", 1, now, nil, nil, nil)
	assertReservationRejectedWithoutRow(t, srv, &candidate, now, common.ErrInternal)
}

func TestReserveFileRecordRejectsMissingGuard(t *testing.T) {
	srv := governanceUnitServer(t)
	if err := srv.DB.Delete(&model.FileQuotaGuard{}, 1).Error; err != nil {
		t.Fatalf("delete quota guard: %v", err)
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	candidate := governanceFile("missing-guard", 1, now, nil, nil, nil)
	assertReservationRejectedWithoutRow(t, srv, &candidate, now, common.ErrInternal)
}

func TestReserveFileRecordPersistsLimitsAcrossServerRestart(t *testing.T) {
	cfg := securityTestConfig(t)
	cfg.FileUploadAnonPresignPerHour = 1
	cfg.FileUploadAnonActiveFiles = 2
	cfg.FileUploadAnonActiveBytes = 10
	cfg.FileUploadMerchantQuotaBytes = 20
	cfg.FileUploadGlobalQuotaBytes = 30
	first, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("first NewServer: %v", err)
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	sourceHash := strings.Repeat("b", 64)
	cleanupAfter := now.Add(time.Hour)
	prior := governanceFile("restart-first", 1, now.Add(-time.Minute), &sourceHash, &cleanupAfter, nil)
	if err := first.reserveFileRecord(&prior, now.Add(-time.Minute)); err != nil {
		t.Fatalf("reserve before restart: %v", err)
	}

	second, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("second NewServer: %v", err)
	}
	candidate := governanceFile("restart-second", 1, now, &sourceHash, &cleanupAfter, nil)
	assertReservationRejectedWithoutRow(t, second, &candidate, now, common.ErrRateLimit)
}

func TestAnonymousCleanupCannotEraseRollingRateEvidenceEarly(t *testing.T) {
	srv := governanceUnitServer(t)
	srv.cfg.FileUploadAnonPresignPerHour = 1
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	sourceHash := strings.Repeat("d", 64)
	capabilityExpiresAt := now.Add(fileCapabilityTTL)
	cleanupAfter := anonymousUploadCleanupAfter(now, capabilityExpiresAt, srv.cfg.FileUploadCleanupGrace)
	first := governanceFile("rate-cleanup-first", 1, now, &sourceHash, &cleanupAfter, nil)
	if err := srv.reserveFileRecord(&first, now); err != nil {
		t.Fatalf("reserve first file: %v", err)
	}

	beforeWindowEnd := now.Add(45 * time.Minute)
	summary, err := srv.runUploadCleanupBatch(context.Background(), beforeWindowEnd)
	if err != nil {
		t.Fatalf("cleanup before rate window end: %v", err)
	}
	if summary.Claimed != 0 || summary.Deleted != 0 {
		t.Fatalf("cleanup erased rolling-rate evidence early: %+v", summary)
	}
	blockedCleanupAfter := beforeWindowEnd.Add(time.Hour)
	blocked := governanceFile("rate-cleanup-blocked", 1, beforeWindowEnd, &sourceHash, &blockedCleanupAfter, nil)
	assertReservationRejectedWithoutRow(t, srv, &blocked, beforeWindowEnd, common.ErrRateLimit)

	windowEnd := now.Add(time.Hour)
	summary, err = srv.runUploadCleanupBatch(context.Background(), windowEnd)
	if err != nil {
		t.Fatalf("cleanup at rate window end: %v", err)
	}
	if summary.Claimed != 1 || summary.Deleted != 1 || summary.Failed != 0 {
		t.Fatalf("cleanup at rate window end = %+v", summary)
	}
	allowedCleanupAfter := windowEnd.Add(time.Hour)
	allowed := governanceFile("rate-cleanup-allowed", 1, windowEnd, &sourceHash, &allowedCleanupAfter, nil)
	if err := srv.reserveFileRecord(&allowed, windowEnd); err != nil {
		t.Fatalf("reserve after rolling window ended: %v", err)
	}
}

func TestQuotaTransactionRollsBackCallbackWrites(t *testing.T) {
	srv := governanceUnitServer(t)
	sentinel := errors.New("stop transaction")
	err := srv.withQuotaTransaction(func(tx *gorm.DB) error {
		row := governanceFile("rollback-row", 1, time.Now(), nil, nil, nil)
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("transaction error = %v", err)
	}
	var count int64
	if err := srv.DB.Model(&model.FileRecord{}).Where("object_key = ?", "rollback-row").Count(&count).Error; err != nil {
		t.Fatalf("count rolled-back row: %v", err)
	}
	if count != 0 {
		t.Fatalf("rollback left %d rows", count)
	}
}

func TestQuotaTransactionOptionsUseReadCommittedOnlyForMySQL(t *testing.T) {
	options := quotaTransactionOptions("mysql")
	if options == nil || options.Isolation != sql.LevelReadCommitted {
		t.Fatalf("MySQL transaction options = %+v", options)
	}
	if options := quotaTransactionOptions("sqlite"); options != nil {
		t.Fatalf("SQLite transaction options = %+v, want driver default", options)
	}
}

func TestQuotaTransactionErrorContracts(t *testing.T) {
	if common.CodeUploadQuotaExceeded != 10013 ||
		common.ErrUploadQuotaExceeded.Code != 10013 ||
		common.ErrUploadQuotaExceeded.HTTPStatus != http.StatusConflict ||
		common.ErrUploadQuotaExceeded.Message != "upload quota exceeded" {
		t.Fatalf("quota error contract = %+v", common.ErrUploadQuotaExceeded)
	}
	if common.ErrUploadTooLarge.Code != common.CodeInvalidUpload ||
		common.ErrUploadTooLarge.HTTPStatus != http.StatusRequestEntityTooLarge ||
		common.ErrUploadTooLarge.Message != "upload file too large" {
		t.Fatalf("upload-too-large contract = %+v", common.ErrUploadTooLarge)
	}
}

func governanceFile(
	objectKey string,
	size int64,
	createdAt time.Time,
	sourceHash *string,
	cleanupAfter *time.Time,
	ownerMerchantID *uint64,
) model.FileRecord {
	uploaderType := model.UserTypeAdmin
	if sourceHash != nil {
		uploaderType = model.UserTypePublic
	}
	return model.FileRecord{
		BizType:         model.FileBizOther,
		ObjectKey:       objectKey,
		MimeType:        "image/jpeg",
		SizeBytes:       size,
		UploaderType:    uploaderType,
		ScanStatus:      model.FileScanPending,
		OwnerMerchantID: ownerMerchantID,
		SourceIPHash:    sourceHash,
		CleanupAfter:    cleanupAfter,
		CreatedAt:       createdAt,
	}
}

func assertReservationRejectedWithoutRow(
	t *testing.T,
	srv *Server,
	candidate *model.FileRecord,
	now time.Time,
	wantErr error,
) {
	t.Helper()
	var before int64
	if err := srv.DB.Model(&model.FileRecord{}).Count(&before).Error; err != nil {
		t.Fatalf("count before reservation: %v", err)
	}
	err := srv.reserveFileRecord(candidate, now)
	if err != wantErr {
		t.Fatalf("reserve error = %v, want %v", err, wantErr)
	}
	var after int64
	if err := srv.DB.Model(&model.FileRecord{}).Count(&after).Error; err != nil {
		t.Fatalf("count after reservation: %v", err)
	}
	if after != before || candidate.ID != 0 {
		t.Fatalf("rejected reservation persisted: before=%d after=%d id=%d", before, after, candidate.ID)
	}
}
