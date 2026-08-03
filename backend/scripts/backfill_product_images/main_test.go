package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/media"
	"second-hand-market-backend/backend/internal/model"
)

type backfillFixture struct {
	db        *gorm.DB
	root      string
	fileID    uint64
	sourceKey string
	targetKey string
	output    []byte
	now       time.Time
	calls     int
}

func newBackfillFixture(t *testing.T) *backfillFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:backfill_"+time.Now().Format("150405.000000")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := app.MigrateSchema(db); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	root := t.TempDir()
	sourceKey := "product_image/F1.jpg"
	sourcePath, err := media.LocalObjectPath(root, sourceKey)
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	source := encodedBackfillJPEG(t)
	if err := media.PublishObjectNoReplace(sourcePath, source, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	file := model.FileRecord{
		BizType:      model.FileBizProductImage,
		ObjectKey:    sourceKey,
		URL:          "/uploads/" + sourceKey,
		MimeType:     "image/jpeg",
		SizeBytes:    int64(len(source)),
		UploaderType: model.UserTypeMerchant,
		ScanStatus:   model.FileScanPass,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file record: %v", err)
	}
	product := model.Product{
		ProductNo:      "PBACKFILL1",
		MerchantID:     1,
		Title:          "backfill",
		CategoryID:     1,
		PriceCent:      100,
		ConditionLevel: "GOOD",
		Stock:          1,
		CoverFileID:    &file.ID,
		Status:         model.ProductDraft,
		CreatedBy:      1,
		UpdatedBy:      1,
		Version:        1,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := db.Create(&model.ProductImage{ProductID: product.ID, FileID: file.ID, SortOrder: 1}).Error; err != nil {
		t.Fatalf("create product image: %v", err)
	}
	return &backfillFixture{
		db:        db,
		root:      root,
		fileID:    file.ID,
		sourceKey: sourceKey,
		targetKey: fmt.Sprintf("product_image/detail-v1/F%d.jpg", file.ID),
		output:    encodedBackfillJPEG(t),
		now:       time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
	}
}

func (fx *backfillFixture) deps() commandDependencies {
	return commandDependencies{
		now: func() time.Time { return fx.now },
		process: func(context.Context, []byte) (media.ProcessResult, error) {
			fx.calls++
			return media.ProcessResult{
				OutputMIME: "image/jpeg",
				OutputExt:  ".jpg",
				Content:    fx.output,
				Width:      4,
				Height:     3,
			}, nil
		},
	}
}

func TestDryRunDoesNotWriteObjectsRecordsOrLedger(t *testing.T) {
	fx := newBackfillFixture(t)

	got, err := run(context.Background(), fx.db, fx.root, commandOptions{DryRun: true, Limit: 10}, fx.deps())
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if got.Evaluated != 1 || got.Committed != 0 || fx.calls != 1 {
		t.Fatalf("dry-run summary=%+v calls=%d", got, fx.calls)
	}
	var record model.FileRecord
	if err := fx.db.First(&record, fx.fileID).Error; err != nil {
		t.Fatalf("load file record: %v", err)
	}
	if record.ObjectKey != fx.sourceKey {
		t.Fatalf("dry-run changed object key: %q", record.ObjectKey)
	}
	if countRows(t, fx.db, &model.ImageBackfillItem{}) != 0 {
		t.Fatal("dry-run wrote ledger item")
	}
	targetPath, err := media.LocalObjectPath(fx.root, fx.targetKey)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run created target object: %v", err)
	}
}

