package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionRevocationAcceptanceRejectsUnsafeEnvironmentBeforeDocker(t *testing.T) {
	script := "../../deploy/acceptance/session-revocation-smoke.sh"
	if info, err := os.Stat(script); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("acceptance script must exist as a regular file: %v", err)
	}
	stubDir := t.TempDir()
	dockerCalled := filepath.Join(stubDir, "docker-called")
	dockerStub := filepath.Join(stubDir, "docker")
	stub := "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n"
	if err := os.WriteFile(dockerStub, []byte(stub), 0o700); err != nil {
		t.Fatalf("write docker stub: %v", err)
	}
	cases := []struct {
		name    string
		confirm string
		engine  string
		project string
	}{
		{name: "missing confirmation", engine: "mysql8.4"},
		{name: "wrong confirmation", confirm: "unsafe", engine: "mysql8.4"},
		{name: "wrong database engine", confirm: "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA", engine: "mysql8.0"},
		{name: "wrong compose project", confirm: "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA", engine: "mysql8.4", project: "secondhand-market"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Remove(dockerCalled); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("reset docker marker: %v", err)
			}
			cmd := exec.Command("/bin/bash", script)
			cmd.Env = []string{
				"PATH=" + stubDir + ":/usr/bin:/bin",
				"DOCKER_CALLED=" + dockerCalled,
				"SESSION_REVOCATION_ACCEPTANCE_CONFIRM=" + tc.confirm,
				"ACCEPTANCE_DB_ENGINE=" + tc.engine,
				"COMPOSE_PROJECT_NAME=" + tc.project,
			}
			if err := cmd.Run(); err == nil {
				t.Fatal("unsafe acceptance environment succeeded")
			}
			if _, err := os.Stat(dockerCalled); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unsafe acceptance environment reached docker")
			}
		})
	}
}

func TestSessionRevocationAcceptanceUsesCurrentMigrationChain(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/acceptance/session-revocation-smoke.sh")
	if err != nil {
		t.Fatalf("read session revocation acceptance script: %v", err)
	}

	requireOrderedSessionSnippets(t, string(raw), []string{
		"for migration in 0004_merchant_multi_stock 0005_file_records_table \\",
		"    0006_file_binding_ownership 0007_license_file_privacy \\",
		"    0008_anonymous_upload_governance 0009_buyer_intent_open_uniqueness; do",
		`    mysql_file "/acceptance/migrations/$migration.preflight.sql"`,
		`    mysql_file "/acceptance/migrations/$migration.up.sql"`,
		`    mysql_file "/acceptance/migrations/$migration.postflight.sql"`,
		"apply_migration_chain",
		"run_focused_test false mysql-auto-migrate-false",
	})
}

func requireOrderedSessionSnippets(t *testing.T, text string, snippets []string) {
	t.Helper()

	offset := 0
	for _, snippet := range snippets {
		index := strings.Index(text[offset:], snippet)
		if index < 0 {
			t.Fatalf("script missing ordered snippet %q", snippet)
		}
		offset += index + len(snippet)
	}
}
