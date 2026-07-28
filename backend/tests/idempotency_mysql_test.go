package tests

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysqlcfg "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/model"
)

const idempotencyMySQLSkipInstruction = "set IDEMPOTENCY_MYSQL_TEST=1 only in the isolated idempotency project"

type idempotencyMySQLTableEngine struct {
	TableName string `gorm:"column:table_name"`
	Engine    string `gorm:"column:engine"`
}

type idempotencyMySQLRequestResult struct {
	response apiResp
	err      error
}

func TestIdempotencyMySQLAcceptance(t *testing.T) {
	dsn := requireIsolatedIdempotencyMySQLDSN(t)
	srv := newIdempotencyMySQLServer(t, dsn)
	assertIdempotencyMySQLTableEngines(t, srv.DB)
	fixture := createBuyerIntentMySQLFixture(t, srv)
	t.Cleanup(func() { cleanupBuyerIntentMySQLFixture(srv, fixture) })

	t.Run("same hash executes one product transition", func(t *testing.T) {
		runIdempotencySameHashMySQL(t, srv, fixture)
	})
	t.Run("different hash returns 10011", func(t *testing.T) {
		runIdempotencyDifferentHashMySQL(t, srv, fixture)
	})
	t.Run("failed callback releases claim for waiter", func(t *testing.T) {
		runIdempotencyFailureReleaseMySQL(t, srv, fixture)
	})
	t.Run("terminal failure rolls back business mutation", func(t *testing.T) {
		runIdempotencyTerminalRollbackMySQL(t, srv, fixture)
	})
}

func requireIsolatedIdempotencyMySQLDSN(t *testing.T) string {
	t.Helper()
	if os.Getenv("IDEMPOTENCY_MYSQL_TEST") != "1" {
		t.Skip(idempotencyMySQLSkipInstruction)
	}

	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	parsed, err := mysqlcfg.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "second_hand_market_acceptance" {
		t.Fatal("DB_DSN must target isolated TCP mysql:3306/second_hand_market_acceptance")
	}
	return dsn
}

