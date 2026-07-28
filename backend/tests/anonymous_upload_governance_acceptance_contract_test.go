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

func TestAnonymousUploadGovernanceAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T) {
	fixtureRepo, fixtureScript := newAnonymousUploadGovernanceFixtureRepo(t)
	stagedPath := "backend/staged_only.go"
	writeAnonymousUploadGovernanceFixtureFile(t, fixtureRepo, stagedPath, "package backend\n", 0o600)
	runAnonymousUploadGovernanceGit(t, fixtureRepo, "add", "--", stagedPath)
	writeAnonymousUploadGovernanceFixtureFile(t, fixtureRepo, "frontend/src/dirty.ts", "export const dirty = true\n", 0o600)

	cmd := exec.Command("/bin/bash", fixtureScript)
	cmd.Dir = fixtureRepo
	cmd.Env = []string{
		"ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_LIST_ONLY=1",
		"PATH=" + os.Getenv("PATH"),
	}
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("run source-list mode: %v", err)
	}

	present := make(map[string]bool)
	for _, path := range splitAnonymousUploadGovernanceNULPaths(t, stdout) {
		if present[path] {
			t.Fatalf("source-list emitted duplicate %q", path)
		}
		present[path] = true
		if forbiddenAnonymousUploadGovernanceSourcePath(path) {
			t.Fatalf("source-list emitted forbidden path %q", path)
		}
		check := exec.Command("git", "cat-file", "-e", "HEAD:"+path)
		check.Dir = fixtureRepo
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("source-list path %q is not committed HEAD content: %v: %s", path, err, output)
		}
	}
	for _, required := range anonymousUploadGovernanceRequiredSourcePaths() {
		if !present[required] {
			t.Errorf("source-list omitted required path %q", required)
		}
	}
	if present[stagedPath] || present["frontend/src/dirty.ts"] {
		t.Fatal("source-list accepted staged or dirty working-tree bytes")
	}
}

func TestAnonymousUploadGovernanceAcceptanceSourceExportUsesImmutableHEAD(t *testing.T) {
	fixtureRepo, fixtureScript := newAnonymousUploadGovernanceFixtureRepo(t)
	writeAnonymousUploadGovernanceFixtureFile(t, fixtureRepo, "Makefile", "fixture:\n\t@false\n", 0o600)
	writeAnonymousUploadGovernanceFixtureFile(t, fixtureRepo, "backend/staged_only.go", "package backend\n", 0o600)
	runAnonymousUploadGovernanceGit(t, fixtureRepo, "add", "--", "backend/staged_only.go")

	exportDir := filepath.Join(t.TempDir(), "source-package")
	cmd := exec.Command("/bin/bash", fixtureScript)
	cmd.Dir = fixtureRepo
	cmd.Env = []string{
		"ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR=" + exportDir,
		"PATH=" + os.Getenv("PATH"),
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export source package: %v: %s", err, output)
	}
	for _, artifact := range []string{"source-files.z", "source-sha256.txt", "source.tar", "package-sha256.txt"} {
		if info, err := os.Lstat(filepath.Join(exportDir, artifact)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("export artifact %q must be a regular file: %v", artifact, err)
		}
	}
	extracted := t.TempDir()
	extractAnonymousUploadGovernanceTar(t, filepath.Join(exportDir, "source.tar"), extracted)
	got, err := os.ReadFile(filepath.Join(extracted, "Makefile"))
	if err != nil {
		t.Fatalf("read exported Makefile: %v", err)
	}
	if string(got) != "fixture:\n\t@true\n" {
		t.Fatalf("exported Makefile = %q, want committed HEAD bytes", got)
	}
	if _, err := os.Stat(filepath.Join(extracted, "backend", "staged_only.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("export included a staged-only source file")
	}
	check := exec.Command("sha256sum", "-c", filepath.Join(exportDir, "source-sha256.txt"))
	check.Dir = extracted
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("verify export manifest: %v: %s", err, output)
	}
}

