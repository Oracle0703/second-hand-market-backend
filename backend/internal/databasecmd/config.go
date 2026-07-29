package databasecmd

import (
	"errors"
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Driver string
	DSN    string
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Driver: strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER"))),
		DSN:    os.Getenv("DB_DSN"),
	}
	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func OpenDatabase(cfg Config) (*gorm.DB, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	var dialector gorm.Dialector
	switch cfg.Driver {
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, errors.New("DATABASE_CONNECTION failed")
	}
	return db, nil
}

func CloseDatabase(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func validateConfig(cfg Config) error {
	switch cfg.Driver {
	case "mysql", "sqlite":
	case "":
		return errors.New("DB_DRIVER is required")
	default:
		return errors.New("DB_DRIVER must be mysql or sqlite")
	}
	if strings.TrimSpace(cfg.DSN) == "" {
		return errors.New("DB_DSN is required")
	}
	return nil
}