func TestEnsureRunCreatesLedgerRunByID(t *testing.T) {
	fx := newBackfillFixture(t)

	const runID = "IMGENSURE1"
	if err := ensureRun(fx.db, runID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := ensureRun(fx.db, runID); err != nil {
		t.Fatalf("ensure run again: %v", err)
	}

	var count int64
	if err := fx.db.Table("image_backfill_runs").Where("id = ?", runID).Count(&count).Error; err != nil {
		t.Fatalf("count run: %v", err)
	}
	if count != 1 {
		t.Fatalf("run count = %d, want 1", count)
	}
}

func TestDryRunWritesJSONLineWithPredictedTarget(t *testing.T) {
	fx := newBackfillFixture(t)
	var output bytes.Buffer
	deps := fx.deps()
	deps.output = &output
	deps.writeJSON = func(w io.Writer, v any) error {
		return json.NewEncoder(w).Encode(v)
	}

	got, err := runCore(context.Background(), fx.db, fx.root, commandOptions{DryRun: true, Limit: 10}, deps)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if got.Evaluated != 1 {
		t.Fatalf("dry-run summary=%+v", got)
	}
	for _, snippet := range []string{
		`"mode":"dry-run"`,
		fmt.Sprintf(`"file_id":%d`, fx.fileID),
		fmt.Sprintf(`"target_object_key":"%s"`, fx.targetKey),
		`"status":"WOULD_COMMIT"`,
	} {
		if !bytes.Contains(output.Bytes(), []byte(snippet)) {
			t.Fatalf("dry-run JSON Lines missing %s in %s", snippet, output.String())
		}
	}
}

func TestParseOptionsRejectsUnsafeModeCombinations(t *testing.T) {
	defaultOptions, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("default options: %v", err)
	}
	if !defaultOptions.DryRun {
		t.Fatalf("default mode should be dry-run: %+v", defaultOptions)
	}
	for _, args := range [][]string{
		{"--dry-run", "--apply"},
		{"--apply"},
		{"--cleanup", "--run-id", "IMG1", "--limit", "1"},
		{"--apply", "--run-id", "IMG1", "--workers", "2"},
		{"--cleanup", "--retry-failed", "--run-id", "IMG1"},
	} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("unsafe args accepted: %v", args)
		}
	}
}

func TestImageDeliveryPackaging(t *testing.T) {
	dockerfile, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	for _, snippet := range []string{
		"ENV GOPROXY=https://goproxy.cn,direct",
		"go build -o /out/migrate ./scripts/migrate",
		"go build -o /out/backfill-product-images ./scripts/backfill_product_images",
		"COPY --from=build /out/migrate /srv/migrate",
		"COPY --from=build /out/backfill-product-images /srv/backfill-product-images",
		"COPY backend/migrations /srv/migrations",
		"ca-certificates curl libheif1 libvips-tools",
	} {
		if !bytes.Contains(dockerfile, []byte(snippet)) {
			t.Fatalf("Dockerfile missing %q", snippet)
		}
	}

	productionEnv, err := os.ReadFile("../../configs/.env.production.mysql.example")
	if err != nil {
		t.Fatalf("read production env example: %v", err)
	}
	for _, snippet := range []string{
		"AUTO_MIGRATE=false",
		"SEED_DEFAULTS=false",
		"FILE_PUBLIC_BASE_URL=",
		"IMAGE_PROCESSOR_DRIVER=vips",
		"REQUIRE_DETAIL_V1_PRODUCT_IMAGES=false",
	} {
		if !bytes.Contains(productionEnv, []byte(snippet)) {
			t.Fatalf("production env missing %q", snippet)
		}
	}
}

func TestApplyStopsWhenAnotherRunHasUnresolvedItemForSameFile(t *testing.T) {
	fx := newBackfillFixture(t)
	if err := fx.db.Create(&model.ImageBackfillRun{ID: "IMGOTHER1", ProfileVersion: media.DetailProfileVersion}).Error; err != nil {
		t.Fatalf("create other run: %v", err)
	}
	if err := fx.db.Create(&model.ImageBackfillItem{
		RunID:           "IMGOTHER1",
		FileID:          fx.fileID,
		SourceObjectKey: fx.sourceKey,
		TargetObjectKey: fx.targetKey,
		ProfileVersion:  media.DetailProfileVersion,
		Status:          statusProcessing,
		Attempts:        1,
		CleanupStatus:   cleanupNotScheduled,
	}).Error; err != nil {
		t.Fatalf("create unresolved item: %v", err)
	}

	got, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: "IMGNEW1"}, fx.deps())
	if err == nil {
		t.Fatalf("apply should stop on unresolved item from another run, summary=%+v", got)
	}
	var record model.FileRecord
	if err := fx.db.First(&record, fx.fileID).Error; err != nil {
		t.Fatalf("load file record: %v", err)
	}
	if record.ObjectKey != fx.sourceKey {
		t.Fatalf("cross-run conflict changed object key: %q", record.ObjectKey)
	}
}

