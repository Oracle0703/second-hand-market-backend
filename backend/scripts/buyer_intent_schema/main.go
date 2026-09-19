package main

import (
	"errors"
	"log"
	"os"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/databasecmd"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	log.Print("BUYER_INTENT_SCHEMA PASS")
}

func run(args []string) error {
	if len(args) != 1 || (args[0] != "--check" && args[0] != "--apply-sqlite") {
		return errors.New("select exactly one of --check or --apply-sqlite")
	}
	cfg, err := databasecmd.LoadConfig()
	if err != nil {
		return err
	}
	if args[0] == "--apply-sqlite" && cfg.Driver != "sqlite" {
		return errors.New("use migration 0011_buyer_intent_open_uniqueness for MySQL")
	}
	db, err := databasecmd.OpenDatabase(cfg)
	if err != nil {
		return err
	}
	defer databasecmd.CloseDatabase(db)
	if args[0] == "--apply-sqlite" {
		err = app.MigrateSQLiteBuyerIntentSchema(db)
	} else {
		err = app.VerifyBuyerIntentSchema(db)
	}
	if err != nil {
		return errors.New("BUYER_INTENT_SCHEMA failed; inspect schema and row state")
	}
	return nil
}
