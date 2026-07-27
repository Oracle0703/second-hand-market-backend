package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

const (
	testBuyerIntentLegacyIndexSQL = `CREATE UNIQUE INDEX uk_buyer_product_open
		ON buyer_intents (buyer_id, product_id, is_open)`
	testBuyerIntentOpenIndexSQL = `CREATE UNIQUE INDEX uk_buyer_intent_open
		ON buyer_intents (buyer_id, product_id)
		WHERE is_open = 1`
)

type buyerIntentFixtureRow struct {
	ID        uint64
	IntentNo  string
	BuyerID   uint64
	ProductID uint64
	Status    string
	IsOpen    bool
}

type buyerIntentSQLiteMasterRow struct {
	Type  string
	Name  string
	Table string
	SQL   string
}

func openBuyerIntentSchemaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := openDB(Config{
		DBDriver: "sqlite",
		DBDSN: "file:" + strings.ReplaceAll(t.Name(), "/", "_") +
			"?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func createBuyerIntentSQLiteFixture(t *testing.T, db *gorm.DB, indexSQL []string, rows []buyerIntentFixtureRow) {
	t.Helper()
	if err := db.Exec(`
		CREATE TABLE buyer_intents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			intent_no TEXT NOT NULL,
			buyer_id INTEGER NOT NULL,
			product_id INTEGER NOT NULL,
			merchant_id INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL,
			is_open INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`).Error; err != nil {
		t.Fatalf("create buyer intent fixture: %v", err)
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX idx_buyer_intents_intent_no
		ON buyer_intents (intent_no)`).Error; err != nil {
		t.Fatalf("create independent intent number index: %v", err)
	}
	for _, statement := range indexSQL {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create fixture index: %v", err)
		}
	}
	for _, row := range rows {
		if err := db.Exec(`
			INSERT INTO buyer_intents
				(id, intent_no, buyer_id, product_id, status, is_open)
			VALUES (?, ?, ?, ?, ?, ?)`,
			row.ID, row.IntentNo, row.BuyerID, row.ProductID, row.Status, row.IsOpen,
		).Error; err != nil {
			t.Fatalf("insert buyer intent fixture row: %v", err)
		}
	}
}

func validBuyerIntentFixtureRows() []buyerIntentFixtureRow {
	return []buyerIntentFixtureRow{
		{ID: 1, IntentNo: "BI-1", BuyerID: 10, ProductID: 20, Status: model.IntentNew, IsOpen: true},
		{ID: 2, IntentNo: "BI-2", BuyerID: 10, ProductID: 20, Status: model.IntentClosed, IsOpen: false},
		{ID: 3, IntentNo: "BI-3", BuyerID: 11, ProductID: 20, Status: model.IntentContacted, IsOpen: true},
	}
}

func snapshotBuyerIntentRows(t *testing.T, db *gorm.DB) []byte {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQL pool for buyer intent snapshot: %v", err)
	}
	result, err := sqlDB.Query("SELECT * FROM buyer_intents ORDER BY id")
	if err != nil {
		t.Fatalf("snapshot buyer intent rows: %v", err)
	}
	defer func() { _ = result.Close() }()
	columns, err := result.Columns()
	if err != nil {
		t.Fatalf("load buyer intent snapshot columns: %v", err)
	}
	rows := make([][]any, 0)
	for result.Next() {
		row := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for i := range row {
			destinations[i] = &row[i]
		}
		if err := result.Scan(destinations...); err != nil {
			t.Fatalf("scan buyer intent snapshot row: %v", err)
		}
		rows = append(rows, row)
	}
	if err := result.Err(); err != nil {
		t.Fatalf("iterate buyer intent snapshot rows: %v", err)
	}
	data, err := json.Marshal(struct {
		Columns []string
		Rows    [][]any
	}{Columns: columns, Rows: rows})
	if err != nil {
		t.Fatalf("marshal buyer intent rows: %v", err)
	}
	return data
}

func snapshotBuyerIntentDatabase(t *testing.T, db *gorm.DB) []byte {
	t.Helper()
	var schema []buyerIntentSQLiteMasterRow
	if err := db.Raw(`
		SELECT type, name, tbl_name AS "table", COALESCE(sql, '') AS sql
		FROM sqlite_master
		WHERE type IN ('table', 'index')
		ORDER BY type, name`).Scan(&schema).Error; err != nil {
		t.Fatalf("snapshot sqlite_master: %v", err)
	}
	snapshot := struct {
		Rows   json.RawMessage
		Schema []buyerIntentSQLiteMasterRow
	}{Rows: snapshotBuyerIntentRows(t, db), Schema: schema}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal database snapshot: %v", err)
	}
	return data
}

func buyerIntentSQLiteIndexColumns(t *testing.T, db *gorm.DB, name string) []string {
	t.Helper()
	var columns []struct {
		Seqno int
		Cid   int
		Name  string
	}
	if err := db.Raw(fmt.Sprintf("PRAGMA index_info(%q)", name)).Scan(&columns).Error; err != nil {
		t.Fatalf("inspect index %s: %v", name, err)
	}
	result := make([]string, 0, len(columns))
	for _, column := range columns {
		result = append(result, column.Name)
	}
	return result
}

func assertFinalBuyerIntentSQLiteSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	var markerCount int64
	if err := db.Raw(`
		SELECT COUNT(*)
		FROM pragma_table_xinfo('buyer_intents')
		WHERE name = 'open_marker'`).Scan(&markerCount).Error; err != nil {
		t.Fatalf("inspect open marker: %v", err)
	}
	if markerCount != 0 {
		t.Fatalf("open_marker column count = %d, want 0", markerCount)
	}

	var indexes []struct {
		Name    string
		Unique  int `gorm:"column:unique"`
		Partial int
	}
	if err := db.Raw(`PRAGMA index_list('buyer_intents')`).Scan(&indexes).Error; err != nil {
		t.Fatalf("list buyer intent indexes: %v", err)
	}
	relevant := 0
	for _, index := range indexes {
		columns := buyerIntentSQLiteIndexColumns(t, db, index.Name)
		containsBuyer := false
		containsProduct := false
		for _, column := range columns {
			containsBuyer = containsBuyer || column == "buyer_id"
			containsProduct = containsProduct || column == "product_id"
		}
		if index.Unique == 1 && containsBuyer && containsProduct {
			relevant++
			if index.Name != "uk_buyer_intent_open" || index.Partial != 1 ||
				!reflect.DeepEqual(columns, []string{"buyer_id", "product_id"}) {
				t.Fatalf("relevant index = %s unique=%d partial=%d columns=%v", index.Name, index.Unique, index.Partial, columns)
			}
		}
		if index.Name == "uk_buyer_product_open" {
			t.Fatal("legacy buyer intent index remains")
		}
	}
	if relevant != 1 {
		t.Fatalf("relevant unique index count = %d, want 1", relevant)
	}
	var sql string
	if err := db.Raw(`
		SELECT sql FROM sqlite_master
		WHERE type = 'index' AND name = 'uk_buyer_intent_open'`).Scan(&sql).Error; err != nil {
		t.Fatalf("load open index SQL: %v", err)
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(sql), ""))
	if normalized != "createuniqueindexuk_buyer_intent_openonbuyer_intents(buyer_id,product_id)whereis_open=1" {
		t.Fatalf("open index SQL = %q", sql)
	}
}

func runBuyerIntentSQLiteConvergence(t *testing.T, indexSQL []string, rows []buyerIntentFixtureRow) {
	t.Helper()
	db := openBuyerIntentSchemaTestDB(t)
	createBuyerIntentSQLiteFixture(t, db, indexSQL, rows)
	before := snapshotBuyerIntentRows(t, db)
	if err := migrateBuyerIntentOpenUniqueness(db); err != nil {
		t.Fatalf("migrate buyer intent uniqueness: %v", err)
	}
	after := snapshotBuyerIntentRows(t, db)
	if !bytes.Equal(before, after) {
		t.Fatalf("buyer intent rows changed:\nbefore: %s\nafter:  %s", before, after)
	}
	assertFinalBuyerIntentSQLiteSchema(t, db)
}

func TestMigrateBuyerIntentOpenUniquenessSQLiteFreshEmpty(t *testing.T) {
	runBuyerIntentSQLiteConvergence(t, nil, nil)
}

func TestMigrateBuyerIntentOpenUniquenessSQLiteLegacy(t *testing.T) {
	runBuyerIntentSQLiteConvergence(t, []string{testBuyerIntentLegacyIndexSQL}, validBuyerIntentFixtureRows())
}

func TestMigrateBuyerIntentOpenUniquenessSQLiteBothIndexes(t *testing.T) {
	runBuyerIntentSQLiteConvergence(t, []string{testBuyerIntentLegacyIndexSQL, testBuyerIntentOpenIndexSQL}, validBuyerIntentFixtureRows())
}

func TestMigrateBuyerIntentOpenUniquenessSQLiteFinalIsIdempotent(t *testing.T) {
	db := openBuyerIntentSchemaTestDB(t)
	createBuyerIntentSQLiteFixture(t, db, []string{testBuyerIntentOpenIndexSQL}, validBuyerIntentFixtureRows())
	before := snapshotBuyerIntentDatabase(t, db)
	if err := migrateBuyerIntentOpenUniqueness(db); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if err := migrateBuyerIntentOpenUniqueness(db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	after := snapshotBuyerIntentDatabase(t, db)
	if !bytes.Equal(before, after) {
		t.Fatalf("idempotent migration changed database:\nbefore: %s\nafter:  %s", before, after)
	}
	assertFinalBuyerIntentSQLiteSchema(t, db)
}

func TestMigrateBuyerIntentOpenUniquenessSQLiteRejectsDrift(t *testing.T) {
	tests := []struct {
		name     string
		indexSQL []string
		rows     []buyerIntentFixtureRow
		alterSQL string
	}{
		{name: "non-empty table with neither relevant index", rows: validBuyerIntentFixtureRows()},
		{name: "physical open_marker column", alterSQL: "ALTER TABLE buyer_intents ADD COLUMN open_marker INTEGER"},
		{name: "legacy index with wrong order", indexSQL: []string{`CREATE UNIQUE INDEX uk_buyer_product_open ON buyer_intents (product_id, buyer_id, is_open)`}},
		{name: "legacy index with non-unique definition", indexSQL: []string{`CREATE INDEX uk_buyer_product_open ON buyer_intents (buyer_id, product_id, is_open)`}},
		{name: "new index with wrong order", indexSQL: []string{`CREATE UNIQUE INDEX uk_buyer_intent_open ON buyer_intents (product_id, buyer_id) WHERE is_open = 1`}},
		{name: "new index with non-unique definition", indexSQL: []string{`CREATE INDEX uk_buyer_intent_open ON buyer_intents (buyer_id, product_id) WHERE is_open = 1`}},
		{name: "new index with wrong predicate", indexSQL: []string{`CREATE UNIQUE INDEX uk_buyer_intent_open ON buyer_intents (buyer_id, product_id) WHERE is_open = 0`}},
		{name: "same open-only shape under another index name", indexSQL: []string{`CREATE UNIQUE INDEX uk_buyer_intent_open_copy ON buyer_intents (buyer_id, product_id) WHERE is_open = 1`}},
		{name: "unknown NEW false", indexSQL: []string{testBuyerIntentLegacyIndexSQL}, rows: []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-X", BuyerID: 1, ProductID: 1, Status: model.IntentNew, IsOpen: false}}},
		{name: "unknown CONTACTED false", indexSQL: []string{testBuyerIntentLegacyIndexSQL}, rows: []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-X", BuyerID: 1, ProductID: 1, Status: model.IntentContacted, IsOpen: false}}},
		{name: "unknown CLOSED true", indexSQL: []string{testBuyerIntentLegacyIndexSQL}, rows: []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-X", BuyerID: 1, ProductID: 1, Status: model.IntentClosed, IsOpen: true}}},
		{name: "unknown BOGUS row", indexSQL: []string{testBuyerIntentLegacyIndexSQL}, rows: []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-X", BuyerID: 1, ProductID: 1, Status: "BOGUS", IsOpen: true}}},
		{name: "two open rows after removing all relevant constraints", rows: []buyerIntentFixtureRow{
			{ID: 1, IntentNo: "BI-X1", BuyerID: 1, ProductID: 1, Status: model.IntentNew, IsOpen: true},
			{ID: 2, IntentNo: "BI-X2", BuyerID: 1, ProductID: 1, Status: model.IntentContacted, IsOpen: true},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openBuyerIntentSchemaTestDB(t)
			createBuyerIntentSQLiteFixture(t, db, tc.indexSQL, tc.rows)
			if tc.alterSQL != "" {
				if err := db.Exec(tc.alterSQL).Error; err != nil {
					t.Fatalf("alter drift fixture: %v", err)
				}
			}
			before := snapshotBuyerIntentDatabase(t, db)
			if err := migrateBuyerIntentOpenUniqueness(db); err == nil {
				t.Fatal("migration accepted drifted buyer intent schema")
			}
			after := snapshotBuyerIntentDatabase(t, db)
			if !bytes.Equal(before, after) {
				t.Fatalf("rejected migration changed database:\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

func TestVerifyBuyerIntentOpenUniquenessSQLiteRequiresFinalState(t *testing.T) {
	t.Run("final", func(t *testing.T) {
		db := openBuyerIntentSchemaTestDB(t)
		createBuyerIntentSQLiteFixture(t, db, []string{testBuyerIntentOpenIndexSQL}, validBuyerIntentFixtureRows())
		if err := verifyBuyerIntentOpenUniqueness(db); err != nil {
			t.Fatalf("verify final schema: %v", err)
		}
	})

	tests := []struct {
		name     string
		indexSQL []string
		alterSQL string
	}{
		{name: "no F-11 index"},
		{name: "legacy", indexSQL: []string{testBuyerIntentLegacyIndexSQL}},
		{name: "both indexes", indexSQL: []string{testBuyerIntentLegacyIndexSQL, testBuyerIntentOpenIndexSQL}},
		{name: "lookalike", indexSQL: []string{`CREATE UNIQUE INDEX other_open ON buyer_intents (buyer_id, product_id) WHERE is_open = 1`}},
		{name: "physical marker", indexSQL: []string{testBuyerIntentOpenIndexSQL}, alterSQL: "ALTER TABLE buyer_intents ADD COLUMN open_marker INTEGER"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openBuyerIntentSchemaTestDB(t)
			createBuyerIntentSQLiteFixture(t, db, tc.indexSQL, validBuyerIntentFixtureRows())
			if tc.alterSQL != "" {
				if err := db.Exec(tc.alterSQL).Error; err != nil {
					t.Fatalf("alter verifier fixture: %v", err)
				}
			}
			if err := verifyBuyerIntentOpenUniqueness(db); err == nil {
				t.Fatal("verifier accepted non-final buyer intent schema")
			}
		})
	}
}

func TestBuyerIntentSQLiteConstraintAllowsHistoryAndRejectsSecondOpen(t *testing.T) {
	db := openBuyerIntentSchemaTestDB(t)
	createBuyerIntentSQLiteFixture(t, db, []string{testBuyerIntentOpenIndexSQL}, nil)
	rows := []buyerIntentFixtureRow{
		{ID: 1, IntentNo: "BI-H1", BuyerID: 7, ProductID: 9, Status: model.IntentClosed, IsOpen: false},
		{ID: 2, IntentNo: "BI-H2", BuyerID: 7, ProductID: 9, Status: model.IntentClosed, IsOpen: false},
		{ID: 3, IntentNo: "BI-O1", BuyerID: 7, ProductID: 9, Status: model.IntentNew, IsOpen: true},
	}
	for _, row := range rows {
		if err := db.Exec(`
			INSERT INTO buyer_intents (id, intent_no, buyer_id, product_id, status, is_open)
			VALUES (?, ?, ?, ?, ?, ?)`,
			row.ID, row.IntentNo, row.BuyerID, row.ProductID, row.Status, row.IsOpen,
		).Error; err != nil {
			t.Fatalf("insert allowed history/open row: %v", err)
		}
	}
	err := db.Exec(`
		INSERT INTO buyer_intents (id, intent_no, buyer_id, product_id, status, is_open)
		VALUES (4, 'BI-O2', 7, 9, 'CONTACTED', 1)`).Error
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("second open error = %v, want gorm.ErrDuplicatedKey", err)
	}
}

func newBuyerIntentStartupConfig(t *testing.T, dsn string, autoMigrate bool) Config {
	t.Helper()
	cfg := securityTestConfig(t)
	cfg.DBDSN = dsn
	cfg.AutoMigrate = autoMigrate
	return cfg
}

func closeServerDB(t *testing.T, server *Server) {
	t.Helper()
	sqlDB, err := server.DB.DB()
	if err != nil {
		t.Fatalf("get server SQL pool: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close server SQL pool: %v", err)
	}
}

func createCompleteBuyerIntentStartupDB(t *testing.T) string {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "buyer-intent.db") + "?_pragma=busy_timeout(5000)"
	server, err := NewServer(newBuyerIntentStartupConfig(t, dsn, true))
	if err != nil {
		t.Fatalf("create complete database: %v", err)
	}
	closeServerDB(t, server)
	return dsn
}

func withBuyerIntentStartupDB(t *testing.T, dsn string, fn func(*gorm.DB)) {
	t.Helper()
	db, err := openDB(Config{DBDriver: "sqlite", DBDSN: dsn})
	if err != nil {
		t.Fatalf("open startup database: %v", err)
	}
	fn(db)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get startup database pool: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close startup database pool: %v", err)
	}
}

func TestNewServerVerifiesBuyerIntentSchemaWithAutoMigrateDisabled(t *testing.T) {
	t.Run("final starts without mutation", func(t *testing.T) {
		dsn := createCompleteBuyerIntentStartupDB(t)
		var before []byte
		withBuyerIntentStartupDB(t, dsn, func(db *gorm.DB) {
			before = snapshotBuyerIntentDatabase(t, db)
		})
		server, err := NewServer(newBuyerIntentStartupConfig(t, dsn, false))
		if err != nil {
			t.Fatalf("start with final schema: %v", err)
		}
		closeServerDB(t, server)
		withBuyerIntentStartupDB(t, dsn, func(db *gorm.DB) {
			after := snapshotBuyerIntentDatabase(t, db)
			if !bytes.Equal(before, after) {
				t.Fatalf("disabled migration changed final database:\nbefore: %s\nafter:  %s", before, after)
			}
		})
	})

	tests := []struct {
		name     string
		indexSQL string
	}{
		{name: "legacy", indexSQL: testBuyerIntentLegacyIndexSQL},
		{name: "drifted lookalike", indexSQL: `CREATE UNIQUE INDEX other_open ON buyer_intents (buyer_id, product_id) WHERE is_open = 1`},
		{name: "no F-11 index"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dsn := createCompleteBuyerIntentStartupDB(t)
			var before []byte
			withBuyerIntentStartupDB(t, dsn, func(db *gorm.DB) {
				if err := db.Exec("DROP INDEX uk_buyer_intent_open").Error; err != nil {
					t.Fatalf("drop final F-11 index: %v", err)
				}
				if tc.indexSQL != "" {
					if err := db.Exec(tc.indexSQL).Error; err != nil {
						t.Fatalf("create replacement F-11 index: %v", err)
					}
				}
				before = snapshotBuyerIntentDatabase(t, db)
			})
			if server, err := NewServer(newBuyerIntentStartupConfig(t, dsn, false)); err == nil {
				closeServerDB(t, server)
				t.Fatal("startup accepted non-final F-11 schema")
			}
			withBuyerIntentStartupDB(t, dsn, func(db *gorm.DB) {
				after := snapshotBuyerIntentDatabase(t, db)
				if !bytes.Equal(before, after) {
					t.Fatalf("rejected disabled migration changed database:\nbefore: %s\nafter:  %s", before, after)
				}
			})
		})
	}
}

func TestBuyerIntentModelDoesNotOwnLegacyUniqueIndex(t *testing.T) {
	db, err := openDB(Config{
		DBDriver: "sqlite",
		DBDSN:    "file:model_contract?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatal(err)
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&model.BuyerIntent{}); err != nil {
		t.Fatal(err)
	}
	for _, fieldName := range []string{"BuyerID", "ProductID", "IsOpen"} {
		field := stmt.Schema.LookUpField(fieldName)
		if strings.Contains(field.Tag.Get("gorm"), "uk_buyer_product_open") {
			t.Fatalf("%s still owns legacy unique index", fieldName)
		}
	}
	marker := stmt.Schema.LookUpField("OpenMarker")
	if marker == nil || marker.Creatable || marker.Updatable || !marker.IgnoreMigration {
		t.Fatalf("OpenMarker permissions = %#v", marker)
	}
}

func formalMySQLBuyerIntentColumns() []mysqlBuyerIntentColumn {
	return []mysqlBuyerIntentColumn{
		{Name: "buyer_id", DataType: "bigint", ColumnType: "bigint", IsNullable: "NO", IsGenerated: "NEVER"},
		{Name: "product_id", DataType: "bigint", ColumnType: "bigint", IsNullable: "NO", IsGenerated: "NEVER"},
		{Name: "status", DataType: "varchar", ColumnType: "varchar(16)", IsNullable: "NO", IsGenerated: "NEVER"},
		{Name: "is_open", DataType: "tinyint", ColumnType: "tinyint(1)", IsNullable: "NO", IsGenerated: "NEVER"},
	}
}

func gormMySQLBuyerIntentColumns() []mysqlBuyerIntentColumn {
	return []mysqlBuyerIntentColumn{
		{Name: "buyer_id", DataType: "bigint", ColumnType: "bigint unsigned", IsNullable: "YES", IsGenerated: "NEVER"},
		{Name: "product_id", DataType: "bigint", ColumnType: "bigint unsigned", IsNullable: "YES", IsGenerated: "NEVER"},
		{Name: "status", DataType: "varchar", ColumnType: "varchar(16)", IsNullable: "YES", IsGenerated: "NEVER"},
		{Name: "is_open", DataType: "tinyint", ColumnType: "tinyint(1)", IsNullable: "YES", IsGenerated: "NEVER"},
	}
}

func TestVerifyMySQLBuyerIntentColumnsAcceptsCanonicalLayouts(t *testing.T) {
	tests := []struct {
		name    string
		columns []mysqlBuyerIntentColumn
	}{
		{name: "formal migration", columns: formalMySQLBuyerIntentColumns()},
		{name: "GORM development", columns: gormMySQLBuyerIntentColumns()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var state buyerIntentSchemaState
			if err := verifyMySQLBuyerIntentColumns(tt.columns, &state); err != nil {
				t.Fatalf("verify canonical columns: %v", err)
			}
		})
	}
}

func TestVerifyMySQLBuyerIntentColumnsRejectsRequiredColumnDrift(t *testing.T) {
	type mutation func([]mysqlBuyerIntentColumn)
	type testCase struct {
		name   string
		mutate mutation
	}
	tests := make([]testCase, 0, 22)
	for i, name := range []string{"buyer_id", "product_id", "status", "is_open"} {
		index := i
		columnName := name
		tests = append(tests,
			testCase{
				name: "drifted data_type for " + columnName,
				mutate: func(columns []mysqlBuyerIntentColumn) {
					columns[index].DataType = "blob"
				},
			},
			testCase{
				name: "drifted column_type for " + columnName,
				mutate: func(columns []mysqlBuyerIntentColumn) {
					columns[index].ColumnType = "blob"
				},
			},
			testCase{
				name: "drifted nullability for " + columnName,
				mutate: func(columns []mysqlBuyerIntentColumn) {
					columns[index].IsNullable = "YES"
				},
			},
			testCase{
				name: "generated " + columnName,
				mutate: func(columns []mysqlBuyerIntentColumn) {
					columns[index].GenerationExpression = "1"
					columns[index].Extra = "STORED GENERATED"
					columns[index].IsGenerated = "ALWAYS"
				},
			},
		)
	}
	tests = append(tests,
		testCase{
			name: "formal types with GORM nullability",
			mutate: func(columns []mysqlBuyerIntentColumn) {
				for i := range columns {
					columns[i].IsNullable = "YES"
				}
			},
		},
		testCase{
			name: "GORM types with formal nullability",
			mutate: func(columns []mysqlBuyerIntentColumn) {
				gormColumns := gormMySQLBuyerIntentColumns()
				copy(columns, gormColumns)
				for i := range columns {
					columns[i].IsNullable = "NO"
				}
			},
		},
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := formalMySQLBuyerIntentColumns()
			tt.mutate(columns)
			var state buyerIntentSchemaState
			if err := verifyMySQLBuyerIntentColumns(columns, &state); err == nil {
				t.Fatal("accepted drifted required-column metadata")
			}
		})
	}
}

func TestOpenDBTranslatesDuplicateKeys(t *testing.T) {
	db, err := openDB(Config{
		DBDriver: "sqlite",
		DBDSN: "file:" + strings.ReplaceAll(t.Name(), "/", "_") +
			"?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(
		"CREATE TABLE duplicate_probe (id INTEGER PRIMARY KEY, value TEXT UNIQUE)",
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO duplicate_probe(value) VALUES (?)", "same").Error; err != nil {
		t.Fatal(err)
	}
	err = db.Exec("INSERT INTO duplicate_probe(value) VALUES (?)", "same").Error
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate error = %v, want gorm.ErrDuplicatedKey", err)
	}
}