func TestApplyCommitsLedgerAndKeepsSourceUntilCleanup(t *testing.T) {
	fx := newBackfillFixture(t)

	got, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: "IMGTEST1", Limit: 10}, fx.deps())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got.Committed != 1 || got.PendingCleanup != 1 {
		t.Fatalf("apply summary=%+v", got)
	}
	var record model.FileRecord
	if err := fx.db.First(&record, fx.fileID).Error; err != nil {
		t.Fatalf("load file record: %v", err)
	}
	if record.ID != fx.fileID || record.ObjectKey != fx.targetKey || record.MimeType != "image/jpeg" {
		t.Fatalf("file record after apply = %+v", record)
	}
	sourcePath, _ := media.LocalObjectPath(fx.root, fx.sourceKey)
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("source must remain before cleanup: %v", err)
	}
	var item model.ImageBackfillItem
	if err := fx.db.Where("run_id = ? AND file_id = ?", "IMGTEST1", fx.fileID).First(&item).Error; err != nil {
		t.Fatalf("load ledger item: %v", err)
	}
	if item.Status != statusCommitted || item.CleanupStatus != cleanupPending || item.CleanupAfter == nil ||
		!item.CleanupAfter.Equal(fx.now.Add(24*time.Hour)) {
		t.Fatalf("ledger item after apply = %+v", item)
	}
}

func TestApplyFailsClosedOnSQLite(t *testing.T) {
	fx := newBackfillFixture(t)

	got, err := run(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: "IMGSQLITE1"}, fx.deps())
	if err == nil {
		t.Fatalf("sqlite apply should fail closed, summary=%+v", got)
	}
	var record model.FileRecord
	if err := fx.db.First(&record, fx.fileID).Error; err != nil {
		t.Fatalf("load file record: %v", err)
	}
	if record.ObjectKey != fx.sourceKey {
		t.Fatalf("sqlite apply changed object key: %q", record.ObjectKey)
	}
	if countRows(t, fx.db, &model.ImageBackfillItem{}) != 0 {
		t.Fatal("sqlite apply wrote ledger item")
	}
}

func TestApplyRecoversStagedItemWithSourceRecordWithoutReprocessing(t *testing.T) {
	fx := newBackfillFixture(t)
	runID := "IMGSTAGED1"
	sourceSHA, sourceSize := mustSHA256Object(t, fx.root, fx.sourceKey)
	outputSHA, outputSize := publishTargetObject(t, fx, fx.output)
	outputSizePtr := outputSize

	if err := fx.db.Create(&model.ImageBackfillRun{ID: runID, ProfileVersion: media.DetailProfileVersion}).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := fx.db.Create(&model.ImageBackfillItem{
		RunID:           runID,
		FileID:          fx.fileID,
		SourceObjectKey: fx.sourceKey,
		TargetObjectKey: fx.targetKey,
		ProfileVersion:  media.DetailProfileVersion,
		SourceSHA256:    &sourceSHA,
		OutputSHA256:    &outputSHA,
		SourceSizeBytes: sourceSize,
		OutputSizeBytes: &outputSizePtr,
		Status:          statusStaged,
		Attempts:        1,
		CleanupStatus:   cleanupNotScheduled,
	}).Error; err != nil {
		t.Fatalf("create staged item: %v", err)
	}
	deps := fx.deps()
	deps.process = func(context.Context, []byte) (media.ProcessResult, error) {
		t.Fatal("staged recovery must not reprocess the source image")
		return media.ProcessResult{}, nil
	}

	got, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: runID}, deps)
	if err != nil {
		t.Fatalf("recover staged item: %v", err)
	}
	if got.Committed != 1 || got.PendingCleanup != 1 {
		t.Fatalf("recovery summary=%+v", got)
	}
	var record model.FileRecord
	if err := fx.db.First(&record, fx.fileID).Error; err != nil {
		t.Fatalf("load file record: %v", err)
	}
	if record.ObjectKey != fx.targetKey || record.MimeType != "image/jpeg" || record.SizeBytes != outputSize {
		t.Fatalf("file record after staged recovery = %+v", record)
	}
	var item model.ImageBackfillItem
	if err := fx.db.Where("run_id = ? AND file_id = ?", runID, fx.fileID).First(&item).Error; err != nil {
		t.Fatalf("load item: %v", err)
	}
	if item.Status != statusCommitted || item.CleanupStatus != cleanupPending || item.CommittedAt == nil {
		t.Fatalf("ledger after staged recovery = %+v", item)
	}
}

