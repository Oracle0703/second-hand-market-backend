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

const idempotencyAcceptanceMissingConfirmation = "set IDEMPOTENCY_ACCEPTANCE_CONFIRM for isolated idempotency tests"

func TestIdempotencyAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T) {
	repoDir := idempotencyAcceptanceRepoDir(t)
	script := filepath.Join(repoDir, "deploy/acceptance/idempotency-atomicity-smoke.sh")
	cmd := exec.Command("/bin/bash", script)
	cmd.Dir = repoDir
	cmd.Env = []string{
		"IDEMPOTENCY_SOURCE_LIST_ONLY=1",
		"PATH=/usr/bin:/bin",
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run idempotency acceptance source-list mode: %v: %s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("source-list mode wrote stderr: %q", stderr.String())
	}

	paths := splitNULPaths(t, stdout.Bytes())
	present := make(map[string]bool, len(paths))
	for _, path := range paths {
		if present[path] {
			t.Fatalf("source-list mode emitted duplicate path %q", path)
		}
		present[path] = true
		if forbiddenIdempotencyAcceptancePath(path) {
			t.Fatalf("source-list mode emitted forbidden path %q", path)
		}

		indexCheck := exec.Command("git", "ls-files", "--error-unmatch", "--", path)
		indexCheck.Dir = repoDir
		if output, err := indexCheck.CombinedOutput(); err != nil {
			t.Fatalf("source-list path %q is not committed: %v: %s", path, err, output)
		}
	}

	required := []string{
		"Makefile",
		"backend/Dockerfile",
		"backend/go.mod",
		"backend/go.sum",
		"backend/internal/app/idempotency.go",
		"backend/tests/idempotency_mysql_test.go",
		"backend/migrations/0001_init.up.sql",
		"backend/migrations/0009_buyer_intent_open_uniqueness.up.sql",
		"deploy/acceptance/README.md",
		"deploy/acceptance/docker-compose.yml",
	}
	for _, path := range required {
		if !present[path] {
			t.Errorf("source-list mode omitted required committed path %q", path)
		}
	}
}

func TestIdempotencyAcceptanceRefusesBeforeDockerWithoutConfirmation(t *testing.T) {
	repoDir := idempotencyAcceptanceRepoDir(t)
	script := filepath.Join(repoDir, "deploy/acceptance/idempotency-atomicity-smoke.sh")
	gateCases := []struct {
		name            string
		expectedMessage string
		env             []string
	}{
		{
			name:            "missing confirmation",
			expectedMessage: idempotencyAcceptanceMissingConfirmation,
		},
		{
			name:            "wrong engine",
			expectedMessage: "set ACCEPTANCE_DB_ENGINE=mysql8.4",
			env: []string{
				"IDEMPOTENCY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_IDEMPOTENCY_DATA",
				"ACCEPTANCE_DB_ENGINE=mysql8.0",
				"COMPOSE_PROJECT_NAME=secondhand-idempotency-acceptance",
			},
		},
		{
			name:            "missing project",
			expectedMessage: "COMPOSE_PROJECT_NAME must be secondhand-idempotency-acceptance",
			env: []string{
				"IDEMPOTENCY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_IDEMPOTENCY_DATA",
				"ACCEPTANCE_DB_ENGINE=mysql8.4",
			},
		},
		{
			name:            "wrong project",
			expectedMessage: "COMPOSE_PROJECT_NAME must be secondhand-idempotency-acceptance",
			env: []string{
				"IDEMPOTENCY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_IDEMPOTENCY_DATA",
				"ACCEPTANCE_DB_ENGINE=mysql8.4",
				"COMPOSE_PROJECT_NAME=secondhand-market",
			},
		},
	}
	for _, tc := range gateCases {
		t.Run(tc.name, func(t *testing.T) {
			assertIdempotencyAcceptanceRefusesBeforeDocker(t, repoDir, tc.expectedMessage, tc.env,
				"/bin/bash", script)
		})
	}

	t.Run("dirty committed source", func(t *testing.T) {
		fixtureRepo := t.TempDir()
		fixtureAcceptanceDir := filepath.Join(fixtureRepo, "deploy", "acceptance")
		if err := os.MkdirAll(fixtureAcceptanceDir, 0o700); err != nil {
			t.Fatalf("create acceptance fixture directory: %v", err)
		}
		fixtureScript := filepath.Join(fixtureAcceptanceDir, "idempotency-atomicity-smoke.sh")
		if err := os.Symlink(script, fixtureScript); err != nil {
			t.Fatalf("link real acceptance script into fixture: %v", err)
		}
		fixtureMakefile := filepath.Join(fixtureRepo, "Makefile")
		if err := os.WriteFile(fixtureMakefile, []byte("fixture:\n\t@true\n"), 0o600); err != nil {
			t.Fatalf("write fixture Makefile: %v", err)
		}
		runIdempotencyAcceptanceGit(t, fixtureRepo, "init", "-q")
		runIdempotencyAcceptanceGit(t, fixtureRepo, "add", "--", "Makefile", "deploy/acceptance/idempotency-atomicity-smoke.sh")
		runIdempotencyAcceptanceGit(t, fixtureRepo,
			"-c", "user.name=Acceptance Contract",
			"-c", "user.email=acceptance-contract@example.invalid",
			"commit", "-q", "-m", "fixture")
		if err := os.WriteFile(fixtureMakefile, []byte("fixture:\n\t@false\n"), 0o600); err != nil {
			t.Fatalf("dirty fixture Makefile: %v", err)
		}

		assertIdempotencyAcceptanceRefusesBeforeDocker(t, fixtureRepo,
			"committed idempotency source must match HEAD",
			[]string{
				"IDEMPOTENCY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_IDEMPOTENCY_DATA",
				"ACCEPTANCE_DB_ENGINE=mysql8.4",
				"COMPOSE_PROJECT_NAME=secondhand-idempotency-acceptance",
			},
			"/bin/bash", fixtureScript)
	})
}

