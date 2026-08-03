package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/media"
	"second-hand-market-backend/backend/internal/model"
)

const (
	statusPending    = "PENDING"
	statusProcessing = "PROCESSING"
	statusStaged     = "STAGED"
	statusCommitted  = "COMMITTED"
	statusFailed     = "FAILED"

	cleanupNotScheduled = "NOT_SCHEDULED"
	cleanupPending      = "PENDING"
	cleanupDone         = "DONE"
	cleanupFailed       = "FAILED"
)

type commandOptions struct {
	DryRun      bool
	Apply       bool
	Cleanup     bool
	RunID       string
	AfterID     uint64
	Limit       int
	RetryFailed bool
}

type commandDependencies struct {
	now       func() time.Time
	process   func(context.Context, []byte) (media.ProcessResult, error)
	output    io.Writer
	writeJSON func(io.Writer, any) error
}

type summary struct {
	Evaluated      int `json:"evaluated"`
	Committed      int `json:"committed"`
	Failed         int `json:"failed"`
	PendingCleanup int `json:"pending_cleanup"`
	CleanupDone    int `json:"cleanup_done"`
	CleanupFailed  int `json:"cleanup_failed"`
}

type candidateFile struct {
	ID        uint64
	ObjectKey string
	SizeBytes int64
}

type itemEvent struct {
	Mode            string `json:"mode"`
	FileID          uint64 `json:"file_id"`
	SourceObjectKey string `json:"source_object_key"`
	TargetObjectKey string `json:"target_object_key"`
	SourceSizeBytes int64  `json:"source_size_bytes,omitempty"`
	OutputSizeBytes int64  `json:"output_size_bytes,omitempty"`
	OutputWidth     int    `json:"output_width,omitempty"`
	OutputHeight    int    `json:"output_height,omitempty"`
	Status          string `json:"status"`
	ErrorCode       string `json:"error_code,omitempty"`
	ProfileVersion  string `json:"profile_version"`
}

func main() {
	options, err := parseOptions(os.Args[1:])
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	dbConfig, err := databasecmd.LoadConfig()
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	if (options.Apply || options.Cleanup) && dbConfig.Driver != "mysql" {
		log.Print("DB_DRIVER must be mysql for apply or cleanup")
		os.Exit(1)
	}
	db, err := databasecmd.OpenDatabase(dbConfig)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	defer databasecmd.CloseDatabase(db)

	root := strings.TrimSpace(os.Getenv("FILE_UPLOAD_LOCAL_DIR"))
	if root == "" {
		log.Print("FILE_UPLOAD_LOCAL_DIR is required")
		os.Exit(1)
	}
	processor := media.NewVipsCLIProcessor(os.Getenv("IMAGE_PROCESSOR_BIN"), media.DefaultUploadPolicy())
	deps := commandDependencies{
		now: time.Now,
		process: func(ctx context.Context, content []byte) (media.ProcessResult, error) {
			return processor.Process(ctx, media.ProcessRequest{
				InputMIME:     "image/jpeg",
				OutputProfile: media.DetailProfileVersion,
				Content:       content,
			})
		},
		output: os.Stdout,
		writeJSON: func(w io.Writer, v any) error {
			return json.NewEncoder(w).Encode(v)
		},
	}
	var got summary
	if options.Apply || options.Cleanup {
		err = db.Connection(func(conn *gorm.DB) error {
			locked, err := acquireGlobalBackfillLock(conn)
			if err != nil {
				return err
			}
			if !locked {
				return errors.New("another image backfill command is running")
			}
			defer func() {
				_ = releaseGlobalBackfillLock(conn)
			}()
			got, err = run(context.Background(), conn, root, options, deps)
			return err
		})
	} else {
		got, err = run(context.Background(), db, root, options, deps)
	}
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	if err := deps.writeJSON(deps.output, got); err != nil {
		log.Print("write summary failed")
		os.Exit(1)
	}
}

func acquireGlobalBackfillLock(db *gorm.DB) (bool, error) {
	var locked int
	if err := db.Raw("SELECT GET_LOCK(?, 0)", "image_delivery_backfill_global").Scan(&locked).Error; err != nil {
		return false, errors.New("BACKFILL_LOCK acquire failed")
	}
	return locked == 1, nil
}

