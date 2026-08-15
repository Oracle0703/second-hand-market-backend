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
	if !db.Migrator().HasTable(&model.ProductStockAdjustment{}) {
		t.Fatalf("expected product_stock_adjustments table to be created")
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

func TestEnsureMerchantDefaultCategoriesCreatesPrivateCopies(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.Merchant{}, &model.Category{}); err != nil {
		t.Fatalf("migrate merchant categories: %v", err)
	}
	merchantOne := model.Merchant{MerchantNo: "M1", MerchantName: "One", ContactName: "A", ContactPhone: "1", ReviewStatus: model.ReviewApproved}
	merchantTwo := model.Merchant{MerchantNo: "M2", MerchantName: "Two", ContactName: "B", ContactPhone: "2", ReviewStatus: model.ReviewApproved}
	if err := db.Create(&merchantOne).Error; err != nil {
		t.Fatalf("create merchant one: %v", err)
	}
	if err := db.Create(&merchantTwo).Error; err != nil {
		t.Fatalf("create merchant two: %v", err)
	}

	for run := 0; run < 2; run++ {
		if err := EnsureMerchantDefaultCategories(db, merchantOne.ID); err != nil {
			t.Fatalf("seed merchant one run %d: %v", run+1, err)
		}
	}
	if err := EnsureMerchantDefaultCategories(db, merchantTwo.ID); err != nil {
		t.Fatalf("seed merchant two: %v", err)
	}

	var count int64
	if err := db.Model(&model.Category{}).Where("merchant_id IN ?", []uint64{merchantOne.ID, merchantTwo.ID}).Count(&count).Error; err != nil {
		t.Fatalf("count merchant categories: %v", err)
	}
	if count != 40 {
		t.Fatalf("merchant category count = %d, want 40", count)
	}

	var roots []model.Category
	if err := db.Where("merchant_id = ? AND parent_id IS NULL", merchantOne.ID).Order("sort ASC").Find(&roots).Error; err != nil {
		t.Fatalf("load roots: %v", err)
	}
	if len(roots) != len(defaultCategorySeeds) {
		t.Fatalf("root count = %d, want %d", len(roots), len(defaultCategorySeeds))
	}
}

func TestBackfillMerchantCategoriesRemapsProductsToMerchantCopies(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.Merchant{}, &model.Category{}, &model.Product{}); err != nil {
		t.Fatalf("migrate backfill schema: %v", err)
	}
	if err := SeedDefaultCategories(db); err != nil {
		t.Fatalf("seed legacy categories: %v", err)
	}
	merchant := model.Merchant{MerchantNo: "M1", MerchantName: "One", ContactName: "A", ContactPhone: "1", ReviewStatus: model.ReviewApproved}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	var legacyChild model.Category
	if err := db.Where("merchant_id IS NULL AND level = ? AND name = ?", 2, "家具").First(&legacyChild).Error; err != nil {
		t.Fatalf("load legacy child: %v", err)
	}
	product := model.Product{
		ProductNo: "P1", MerchantID: merchant.ID, Title: "Desk", Description: "Desk",
		CategoryID: legacyChild.ID, PriceCent: 100, ConditionLevel: "GOOD",
		Stock: 1, Status: model.ProductDraft, CreatedBy: 1, UpdatedBy: 1,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := BackfillMerchantCategories(db); err != nil {
		t.Fatalf("backfill merchant categories: %v", err)
	}

	var updated model.Product
	if err := db.First(&updated, product.ID).Error; err != nil {
		t.Fatalf("load product: %v", err)
	}
	if updated.CategoryID == legacyChild.ID {
		t.Fatal("product still points at legacy global category")
	}
	var ownedCategory model.Category
	if err := db.First(&ownedCategory, updated.CategoryID).Error; err != nil {
		t.Fatalf("load owned category: %v", err)
	}
	if ownedCategory.MerchantID == nil || *ownedCategory.MerchantID != merchant.ID || ownedCategory.Name != legacyChild.Name {
		t.Fatalf("product category not remapped to merchant copy: %+v", ownedCategory)
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
