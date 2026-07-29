package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/model"
)

func TestRunRequiresExplicitCategorySchema(t *testing.T) {
	dsn := fmt.Sprintf("file:seed_command_%d?mode=memory&cache=shared", time.Now().UnixNano())
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", dsn)

	if err := run(); err == nil || !strings.Contains(err.Error(), "CATEGORY_SEED") {
		t.Fatalf("seed without schema error = %v", err)
	}

	db, err := databasecmd.OpenDatabase(databasecmd.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open probe database: %v", err)
	}
	if err := db.AutoMigrate(&model.Category{}); err != nil {
		t.Fatalf("explicit test migration: %v", err)
	}
	if err := run(); err != nil {
		t.Fatalf("run category seed: %v", err)
	}
	if err := run(); err != nil {
		t.Fatalf("run category seed again: %v", err)
	}

	var count int64
	if err := db.Model(&model.Category{}).Count(&count).Error; err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if count != 20 {
		t.Fatalf("category count = %d, want 20", count)
	}
}
