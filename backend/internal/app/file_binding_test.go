package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newFileBindingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open file binding test db: %v", err)
	}
	if err := db.AutoMigrate(&model.FileRecord{}, &model.FileQuotaGuard{}); err != nil {
		t.Fatalf("migrate file binding test db: %v", err)
	}
	if err := db.Create(&model.FileQuotaGuard{ID: 1, GuardName: "file_records"}).Error; err != nil {
		t.Fatalf("seed file quota guard: %v", err)
	}
	return db
}

func newFileBindingTestServer(t *testing.T, db *gorm.DB) *Server {
	t.Helper()
	return &Server{cfg: securityTestConfig(t), DB: db}
}

func createBindingFile(
	t *testing.T,
	db *gorm.DB,
	bizType string,
	scanStatus string,
	url string,
	ownerMerchantID *uint64,
) model.FileRecord {
	t.Helper()
	file := model.FileRecord{
		BizType:         bizType,
		ObjectKey:       fmt.Sprintf("test/%d", time.Now().UnixNano()),
		URL:             url,
		MimeType:        "image/jpeg",
		SizeBytes:       10,
		UploaderType:    model.UserTypeMerchant,
		ScanStatus:      scanStatus,
		OwnerMerchantID: ownerMerchantID,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create binding file: %v", err)
	}
	return file
}

func TestValidateMerchantFilesForBinding(t *testing.T) {
	db := newFileBindingTestDB(t)
	merchantID := uint64(10)
	otherMerchantID := uint64(20)
	valid := createBindingFile(t, db, model.FileBizProductImage, model.FileScanPass, "/uploads/ok.jpg", &merchantID)
	foreign := createBindingFile(t, db, model.FileBizProductImage, model.FileScanPass, "/uploads/foreign.jpg", &otherMerchantID)
	wrongType := createBindingFile(t, db, model.FileBizMerchantLicense, model.FileScanPass, "/uploads/license.jpg", &merchantID)
	pending := createBindingFile(t, db, model.FileBizProductImage, model.FileScanPending, "", &merchantID)
	blocked := createBindingFile(t, db, model.FileBizProductImage, model.FileScanBlocked, "/uploads/blocked.jpg", &merchantID)
	emptyURL := createBindingFile(t, db, model.FileBizProductImage, model.FileScanPass, "  ", &merchantID)
	unowned := createBindingFile(t, db, model.FileBizProductImage, model.FileScanPass, "/uploads/unowned.jpg", nil)

	tests := []struct {
		name string
		ids  []uint64
		want error
	}{
		{name: "valid", ids: []uint64{valid.ID}},
		{name: "missing", ids: []uint64{999999}, want: common.ErrInvalidFileBinding},
		{name: "duplicate", ids: []uint64{valid.ID, valid.ID}, want: common.ErrInvalidFileBinding},
		{name: "foreign", ids: []uint64{foreign.ID}, want: common.ErrInvalidFileBinding},
		{name: "wrong type", ids: []uint64{wrongType.ID}, want: common.ErrInvalidFileBinding},
		{name: "pending", ids: []uint64{pending.ID}, want: common.ErrInvalidFileBinding},
		{name: "blocked", ids: []uint64{blocked.ID}, want: common.ErrInvalidFileBinding},
		{name: "empty URL", ids: []uint64{emptyURL.ID}, want: common.ErrInvalidFileBinding},
		{name: "unowned", ids: []uint64{unowned.ID}, want: common.ErrInvalidFileBinding},
		{name: "zero ID", ids: []uint64{0}, want: common.ErrInvalidFileBinding},
		{name: "empty list", ids: nil, want: common.ErrInvalidFileBinding},
		{name: "mixed valid and foreign", ids: []uint64{valid.ID, foreign.ID}, want: common.ErrInvalidFileBinding},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Transaction(func(tx *gorm.DB) error {
				return validateMerchantFilesForBinding(tx, merchantID, tt.ids, model.FileBizProductImage)
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v want %v", err, tt.want)
			}
		})
	}
}

func TestValidateMerchantLicenseFilesForBindingUsesObjectKey(t *testing.T) {
	db := newFileBindingTestDB(t)
	merchantID := uint64(10)
	valid := createBindingFile(t, db, model.FileBizMerchantLicense, model.FileScanPass, "", &merchantID)
	emptyObjectKey := createBindingFile(t, db, model.FileBizMerchantLicense, model.FileScanPass, "", &merchantID)
	if err := db.Model(&emptyObjectKey).Update("object_key", "").Error; err != nil {
		t.Fatalf("clear license object key: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return validateMerchantFilesForBinding(tx, merchantID, []uint64{valid.ID}, model.FileBizMerchantLicense)
	}); err != nil {
		t.Fatalf("private license binding = %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return validateMerchantFilesForBinding(tx, merchantID, []uint64{emptyObjectKey.ID}, model.FileBizMerchantLicense)
	}); !errors.Is(err, common.ErrInvalidFileBinding) {
		t.Fatalf("empty object key binding = %v", err)
	}
}

func createClaimableLicense(t *testing.T, db *gorm.DB, now time.Time, rawToken string) model.FileRecord {
	t.Helper()
	hash := fileCapabilityHash(rawToken)
	expires := now.Add(15 * time.Minute)
	file := model.FileRecord{
		BizType:             model.FileBizMerchantLicense,
		ObjectKey:           fmt.Sprintf("merchant_license/%d.jpg", time.Now().UnixNano()),
		URL:                 "",
		MimeType:            "image/jpeg",
		SizeBytes:           10,
		UploaderType:        model.UserTypePublic,
		ScanStatus:          model.FileScanPass,
		CapabilityTokenHash: &hash,
		CapabilityExpiresAt: &expires,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create claimable license: %v", err)
	}
	return file
}

