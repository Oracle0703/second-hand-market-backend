package main

import (
	"errors"
	"log"
	"os"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/databasecmd"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	log.Print("CATEGORY_SEED PASS")
}

func run() error {
	databaseConfig, err := databasecmd.LoadConfig()
	if err != nil {
		return err
	}
	db, err := databasecmd.OpenDatabase(databaseConfig)
	if err != nil {
		return err
	}
	defer databasecmd.CloseDatabase(db)
	if err := app.SeedDefaultCategories(db); err != nil {
		return errors.New("CATEGORY_SEED failed")
	}
	return nil
}
