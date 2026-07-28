package tests

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionRevocationAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T) {
	repoDir := sessionRevocationAcceptanceRepoDir(t)
	script := filepath.Join(repoDir, "deploy/acceptance/session-revocation-smoke.sh")
	cmd := exec.Command("/bin/bash", script)
	cmd.Dir = repoDir
	cmd.Env = []string{
		"SESSION_REVOCATION_SOURCE_LIST_ONLY=1",
		"PATH=" + os.Getenv("PATH"),
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run session revocation source-list mode: %v: %s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("source-list mode wrote stderr: %q", stderr.String())
	}

	paths := splitNULPaths(t, stdout.Bytes())
	present := make(map[string]bool, len(paths))
	previous := ""
	for _, path := range paths {
		if present[path] {
			t.Fatalf("source-list mode emitted duplicate path %q", path)
		}
		if previous != "" && path <= previous {
			t.Fatalf("source-list mode is not bytewise sorted: %q before %q", previous, path)
		}
		previous = path
		present[path] = true
		if forbiddenIdempotencyAcceptancePath(path) {
			t.Fatalf("source-list mode emitted forbidden path %q", path)
		}
		headCheck := exec.Command("git", "cat-file", "-e", "HEAD:"+path)
		headCheck.Dir = repoDir
		if output, err := headCheck.CombinedOutput(); err != nil {
			t.Fatalf("source-list path %q is not in HEAD: %v: %s", path, err, output)
		}
	}

	for _, path := range requiredSessionRevocationAcceptancePaths() {
		if !present[path] {
			t.Errorf("source-list mode omitted required committed path %q", path)
		}
	}

	t.Run("excludes staged and untracked allowed-looking paths", func(t *testing.T) {
		fixtureRepo, fixtureScript := newSessionRevocationAcceptanceFixtureRepo(t)
		stagedPath := "backend/staged_only.go"
		untrackedPath := "backend/untracked.go"
		writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, stagedPath, "package backend\n", 0o600)
		runIdempotencyAcceptanceGit(t, fixtureRepo, "add", "--", stagedPath)
		writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, untrackedPath, "package backend\n", 0o600)

		cmd := exec.Command("/bin/bash", fixtureScript)
		cmd.Dir = fixtureRepo
		cmd.Env = []string{
			"SESSION_REVOCATION_SOURCE_LIST_ONLY=1",
			"PATH=" + os.Getenv("PATH"),
		}
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("run fixture source-list mode: %v", err)
		}
		for _, path := range splitNULPaths(t, output) {
			if path == stagedPath || path == untrackedPath {
				t.Fatalf("source-list mode included non-HEAD path %q", path)
			}
		}
	})
}

func TestSessionRevocationAcceptanceSourceExportUsesImmutableHEAD(t *testing.T) {
	fixtureRepo, fixtureScript := newSessionRevocationAcceptanceFixtureRepo(t)
	writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, "Makefile", "fixture:\n\t@false\n", 0o600)
	stagedPath := "backend/staged_only.go"
	untrackedPath := "backend/untracked.go"
	writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, stagedPath, "package backend\n", 0o600)
	runIdempotencyAcceptanceGit(t, fixtureRepo, "add", "--", stagedPath)
	writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, untrackedPath, "package backend\n", 0o600)

	tripwireDir := t.TempDir()
	dockerMarker := filepath.Join(tripwireDir, "docker-called")
	writeIdempotencyAcceptanceFixtureFile(t, tripwireDir, "docker", "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n", 0o700)
	exportDir := filepath.Join(t.TempDir(), "source-package")
	cmd := exec.Command("/bin/bash", fixtureScript)
	cmd.Dir = fixtureRepo
	cmd.Env = []string{
		"SESSION_REVOCATION_SOURCE_EXPORT_DIR=" + exportDir,
		"DOCKER_CALLED=" + dockerMarker,
		"PATH=" + tripwireDir + ":" + os.Getenv("PATH"),
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export immutable session revocation HEAD package: %v: %s", err, output)
	}
	if _, err := os.Stat(dockerMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source export reached Docker")
	}
	if info, err := os.Stat(exportDir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("source export directory mode = %v, %v; want 0700", info, err)
	}

	wantArtifacts := map[string]bool{
		"source-files.z":     true,
		"source-sha256.txt":  true,
		"source.tar":         true,
		"package-sha256.txt": true,
	}
	entries, err := os.ReadDir(exportDir)
	if err != nil {
		t.Fatalf("read source export directory: %v", err)
	}
	if len(entries) != len(wantArtifacts) {
		t.Fatalf("source export contains %d entries, want exactly %d", len(entries), len(wantArtifacts))
	}
	for _, entry := range entries {
		if !wantArtifacts[entry.Name()] {
			t.Fatalf("source export contains unexpected artifact %q", entry.Name())
		}
		info, err := os.Lstat(filepath.Join(exportDir, entry.Name()))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
			t.Fatalf("source export artifact %q must be a mode-0600 regular non-symlink file: %v, %v", entry.Name(), info, err)
		}
	}

	packageManifest, err := os.ReadFile(filepath.Join(exportDir, "package-sha256.txt"))
	if err != nil {
		t.Fatalf("read package checksum manifest: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(packageManifest), "\n"), "\n")
	wantManifestNames := []string{"source-files.z", "source-sha256.txt", "source.tar"}
	if len(lines) != len(wantManifestNames) {
		t.Fatalf("package checksum manifest has %d lines, want 3", len(lines))
	}
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 || fields[1] != wantManifestNames[i] {
			t.Fatalf("package checksum line %d = %q, want hash for %s", i+1, line, wantManifestNames[i])
		}
	}
	packageCheck := exec.Command("sha256sum", "-c", "package-sha256.txt")
	packageCheck.Dir = exportDir
	if output, err := packageCheck.CombinedOutput(); err != nil {
		t.Fatalf("verify package checksums: %v: %s", err, output)
	}

	rawList, err := os.ReadFile(filepath.Join(exportDir, "source-files.z"))
	if err != nil {
		t.Fatalf("read exported source list: %v", err)
	}
	for _, path := range splitNULPaths(t, rawList) {
		if path == stagedPath || path == untrackedPath {
			t.Fatalf("source export included non-HEAD path %q", path)
		}
	}
	extracted := t.TempDir()
	extractIdempotencyAcceptanceTar(t, filepath.Join(exportDir, "source.tar"), extracted)
	gotMakefile, err := os.ReadFile(filepath.Join(extracted, "Makefile"))
	if err != nil {
		t.Fatalf("read exported Makefile: %v", err)
	}
	if string(gotMakefile) != "fixture:\n\t@true\n" {
		t.Fatalf("exported Makefile = %q, want committed HEAD bytes", gotMakefile)
	}
	sourceCheck := exec.Command("sha256sum", "-c", filepath.Join(exportDir, "source-sha256.txt"))
	sourceCheck.Dir = extracted
	if output, err := sourceCheck.CombinedOutput(); err != nil {
		t.Fatalf("verify exported source manifest: %v: %s", err, output)
	}

	t.Run("rejects unsafe or ambiguous destinations", func(t *testing.T) {
		preexisting := filepath.Join(t.TempDir(), "existing")
		if err := os.Mkdir(preexisting, 0o700); err != nil {
			t.Fatalf("create pre-existing destination: %v", err)
		}
		cases := []struct {
			name string
			env  []string
		}{
			{name: "relative", env: []string{"SESSION_REVOCATION_SOURCE_EXPORT_DIR=relative-package"}},
			{name: "root", env: []string{"SESSION_REVOCATION_SOURCE_EXPORT_DIR=/"}},
			{name: "pre-existing", env: []string{"SESSION_REVOCATION_SOURCE_EXPORT_DIR=" + preexisting}},
			{name: "list and export", env: []string{"SESSION_REVOCATION_SOURCE_LIST_ONLY=1", "SESSION_REVOCATION_SOURCE_EXPORT_DIR=" + filepath.Join(t.TempDir(), "package")}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "docker-called")
				command := exec.Command("/bin/bash", fixtureScript)
				command.Dir = fixtureRepo
				command.Env = append([]string{
					"DOCKER_CALLED=" + marker,
					"PATH=" + tripwireDir + ":" + os.Getenv("PATH"),
				}, tc.env...)
				if output, err := command.CombinedOutput(); err == nil {
					t.Fatalf("unsafe source destination succeeded: %s", output)
				}
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("unsafe source destination reached Docker")
				}
			})
		}
	})
}

