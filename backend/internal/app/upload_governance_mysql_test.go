package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	mysqlcfg "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestUploadGovernanceMySQLConcurrencyAndCleanup(t *testing.T) {
	if os.Getenv("UPLOAD_GOVERNANCE_MYSQL_TEST") != "1" {
		t.Skip("set UPLOAD_GOVERNANCE_MYSQL_TEST=1 only in the isolated upload governance project")
	}
	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	parsed, err := mysqlcfg.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "second_hand_market_acceptance" {
		t.Fatal("DB_DSN must target the isolated mysql:3306 acceptance database")
	}

	prefix := "acceptance/f06/" + mysqlAcceptanceNonce(t)
	uploadDir := t.TempDir()
	autoMigrate := strings.EqualFold(os.Getenv("AUTO_MIGRATE"), "true")
	first := newMySQLUploadGovernanceServer(t, dsn, uploadDir, autoMigrate)
	second := newMySQLUploadGovernanceServer(t, dsn, uploadDir, autoMigrate)
	for _, srv := range []*Server{first, second} {
		srv.cleanupLogf = func(string, ...interface{}) {}
	}
	t.Cleanup(func() {
		first.DB.Where("object_key LIKE ?", prefix+"/%").Delete(&model.FileRecord{})
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	var baselineBytes int64
	if baselineBytes, err = sumFileBytes(first.DB.Model(&model.FileRecord{})); err != nil {
		t.Fatal("load isolated baseline byte count")
	}

	t.Run("serialized anonymous and global quota", func(t *testing.T) {
		configureMySQLAcceptanceLimits(first, baselineBytes+5, 5, 1, 5)
		configureMySQLAcceptanceLimits(second, baselineBytes+5, 5, 1, 5)
		sourceHash := strings.Repeat("a", 64)
		cleanupAfter := now.Add(time.Hour)
		results := reserveConcurrently(t, first, second, func(index int) model.FileRecord {
			return governanceFile(
				prefix+"/anonymous-"+string(rune('a'+index)),
				5,
				now,
				&sourceHash,
				&cleanupAfter,
				nil,
			)
		}, now)
		requireOneReservationWinner(t, results, common.ErrRateLimit, common.ErrUploadQuotaExceeded)
		requireObjectKeyPrefixCount(t, first.DB, prefix+"/anonymous-%", 1)
		if err := first.DB.Where("object_key LIKE ?", prefix+"/anonymous-%").Delete(&model.FileRecord{}).Error; err != nil {
			t.Fatal("remove isolated anonymous quota fixtures")
		}
	})

	t.Run("serialized merchant and global quota", func(t *testing.T) {
		configureMySQLAcceptanceLimits(first, baselineBytes+7, 7, 10, 70)
		configureMySQLAcceptanceLimits(second, baselineBytes+7, 7, 10, 70)
		merchantID := uint64(900000001)
		results := reserveConcurrently(t, first, second, func(index int) model.FileRecord {
			return governanceFile(
				prefix+"/merchant-"+string(rune('a'+index)),
				7,
				now.Add(time.Second),
				nil,
				nil,
				&merchantID,
			)
		}, now.Add(time.Second))
		requireOneReservationWinner(t, results, common.ErrUploadQuotaExceeded)
		requireObjectKeyPrefixCount(t, first.DB, prefix+"/merchant-%", 1)
		if err := first.DB.Where("object_key LIKE ?", prefix+"/merchant-%").Delete(&model.FileRecord{}).Error; err != nil {
			t.Fatal("remove isolated merchant quota fixtures")
		}
	})

	configureMySQLAcceptanceLimits(first, baselineBytes+1024, 1024, 20, 1024)
	configureMySQLAcceptanceLimits(second, baselineBytes+1024, 1024, 20, 1024)
	first.cfg.FileUploadCleanupBatchSize = 1
	second.cfg.FileUploadCleanupBatchSize = 1

	t.Run("registration and cleanup never produce a deleted bound file", func(t *testing.T) {
		rawToken, tokenHash, expiresAt, tokenErr := newFileCapability(now)
		if tokenErr != nil {
			t.Fatal("create isolated capability")
		}
		sourceHash := strings.Repeat("b", 64)
		expiredCleanup := now.Add(-time.Minute)
		file := governanceFile(prefix+"/binding-race", 4, now.Add(-time.Hour), &sourceHash, &expiredCleanup, nil)
		file.BizType = model.FileBizMerchantLicense
		file.ScanStatus = model.FileScanPass
		file.CapabilityTokenHash = &tokenHash
		file.CapabilityExpiresAt = &expiresAt
		if err := first.DB.Create(&file).Error; err != nil {
			t.Fatal("create isolated binding race row")
		}
		path := writeMySQLCleanupFixture(t, uploadDir, file.ObjectKey)

		var bindErr error
		var cleanup uploadCleanupSummary
		var cleanupErr error
		ready := sync.WaitGroup{}
		ready.Add(2)
		start := make(chan struct{})
		done := sync.WaitGroup{}
		done.Add(2)
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			bindErr = first.withQuotaTransaction(func(tx *gorm.DB) error {
				return first.claimPublicMerchantLicense(tx, file.ID, rawToken, 900000002, now)
			})
		}()
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			cleanup, cleanupErr = second.runUploadCleanupBatch(context.Background(), now)
		}()
		ready.Wait()
		close(start)
		done.Wait()
		if cleanupErr != nil {
			t.Fatal("run isolated binding race cleanup")
		}

		var current model.FileRecord
		lookupErr := first.DB.First(&current, file.ID).Error
		switch {
		case bindErr == nil:
			if lookupErr != nil || current.OwnerMerchantID == nil || *current.OwnerMerchantID != 900000002 {
				t.Fatal("successful binding did not preserve the owned row")
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("cleanup deleted a successfully bound physical file")
			}
			if cleanup.Deleted != 0 {
				t.Fatal("cleanup reported deletion after binding won")
			}
		case errors.Is(bindErr, common.ErrInvalidFileBinding):
			if !errors.Is(lookupErr, gorm.ErrRecordNotFound) || cleanup.Claimed != 1 || cleanup.Deleted != 1 || cleanup.Failed != 0 {
				t.Fatal("cleanup winner left an ambiguous binding race state")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("cleanup winner left its isolated physical file")
			}
		default:
			t.Fatal("binding race returned an unexpected error category")
		}
		_ = os.Remove(path)
		if err := first.DB.Delete(&model.FileRecord{}, file.ID).Error; err != nil {
			t.Fatal("remove isolated binding race row")
		}
	})

	t.Run("two cleanup claimers process one candidate at most once", func(t *testing.T) {
		sourceHash := strings.Repeat("c", 64)
		expiredCleanup := now.Add(-time.Minute)
		file := governanceFile(prefix+"/double-cleanup", 3, now.Add(-time.Hour), &sourceHash, &expiredCleanup, nil)
		if err := first.DB.Create(&file).Error; err != nil {
			t.Fatal("create isolated double-cleanup row")
		}
		path := writeMySQLCleanupFixture(t, uploadDir, file.ObjectKey)
		summaries, errs := cleanupConcurrently(first, second, now)
		if errs[0] != nil || errs[1] != nil {
			t.Fatal("concurrent isolated cleanup returned an error")
		}
		if summaries[0].Claimed+summaries[1].Claimed != 1 || summaries[0].Deleted+summaries[1].Deleted != 1 ||
			summaries[0].Failed+summaries[1].Failed != 0 {
			t.Fatal("concurrent cleanup did not process exactly one claim")
		}
		if err := first.DB.First(&model.FileRecord{}, file.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal("double-cleanup candidate remains")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("double-cleanup physical file remains")
		}
	})

	t.Run("stale claim and missing physical file are retried idempotently", func(t *testing.T) {
		sourceHash := strings.Repeat("d", 64)
		expiredCleanup := now.Add(-time.Hour)
		staleClaimedAt := now.Add(-first.cfg.FileUploadCleanupClaimTTL)
		staleToken := strings.Repeat("e", 64)
		file := governanceFile(prefix+"/stale-missing", 2, now.Add(-2*time.Hour), &sourceHash, &expiredCleanup, nil)
		file.CleanupClaimedAt = &staleClaimedAt
		file.CleanupClaimToken = &staleToken
		file.CleanupAttempts = 1
		if err := first.DB.Create(&file).Error; err != nil {
			t.Fatal("create isolated stale claim row")
		}
		summary, cleanupErr := first.runUploadCleanupBatch(context.Background(), now)
		if cleanupErr != nil || summary.Claimed != 1 || summary.Deleted != 1 || summary.Failed != 0 {
			t.Fatal("stale missing-file cleanup did not complete idempotently")
		}
		if err := first.DB.First(&model.FileRecord{}, file.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal("stale missing-file row remains")
		}
	})

	t.Run("unsupported provider fails closed", func(t *testing.T) {
		sourceHash := strings.Repeat("f", 64)
		expiredCleanup := now.Add(-time.Minute)
		file := governanceFile(prefix+"/unsupported-provider", 2, now.Add(-time.Hour), &sourceHash, &expiredCleanup, nil)
		if err := first.DB.Create(&file).Error; err != nil {
			t.Fatal("create isolated unsupported-provider row")
		}
		unsupported := *first
		unsupported.cfg.FileStorageProvider = "unsupported"
		summary, cleanupErr := unsupported.runUploadCleanupBatch(context.Background(), now)
		if cleanupErr != nil || summary.Claimed != 1 || summary.Deleted != 0 || summary.Failed != 1 ||
			summary.FailureCategories["unsupported_provider"] != 1 {
			t.Fatal("unsupported provider did not fail closed")
		}
		var current model.FileRecord
		if err := first.DB.First(&current, file.ID).Error; err != nil || current.CleanupAttempts != 1 ||
			current.CleanupClaimedAt != nil || current.CleanupClaimToken != nil {
			t.Fatal("unsupported provider changed row ownership or retained its claim")
		}
		if err := first.DB.Delete(&model.FileRecord{}, file.ID).Error; err != nil {
			t.Fatal("remove isolated unsupported-provider row")
		}
	})

	t.Run("historical null governance row remains unchanged", func(t *testing.T) {
		historical := governanceFile(prefix+"/historical-null", 1, now.Add(-24*time.Hour), nil, nil, nil)
		historical.UploaderType = model.UserTypePublic
		if err := first.DB.Create(&historical).Error; err != nil {
			t.Fatal("create isolated historical row")
		}
		var before model.FileRecord
		if err := first.DB.First(&before, historical.ID).Error; err != nil {
			t.Fatal("load isolated historical row before cleanup")
		}
		if _, err := first.runUploadCleanupBatch(context.Background(), now); err != nil {
			t.Fatal("run cleanup with isolated historical row")
		}
		var after model.FileRecord
		if err := first.DB.First(&after, historical.ID).Error; err != nil || !reflect.DeepEqual(after, before) {
			t.Fatal("historical null-governance row changed")
		}
	})
}

