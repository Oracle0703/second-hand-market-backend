package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

var cleanupTestSequence atomic.Uint64

func newUploadCleanupTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := securityTestConfig(t)
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("new cleanup test server: %v", err)
	}
	return srv
}

func createCleanupTestFile(
	t *testing.T,
	srv *Server,
	cleanupAfter *time.Time,
	mutate func(*model.FileRecord),
) model.FileRecord {
	t.Helper()
	sequence := cleanupTestSequence.Add(1)
	sourceHash := strings.Repeat("a", 64)
	file := model.FileRecord{
		BizType:         model.FileBizMerchantLicense,
		ObjectKey:       fmt.Sprintf("merchant_license/cleanup-%d.jpg", sequence),
		MimeType:        "image/jpeg",
		SizeBytes:       32,
		UploaderType:    model.UserTypePublic,
		ScanStatus:      model.FileScanPending,
		SourceIPHash:    &sourceHash,
		CleanupAfter:    cleanupAfter,
		CleanupAttempts: 0,
		CreatedAt:       time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC).Add(time.Duration(sequence) * time.Second),
	}
	if mutate != nil {
		mutate(&file)
	}
	if err := srv.DB.Create(&file).Error; err != nil {
		t.Fatalf("create cleanup test file: %v", err)
	}
	return file
}

func writeCleanupTestFile(t *testing.T, srv *Server, objectKey string, content []byte) string {
	t.Helper()
	path := filepath.Join(srv.cfg.FileUploadLocalDir, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create cleanup fixture directory: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write cleanup fixture: %v", err)
	}
	return path
}

func requireCleanupFileMissing(t *testing.T, db *gorm.DB, id uint64) {
	t.Helper()
	var file model.FileRecord
	if err := db.First(&file, id).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("file record %d still exists or lookup failed: %v", id, err)
	}
}

func TestUploadCleanupNeverClaimsHistoricalAuthenticatedOrBoundFiles(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Hour)
	ownerID := uint64(71)
	files := []model.FileRecord{
		createCleanupTestFile(t, srv, nil, nil),
		createCleanupTestFile(t, srv, &expired, func(file *model.FileRecord) {
			file.UploaderType = model.UserTypeMerchant
			file.SourceIPHash = nil
		}),
		createCleanupTestFile(t, srv, &expired, func(file *model.FileRecord) {
			file.OwnerMerchantID = &ownerID
		}),
	}
	contents := make(map[uint64][]byte, len(files))
	paths := make(map[uint64]string, len(files))
	for _, file := range files {
		content := []byte(fmt.Sprintf("protected-file-%d", file.ID))
		contents[file.ID] = content
		paths[file.ID] = writeCleanupTestFile(t, srv, file.ObjectKey, content)
	}

	summary, err := srv.runUploadCleanupBatch(context.Background(), now)
	if err != nil {
		t.Fatalf("run cleanup batch: %v", err)
	}
	if summary.Claimed != 0 || summary.Deleted != 0 || summary.Failed != 0 {
		t.Fatalf("protected rows were selected: %+v", summary)
	}
	for _, want := range files {
		var got model.FileRecord
		if err := srv.DB.First(&got, want.ID).Error; err != nil {
			t.Fatalf("reload protected row %d: %v", want.ID, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("protected row %d changed:\n got=%+v\nwant=%+v", want.ID, got, want)
		}
		content, err := os.ReadFile(paths[want.ID])
		if err != nil {
			t.Fatalf("read protected file %d: %v", want.ID, err)
		}
		if !bytes.Equal(content, contents[want.ID]) {
			t.Fatalf("protected file %d changed", want.ID)
		}
	}
}

