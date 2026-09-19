// This executable is used only by browser tests; it never loads deployment config.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/model"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	if os.Getenv("E2E_FIXTURE") != "1" {
		return fmt.Errorf("E2E_FIXTURE=1 is required")
	}
	root, err := os.MkdirTemp("", "secondhand-browser-fixture-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	cfg := app.Config{AppEnv: "test", Addr: "127.0.0.1:19080", DBDriver: "sqlite", DBTarget: "local", DBDSN: "file:" + filepath.Join(root, "fixture.db") + "?_foreign_keys=on", JWTAccessSecret: "synthetic-browser-access-secret", JWTRefreshSecret: "synthetic-browser-refresh-secret", AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour, FileStorageProvider: "local", FileUploadLocalDir: filepath.Join(root, "uploads"), ImageProcessorDriver: "passthrough", BuyerWechatLoginMode: "disabled", BuyerDouyinLoginMode: "disabled"}
	s, err := app.NewServer(cfg)
	if err != nil {
		return err
	}
	sqlDB, err := s.DB.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := app.MigrateSchema(s.DB); err != nil {
		return err
	}
	if err := app.SeedDefaultCategories(s.DB); err != nil {
		return err
	}
	for _, account := range []app.AdminBootstrap{
		{Username: "e2e_admin", DisplayName: "Browser Administrator", Role: model.AdminRoleSuper, Password: "BrowserAdminSeed123!"},
		{Username: "e2e_security", DisplayName: "Browser Security", Role: model.AdminRoleAdmin, Password: "BrowserSecuritySeed123!"},
	} {
		if err := app.BootstrapAdmin(s.DB, account); err != nil {
			return err
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return s.RunContext(ctx)
}