func newIdempotencyMySQLServer(t *testing.T, dsn string) *app.Server {
	t.Helper()
	cfg := app.Config{
		AppEnv:                   "test",
		Addr:                     ":0",
		DBDriver:                 "mysql",
		DBDSN:                    dsn,
		JWTAccessSecret:          "idempotency-test-access",
		JWTRefreshSecret:         "idempotency-test-refresh",
		AccessTTL:                time.Hour,
		RefreshTTL:               24 * time.Hour,
		AutoMigrate:              strings.EqualFold(os.Getenv("AUTO_MIGRATE"), "true"),
		FileStorageProvider:      "local",
		FileUploadLocalDir:       t.TempDir(),
		ImageCompressTargetBytes: 10 * 1024 * 1024,
		ImageProcessorDriver:     "passthrough",
		BuyerWechatLoginMode:     "mock",
		BuyerDouyinLoginMode:     "mock",
		BuyerWechatHTTPTimeout:   5 * time.Second,
		BuyerDouyinHTTPTimeout:   5 * time.Second,
	}
	configureTestUploadGovernance(&cfg)
	srv, err := app.NewServer(cfg)
	if err != nil {
		t.Fatal("start isolated idempotency server")
	}
	sqlDB, err := srv.DB.DB()
	if err != nil {
		t.Fatal("open isolated idempotency pool")
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return srv
}

func assertIdempotencyMySQLTableEngines(t *testing.T, db *gorm.DB) {
	t.Helper()
	want := []string{
		"buyer_intents",
		"idempotency_records",
		"operation_logs",
		"order_events",
		"orders",
		"products",
	}
	var rows []idempotencyMySQLTableEngine
	if err := db.Raw(`
		SELECT table_name, engine
		FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name IN ?
		ORDER BY table_name`, want).Scan(&rows).Error; err != nil {
		t.Fatal("inspect isolated idempotency table engines")
	}
	if len(rows) != len(want) {
		t.Fatal("isolated idempotency tables were missing or duplicated")
	}
	for index, row := range rows {
		if row.TableName != want[index] || !strings.EqualFold(row.Engine, "InnoDB") {
			t.Fatal("isolated idempotency tables must be the exact InnoDB set")
		}
	}
}

func runIdempotencySameHashMySQL(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture) {
	t.Helper()
	suffix := time.Now().UnixNano()
	price, originalPrice := 10000, 12000
	product := model.Product{
		ProductNo:         fmt.Sprintf("F15P%d", suffix),
		MerchantID:        fixture.merchantID,
		Title:             "F15 Contention Product",
		Description:       "isolated idempotency contention product",
		CategoryID:        fixture.categoryID,
		PriceCent:         price,
		OriginalPriceCent: &originalPrice,
		ConditionLevel:    "GOOD",
		Stock:             1,
		Status:            model.ProductDraft,
		CreatedBy:         fixture.merchantAccountID,
		UpdatedBy:         fixture.merchantAccountID,
		Version:           1,
	}
	if err := srv.DB.Create(&product).Error; err != nil {
		t.Fatal("create isolated idempotency draft product")
	}
	image := model.ProductImage{ProductID: product.ID, FileID: 0, SortOrder: 1}
	if err := srv.DB.Create(&image).Error; err != nil {
		t.Fatal("create isolated idempotency product image")
	}
	key := fmt.Sprintf("f15-product-%d", product.ID)
	t.Cleanup(func() {
		_ = srv.DB.Where("idem_key = ?", key).Delete(&model.IdempotencyRecord{}).Error
		_ = srv.DB.Where("resource_type = ? AND resource_id = ?", "product", product.ID).Delete(&model.OperationLog{}).Error
		_ = srv.DB.Where("product_id = ?", product.ID).Delete(&model.ProductImage{}).Error
		_ = srv.DB.Unscoped().Where("id = ?", product.ID).Delete(&model.Product{}).Error
	})

	path := fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", product.ID)
	headers := map[string]string{"Authorization": "Bearer " + fixture.merchantToken}
	responses := runSynchronizedIdempotencyMySQLRequests(t, srv, path, key, []interface{}{map[string]interface{}{}, map[string]interface{}{}}, headers)

	originals, replays := 0, 0
	for _, response := range responses {
		if response.HTTPStatus != http.StatusOK || response.Code != 0 {
			t.Fatal("same-hash request did not succeed")
		}
		flag, ok := response.Data["idempotent"].(bool)
		if !ok {
			t.Fatal("same-hash response omitted idempotent flag")
		}
		if flag {
			replays++
		} else {
			originals++
		}
	}
	if originals != 1 || replays != 1 {
		t.Fatal("same-hash contention did not produce one original and one replay")
	}
	assertSameIdempotencyBusinessPayload(t, responses[0], responses[1])

	var stored model.Product
	if err := srv.DB.First(&stored, product.ID).Error; err != nil {
		t.Fatal("load isolated idempotency product")
	}
	if stored.Status != model.ProductOnShelf || stored.Version != product.Version+1 {
		t.Fatal("same-hash contention did not commit exactly one product transition")
	}
	assertIdempotencyMySQLOperationLogCount(t, srv.DB, "product", product.ID, "product_on_shelf", 1)
	assertIdempotencyMySQLTerminalRecord(t, srv.DB, key, fixture.merchantAccountID, "/api/v1/merchant/products/:id/on-shelf")
}

func runIdempotencyDifferentHashMySQL(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture) {
	t.Helper()
	intent := createIdempotencyMySQLOpenIntent(t, srv, fixture)
	key := fmt.Sprintf("f15-different-%d", intent.ID)
	registerIdempotencyMySQLIntentCleanup(t, srv, intent.ID, key)

	path := fmt.Sprintf("/api/v1/merchant/intents/%d/close", intent.ID)
	headers := map[string]string{"Authorization": "Bearer " + fixture.merchantToken}
	bodies := []interface{}{
		map[string]interface{}{"reason": "NO_RESPONSE", "merchant_note": "first contention note"},
		map[string]interface{}{"reason": "NO_RESPONSE", "merchant_note": "second contention note"},
	}
	responses := runSynchronizedIdempotencyMySQLRequests(t, srv, path, key, bodies, headers)

	successes, conflicts := 0, 0
	for _, response := range responses {
		switch {
		case response.HTTPStatus == http.StatusOK && response.Code == 0:
			successes++
		case response.HTTPStatus == http.StatusConflict && response.Code == 10011:
			conflicts++
		default:
			t.Fatal("different-hash contention returned an unexpected response")
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("different-hash contention did not produce one success and one conflict")
	}

	var stored model.BuyerIntent
	if err := srv.DB.First(&stored, intent.ID).Error; err != nil {
		t.Fatal("load different-hash buyer intent")
	}
	if stored.Status != model.IntentClosed || stored.IsOpen || stored.MerchantNote == nil {
		t.Fatal("different-hash contention did not commit one close transition")
	}
	assertIdempotencyMySQLOperationLogCount(t, srv.DB, "intent", intent.ID, "merchant_intent_close", 1)
	record := assertIdempotencyMySQLTerminalRecord(t, srv.DB, key, fixture.merchantAccountID, "/api/v1/merchant/intents/:id/close")
	firstHash := idempotencyMySQLCloseRequestHash(intent.ID, "first contention note")
	secondHash := idempotencyMySQLCloseRequestHash(intent.ID, "second contention note")
	if record.RequestHash != firstHash && record.RequestHash != secondHash {
		t.Fatal("different-hash contention stored an unexpected request hash")
	}
	wantNote := "first contention note"
	if record.RequestHash == secondHash {
		wantNote = "second contention note"
	}
	if *stored.MerchantNote != wantNote {
		t.Fatal("different-hash contention persisted fields from the losing request")
	}
}

func runIdempotencyFailureReleaseMySQL(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture) {
	t.Helper()
	intent := createIdempotencyMySQLOpenIntent(t, srv, fixture)
	key := fmt.Sprintf("f15-release-%d", intent.ID)
	registerIdempotencyMySQLIntentCleanup(t, srv, intent.ID, key)

	callbackName := "idempotency-mysql:fail-first-intent-transition"
	var failed atomic.Int32
	callback := func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "buyer_intents" && failed.CompareAndSwap(0, 1) {
			tx.AddError(errors.New("forced first buyer intent transition failure"))
		}
	}
	if err := srv.DB.Callback().Update().Before("gorm:update").Register(callbackName, callback); err != nil {
		t.Fatal("register isolated buyer intent failure callback")
	}
	t.Cleanup(func() { _ = srv.DB.Callback().Update().Remove(callbackName) })

	path := fmt.Sprintf("/api/v1/merchant/intents/%d/close", intent.ID)
	headers := map[string]string{"Authorization": "Bearer " + fixture.merchantToken}
	body := map[string]interface{}{"reason": "NO_RESPONSE", "merchant_note": "release contention note"}
	responses := runSynchronizedIdempotencyMySQLRequests(t, srv, path, key, []interface{}{body, body}, headers)
	_ = srv.DB.Callback().Update().Remove(callbackName)

	successes, failures := 0, 0
	for _, response := range responses {
		switch {
		case response.HTTPStatus == http.StatusOK && response.Code == 0:
			successes++
		case response.HTTPStatus == http.StatusInternalServerError && response.Code == 20001:
			failures++
		default:
			t.Fatal("claim-release contention returned an unexpected response")
		}
	}
	if failed.Load() != 1 || successes != 1 || failures != 1 {
		t.Fatal("failed callback did not release the claim for exactly one waiter")
	}

	var stored model.BuyerIntent
	if err := srv.DB.First(&stored, intent.ID).Error; err != nil {
		t.Fatal("load claim-release buyer intent")
	}
	if stored.Status != model.IntentClosed || stored.IsOpen {
		t.Fatal("claim-release contention did not commit exactly one transition")
	}
	assertIdempotencyMySQLOperationLogCount(t, srv.DB, "intent", intent.ID, "merchant_intent_close", 1)
	assertIdempotencyMySQLTerminalRecord(t, srv.DB, key, fixture.merchantAccountID, "/api/v1/merchant/intents/:id/close")
}

func runIdempotencyTerminalRollbackMySQL(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture) {
	t.Helper()
	intent := createIdempotencyMySQLOpenIntent(t, srv, fixture)
	key := fmt.Sprintf("f15-terminal-%d", intent.ID)
	registerIdempotencyMySQLIntentCleanup(t, srv, intent.ID, key)

	callbackName := "idempotency-mysql:fail-first-terminal-update"
	var failed atomic.Int32
	callback := func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "idempotency_records" && failed.CompareAndSwap(0, 1) {
			tx.AddError(errors.New("forced first idempotency finalization failure"))
		}
	}
	if err := srv.DB.Callback().Update().Before("gorm:update").Register(callbackName, callback); err != nil {
		t.Fatal("register isolated terminal failure callback")
	}
	t.Cleanup(func() { _ = srv.DB.Callback().Update().Remove(callbackName) })

	path := fmt.Sprintf("/api/v1/merchant/intents/%d/close", intent.ID)
	headers := map[string]string{
		"Authorization":   "Bearer " + fixture.merchantToken,
		"Idempotency-Key": key,
	}
	body := map[string]interface{}{"reason": "NO_RESPONSE", "merchant_note": "terminal rollback note"}
	failedResponse, err := executeJSONRequest(srv.Router, http.MethodPost, path, body, headers)
	if err != nil {
		t.Fatal("execute terminal rollback request")
	}
	if failedResponse.HTTPStatus != http.StatusInternalServerError || failedResponse.Code != 20001 || failed.Load() != 1 {
		t.Fatal("terminal finalization failure did not return the internal error")
	}
	assertIdempotencyMySQLIntentState(t, srv.DB, intent.ID, model.IntentNew, true)
	assertIdempotencyMySQLOperationLogCount(t, srv.DB, "intent", intent.ID, "merchant_intent_close", 0)
	assertIdempotencyMySQLRecordCount(t, srv.DB, key, 0)

	_ = srv.DB.Callback().Update().Remove(callbackName)
	retry, err := executeJSONRequest(srv.Router, http.MethodPost, path, body, headers)
	if err != nil {
		t.Fatal("execute terminal rollback retry")
	}
	if retry.HTTPStatus != http.StatusOK || retry.Code != 0 {
		t.Fatal("terminal rollback retry did not succeed")
	}
	if flag, ok := retry.Data["idempotent"].(bool); !ok || flag {
		t.Fatal("terminal rollback retry was not a fresh execution")
	}
	assertIdempotencyMySQLIntentState(t, srv.DB, intent.ID, model.IntentClosed, false)
	assertIdempotencyMySQLOperationLogCount(t, srv.DB, "intent", intent.ID, "merchant_intent_close", 1)
	assertIdempotencyMySQLTerminalRecord(t, srv.DB, key, fixture.merchantAccountID, "/api/v1/merchant/intents/:id/close")
}