func TestAnonymousUploadGovernanceAcceptanceMetadataFreePackageRefusesOrProgressesBeforeDocker(t *testing.T) {
	t.Run("valid package reaches Docker without Git metadata", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		_, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, "")
		if err == nil {
			t.Fatal("Docker tripwire unexpectedly allowed acceptance to succeed")
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("valid metadata-free package did not reach Docker: %v", err)
		}
	})

	for _, tamper := range []struct {
		name  string
		apply func(t *testing.T, remoteRepo, packageDir string)
	}{
		{"package manifest", func(t *testing.T, _, packageDir string) {
			writeAnonymousUploadGovernanceFixtureFile(t, packageDir, "package-sha256.txt", "0 bad\n", 0o600)
		}},
		{"source list", func(t *testing.T, _, packageDir string) {
			writeAnonymousUploadGovernanceFixtureFile(t, packageDir, "source-files.z", "Makefile\x00backend/app.db\x00", 0o600)
		}},
		{"archive", func(t *testing.T, _, packageDir string) {
			writeAnonymousUploadGovernanceFixtureFile(t, packageDir, "source.tar", "not a tar archive", 0o600)
		}},
		{"received file", func(t *testing.T, remoteRepo, _ string) {
			writeAnonymousUploadGovernanceFixtureFile(t, remoteRepo, "Makefile", "fixture:\n\t@false\n", 0o600)
		}},
	} {
		t.Run("tampered "+tamper.name+" refuses before Docker", func(t *testing.T) {
			remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
			tamper.apply(t, remoteRepo, packageDir)
			marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
			if _, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, ""); err == nil {
				t.Fatal("tampered package unexpectedly succeeded")
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("tampered %s reached Docker", tamper.name)
			}
		})
	}
}

func TestAnonymousUploadGovernanceAcceptanceRefusesEvidenceAndProjectReuse(t *testing.T) {
	t.Run("evidence directory", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
		if err := os.MkdirAll(evidenceDir, 0o700); err != nil {
			t.Fatal(err)
		}
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, "")
		if err == nil || !strings.Contains(string(output), "refusing to overwrite existing anonymous upload governance evidence") {
			t.Fatalf("evidence reuse refusal = %q, err = %v", output, err)
		}
	})

	for _, resource := range []string{"container", "volume", "network"} {
		t.Run(resource+" reuse", func(t *testing.T) {
			remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
			marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nif [ \"$1\" = \""+resource+"\" ]; then printf 'occupied\\n'; fi\nexit 0\n")
			output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, "")
			if err == nil || !strings.Contains(string(output), "refusing to reuse existing secondhand-upload-governance-acceptance resources") {
				t.Fatalf("%s reuse refusal = %q, err = %v", resource, output, err)
			}
		})
	}
}

func TestAnonymousUploadGovernanceAcceptanceRetainsSanitizedFailureEvidence(t *testing.T) {
	remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
	marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, `#!/bin/sh
: >>"$DOCKER_CALLED"
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" compose "*) printf 'Authorization: Bearer injected-secret object_key=unsafe /var/lib/second-hand-market/uploads\n' >&2; exit 42 ;;
esac
exit 0
`)
	if _, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, ""); err == nil {
		t.Fatal("controlled Docker failure unexpectedly succeeded")
	}
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		t.Fatalf("read retained evidence: %v", err)
	}
	var retained bytes.Buffer
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("retained evidence contains directory %q", entry.Name())
		}
		raw, err := os.ReadFile(filepath.Join(evidenceDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		retained.Write(raw)
	}
	for _, forbidden := range []string{"Authorization", "Bearer", "injected-secret", "object_key", "/var/lib/second-hand-market/uploads"} {
		if strings.Contains(retained.String(), forbidden) {
			t.Fatalf("retained evidence leaked %q", forbidden)
		}
	}
	check := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
	check.Dir = evidenceDir
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("verify retained evidence hashes: %v: %s", err, output)
	}
}

