package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"second-hand-market-backend/backend/internal/media"
	"second-hand-market-backend/backend/internal/model"
)

func TestMigrateSchemaCreatesApplicationTables(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := MigrateSchema(db); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	for _, target := range []any{&model.AdminUser{}, &model.Category{}, &model.Product{}, &model.BuyerUser{}} {
		if !db.Migrator().HasTable(target) {
			t.Fatalf("missing migrated table for %T", target)
		}
	}
}

func TestMigrateSchemaUsesExplicitFileTableName(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := MigrateSchema(db); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	if !db.Migrator().HasTable("files") {
		t.Fatal("FileRecord must use the explicit SQL migration table name files")
	}
	if db.Migrator().HasTable("file_records") {
		t.Fatal("FileRecord must not create GORM default table file_records")
	}
}

func TestMigrateSchemaCreatesImageBackfillLedger(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := MigrateSchema(db); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	for _, target := range []any{&model.ImageBackfillRun{}, &model.ImageBackfillItem{}} {
		if !db.Migrator().HasTable(target) {
			t.Fatalf("missing migrated table for %T", target)
		}
	}

	run := model.ImageBackfillRun{ID: "IMGTEST1", ProfileVersion: media.DetailProfileVersion}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create backfill run: %v", err)
	}
	item := model.ImageBackfillItem{
		RunID:           run.ID,
		FileID:          10,
		SourceObjectKey: "product_image/F10.jpg",
		TargetObjectKey: "product_image/detail-v1/F10.jpg",
		ProfileVersion:  media.DetailProfileVersion,
		Status:          "PENDING",
		CleanupStatus:   "NOT_SCHEDULED",
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create backfill item: %v", err)
	}
	duplicate := item
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("duplicate run/file backfill item was accepted")
	}
	anotherRun := model.ImageBackfillRun{ID: "IMGTEST2", ProfileVersion: media.DetailProfileVersion}
	if err := db.Create(&anotherRun).Error; err != nil {
		t.Fatalf("create second backfill run: %v", err)
	}
	anotherItem := item
	anotherItem.ID = 0
	anotherItem.RunID = anotherRun.ID
	if err := db.Create(&anotherItem).Error; err != nil {
		t.Fatalf("same file in a different run should remain allowed: %v", err)
	}
}

func TestBootstrapAdminRequiresExplicitSafeInput(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.AdminUser{}); err != nil {
		t.Fatalf("migrate admin: %v", err)
	}

	tests := []struct {
		name      string
		field     string
		bootstrap AdminBootstrap
	}{
		{name: "username", field: "ADMIN_USERNAME", bootstrap: AdminBootstrap{DisplayName: "Admin", Role: model.AdminRoleAdmin, Password: "explicit-test-password"}},
		{name: "password", field: "ADMIN_PASSWORD", bootstrap: AdminBootstrap{Username: "admin", DisplayName: "Admin", Role: model.AdminRoleAdmin}},
		{name: "role", field: "ADMIN_ROLE", bootstrap: AdminBootstrap{Username: "admin", DisplayName: "Admin", Role: "owner", Password: "explicit-test-password"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := BootstrapAdmin(db, tc.bootstrap)
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("expected %s error, got %v", tc.field, err)
			}
		})
	}

	const passwordSentinel = "bootstrap-password-secret-sentinel-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	err := BootstrapAdmin(db, AdminBootstrap{
		Username:    "admin",
		DisplayName: "Admin",
		Role:        model.AdminRoleAdmin,
		Password:    passwordSentinel,
	})
	if err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD") {
		t.Fatalf("expected excessive password error, got %v", err)
	}
	if strings.Contains(err.Error(), passwordSentinel) {
		t.Fatalf("bootstrap error leaked password: %q", err)
	}
}

func TestBootstrapAdminIsIdempotentWithoutResettingPassword(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.AdminUser{}); err != nil {
		t.Fatalf("migrate admin: %v", err)
	}
	first := AdminBootstrap{
		Username:    "admin",
		DisplayName: "Admin",
		Role:        model.AdminRoleAdmin,
		Password:    "first-explicit-test-password",
	}
	if err := BootstrapAdmin(db, first); err != nil {
		t.Fatalf("bootstrap first admin: %v", err)
	}
	second := first
	second.Password = "second-explicit-test-password"
	if err := BootstrapAdmin(db, second); err != nil {
		t.Fatalf("bootstrap existing admin: %v", err)
	}

	var admin model.AdminUser
	if err := db.Where("username = ?", first.Username).First(&admin).Error; err != nil {
		t.Fatalf("load admin: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(first.Password)); err != nil {
		t.Fatalf("original password was not retained: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(second.Password)); err == nil {
		t.Fatal("idempotent bootstrap reset an existing password")
	}
}

func TestSeedDefaultCategoriesIsIdempotent(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.Category{}); err != nil {
		t.Fatalf("migrate categories: %v", err)
	}
	if err := SeedDefaultCategories(db); err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	if err := SeedDefaultCategories(db); err != nil {
		t.Fatalf("seed categories again: %v", err)
	}

	var count int64
	if err := db.Model(&model.Category{}).Count(&count).Error; err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if count != 20 {
		t.Fatalf("category count = %d, want 20", count)
	}
}

func newDatabaseOperationsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:database_operations_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}