func TestClaimPublicMerchantLicenseConsumesTokenOnce(t *testing.T) {
	db := newFileBindingTestDB(t)
	srv := newFileBindingTestServer(t, db)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	raw := "test-only-public-license-token"
	file := createClaimableLicense(t, db, now, raw)

	if err := srv.withQuotaTransaction(func(tx *gorm.DB) error {
		return srv.claimPublicMerchantLicense(tx, file.ID, raw, 77, now)
	}); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := srv.withQuotaTransaction(func(tx *gorm.DB) error {
		return srv.claimPublicMerchantLicense(tx, file.ID, raw, 88, now)
	}); !errors.Is(err, common.ErrInvalidFileBinding) {
		t.Fatalf("second claim = %v", err)
	}

	var claimed model.FileRecord
	if err := db.First(&claimed, file.ID).Error; err != nil {
		t.Fatalf("reload claimed file: %v", err)
	}
	if claimed.OwnerMerchantID == nil || *claimed.OwnerMerchantID != 77 {
		t.Fatalf("owner = %v, want 77", claimed.OwnerMerchantID)
	}
	if claimed.CapabilityTokenHash != nil || claimed.CapabilityExpiresAt != nil {
		t.Fatalf("claim capability was not cleared: %+v", claimed)
	}
}

func TestClaimPublicMerchantLicenseRejectsInvalidClaims(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*model.FileRecord)
		raw    string
		fileID func(model.FileRecord) uint64
	}{
		{name: "missing file", raw: "valid-token", fileID: func(model.FileRecord) uint64 { return 999999 }},
		{name: "empty token", raw: ""},
		{name: "wrong token", raw: "wrong-token"},
		{name: "expired token", raw: "valid-token", mutate: func(file *model.FileRecord) {
			expired := now
			file.CapabilityExpiresAt = &expired
		}},
		{name: "wrong biz type", raw: "valid-token", mutate: func(file *model.FileRecord) {
			file.BizType = model.FileBizProductImage
		}},
		{name: "pending", raw: "valid-token", mutate: func(file *model.FileRecord) {
			file.ScanStatus = model.FileScanPending
		}},
		{name: "empty object key", raw: "valid-token", mutate: func(file *model.FileRecord) {
			file.ObjectKey = ""
		}},
		{name: "merchant uploader", raw: "valid-token", mutate: func(file *model.FileRecord) {
			file.UploaderType = model.UserTypeMerchant
		}},
		{name: "already owned", raw: "valid-token", mutate: func(file *model.FileRecord) {
			owner := uint64(9)
			file.OwnerMerchantID = &owner
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newFileBindingTestDB(t)
			srv := newFileBindingTestServer(t, db)
			file := createClaimableLicense(t, db, now, "valid-token")
			if tt.mutate != nil {
				tt.mutate(&file)
				if err := db.Save(&file).Error; err != nil {
					t.Fatalf("mutate file: %v", err)
				}
			}
			fileID := file.ID
			if tt.fileID != nil {
				fileID = tt.fileID(file)
			}
			err := srv.withQuotaTransaction(func(tx *gorm.DB) error {
				return srv.claimPublicMerchantLicense(tx, fileID, tt.raw, 77, now)
			})
			if !errors.Is(err, common.ErrInvalidFileBinding) {
				t.Fatalf("claim error = %v", err)
			}
		})
	}
}

func TestClaimPublicMerchantLicenseCanRetryAfterTransactionRollback(t *testing.T) {
	db := newFileBindingTestDB(t)
	srv := newFileBindingTestServer(t, db)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	raw := "rollback-retry-token"
	file := createClaimableLicense(t, db, now, raw)
	rollback := errors.New("force rollback")

	err := srv.withQuotaTransaction(func(tx *gorm.DB) error {
		if err := srv.claimPublicMerchantLicense(tx, file.ID, raw, 77, now); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction = %v", err)
	}

	if err := srv.withQuotaTransaction(func(tx *gorm.DB) error {
		return srv.claimPublicMerchantLicense(tx, file.ID, raw, 88, now)
	}); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}

func TestClaimPublicMerchantLicenseRejectsCleanupClaim(t *testing.T) {
	db := newFileBindingTestDB(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	raw := "cleanup-claimed-license-token"
	file := createClaimableLicense(t, db, now, raw)
	claimToken := "cleanup-claim-token"
	if err := db.Model(&model.FileRecord{}).Where("id = ?", file.ID).
		Update("cleanup_claim_token", claimToken).Error; err != nil {
		t.Fatalf("claim file for cleanup: %v", err)
	}
	srv := newFileBindingTestServer(t, db)
	err := srv.withQuotaTransaction(func(tx *gorm.DB) error {
		return srv.claimPublicMerchantLicense(tx, file.ID, raw, 77, now)
	})
	if !errors.Is(err, common.ErrInvalidFileBinding) {
		t.Fatalf("cleanup-claimed binding error = %v", err)
	}
	var after model.FileRecord
	if err := db.First(&after, file.ID).Error; err != nil {
		t.Fatalf("reload cleanup-claimed file: %v", err)
	}
	if after.OwnerMerchantID != nil || after.CapabilityTokenHash == nil ||
		after.CleanupClaimToken == nil || *after.CleanupClaimToken != claimToken {
		t.Fatalf("cleanup-claimed file mutated: %+v", after)
	}
}