func TestUploadCleanupClaimsOnlyExpiredGraceInBoundedOrder(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	srv.cfg.FileUploadCleanupBatchSize = 2
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	firstExpiry := now.Add(-3 * time.Minute)
	secondExpiry := now.Add(-2 * time.Minute)
	thirdExpiry := now.Add(-time.Minute)
	futureExpiry := now.Add(time.Minute)
	first := createCleanupTestFile(t, srv, &firstExpiry, nil)
	second := createCleanupTestFile(t, srv, &secondExpiry, nil)
	third := createCleanupTestFile(t, srv, &thirdExpiry, nil)
	future := createCleanupTestFile(t, srv, &futureExpiry, nil)

	summary, err := srv.runUploadCleanupBatch(context.Background(), now)
	if err != nil {
		t.Fatalf("run cleanup batch: %v", err)
	}
	if summary.Claimed != 2 || summary.Deleted != 2 || summary.Failed != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	requireCleanupFileMissing(t, srv.DB, first.ID)
	requireCleanupFileMissing(t, srv.DB, second.ID)
	for _, remaining := range []model.FileRecord{third, future} {
		var got model.FileRecord
		if err := srv.DB.First(&got, remaining.ID).Error; err != nil {
			t.Fatalf("reload unclaimed row %d: %v", remaining.ID, err)
		}
		if got.CleanupClaimToken != nil || got.CleanupClaimedAt != nil || got.CleanupAttempts != 0 {
			t.Fatalf("unclaimed row %d changed: %+v", remaining.ID, got)
		}
	}
}

func TestUploadCleanupDeletesManagedFileAndRow(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	file := createCleanupTestFile(t, srv, &expired, nil)
	path := writeCleanupTestFile(t, srv, file.ObjectKey, []byte("expired anonymous upload"))

	summary, err := srv.runUploadCleanupBatch(context.Background(), now)
	if err != nil {
		t.Fatalf("run cleanup batch: %v", err)
	}
	if summary.Claimed != 1 || summary.Deleted != 1 || summary.Failed != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("managed file still exists: %v", err)
	}
	requireCleanupFileMissing(t, srv.DB, file.ID)
}

func TestUploadCleanupTreatsMissingPhysicalFileAsSuccess(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	file := createCleanupTestFile(t, srv, &expired, nil)

	summary, err := srv.runUploadCleanupBatch(context.Background(), now)
	if err != nil {
		t.Fatalf("run cleanup batch: %v", err)
	}
	if summary.Claimed != 1 || summary.Deleted != 1 || summary.Failed != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	requireCleanupFileMissing(t, srv.DB, file.ID)
}

func TestUploadCleanupReleasesClaimAndRetriesAfterDeleteFailure(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	file := createCleanupTestFile(t, srv, &expired, nil)
	path := filepath.Join(srv.cfg.FileUploadLocalDir, filepath.FromSlash(file.ObjectKey))
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create invalid directory target: %v", err)
	}

	first, err := srv.runUploadCleanupBatch(context.Background(), now)
	if err != nil {
		t.Fatalf("first cleanup batch: %v", err)
	}
	if first.Claimed != 1 || first.Deleted != 0 || first.Failed != 1 || first.FailureCategories["unsafe_path"] != 1 {
		t.Fatalf("first summary = %+v", first)
	}
	var retry model.FileRecord
	if err := srv.DB.First(&retry, file.ID).Error; err != nil {
		t.Fatalf("reload retry row: %v", err)
	}
	if retry.CleanupClaimToken != nil || retry.CleanupClaimedAt != nil || retry.CleanupAttempts != 1 {
		t.Fatalf("failed cleanup did not release claim: %+v", retry)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove invalid target: %v", err)
	}
	writeCleanupTestFile(t, srv, file.ObjectKey, []byte("retry succeeds"))

	second, err := srv.runUploadCleanupBatch(context.Background(), now.Add(time.Second))
	if err != nil {
		t.Fatalf("second cleanup batch: %v", err)
	}
	if second.Claimed != 1 || second.Deleted != 1 || second.Failed != 0 {
		t.Fatalf("second summary = %+v", second)
	}
	requireCleanupFileMissing(t, srv.DB, file.ID)
}

