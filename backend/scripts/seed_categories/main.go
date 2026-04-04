package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

func openDB() (*gorm.DB, error) {
	driver := os.Getenv("DB_DRIVER")
	dsn := os.Getenv("DB_DSN")
	if driver == "" {
		driver = "sqlite"
		dsn = "file:app.db?cache=shared&_foreign_keys=on"
	}
	if driver == "mysql" {
		return gorm.Open(mysql.Open(dsn), &gorm.Config{})
	}
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{})
}

func main() {
	db, err := openDB()
	if err != nil {
		panic(err)
	}
	if err := db.AutoMigrate(&model.Category{}); err != nil {
		panic(err)
	}
	if err := ensureDefaultCategories(db); err != nil {
		panic(err)
	}
	fmt.Println("seed_categories done")
}

type categorySeed struct {
	Name     string
	Children []string
}

var defaultCategorySeeds = []categorySeed{
	{Name: "家具类", Children: []string{"家具", "家电", "麻将机", "商铺用品"}},
	{Name: "办公类", Children: []string{"老板桌", "办公桌", "老板椅", "老板办公座椅套装", "会议桌", "办公沙发", "会议桌椅套装", "文件柜书柜"}},
	{Name: "麻将机类", Children: []string{"旧麻将机", "新麻将机", "麻将椅", "茶几", "麻将机维修"}},
}

func ensureDefaultCategories(db *gorm.DB) error {
	for i, seed := range defaultCategorySeeds {
		root, err := findOrCreateCategory(db, nil, 1, seed.Name, i+1)
		if err != nil {
			return err
		}
		seen := map[string]struct{}{}
		sortOrder := 1
		for _, childName := range seed.Children {
			name := strings.TrimSpace(childName)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			if _, err := findOrCreateCategory(db, &root.ID, 2, name, sortOrder); err != nil {
				return err
			}
			sortOrder++
		}
	}
	return nil
}

func findOrCreateCategory(db *gorm.DB, parentID *uint64, level int8, name string, sort int) (model.Category, error) {
	var cat model.Category
	if err := db.Model(&model.Category{}).Where("name = ?", name).First(&cat).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cat = model.Category{
				ParentID: parentID,
				Level:    level,
				Name:     name,
				Status:   model.CategoryEnabled,
				Sort:     sort,
			}
			return cat, db.Create(&cat).Error
		}
		return model.Category{}, err
	}
	updates := map[string]interface{}{}
	if !sameParentID(cat.ParentID, parentID) {
		updates["parent_id"] = parentID
	}
	if cat.Level != level {
		updates["level"] = level
	}
	if cat.Status != model.CategoryEnabled {
		updates["status"] = model.CategoryEnabled
	}
	if cat.Sort != sort {
		updates["sort"] = sort
	}
	if len(updates) > 0 {
		if err := db.Model(&model.Category{}).Where("id = ?", cat.ID).Updates(updates).Error; err != nil {
			return model.Category{}, err
		}
		cat.Status = model.CategoryEnabled
		cat.Sort = sort
	}
	return cat, nil
}

func sameParentID(a, b *uint64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
