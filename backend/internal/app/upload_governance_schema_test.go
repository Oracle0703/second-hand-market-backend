package app

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

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