func releaseGlobalBackfillLock(db *gorm.DB) error {
	var released int
	if err := db.Raw("SELECT RELEASE_LOCK(?)", "image_delivery_backfill_global").Scan(&released).Error; err != nil {
		return errors.New("BACKFILL_LOCK release failed")
	}
	return nil
}

func parseOptions(args []string) (commandOptions, error) {
	var options commandOptions
	flags := flag.NewFlagSet("backfill_product_images", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.DryRun, "dry-run", false, "")
	flags.BoolVar(&options.Apply, "apply", false, "")
	flags.BoolVar(&options.Cleanup, "cleanup", false, "")
	flags.StringVar(&options.RunID, "run-id", "", "")
	flags.Uint64Var(&options.AfterID, "after-id", 0, "")
	flags.IntVar(&options.Limit, "limit", 0, "")
	flags.BoolVar(&options.RetryFailed, "retry-failed", false, "")
	if err := flags.Parse(args); err != nil {
		return commandOptions{}, errors.New("BACKFILL_ARGUMENTS are invalid")
	}
	if flags.NArg() != 0 {
		return commandOptions{}, errors.New("BACKFILL_ARGUMENTS are invalid")
	}
	modeCount := 0
	for _, enabled := range []bool{options.DryRun, options.Apply, options.Cleanup} {
		if enabled {
			modeCount++
		}
	}
	if modeCount == 0 {
		options.DryRun = true
		modeCount = 1
	}
	if modeCount != 1 {
		return commandOptions{}, errors.New("choose exactly one of --dry-run, --apply, or --cleanup")
	}
	options.RunID = strings.TrimSpace(options.RunID)
	if (options.Apply || options.Cleanup) && options.RunID == "" {
		return commandOptions{}, errors.New("--run-id is required for apply or cleanup")
	}
	if options.RetryFailed && !options.Apply {
		return commandOptions{}, errors.New("--retry-failed requires --apply")
	}
	if options.Cleanup && (options.AfterID != 0 || options.Limit != 0) {
		return commandOptions{}, errors.New("--cleanup scans the selected run without --after-id or --limit")
	}
	if options.Limit < 0 {
		return commandOptions{}, errors.New("--limit must not be negative")
	}
	return options, nil
}

func run(ctx context.Context, db *gorm.DB, root string, options commandOptions, deps commandDependencies) (summary, error) {
	if err := validateMutationDriver(db, options); err != nil {
		return summary{}, err
	}
	return runCore(ctx, db, root, options, deps)
}

func runCore(ctx context.Context, db *gorm.DB, root string, options commandOptions, deps commandDependencies) (summary, error) {
	if deps.now == nil {
		deps.now = time.Now
	}
	if deps.writeJSON == nil {
		deps.writeJSON = func(io.Writer, any) error { return nil }
	}
	if deps.output == nil {
		deps.output = io.Discard
	}
	if err := validateRunOptions(options); err != nil {
		return summary{}, err
	}
	if options.Cleanup {
		return runCleanup(db, root, options, deps)
	}
	if options.Apply {
		return runApply(ctx, db, root, options, deps)
	}
	return runDryRun(ctx, db, root, options, deps)
}

func runDryRun(ctx context.Context, db *gorm.DB, root string, options commandOptions, deps commandDependencies) (summary, error) {
	candidates, err := loadCandidates(db, options)
	if err != nil {
		return summary{}, err
	}
	var got summary
	for _, file := range candidates {
		select {
		case <-ctx.Done():
			return got, common.ErrInternal
		default:
		}
		if !isReferencedProductImage(db, file.ID) {
			continue
		}
		if media.IsDetailProductImageKey(file.ObjectKey) {
			continue
		}
		sourcePath, err := media.LocalObjectPath(root, file.ObjectKey)
		if err != nil {
			got.Failed++
			_ = emitJSON(deps, dryRunEvent(file, media.ProcessResult{}, "FAILED", "INVALID_SOURCE_KEY", 0))
			continue
		}
		content, err := os.ReadFile(sourcePath)
		if err != nil || len(content) == 0 {
			got.Failed++
			_ = emitJSON(deps, dryRunEvent(file, media.ProcessResult{}, "FAILED", "SOURCE_READ_FAILED", 0))
			continue
		}
		processed, err := deps.process(ctx, content)
		got.Evaluated++
		if err != nil {
			got.Failed++
			_ = emitJSON(deps, dryRunEvent(file, media.ProcessResult{}, "FAILED", "PROCESS_FAILED", int64(len(content))))
			continue
		}
		if _, _, err := media.ValidateDetailJPEG(media.DefaultDetailImagePolicy(), processed.Content); err != nil ||
			processed.OutputMIME != "image/jpeg" || strings.ToLower(strings.TrimSpace(processed.OutputExt)) != ".jpg" {
			got.Failed++
			_ = emitJSON(deps, dryRunEvent(file, processed, "FAILED", "INVALID_OUTPUT", int64(len(content))))
			continue
		}
		_ = emitJSON(deps, dryRunEvent(file, processed, "WOULD_COMMIT", "", int64(len(content))))
	}
	return got, nil
}