func TestUploadCleanupReclaimsOnlyStaleClaims(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Hour)
	freshClaimedAt := now.Add(-srv.cfg.FileUploadCleanupClaimTTL).Add(time.Second)
	staleClaimedAt := now.Add(-srv.cfg.FileUploadCleanupClaimTTL)
	freshToken := strings.Repeat("b", 64)
	staleToken := strings.Repeat("c", 64)
	fresh := createCleanupTestFile(t, srv, &expired, func(file *model.FileRecord) {
		file.CleanupClaimedAt = &freshClaimedAt
		file.CleanupClaimToken = &freshToken
		file.CleanupAttempts = 1
	})
	stale := createCleanupTestFile(t, srv, &expired, func(file *model.FileRecord) {
		file.CleanupClaimedAt = &staleClaimedAt
		file.CleanupClaimToken = &staleToken
		file.CleanupAttempts = 1
	})

	summary, err := srv.runUploadCleanupBatch(context.Background(), now)
	if err != nil {
		t.Fatalf("run cleanup batch: %v", err)
	}
	if summary.Claimed != 1 || summary.Deleted != 1 || summary.Failed != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	requireCleanupFileMissing(t, srv.DB, stale.ID)
	var got model.FileRecord
	if err := srv.DB.First(&got, fresh.ID).Error; err != nil {
		t.Fatalf("reload fresh claim: %v", err)
	}
	if got.CleanupClaimToken == nil || *got.CleanupClaimToken != freshToken || got.CleanupAttempts != 1 {
		t.Fatalf("fresh claim changed: %+v", got)
	}
}

func TestUploadCleanupClaimTokenIsRandomLowercaseHex(t *testing.T) {
	first, err := newUploadCleanupClaimToken()
	if err != nil {
		t.Fatalf("first claim token: %v", err)
	}
	second, err := newUploadCleanupClaimToken()
	if err != nil {
		t.Fatalf("second claim token: %v", err)
	}
	for name, token := range map[string]string{"first": first, "second": second} {
		if len(token) != 64 || strings.Trim(token, "0123456789abcdef") != "" {
			t.Fatalf("%s claim token is not 64 lowercase hex characters", name)
		}
	}
	if first == second {
		t.Fatal("independent cleanup batches reused a claim token")
	}
}

func TestUploadCleanupRejectsParentSymlinkEscape(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.jpg")
	outsideContent := []byte("must survive cleanup")
	if err := os.WriteFile(outsidePath, outsideContent, 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	linkPath := filepath.Join(srv.cfg.FileUploadLocalDir, "merchant_license")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	file := createCleanupTestFile(t, srv, &expired, func(file *model.FileRecord) {
		file.ObjectKey = "merchant_license/outside.jpg"
	})

	summary, err := srv.runUploadCleanupBatch(context.Background(), now)
	if err != nil {
		t.Fatalf("run cleanup batch: %v", err)
	}
	if summary.Claimed != 1 || summary.Deleted != 0 || summary.Failed != 1 || summary.FailureCategories["unsafe_path"] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	content, err := os.ReadFile(outsidePath)
	if err != nil || !bytes.Equal(content, outsideContent) {
		t.Fatalf("outside file changed: content=%q err=%v", content, err)
	}
	var got model.FileRecord
	if err := srv.DB.First(&got, file.ID).Error; err != nil {
		t.Fatalf("escaped row was deleted: %v", err)
	}
	if got.CleanupClaimToken != nil || got.CleanupClaimedAt != nil {
		t.Fatalf("escaped row claim was not released: %+v", got)
	}
}

func TestUploadCleanupFailsClosedForUnsupportedProvider(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	srv.cfg.FileStorageProvider = "object-storage"
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	file := createCleanupTestFile(t, srv, &expired, nil)

	summary, err := srv.runUploadCleanupBatch(context.Background(), now)
	if err != nil {
		t.Fatalf("run cleanup batch: %v", err)
	}
	if summary.Claimed != 1 || summary.Deleted != 0 || summary.Failed != 1 || summary.FailureCategories["unsupported_provider"] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	var got model.FileRecord
	if err := srv.DB.First(&got, file.ID).Error; err != nil {
		t.Fatalf("unsupported-provider row was deleted: %v", err)
	}
	if got.CleanupClaimToken != nil || got.CleanupClaimedAt != nil || got.CleanupAttempts != 1 {
		t.Fatalf("unsupported-provider claim state = %+v", got)
	}
}

func TestUploadCleanupCannotDeleteFileClaimedByRegistration(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	file := createCleanupTestFile(t, srv, &expired, nil)
	path := writeCleanupTestFile(t, srv, file.ObjectKey, []byte("bound license"))
	claimed, err := srv.claimUploadCleanupCandidates(context.Background(), now)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim candidates = %d, err=%v", len(claimed), err)
	}
	ownerID := uint64(88)
	if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", file.ID).Updates(map[string]interface{}{
		"owner_merchant_id":     ownerID,
		"capability_token_hash": nil,
		"capability_expires_at": nil,
	}).Error; err != nil {
		t.Fatalf("simulate completed registration: %v", err)
	}

	if err := srv.processUploadCleanupClaim(context.Background(), claimed[0]); err == nil {
		t.Fatal("expected cleanup processing to abandon a newly bound row")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "bound license" {
		t.Fatalf("bound file changed: content=%q err=%v", content, err)
	}
	var got model.FileRecord
	if err := srv.DB.First(&got, file.ID).Error; err != nil {
		t.Fatalf("bound row was deleted: %v", err)
	}
	if got.OwnerMerchantID == nil || *got.OwnerMerchantID != ownerID {
		t.Fatalf("bound owner changed: %+v", got)
	}
}