type mysqlReservationResult struct {
	err error
}

func newMySQLUploadGovernanceServer(t *testing.T, dsn, uploadDir string, autoMigrate bool) *Server {
	t.Helper()
	cfg := Config{
		AppEnv:                       "test",
		Addr:                         ":0",
		DBDriver:                     "mysql",
		DBDSN:                        dsn,
		JWTAccessSecret:              "isolated-upload-governance-access",
		JWTRefreshSecret:             "isolated-upload-governance-refresh",
		AccessTTL:                    time.Hour,
		RefreshTTL:                   24 * time.Hour,
		AutoMigrate:                  autoMigrate,
		FileStorageProvider:          "local",
		FileUploadLocalDir:           uploadDir,
		FileUploadMaxBytes:           10 * 1024 * 1024,
		FileUploadMultipartMaxBytes:  11 * 1024 * 1024,
		FileUploadIPHashSecret:       "isolated-upload-governance-hmac-secret",
		FileUploadAnonPresignPerHour: 20,
		FileUploadAnonActiveFiles:    5,
		FileUploadAnonActiveBytes:    50 * 1024 * 1024,
		FileUploadMerchantQuotaBytes: 2 * 1024 * 1024 * 1024,
		FileUploadGlobalQuotaBytes:   20 * 1024 * 1024 * 1024,
		FileUploadCleanupInterval:    5 * time.Minute,
		FileUploadCleanupBatchSize:   1,
		FileUploadCleanupClaimTTL:    10 * time.Minute,
		FileUploadCleanupGrace:       30 * time.Minute,
		ImageCompressTargetBytes:     512,
		ImageProcessorDriver:         "passthrough",
		BuyerWechatLoginMode:         "mock",
		BuyerDouyinLoginMode:         "mock",
		BuyerWechatHTTPTimeout:       5 * time.Second,
		BuyerDouyinHTTPTimeout:       5 * time.Second,
	}
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatal("start isolated upload governance server")
	}
	sqlDB, err := srv.DB.DB()
	if err != nil {
		t.Fatal("open isolated upload governance pool")
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return srv
}