func runApply(ctx context.Context, db *gorm.DB, root string, options commandOptions, deps commandDependencies) (summary, error) {
	if err := ensureRun(db, options.RunID); err != nil {
		return summary{}, err
	}
	got, err := recoverApplyItems(db, root, options, deps.now())
	if err != nil {
		return got, err
	}
	candidates, err := loadCandidates(db, options)
	if err != nil {
		return got, err
	}
	for _, file := range candidates {
		select {
		case <-ctx.Done():
			return got, common.ErrInternal
		default:
		}
		if !isReferencedProductImage(db, file.ID) {
			continue
		}
		if media.IsDetailProductImageKey(file.ObjectKey) {
			continue
		}
		if err := ensureNoUnresolvedOtherRunItem(db, options.RunID, file.ID); err != nil {
			return got, err
		}
		item, hasItem, err := loadBackfillItem(db, options.RunID, file.ID)
		if err != nil {
			return got, err
		}
		if hasItem {
			switch item.Status {
			case statusCommitted:
				continue
			case statusStaged, statusProcessing:
				itemSummary, err := recoverApplyItem(db, root, item, deps.now())
				got.add(itemSummary)
				if err != nil {
					return got, err
				}
				continue
			case statusFailed:
				if !options.RetryFailed {
					continue
				}
			}
		}

		sourcePath, err := media.LocalObjectPath(root, file.ObjectKey)
		if err != nil {
			got.Failed++
			continue
		}
		content, err := os.ReadFile(sourcePath)
		if err != nil || len(content) == 0 {
			got.Failed++
			continue
		}
		if err := beginProcessingItem(db, options.RunID, file); err != nil {
			got.Failed++
			continue
		}
		processed, err := deps.process(ctx, content)
		got.Evaluated++
		if err != nil {
			got.Failed++
			_ = markFailed(db, options.RunID, file, "PROCESS_FAILED")
			continue
		}
		if _, _, err := media.ValidateDetailJPEG(media.DefaultDetailImagePolicy(), processed.Content); err != nil ||
			processed.OutputMIME != "image/jpeg" || strings.ToLower(strings.TrimSpace(processed.OutputExt)) != ".jpg" {
			got.Failed++
			_ = markFailed(db, options.RunID, file, "INVALID_OUTPUT")
			continue
		}
		committed, err := applyCandidate(db, root, file, processed.Content, options, deps.now())
		if err != nil {
			got.Failed++
			continue
		}
		if committed {
			got.Committed++
			got.PendingCleanup++
		}
	}
	return got, nil
}

func emitJSON(deps commandDependencies, value any) error {
	if deps.writeJSON == nil {
		return nil
	}
	if deps.output == nil {
		deps.output = io.Discard
	}
	return deps.writeJSON(deps.output, value)
}

func dryRunEvent(file candidateFile, processed media.ProcessResult, status, errorCode string, sourceSize int64) itemEvent {
	return itemEvent{
		Mode:            "dry-run",
		FileID:          file.ID,
		SourceObjectKey: file.ObjectKey,
		TargetObjectKey: fmt.Sprintf("product_image/detail-v1/F%d.jpg", file.ID),
		SourceSizeBytes: sourceSize,
		OutputSizeBytes: int64(len(processed.Content)),
		OutputWidth:     processed.Width,
		OutputHeight:    processed.Height,
		Status:          status,
		ErrorCode:       errorCode,
		ProfileVersion:  media.DetailProfileVersion,
	}
}

