package app

import (
	"strings"
	"testing"
)

func TestValidateIdempotencyTableEnginesAcceptsExactInnoDBSet(t *testing.T) {
	tests := []struct {
		name string
		rows []mysqlTableEngine
	}{
		{
			name: "canonical",
			rows: []mysqlTableEngine{
				{TableName: "buyer_intents", Engine: "InnoDB"},
				{TableName: "idempotency_records", Engine: "InnoDB"},
				{TableName: "operation_logs", Engine: "InnoDB"},
				{TableName: "order_events", Engine: "InnoDB"},
				{TableName: "orders", Engine: "InnoDB"},
				{TableName: "products", Engine: "InnoDB"},
			},
		},
		{
			name: "reordered",
			rows: []mysqlTableEngine{
				{TableName: "products", Engine: "InnoDB"},
				{TableName: "orders", Engine: "InnoDB"},
				{TableName: "order_events", Engine: "InnoDB"},
				{TableName: "operation_logs", Engine: "InnoDB"},
				{TableName: "idempotency_records", Engine: "InnoDB"},
				{TableName: "buyer_intents", Engine: "InnoDB"},
			},
		},
		{
			name: "lowercase engine",
			rows: []mysqlTableEngine{
				{TableName: "buyer_intents", Engine: "innodb"},
				{TableName: "idempotency_records", Engine: "innodb"},
				{TableName: "operation_logs", Engine: "innodb"},
				{TableName: "order_events", Engine: "innodb"},
				{TableName: "orders", Engine: "innodb"},
				{TableName: "products", Engine: "innodb"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateIdempotencyTableEngines(tc.rows); err != nil {
				t.Fatalf("validate idempotency table engines: %v", err)
			}
		})
	}
}

func TestValidateIdempotencyTableEnginesRejectsMissingDuplicateAndNonInnoDB(t *testing.T) {
	tests := []struct {
		name string
		rows []mysqlTableEngine
	}{
		{
			name: "missing table",
			rows: []mysqlTableEngine{
				{TableName: "buyer_intents", Engine: "InnoDB"},
				{TableName: "idempotency_records", Engine: "InnoDB"},
				{TableName: "operation_logs", Engine: "InnoDB"},
				{TableName: "order_events", Engine: "InnoDB"},
				{TableName: "orders", Engine: "InnoDB"},
			},
		},
		{
			name: "duplicate table",
			rows: []mysqlTableEngine{
				{TableName: "buyer_intents", Engine: "InnoDB"},
				{TableName: "buyer_intents", Engine: "InnoDB"},
				{TableName: "idempotency_records", Engine: "InnoDB"},
				{TableName: "operation_logs", Engine: "InnoDB"},
				{TableName: "order_events", Engine: "InnoDB"},
				{TableName: "orders", Engine: "InnoDB"},
				{TableName: "products", Engine: "InnoDB"},
			},
		},
		{
			name: "unknown table",
			rows: []mysqlTableEngine{
				{TableName: "buyer_intents", Engine: "InnoDB"},
				{TableName: "idempotency_records", Engine: "InnoDB"},
				{TableName: "operation_logs", Engine: "InnoDB"},
				{TableName: "order_events", Engine: "InnoDB"},
				{TableName: "orders", Engine: "InnoDB"},
				{TableName: "products", Engine: "InnoDB"},
				{TableName: "unknown_table", Engine: "InnoDB"},
			},
		},
		{
			name: "empty engine",
			rows: []mysqlTableEngine{
				{TableName: "buyer_intents", Engine: ""},
				{TableName: "idempotency_records", Engine: "InnoDB"},
				{TableName: "operation_logs", Engine: "InnoDB"},
				{TableName: "order_events", Engine: "InnoDB"},
				{TableName: "orders", Engine: "InnoDB"},
				{TableName: "products", Engine: "InnoDB"},
			},
		},
		{
			name: "MyISAM",
			rows: []mysqlTableEngine{
				{TableName: "buyer_intents", Engine: "InnoDB"},
				{TableName: "idempotency_records", Engine: "MyISAM"},
				{TableName: "operation_logs", Engine: "InnoDB"},
				{TableName: "order_events", Engine: "InnoDB"},
				{TableName: "orders", Engine: "InnoDB"},
				{TableName: "products", Engine: "InnoDB"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateIdempotencyTableEngines(tc.rows)
			if err == nil || err.Error() != "idempotency transactional tables are missing or drifted" {
				t.Fatalf("validate idempotency table engines error = %v, want stable drift error", err)
			}
		})
	}
}

func TestVerifyIdempotencyTransactionalTablesSkipsSQLite(t *testing.T) {
	db, err := openDB(Config{
		DBDriver: "sqlite",
		DBDSN:    "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQLite database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := verifyIdempotencyTransactionalTables(db); err != nil {
		t.Fatalf("verify SQLite idempotency transactional tables: %v", err)
	}
}
