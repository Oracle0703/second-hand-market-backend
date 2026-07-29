package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/model"
)

const bootstrapPasswordSentinel = "bootstrap-password-secret-sentinel"

func TestAdminBootstrapFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		display   string
		role      string
		password  string
		wantField string
	}{
		{name: "requires username", display: "Admin", role: model.AdminRoleAdmin, password: bootstrapPasswordSentinel, wantField: "ADMIN_USERNAME"},
		{name: "requires display name", username: "admin", role: model.AdminRoleAdmin, password: bootstrapPasswordSentinel, wantField: "ADMIN_DISPLAY_NAME"},
		{name: "requires role", username: "admin", display: "Admin", password: bootstrapPasswordSentinel, wantField: "ADMIN_ROLE"},
		{name: "rejects unknown role", username: "admin", display: "Admin", role: "owner-secret-sentinel", password: bootstrapPasswordSentinel, wantField: "ADMIN_ROLE"},
		{name: "requires password", username: "admin", display: "Admin", role: model.AdminRoleAdmin, password: " \t ", wantField: "ADMIN_PASSWORD"},
		{name: "accepts explicit input", username: " admin ", display: " Admin ", role: model.AdminRoleAdmin, password: bootstrapPasswordSentinel},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setAdminBootstrapEnv(t, tc.username, tc.display, tc.role, tc.password)
			bootstrap, err := adminBootstrapFromEnv()
			if tc.wantField != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantField) {
					t.Fatalf("expected %s error, got %v", tc.wantField, err)
				}
				for _, forbidden := range []string{tc.username, tc.display, tc.role, tc.password} {
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
				t.Fatal("password was modified")
			}
		})
	}
}

func TestRunRequiresExplicitSchemaAndUsesExplicitPassword(t *testing.T) {
	dsn := fmt.Sprintf("file:bootstrap_command_%d?mode=memory&cache=shared", time.Now().UnixNano())
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", dsn)
	setAdminBootstrapEnv(t, "admin", "Admin", model.AdminRoleAdmin, bootstrapPasswordSentinel)

	if err := run(); err == nil || !strings.Contains(err.Error(), "ADMIN_BOOTSTRAP") {
		t.Fatalf("bootstrap without schema error = %v", err)
	}

	db, err := databasecmd.OpenDatabase(databasecmd.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open probe database: %v", err)
	}
	if err := db.AutoMigrate(&model.AdminUser{}); err != nil {
		t.Fatalf("explicit test migration: %v", err)
	}
	if err := run(); err != nil {
		t.Fatalf("run bootstrap: %v", err)
	}

	var admin model.AdminUser
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		t.Fatalf("load bootstrapped admin: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(bootstrapPasswordSentinel)); err != nil {
		t.Fatalf("explicit password was not used: %v", err)
	}
}

func setAdminBootstrapEnv(t *testing.T, username, display, role, password string) {
	t.Helper()
	t.Setenv("ADMIN_USERNAME", username)
	t.Setenv("ADMIN_DISPLAY_NAME", display)
	t.Setenv("ADMIN_ROLE", role)
	t.Setenv("ADMIN_PASSWORD", password)
}
