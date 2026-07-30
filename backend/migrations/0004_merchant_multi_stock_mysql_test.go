//go:build mysqlacceptance

package migrations_test

import (
	"database/sql"
	"net"
	"os"
	"regexp"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

var issue16SchemaName = regexp.MustCompile(`\Ashm_issue16_[a-z0-9_]+\z`)

func TestMerchantMultiStockMySQL84Matrix(t *testing.T) {
	db, driverConfig := openIsolatedIssue16MySQL(t)
	t.Cleanup(func() {
		dropIssue16Tables(t, db)
		_ = db.Close()
	})

	t.Run("clean_schema_backfill_constraints_and_auto_migrate", func(t *testing.T) {
		resetLegacyIssue16Schema(t, db)
		productID := insertLegacyFixture(t, db)

		execMigrationArtifact(t, db, "preflight.sql")
		execMigrationArtifact(t, db, "up.sql")
		execMigrationArtifact(t, db, "postflight.sql")

		var stock, reservedStock, quantity int
		if err := db.QueryRow(
			"SELECT stock, reserved_stock FROM products WHERE id = ?",
			productID,
		).Scan(&stock, &reservedStock); err != nil {
			t.Fatalf("load migrated product: %v", err)
		}
		if err := db.QueryRow(
			"SELECT quantity FROM orders WHERE product_id = ?",
			productID,
		).Scan(&quantity); err != nil {
			t.Fatalf("load migrated order: %v", err)
		}
		if stock != 5 || reservedStock != 0 || quantity != 1 {
			t.Fatalf(
				"legacy backfill = stock:%d reserved:%d quantity:%d",
				stock,
				reservedStock,
				quantity,
			)
		}

		assertIssue16Index(t, db, "idx_product_active", 1, "product_id,is_active")
		assertIssue16IndexAbsent(t, db, "uk_product_active")

		gormDB, err := gorm.Open(
			gormmysql.New(gormmysql.Config{DSN: driverConfig.FormatDSN()}),
			&gorm.Config{},
		)
		if err != nil {
			t.Fatal("open isolated GORM database")
		}
		if err := gormDB.AutoMigrate(&model.Product{}, &model.Order{}); err != nil {
			t.Fatalf("AutoMigrate final schema: %v", err)
		}
		execMigrationArtifact(t, db, "postflight.sql")

		for _, active := range []int{1, 1, 0, 0} {
			status := model.OrderClosed
			if active == 1 {
				status = model.OrderCreated
			}
			if _, err := db.Exec(
				"INSERT INTO orders (product_id, quantity, status, is_active) VALUES (?, 1, ?, ?)",
				productID,
				status,
				active,
			); err != nil {
				t.Fatalf("insert duplicate active=%d order: %v", active, err)
			}
		}

		assertIssue16StatementFails(
			t,
			db,
			"UPDATE products SET stock = -1 WHERE id = ?",
			productID,
		)
		assertIssue16StatementFails(
			t,
			db,
			"UPDATE products SET reserved_stock = -1 WHERE id = ?",
			productID,
		)
		assertIssue16StatementFails(
			t,
			db,
			"UPDATE products SET reserved_stock = stock + 1 WHERE id = ?",
			productID,
		)
		assertIssue16StatementFails(
			t,
			db,
			"INSERT INTO orders (product_id, quantity, status, is_active) VALUES (?, 0, 'CLOSED', 0)",
			productID,
		)

		if err := gormDB.AutoMigrate(&model.Product{}, &model.Order{}); err != nil {
			t.Fatalf("repeat AutoMigrate with duplicate orders: %v", err)
		}
		if _, err := db.Exec(
			"UPDATE orders SET status = 'CLOSED', is_active = 0",
		); err != nil {
			t.Fatalf("quiesce duplicate-order fixture: %v", err)
		}
		execMigrationArtifact(t, db, "postflight.sql")
		assertIssue16Index(t, db, "idx_product_active", 1, "product_id,is_active")
		assertIssue16Index(t, db, "idx_product_id", 1, "product_id")
		assertIssue16IndexAbsent(t, db, "uk_product_active")
	})

	t.Run("active_and_locked_data_fail_before_ddl", func(t *testing.T) {
		resetLegacyIssue16Schema(t, db)
		result, err := db.Exec(
			"INSERT INTO products (stock, status, active_order_id, locked_at) VALUES (1, 'LOCKED', 41, CURRENT_TIMESTAMP)",
		)
		if err != nil {
			t.Fatalf("insert locked fixture: %v", err)
		}
		productID, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("load locked product ID: %v", err)
		}
		if _, err := db.Exec(
			"INSERT INTO orders (product_id, status, is_active) VALUES (?, 'CREATED', 1)",
			productID,
		); err != nil {
			t.Fatalf("insert active fixture: %v", err)
		}

		assertMigrationArtifactFails(t, db, "preflight.sql")
		assertMigrationArtifactFails(t, db, "up.sql")
		assertIssue16ColumnAbsent(t, db, "products", "reserved_stock")
		assertIssue16ColumnAbsent(t, db, "orders", "quantity")
		assertIssue16Index(t, db, "uk_product_active", 0, "product_id,is_active")

		resetLegacyIssue16Schema(t, db)
		productID = insertLegacyProduct(t, db, "ON_SHELF")
		if _, err := db.Exec(
			"INSERT INTO orders (product_id, status, is_active) VALUES (?, 'CREATED', 0)",
			productID,
		); err != nil {
			t.Fatalf("insert status mismatch fixture: %v", err)
		}
		assertMigrationArtifactFails(t, db, "preflight.sql")
		assertMigrationArtifactFails(t, db, "up.sql")
		assertIssue16ColumnAbsent(t, db, "products", "reserved_stock")
		assertIssue16ColumnAbsent(t, db, "orders", "quantity")
	})

	t.Run("status_values_are_byte_exact_and_fail_closed", func(t *testing.T) {
		scenarios := []struct {
			name   string
			target string
			status string
		}{
			{name: "lowercase_order", target: "orders", status: "closed"},
			{name: "trailing_space_order", target: "orders", status: "CLOSED "},
			{name: "unknown_order", target: "orders", status: "UNKNOWN"},
			{name: "lowercase_product", target: "products", status: "on_shelf"},
			{name: "trailing_space_product", target: "products", status: "ON_SHELF "},
			{name: "unknown_product", target: "products", status: "UNKNOWN"},
		}

		for _, scenario := range scenarios {
			t.Run(scenario.name+"_before_ddl", func(t *testing.T) {
				resetLegacyIssue16Schema(t, db)
				productStatus := model.ProductOnShelf
				orderStatus := model.OrderClosed
				if scenario.target == "products" {
					productStatus = scenario.status
				} else {
					orderStatus = scenario.status
				}
				productID := insertLegacyProduct(t, db, productStatus)
				if _, err := db.Exec(
					"INSERT INTO orders (product_id, status, is_active) VALUES (?, ?, 0)",
					productID,
					orderStatus,
				); err != nil {
					t.Fatalf("insert invalid status fixture: %v", err)
				}

				assertMigrationArtifactFails(t, db, "preflight.sql")
				assertMigrationArtifactFails(t, db, "up.sql")
				assertIssue16ColumnAbsent(t, db, "products", "reserved_stock")
				assertIssue16ColumnAbsent(t, db, "orders", "quantity")
			})

			t.Run(scenario.name+"_after_ddl", func(t *testing.T) {
				resetLegacyIssue16Schema(t, db)
				productID := insertLegacyFixture(t, db)
				execMigrationArtifact(t, db, "preflight.sql")
				execMigrationArtifact(t, db, "up.sql")
				execMigrationArtifact(t, db, "postflight.sql")

				var (
					statement string
					arguments []any
				)
				if scenario.target == "products" {
					statement = "UPDATE products SET status = ? WHERE id = ?"
					arguments = []any{scenario.status, productID}
				} else {
					statement = "UPDATE orders SET status = ? WHERE product_id = ?"
					arguments = []any{scenario.status, productID}
				}
				if _, err := db.Exec(statement, arguments...); err != nil {
					t.Fatalf("introduce invalid status drift: %v", err)
				}

				assertMigrationArtifactFails(t, db, "postflight.sql")
				assertMigrationArtifactFails(t, db, "down.sql")
				assertIssue16ColumnPresent(t, db, "products", "reserved_stock")
				assertIssue16ColumnPresent(t, db, "orders", "quantity")
			})
		}
	})

	t.Run("wrong_index_and_post_preflight_drift_fail_closed", func(t *testing.T) {
		resetLegacyIssue16Schema(t, db)
		insertLegacyFixture(t, db)
		execMigrationArtifact(t, db, "preflight.sql")

		if _, err := db.Exec(
			"ALTER TABLE orders DROP INDEX uk_product_active, ADD INDEX uk_product_active (product_id, is_active)",
		); err != nil {
			t.Fatalf("create wrong legacy index: %v", err)
		}
		assertMigrationArtifactFails(t, db, "preflight.sql")
		assertMigrationArtifactFails(t, db, "up.sql")
		assertIssue16ColumnAbsent(t, db, "products", "reserved_stock")
		assertIssue16ColumnAbsent(t, db, "orders", "quantity")
		assertIssue16Index(t, db, "uk_product_active", 1, "product_id,is_active")

		resetLegacyIssue16Schema(t, db)
		insertLegacyFixture(t, db)
		if _, err := db.Exec(
			"ALTER TABLE orders ADD UNIQUE INDEX uk_active_product (is_active, product_id)",
		); err != nil {
			t.Fatalf("create reversed equivalent unique index: %v", err)
		}
		assertMigrationArtifactFails(t, db, "preflight.sql")
		assertMigrationArtifactFails(t, db, "up.sql")
		assertIssue16ColumnAbsent(t, db, "products", "reserved_stock")
		assertIssue16ColumnAbsent(t, db, "orders", "quantity")

		resetLegacyIssue16Schema(t, db)
		insertLegacyFixture(t, db)
		execMigrationArtifact(t, db, "preflight.sql")
		execMigrationArtifact(t, db, "up.sql")
		if _, err := db.Exec(
			"ALTER TABLE orders ADD UNIQUE INDEX uk_active_product (is_active, product_id)",
		); err != nil {
			t.Fatalf("create post-migration equivalent unique index: %v", err)
		}
		assertMigrationArtifactFails(t, db, "postflight.sql")
	})

	t.Run("repeat_and_partial_schema_are_rejected", func(t *testing.T) {
		resetLegacyIssue16Schema(t, db)
		insertLegacyFixture(t, db)
		execMigrationArtifact(t, db, "preflight.sql")
		execMigrationArtifact(t, db, "up.sql")
		execMigrationArtifact(t, db, "postflight.sql")

		assertMigrationArtifactFails(t, db, "preflight.sql")
		assertMigrationArtifactFails(t, db, "up.sql")
		assertIssue16ColumnPresent(t, db, "products", "reserved_stock")
		assertIssue16ColumnPresent(t, db, "orders", "quantity")

		resetLegacyIssue16Schema(t, db)
		insertLegacyFixture(t, db)
		if _, err := db.Exec(
			"ALTER TABLE products ADD COLUMN reserved_stock INT NOT NULL DEFAULT 0 AFTER stock",
		); err != nil {
			t.Fatalf("create partial schema: %v", err)
		}
		assertMigrationArtifactFails(t, db, "preflight.sql")
		assertMigrationArtifactFails(t, db, "up.sql")
		assertIssue16ColumnPresent(t, db, "products", "reserved_stock")
		assertIssue16ColumnAbsent(t, db, "orders", "quantity")
		assertIssue16Index(t, db, "uk_product_active", 0, "product_id,is_active")
	})

	t.Run("limited_down_requires_default_data_and_never_restores_unique_index", func(t *testing.T) {
		resetLegacyIssue16Schema(t, db)
		productID := insertLegacyFixture(t, db)
		execMigrationArtifact(t, db, "preflight.sql")
		execMigrationArtifact(t, db, "up.sql")
		execMigrationArtifact(t, db, "postflight.sql")

		if _, err := db.Exec(
			"UPDATE products SET reserved_stock = 1 WHERE id = ?",
			productID,
		); err != nil {
			t.Fatalf("prepare reserved stock: %v", err)
		}
		if _, err := db.Exec(
			"UPDATE orders SET quantity = 2 WHERE product_id = ?",
			productID,
		); err != nil {
			t.Fatalf("prepare multi-stock order: %v", err)
		}
		assertMigrationArtifactFails(t, db, "down.sql")
		assertIssue16ColumnPresent(t, db, "products", "reserved_stock")
		assertIssue16ColumnPresent(t, db, "orders", "quantity")

		if _, err := db.Exec(
			"UPDATE products SET reserved_stock = 0 WHERE id = ?",
			productID,
		); err != nil {
			t.Fatalf("restore reserved stock: %v", err)
		}
		if _, err := db.Exec(
			"UPDATE orders SET quantity = 1 WHERE product_id = ?",
			productID,
		); err != nil {
			t.Fatalf("restore order quantity: %v", err)
		}
		execMigrationArtifact(t, db, "down.sql")
		assertIssue16ColumnAbsent(t, db, "products", "reserved_stock")
		assertIssue16ColumnAbsent(t, db, "orders", "quantity")
		assertIssue16Index(t, db, "idx_product_active", 1, "product_id,is_active")
		assertIssue16IndexAbsent(t, db, "uk_product_active")
		assertMigrationArtifactFails(t, db, "down.sql")
	})
}

