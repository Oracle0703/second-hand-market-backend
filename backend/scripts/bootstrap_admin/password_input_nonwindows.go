//go:build !windows

package main

import (
	"errors"
	"os"
)

func hasControlledPasswordFileMode(mode os.FileMode) bool {
	permissions := mode.Perm()
	return permissions == 0o400 || permissions == 0o600
}

func readHiddenAdminPassword() (string, error) {
	return "", errors.New("ADMIN_PASSWORD_FILE is required")
}
