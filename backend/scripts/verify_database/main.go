package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"second-hand-market-backend/backend/internal/app"
)

func main() {
	os.Exit(run(verifyDatabase, os.Stdout, os.Stderr))
}

func run(verify func() error, stdout io.Writer, stderr io.Writer) int {
	if err := verify(); err != nil {
		_, _ = fmt.Fprintln(stderr, "DATABASE_IDENTITY FAIL")
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "DATABASE_IDENTITY PASS")
	return 0
}

func verifyDatabase() error {
	cfg := app.LoadConfig()
	if cfg.DBTarget != "remote-development" {
		return errors.New("DB_TARGET must be remote-development")
	}
	srv, err := app.NewServer(cfg)
	if err != nil {
		return errors.New("DATABASE_IDENTITY verification failed")
	}
	sqlDB, err := srv.DB.DB()
	if err != nil {
		return errors.New("DATABASE_IDENTITY connection unavailable")
	}
	if err := sqlDB.Close(); err != nil {
		return errors.New("DATABASE_IDENTITY connection close failed")
	}
	return nil
}