func openIsolatedIssue16MySQL(t *testing.T) (*sql.DB, *mysqldriver.Config) {
	t.Helper()
	if os.Getenv("ISSUE16_MYSQL_ISOLATED") != "YES" {
		t.Fatal("ISSUE16_MYSQL_ISOLATED=YES is required")
	}
	rawDSN, exists := os.LookupEnv("ISSUE16_MYSQL_DSN")
	if !exists || rawDSN == "" {
		t.Fatal("ISSUE16_MYSQL_DSN is required")
	}
	driverConfig, err := mysqldriver.ParseDSN(rawDSN)
	if err != nil {
		t.Fatal("ISSUE16_MYSQL_DSN is invalid")
	}
	if !issue16SchemaName.MatchString(driverConfig.DBName) {
		t.Fatal("ISSUE16_MYSQL_DSN database name is invalid")
	}
	if !strings.HasPrefix(driverConfig.Net, "tcp") {
		t.Fatal("ISSUE16_MYSQL_DSN must use loopback TCP")
	}
	host, _, err := net.SplitHostPort(driverConfig.Addr)
	if err != nil {
		t.Fatal("ISSUE16_MYSQL_DSN address is invalid")
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		t.Fatal("ISSUE16_MYSQL_DSN must use a numeric loopback host")
	}
	driverConfig.MultiStatements = true
	driverConfig.ParseTime = true

	db, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatal("open isolated MySQL failed")
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal("connect to isolated MySQL failed")
	}

	var version, versionComment, databaseName string
	if err := db.QueryRow(
		"SELECT VERSION(), @@version_comment, DATABASE()",
	).Scan(&version, &versionComment, &databaseName); err != nil {
		_ = db.Close()
		t.Fatal("inspect isolated MySQL failed")
	}
	if !strings.HasPrefix(version, "8.4.") ||
		!strings.Contains(strings.ToLower(versionComment), "mysql") ||
		databaseName != driverConfig.DBName {
		_ = db.Close()
		t.Fatal("isolated database must be MySQL 8.4")
	}

	var tableCount int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()",
	).Scan(&tableCount); err != nil {
		_ = db.Close()
		t.Fatal("inspect isolated schema failed")
	}
	if tableCount != 0 {
		_ = db.Close()
		t.Fatal("isolated schema must start empty")
	}
	return db, driverConfig
}