func TestAnonymousUploadGovernanceAcceptancePreservesUploadBoundaryMatrix(t *testing.T) {
	remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
	marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, anonymousUploadGovernanceHappyDockerStub)
	writeAnonymousUploadGovernanceFixtureFile(t, stubDir, "curl", anonymousUploadGovernanceCurlStub, 0o700)
	writeAnonymousUploadGovernanceFixtureFile(t, stubDir, "jq", "#!/bin/sh\ncase \"$1\" in -er) printf 'fixture\\n' ;; esac\nexit 0\n", 0o700)
	output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, "")
	if err != nil {
		t.Fatalf("controlled F-06 acceptance run failed: %v: %s", err, output)
	}
	evidence, err := os.ReadFile(filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance", "acceptance-results.txt"))
	if err != nil {
		t.Fatalf("read F-06 acceptance results: %v", err)
	}
	if !strings.Contains(string(evidence), "classification=upload_boundaries|result=PASS|count=7\n") {
		t.Fatalf("published results lost the seven upload boundary outcomes: %q", evidence)
	}
}

const anonymousUploadGovernanceHappyDockerStub = `#!/bin/sh
: >>"$DOCKER_CALLED"
mkdir -p "$DOCKER_STATE"
if [ "$1" = "container" ] || [ "$1" = "volume" ] || [ "$1" = "network" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then
  case "$*" in
    *secondhand-market-api*) name=secondhand-market-api ;;
    *secondhand-market-web*) name=secondhand-market-web ;;
    *) name=secondhand-market-mysql ;;
  esac
  printf '/%s|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|running|0\n' "$name"
  exit 0
fi
if [ "$1" != "compose" ]; then exit 0; fi
case " $* " in
  *" exec "*)
    case " $* " in
      *"SELECT VERSION()"*) printf '8.4.0\n' ;;
      *"0008_anonymous_upload_governance.preflight.sql"*)
        n=0; [ -f "$DOCKER_STATE/0008" ] && n=$(cat "$DOCKER_STATE/0008")
        n=$((n + 1)); printf '%s' "$n" >"$DOCKER_STATE/0008"
        case "$n" in
          1) printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: 0007 merchant license URL remains public' >&2; exit 1 ;;
          2) printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: partial 0008 schema exists' >&2; exit 1 ;;
          4) printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: 0008 columns are drifted' >&2; exit 1 ;;
          6) printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: fixed quota guard row is missing or drifted' >&2; exit 1 ;;
          8) printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: quota guard table must use InnoDB' >&2; exit 1 ;;
        esac ;;
      *"COUNT(*)"*) printf '0\n' ;;
      *"SELECT CONCAT"*) printf 'historical\n' ;;
      *"postflight.sql"*) printf 'anonymous_upload_governance_preflight_passed\nanonymous_upload_governance_migration_applied\nanonymous_upload_governance_postflight_passed\n' ;;
    esac
    exit 0 ;;
  *" run "*)
    case " $* " in
      *"sha256sum /var/lib"*) printf 'license=fixture\nproduct=fixture\n' ;;
      *"TestUploadGovernanceMySQLConcurrencyAndCleanup"*) printf '%s\n' '--- PASS: TestUploadGovernanceMySQLConcurrencyAndCleanup' ;;
    esac
    exit 0 ;;
esac
exit 0
`

const anonymousUploadGovernanceCurlStub = `#!/bin/sh
output=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) output=$2; shift 2 ;;
    *) shift ;;
  esac
done
case "$output" in
  *presign-exact*) status=200; body='{"code":0,"data":{"file_id":"fixture","object_key":"fixture","file_token":"fixture"}}' ;;
  *upload-exact*) status=200; body='{"code":0}' ;;
  *presign-over*) status=400; body='{"code":10008}' ;;
  *exact_11*) status=400; body='{"code":10001,"request_id":"fixture"}' ;;
  *over_11*) status=413; body='{"code":10008,"request_id":"fixture"}' ;;
  *) status=500; body='{"code":1}' ;;
esac
printf '%s' "$body" >"$output"
printf '%s' "$status"
`