func TestApplySkipsFailedItemsUnlessRetryFailedIsExplicit(t *testing.T) {
	fx := newBackfillFixture(t)
	runID := "IMGRETRY1"
	if err := fx.db.Create(&model.ImageBackfillRun{ID: runID, ProfileVersion: media.DetailProfileVersion}).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	errCode := "PROCESS_FAILED"
	if err := fx.db.Create(&model.ImageBackfillItem{
		RunID:           runID,
		FileID:          fx.fileID,
		SourceObjectKey: fx.sourceKey,
		TargetObjectKey: fx.targetKey,
		ProfileVersion:  media.DetailProfileVersion,
		Status:          statusFailed,
		Attempts:        2,
		ErrorCode:       &errCode,
		CleanupStatus:   cleanupNotScheduled,
	}).Error; err != nil {
		t.Fatalf("create failed item: %v", err)
	}
	deps := fx.deps()
	deps.process = func(context.Context, []byte) (media.ProcessResult, error) {
		t.Fatal("failed item must not be retried without --retry-failed")
		return media.ProcessResult{}, nil
	}

	got, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: runID}, deps)
	if err != nil {
		t.Fatalf("apply without retry: %v", err)
	}
	if got.Evaluated != 0 || got.Committed != 0 {
		t.Fatalf("apply without retry summary=%+v", got)
	}

	got, err = runCore(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: runID, RetryFailed: true}, fx.deps())
	if err != nil {
		t.Fatalf("apply with retry: %v", err)
	}
	if got.Committed != 1 {
		t.Fatalf("apply with retry summary=%+v", got)
	}
	var item model.ImageBackfillItem
	if err := fx.db.Where("run_id = ? AND file_id = ?", runID, fx.fileID).First(&item).Error; err != nil {
		t.Fatalf("load retried item: %v", err)
	}
	if item.Status != statusCommitted || item.Attempts != 3 {
		t.Fatalf("retried item = %+v", item)
	}
}

func TestCleanupRemovesExpiredSourceObject(t *testing.T) {
	fx := newBackfillFixture(t)
	if _, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: "IMGTEST2", Limit: 10}, fx.deps()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Cleanup: true, RunID: "IMGTEST2"}, fx.deps())
	if err != nil {
		t.Fatalf("early cleanup: %v", err)
	}
	if got.CleanupDone != 0 {
		t.Fatalf("early cleanup summary=%+v", got)
	}
	fx.now = fx.now.Add(24*time.Hour + time.Second)
	got, err = runCore(context.Background(), fx.db, fx.root, commandOptions{Cleanup: true, RunID: "IMGTEST2"}, fx.deps())
	if err != nil {
		t.Fatalf("expired cleanup: %v", err)
	}
	if got.CleanupDone != 1 {
		t.Fatalf("expired cleanup summary=%+v", got)
	}
	sourcePath, _ := media.LocalObjectPath(fx.root, fx.sourceKey)
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source object should be removed after cleanup: %v", err)
	}
}

func TestCleanupRetriesFailedItems(t *testing.T) {
	fx := newBackfillFixture(t)
	runID := "IMGCLEANRETRY1"
	if _, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: runID, Limit: 10}, fx.deps()); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := fx.db.Model(&model.ImageBackfillItem{}).
		Where("run_id = ? AND file_id = ?", runID, fx.fileID).
		Updates(map[string]any{"cleanup_status": cleanupFailed, "cleanup_error_code": "SOURCE_DELETE_FAILED"}).Error; err != nil {
		t.Fatalf("mark cleanup failed: %v", err)
	}
	fx.now = fx.now.Add(24*time.Hour + time.Second)

	got, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Cleanup: true, RunID: runID}, fx.deps())
	if err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	if got.CleanupDone != 1 || got.CleanupFailed != 0 {
		t.Fatalf("retry cleanup summary=%+v", got)
	}
}