func validateMutationDriver(db *gorm.DB, options commandOptions) error {
	if !options.Apply && !options.Cleanup {
		return nil
	}
	if db == nil || db.Dialector == nil || db.Dialector.Name() != "mysql" {
		return errors.New("DB_DRIVER must be mysql for apply or cleanup")
	}
	return nil
}

func ensureNoUnresolvedOtherRunItem(db *gorm.DB, runID string, fileID uint64) error {
	var count int64
	if err := db.Model(&model.ImageBackfillItem{}).
		Where("file_id = ? AND profile_version = ? AND run_id <> ? AND status <> ?", fileID, media.DetailProfileVersion, runID, statusCommitted).
		Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return errors.New("unresolved image backfill item exists in another run")
	}
	return nil
}

func validateRunOptions(options commandOptions) error {
	modeCount := 0
	for _, enabled := range []bool{options.DryRun, options.Apply, options.Cleanup} {
		if enabled {
			modeCount++
		}
	}
	if modeCount != 1 {
		return errors.New("choose exactly one mode")
	}
	if (options.Apply || options.Cleanup) && strings.TrimSpace(options.RunID) == "" {
		return errors.New("run-id is required")
	}
	return nil
}

func loadCandidates(db *gorm.DB, options commandOptions) ([]candidateFile, error) {
	query := db.Model(&model.FileRecord{}).
		Select("id, object_key, size_bytes").
		Where("biz_type = ? AND scan_status = ?", model.FileBizProductImage, model.FileScanPass).
		Where("object_key NOT LIKE ?", "product_image/detail-v1/%")
	if options.AfterID > 0 {
		query = query.Where("id > ?", options.AfterID)
	}
	if options.Limit > 0 {
		query = query.Limit(options.Limit)
	}
	var files []candidateFile
	if err := query.Order("id ASC").Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

func isReferencedProductImage(db *gorm.DB, fileID uint64) bool {
	var imageCount int64
	_ = db.Model(&model.ProductImage{}).Where("file_id = ?", fileID).Count(&imageCount).Error
	if imageCount > 0 {
		return true
	}
	var coverCount int64
	_ = db.Model(&model.Product{}).Where("cover_file_id = ? AND deleted_at IS NULL", fileID).Count(&coverCount).Error
	return coverCount > 0
}

func applyCandidate(db *gorm.DB, root string, file candidateFile, output []byte, options commandOptions, now time.Time) (bool, error) {
	targetKey := fmt.Sprintf("product_image/detail-v1/F%d.jpg", file.ID)
	targetPath, err := media.LocalObjectPath(root, targetKey)
	if err != nil {
		return false, err
	}
	if err := media.PublishObjectNoReplace(targetPath, output, 0o600); err != nil {
		_ = markFailed(db, options.RunID, file, "TARGET_CONFLICT")
		return false, err
	}
	sourcePath, err := media.LocalObjectPath(root, file.ObjectKey)
	if err != nil {
		return false, err
	}
	sourceSHA, sourceSize, err := media.SHA256File(sourcePath)
	if err != nil {
		return false, err
	}
	outputSHA := sha256Hex(output)
	outputSize := int64(len(output))
	outputSizePtr := outputSize
	if err := db.Model(&model.ImageBackfillItem{}).
		Where("run_id = ? AND file_id = ?", options.RunID, file.ID).
		Updates(map[string]any{
			"source_object_key": file.ObjectKey,
			"target_object_key": targetKey,
			"profile_version":   media.DetailProfileVersion,
			"source_sha256":     sourceSHA,
			"output_sha256":     outputSHA,
			"source_size_bytes": sourceSize,
			"output_size_bytes": outputSizePtr,
			"status":            statusStaged,
			"error_code":        nil,
			"cleanup_status":    cleanupNotScheduled,
		}).Error; err != nil {
		return false, err
	}
	item, ok, err := loadBackfillItem(db, options.RunID, file.ID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, errors.New("backfill item missing after stage")
	}
	return commitStagedItem(db, root, item, now)
}

func ensureRun(db *gorm.DB, runID string) error {
	return db.Table("image_backfill_runs").
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).
		Create(map[string]any{
			"id":              runID,
			"profile_version": media.DetailProfileVersion,
			"created_at":      time.Now(),
		}).Error
}

