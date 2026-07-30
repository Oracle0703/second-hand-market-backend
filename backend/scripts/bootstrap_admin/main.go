package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/model"
)

const maxAdminPasswordBytes = 72

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	log.Print("ADMIN_BOOTSTRAP PASS")
}

func run(args []string) error {
	return runWithHiddenPasswordReader(args, readHiddenAdminPassword)
}

func runWithHiddenPasswordReader(
	args []string,
	hiddenPasswordReader func() (string, error),
) error {
	if len(args) != 0 {
		return errors.New("ADMIN_BOOTSTRAP_ARGUMENTS are invalid")
	}

	bootstrap, err := adminBootstrapFromEnvWithHiddenPasswordReader(hiddenPasswordReader)
	if err != nil {
		return err
	}
	databaseConfig, err := databasecmd.LoadConfig()
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
	return adminBootstrapFromEnvWithHiddenPasswordReader(readHiddenAdminPassword)
}

func adminBootstrapFromEnvWithHiddenPasswordReader(
	hiddenPasswordReader func() (string, error),
) (app.AdminBootstrap, error) {
	if _, exists := os.LookupEnv("ADMIN_PASSWORD"); exists {
		return app.AdminBootstrap{}, errors.New("ADMIN_PASSWORD must not be set")
	}

	bootstrap := app.AdminBootstrap{
		Username:    strings.TrimSpace(os.Getenv("ADMIN_USERNAME")),
		DisplayName: strings.TrimSpace(os.Getenv("ADMIN_DISPLAY_NAME")),
		Role:        strings.TrimSpace(os.Getenv("ADMIN_ROLE")),
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

	passwordFile, passwordFileExists := os.LookupEnv("ADMIN_PASSWORD_FILE")
	var password string
	if passwordFileExists {
		if strings.TrimSpace(passwordFile) == "" ||
			strings.TrimSpace(passwordFile) != passwordFile ||
			!filepath.IsAbs(passwordFile) {
			return app.AdminBootstrap{}, errors.New("ADMIN_PASSWORD_FILE is invalid")
		}
		var err error
		password, err = readAdminPasswordFile(passwordFile)
		if err != nil {
			return app.AdminBootstrap{}, errors.New("ADMIN_PASSWORD_FILE is invalid")
		}
	} else {
		var err error
		password, err = hiddenPasswordReader()
		if err != nil {
			return app.AdminBootstrap{}, err
		}
	}
	bootstrap.Password = password
	return bootstrap, nil
}

func readAdminPasswordFile(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", errors.New("password file is invalid")
	}

	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("password file is invalid")
	}
	if !hasControlledPasswordFileMode(before.Mode()) {
		return "", errors.New("password file is invalid")
	}

	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("password file is invalid")
	}
	defer file.Close()

	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return "", errors.New("password file is invalid")
	}
	if !hasControlledPasswordFileMode(after.Mode()) {
		return "", errors.New("password file is invalid")
	}

	data, err := io.ReadAll(io.LimitReader(file, maxAdminPasswordBytes+3))
	if err != nil {
		return "", errors.New("password file is invalid")
	}
	defer clear(data)

	return normalizeAdminPassword(data)
}

func normalizeAdminPassword(data []byte) (string, error) {
	switch {
	case bytes.HasSuffix(data, []byte("\r\n")):
		data = data[:len(data)-2]
	case bytes.HasSuffix(data, []byte("\n")):
		data = data[:len(data)-1]
	}
	if len(data) == 0 || len(data) > maxAdminPasswordBytes ||
		bytes.IndexAny(data, "\r\n\x00") >= 0 ||
		strings.TrimSpace(string(data)) == "" {
		return "", errors.New("password file is invalid")
	}
	return string(data), nil
}
