package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/model"
)

const categorySeedSentinel = "category-seed-secret-sentinel"

func TestParseCategorySeedSelectionRequiresOneExplicitSeed(t *testing.T) {
	for _, args := range [][]string{
		{"--seed", defaultCategorySeedID},
		{"--seed=" + defaultCategorySeedID},
	} {
		seedID, err := parseCategorySeedSelection(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		if seedID != defaultCategorySeedID {
			t.Fatalf("seed ID = %q", seedID)
		}
	}

	invalid := []struct {
		name string
		args []string
	}{
		{name: "missing"},
		{name: "empty", args: []string{"--seed="}},
		{name: "unknown", args: []string{"--seed", categorySeedSentinel}},
		{name: "duplicate", args: []string{"--seed", defaultCategorySeedID, "--seed", defaultCategorySeedID}},
		{name: "extra positional", args: []string{"--seed", defaultCategorySeedID, categorySeedSentinel}},
		{name: "unknown flag", args: []string{"--categories=" + categorySeedSentinel}},
		{name: "surrounding whitespace", args: []string{"--seed", " " + defaultCategorySeedID + " "}},
	}
	for _, testCase := range invalid {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseCategorySeedSelection(testCase.args)
			if err == nil {
				t.Fatal("expected seed selection to fail")
			}
			if strings.Contains(err.Error(), categorySeedSentinel) {
				t.Fatalf("seed selection error leaked input: %q", err)
			}
		})
	}
}

func TestRunFailsBeforeDatabaseAccessWithoutValidSeedSelection(t *testing.T) {
	dependencies := categorySeedDependencies{
		loadConfig: func() (databasecmd.Config, error) {
			t.Fatal("database config must not be loaded")
			return databasecmd.Config{}, nil
		},
		openDatabase: func(databasecmd.Config) (*gorm.DB, error) {
			t.Fatal("database must not be opened")
			return nil, nil
		},
		closeDatabase: func(*gorm.DB) {
			t.Fatal("database must not be closed")
		},
		seed: func(*gorm.DB) error {
			t.Fatal("categories must not be seeded")
			return nil
		},
	}

	if _, err := run(nil, dependencies); err == nil {
		t.Fatal("missing seed selection succeeded")
	}
	if _, err := run([]string{"--seed", categorySeedSentinel}, dependencies); err == nil {
		t.Fatal("unknown seed selection succeeded")
	}
}

func TestRunRequiresExplicitCategorySchemaAndDoesNotCreateOtherSchema(t *testing.T) {
	dsn := fmt.Sprintf("file:seed_command_%d?mode=memory&cache=shared", time.Now().UnixNano())
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", dsn)

	if _, err := run(
		[]string{"--seed", defaultCategorySeedID},
		defaultCategorySeedDependencies(),
	); err == nil || !strings.Contains(err.Error(), "CATEGORY_SEED") {
		t.Fatalf("seed without schema error = %v", err)
	}

	db, err := databasecmd.OpenDatabase(databasecmd.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open probe database: %v", err)
	}
	defer databasecmd.CloseDatabase(db)
	createCategorySeedTestSchema(t, db)

	for runIndex := 0; runIndex < 2; runIndex++ {
		if _, err := run(
			[]string{"--seed=" + defaultCategorySeedID},
			defaultCategorySeedDependencies(),
		); err != nil {
			t.Fatalf("run category seed %d: %v", runIndex+1, err)
		}
	}

	var count int64
	if err := db.Model(&model.Category{}).Count(&count).Error; err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if count != 20 {
		t.Fatalf("category count = %d, want 20", count)
	}
	if db.Migrator().HasTable(&model.AdminUser{}) {
		t.Fatal("category seed created the admin schema")
	}
}

