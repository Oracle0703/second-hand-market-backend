package tests

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
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

		headCheck := exec.Command("git", "cat-file", "-e", "HEAD:"+path)
		headCheck.Dir = repoDir
		if output, err := headCheck.CombinedOutput(); err != nil {
			t.Fatalf("source-list path %q is not in HEAD: %v: %s", path, err, output)
		}
	}

	required := []string{
		"Makefile",
		"backend/Dockerfile",
		"backend/go.mod",
		"backend/go.sum",
		"backend/internal/app/idempotency.go",
		"backend/tests/idempotency_acceptance_contract_test.go",
		"backend/tests/idempotency_mysql_test.go",
		"backend/migrations/0001_init.up.sql",
		"backend/migrations/0009_buyer_intent_open_uniqueness.up.sql",
		"deploy/acceptance/README.md",
		"deploy/acceptance/docker-compose.yml",
		"deploy/acceptance/idempotency-atomicity-smoke.sh",
	}
	for _, path := range required {
		if !present[path] {
			t.Errorf("source-list mode omitted required committed path %q", path)
		}
	}

	t.Run("excludes staged-only path", func(t *testing.T) {
		fixtureRepo, fixtureScript := newIdempotencyAcceptanceFixtureRepo(t)
		stagedPath := "backend/staged_only.go"
		writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, stagedPath, "package backend\n", 0o600)
		runIdempotencyAcceptanceGit(t, fixtureRepo, "add", "--", stagedPath)

		cmd := exec.Command("/bin/bash", fixtureScript)
		cmd.Dir = fixtureRepo
		cmd.Env = []string{
			"IDEMPOTENCY_SOURCE_LIST_ONLY=1",
			"PATH=" + os.Getenv("PATH"),
		}
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("run fixture source-list mode: %v", err)
		}
		for _, path := range splitNULPaths(t, output) {
			if path == stagedPath {
				t.Fatalf("source-list mode included staged-only path %q", stagedPath)
			}
		}
	})
}

