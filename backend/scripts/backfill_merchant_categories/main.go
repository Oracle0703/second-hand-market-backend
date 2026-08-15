package main

import (
	"errors"
	"log"
	"os"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/databasecmd"
)

type backfillDependencies struct {
	loadConfig    func() (databasecmd.Config, error)
	openDatabase  func(databasecmd.Config) (*gorm.DB, error)
	closeDatabase func(*gorm.DB)
	backfill      func(*gorm.DB) error
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	log.Print("BACKFILL_MERCHANT_CATEGORIES PASS")
}

func defaultBackfillDependencies() backfillDependencies {
	return backfillDependencies{
		loadConfig:    databasecmd.LoadConfig,
		openDatabase:  databasecmd.OpenDatabase,
		closeDatabase: databasecmd.CloseDatabase,
		backfill:      app.BackfillMerchantCategories,
	}
}

func run(args []string) error {
	return runWithDependencies(args, defaultBackfillDependencies())
}

func runWithDependencies(args []string, dependencies backfillDependencies) error {
	if len(args) != 0 {
		return errors.New("BACKFILL_MERCHANT_CATEGORIES_ARGUMENTS are invalid")
	}
	cfg, err := dependencies.loadConfig()
	if err != nil {
		return err
	}
	db, err := dependencies.openDatabase(cfg)
	if err != nil {
		return err
	}
	defer dependencies.closeDatabase(db)
	if err := dependencies.backfill(db); err != nil {
		return errors.New("BACKFILL_MERCHANT_CATEGORIES failed")
	}
	return nil
}
