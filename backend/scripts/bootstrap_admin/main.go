package main

import (
	"errors"
	"log"
	"os"
	"strings"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/model"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	log.Print("ADMIN_BOOTSTRAP PASS")
}

func run() error {
	databaseConfig, err := databasecmd.LoadConfig()
	if err != nil {
		return err
	}
	bootstrap, err := adminBootstrapFromEnv()
	if err != nil {
		return err
	}
	db, err := databasecmd.OpenDatabase(databaseConfig)
	if err != nil {
		return err
	}
	defer databasecmd.CloseDatabase(db)
	if err := app.BootstrapAdmin(db, bootstrap); err != nil {
		return errors.New("ADMIN_BOOTSTRAP failed")
	}
	return nil
}

func adminBootstrapFromEnv() (app.AdminBootstrap, error) {
	bootstrap := app.AdminBootstrap{
		Username:    strings.TrimSpace(os.Getenv("ADMIN_USERNAME")),
		DisplayName: strings.TrimSpace(os.Getenv("ADMIN_DISPLAY_NAME")),
		Role:        strings.TrimSpace(os.Getenv("ADMIN_ROLE")),
		Password:    os.Getenv("ADMIN_PASSWORD"),
	}
	if bootstrap.Username == "" {
		return app.AdminBootstrap{}, errors.New("ADMIN_USERNAME is required")
	}
	if bootstrap.DisplayName == "" {
		return app.AdminBootstrap{}, errors.New("ADMIN_DISPLAY_NAME is required")
	}
	if bootstrap.Role != model.AdminRoleAdmin && bootstrap.Role != model.AdminRoleSuper {
		return app.AdminBootstrap{}, errors.New("ADMIN_ROLE must be ADMIN or SUPER_ADMIN")
	}
	if strings.TrimSpace(bootstrap.Password) == "" {
		return app.AdminBootstrap{}, errors.New("ADMIN_PASSWORD is required")
	}
	return bootstrap, nil
}