func runSynchronizedIdempotencyMySQLRequests(t *testing.T, srv *app.Server, path, key string, bodies []interface{}, baseHeaders map[string]string) []apiResp {
	t.Helper()
	start := make(chan struct{})
	results := make(chan idempotencyMySQLRequestResult, len(bodies))
	var concurrent sync.WaitGroup
	for _, body := range bodies {
		headers := make(map[string]string, len(baseHeaders)+1)
		for name, value := range baseHeaders {
			headers[name] = value
		}
		headers["Idempotency-Key"] = key
		concurrent.Add(1)
		go func(body interface{}, headers map[string]string) {
			defer concurrent.Done()
			<-start
			response, err := executeJSONRequest(srv.Router, http.MethodPost, path, body, headers)
			results <- idempotencyMySQLRequestResult{response: response, err: err}
		}(body, headers)
	}
	close(start)
	concurrent.Wait()
	close(results)

	responses := make([]apiResp, 0, len(bodies))
	for result := range results {
		if result.err != nil {
			t.Fatal("execute synchronized idempotency request")
		}
		responses = append(responses, result.response)
	}
	return responses
}

func createIdempotencyMySQLOpenIntent(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture) model.BuyerIntent {
	t.Helper()
	intent := model.BuyerIntent{
		IntentNo:   fmt.Sprintf("F15I%d", time.Now().UnixNano()),
		BuyerID:    fixture.buyerID,
		ProductID:  fixture.productID,
		MerchantID: fixture.merchantID,
		Status:     model.IntentNew,
		IsOpen:     true,
	}
	if err := srv.DB.Create(&intent).Error; err != nil {
		t.Fatal("create isolated idempotency buyer intent")
	}
	return intent
}

