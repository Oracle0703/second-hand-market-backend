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
	log.Print("DATABASE_MIGRATION PASS")
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
	if err := app.MigrateSchema(db); err != nil {
		return errors.New("DATABASE_MIGRATION failed")
	}
	return nil
}