func TestIdempotencyAcceptanceSourceExportUsesImmutableHEAD(t *testing.T) {
	fixtureRepo, fixtureScript := newIdempotencyAcceptanceFixtureRepo(t)
	writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, "Makefile", "fixture:\n\t@false\n", 0o600)
	stagedPath := "backend/staged_only.go"
	writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, stagedPath, "package backend\n", 0o600)
	runIdempotencyAcceptanceGit(t, fixtureRepo, "add", "--", stagedPath)

	exportDir := filepath.Join(t.TempDir(), "source-package")
	cmd := exec.Command("/bin/bash", fixtureScript)
	cmd.Dir = fixtureRepo
	cmd.Env = []string{
		"IDEMPOTENCY_SOURCE_EXPORT_DIR=" + exportDir,
		"PATH=" + os.Getenv("PATH"),
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export immutable HEAD package: %v: %s", err, output)
	}

	for _, name := range []string{"source-files.z", "source-sha256.txt", "source.tar", "package-sha256.txt"} {
		if info, err := os.Lstat(filepath.Join(exportDir, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("source export artifact %q must be a regular file: %v", name, err)
		}
	}
	rawList, err := os.ReadFile(filepath.Join(exportDir, "source-files.z"))
	if err != nil {
		t.Fatalf("read exported source list: %v", err)
	}
	for _, path := range splitNULPaths(t, rawList) {
		if path == stagedPath {
			t.Fatalf("source export included staged-only path %q", stagedPath)
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
	manifestCheck := exec.Command("sha256sum", "-c", filepath.Join(exportDir, "source-sha256.txt"))
	manifestCheck.Dir = extracted
	if output, err := manifestCheck.CombinedOutput(); err != nil {
		t.Fatalf("verify exported source manifest: %v: %s", err, output)
	}
}

func TestIdempotencyAcceptanceMetadataFreePackageRefusesOrProgressesBeforeDocker(t *testing.T) {
	t.Run("valid package reaches Docker without Git metadata", func(t *testing.T) {
		remoteRepo, packageDir, remoteScript := prepareMetadataFreeIdempotencyAcceptance(t)
		dockerMarker := filepath.Join(t.TempDir(), "docker-called")
		stubDir := writeIdempotencyAcceptanceDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		output, err := runMetadataFreeIdempotencyAcceptance(t, remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
		if err == nil {
			t.Fatal("fake Docker tripwire unexpectedly allowed acceptance to succeed")
		}
		if _, err := os.Stat(dockerMarker); err != nil {
			t.Fatalf("valid metadata-free package did not progress to Docker: %v; output = %q", err, output)
		}
	})

	t.Run("tampered received source refuses before Docker", func(t *testing.T) {
		remoteRepo, packageDir, remoteScript := prepareMetadataFreeIdempotencyAcceptance(t)
		writeIdempotencyAcceptanceFixtureFile(t, remoteRepo, "Makefile", "fixture:\n\t@false\n", 0o600)
		dockerMarker := filepath.Join(t.TempDir(), "docker-called")
		stubDir := writeIdempotencyAcceptanceDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		output, err := runMetadataFreeIdempotencyAcceptance(t, remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
		if err == nil {
			t.Fatal("tampered metadata-free package unexpectedly succeeded")
		}
		if !strings.Contains(string(output), "received idempotency source does not match package manifest") {
			t.Fatalf("tampered package missing stable refusal; output = %q", output)
		}
		if _, err := os.Stat(dockerMarker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("tampered metadata-free package reached Docker")
		}
	})

	t.Run("wrong authorized package digest refuses before Docker", func(t *testing.T) {
		remoteRepo, packageDir, remoteScript := prepareMetadataFreeIdempotencyAcceptance(t)
		dockerMarker := filepath.Join(t.TempDir(), "docker-called")
		stubDir := writeIdempotencyAcceptanceDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		output, err := runMetadataFreeIdempotencyAcceptanceWithDigest(t,
			remoteRepo, packageDir, remoteScript, stubDir, dockerMarker, strings.Repeat("0", 64))
		if err == nil {
			t.Fatal("wrong authorized package digest unexpectedly succeeded")
		}
		if !strings.Contains(string(output), "source package manifest digest does not match authorization") {
			t.Fatalf("wrong package digest missing stable refusal; output = %q", output)
		}
		if _, err := os.Stat(dockerMarker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("wrong authorized package digest reached Docker")
		}
	})
}

func TestIdempotencyAcceptanceRetainsSanitizedFailureEvidence(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeIdempotencyAcceptance(t)
	dockerMarker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := writeIdempotencyAcceptanceDockerStub(t, `#!/bin/sh
: >>"$DOCKER_CALLED"
args=" $* "
case "$args" in
  *" container ls "*|*" volume ls "*|*" network ls "*)
    exit 0
    ;;
  *" compose "*" stop "*|*" compose "*" up "*|*" compose "*" build "*)
    exit 0
    ;;
  *" compose "*" exec "*)
    case "$args" in
      *"SELECT VERSION()"*) printf '8.4.0\n' ;;
    esac
    exit 0
    ;;
  *" compose "*" run "*"git init"*)
    exit 0
    ;;
  *" compose "*" run "*)
    printf 'Authorization: Bearer raw-test-identifier\n' >&2
    exit 42
    ;;
esac
exit 0
`)
	output, err := runMetadataFreeIdempotencyAcceptance(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
	if err == nil {
		t.Fatal("controlled focused-test failure unexpectedly succeeded")
	}
	if _, err := os.Stat(dockerMarker); err != nil {
		t.Fatalf("controlled failure did not exercise fake Docker: %v", err)
	}

	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "idempotency-atomicity")
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		t.Fatalf("read retained failure evidence: %v; command output = %q", err, output)
	}
	allowed := map[string]bool{
		"acceptance-results.txt": true,
		"evidence-leak-scan.txt": true,
		"evidence-sha256.txt":    true,
		"failure-status.txt":     true,
		"production-before.txt":  true,
	}
	var retained bytes.Buffer
	for _, entry := range entries {
		if entry.IsDir() || !allowed[entry.Name()] {
			t.Fatalf("retained failure evidence contains unexpected entry %q", entry.Name())
		}
		raw, err := os.ReadFile(filepath.Join(evidenceDir, entry.Name()))
		if err != nil {
			t.Fatalf("read retained failure evidence %q: %v", entry.Name(), err)
		}
		retained.Write(raw)
	}
	retainedText := retained.String()
	for _, required := range []string{
		"classification=acceptance_failure|result=FAIL|stage=mysql_auto_migrate_false|count=1",
		"classification=source_manifest|result=PASS|count=",
		"classification=evidence_scan|result=PASS|count=0",
	} {
		if !strings.Contains(retainedText, required) {
			t.Fatalf("retained failure evidence omitted %q", required)
		}
	}
	for _, forbidden := range []string{"Authorization", "Bearer", "raw-test-identifier", "TestIdempotency"} {
		if strings.Contains(retainedText, forbidden) {
			t.Fatalf("retained failure evidence leaked %q", forbidden)
		}
	}
	wantSnapshot := strings.Join([]string{
		"/secondhand-market-api|absent|absent|absent",
		"/secondhand-market-web|absent|absent|absent",
		"/secondhand-market-mysql|absent|absent|absent",
		"",
	}, "\n")
	gotSnapshot, err := os.ReadFile(filepath.Join(evidenceDir, "production-before.txt"))
	if err != nil {
		t.Fatalf("read authorized failure snapshot: %v", err)
	}
	if string(gotSnapshot) != wantSnapshot {
		t.Fatalf("authorized failure snapshot = %q, want fixed absent snapshot", gotSnapshot)
	}
	hashCheck := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
	hashCheck.Dir = evidenceDir
	if output, err := hashCheck.CombinedOutput(); err != nil {
		t.Fatalf("verify retained failure evidence hashes: %v: %s", err, output)
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

}

func TestIdempotencyAcceptanceMakeTargetRefusesBeforeDockerWithoutConfirmation(t *testing.T) {
	repoDir := idempotencyAcceptanceRepoDir(t)
	assertIdempotencyAcceptanceRefusesBeforeDocker(t, repoDir, idempotencyAcceptanceMissingConfirmation, nil,
		"make", "acceptance-idempotency-smoke")
}

func newIdempotencyAcceptanceFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	fixtureRepo := t.TempDir()
	realScript := filepath.Join(idempotencyAcceptanceRepoDir(t), "deploy", "acceptance", "idempotency-atomicity-smoke.sh")
	scriptBytes, err := os.ReadFile(realScript)
	if err != nil {
		t.Fatalf("read real acceptance script: %v", err)
	}
	fixtureFiles := map[string]string{
		"Makefile":                            "fixture:\n\t@true\n",
		"backend/Dockerfile":                  "FROM scratch\n",
		"backend/go.mod":                      "module fixture.invalid/idempotency\n\ngo 1.22\n",
		"backend/go.sum":                      "",
		"backend/internal/app/idempotency.go": "package app\n",
		"backend/tests/idempotency_acceptance_contract_test.go":       "package tests\n",
		"backend/tests/idempotency_mysql_test.go":                     "package tests\n",
		"backend/migrations/0001_init.up.sql":                         "SELECT 1;\n",
		"backend/migrations/0009_buyer_intent_open_uniqueness.up.sql": "SELECT 1;\n",
		"deploy/acceptance/README.md":                                 "# Fixture\n",
		"deploy/acceptance/docker-compose.yml":                        "services: {}\n",
	}
	for path, content := range fixtureFiles {
		writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo, path, content, 0o600)
	}
	fixtureScript := filepath.Join(fixtureRepo, "deploy", "acceptance", "idempotency-atomicity-smoke.sh")
	writeIdempotencyAcceptanceFixtureFile(t, fixtureRepo,
		"deploy/acceptance/idempotency-atomicity-smoke.sh", string(scriptBytes), 0o700)
	runIdempotencyAcceptanceGit(t, fixtureRepo, "init", "-q")
	runIdempotencyAcceptanceGit(t, fixtureRepo, "add", "--", ".")
	runIdempotencyAcceptanceGit(t, fixtureRepo,
		"-c", "user.name=Acceptance Contract",
		"-c", "user.email=acceptance-contract@example.invalid",
		"commit", "-q", "-m", "fixture")
	return fixtureRepo, fixtureScript
}

func writeIdempotencyAcceptanceFixtureFile(
	t *testing.T,
	repoDir string,
	path string,
	content string,
	mode os.FileMode,
) {
	t.Helper()
	fullPath := filepath.Join(repoDir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatalf("create fixture parent for %q: %v", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), mode); err != nil {
		t.Fatalf("write fixture file %q: %v", path, err)
	}
}

func prepareMetadataFreeIdempotencyAcceptance(t *testing.T) (string, string, string) {
	t.Helper()
	fixtureRepo, fixtureScript := newIdempotencyAcceptanceFixtureRepo(t)
	remoteRepo := t.TempDir()
	packageDir := filepath.Join(remoteRepo, ".idempotency-source")
	exportCmd := exec.Command("/bin/bash", fixtureScript)
	exportCmd.Dir = fixtureRepo
	exportCmd.Env = []string{
		"IDEMPOTENCY_SOURCE_EXPORT_DIR=" + packageDir,
		"PATH=" + os.Getenv("PATH"),
	}
	if output, err := exportCmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare metadata-free source package: %v: %s", err, output)
	}
	extractIdempotencyAcceptanceTar(t, filepath.Join(packageDir, "source.tar"), remoteRepo)
	writeIdempotencyAcceptanceFixtureFile(t, remoteRepo, "deploy/acceptance/.env",
		"MYSQL_DATABASE=second_hand_market_acceptance\n", 0o600)
	if _, err := os.Stat(filepath.Join(remoteRepo, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("metadata-free fixture unexpectedly contains Git metadata")
	}
	return remoteRepo, packageDir,
		filepath.Join(remoteRepo, "deploy", "acceptance", "idempotency-atomicity-smoke.sh")
}

func extractIdempotencyAcceptanceTar(t *testing.T, archivePath string, destination string) {
	t.Helper()
	archive, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open source archive: %v", err)
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("read source archive: %v", err)
		}
		clean := filepath.Clean(filepath.FromSlash(header.Name))
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			t.Fatalf("source archive contains unsafe path %q", header.Name)
		}
		target := filepath.Join(destination, clean)
		switch header.Typeflag {
		case tar.TypeXHeader, tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatalf("create source archive directory: %v", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatalf("create source archive parent: %v", err)
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode)&0o777)
			if err != nil {
				t.Fatalf("create extracted source file %q: %v", header.Name, err)
			}
			if _, err := io.Copy(file, reader); err != nil {
				_ = file.Close()
				t.Fatalf("extract source file %q: %v", header.Name, err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("close extracted source file %q: %v", header.Name, err)
			}
		default:
			t.Fatalf("source archive contains unsupported entry %q", header.Name)
		}
	}
}

