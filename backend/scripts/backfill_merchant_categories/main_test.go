package main

import (
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/databasecmd"
)

const backfillCategoryDSNSentinel = "backfill-category-dsn-secret-sentinel"

func TestBackfillMerchantCategoriesRejectsArguments(t *testing.T) {
	err := run([]string{"unexpected"})
	if err == nil || !strings.Contains(err.Error(), "BACKFILL_MERCHANT_CATEGORIES_ARGUMENTS") {
		t.Fatalf("argument error = %v", err)
	}
}

func TestBackfillMerchantCategoriesCallsBackfillOnce(t *testing.T) {
	var backfillCalls int
	err := runWithDependencies(nil, backfillDependencies{
		loadConfig: func() (databasecmd.Config, error) {
			return databasecmd.Config{Driver: "sqlite", DSN: "file:test?mode=memory&cache=shared"}, nil
		},
		openDatabase: func(databasecmd.Config) (*gorm.DB, error) {
			return &gorm.DB{}, nil
		},
		closeDatabase: func(*gorm.DB) {},
		backfill: func(*gorm.DB) error {
			backfillCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run backfill: %v", err)
	}
	if backfillCalls != 1 {
		t.Fatalf("backfill calls = %d, want 1", backfillCalls)
	}
}

func TestBackfillMerchantCategoriesDoesNotLeakConfigValues(t *testing.T) {
	err := runWithDependencies(nil, backfillDependencies{
		loadConfig: func() (databasecmd.Config, error) {
			return databasecmd.Config{}, errors.New("DB_DSN is invalid")
		},
		openDatabase: func(databasecmd.Config) (*gorm.DB, error) {
			t.Fatal("open database should not be called")
			return nil, nil
		},
		closeDatabase: func(*gorm.DB) {},
		backfill: func(*gorm.DB) error {
			t.Fatal("backfill should not be called")
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "DB_DSN") {
		t.Fatalf("config error = %v", err)
	}
	if strings.Contains(err.Error(), backfillCategoryDSNSentinel) {
		t.Fatalf("config error leaked DSN: %q", err)
	}
}
