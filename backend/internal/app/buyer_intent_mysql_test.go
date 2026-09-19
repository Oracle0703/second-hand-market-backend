package app

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	mysqlcfg "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/model"
)

// These tests own only a disposable CI database on loopback. They never load
// application configuration or accept a production database name.
func newBuyerIntentMySQLDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("BUYER_INTENT_MYSQL_TEST") != "1" {
		t.Skip("set BUYER_INTENT_MYSQL_TEST=1 for the isolated MySQL suite")
	}
	dsn := os.Getenv("BUYER_INTENT_MYSQL_DSN")
	cfg, err := mysqlcfg.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid isolated MySQL DSN")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || (host != "127.0.0.1" && host != "localhost") || cfg.DBName != "buyer_intent_ci" {
		t.Fatal("buyer intent tests require loopback TCP and database buyer_intent_ci")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot connect to isolated MySQL")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(16)
	var version string
	if err := db.Raw("SELECT VERSION()").Scan(&version).Error; err != nil || (!strings.HasPrefix(version, "8.0.") && !strings.HasPrefix(version, "8.4.")) {
		t.Fatalf("requires MySQL 8.0 or 8.4, got %q (%v)", version, err)
	}
	t.Logf("isolated MySQL %s", version)
	t.Cleanup(func() {
		for _, name := range []string{"buyer_intents", "products", "merchants", "operation_logs", "idempotency_records"} {
			if err := db.Migrator().DropTable(name); err != nil {
				t.Errorf("clean up fixture %s: %v", name, err)
			}
		}
		_ = pool.Close()
	})
	return db
}

func resetBuyerIntentMySQLFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Migrator().DropTable("buyer_intents"); err != nil {
		t.Fatal(err)
	}
	// Use the actual legacy table definition, including its column order and
	// indexes, rather than a separately maintained approximation.
	source, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0002_buyer_domain.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	_, table, ok := strings.Cut(string(source), "CREATE TABLE IF NOT EXISTS buyer_intents (")
	if !ok {
		t.Fatal("legacy buyer intent definition missing")
	}
	if err := db.Exec("CREATE TABLE IF NOT EXISTS buyer_intents (" + table).Error; err != nil {
		t.Fatal(err)
	}
}