func writeIdempotencyAcceptanceDockerStub(t *testing.T, content string) string {
	t.Helper()
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "docker"), []byte(content), 0o700); err != nil {
		t.Fatalf("write Docker stub: %v", err)
	}
	return stubDir
}

func runMetadataFreeIdempotencyAcceptance(
	t *testing.T,
	remoteRepo string,
	packageDir string,
	remoteScript string,
	stubDir string,
	dockerMarker string,
) ([]byte, error) {
	t.Helper()
	manifest, err := os.ReadFile(filepath.Join(packageDir, "package-sha256.txt"))
	if err != nil {
		t.Fatalf("read package manifest for authorization digest: %v", err)
	}
	digest := sha256.Sum256(manifest)
	return runMetadataFreeIdempotencyAcceptanceWithDigest(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker, hex.EncodeToString(digest[:]))
}

func runMetadataFreeIdempotencyAcceptanceWithDigest(
	t *testing.T,
	remoteRepo string,
	packageDir string,
	remoteScript string,
	stubDir string,
	dockerMarker string,
	manifestDigest string,
) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("/bin/bash", remoteScript)
	cmd.Dir = remoteRepo
	cmd.Env = []string{
		"IDEMPOTENCY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_IDEMPOTENCY_DATA",
		"ACCEPTANCE_DB_ENGINE=mysql8.4",
		"COMPOSE_PROJECT_NAME=secondhand-idempotency-acceptance",
		"IDEMPOTENCY_SOURCE_PACKAGE_DIR=" + packageDir,
		"IDEMPOTENCY_SOURCE_PACKAGE_MANIFEST_SHA256=" + manifestDigest,
		"DOCKER_CALLED=" + dockerMarker,
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}
	return cmd.CombinedOutput()
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
