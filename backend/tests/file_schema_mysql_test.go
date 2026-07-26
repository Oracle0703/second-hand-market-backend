package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestLicenseFilePrivacyWithMigrationOnlyMySQL(t *testing.T) {
	if os.Getenv("FILE_SCHEMA_MYSQL_TEST") != "1" {
		t.Skip("set FILE_SCHEMA_MYSQL_TEST=1 only in the isolated MySQL acceptance project")
	}
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Fatal("DB_DSN is required for isolated license privacy acceptance")
	}
	uploadDir := os.Getenv("FILE_UPLOAD_LOCAL_DIR")
	if uploadDir == "" {
		t.Fatal("FILE_UPLOAD_LOCAL_DIR is required for isolated license privacy acceptance")
	}

	cfg := app.Config{
		AppEnv:                   "test",
		Addr:                     ":0",
		DBDriver:                 "mysql",
		DBDSN:                    dsn,
		JWTAccessSecret:          "license-privacy-test-access",
		JWTRefreshSecret:         "license-privacy-test-refresh",
		AccessTTL:                time.Hour,
		RefreshTTL:               24 * time.Hour,
		AutoMigrate:              strings.EqualFold(os.Getenv("AUTO_MIGRATE"), "true"),
		FileStorageProvider:      "local",
		FileUploadLocalDir:       uploadDir,
		ImageCompressTargetBytes: 20 * 1024 * 1024,
		ImageProcessorDriver:     "passthrough",
		BuyerWechatLoginMode:     "mock",
		BuyerDouyinLoginMode:     "mock",
		BuyerWechatHTTPTimeout:   5 * time.Second,
		BuyerDouyinHTTPTimeout:   5 * time.Second,
	}
	configureTestUploadGovernance(&cfg)
	srv, err := app.NewServer(cfg)
	if err != nil {
		t.Fatalf("start license privacy migration-only server: %v", err)
	}
	assertFileTableState(t, srv, 0, 1)
	assertFileBindingSchemaSingletons(t, srv)

	hash, err := bcrypt.GenerateFromPassword([]byte(testAdminPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash acceptance admin password: %v", err)
	}
	admin := model.AdminUser{
		Username: "admin", DisplayName: "License Privacy Admin", Role: model.AdminRoleAdmin,
		Status: model.AccountStatusActive, PasswordHash: string(hash),
	}
	if err := srv.DB.Where("username = ?", admin.Username).FirstOrCreate(&admin).Error; err != nil {
		t.Fatalf("create acceptance admin: %v", err)
	}

	fixture := newLicensePrivacyFixtureForServer(t, srv, uploadDir)
	productKey := fmt.Sprintf("product_image/mysql-privacy-%d.jpg", time.Now().UnixNano())
	productBytes := []byte("isolated-public-product-image")
	product := model.FileRecord{
		BizType: model.FileBizProductImage, ObjectKey: productKey,
		URL: "/uploads/" + productKey, MimeType: "image/jpeg", SizeBytes: int64(len(productBytes)),
		UploaderType: model.UserTypePublic, ScanStatus: model.FileScanPass,
	}
	if err := srv.DB.Create(&product).Error; err != nil {
		t.Fatalf("create public product record: %v", err)
	}
	writeUploadFixture(t, uploadDir, productKey, productBytes)

	publicReq := httptest.NewRequest(http.MethodGet, "/uploads/"+productKey, nil)
	publicW := httptest.NewRecorder()
	srv.Router.ServeHTTP(publicW, publicReq)
	if publicW.Code != http.StatusOK || !bytes.Equal(publicW.Body.Bytes(), productBytes) {
		t.Fatalf("anonymous product response: status=%d body=%q", publicW.Code, publicW.Body.String())
	}

	privateReq := httptest.NewRequest(http.MethodGet, "/uploads/"+fixture.file.ObjectKey, nil)
	privateW := httptest.NewRecorder()
	srv.Router.ServeHTTP(privateW, privateReq)
	if privateW.Code != http.StatusNotFound || bytes.Contains(privateW.Body.Bytes(), fixture.fileBytes) {
		t.Fatalf("anonymous license response: status=%d body=%q", privateW.Code, privateW.Body.String())
	}

	contentW := requestAdminLicenseContent(t, fixture, fixture.adminToken)
	if contentW.Code != http.StatusOK || !bytes.Equal(contentW.Body.Bytes(), fixture.fileBytes) {
		t.Fatalf("admin license response: status=%d body=%q", contentW.Code, contentW.Body.String())
	}
	for name, want := range map[string]string{
		"Content-Type": "image/jpeg", "Content-Disposition": "inline",
		"Cache-Control": "private, no-store", "Pragma": "no-cache", "X-Content-Type-Options": "nosniff",
	} {
		if got := contentW.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	var logs []model.OperationLog
	if err := srv.DB.Where("action = ? AND resource_id = ?", "admin_file_read", fixture.file.ID).Find(&logs).Error; err != nil {
		t.Fatalf("load admin file read audit: %v", err)
	}
	if len(logs) != 1 || string(logs[0].DetailJSON) != `{"biz_type":"MERCHANT_LICENSE","scan_status":"PASS"}` {
		t.Fatalf("unsafe admin file read audit: %+v", logs)
	}

	jpeg := minimalJPEG()
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": model.FileBizMerchantLicense, "file_name": "new-private-license.jpg",
		"file_size": len(jpeg), "mime_type": "image/jpeg",
	}, nil)
	if presign.Code != common.CodeOK {
		t.Fatalf("private license presign: %+v", presign)
	}
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	fileToken := str(presign.Data["file_token"])
	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey, "file_token": fileToken,
	}, "file", filepath.Base(objectKey), jpeg, nil)
	if upload.Code != common.CodeOK {
		t.Fatalf("private license upload: %+v", upload)
	}
	if _, exists := upload.Data["url"]; exists {
		t.Fatalf("private license upload exposed url: %+v", upload.Data)
	}
	var uploaded model.FileRecord
	if err := srv.DB.First(&uploaded, fileID).Error; err != nil {
		t.Fatalf("load private license upload: %v", err)
	}
	if uploaded.URL != "" || uploaded.ObjectKey == "" || uploaded.ScanStatus != model.FileScanPass {
		t.Fatalf("private license upload state: %+v", uploaded)
	}
}