func resetLegacyIssue16Schema(t *testing.T, db *sql.DB) {
	t.Helper()
	dropIssue16Tables(t, db)
	statements := []string{
		`CREATE TABLE products (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stock INT NOT NULL DEFAULT 1,
			status VARCHAR(16) NOT NULL,
			active_order_id BIGINT NULL,
			locked_at DATETIME NULL,
			INDEX idx_active_order (active_order_id)
		) ENGINE=InnoDB`,
		`CREATE TABLE orders (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			product_id BIGINT NOT NULL,
			status VARCHAR(16) NOT NULL,
			is_active TINYINT(1) NOT NULL,
			UNIQUE KEY uk_product_active (product_id, is_active),
			INDEX idx_product_id (product_id)
		) ENGINE=InnoDB`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
}

func insertLegacyFixture(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	productID := insertLegacyProduct(t, db, "ON_SHELF")
	if _, err := db.Exec(
		"INSERT INTO orders (product_id, status, is_active) VALUES (?, 'CLOSED', 0)",
		productID,
	); err != nil {
		t.Fatalf("insert legacy order: %v", err)
	}
	return productID
}

func insertLegacyProduct(t *testing.T, db *sql.DB, status string) int64 {
	t.Helper()
	result, err := db.Exec(
		"INSERT INTO products (stock, status) VALUES (5, ?)",
		status,
	)
	if err != nil {
		t.Fatalf("insert legacy product: %v", err)
	}
	productID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("load legacy product ID: %v", err)
	}
	return productID
}