func anonymousUploadGovernanceRequiredSourcePaths() []string {
	return []string{
		"Makefile", "backend/Dockerfile", "backend/go.mod", "backend/go.sum",
		"backend/internal/app/upload_governance.go", "backend/internal/app/upload_governance_mysql_test.go",
		"backend/tests/anonymous_upload_governance_acceptance_contract_test.go",
		"backend/migrations/0008_anonymous_upload_governance.preflight.sql",
		"backend/migrations/0008_anonymous_upload_governance.up.sql",
		"backend/migrations/0008_anonymous_upload_governance.postflight.sql",
		"backend/migrations/anonymous_upload_governance_migration_test.go",
		"frontend/package.json", "frontend/package-lock.json", "frontend/index.html",
		"frontend/tsconfig.json", "frontend/vite.config.ts", "frontend/vitest.config.ts",
		"frontend/src/utils/upload.ts", "frontend/src/utils/upload.test.ts",
		"deploy/acceptance/docker-compose.yml", "deploy/acceptance/frontend.Dockerfile",
		"deploy/acceptance/nginx.conf", "deploy/acceptance/anonymous-upload-governance-smoke.sh",
		"deploy/acceptance/sql/post-smoke.sql", "deploy/acceptance/sql/protected-fingerprint.sql",
	}
}

func newAnonymousUploadGovernanceFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	fixtureRepo := t.TempDir()
	realScript := filepath.Join(anonymousUploadGovernanceRepoDir(t), "deploy", "acceptance", "anonymous-upload-governance-smoke.sh")
	script, err := os.ReadFile(realScript)
	if err != nil {
		t.Fatalf("read smoke script: %v", err)
	}
	for _, path := range anonymousUploadGovernanceRequiredSourcePaths() {
		content := "fixture\n"
		if path == "Makefile" {
			content = "fixture:\n\t@true\n"
		}
		if strings.HasSuffix(path, ".go") {
			content = "package fixture\n"
		}
		writeAnonymousUploadGovernanceFixtureFile(t, fixtureRepo, path, content, 0o600)
	}
	fixtureScript := filepath.Join(fixtureRepo, "deploy", "acceptance", "anonymous-upload-governance-smoke.sh")
	writeAnonymousUploadGovernanceFixtureFile(t, fixtureRepo, "deploy/acceptance/anonymous-upload-governance-smoke.sh", string(script), 0o700)
	runAnonymousUploadGovernanceGit(t, fixtureRepo, "init", "-q")
	runAnonymousUploadGovernanceGit(t, fixtureRepo, "add", "--", ".")
	runAnonymousUploadGovernanceGit(t, fixtureRepo, "-c", "user.name=Acceptance Contract", "-c", "user.email=acceptance-contract@example.invalid", "commit", "-q", "-m", "fixture")
	return fixtureRepo, fixtureScript
}

func writeAnonymousUploadGovernanceFixtureFile(t *testing.T, repoDir, path, content string, mode os.FileMode) {
	t.Helper()
	fullPath := filepath.Join(repoDir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatalf("create parent for %q: %v", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), mode); err != nil {
		t.Fatalf("write fixture %q: %v", path, err)
	}
}

func runAnonymousUploadGovernanceGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func splitAnonymousUploadGovernanceNULPaths(t *testing.T, raw []byte) []string {
	t.Helper()
	if len(raw) == 0 || raw[len(raw)-1] != 0 {
		t.Fatal("source list must be non-empty and NUL-delimited")
	}
	parts := bytes.Split(raw[:len(raw)-1], []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		path := string(part)
		if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.HasPrefix(path, "../") {
			t.Fatalf("unsafe source-list path %q", path)
		}
		paths = append(paths, path)
	}
	return paths
}