func TestUploadCleanupLogsOnlySanitizedSummary(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	srv.cfg.FileStorageProvider = "unsupported-sensitive-provider"
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Hour)
	staleClaimedAt := now.Add(-srv.cfg.FileUploadCleanupClaimTTL)
	sourceHash := "source-ip-hash-sensitive-marker"
	capabilityHash := "capability-sensitive-marker"
	claimToken := "cleanup-claim-sensitive-marker"
	createCleanupTestFile(t, srv, &expired, func(file *model.FileRecord) {
		file.ObjectKey = "merchant_license/raw-ip-192.0.2.44-sensitive-marker.jpg"
		file.URL = "/uploads/private-url-sensitive-marker"
		file.SourceIPHash = &sourceHash
		file.CapabilityTokenHash = &capabilityHash
		file.CleanupClaimedAt = &staleClaimedAt
		file.CleanupClaimToken = &claimToken
	})
	var logs []string
	srv.cleanupLogf = func(format string, args ...interface{}) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}

	summary, err := srv.runUploadCleanupBatch(context.Background(), now)
	if err != nil {
		t.Fatalf("run cleanup batch: %v", err)
	}
	if summary.FailureCategories["unsupported_provider"] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	joined := strings.Join(logs, "\n")
	for _, secret := range []string{
		"192.0.2.44",
		"source-ip-hash-sensitive-marker",
		"capability-sensitive-marker",
		"cleanup-claim-sensitive-marker",
		"raw-ip-192.0.2.44-sensitive-marker.jpg",
		"private-url-sensitive-marker",
		srv.cfg.FileUploadIPHashSecret,
		srv.cfg.FileUploadLocalDir,
	} {
		if strings.Contains(joined, secret) {
			t.Fatalf("cleanup log leaked %q: %q", secret, joined)
		}
	}
	if !strings.Contains(joined, "claimed=1") || !strings.Contains(joined, "failed=1") ||
		!strings.Contains(joined, "unsupported_provider=1") {
		t.Fatalf("cleanup log lacks sanitized summary: %q", joined)
	}
}

func TestUploadCleanupLoopStopsWhenContextCancelled(t *testing.T) {
	srv := newUploadCleanupTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		srv.runUploadCleanupLoop(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup loop did not stop after cancellation")
	}
}