func loadBackfillItem(db *gorm.DB, runID string, fileID uint64) (model.ImageBackfillItem, bool, error) {
	var item model.ImageBackfillItem
	err := db.Where("run_id = ? AND file_id = ?", runID, fileID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ImageBackfillItem{}, false, nil
	}
	if err != nil {
		return model.ImageBackfillItem{}, false, err
	}
	return item, true, nil
}

func beginProcessingItem(db *gorm.DB, runID string, file candidateFile) error {
	item := model.ImageBackfillItem{
		RunID:           runID,
		FileID:          file.ID,
		SourceObjectKey: file.ObjectKey,
		TargetObjectKey: fmt.Sprintf("product_image/detail-v1/F%d.jpg", file.ID),
		ProfileVersion:  media.DetailProfileVersion,
		Status:          statusPending,
		CleanupStatus:   cleanupNotScheduled,
	}
	if err := db.Where("run_id = ? AND file_id = ?", runID, file.ID).FirstOrCreate(&item).Error; err != nil {
		return err
	}
	return db.Model(&model.ImageBackfillItem{}).
		Where("run_id = ? AND file_id = ?", runID, file.ID).
		Updates(map[string]any{
			"source_object_key": file.ObjectKey,
			"target_object_key": fmt.Sprintf("product_image/detail-v1/F%d.jpg", file.ID),
			"profile_version":   media.DetailProfileVersion,
			"status":            statusProcessing,
			"attempts":          gorm.Expr("attempts + 1"),
			"error_code":        nil,
			"cleanup_status":    cleanupNotScheduled,
		}).Error
}

func recoverApplyItems(db *gorm.DB, root string, options commandOptions, now time.Time) (summary, error) {
	var items []model.ImageBackfillItem
	if err := db.Where("run_id = ? AND status IN ?", options.RunID, []string{statusProcessing, statusStaged}).
		Order("file_id ASC").Find(&items).Error; err != nil {
		return summary{}, err
	}
	var got summary
	for _, item := range items {
		itemSummary, err := recoverApplyItem(db, root, item, now)
		got.add(itemSummary)
		if err != nil {
			return got, err
		}
	}
	return got, nil
}

func recoverApplyItem(db *gorm.DB, root string, item model.ImageBackfillItem, now time.Time) (summary, error) {
	if item.Status == statusProcessing {
		targetMatches, err := objectHashMatches(root, item.TargetObjectKey, item.OutputSHA256)
		if err != nil {
			_ = markItemFailed(db, item, "TARGET_HASH_MISMATCH")
			return summary{Failed: 1}, nil
		}
		if !targetMatches {
			if err := db.Model(&model.ImageBackfillItem{}).Where("id = ?", item.ID).
				Updates(map[string]any{"status": statusPending}).Error; err != nil {
				return summary{}, err
			}
			return summary{}, nil
		}
		if err := db.Model(&model.ImageBackfillItem{}).Where("id = ?", item.ID).
			Updates(map[string]any{"status": statusStaged}).Error; err != nil {
			return summary{}, err
		}
		item.Status = statusStaged
	}
	committed, err := commitStagedItem(db, root, item, now)
	if err != nil {
		_ = markItemFailed(db, item, "STAGED_RECOVERY_FAILED")
		return summary{Failed: 1}, nil
	}
	if committed {
		return summary{Committed: 1, PendingCleanup: 1}, nil
	}
	return summary{}, nil
}