func TestSeedDefaultCategoriesIsIdempotentAndPreservesIdentityFields(t *testing.T) {
	db := newCategorySeedTestDB(t)
	if err := seedDefaultCategories(db); err != nil {
		t.Fatalf("initial category seed: %v", err)
	}

	var target model.Category
	if err := db.Where("parent_id IS NULL AND name = ?", "家具类").First(&target).Error; err != nil {
		t.Fatalf("load target category: %v", err)
	}
	marker := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	if err := db.Model(&model.Category{}).
		Where("id = ?", target.ID).
		UpdateColumns(map[string]interface{}{
			"status":     "DISABLED",
			"sort":       99,
			"updated_at": marker,
		}).Error; err != nil {
		t.Fatalf("prepare mutable drift: %v", err)
	}

	before := loadCategoryIdentitySnapshot(t, db)
	if err := seedDefaultCategories(db); err != nil {
		t.Fatalf("repair mutable fields: %v", err)
	}
	after := loadCategoryIdentitySnapshot(t, db)
	assertCategoryIdentitySnapshotsEqual(t, before, after)

	var repaired model.Category
	if err := db.First(&repaired, target.ID).Error; err != nil {
		t.Fatalf("load repaired category: %v", err)
	}
	if repaired.Status != model.CategoryEnabled || repaired.Sort != 1 {
		t.Fatalf("allowed mutable fields were not repaired: %+v", repaired)
	}
	if !repaired.UpdatedAt.Equal(marker) {
		t.Fatalf("seed changed updated_at: got %s want %s", repaired.UpdatedAt, marker)
	}

	if err := seedDefaultCategories(db); err != nil {
		t.Fatalf("repeat category seed: %v", err)
	}
	repeated := loadCategoryIdentitySnapshot(t, db)
	assertCategoryIdentitySnapshotsEqual(t, after, repeated)
}

func TestSeedDefaultCategoriesUsesParentAndNameBusinessKey(t *testing.T) {
	db := newCategorySeedTestDB(t)
	customRoot := model.Category{
		Level:  1,
		Name:   "自定义分类",
		Status: model.CategoryEnabled,
		Sort:   100,
	}
	if err := db.Create(&customRoot).Error; err != nil {
		t.Fatalf("create custom root: %v", err)
	}
	original := model.Category{
		ParentID: &customRoot.ID,
		Level:    2,
		Name:     "家具",
		Status:   model.CategoryEnabled,
		Sort:     1,
	}
	if err := db.Create(&original).Error; err != nil {
		t.Fatalf("create same-name category under custom root: %v", err)
	}
	originalCreatedAt := original.CreatedAt

	if err := seedDefaultCategories(db); err != nil {
		t.Fatalf("seed default categories: %v", err)
	}

	var unchanged model.Category
	if err := db.First(&unchanged, original.ID).Error; err != nil {
		t.Fatalf("load original same-name category: %v", err)
	}
	if unchanged.ParentID == nil ||
		*unchanged.ParentID != customRoot.ID ||
		unchanged.Level != original.Level ||
		unchanged.Name != original.Name ||
		!unchanged.CreatedAt.Equal(originalCreatedAt) {
		t.Fatalf("same-name category was reparented or rewritten: before=%+v after=%+v", original, unchanged)
	}

	var defaultRoot model.Category
	if err := db.Where("parent_id IS NULL AND name = ?", "家具类").First(&defaultRoot).Error; err != nil {
		t.Fatalf("load default root: %v", err)
	}
	var defaultChild model.Category
	if err := db.Where("parent_id = ? AND name = ?", defaultRoot.ID, "家具").First(&defaultChild).Error; err != nil {
		t.Fatalf("load default child: %v", err)
	}
	if defaultChild.ID == original.ID {
		t.Fatal("seed reused the category from another parent")
	}
}

func TestSeedDefaultCategoriesFailsClosedOnIdentityConflict(t *testing.T) {
	db := newCategorySeedTestDB(t)
	conflict := model.Category{
		Level:  2,
		Name:   "家具类",
		Status: "DISABLED",
		Sort:   99,
	}
	if err := db.Create(&conflict).Error; err != nil {
		t.Fatalf("create identity conflict: %v", err)
	}
	before := loadCategoryIdentitySnapshot(t, db)

	err := seedDefaultCategories(db)
	if err == nil || !strings.Contains(err.Error(), "identity conflict") {
		t.Fatalf("identity conflict error = %v", err)
	}
	after := loadCategoryIdentitySnapshot(t, db)
	assertCategoryIdentitySnapshotsEqual(t, before, after)

	var count int64
	if err := db.Unscoped().Model(&model.Category{}).Count(&count).Error; err != nil {
		t.Fatalf("count categories after conflict: %v", err)
	}
	if count != 1 {
		t.Fatalf("identity conflict left partial seed data: count=%d", count)
	}
}

