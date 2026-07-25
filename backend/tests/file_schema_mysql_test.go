package tests

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
)

func TestFileFlowWithMigrationOnlyMySQL(t *testing.T) {
	if os.Getenv("FILE_SCHEMA_MYSQL_TEST") != "1" {
		t.Skip("set FILE_SCHEMA_MYSQL_TEST=1 only in the isolated MySQL acceptance project")
	}
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Fatal("DB_DSN is required for isolated file schema acceptance")
	}

	newConfig := func(autoMigrate bool) app.Config {
		return app.Config{
			AppEnv:                   "test",
			Addr:                     ":0",
			DBDriver:                 "mysql",
			DBDSN:                    dsn,
			JWTAccessSecret:          "file-schema-test-access",
			JWTRefreshSecret:         "file-schema-test-refresh",
			AccessTTL:                time.Hour,
			RefreshTTL:               24 * time.Hour,
			AutoMigrate:              autoMigrate,
			FileStorageProvider:      "local",
			FileUploadLocalDir:       t.TempDir(),
			FileUploadMaxBytes:       40 * 1024 * 1024,
			ImageCompressTargetBytes: 20 * 1024 * 1024,
			ImageProcessorDriver:     "passthrough",
			BuyerWechatLoginMode:     "mock",
			BuyerDouyinLoginMode:     "mock",
			BuyerWechatHTTPTimeout:   5 * time.Second,
			BuyerDouyinHTTPTimeout:   5 * time.Second,
		}
	}

	srv, err := app.NewServer(newConfig(false))
	if err != nil {
		t.Fatalf("start migration-only server: %v", err)
	}

	assertFileTableState(t, srv, 0, 1)

	jpeg := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F',
		0x00, 0x01, 0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00,
		0xFF, 0xD9,
	}
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "migration-only-license.jpg",
		"file_size": len(jpeg),
		"mime_type": "image/jpeg",
	}, nil)
	if presign.Code != 0 {
		t.Fatalf("presign against migration-only schema failed: %+v", presign)
	}

	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey,
	}, "file", "migration-only-license.jpg", jpeg, nil)
	if upload.Code != 0 {
		t.Fatalf("upload against migration-only schema failed: %+v", upload)
	}
	confirm := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/confirm", map[string]interface{}{
		"file_id": fileID, "object_key": objectKey,
	}, nil)
	if confirm.Code != 0 {
		t.Fatalf("confirm against migration-only schema failed: %+v", confirm)
	}

	var rows int64
	if err := srv.DB.Table("file_records").Where("id = ?", fileID).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("file_records row check: rows=%d err=%v", rows, err)
	}

	autoSrv, err := app.NewServer(newConfig(true))
	if err != nil {
		t.Fatalf("AutoMigrate compatibility start failed: %v", err)
	}
	assertFileTableState(t, autoSrv, 0, 1)
}

func assertFileTableState(t *testing.T, srv *app.Server, wantFiles, wantFileRecords int64) {
	t.Helper()
	var files int64
	var fileRecords int64
	if err := srv.DB.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'files'`).Scan(&files).Error; err != nil {
		t.Fatalf("count files table: %v", err)
	}
	if err := srv.DB.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'file_records'`).Scan(&fileRecords).Error; err != nil {
		t.Fatalf("count file_records table: %v", err)
	}
	if files != wantFiles || fileRecords != wantFileRecords {
		t.Fatalf("file table state files=%d file_records=%d, want %d/%d", files, fileRecords, wantFiles, wantFileRecords)
	}
}
