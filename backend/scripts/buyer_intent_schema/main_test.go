package main

import (
	"path/filepath"
	"testing"

	"second-hand-market-backend/backend/internal/databasecmd"
)

func TestCheckDoesNotMigrateAndSQLiteApplyPreservesRows(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "intents.db")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", dsn)
	db, err := databasecmd.OpenDatabase(databasecmd.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer databasecmd.CloseDatabase(db)
	for _, statement := range []string{
		"CREATE TABLE buyer_intents (id INTEGER PRIMARY KEY, buyer_id INTEGER NOT NULL, product_id INTEGER NOT NULL, status TEXT NOT NULL, is_open INTEGER NOT NULL, intent_no TEXT NOT NULL)",
		"CREATE UNIQUE INDEX uk_buyer_product_open ON buyer_intents (buyer_id, product_id, is_open)",
		"INSERT INTO buyer_intents VALUES (1, 10, 20, 'CLOSED', 0, 'history-1')",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := run([]string{"--check"}); err == nil {
		t.Fatal("legacy schema passed check")
	}
	if !db.Migrator().HasIndex("buyer_intents", "uk_buyer_product_open") {
		t.Fatal("check mutated schema")
	}
	for _, arg := range []string{"--apply-sqlite", "--apply-sqlite", "--check"} {
		if err := run([]string{arg}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("INSERT INTO buyer_intents VALUES (2, 10, 20, 'CLOSED', 0, 'history-2')").Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Table("buyer_intents").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("history count=%d, err=%v", count, err)
	}
}

func TestSelectionAndMySQLApplyFailBeforeConnecting(t *testing.T) {
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DSN", "secret-sentinel-invalid-dsn")
	for _, args := range [][]string{nil, {"--apply"}, {"--check", "--apply-sqlite"}, {"--apply-sqlite"}} {
		if err := run(args); err == nil {
			t.Fatalf("accepted invalid selection %v", args)
		}
	}
}
