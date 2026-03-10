package main

import (
	"fmt"
	"os"

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
	var count int64
	_ = db.Model(&model.Category{}).Count(&count).Error
	if count > 0 {
		fmt.Println("categories already exist")
		return
	}
	root1 := model.Category{Level: 1, Name: "数码", Status: model.CategoryEnabled, Sort: 1}
	root2 := model.Category{Level: 1, Name: "家电", Status: model.CategoryEnabled, Sort: 2}
	_ = db.Create(&root1).Error
	_ = db.Create(&root2).Error
	cats := []model.Category{
		{Level: 2, ParentID: &root1.ID, Name: "手机", Status: model.CategoryEnabled, Sort: 1},
		{Level: 2, ParentID: &root1.ID, Name: "笔记本", Status: model.CategoryEnabled, Sort: 2},
		{Level: 2, ParentID: &root2.ID, Name: "洗衣机", Status: model.CategoryEnabled, Sort: 1},
	}
	if err := db.Create(&cats).Error; err != nil {
		panic(err)
	}
	fmt.Println("seed_categories done")
}