func commitStagedItem(db *gorm.DB, root string, item model.ImageBackfillItem, now time.Time) (bool, error) {
	targetSize, err := requireObjectHash(root, item.TargetObjectKey, item.OutputSHA256)
	if err != nil {
		return false, err
	}

	var record model.FileRecord
	err = db.First(&record, item.FileID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, errors.New("file record missing before commit")
	}
	if err != nil {
		return false, err
	}

	switch record.ObjectKey {
	case item.SourceObjectKey:
		if _, err := requireObjectHashWithSize(root, item.SourceObjectKey, item.SourceSHA256); err != nil {
			return false, err
		}
		cleanupAfter := now.Add(24 * time.Hour)
		return true, db.Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&model.FileRecord{}).
				Where("id = ? AND object_key = ? AND scan_status = ?", item.FileID, item.SourceObjectKey, model.FileScanPass).
				Updates(map[string]any{
					"object_key": item.TargetObjectKey,
					"url":        "/uploads/" + item.TargetObjectKey,
					"mime_type":  "image/jpeg",
					"size_bytes": targetSize,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("file record conditional update failed")
			}
			return tx.Model(&model.ImageBackfillItem{}).
				Where("id = ?", item.ID).
				Updates(committedItemUpdates(targetSize, now, cleanupAfter)).Error
		})
	case item.TargetObjectKey:
		if record.MimeType != "image/jpeg" || record.SizeBytes != targetSize || record.URL != "/uploads/"+item.TargetObjectKey {
			return false, errors.New("target file record metadata mismatch")
		}
		cleanupAfter := now.Add(24 * time.Hour)
		return true, db.Transaction(func(tx *gorm.DB) error {
			return tx.Model(&model.ImageBackfillItem{}).
				Where("id = ?", item.ID).
				Updates(committedItemUpdates(targetSize, now, cleanupAfter)).Error
		})
	default:
		return false, errors.New("file record object key changed during backfill")
	}
}

func committedItemUpdates(outputSize int64, now, cleanupAfter time.Time) map[string]any {
	return map[string]any{
		"output_size_bytes":  outputSize,
		"status":             statusCommitted,
		"error_code":         nil,
		"committed_at":       now,
		"cleanup_after":      cleanupAfter,
		"cleanup_status":     cleanupPending,
		"cleanup_error_code": nil,
	}
}

