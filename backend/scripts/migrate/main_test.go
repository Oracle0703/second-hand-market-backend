package main

import (
	"fmt"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/model"
)

func TestRunMigratesSchemaOnlyWhenInvoked(t *testing.T) {
	dsn := fmt.Sprintf("file:migrate_command_%d?mode=memory&cache=shared", time.Now().UnixNano())
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", dsn)

	db, err := databasecmd.OpenDatabase(databasecmd.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open probe database: %v", err)
	}
	if db.Migrator().HasTable(&model.AdminUser{}) {
		t.Fatal("schema existed before explicit migration")
	}
	if err := run(); err != nil {
		t.Fatalf("run migration: %v", err)
	}
	if !db.Migrator().HasTable(&model.AdminUser{}) || !db.Migrator().HasTable(&model.Product{}) {
		t.Fatal("explicit migration did not create application schema")
	}
}