func TestIdempotencyAcceptanceMakeTargetRefusesBeforeDockerWithoutConfirmation(t *testing.T) {
	repoDir := idempotencyAcceptanceRepoDir(t)
	assertIdempotencyAcceptanceRefusesBeforeDocker(t, repoDir, idempotencyAcceptanceMissingConfirmation, nil,
		"make", "acceptance-idempotency-smoke")
}

func idempotencyAcceptanceRepoDir(t *testing.T) string {
	t.Helper()
	repoDir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repository directory: %v", err)
	}
	return repoDir
}

func assertIdempotencyAcceptanceRefusesBeforeDocker(
	t *testing.T,
	repoDir string,
	expectedMessage string,
	extraEnv []string,
	name string,
	args ...string,
) {
	t.Helper()
	stubDir := t.TempDir()
	dockerMarker := filepath.Join(stubDir, "docker-called")
	dockerStub := filepath.Join(stubDir, "docker")
	if err := os.WriteFile(dockerStub, []byte("#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n"), 0o700); err != nil {
		t.Fatalf("write Docker tripwire: %v", err)
	}

	cmd := exec.Command(name, args...)
	cmd.Dir = repoDir
	cmd.Env = append([]string{
		"DOCKER_CALLED=" + dockerMarker,
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}, extraEnv...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("acceptance command succeeded without confirmation")
	}
	if !strings.Contains(string(output), expectedMessage) {
		t.Fatalf("missing stable refusal %q; output = %q", expectedMessage, output)
	}
	if _, err := os.Stat(dockerMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("acceptance command reached Docker before refusing missing confirmation")
	}
}

func runIdempotencyAcceptanceGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func splitNULPaths(t *testing.T, raw []byte) []string {
	t.Helper()
	if len(raw) == 0 || raw[len(raw)-1] != 0 {
		t.Fatal("source-list output must be a non-empty NUL-delimited list")
	}
	parts := bytes.Split(raw[:len(raw)-1], []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		path := string(part)
		if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.HasPrefix(path, "../") {
			t.Fatalf("source-list mode emitted unsafe path %q", path)
		}
		paths = append(paths, path)
	}
	return paths
}

func forbiddenIdempotencyAcceptancePath(path string) bool {
	if path == "backend/app.db" || strings.HasPrefix(path, "docs/superpowers/") {
		return true
	}
	lower := strings.ToLower(path)
	for _, suffix := range []string{".db", ".sqlite", ".sqlite3"} {
		if strings.HasSuffix(lower, suffix) || strings.Contains(lower, suffix+".") {
			return true
		}
	}
	for _, component := range strings.Split(lower, "/") {
		switch component {
		case ".env", ".git", ".tmp", ".cache", "cache", "caches", "secret", "secrets",
			"database", "databases", "upload", "uploads", "evidence", "backup", "backups", "node_modules":
			return true
		}
		if strings.HasPrefix(component, ".env.") {
			return true
		}
	}
	return false
}