func configureMySQLAcceptanceLimits(srv *Server, global, merchant, anonFiles, anonBytes int64) {
	srv.cfg.FileUploadGlobalQuotaBytes = global
	srv.cfg.FileUploadMerchantQuotaBytes = merchant
	srv.cfg.FileUploadAnonPresignPerHour = anonFiles
	srv.cfg.FileUploadAnonActiveFiles = anonFiles
	srv.cfg.FileUploadAnonActiveBytes = anonBytes
}

func reserveConcurrently(
	t *testing.T,
	first, second *Server,
	file func(int) model.FileRecord,
	now time.Time,
) []mysqlReservationResult {
	t.Helper()
	servers := []*Server{first, second}
	results := make([]mysqlReservationResult, len(servers))
	ready := sync.WaitGroup{}
	ready.Add(len(servers))
	start := make(chan struct{})
	done := sync.WaitGroup{}
	done.Add(len(servers))
	for i, srv := range servers {
		go func(index int, current *Server) {
			defer done.Done()
			candidate := file(index)
			ready.Done()
			<-start
			results[index].err = current.reserveFileRecord(&candidate, now)
		}(i, srv)
	}
	ready.Wait()
	close(start)
	done.Wait()
	return results
}

func requireOneReservationWinner(t *testing.T, results []mysqlReservationResult, allowedErrors ...error) {
	t.Helper()
	winners := 0
	rejections := 0
	for _, result := range results {
		if result.err == nil {
			winners++
			continue
		}
		for _, allowed := range allowedErrors {
			if errors.Is(result.err, allowed) {
				rejections++
				break
			}
		}
	}
	if winners != 1 || rejections != 1 {
		t.Fatal("concurrent quota reservation did not produce one committed winner and one quota rejection")
	}
}