// MySQL's DELIMITER directive is a client command. Send each procedure body
// as one statement, so the exact checked-in SQL runs in both CLI and CI.
func runBuyerIntentSQL(db *gorm.DB, suffix string) error {
	source, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0011_buyer_intent_open_uniqueness."+suffix+".sql"))
	if err != nil {
		return err
	}
	statements, err := databasecmd.SplitSQLStatements(string(source))
	if err != nil {
		return err
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyBuyerIntentSQL(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, suffix := range []string{"preflight", "up", "postflight"} {
		if err := runBuyerIntentSQL(db, suffix); err != nil {
			t.Fatalf("%s: %v", suffix, err)
		}
	}
}

func TestBuyerIntentMySQLMigration(t *testing.T) {
	db := newBuyerIntentMySQLDB(t)
	t.Run("legacy_history_replay_and_auto_migrate", func(t *testing.T) {
		resetBuyerIntentMySQLFixture(t, db)
		original := model.BuyerIntent{IntentNo: "legacy", BuyerID: 10, ProductID: 20, MerchantID: 7, Status: model.IntentNew, IsOpen: true}
		if err := db.Create(&original).Error; err != nil {
			t.Fatal(err)
		}
		applyBuyerIntentSQL(t, db)
		var preserved model.BuyerIntent
		if err := db.First(&preserved, original.ID).Error; err != nil {
			t.Fatal(err)
		}
		if preserved.IntentNo != original.IntentNo || preserved.BuyerID != 10 || !preserved.IsOpen || preserved.OpenMarker == nil || *preserved.OpenMarker != 1 {
			t.Fatalf("legacy intent was not preserved: %+v", preserved)
		}
		for cycle := 0; cycle < 3; cycle++ {
			if err := db.Model(&original).Updates(map[string]interface{}{"status": model.IntentClosed, "is_open": false, "closed_at": time.Now()}).Error; err != nil {
				t.Fatalf("cycle %d close: %v", cycle, err)
			}
			original = model.BuyerIntent{IntentNo: fmt.Sprintf("cycle-%d", cycle), BuyerID: 10, ProductID: 20, MerchantID: 7, Status: model.IntentNew, IsOpen: true}
			if err := db.Create(&original).Error; err != nil {
				t.Fatal(err)
			}
		}
		duplicate := original
		duplicate.ID, duplicate.IntentNo = 0, "duplicate-open"
		if err := db.Create(&duplicate).Error; !isIdempotencyDuplicate(err) {
			t.Fatalf("second open intent must fail with unique constraint, got %v", err)
		}
		// The nullable marker is computed even when inserted through GORM.
		var bad int64
		if err := db.Model(&model.BuyerIntent{}).Where("is_open = 0 AND open_marker IS NOT NULL").Count(&bad).Error; err != nil || bad != 0 {
			t.Fatalf("closed markers: %d, %v", bad, err)
		}
		for repeat := 0; repeat < 2; repeat++ {
			if err := db.AutoMigrate(&model.BuyerIntent{}); err != nil {
				t.Fatal(err)
			}
			if err := migrateBuyerIntentOpenUniqueness(db); err != nil {
				t.Fatal(err)
			}
			applyBuyerIntentSQL(t, db)
		}
	})
	t.Run("fresh_development_database", func(t *testing.T) {
		if err := db.Migrator().DropTable("buyer_intents"); err != nil {
			t.Fatal(err)
		}
		if err := db.AutoMigrate(&model.BuyerIntent{}); err != nil {
			t.Fatal(err)
		}
		if err := migrateBuyerIntentOpenUniqueness(db); err != nil {
			t.Fatal(err)
		}
		if err := verifyBuyerIntentOpenUniqueness(db); err != nil {
			t.Fatal(err)
		}
	})
	for _, both := range []bool{false, true} {
		t.Run(fmt.Sprintf("resume_with_new_index_%v", both), func(t *testing.T) {
			resetBuyerIntentMySQLFixture(t, db)
			if err := db.Exec("ALTER TABLE buyer_intents ADD COLUMN open_marker TINYINT GENERATED ALWAYS AS (CASE WHEN is_open = 1 THEN 1 ELSE NULL END) STORED AFTER is_open").Error; err != nil {
				t.Fatal(err)
			}
			if both {
				if err := db.Exec("ALTER TABLE buyer_intents ADD UNIQUE KEY uk_buyer_intent_open (buyer_id, product_id, open_marker)").Error; err != nil {
					t.Fatal(err)
				}
			}
			applyBuyerIntentSQL(t, db)
		})
	}
	for _, tc := range []struct{ name, drift string }{
		{"invalid_state", "INSERT INTO buyer_intents (intent_no,buyer_id,product_id,merchant_id,status,is_open) VALUES ('bad',1,2,7,'CLOSED',1)"},
		{"case_sensitive_status", "INSERT INTO buyer_intents (intent_no,buyer_id,product_id,merchant_id,status,is_open) VALUES ('bad',1,2,7,'new',1)"},
		{"trailing_status_space", "INSERT INTO buyer_intents (intent_no,buyer_id,product_id,merchant_id,status,is_open) VALUES ('bad',1,2,7,'NEW ',1)"},
		{"ordinary_marker", "ALTER TABLE buyer_intents ADD COLUMN open_marker TINYINT NULL"},
		{"wrong_index", "ALTER TABLE buyer_intents DROP INDEX uk_buyer_product_open, ADD INDEX uk_buyer_product_open (buyer_id,product_id,is_open)"},
		{"extra_history_constraint", "ALTER TABLE buyer_intents ADD UNIQUE INDEX unexpected_history (buyer_id,product_id)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetBuyerIntentMySQLFixture(t, db)
			if err := db.Exec(tc.drift).Error; err != nil {
				t.Fatal(err)
			}
			before := buyerIntentMySQLSnapshot(t, db)
			for _, suffix := range []string{"preflight", "up", "postflight"} {
				if err := runBuyerIntentSQL(db, suffix); err == nil {
					t.Fatalf("%s accepted %s", suffix, tc.name)
				}
			}
			if after := buyerIntentMySQLSnapshot(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected migration changed schema or business rows")
			}
		})
	}
}

func buyerIntentMySQLSnapshot(t *testing.T, db *gorm.DB) interface{} {
	t.Helper()
	var ddl []map[string]interface{}
	var rows []model.BuyerIntent
	if err := db.Raw("SHOW CREATE TABLE buyer_intents").Scan(&ddl).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return []interface{}{ddl, rows}
}

func TestBuyerIntentMySQLConcurrentLifecycle(t *testing.T) {
	db := newBuyerIntentMySQLDB(t)
	resetBuyerIntentMySQLFixture(t, db)
	applyBuyerIntentSQL(t, db)
	if err := db.AutoMigrate(&model.Product{}, &model.Merchant{}, &model.OperationLog{}, &model.IdempotencyRecord{}); err != nil {
		t.Fatal(err)
	}
	m := model.Merchant{ID: 7, MerchantNo: "intent-ci", MerchantName: "Synthetic shop"}
	p := model.Product{ProductNo: "intent-product", MerchantID: m.ID, Status: model.ProductOnShelf, Stock: 5}
	for _, row := range []interface{}{&m, &p} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{DB: db, limiter: newMemoryRateLimiter()}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/buyer/intents", func(c *gin.Context) {
		common.SetActor(c, common.Actor{UserID: 10, UserType: model.UserTypeBuyer})
		s.handleBuyerIntentCreate(c)
	})
	merchant := r.Group("/merchant")
	merchant.Use(func(c *gin.Context) {
		common.SetActor(c, common.Actor{UserID: 42, UserType: model.UserTypeMerchant, MerchantID: m.ID})
	})
	merchant.POST("/intents/:id/close", s.handleMerchantIntentClose)
	merchant.POST("/intents/:id/contacted", s.handleMerchantIntentContacted)

	createResults := concurrentBuyerIntentRequests(4, func(i int) p1Response {
		return p1Request(r, http.MethodPost, "/buyer/intents?merchant_no=intent-ci", fmt.Sprintf(`{"product_id":%d,"contact_phone":"13800138000"}`, p.ID), fmt.Sprintf("create-%d", i))
	})
	winners := 0
	var intentID uint64
	for _, result := range createResults {
		switch result.Code {
		case common.CodeOK:
			winners++
			intentID = uint64(result.Data["intent_id"].(float64))
		case common.CodeConflict:
		default:
			t.Fatalf("concurrent create: %+v", result)
		}
	}
	if winners != 1 {
		t.Fatalf("create winners = %d, want 1", winners)
	}
	closePath := fmt.Sprintf("/merchant/intents/%d/close", intentID)
	closed := concurrentBuyerIntentRequests(8, func(i int) p1Response {
		return p1Request(r, http.MethodPost, closePath, `{"reason":"NO_RESPONSE"}`, fmt.Sprintf("close-%d", i))
	})
	transitions := 0
	for _, result := range closed {
		if result.Code != common.CodeOK {
			t.Fatalf("concurrent close: %+v", result)
		}
		if result.Data["idempotent"] == false {
			transitions++
		}
	}
	if transitions != 1 {
		t.Fatalf("close transitions = %d, want 1", transitions)
	}
	// Repeated contact/close races must end CLOSED, never CONTACTED/is_open=0.
	for cycle := 0; cycle < 5; cycle++ {
		intent := model.BuyerIntent{IntentNo: fmt.Sprintf("race-%d", cycle), BuyerID: 10, ProductID: p.ID, MerchantID: m.ID, Status: model.IntentNew, IsOpen: true}
		if err := db.Create(&intent).Error; err != nil {
			t.Fatal(err)
		}
		results := concurrentBuyerIntentRequests(2, func(i int) p1Response {
			action := "close"
			if i == 1 {
				action = "contacted"
			}
			return p1Request(r, http.MethodPost, fmt.Sprintf("/merchant/intents/%d/%s", intent.ID, action), `{}`, "")
		})
		if results[0].Code != common.CodeOK || (results[1].Code != common.CodeOK && results[1].Code != common.CodeInvalidTransition) {
			t.Fatalf("contact/close race: %+v", results)
		}
		if err := db.First(&intent, intent.ID).Error; err != nil || intent.Status != model.IntentClosed || intent.IsOpen || intent.ClosedAt == nil {
			t.Fatalf("invalid final state: %+v (%v)", intent, err)
		}
	}
	var closeLogs, histories int64
	if err := db.Model(&model.OperationLog{}).Where("action = ?", "merchant_intent_close").Count(&closeLogs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.BuyerIntent{}).Where("status = ? AND is_open = ?", model.IntentClosed, false).Count(&histories).Error; err != nil {
		t.Fatal(err)
	}
	if closeLogs != 6 || histories != 6 {
		t.Fatalf("close logs=%d histories=%d, want 6/6", closeLogs, histories)
	}
}

func concurrentBuyerIntentRequests(n int, request func(int) p1Response) []p1Response {
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]p1Response, n)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = request(i)
		}(i)
	}
	close(start)
	wg.Wait()
	return results
}
