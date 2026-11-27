package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"second-hand-market-backend/backend/internal/app"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := check(ctx); err != nil {
		log.Fatal(err)
	}
}

func check(ctx context.Context) error {
	cfg := app.LoadConfig()
	if strings.TrimSpace(cfg.DBDSN) == "" {
		return errors.New("DB_DSN is required")
	}
	switch cfg.DBDriver {
	case "mysql":
	case "sqlite":
		log.Print("InnoDB preflight skipped for SQLite")
		return nil
	default:
		return errors.New("DB_DRIVER must be mysql or sqlite")
	}
	// sql.Open is lazy: the first connection and MySQL handshake use QueryContext's deadline.
	db, err := sql.Open("mysql", cfg.DBDSN)
	if err != nil {
		return errors.New("invalid MySQL connection configuration")
	}
	defer db.Close()
	if err := app.VerifyTransactionTables(ctx, db); err != nil {
		return err
	}
	log.Print("TRANSACTION_TABLES PASS")
	return nil
}
