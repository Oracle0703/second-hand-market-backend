//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/model"
)

const bootstrapPasswordSentinel = "bootstrap-password-secret-sentinel"

func TestAdminBootstrapFromEnvRequiresExplicitSafeInput(t *testing.T) {
	passwordFile := writeAdminPasswordFile(t, bootstrapPasswordSentinel, 0o600)
	tests := []struct {
		name         string
		username     string
		display      string
		role         string
		passwordFile string
		wantField    string
	}{
		{name: "requires username", display: "Admin", role: model.AdminRoleAdmin, passwordFile: passwordFile, wantField: "ADMIN_USERNAME"},
		{name: "requires display name", username: "admin", role: model.AdminRoleAdmin, passwordFile: passwordFile, wantField: "ADMIN_DISPLAY_NAME"},
		{name: "requires role", username: "admin", display: "Admin", passwordFile: passwordFile, wantField: "ADMIN_ROLE"},
		{name: "rejects unknown role", username: "admin", display: "Admin", role: "owner-" + bootstrapPasswordSentinel, passwordFile: passwordFile, wantField: "ADMIN_ROLE"},
		{name: "requires password file", username: "admin", display: "Admin", role: model.AdminRoleAdmin, wantField: "ADMIN_PASSWORD_FILE"},
		{name: "rejects relative password file", username: "admin", display: "Admin", role: model.AdminRoleAdmin, passwordFile: "relative-secret", wantField: "ADMIN_PASSWORD_FILE"},
		{name: "accepts controlled input", username: " admin ", display: " Admin ", role: model.AdminRoleAdmin, passwordFile: passwordFile},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			unsetTestEnv(t, "ADMIN_PASSWORD")
			setAdminBootstrapEnv(t, testCase.username, testCase.display, testCase.role, testCase.passwordFile)
			bootstrap, err := adminBootstrapFromEnv()
			if testCase.wantField != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantField) {
					t.Fatalf("expected %s error, got %v", testCase.wantField, err)
				}
				for _, forbidden := range []string{testCase.username, testCase.display, testCase.role, bootstrapPasswordSentinel} {
					if strings.Contains(forbidden, "sentinel") && strings.Contains(err.Error(), forbidden) {
						t.Fatalf("bootstrap config error leaked a value: %q", err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("adminBootstrapFromEnv: %v", err)
			}
			if bootstrap.Username != "admin" || bootstrap.DisplayName != "Admin" {
				t.Fatalf("identity fields were not normalized: %+v", bootstrap)
			}
			if bootstrap.Password != bootstrapPasswordSentinel {
				t.Fatal("controlled password file was not used")
			}
		})
	}
}

func TestAdminBootstrapRejectsDirectPasswordEnvironment(t *testing.T) {
	passwordFile := writeAdminPasswordFile(t, "controlled-password", 0o600)
	setAdminBootstrapEnv(t, "admin", "Admin", model.AdminRoleAdmin, passwordFile)
	t.Setenv("ADMIN_PASSWORD", bootstrapPasswordSentinel)

	_, err := adminBootstrapFromEnv()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD") {
		t.Fatalf("direct password environment error = %v", err)
	}
	if strings.Contains(err.Error(), bootstrapPasswordSentinel) {
		t.Fatalf("direct password environment error leaked the password: %q", err)
	}
}

func TestReadAdminPasswordFileAcceptsControlledRegularFiles(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		mode     os.FileMode
		want     string
	}{
		{name: "owner read write", contents: bootstrapPasswordSentinel, mode: 0o600, want: bootstrapPasswordSentinel},
		{name: "owner read only", contents: bootstrapPasswordSentinel + "\n", mode: 0o400, want: bootstrapPasswordSentinel},
		{name: "CRLF terminator", contents: bootstrapPasswordSentinel + "\r\n", mode: 0o600, want: bootstrapPasswordSentinel},
		{name: "preserves spaces", contents: " padded bootstrap password ", mode: 0o600, want: " padded bootstrap password "},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			path := writeAdminPasswordFile(t, testCase.contents, testCase.mode)
			password, err := readAdminPasswordFile(path)
			if err != nil {
				t.Fatalf("read controlled password file: %v", err)
			}
			if password != testCase.want {
				t.Fatalf("password changed: got %q", password)
			}
		})
	}
}

