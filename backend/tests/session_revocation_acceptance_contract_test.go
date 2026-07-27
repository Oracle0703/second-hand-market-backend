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
	// Catches calling the false-mode focused API helper before the clean
	// apply_migration_chain invocation that executes 0009 postflight.
	requireSessionCurrentChainBeforeFirstFalseModeFocusedAPI(t, string(raw))
}

func TestSessionCurrentMigrationChainRejectsEarlyFalseModeFocusedAPI(t *testing.T) {
	// This fixture models the script mutation that runs the false-mode focused
	// API helper before the clean migration-chain helper.
	script := strings.Join([]string{
		"apply_migration_chain() {",
		"  for migration in 0009_buyer_intent_open_uniqueness; do",
		"  mysql_file \"/acceptance/migrations/$migration.postflight.sql\"",
		"  done",
		"}",
		"run_focused_test false mysql-auto-migrate-false",
		"apply_migration_chain",
	}, "\n")

	err := sessionCurrentChainBeforeFirstFalseModeFocusedAPI(script)
	if err == nil || !strings.Contains(err.Error(), "precedes clean 0009 migration chain") {
		t.Fatalf("early false-mode focused API invocation error = %v, want 0009 ordering rejection", err)
	}
}

func TestSessionCurrentMigrationChainRejectsCrossPhaseFalseModeInvocation(t *testing.T) {
	// This fixture models a realistic script mutation where the clean phase
	// keeps its apply_migration_chain call, but the false-mode focused test is
	// moved into a later phase after a second clean-chain restart.
	script := strings.Join([]string{
		"apply_migration_chain() {",
		"  for migration in 0009_buyer_intent_open_uniqueness; do",
		"  mysql_file \"/acceptance/migrations/$migration.postflight.sql\"",
		"  done",
		"}",
		"apply_migration_chain",
		"run_focused_test true mysql-auto-migrate-true",
		"apply_migration_chain",
		"run_focused_test false mysql-auto-migrate-false",
	}, "\n")

	err := sessionCurrentChainBeforeFirstFalseModeFocusedAPI(script)
	if err == nil || !strings.Contains(err.Error(), "missing false-mode focused API invocation in first clean phase") {
		t.Fatalf("cross-phase false-mode invocation error = %v, want first-clean-phase rejection", err)
	}
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

func requireSessionCurrentChainBeforeFirstFalseModeFocusedAPI(t *testing.T, script string) {
	t.Helper()

	if err := sessionCurrentChainBeforeFirstFalseModeFocusedAPI(script); err != nil {
		t.Fatal(err)
	}
}

func sessionCurrentChainBeforeFirstFalseModeFocusedAPI(script string) error {
	const falseModeInvocation = "run_focused_test false mysql-auto-migrate-false"

	chainStart := strings.Index(script, "apply_migration_chain() {")
	if chainStart < 0 {
		return errors.New("script missing apply_migration_chain helper")
	}
	chainEndOffset := strings.Index(script[chainStart:], "\n}\n")
	if chainEndOffset < 0 {
		return errors.New("script has unterminated apply_migration_chain helper")
	}
	chainEnd := chainStart + chainEndOffset + len("\n}\n")
	chain := script[chainStart:chainEnd]
	if !strings.Contains(chain, "0009_buyer_intent_open_uniqueness") ||
		!strings.Contains(chain, `mysql_file "/acceptance/migrations/$migration.postflight.sql"`) {
		return errors.New("apply_migration_chain must execute 0009 postflight")
	}

	runtime := script[chainEnd:]
	cleanChainAt := strings.Index(runtime, "apply_migration_chain")
	if cleanChainAt < 0 {
		return errors.New("script missing clean apply_migration_chain invocation")
	}
	firstFalseModeAt := strings.Index(runtime, falseModeInvocation)
	if firstFalseModeAt >= 0 && firstFalseModeAt < cleanChainAt {
		return errors.New("first false-mode focused API invocation precedes clean 0009 migration chain")
	}
	phase := runtime[cleanChainAt:]
	nextCleanChainAt := strings.Index(phase[len("apply_migration_chain"):], "\napply_migration_chain\n")
	if nextCleanChainAt >= 0 {
		phase = phase[:len("apply_migration_chain")+nextCleanChainAt]
	}
	if !strings.Contains(phase, falseModeInvocation) {
		return errors.New("missing false-mode focused API invocation in first clean phase")
	}

	return nil
}
