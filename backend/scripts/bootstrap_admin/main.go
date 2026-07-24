package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/auth"
	"second-hand-market-backend/backend/internal/model"
)

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func openDB() (*gorm.DB, error) {
	driver, err := requiredEnv("DB_DRIVER")
	if err != nil {
		return nil, err
	}
	dsn, err := requiredEnv("DB_DSN")
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(driver) {
	case "mysql":
		return gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "sqlite":
		return gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER: %s", driver)
	}
}

func readPasswordFile() (string, error) {
	path, err := requiredEnv("ADMIN_BOOTSTRAP_PASSWORD_FILE")
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("password file must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("password file permissions must be 0600 or stricter")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if !auth.IsSafeAdministratorPassword(password) {
		return "", fmt.Errorf("administrator password must be 12-72 bytes and must not use a known default")
	}
	return password, nil
}

func main() {
	username, err := requiredEnv("ADMIN_BOOTSTRAP_USERNAME")
	if err != nil {
		panic(err)
	}
	role, err := requiredEnv("ADMIN_BOOTSTRAP_ROLE")
	if err != nil {
		panic(err)
	}
	role = strings.ToUpper(role)
	if role != model.AdminRoleAdmin && role != model.AdminRoleSuper {
		panic("ADMIN_BOOTSTRAP_ROLE must be ADMIN or SUPER_ADMIN")
	}
	password, err := readPasswordFile()
	if err != nil {
		panic(err)
	}
	db, err := openDB()
	if err != nil {
		panic(err)
	}
	if !db.Migrator().HasTable(&model.AdminUser{}) {
		panic("admin_users table does not exist; run schema migrations first")
	}
	var count int64
	if err := db.Unscoped().Model(&model.AdminUser{}).Where("username = ?", username).Count(&count).Error; err != nil {
		panic(err)
	}
	if count > 0 {
		panic("administrator already exists; bootstrap never overwrites existing accounts")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	displayName := strings.TrimSpace(os.Getenv("ADMIN_BOOTSTRAP_DISPLAY_NAME"))
	if displayName == "" {
		displayName = username
	}
	admin := model.AdminUser{
		Username:     username,
		DisplayName:  displayName,
		Role:         role,
		Status:       model.AccountStatusActive,
		PasswordHash: string(hash),
	}
	if err := db.Create(&admin).Error; err != nil {
		panic(err)
	}
	fmt.Printf("administrator %s created\n", username)
}