func TestSeedDefaultCategoriesSerializesConcurrentRuns(t *testing.T) {
	db := newCategorySeedTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("load SQL database: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)

	start := make(chan struct{})
	errorsByRun := make([]error, 2)
	var waitGroup sync.WaitGroup
	for runIndex := range errorsByRun {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			<-start
			errorsByRun[index] = seedDefaultCategories(db)
		}(runIndex)
	}
	close(start)
	waitGroup.Wait()

	for runIndex, seedErr := range errorsByRun {
		if seedErr != nil {
			t.Fatalf("concurrent seed %d failed: %v", runIndex+1, seedErr)
		}
	}
	var count int64
	if err := db.Model(&model.Category{}).Count(&count).Error; err != nil {
		t.Fatalf("count categories after concurrent seeds: %v", err)
	}
	if count != 20 {
		t.Fatalf("category count after concurrent seeds = %d, want 20", count)
	}
}

func TestCategoryMutableUpdatesPinMySQLUpdatedAt(t *testing.T) {
	database, err := gorm.Open(
		mysql.New(mysql.Config{
			DSN:                       "user:password@tcp(127.0.0.1:3306)/database",
			SkipInitializeWithVersion: true,
		}),
		&gorm.Config{
			DryRun:                 true,
			DisableAutomaticPing:   true,
			SkipDefaultTransaction: true,
		},
	)
	if err != nil {
		t.Fatalf("open dry-run MySQL database: %v", err)
	}
	category := model.Category{
		ID:        42,
		Level:     1,
		Name:      "家具类",
		Status:    "DISABLED",
		Sort:      99,
		UpdatedAt: time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC),
	}
	updates := categoryMutableUpdates(category, 1)
	result := categoryBusinessKeyQuery(database, nil, category.Name).
		Where("id = ?", category.ID).
		UpdateColumns(updates)
	if result.Error != nil {
		t.Fatalf("build MySQL category update: %v", result.Error)
	}
	statement := result.Statement.SQL.String()
	if !strings.Contains(statement, "`updated_at`=updated_at") {
		t.Fatalf("MySQL update does not pin updated_at: %s", statement)
	}
}

type categoryIdentity struct {
	ID        uint64
	ParentID  *uint64
	Level     int8
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

func newCategorySeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:seed_identity_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := databasecmd.OpenDatabase(databasecmd.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open category seed database: %v", err)
	}
	t.Cleanup(func() {
		databasecmd.CloseDatabase(db)
	})
	createCategorySeedTestSchema(t, db)
	return db
}

func createCategorySeedTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			parent_id INTEGER NULL,
			level INTEGER NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			sort INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at DATETIME NULL
		)`,
		`CREATE UNIQUE INDEX uk_parent_name ON categories(parent_id, name)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create category test schema: %v", err)
		}
	}
}

func loadCategoryIdentitySnapshot(t *testing.T, db *gorm.DB) map[uint64]categoryIdentity {
	t.Helper()
	var categories []model.Category
	if err := db.Unscoped().Order("id").Find(&categories).Error; err != nil {
		t.Fatalf("load category snapshot: %v", err)
	}
	snapshot := make(map[uint64]categoryIdentity, len(categories))
	for _, category := range categories {
		var parentID *uint64
		if category.ParentID != nil {
			value := *category.ParentID
			parentID = &value
		}
		snapshot[category.ID] = categoryIdentity{
			ID:        category.ID,
			ParentID:  parentID,
			Level:     category.Level,
			Name:      category.Name,
			CreatedAt: category.CreatedAt,
			UpdatedAt: category.UpdatedAt,
			DeletedAt: category.DeletedAt,
		}
	}
	return snapshot
}

func assertCategoryIdentitySnapshotsEqual(
	t *testing.T,
	expected map[uint64]categoryIdentity,
	actual map[uint64]categoryIdentity,
) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("category identity count changed: got=%d want=%d", len(actual), len(expected))
	}
	for id, want := range expected {
		got, exists := actual[id]
		if !exists {
			t.Fatalf("category ID %d disappeared", id)
		}
		if got.ID != want.ID ||
			!sameCategoryParentID(got.ParentID, want.ParentID) ||
			got.Level != want.Level ||
			got.Name != want.Name ||
			!got.CreatedAt.Equal(want.CreatedAt) ||
			!got.UpdatedAt.Equal(want.UpdatedAt) ||
			got.DeletedAt.Valid != want.DeletedAt.Valid ||
			(got.DeletedAt.Valid && !got.DeletedAt.Time.Equal(want.DeletedAt.Time)) {
			t.Fatalf("category identity changed:\nwant=%+v\ngot=%+v", want, got)
		}
	}
}
