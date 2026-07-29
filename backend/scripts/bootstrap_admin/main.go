package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

func databaseConfigFromEnv() (string, string, error) {
	driver := strings.TrimSpace(os.Getenv("DB_DRIVER"))
	if driver == "" {
		return "", "", fmt.Errorf("DB_DRIVER is required")
	}

	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	if dsn == "" {
		return "", "", fmt.Errorf("DB_DSN is required")
	}

	switch driver {
	case "mysql", "sqlite":
		return driver, dsn, nil
	default:
		return "", "", fmt.Errorf("unsupported DB_DRIVER %q", driver)
	}
}

func openDB() (*gorm.DB, error) {
	driver, dsn, err := databaseConfigFromEnv()
	if err != nil {
		return nil, err
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
	if err := db.AutoMigrate(&model.AdminUser{}); err != nil {
		panic(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("Admin@123456"), bcrypt.DefaultCost)
	admins := []model.AdminUser{
		{Username: "superadmin", DisplayName: "Super Admin", Role: model.AdminRoleSuper, Status: model.AccountStatusActive, PasswordHash: string(hash)},
		{Username: "admin", DisplayName: "Admin", Role: model.AdminRoleAdmin, Status: model.AccountStatusActive, PasswordHash: string(hash)},
	}
	for _, a := range admins {
		var cnt int64
		_ = db.Model(&model.AdminUser{}).Where("username = ?", a.Username).Count(&cnt).Error
		if cnt == 0 {
			if err := db.Create(&a).Error; err != nil {
				panic(err)
			}
		}
	}
	fmt.Println("bootstrap_admin done")
}