func requiredSessionRevocationAcceptancePaths() []string {
	return []string{
		"Makefile",
		"backend/Dockerfile",
		"backend/go.mod",
		"backend/go.sum",
		"backend/internal/app/admin_handlers.go",
		"backend/internal/app/auth_handlers.go",
		"backend/internal/app/server.go",
		"backend/migrations/0001_init.up.sql",
		"backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql",
		"backend/migrations/0009_buyer_intent_open_uniqueness.up.sql",
		"backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql",
		"backend/tests/session_revocation_acceptance_contract_test.go",
		"backend/tests/session_revocation_mysql_test.go",
		"deploy/acceptance/README.md",
		"deploy/acceptance/docker-compose.yml",
		"deploy/acceptance/session-revocation-smoke.sh",
	}
}

func newSessionRevocationAcceptanceFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	fixtureRepo := t.TempDir()
	realScript := filepath.Join(sessionRevocationAcceptanceRepoDir(t), "deploy", "acceptance", "session-revocation-smoke.sh")
	scriptBytes, err := os.ReadFile(realScript)
	if err != nil {
		t.Fatalf("read real session revocation script: %v", err)
	}
	for _, path := range requiredSessionRevocationAcceptancePaths() {
		content := "fixture\n"
		switch {
		case path == "Makefile":
			content = "fixture:\n\t@true\n"
		case path == "backend/Dockerfile":
			content = "FROM scratch\n"
		case path == "backend/go.mod":
			content = "module fixture.invalid/session-revocation\n\ngo 1.22\n"
		case path == "backend/go.sum":
			content = ""
		case strings.HasSuffix(path, ".go"):
			content = "package fixture\n"
		case strings.HasSuffix(path, ".sql"):
			content = "SELECT 1;\n"
		case strings.HasSuffix(path, ".yml"):
			content = "services: {}\n"
		case strings.HasSuffix(path, ".md"):
			content = "# Fixture\n"
		}
		writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, path, content, 0o600)
	}
	fixtureScript := filepath.Join(fixtureRepo, "deploy", "acceptance", "session-revocation-smoke.sh")
	writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo,
		"deploy/acceptance/session-revocation-smoke.sh", string(scriptBytes), 0o700)
	runIdempotencyAcceptanceGit(t, fixtureRepo, "init", "-q")
	runIdempotencyAcceptanceGit(t, fixtureRepo, "add", "--", ".")
	runIdempotencyAcceptanceGit(t, fixtureRepo,
		"-c", "user.name=Acceptance Contract",
		"-c", "user.email=acceptance-contract@example.invalid",
		"commit", "-q", "-m", "fixture")
	return fixtureRepo, fixtureScript
}

func sessionRevocationAcceptanceRepoDir(t *testing.T) string {
	t.Helper()
	repoDir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repository directory: %v", err)
	}
	return repoDir
}

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