func registerIdempotencyMySQLIntentCleanup(t *testing.T, srv *app.Server, intentID uint64, key string) {
	t.Helper()
	t.Cleanup(func() {
		_ = srv.DB.Where("idem_key = ?", key).Delete(&model.IdempotencyRecord{}).Error
		_ = srv.DB.Where("resource_type = ? AND resource_id = ?", "intent", intentID).Delete(&model.OperationLog{}).Error
		_ = srv.DB.Where("id = ?", intentID).Delete(&model.BuyerIntent{}).Error
	})
}

func assertIdempotencyMySQLIntentState(t *testing.T, db *gorm.DB, intentID uint64, status string, open bool) {
	t.Helper()
	var intent model.BuyerIntent
	if err := db.First(&intent, intentID).Error; err != nil {
		t.Fatal("load isolated idempotency buyer intent state")
	}
	if intent.Status != status || intent.IsOpen != open {
		t.Fatal("isolated idempotency buyer intent state did not match")
	}
}

func assertIdempotencyMySQLOperationLogCount(t *testing.T, db *gorm.DB, resourceType string, resourceID uint64, action string, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.OperationLog{}).Where(
		"resource_type = ? AND resource_id = ? AND action = ?", resourceType, resourceID, action,
	).Count(&count).Error; err != nil {
		t.Fatal("count isolated idempotency operation logs")
	}
	if count != want {
		t.Fatal("isolated idempotency operation log count did not match")
	}
}

func assertIdempotencyMySQLRecordCount(t *testing.T, db *gorm.DB, key string, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.IdempotencyRecord{}).Where("idem_key = ?", key).Count(&count).Error; err != nil {
		t.Fatal("count isolated idempotency records")
	}
	if count != want {
		t.Fatal("isolated idempotency record count did not match")
	}
}

func assertIdempotencyMySQLTerminalRecord(t *testing.T, db *gorm.DB, key string, operatorID uint64, path string) model.IdempotencyRecord {
	t.Helper()
	var records []model.IdempotencyRecord
	if err := db.Where("idem_key = ? AND operator_id = ? AND path = ?", key, operatorID, path).Find(&records).Error; err != nil {
		t.Fatal("load isolated terminal idempotency record")
	}
	if len(records) != 1 || records[0].ResultCode != 0 || len(records[0].ResponseRaw) == 0 || string(records[0].ResponseRaw) == "null" {
		t.Fatal("isolated idempotency claim was not terminal")
	}
	return records[0]
}

func idempotencyMySQLCloseRequestHash(intentID uint64, note string) string {
	payload := fmt.Sprintf(`{"id":%d,"merchant_note":%q,"reason":"NO_RESPONSE","to_status":"CLOSED"}`, intentID, note)
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}
