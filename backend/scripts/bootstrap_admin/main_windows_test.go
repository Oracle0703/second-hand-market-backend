//go:build windows

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"second-hand-market-backend/backend/internal/model"
)

const windowsBootstrapPasswordSentinel = "windows-bootstrap-password-secret-sentinel"

func TestWindowsPasswordFileFailsClosedWithoutACLValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin-password")
	if err := os.WriteFile(path, []byte(windowsBootstrapPasswordSentinel), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}

	_, err := readAdminPasswordFile(path)
	if err == nil {
		t.Fatal("Windows password file was accepted without ACL validation")
	}
	if strings.Contains(err.Error(), path) ||
		strings.Contains(err.Error(), windowsBootstrapPasswordSentinel) {
		t.Fatalf("password file error leaked protected input: %q", err)
	}
}

func TestWindowsBootstrapAcceptsInjectedHiddenPassword(t *testing.T) {
	unsetWindowsTestEnv(t, "ADMIN_PASSWORD")
	unsetWindowsTestEnv(t, "ADMIN_PASSWORD_FILE")
	t.Setenv("ADMIN_USERNAME", " admin ")
	t.Setenv("ADMIN_DISPLAY_NAME", " Admin ")
	t.Setenv("ADMIN_ROLE", model.AdminRoleAdmin)

	bootstrap, err := adminBootstrapFromEnvWithHiddenPasswordReader(func() (string, error) {
		return windowsBootstrapPasswordSentinel, nil
	})
	if err != nil {
		t.Fatalf("load hidden bootstrap password: %v", err)
	}
	if bootstrap.Username != "admin" ||
		bootstrap.DisplayName != "Admin" ||
		bootstrap.Password != windowsBootstrapPasswordSentinel {
		t.Fatalf("hidden bootstrap input was not preserved: %+v", bootstrap)
	}
}

func TestWindowsRunRejectsArgumentsBeforeSecretsAndDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "must-not-exist.sqlite")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", databasePath)

	err := run([]string{windowsBootstrapPasswordSentinel})
	if err == nil || !strings.Contains(err.Error(), "ADMIN_BOOTSTRAP_ARGUMENTS") {
		t.Fatalf("argument validation error = %v", err)
	}
	if strings.Contains(err.Error(), windowsBootstrapPasswordSentinel) {
		t.Fatalf("argument validation error leaked input: %q", err)
	}
	if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
		t.Fatalf("database was opened before argument validation: %v", statErr)
	}
}

func TestWindowsHiddenInputDrainsOverflowBeforeRestoringEcho(t *testing.T) {
	overflowSecret := strings.Repeat("s", maxAdminPasswordBytes+20)
	input := strings.NewReader(overflowSecret + "\r\nnext-command\r\n")
	var prompt bytes.Buffer
	originalMode := uint32(windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT)
	var modes []uint32

	password, err := readHiddenAdminPasswordFromConsole(
		input,
		&prompt,
		func(mode *uint32) error {
			*mode = originalMode
			return nil
		},
		func(mode uint32) error {
			modes = append(modes, mode)
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD_INPUT") {
		t.Fatalf("overflow error = %v", err)
	}
	if password != "" ||
		strings.Contains(err.Error(), overflowSecret) ||
		strings.Contains(prompt.String(), overflowSecret) {
		t.Fatal("overflowing hidden password was retained or leaked")
	}
	if len(modes) != 2 {
		t.Fatalf("console mode changes = %v", modes)
	}
	if modes[0]&windows.ENABLE_ECHO_INPUT != 0 {
		t.Fatalf("echo was not disabled before reading: mode=%d", modes[0])
	}
	if modes[1] != originalMode {
		t.Fatalf("console mode was not restored: got=%d want=%d", modes[1], originalMode)
	}
	remainder, readErr := io.ReadAll(input)
	if readErr != nil {
		t.Fatalf("read remaining console input: %v", readErr)
	}
	if string(remainder) != "next-command\r\n" {
		t.Fatalf("hidden input drain consumed the next line: %q", remainder)
	}
}

func unsetWindowsTestEnv(t *testing.T, key string) {
	t.Helper()
	previous, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, previous)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
