package tests

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	mysqlcfg "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/model"
)

func TestBuyerIntentMySQLAcceptance(t *testing.T) {
	if os.Getenv("BUYER_INTENT_MYSQL_TEST") != "1" {
		t.Skip("set BUYER_INTENT_MYSQL_TEST=1 only in the isolated buyer intent project")
	}
	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	parsed, err := mysqlcfg.ParseDSN(dsn)
	if err != nil ||
		parsed.Net != "tcp" ||
		parsed.Addr != "mysql:3306" ||
		parsed.DBName != "second_hand_market_acceptance" {
		t.Fatal("DB_DSN must target isolated mysql:3306/second_hand_market_acceptance")
	}
	cfg := app.Config{
		AppEnv:                   "test",
		Addr:                     ":0",
		DBDriver:                 "mysql",
		DBDSN:                    dsn,
		JWTAccessSecret:          "buyer-intent-test-access",
		JWTRefreshSecret:         "buyer-intent-test-refresh",
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
		t.Fatal("start isolated buyer intent server")
	}
	sqlDB, err := srv.DB.DB()
	if err != nil {
		t.Fatal("open isolated buyer intent pool")
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	assertBuyerIntentMySQLSchema(t, srv)
	fixture := createBuyerIntentMySQLFixture(t, srv)
	t.Cleanup(func() { cleanupBuyerIntentMySQLFixture(srv, fixture) })

	buyerHeaders := map[string]string{
		"Authorization": "Bearer " + fixture.buyerToken,
		"X-Device-Id":   fixture.buyerDevice,
	}
	merchantHeaders := map[string]string{"Authorization": "Bearer " + fixture.merchantToken}

	first := createBuyerIntentMySQL(t, srv, buyerHeaders, fixture.productID, "f11-cycle-one")
	requireBuyerIntentMySQLStatus(t, "cycle one create", first, http.StatusOK, 0)
	contacted := buyerIntentMySQLRequest(t, srv, http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/intents/%d/contacted", first.intentID), map[string]interface{}{}, merchantHeaders)
	requireBuyerIntentMySQLStatus(t, "cycle one contacted", contacted, http.StatusOK, 0)
	closed := buyerIntentMySQLRequest(t, srv, http.MethodPost,
		fmt.Sprintf("/api/v1/merchant/intents/%d/close", first.intentID), map[string]interface{}{"reason": "NO_RESPONSE"}, merchantHeaders)
	requireBuyerIntentMySQLStatus(t, "cycle one close", closed, http.StatusOK, 0)

	for cycle := 2; cycle <= 3; cycle++ {
		created := createBuyerIntentMySQL(t, srv, buyerHeaders, fixture.productID, fmt.Sprintf("f11-cycle-%d", cycle))
		requireBuyerIntentMySQLStatus(t, "history create", created, http.StatusOK, 0)
		closed := buyerIntentMySQLRequest(t, srv, http.MethodPost,
			fmt.Sprintf("/api/v1/merchant/intents/%d/close", created.intentID), map[string]interface{}{"reason": "NO_RESPONSE"}, merchantHeaders)
		requireBuyerIntentMySQLStatus(t, "history close", closed, http.StatusOK, 0)
	}
	assertBuyerIntentMySQLStates(t, srv, fixture.buyerID, fixture.productID, 3, 0)

	start := make(chan struct{})
	responses := make(chan buyerIntentMySQLResponse, 2)
	var concurrent sync.WaitGroup
	for request := 0; request < 2; request++ {
		concurrent.Add(1)
		go func(request int) {
			defer concurrent.Done()
			<-start
			response, err := executeJSONRequest(srv.Router, http.MethodPost, "/api/v1/buyer/intents", map[string]interface{}{
				"product_id": fixture.productID, "contact_wechat": fmt.Sprintf("f11-concurrent-%d", request),
			}, buyerHeaders)
			if err != nil {
				responses <- buyerIntentMySQLResponse{err: err}
				return
			}
			responses <- buyerIntentMySQLResponse{HTTPStatus: response.HTTPStatus, Code: response.Code, intentID: numToUint64(response.Data["intent_id"])}
		}(request)
	}
	close(start)
	concurrent.Wait()
	close(responses)

	successes, conflicts := 0, 0
	statusCodes := make([]string, 0, 2)
	for response := range responses {
		if response.err != nil {
			t.Fatal("execute concurrent buyer intent request")
		}
		statusCodes = append(statusCodes, fmt.Sprintf("%d/%d", response.HTTPStatus, response.Code))
		switch {
		case response.HTTPStatus == http.StatusOK && response.Code == 0:
			successes++
		case response.HTTPStatus == http.StatusConflict && response.Code == 10010:
			conflicts++
		default:
			t.Fatal("unexpected concurrent buyer intent response")
		}
	}
	sort.Strings(statusCodes)
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent buyer intent creation did not have one winner")
	}
	t.Logf("concurrent create status/codes = %v", statusCodes)
	closedCount, openCount := assertBuyerIntentMySQLStates(t, srv, fixture.buyerID, fixture.productID, 3, 1)
	t.Logf("intent history/open counts = %d/%d", closedCount, openCount)

	secondBuyerHeaders := map[string]string{
		"Authorization": "Bearer " + fixture.secondBuyerToken,
		"X-Device-Id":   fixture.secondBuyerDevice,
	}
	requireBuyerIntentMySQLStatus(t, "second buyer original product",
		createBuyerIntentMySQL(t, srv, secondBuyerHeaders, fixture.productID, "f11-second-original"), http.StatusOK, 0)
	requireBuyerIntentMySQLStatus(t, "second buyer second product",
		createBuyerIntentMySQL(t, srv, secondBuyerHeaders, fixture.secondProductID, "f11-second-product"), http.StatusOK, 0)
	var independent int64
	if err := srv.DB.Model(&model.BuyerIntent{}).Where("buyer_id = ? AND product_id IN ? AND status = ? AND is_open = ?",
		fixture.secondBuyerID, []uint64{fixture.productID, fixture.secondProductID}, model.IntentNew, true).Count(&independent).Error; err != nil || independent != 2 {
		t.Fatal("buyer and product independence was not preserved")
	}

	assertBuyerIntentMySQLDriftFailsClosed(t, srv, fixture, merchantHeaders)
}

func TestBuyerIntentDriftBuyerCreateFailsClosed(t *testing.T) {
	states := []struct {
		name   string
		status string
		open   bool
	}{
		{name: "closed open", status: model.IntentClosed, open: true},
		{name: "bogus closed", status: "BOGUS", open: false},
	}
	for index, state := range states {
		t.Run(state.name, func(t *testing.T) {
			srv := newTestServer(t)
			fixture := createBuyerIntentMySQLFixture(t, srv)
			t.Cleanup(func() { cleanupBuyerIntentMySQLFixture(srv, fixture) })
			intent := model.BuyerIntent{
				IntentNo:   fmt.Sprintf("F11L%d%d", index, time.Now().UnixNano()),
				BuyerID:    fixture.buyerID,
				ProductID:  fixture.secondProductID,
				MerchantID: fixture.merchantID,
				Status:     state.status,
				IsOpen:     state.open,
			}
			if err := srv.DB.Create(&intent).Error; err != nil {
				t.Fatal("insert local buyer intent drift state")
			}

			beforeDigest := buyerIntentMySQLDigest(t, srv, intent.ID)
			var beforeLogs int64
			if err := srv.DB.Model(&model.OperationLog{}).Count(&beforeLogs).Error; err != nil {
				t.Fatal("count operation logs before local drift create")
			}
			response := createBuyerIntentMySQL(t, srv, map[string]string{
				"Authorization": "Bearer " + fixture.buyerToken,
				"X-Device-Id":   fixture.buyerDevice,
			}, fixture.secondProductID, fmt.Sprintf("f11-local-drift-%d", index))
			requireBuyerIntentMySQLStatus(t, "local drift buyer create", response, http.StatusInternalServerError, 20001)
			if afterDigest := buyerIntentMySQLDigest(t, srv, intent.ID); afterDigest != beforeDigest {
				t.Fatal("local drift buyer create mutated the isolated buyer intent row")
			}
			var afterLogs int64
			if err := srv.DB.Model(&model.OperationLog{}).Count(&afterLogs).Error; err != nil || afterLogs != beforeLogs {
				t.Fatal("local drift buyer create changed isolated operation logs")
			}
		})
	}
}

type buyerIntentMySQLResponse struct {
	HTTPStatus int
	Code       int
	intentID   uint64
	err        error
}

type buyerIntentMySQLFixture struct {
	merchantID        uint64
	merchantAccountID uint64
	merchantToken     string
	categoryID        uint64
	productID         uint64
	secondProductID   uint64
	buyerID           uint64
	buyerToken        string
	buyerDevice       string
	secondBuyerID     uint64
	secondBuyerToken  string
	secondBuyerDevice string
}

func buyerIntentMySQLRequest(t *testing.T, srv *app.Server, method, path string, body interface{}, headers map[string]string) buyerIntentMySQLResponse {
	t.Helper()
	response, err := executeJSONRequest(srv.Router, method, path, body, headers)
	if err != nil {
		t.Fatal("execute buyer intent MySQL acceptance request")
	}
	return buyerIntentMySQLResponse{HTTPStatus: response.HTTPStatus, Code: response.Code, intentID: numToUint64(response.Data["intent_id"])}
}

func requireBuyerIntentMySQLStatus(t *testing.T, label string, response buyerIntentMySQLResponse, wantHTTP, wantCode int) {
	t.Helper()
	if response.HTTPStatus != wantHTTP || response.Code != wantCode {
		t.Fatalf("%s status/code did not match", label)
	}
}

func createBuyerIntentMySQL(t *testing.T, srv *app.Server, headers map[string]string, productID uint64, contactWechat string) buyerIntentMySQLResponse {
	t.Helper()
	return buyerIntentMySQLRequest(t, srv, http.MethodPost, "/api/v1/buyer/intents", map[string]interface{}{
		"product_id": productID, "contact_wechat": contactWechat,
	}, headers)
}

func createBuyerIntentMySQLFixture(t *testing.T, srv *app.Server) buyerIntentMySQLFixture {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	hash, err := bcrypt.GenerateFromPassword([]byte("F11-Merchant-Password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal("hash isolated buyer intent merchant password")
	}
	merchant := model.Merchant{
		MerchantNo: "F11M" + suffix, MerchantName: "F11 Merchant", ContactName: "F11 Contact",
		ContactPhone: "13900000000", ReviewStatus: model.ReviewApproved,
	}
	if err := srv.DB.Create(&merchant).Error; err != nil {
		t.Fatal("create isolated buyer intent merchant")
	}
	account := model.MerchantAccount{
		MerchantID: merchant.ID, Username: "f11m_" + suffix, PasswordHash: string(hash),
		Role: model.AccountRoleOwner, Status: model.AccountStatusActive,
	}
	if err := srv.DB.Create(&account).Error; err != nil {
		t.Fatal("create isolated buyer intent merchant account")
	}
	category := model.Category{
		Name: "F11 Category " + suffix, Level: 2, Status: model.CategoryEnabled, Sort: 1,
	}
	if err := srv.DB.Create(&category).Error; err != nil {
		t.Fatal("create isolated buyer intent category")
	}
	price, originalPrice := 10000, 12000
	product := model.Product{
		ProductNo: "F11P" + suffix, MerchantID: merchant.ID, Title: "F11 Product", Description: "isolated product",
		CategoryID: category.ID, PriceCent: price, OriginalPriceCent: &originalPrice, ConditionLevel: "GOOD", Stock: 2,
		Status: model.ProductOnShelf, CreatedBy: account.ID, UpdatedBy: account.ID, Version: 1,
	}
	if err := srv.DB.Create(&product).Error; err != nil {
		t.Fatal("create isolated buyer intent product")
	}
	secondProduct := product
	secondProduct.ID = 0
	secondProduct.ProductNo = "F11Q" + suffix
	secondProduct.Title = "F11 Second Product"
	if err := srv.DB.Create(&secondProduct).Error; err != nil {
		t.Fatal("create isolated second buyer intent product")
	}

	merchantResponse, err := executeJSONRequest(srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": model.UserTypeMerchant, "username": account.Username, "password": "F11-Merchant-Password",
	}, nil)
	if err != nil || merchantResponse.HTTPStatus != http.StatusOK || merchantResponse.Code != 0 {
		t.Fatal("obtain isolated merchant session")
	}
	merchantToken := str(merchantResponse.Data["access_token"])
	if merchantToken == "" {
		t.Fatal("isolated merchant session omitted access token")
	}

	firstBuyer := createBuyerIntentMySQLBuyer(t, srv, "f11-buyer-"+suffix, "f11-device-a-"+suffix)
	secondBuyer := createBuyerIntentMySQLBuyer(t, srv, "f11-second-buyer-"+suffix, "f11-device-b-"+suffix)
	return buyerIntentMySQLFixture{
		merchantID: merchant.ID, merchantAccountID: account.ID, merchantToken: merchantToken,
		categoryID: category.ID, productID: product.ID, secondProductID: secondProduct.ID,
		buyerID: firstBuyer.id, buyerToken: firstBuyer.token, buyerDevice: firstBuyer.device,
		secondBuyerID: secondBuyer.id, secondBuyerToken: secondBuyer.token, secondBuyerDevice: secondBuyer.device,
	}
}

type buyerIntentMySQLBuyer struct {
	id     uint64
	token  string
	device string
}

func createBuyerIntentMySQLBuyer(t *testing.T, srv *app.Server, code, device string) buyerIntentMySQLBuyer {
	t.Helper()
	response, err := executeJSONRequest(srv.Router, http.MethodPost, "/api/v1/buyer/auth/wechat-login", map[string]interface{}{
		"code": code, "device_id": device, "nickname": "F11 Buyer",
	}, nil)
	if err != nil || response.Code != 0 {
		t.Fatal("create isolated buyer session")
	}
	user, ok := response.Data["user"].(map[string]interface{})
	if !ok {
		t.Fatal("isolated buyer session omitted user")
	}
	buyerID := numToUint64(user["id"])
	token := str(response.Data["access_token"])
	if buyerID == 0 || token == "" {
		t.Fatal("isolated buyer session fields were invalid")
	}
	return buyerIntentMySQLBuyer{id: buyerID, token: token, device: device}
}

func cleanupBuyerIntentMySQLFixture(srv *app.Server, fixture buyerIntentMySQLFixture) {
	productIDs := []uint64{fixture.productID, fixture.secondProductID}
	buyerIDs := []uint64{fixture.buyerID, fixture.secondBuyerID}
	_ = srv.DB.Where("buyer_id IN ? OR product_id IN ?", buyerIDs, productIDs).Delete(&model.BuyerIntent{}).Error
	_ = srv.DB.Where("merchant_id = ?", fixture.merchantID).Delete(&model.OperationLog{}).Error
	_ = srv.DB.Where("user_type = ? AND user_id = ?", model.UserTypeMerchant, fixture.merchantAccountID).Delete(&model.AuthSession{}).Error
	_ = srv.DB.Where("user_type = ? AND user_id IN ?", model.UserTypeBuyer, buyerIDs).Delete(&model.AuthSession{}).Error
	_ = srv.DB.Where("buyer_id IN ?", buyerIDs).Delete(&model.BuyerDeviceBinding{}).Error
	_ = srv.DB.Unscoped().Where("id IN ?", buyerIDs).Delete(&model.BuyerUser{}).Error
	_ = srv.DB.Where("product_id IN ?", productIDs).Delete(&model.ProductImage{}).Error
	_ = srv.DB.Unscoped().Where("id IN ?", productIDs).Delete(&model.Product{}).Error
	_ = srv.DB.Unscoped().Where("id = ?", fixture.categoryID).Delete(&model.Category{}).Error
	_ = srv.DB.Where("merchant_id = ?", fixture.merchantID).Delete(&model.MerchantAuditLog{}).Error
	_ = srv.DB.Unscoped().Where("id = ?", fixture.merchantAccountID).Delete(&model.MerchantAccount{}).Error
	_ = srv.DB.Unscoped().Where("id = ?", fixture.merchantID).Delete(&model.Merchant{}).Error
}

type buyerIntentMySQLColumn struct {
	DataType             string `gorm:"column:data_type"`
	ColumnType           string `gorm:"column:column_type"`
	IsNullable           string `gorm:"column:is_nullable"`
	GenerationExpression string `gorm:"column:generation_expression"`
	Extra                string `gorm:"column:extra"`
	IsGenerated          string `gorm:"column:is_generated"`
}

type buyerIntentMySQLIndexColumn struct {
	IndexName  string `gorm:"column:index_name"`
	NonUnique  int    `gorm:"column:non_unique"`
	Sequence   int    `gorm:"column:seq_in_index"`
	ColumnName string `gorm:"column:column_name"`
}

func assertBuyerIntentMySQLSchema(t *testing.T, srv *app.Server) {
	t.Helper()
	var columns []buyerIntentMySQLColumn
	if err := srv.DB.Raw(`
			SELECT data_type AS data_type,
				column_type AS column_type,
				is_nullable AS is_nullable,
				generation_expression AS generation_expression,
				extra AS extra,
				CASE WHEN generation_expression IS NOT NULL AND generation_expression <> '' THEN 'ALWAYS' ELSE 'NEVER' END AS is_generated
			FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = 'buyer_intents' AND column_name = 'open_marker'`).Scan(&columns).Error; err != nil || len(columns) != 1 {
		t.Fatal("inspect MySQL buyer intent open marker")
	}
	marker := columns[0]
	if !strings.EqualFold(marker.DataType, "tinyint") || !strings.EqualFold(marker.ColumnType, "tinyint") ||
		!strings.EqualFold(marker.IsNullable, "YES") || !strings.EqualFold(marker.IsGenerated, "ALWAYS") ||
		!strings.Contains(strings.ToUpper(marker.Extra), "STORED GENERATED") ||
		normalizeBuyerIntentMySQLExpression(marker.GenerationExpression) != "casewhenis_open=1then1elsenullend" {
		t.Fatal("MySQL buyer intent open marker did not match the final generated schema")
	}

	var rows []buyerIntentMySQLIndexColumn
	if err := srv.DB.Raw(`
			SELECT index_name AS index_name,
				non_unique AS non_unique,
				seq_in_index AS seq_in_index,
				column_name AS column_name
			FROM information_schema.statistics
			WHERE table_schema = DATABASE() AND table_name = 'buyer_intents'
		ORDER BY index_name, seq_in_index`).Scan(&rows).Error; err != nil {
		t.Fatal("inspect MySQL buyer intent indexes")
	}
	indexes := map[string][]buyerIntentMySQLIndexColumn{}
	for _, row := range rows {
		indexes[row.IndexName] = append(indexes[row.IndexName], row)
	}
	open := indexes["uk_buyer_intent_open"]
	if len(open) != 3 || open[0].NonUnique != 0 || open[1].NonUnique != 0 || open[2].NonUnique != 0 ||
		open[0].Sequence != 1 || open[1].Sequence != 2 || open[2].Sequence != 3 ||
		open[0].ColumnName != "buyer_id" || open[1].ColumnName != "product_id" || open[2].ColumnName != "open_marker" {
		t.Fatal("MySQL buyer intent open key did not match the final unique index")
	}
	if len(indexes["uk_buyer_product_open"]) != 0 {
		t.Fatal("legacy MySQL buyer intent open key remains")
	}
	lookalikes := 0
	for name, index := range indexes {
		if name == "PRIMARY" || name == "uk_buyer_intent_open" {
			continue
		}
		unique, buyer, product := true, false, false
		for _, column := range index {
			unique = unique && column.NonUnique == 0
			buyer = buyer || column.ColumnName == "buyer_id"
			product = product || column.ColumnName == "product_id"
		}
		if unique && buyer && product {
			lookalikes++
		}
	}
	if lookalikes != 0 {
		t.Fatal("MySQL buyer intent relevant unique lookalike key remains")
	}

	dryRun := srv.DB.Session(&gorm.Session{DryRun: true}).Create(&model.BuyerIntent{
		IntentNo: "F11-DRY-RUN", BuyerID: 1, ProductID: 1, MerchantID: 1, Status: model.IntentNew, IsOpen: true,
	})
	if dryRun.Error != nil || strings.Contains(strings.ToLower(dryRun.Statement.SQL.String()), "open_marker") {
		t.Fatal("GORM buyer intent create attempted to write the generated open marker")
	}
}

func normalizeBuyerIntentMySQLExpression(value string) string {
	replacer := strings.NewReplacer("`", "", " ", "", "\t", "", "\n", "", "\r", "", "(", "", ")", "")
	return strings.ToLower(replacer.Replace(value))
}

func assertBuyerIntentMySQLStates(t *testing.T, srv *app.Server, buyerID, productID uint64, wantClosed, wantOpen int) (int, int) {
	t.Helper()
	var intents []model.BuyerIntent
	if err := srv.DB.Where("buyer_id = ? AND product_id = ?", buyerID, productID).Order("id").Find(&intents).Error; err != nil {
		t.Fatal("load isolated buyer intent histories")
	}
	closed, open := 0, 0
	for _, intent := range intents {
		switch {
		case intent.Status == model.IntentClosed && !intent.IsOpen:
			closed++
		case intent.Status == model.IntentNew && intent.IsOpen:
			open++
		default:
			t.Fatal("buyer intent history state changed unexpectedly")
		}
	}
	if closed != wantClosed || open != wantOpen {
		t.Fatal("buyer intent history and open counts did not match")
	}
	return closed, open
}

func assertBuyerIntentMySQLDriftFailsClosed(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture, merchantHeaders map[string]string) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	driftBuyer := createBuyerIntentMySQLBuyer(t, srv, "f11-drift-buyer-"+suffix, "f11-drift-device-"+suffix)
	driftBuyerHeaders := map[string]string{
		"Authorization": "Bearer " + driftBuyer.token,
		"X-Device-Id":   driftBuyer.device,
	}
	t.Cleanup(func() {
		_ = srv.DB.Where("buyer_id = ?", driftBuyer.id).Delete(&model.BuyerIntent{}).Error
		_ = srv.DB.Where("user_type = ? AND user_id = ?", model.UserTypeBuyer, driftBuyer.id).Delete(&model.AuthSession{}).Error
		_ = srv.DB.Where("buyer_id = ?", driftBuyer.id).Delete(&model.BuyerDeviceBinding{}).Error
		_ = srv.DB.Unscoped().Where("id = ?", driftBuyer.id).Delete(&model.BuyerUser{}).Error
	})
	states := []struct {
		status string
		open   bool
	}{
		{status: model.IntentClosed, open: true},
		{status: "BOGUS", open: false},
	}
	for index, state := range states {
		intentNo := fmt.Sprintf("F11D%d%d", index, time.Now().UnixNano())
		if result := srv.DB.Exec(`
			INSERT INTO buyer_intents (intent_no, buyer_id, product_id, merchant_id, status, is_open, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW(3), NOW(3))`, intentNo, driftBuyer.id, fixture.secondProductID, fixture.merchantID, state.status, state.open); result.Error != nil || result.RowsAffected != 1 {
			t.Fatal("insert isolated MySQL buyer intent drift state")
		}
		var intent model.BuyerIntent
		if err := srv.DB.Where("intent_no = ?", intentNo).First(&intent).Error; err != nil {
			t.Fatal("load isolated MySQL buyer intent drift state")
		}
		calls := []func() buyerIntentMySQLResponse{
			func() buyerIntentMySQLResponse {
				return createBuyerIntentMySQL(t, srv, driftBuyerHeaders, fixture.secondProductID, fmt.Sprintf("f11-drift-create-%d", index))
			},
			func() buyerIntentMySQLResponse {
				return buyerIntentMySQLRequest(t, srv, http.MethodGet, "/api/v1/buyer/intents", nil, driftBuyerHeaders)
			},
			func() buyerIntentMySQLResponse {
				return buyerIntentMySQLRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/buyer/intents/%d", intent.ID), nil, driftBuyerHeaders)
			},
			func() buyerIntentMySQLResponse {
				return buyerIntentMySQLRequest(t, srv, http.MethodGet, "/api/v1/merchant/intents", nil, merchantHeaders)
			},
			func() buyerIntentMySQLResponse {
				return buyerIntentMySQLRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/merchant/intents/%d", intent.ID), nil, merchantHeaders)
			},
			func() buyerIntentMySQLResponse {
				return buyerIntentMySQLRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/merchant/intents/%d/contacted", intent.ID), map[string]interface{}{}, merchantHeaders)
			},
			func() buyerIntentMySQLResponse {
				return buyerIntentMySQLRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/merchant/intents/%d/close", intent.ID), map[string]interface{}{"reason": "NO_RESPONSE"}, merchantHeaders)
			},
		}
		for _, call := range calls {
			beforeDigest := buyerIntentMySQLDigest(t, srv, intent.ID)
			var beforeLogs int64
			if err := srv.DB.Model(&model.OperationLog{}).Count(&beforeLogs).Error; err != nil {
				t.Fatal("count operation logs before drift request")
			}
			response := call()
			requireBuyerIntentMySQLStatus(t, "drift request", response, http.StatusInternalServerError, 20001)
			if afterDigest := buyerIntentMySQLDigest(t, srv, intent.ID); afterDigest != beforeDigest {
				t.Fatal("drift request mutated the isolated buyer intent row")
			}
			var afterLogs int64
			if err := srv.DB.Model(&model.OperationLog{}).Count(&afterLogs).Error; err != nil || afterLogs != beforeLogs {
				t.Fatal("drift request changed isolated operation logs")
			}
		}
		if result := srv.DB.Model(&model.BuyerIntent{}).Where("id = ?", intent.ID).Updates(map[string]interface{}{
			"status": model.IntentClosed, "is_open": false,
		}); result.Error != nil || result.RowsAffected != 1 {
			t.Fatal("restore isolated MySQL buyer intent drift state")
		}
	}
}

func buyerIntentMySQLDigest(t *testing.T, srv *app.Server, intentID uint64) [sha256.Size]byte {
	t.Helper()
	var intent model.BuyerIntent
	if err := srv.DB.Where("id = ?", intentID).First(&intent).Error; err != nil {
		t.Fatal("load isolated buyer intent digest")
	}
	raw, err := json.Marshal(intent)
	if err != nil {
		t.Fatal("marshal isolated buyer intent digest")
	}
	return sha256.Sum256(raw)
}