func dropIssue16Tables(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("DROP TABLE IF EXISTS orders, products"); err != nil {
		t.Fatalf("clean isolated schema: %v", err)
	}
}

func execMigrationArtifact(t *testing.T, db *sql.DB, suffix string) {
	t.Helper()
	if _, err := db.Exec(loadMigrationArtifact(t, suffix)); err != nil {
		t.Fatalf("execute %s: %v", suffix, err)
	}
}

func assertMigrationArtifactFails(t *testing.T, db *sql.DB, suffix string) {
	t.Helper()
	if _, err := db.Exec(loadMigrationArtifact(t, suffix)); err == nil {
		t.Fatalf("%s unexpectedly succeeded", suffix)
	}
}

func assertIssue16StatementFails(
	t *testing.T,
	db *sql.DB,
	statement string,
	arguments ...any,
) {
	t.Helper()
	if _, err := db.Exec(statement, arguments...); err == nil {
		t.Fatalf("invalid statement unexpectedly succeeded: %s", statement)
	}
}

func assertIssue16ColumnPresent(t *testing.T, db *sql.DB, tableName, columnName string) {
	t.Helper()
	assertIssue16ColumnCount(t, db, tableName, columnName, 1)
}

func assertIssue16ColumnAbsent(t *testing.T, db *sql.DB, tableName, columnName string) {
	t.Helper()
	assertIssue16ColumnCount(t, db, tableName, columnName, 0)
}