func TestReadAdminPasswordFileFailsClosed(t *testing.T) {
	assertRejected := func(name, path string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			_, err := readAdminPasswordFile(path)
			if err == nil {
				t.Fatal("unsafe password file was accepted")
			}
			if strings.Contains(err.Error(), bootstrapPasswordSentinel) ||
				(path != "" && strings.Contains(err.Error(), path)) {
				t.Fatalf("password file error leaked protected input: %q", err)
			}
		})
	}

	assertRejected("missing path", "")
	assertRejected("relative path", "relative-"+bootstrapPasswordSentinel)
	assertRejected("missing file", filepath.Join(t.TempDir(), bootstrapPasswordSentinel))
	assertRejected("directory", t.TempDir())
	assertRejected("empty", writeAdminPasswordFile(t, "", 0o600))
	assertRejected("whitespace", writeAdminPasswordFile(t, " \t \n", 0o600))
	assertRejected("embedded newline", writeAdminPasswordFile(t, "first\n"+bootstrapPasswordSentinel, 0o600))
	assertRejected("too long", writeAdminPasswordFile(t, strings.Repeat("x", maxAdminPasswordBytes+1), 0o600))

	if runtime.GOOS != "windows" {
		assertRejected("group readable", writeAdminPasswordFile(t, bootstrapPasswordSentinel, 0o640))
		assertRejected("world readable", writeAdminPasswordFile(t, bootstrapPasswordSentinel, 0o644))
	}

	target := writeAdminPasswordFile(t, bootstrapPasswordSentinel, 0o600)
	link := filepath.Join(t.TempDir(), "password-link")
	if err := os.Symlink(target, link); err == nil {
		assertRejected("symbolic link", link)
	}
}

func TestRunFailsBeforeDatabaseOpenWithoutControlledPassword(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "must-not-exist.sqlite")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", databasePath)
	unsetTestEnv(t, "ADMIN_PASSWORD")
	setAdminBootstrapEnv(t, "admin", "Admin", model.AdminRoleAdmin, "")

	err := run(nil)
	if err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD_FILE") {
		t.Fatalf("missing controlled password error = %v", err)
	}
	if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
		t.Fatalf("database was opened before password validation: %v", statErr)
	}
}

func TestRunRequiresExplicitSchemaAndUsesControlledPasswordFile(t *testing.T) {
	dsn := fmt.Sprintf("file:bootstrap_command_%d?mode=memory&cache=shared", time.Now().UnixNano())
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", dsn)
	unsetTestEnv(t, "ADMIN_PASSWORD")
	setAdminBootstrapEnv(
		t,
		"admin",
		"Admin",
		model.AdminRoleAdmin,
		writeAdminPasswordFile(t, bootstrapPasswordSentinel, 0o600),
	)

	if err := run(nil); err == nil || !strings.Contains(err.Error(), "ADMIN_BOOTSTRAP") {
		t.Fatalf("bootstrap without schema error = %v", err)
	}

	db, err := databasecmd.OpenDatabase(databasecmd.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open probe database: %v", err)
	}
	defer databasecmd.CloseDatabase(db)
	if err := db.AutoMigrate(&model.AdminUser{}); err != nil {
		t.Fatalf("explicit test migration: %v", err)
	}
	if err := run(nil); err != nil {
		t.Fatalf("run bootstrap: %v", err)
	}

	var admin model.AdminUser
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		t.Fatalf("load bootstrapped admin: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(bootstrapPasswordSentinel)); err != nil {
		t.Fatalf("controlled password was not used: %v", err)
	}
}