func objectHashMatches(root, objectKey string, expected *string) (bool, error) {
	path, err := media.LocalObjectPath(root, objectKey)
	if err != nil {
		return false, err
	}
	actual, _, err := media.SHA256File(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if expected == nil || strings.TrimSpace(*expected) == "" {
		return false, errors.New("missing expected hash")
	}
	return actual == *expected, nil
}

func requireObjectHash(root, objectKey string, expected *string) (int64, error) {
	size, err := requireObjectHashWithSize(root, objectKey, expected)
	return size, err
}

func requireObjectHashWithSize(root, objectKey string, expected *string) (int64, error) {
	path, err := media.LocalObjectPath(root, objectKey)
	if err != nil {
		return 0, err
	}
	actual, size, err := media.SHA256File(path)
	if err != nil {
		return 0, err
	}
	if expected == nil || strings.TrimSpace(*expected) == "" || actual != *expected {
		return 0, errors.New("object hash mismatch")
	}
	return size, nil
}

func markItemFailed(db *gorm.DB, item model.ImageBackfillItem, code string) error {
	return db.Model(&model.ImageBackfillItem{}).
		Where("id = ?", item.ID).
		Updates(map[string]any{"status": statusFailed, "error_code": code}).Error
}

func (s *summary) add(other summary) {
	s.Evaluated += other.Evaluated
	s.Committed += other.Committed
	s.Failed += other.Failed
	s.PendingCleanup += other.PendingCleanup
	s.CleanupDone += other.CleanupDone
	s.CleanupFailed += other.CleanupFailed
}

func markFailed(db *gorm.DB, runID string, file candidateFile, code string) error {
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	item := model.ImageBackfillItem{
		RunID:           runID,
		FileID:          file.ID,
		SourceObjectKey: file.ObjectKey,
		TargetObjectKey: fmt.Sprintf("product_image/detail-v1/F%d.jpg", file.ID),
		ProfileVersion:  media.DetailProfileVersion,
		Status:          statusFailed,
		CleanupStatus:   cleanupNotScheduled,
	}
	if err := db.Where("run_id = ? AND file_id = ?", runID, file.ID).FirstOrCreate(&item).Error; err != nil {
		return err
	}
	return db.Model(&model.ImageBackfillItem{}).
		Where("run_id = ? AND file_id = ?", runID, file.ID).
		Updates(map[string]any{"status": statusFailed, "error_code": code}).Error
}

func runCleanup(db *gorm.DB, root string, options commandOptions, deps commandDependencies) (summary, error) {
	var items []model.ImageBackfillItem
	if err := db.Where(
		"run_id = ? AND status = ? AND cleanup_status IN ? AND cleanup_after <= ?",
		options.RunID,
		statusCommitted,
		[]string{cleanupPending, cleanupFailed},
		deps.now(),
	).Order("file_id ASC").Find(&items).Error; err != nil {
		return summary{}, err
	}
	var got summary
	for _, item := range items {
		done, code, err := cleanupCommittedItem(db, root, item)
		if err != nil {
			return got, err
		}
		if !done {
			got.CleanupFailed++
			_ = updateCleanupStatus(db, item, cleanupFailed, code)
			continue
		}
		if err := updateCleanupStatus(db, item, cleanupDone, ""); err != nil {
			got.CleanupFailed++
			return got, err
		}
		got.CleanupDone++
	}
	return got, nil
}

func cleanupCommittedItem(db *gorm.DB, root string, item model.ImageBackfillItem) (bool, string, error) {
	if strings.TrimSpace(item.SourceObjectKey) == "" ||
		strings.TrimSpace(item.TargetObjectKey) == "" ||
		item.SourceObjectKey == item.TargetObjectKey {
		return false, "INVALID_KEYS", nil
	}

	var record model.FileRecord
	err := db.First(&record, item.FileID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		imageRefs, coverRefs, err := countFileReferences(db, item.FileID)
		if err != nil {
			return false, "", err
		}
		if imageRefs != 0 || coverRefs != 0 {
			return false, "FILE_RECORD_DELETED_WITH_REFERENCES", nil
		}
		if err := removeObjectIfExistsAndMatches(root, item.SourceObjectKey, item.SourceSHA256); err != nil {
			return false, "SOURCE_HASH_MISMATCH", nil
		}
		if err := removeObjectIfExistsAndMatches(root, item.TargetObjectKey, item.OutputSHA256); err != nil {
			return false, "TARGET_HASH_MISMATCH", nil
		}
		return true, "", nil
	}
	if err != nil {
		return false, "", err
	}

	switch record.ObjectKey {
	case item.TargetObjectKey:
		if _, err := requireObjectHashWithSize(root, item.TargetObjectKey, item.OutputSHA256); err != nil {
			return false, "TARGET_HASH_MISMATCH", nil
		}
		var sourceRecords int64
		if err := db.Model(&model.FileRecord{}).Where("object_key = ?", item.SourceObjectKey).Count(&sourceRecords).Error; err != nil {
			return false, "", err
		}
		if sourceRecords != 0 {
			return false, "SOURCE_STILL_REFERENCED", nil
		}
		if err := removeObjectIfExistsAndMatches(root, item.SourceObjectKey, item.SourceSHA256); err != nil {
			return false, "SOURCE_HASH_MISMATCH", nil
		}
		return true, "", nil
	case item.SourceObjectKey:
		return false, "SOURCE_STILL_REFERENCED", nil
	default:
		return false, "FILE_RECORD_OBJECT_CHANGED", nil
	}
}

func countFileReferences(db *gorm.DB, fileID uint64) (int64, int64, error) {
	var imageRefs int64
	if err := db.Model(&model.ProductImage{}).Where("file_id = ?", fileID).Count(&imageRefs).Error; err != nil {
		return 0, 0, err
	}
	var coverRefs int64
	if err := db.Model(&model.Product{}).Where("cover_file_id = ?", fileID).Count(&coverRefs).Error; err != nil {
		return 0, 0, err
	}
	return imageRefs, coverRefs, nil
}

func removeObjectIfExistsAndMatches(root, objectKey string, expected *string) error {
	path, err := media.LocalObjectPath(root, objectKey)
	if err != nil {
		return err
	}
	actual, _, err := media.SHA256File(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if expected == nil || strings.TrimSpace(*expected) == "" || actual != *expected {
		return errors.New("object hash mismatch")
	}
	return media.RemoveLocalObject(root, objectKey)
}

func updateCleanupStatus(db *gorm.DB, item model.ImageBackfillItem, status, code string) error {
	updates := map[string]any{"cleanup_status": status}
	if code == "" {
		updates["cleanup_error_code"] = nil
	} else {
		updates["cleanup_error_code"] = code
	}
	return db.Model(&model.ImageBackfillItem{}).Where("id = ?", item.ID).Updates(updates).Error
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
