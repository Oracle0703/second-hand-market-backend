package app

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

func TestMigratePreservesExistingFileQuotaGuardDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE file_quota_guards (
			id integer PRIMARY KEY,
			guard_name varchar(32) NOT NULL UNIQUE,
			created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`).Error; err != nil {
		t.Fatalf("create migration-owned quota guard: %v", err)
	}
	if err := db.Exec("INSERT INTO file_quota_guards (id, guard_name) VALUES (1, 'file_records')").Error; err != nil {
		t.Fatalf("seed migration-owned quota guard: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	columns, err := db.Migrator().ColumnTypes(&model.FileQuotaGuard{})
	if err != nil {
		t.Fatalf("quota guard columns: %v", err)
	}
	for _, column := range columns {
		if column.Name() != "created_at" {
			continue
		}
		if defaultValue, ok := column.DefaultValue(); !ok || !strings.EqualFold(defaultValue, "CURRENT_TIMESTAMP") {
			t.Fatalf("created_at default = %q, %v; migration-owned default was not preserved", defaultValue, ok)
		}
		return
	}
	t.Fatal("created_at column is missing")
}

func TestMigrateRejectsDriftedFileQuotaGuard(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.FileQuotaGuard{}); err != nil {
		t.Fatalf("create quota guard table: %v", err)
	}
	if err := db.Create(&model.FileQuotaGuard{ID: 1, GuardName: "wrong_guard"}).Error; err != nil {
		t.Fatalf("seed drifted guard: %v", err)
	}

	err = migrate(db)
	if err == nil || !strings.Contains(err.Error(), "file quota guard") {
		t.Fatalf("migrate error = %v, want file quota guard drift rejection", err)
	}
}