func TestRunDoesNotModifyExistingAdmin(t *testing.T) {
	dsn := fmt.Sprintf("file:bootstrap_existing_%d?mode=memory&cache=shared", time.Now().UnixNano())
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", dsn)
	unsetTestEnv(t, "ADMIN_PASSWORD")

	db, err := databasecmd.OpenDatabase(databasecmd.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open probe database: %v", err)
	}
	defer databasecmd.CloseDatabase(db)
	if err := db.AutoMigrate(&model.AdminUser{}); err != nil {
		t.Fatalf("explicit test migration: %v", err)
	}

	setAdminBootstrapEnv(
		t,
		"existing-admin",
		"Original Admin",
		model.AdminRoleAdmin,
		writeAdminPasswordFile(t, "first-controlled-password", 0o600),
	)
	if err := run(nil); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}

	var before model.AdminUser
	if err := db.Where("username = ?", "existing-admin").First(&before).Error; err != nil {
		t.Fatalf("load original admin: %v", err)
	}

	setAdminBootstrapEnv(
		t,
		"existing-admin",
		"Changed Admin",
		model.AdminRoleSuper,
		writeAdminPasswordFile(t, "second-controlled-password", 0o600),
	)
	if err := run(nil); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}

	var after model.AdminUser
	if err := db.Where("username = ?", "existing-admin").First(&after).Error; err != nil {
		t.Fatalf("load admin after second bootstrap: %v", err)
	}
	var count int64
	if err := db.Model(&model.AdminUser{}).Where("username = ?", "existing-admin").Count(&count).Error; err != nil {
		t.Fatalf("count existing admins: %v", err)
	}
	if count != 1 {
		t.Fatalf("admin count = %d, want 1", count)
	}
	if after.ID != before.ID ||
		after.Username != before.Username ||
		after.DisplayName != before.DisplayName ||
		after.Role != before.Role ||
		after.Status != before.Status ||
		after.PasswordHash != before.PasswordHash ||
		!after.CreatedAt.Equal(before.CreatedAt) ||
		!after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("existing admin was modified:\nbefore=%+v\nafter=%+v", before, after)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte("first-controlled-password")); err != nil {
		t.Fatal("existing password was not retained")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte("second-controlled-password")); err == nil {
		t.Fatal("existing password was overwritten")
	}
}

func TestRunRejectsArgumentsBeforeReadingSecretsOrDatabaseConfig(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "must-not-exist.sqlite")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", databasePath)
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_DISPLAY_NAME", "Admin")
	t.Setenv("ADMIN_ROLE", model.AdminRoleAdmin)
	t.Setenv("ADMIN_PASSWORD_FILE", filepath.Join(t.TempDir(), bootstrapPasswordSentinel))

	err := run([]string{bootstrapPasswordSentinel})
	if err == nil || !strings.Contains(err.Error(), "ADMIN_BOOTSTRAP_ARGUMENTS") {
		t.Fatalf("argument validation error = %v", err)
	}
	if strings.Contains(err.Error(), bootstrapPasswordSentinel) {
		t.Fatalf("argument validation error leaked input: %q", err)
	}
	if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
		t.Fatalf("database was opened before argument validation: %v", statErr)
	}
}

func setAdminBootstrapEnv(t *testing.T, username, display, role, passwordFile string) {
	t.Helper()
	t.Setenv("ADMIN_USERNAME", username)
	t.Setenv("ADMIN_DISPLAY_NAME", display)
	t.Setenv("ADMIN_ROLE", role)
	t.Setenv("ADMIN_PASSWORD_FILE", passwordFile)
}

func writeAdminPasswordFile(t *testing.T, contents string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "admin-password")
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("set password file mode: %v", err)
	}
	return path
}

func unsetTestEnv(t *testing.T, key string) {
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