func TestFileFlowWithMigrationOnlyMySQL(t *testing.T) {
	if os.Getenv("FILE_SCHEMA_MYSQL_TEST") != "1" {
		t.Skip("set FILE_SCHEMA_MYSQL_TEST=1 only in the isolated MySQL acceptance project")
	}
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Fatal("DB_DSN is required for isolated file schema acceptance")
	}

	newConfig := func(autoMigrate bool) app.Config {
		cfg := app.Config{
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
			ImageCompressTargetBytes: 20 * 1024 * 1024,
			ImageProcessorDriver:     "passthrough",
			BuyerWechatLoginMode:     "mock",
			BuyerDouyinLoginMode:     "mock",
			BuyerWechatHTTPTimeout:   5 * time.Second,
			BuyerDouyinHTTPTimeout:   5 * time.Second,
		}
		configureTestUploadGovernance(&cfg)
		return cfg
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
	fileToken := str(presign.Data["file_token"])
	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey, "file_token": fileToken,
	}, "file", "migration-only-license.jpg", jpeg, nil)
	if upload.Code != 0 {
		t.Fatalf("upload against migration-only schema failed: %+v", upload)
	}
	confirm := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/confirm", map[string]interface{}{
		"file_id": fileID, "object_key": objectKey, "file_token": fileToken,
	}, nil)
	if confirm.Code != 0 {
		t.Fatalf("confirm against migration-only schema failed: %+v", confirm)
	}

	var rows int64
	if err := srv.DB.Table("file_records").Where("id = ?", fileID).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("file_records row check: rows=%d err=%v", rows, err)
	}

	username := uniqueUsername("mysql_file_binding")
	password := "Passw0rd!2026"
	register := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register", map[string]interface{}{
		"merchant_name":      "MySQL Binding Store",
		"contact_name":       "Owner",
		"phone":              "13800138000",
		"username":           username,
		"password":           password,
		"license_file_id":    fileID,
		"license_file_token": fileToken,
	}, nil)
	if register.Code != common.CodeOK {
		t.Fatalf("register against migration-only schema failed: %+v", register)
	}
	merchantID := numToUint64(register.Data["merchant_id"])
	var claimed model.FileRecord
	if err := srv.DB.First(&claimed, fileID).Error; err != nil {
		t.Fatalf("load claimed license: %v", err)
	}
	if claimed.OwnerMerchantID == nil || *claimed.OwnerMerchantID != merchantID ||
		claimed.CapabilityTokenHash != nil || claimed.CapabilityExpiresAt != nil {
		t.Fatalf("registration did not consume capability: %+v", claimed)
	}

	if err := srv.DB.Model(&model.Merchant{}).Where("id = ?", merchantID).
		Update("review_status", model.ReviewApproved).Error; err != nil {
		t.Fatalf("approve migration-only merchant: %v", err)
	}
	login := merchantLogin(t, srv, username, password)
	if login.Code != common.CodeOK {
		t.Fatalf("login migration-only merchant: %+v", login)
	}
	merchantToken := str(login.Data["access_token"])
	imagePresign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": model.FileBizProductImage, "file_name": "product.jpg", "file_size": len(jpeg), "mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if imagePresign.Code != common.CodeOK {
		t.Fatalf("product image presign: %+v", imagePresign)
	}
	imageID := numToUint64(imagePresign.Data["file_id"])
	imageKey := str(imagePresign.Data["object_key"])
	imageUpload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", imageID), "object_key": imageKey,
	}, "file", "product.jpg", jpeg, map[string]string{"Authorization": "Bearer " + merchantToken})
	if imageUpload.Code != common.CodeOK {
		t.Fatalf("product image upload: %+v", imageUpload)
	}
	var category model.Category
	if err := srv.DB.Where("level = ? AND status = ?", 2, model.CategoryEnabled).First(&category).Error; err != nil {
		t.Fatalf("load seeded category: %v", err)
	}
	product := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/products", productCreatePayload(category.ID, []uint64{imageID}),
		map[string]string{"Authorization": "Bearer " + merchantToken})
	if product.Code != common.CodeOK {
		t.Fatalf("product binding against migration-only schema: %+v", product)
	}

	concurrentFileID, concurrentToken := uploadReadyPublicLicense(t, srv)
	payloads := []map[string]interface{}{
		registrationPayload(uniqueUsername("mysql_claim_a"), concurrentFileID, concurrentToken),
		registrationPayload(uniqueUsername("mysql_claim_b"), concurrentFileID, concurrentToken),
	}
	results := make([]apiResp, len(payloads))
	errs := make([]error, len(payloads))
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(len(payloads))
	for i := range payloads {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ready.Done()
			<-start
			results[index], errs[index] = postJSONWithoutTesting(srv.Router, "/api/v1/auth/register", payloads[index])
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()
	successes := 0
	rejections := 0
	winnerIndex := -1
	for i, result := range results {
		if errs[i] != nil {
			t.Fatalf("concurrent registration %d: %v", i, errs[i])
		}
		switch result.Code {
		case common.CodeOK:
			successes++
			winnerIndex = i
		case common.CodeInvalidFileBinding:
			rejections++
		default:
			t.Fatalf("unexpected concurrent registration result: %+v", result)
		}
	}
	if successes != 1 || rejections != 1 {
		t.Fatalf("concurrent claim results: successes=%d rejections=%d results=%+v", successes, rejections, results)
	}
	assertConcurrentRegistrationState(t, srv, payloads, results, winnerIndex, concurrentFileID)

	autoSrv, err := app.NewServer(newConfig(true))
	if err != nil {
		t.Fatalf("AutoMigrate compatibility start failed: %v", err)
	}
	assertFileTableState(t, autoSrv, 0, 1)
	assertFileBindingSchemaSingletons(t, autoSrv)
}