func forbiddenAnonymousUploadGovernanceSourcePath(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".db") || strings.Contains(lower, ".db.") {
		return true
	}
	for _, component := range strings.Split(lower, "/") {
		switch component {
		case ".env", ".git", ".tmp", ".cache", "cache", "caches", "secret", "secrets", "database", "databases", "upload", "uploads", "evidence", "backup", "backups", "node_modules":
			return true
		}
		if strings.HasPrefix(component, ".env.") {
			return true
		}
	}
	return false
}

func extractAnonymousUploadGovernanceTar(t *testing.T, archivePath, destination string) {
	t.Helper()
	archive, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		clean := filepath.Clean(filepath.FromSlash(header.Name))
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			t.Fatalf("unsafe archive path %q", header.Name)
		}
		if header.Typeflag == tar.TypeDir || header.Typeflag == tar.TypeXHeader || header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			t.Fatalf("unsupported archive entry %q", header.Name)
		}
		target := filepath.Join(destination, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatalf("create archive parent: %v", err)
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("create archive target: %v", err)
		}
		if _, err := io.Copy(file, reader); err != nil {
			_ = file.Close()
			t.Fatalf("extract archive target: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close archive target: %v", err)
		}
	}
}

func anonymousUploadGovernanceRepoDir(t *testing.T) string {
	t.Helper()
	repoDir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo directory: %v", err)
	}
	return repoDir
}

func prepareAnonymousUploadGovernanceMetadataFreeRepo(t *testing.T) (string, string, string) {
	t.Helper()
	fixtureRepo, fixtureScript := newAnonymousUploadGovernanceFixtureRepo(t)
	remoteRepo := t.TempDir()
	packageDir := filepath.Join(remoteRepo, ".anonymous-upload-governance-source")
	export := exec.Command("/bin/bash", fixtureScript)
	export.Dir = fixtureRepo
	export.Env = []string{
		"ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR=" + packageDir,
		"PATH=" + os.Getenv("PATH"),
	}
	if output, err := export.CombinedOutput(); err != nil {
		t.Fatalf("export fixture package: %v: %s", err, output)
	}
	extractAnonymousUploadGovernanceTar(t, filepath.Join(packageDir, "source.tar"), remoteRepo)
	writeAnonymousUploadGovernanceFixtureFile(t, remoteRepo, "deploy/acceptance/.env", "MYSQL_DATABASE=second_hand_market_acceptance\n", 0o600)
	if _, err := os.Stat(filepath.Join(remoteRepo, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("metadata-free fixture retained Git metadata")
	}
	return remoteRepo, packageDir, filepath.Join(remoteRepo, "deploy", "acceptance", "anonymous-upload-governance-smoke.sh")
}

func anonymousUploadGovernanceDockerTripwire(t *testing.T, content string) (string, string) {
	t.Helper()
	stubDir := t.TempDir()
	marker := filepath.Join(stubDir, "docker-called")
	if err := os.WriteFile(filepath.Join(stubDir, "docker"), []byte(content), 0o700); err != nil {
		t.Fatalf("write Docker tripwire: %v", err)
	}
	return marker, stubDir
}

func runAnonymousUploadGovernanceAcceptance(
	t *testing.T, remoteRepo, packageDir, script, stubDir, marker, expectedDigest string,
) ([]byte, error) {
	t.Helper()
	if expectedDigest == "" {
		manifest, err := os.ReadFile(filepath.Join(packageDir, "package-sha256.txt"))
		if err != nil {
			t.Fatalf("read package manifest: %v", err)
		}
		digest := sha256.Sum256(manifest)
		expectedDigest = hex.EncodeToString(digest[:])
	}
	cmd := exec.Command("/bin/bash", script)
	cmd.Dir = remoteRepo
	cmd.Env = []string{
		"ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA",
		"ACCEPTANCE_DB_ENGINE=mysql8.4",
		"COMPOSE_PROJECT_NAME=secondhand-upload-governance-acceptance",
		"ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_DIR=" + packageDir,
		"ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_MANIFEST_SHA256=" + expectedDigest,
		"DOCKER_CALLED=" + marker,
		"DOCKER_STATE=" + filepath.Join(stubDir, "state"),
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}
	return cmd.CombinedOutput()
}
