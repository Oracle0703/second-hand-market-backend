package main

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

func openCategorySeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open category test database: %v", err)
	}
	if err := db.AutoMigrate(&model.Category{}); err != nil {
		t.Fatalf("migrate category test database: %v", err)
	}
	return db
}

func TestEnsureDefaultCategoriesScopesSameNameToParentAndIsIdempotent(t *testing.T) {
	originalSeeds := defaultCategorySeeds
	defaultCategorySeeds = []categorySeed{
		{Name: "Root A", Children: []string{"Shared"}},
		{Name: "Root B", Children: []string{"Shared"}},
	}
	t.Cleanup(func() { defaultCategorySeeds = originalSeeds })

	db := openCategorySeedTestDB(t)
	if err := ensureDefaultCategories(db); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	var firstChildren []model.Category
	if err := db.Where("level = ? AND name = ?", 2, "Shared").Order("parent_id").Find(&firstChildren).Error; err != nil {
		t.Fatalf("load first children: %v", err)
	}
	if len(firstChildren) != 2 {
		t.Fatalf("first shared child count = %d, want 2", len(firstChildren))
	}
	if firstChildren[0].ParentID == nil || firstChildren[1].ParentID == nil || *firstChildren[0].ParentID == *firstChildren[1].ParentID {
		t.Fatalf("shared children have invalid parents: %+v", firstChildren)
	}
	firstIDs := map[uint64]uint64{
		*firstChildren[0].ParentID: firstChildren[0].ID,
		*firstChildren[1].ParentID: firstChildren[1].ID,
	}
	if err := db.Model(&model.Category{}).Where("id = ?", firstChildren[0].ID).Updates(map[string]interface{}{
		"level":  9,
		"status": model.CategoryDisabled,
		"sort":   99,
	}).Error; err != nil {
		t.Fatalf("drift first shared child: %v", err)
	}
	reconciled, err := findOrCreateCategory(db, firstChildren[0].ParentID, 2, "Shared", 1)
	if err != nil {
		t.Fatalf("reconcile first shared child: %v", err)
	}
	if reconciled.Level != 2 || reconciled.Status != model.CategoryEnabled || reconciled.Sort != 1 {
		t.Fatalf("returned category was not reconciled: %+v", reconciled)
	}

	if err := ensureDefaultCategories(db); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	var secondChildren []model.Category
	if err := db.Where("level = ? AND name = ?", 2, "Shared").Order("parent_id").Find(&secondChildren).Error; err != nil {
		t.Fatalf("load second children: %v", err)
	}
	if len(secondChildren) != 2 {
		t.Fatalf("second shared child count = %d, want 2", len(secondChildren))
	}
	for _, child := range secondChildren {
		if child.ParentID == nil || firstIDs[*child.ParentID] != child.ID {
			t.Fatalf("seed changed shared child identity: %+v", child)
		}
		if child.Level != 2 || child.Status != model.CategoryEnabled || child.Sort != 1 {
			t.Fatalf("seed did not reconcile shared child: %+v", child)
		}
	}

	var total int64
	if err := db.Model(&model.Category{}).Count(&total).Error; err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if total != 4 {
		t.Fatalf("category count after repeated seed = %d, want 4", total)
	}
}