func assertIssue16ColumnCount(
	t *testing.T,
	db *sql.DB,
	tableName string,
	columnName string,
	expected int,
) {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND COLUMN_NAME = ?`,
		tableName,
		columnName,
	).Scan(&count); err != nil {
		t.Fatalf("inspect %s.%s: %v", tableName, columnName, err)
	}
	if count != expected {
		t.Fatalf("%s.%s count = %d, want %d", tableName, columnName, count, expected)
	}
}

func assertIssue16Index(
	t *testing.T,
	db *sql.DB,
	indexName string,
	nonUnique int,
	columns string,
) {
	t.Helper()
	var count, minimumNonUnique, maximumNonUnique int
	var actualColumns string
	if err := db.QueryRow(
		`SELECT
			COUNT(*),
			COALESCE(MIN(NON_UNIQUE), -1),
			COALESCE(MAX(NON_UNIQUE), -1),
			COALESCE(GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ','), '')
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'orders'
		  AND INDEX_NAME = ?`,
		indexName,
	).Scan(&count, &minimumNonUnique, &maximumNonUnique, &actualColumns); err != nil {
		t.Fatalf("inspect order index %s: %v", indexName, err)
	}
	expectedCount := len(strings.Split(columns, ","))
	if count != expectedCount ||
		minimumNonUnique != nonUnique ||
		maximumNonUnique != nonUnique ||
		actualColumns != columns {
		t.Fatalf(
			"index %s = count:%d non_unique:%d/%d columns:%s",
			indexName,
			count,
			minimumNonUnique,
			maximumNonUnique,
			actualColumns,
		)
	}
}

func assertIssue16IndexAbsent(t *testing.T, db *sql.DB, indexName string) {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'orders'
		  AND INDEX_NAME = ?`,
		indexName,
	).Scan(&count); err != nil {
		t.Fatalf("inspect absent order index %s: %v", indexName, err)
	}
	if count != 0 {
		t.Fatalf("order index %s still exists", indexName)
	}
}