func requireObjectKeyPrefixCount(t *testing.T, db *gorm.DB, pattern string, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.FileRecord{}).Where("object_key LIKE ?", pattern).Count(&count).Error; err != nil || count != want {
		t.Fatal("isolated fixture row count differs from the serialized result")
	}
}

func cleanupConcurrently(first, second *Server, now time.Time) ([2]uploadCleanupSummary, [2]error) {
	servers := [2]*Server{first, second}
	var summaries [2]uploadCleanupSummary
	var errs [2]error
	ready := sync.WaitGroup{}
	ready.Add(len(servers))
	start := make(chan struct{})
	done := sync.WaitGroup{}
	done.Add(len(servers))
	for i, srv := range servers {
		go func(index int, current *Server) {
			defer done.Done()
			ready.Done()
			<-start
			summaries[index], errs[index] = current.runUploadCleanupBatch(context.Background(), now)
		}(i, srv)
	}
	ready.Wait()
	close(start)
	done.Wait()
	return summaries, errs
}

func writeMySQLCleanupFixture(t *testing.T, uploadDir, objectKey string) string {
	t.Helper()
	path := filepath.Join(uploadDir, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal("create isolated cleanup fixture directory")
	}
	if err := os.WriteFile(path, []byte("isolated-upload-governance-fixture"), 0o600); err != nil {
		t.Fatal("write isolated cleanup fixture")
	}
	return path
}

func mysqlAcceptanceNonce(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal("create isolated fixture nonce")
	}
	return hex.EncodeToString(raw)
}