func postJSONWithoutTesting(h http.Handler, path string, body interface{}) (apiResp, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return apiResp{}, err
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp apiResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		return apiResp{}, fmt.Errorf("decode response status=%d body=%q: %w", w.Code, w.Body.String(), err)
	}
	return resp, nil
}

func assertConcurrentRegistrationState(
	t *testing.T,
	srv *app.Server,
	payloads []map[string]interface{},
	results []apiResp,
	winnerIndex int,
	fileID uint64,
) {
	t.Helper()
	if winnerIndex < 0 || winnerIndex >= len(payloads) || winnerIndex >= len(results) {
		t.Fatalf("invalid concurrent registration winner index %d", winnerIndex)
	}

	usernames := make([]string, 0, len(payloads))
	for _, payload := range payloads {
		username, ok := payload["username"].(string)
		if !ok || username == "" {
			t.Fatalf("concurrent registration payload has invalid username: %+v", payload)
		}
		usernames = append(usernames, username)
	}
	winnerUsername := usernames[winnerIndex]
	winnerMerchantID := numToUint64(results[winnerIndex].Data["merchant_id"])
	if winnerMerchantID == 0 {
		t.Fatalf("concurrent registration winner has invalid merchant id: %+v", results[winnerIndex])
	}

	var accounts []model.MerchantAccount
	if err := srv.DB.Where("username IN ?", usernames).Find(&accounts).Error; err != nil {
		t.Fatalf("load concurrent registration accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Username != winnerUsername || accounts[0].MerchantID != winnerMerchantID {
		t.Fatalf("concurrent registration accounts=%+v, want only winner %s merchant=%d", accounts, winnerUsername, winnerMerchantID)
	}

	var attemptedMerchants int64
	if err := srv.DB.Model(&model.Merchant{}).Where("merchant_name = ?", "Claim Store").Count(&attemptedMerchants).Error; err != nil {
		t.Fatalf("count concurrent registration merchants: %v", err)
	}
	if attemptedMerchants != 1 {
		t.Fatalf("concurrent registration merchant count=%d, want 1", attemptedMerchants)
	}

	var claimed model.FileRecord
	if err := srv.DB.First(&claimed, fileID).Error; err != nil {
		t.Fatalf("load concurrently claimed file: %v", err)
	}
	if claimed.OwnerMerchantID == nil || *claimed.OwnerMerchantID != winnerMerchantID ||
		claimed.CapabilityTokenHash != nil || claimed.CapabilityExpiresAt != nil {
		t.Fatalf("concurrent claim durable state does not match winner merchant=%d: %+v", winnerMerchantID, claimed)
	}
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

func assertFileBindingSchemaSingletons(t *testing.T, srv *app.Server) {
	t.Helper()
	for _, column := range []string{"owner_merchant_id", "capability_token_hash", "capability_expires_at"} {
		var count int64
		if err := srv.DB.Raw(`
			SELECT COUNT(*)
			FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = 'file_records' AND column_name = ?
		`, column).Scan(&count).Error; err != nil {
			t.Fatalf("count file_records column %s: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("file_records column %s count=%d, want 1", column, count)
		}
	}

	type expectedIndex struct {
		name      string
		columns   string
		unique    bool
		partCount int64
	}
	for _, index := range []expectedIndex{
		{name: "idx_file_owner_biz_scan", columns: "owner_merchant_id,biz_type,scan_status", partCount: 3},
		{name: "uk_file_capability_token", columns: "capability_token_hash", unique: true, partCount: 1},
		{name: "idx_file_capability_expires", columns: "capability_expires_at", partCount: 1},
	} {
		wantNonUnique := 1
		if index.unique {
			wantNonUnique = 0
		}
		var namedMatches int64
		if err := srv.DB.Raw(`
			SELECT COUNT(*)
			FROM (
				SELECT index_name
				FROM information_schema.statistics
				WHERE table_schema = DATABASE() AND table_name = 'file_records' AND index_name = ?
				GROUP BY index_name
				HAVING COUNT(*) = ?
					AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = ?
					AND MIN(non_unique) = ? AND MAX(non_unique) = ?
					AND SUM(CASE WHEN sub_part IS NULL THEN 0 ELSE 1 END) = 0
					AND SUM(CASE WHEN collation = 'A' THEN 0 ELSE 1 END) = 0
					AND MIN(index_type) = 'BTREE' AND MAX(index_type) = 'BTREE'
			) expected_index
		`, index.name, index.partCount, index.columns, wantNonUnique, wantNonUnique).Scan(&namedMatches).Error; err != nil {
			t.Fatalf("inspect file_records index %s: %v", index.name, err)
		}
		if namedMatches != 1 {
			t.Fatalf("file_records index %s exact-shape count=%d, want 1", index.name, namedMatches)
		}

		var equivalentMatches int64
		if err := srv.DB.Raw(`
			SELECT COUNT(*)
			FROM (
				SELECT index_name
				FROM information_schema.statistics
				WHERE table_schema = DATABASE() AND table_name = 'file_records'
				GROUP BY index_name
				HAVING COUNT(*) = ?
					AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = ?
					AND MIN(non_unique) = ? AND MAX(non_unique) = ?
					AND SUM(CASE WHEN sub_part IS NULL THEN 0 ELSE 1 END) = 0
					AND SUM(CASE WHEN collation = 'A' THEN 0 ELSE 1 END) = 0
					AND MIN(index_type) = 'BTREE' AND MAX(index_type) = 'BTREE'
			) equivalent_indexes
		`, index.partCount, index.columns, wantNonUnique, wantNonUnique).Scan(&equivalentMatches).Error; err != nil {
			t.Fatalf("count equivalent file_records indexes for %s: %v", index.name, err)
		}
		if equivalentMatches != 1 {
			t.Fatalf("file_records index shape %s has %d copies, want 1", index.columns, equivalentMatches)
		}
	}
}