func TestCleanupKeepsSourceWhenTargetHashDoesNotMatchLedger(t *testing.T) {
	fx := newBackfillFixture(t)
	runID := "IMGCLEANHASH1"
	if _, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: runID, Limit: 10}, fx.deps()); err != nil {
		t.Fatalf("apply: %v", err)
	}
	targetPath, err := media.LocalObjectPath(fx.root, fx.targetKey)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("corrupted target"), 0o600); err != nil {
		t.Fatalf("corrupt target: %v", err)
	}
	fx.now = fx.now.Add(24*time.Hour + time.Second)

	got, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Cleanup: true, RunID: runID}, fx.deps())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if got.CleanupFailed != 1 || got.CleanupDone != 0 {
		t.Fatalf("cleanup summary=%+v", got)
	}
	sourcePath, _ := media.LocalObjectPath(fx.root, fx.sourceKey)
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("source should remain when target hash mismatches: %v", err)
	}
}

func TestCleanupDeletedFileRecordRemovesSourceAndTargetWhenUnreferenced(t *testing.T) {
	fx := newBackfillFixture(t)
	runID := "IMGCLEANDELETED1"
	if _, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Apply: true, RunID: runID, Limit: 10}, fx.deps()); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := fx.db.Where("file_id = ?", fx.fileID).Delete(&model.ProductImage{}).Error; err != nil {
		t.Fatalf("delete product images: %v", err)
	}
	if err := fx.db.Where("cover_file_id = ?", fx.fileID).Delete(&model.Product{}).Error; err != nil {
		t.Fatalf("delete products: %v", err)
	}
	if err := fx.db.Delete(&model.FileRecord{}, fx.fileID).Error; err != nil {
		t.Fatalf("delete file record: %v", err)
	}
	fx.now = fx.now.Add(24*time.Hour + time.Second)

	got, err := runCore(context.Background(), fx.db, fx.root, commandOptions{Cleanup: true, RunID: runID}, fx.deps())
	if err != nil {
		t.Fatalf("cleanup deleted record: %v", err)
	}
	if got.CleanupDone != 1 || got.CleanupFailed != 0 {
		t.Fatalf("cleanup deleted record summary=%+v", got)
	}
	for _, key := range []string{fx.sourceKey, fx.targetKey} {
		path, _ := media.LocalObjectPath(fx.root, key)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("object %q should be removed, stat err=%v", key, err)
		}
	}
}

func countRows(t *testing.T, db *gorm.DB, modelValue any) int64 {
	t.Helper()
	var count int64
	if err := db.Model(modelValue).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func publishTargetObject(t *testing.T, fx *backfillFixture, content []byte) (string, int64) {
	t.Helper()
	targetPath, err := media.LocalObjectPath(fx.root, fx.targetKey)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if err := media.PublishObjectNoReplace(targetPath, content, 0o600); err != nil {
		t.Fatalf("publish target: %v", err)
	}
	sha, size, err := media.SHA256File(targetPath)
	if err != nil {
		t.Fatalf("hash target: %v", err)
	}
	return sha, size
}

func mustSHA256Object(t *testing.T, root, objectKey string) (string, int64) {
	t.Helper()
	path, err := media.LocalObjectPath(root, objectKey)
	if err != nil {
		t.Fatalf("resolve object %q: %v", objectKey, err)
	}
	sha, size, err := media.SHA256File(path)
	if err != nil {
		t.Fatalf("hash object %q: %v", objectKey, err)
	}
	return sha, size
}

func encodedBackfillJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, color.NRGBA{R: 20, G: uint8(40 + y*20), B: uint8(80 + x*10), A: 255})
		}
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode JPEG: %v", err)
	}
	return output.Bytes()
}
