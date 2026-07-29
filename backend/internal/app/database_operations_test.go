package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

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
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}
