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

func TestLicenseFilePrivacyAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T) {
	repoDir := licensePrivacyAcceptanceRepoDir(t)
	cmd := exec.Command("/bin/bash", filepath.Join(repoDir, "deploy/acceptance/license-file-privacy-smoke.sh"))
	cmd.Dir = repoDir
	cmd.Env = []string{"LICENSE_FILE_PRIVACY_SOURCE_LIST_ONLY=1", "PATH=/usr/bin:/bin"}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run source-list mode: %v: %s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("source-list mode wrote stderr: %q", stderr.String())
	}
	present := map[string]bool{}
	for _, path := range licensePrivacySplitNULPaths(t, stdout.Bytes()) {
		if present[path] {
			t.Fatalf("source-list mode emitted duplicate path %q", path)
		}
		present[path] = true
		if licensePrivacyForbiddenPath(path) {
			t.Fatalf("source-list mode emitted forbidden path %q", path)
		}
		check := exec.Command("git", "cat-file", "-e", "HEAD:"+path)
		check.Dir = repoDir
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("source-list path %q is not in HEAD: %v: %s", path, err, output)
		}
	}
	for _, path := range licensePrivacyRequiredPaths() {
		if path == "backend/tests/license_file_privacy_acceptance_contract_test.go" {
			// This uncommitted contract is asserted in the committed fixture below.
			continue
		}
		if !present[path] {
			t.Errorf("source-list mode omitted required committed path %q", path)
		}
	}

	fixtureRepo, fixtureScript := newLicensePrivacyFixtureRepo(t)
	writeLicensePrivacyFixtureFile(t, fixtureRepo, "backend/committed.go", "package backend\nconst FromHEAD = true\n", 0o600)
	runLicensePrivacyGit(t, fixtureRepo, "add", "--", "backend/committed.go")
	runLicensePrivacyGit(t, fixtureRepo, "commit", "-q", "-m", "committed source")
	writeLicensePrivacyFixtureFile(t, fixtureRepo, "backend/committed.go", "package backend\nconst FromHEAD = false\n", 0o600)
	writeLicensePrivacyFixtureFile(t, fixtureRepo, "backend/untracked.go", "package backend\n", 0o600)
	writeLicensePrivacyFixtureFile(t, fixtureRepo, "backend/staged_only.go", "package backend\n", 0o600)
	runLicensePrivacyGit(t, fixtureRepo, "add", "--", "backend/staged_only.go")
	cmd = exec.Command("/bin/bash", fixtureScript)
	cmd.Dir = fixtureRepo
	cmd.Env = []string{"LICENSE_FILE_PRIVACY_SOURCE_LIST_ONLY=1", "PATH=" + os.Getenv("PATH")}
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run fixture source-list mode: %v", err)
	}
	paths := strings.Join(licensePrivacySplitNULPaths(t, output), "\n")
	if !strings.Contains(paths, "backend/committed.go") || strings.Contains(paths, "backend/untracked.go") || strings.Contains(paths, "backend/staged_only.go") {
		t.Fatalf("fixture source list did not bind HEAD only: %q", paths)
	}
	for _, path := range licensePrivacyRequiredPaths() {
		if !strings.Contains(paths, path) {
			t.Fatalf("fixture source list omitted required committed path %q", path)
		}
	}
}

func TestLicenseFilePrivacyAcceptanceSourceExportUsesImmutableHEAD(t *testing.T) {
	fixtureRepo, fixtureScript := newLicensePrivacyFixtureRepo(t)
	writeLicensePrivacyFixtureFile(t, fixtureRepo, "Makefile", "fixture:\n\t@false\n", 0o600)
	writeLicensePrivacyFixtureFile(t, fixtureRepo, "backend/staged_only.go", "package backend\n", 0o600)
	runLicensePrivacyGit(t, fixtureRepo, "add", "--", "backend/staged_only.go")
	stubDir := t.TempDir()
	for _, name := range []string{"docker", "npm"} {
		writeLicensePrivacyFixtureFile(t, stubDir, name, "#!/bin/sh\n: >\"$TRIPWIRE\"\nexit 97\n", 0o700)
	}
	exportDir := filepath.Join(t.TempDir(), "source-package")
	cmd := exec.Command("/bin/bash", fixtureScript)
	cmd.Dir = fixtureRepo
	cmd.Env = []string{
		"LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR=" + exportDir,
		"TRIPWIRE=" + filepath.Join(t.TempDir(), "called"),
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export immutable HEAD package: %v: %s", err, output)
	}
	entries, err := os.ReadDir(exportDir)
	if err != nil || len(entries) != 4 {
		t.Fatalf("source export must contain exactly four artifacts: %v, %v", entries, err)
	}
	for _, name := range []string{"source-files.z", "source-sha256.txt", "source.tar", "package-sha256.txt"} {
		info, err := os.Lstat(filepath.Join(exportDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("source export artifact %q must be mode-0600 regular: %v, %v", name, info, err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(exportDir, "package-sha256.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(manifest), "\n"), "\n")
	if len(lines) != 3 || !strings.HasSuffix(lines[0], "  source-files.z") || !strings.HasSuffix(lines[1], "  source-sha256.txt") || !strings.HasSuffix(lines[2], "  source.tar") {
		t.Fatalf("package manifest must have strict three-artifact order: %q", manifest)
	}
	check := exec.Command("sha256sum", "-c", "package-sha256.txt")
	check.Dir = exportDir
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("verify package manifest: %v: %s", err, output)
	}
	extracted := t.TempDir()
	licensePrivacyExtractTar(t, filepath.Join(exportDir, "source.tar"), extracted)
	list, err := os.ReadFile(filepath.Join(exportDir, "source-files.z"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(licensePrivacyDirectoryFiles(t, extracted), "\x00")+"\x00", string(list); got != want {
		t.Fatalf("archive file list does not equal source list: got %q want %q", got, want)
	}
	gotMakefile, err := os.ReadFile(filepath.Join(extracted, "Makefile"))
	if err != nil || string(gotMakefile) != "fixture:\n\t@true\n" {
		t.Fatalf("exported Makefile = %q, %v; want committed HEAD bytes", gotMakefile, err)
	}
	if _, err := os.Stat(cmd.Env[1][9:]); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source export invoked Docker or npm")
	}

	for _, tc := range []struct{ name, destination string }{
		{"relative", "source-package"},
		{"root", "/"},
		{"pre-existing", t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("/bin/bash", fixtureScript)
			cmd.Dir = fixtureRepo
			cmd.Env = []string{"LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR=" + tc.destination, "PATH=" + os.Getenv("PATH")}
			if err := cmd.Run(); err == nil {
				t.Fatalf("export destination %q unexpectedly succeeded", tc.destination)
			}
		})
	}
	cmd = exec.Command("/bin/bash", fixtureScript)
	cmd.Dir = fixtureRepo
	cmd.Env = []string{"LICENSE_FILE_PRIVACY_SOURCE_LIST_ONLY=1", "LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR=" + filepath.Join(t.TempDir(), "both"), "PATH=" + os.Getenv("PATH")}
	if err := cmd.Run(); err == nil {
		t.Fatal("combined source modes unexpectedly succeeded")
	}
}

func TestLicenseFilePrivacyAcceptanceMetadataFreePackageRefusesOrProgressesBeforeDocker(t *testing.T) {
	t.Run("valid package reaches Docker without Git metadata", func(t *testing.T) {
		remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
		marker := filepath.Join(t.TempDir(), "docker-called")
		output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, licensePrivacyDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n"), marker, "")
		if err == nil || !licensePrivacyFileExists(marker) {
			t.Fatalf("valid metadata-free package did not reach Docker: %v: %s", err, output)
		}
	})
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, remote, packageDir string)
	}{
		{"wrong authorization digest", func(t *testing.T, _, _ string) {}},
		{"changed package artifact", func(t *testing.T, _, packageDir string) {
			writeLicensePrivacyFixtureFile(t, packageDir, "source.tar", "tampered", 0o600)
		}},
		{"changed received source", func(t *testing.T, remote, _ string) {
			writeLicensePrivacyFixtureFile(t, remote, "Makefile", "tampered\n", 0o600)
		}},
		{"missing package artifact", func(t *testing.T, _, packageDir string) {
			if err := os.Remove(filepath.Join(packageDir, "source.tar")); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink package artifact", func(t *testing.T, _, packageDir string) {
			if err := os.Remove(filepath.Join(packageDir, "source.tar")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("source-files.z", filepath.Join(packageDir, "source.tar")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
			tc.mutate(t, remote, packageDir)
			marker := filepath.Join(t.TempDir(), "docker-called")
			digest := ""
			if tc.name == "wrong authorization digest" {
				digest = strings.Repeat("0", 64)
			}
			output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, licensePrivacyDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n"), marker, digest)
			if err == nil {
				t.Fatal("invalid source package unexpectedly succeeded")
			}
			if licensePrivacyFileExists(marker) {
				t.Fatalf("invalid source package reached Docker: %s", output)
			}
		})
	}
}

func TestLicenseFilePrivacyAcceptanceRefusesEvidenceAndProjectReuse(t *testing.T) {
	t.Run("existing evidence", func(t *testing.T) {
		remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
		evidence := filepath.Join(remote, "deploy", "acceptance", "evidence", "license-file-privacy")
		if err := os.MkdirAll(evidence, 0o700); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(t.TempDir(), "docker-called")
		output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, licensePrivacyDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n"), marker, "")
		if err == nil || licensePrivacyFileExists(marker) {
			t.Fatalf("existing evidence was not refused before Docker: %v: %s", err, output)
		}
	})
	for _, resource := range []string{"container", "volume", "network"} {
		t.Run(resource, func(t *testing.T) {
			remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
			marker := filepath.Join(t.TempDir(), "docker-called")
			stub := "#!/bin/sh\ncase \" $* \" in\n  *\" " + resource + " ls \"*) printf 'present\\n'; exit 0;;\n  *\" container ls \"*|*\" volume ls \"*|*\" network ls \"*) exit 0;;\nesac\n: >\"$DOCKER_CALLED\"\nexit 99\n"
			output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, licensePrivacyDockerStub(t, stub), marker, "")
			if err == nil || licensePrivacyFileExists(marker) {
				t.Fatalf("existing %s was not refused: %v: %s", resource, err, output)
			}
		})
	}
}

func TestLicenseFilePrivacyAcceptanceRetainsSanitizedFailureEvidence(t *testing.T) {
	remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
	marker := filepath.Join(t.TempDir(), "docker-called")
	stub := `#!/bin/sh
: >>"$DOCKER_CALLED"
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0;;
  *" inspect "*) printf '/secondhand-market-api|absent|absent|absent\n/secondhand-market-web|absent|absent|absent\n/secondhand-market-mysql|absent|absent|absent\n'; exit 0;;
  *" compose "*) printf 'Authorization: Bearer license-privacy-secret\n' >&2; exit 42;;
esac
exit 0
`
	output, runErr := runLicensePrivacyAcceptance(t, remote, packageDir, script, licensePrivacyDockerStub(t, stub), marker, "")
	if runErr == nil {
		t.Fatal("controlled Docker failure unexpectedly succeeded")
	}
	evidence := filepath.Join(remote, "deploy", "acceptance", "evidence", "license-file-privacy")
	entries, err := os.ReadDir(evidence)
	if err != nil {
		t.Fatalf("read retained evidence: %v; output: %s", err, output)
	}
	allowed := map[string]bool{"acceptance-results.txt": true, "production-before.txt": true, "production-after.txt": true, "evidence-leak-scan.txt": true, "evidence-sha256.txt": true}
	var retained bytes.Buffer
	for _, entry := range entries {
		if entry.IsDir() || !allowed[entry.Name()] {
			t.Fatalf("unexpected retained evidence %q", entry.Name())
		}
		raw, err := os.ReadFile(filepath.Join(evidence, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		retained.Write(raw)
	}
	if strings.Contains(retained.String(), "license-privacy-secret") || strings.Contains(retained.String(), "missing-file-records") {
		t.Fatalf("retained evidence leaked raw output: %q", retained.String())
	}
	check := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
	check.Dir = evidence
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("verify evidence hashes: %v: %s", err, output)
	}
}

func TestLicenseFilePrivacyAcceptancePreservesBehaviorMatrix(t *testing.T) {
	script, err := os.ReadFile(filepath.Join(licensePrivacyAcceptanceRepoDir(t), "deploy/acceptance/license-file-privacy-smoke.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"expect_preflight_failure missing-file-records", "expect_preflight_failure both-file-tables", "missing-column-$column", "missing-index-owner", "empty-license-object-key", "merchant-uploader-mismatch",
		"0007_license_file_privacy.preflight.sql", "0008_anonymous_upload_governance.preflight.sql", "0009_buyer_intent_open_uniqueness.preflight.sql", "AUTO_MIGRATE=false", "AUTO_MIGRATE=true", "production-before", "production-after",
	} {
		if !bytes.Contains(script, []byte(marker)) {
			t.Errorf("license privacy behavior matrix omitted %q", marker)
		}
	}
}

func prepareMetadataFreeLicensePrivacyAcceptance(t *testing.T) (string, string, string) {
	t.Helper()
	fixture, fixtureScript := newLicensePrivacyFixtureRepo(t)
	remote := t.TempDir()
	packageDir := filepath.Join(remote, ".license-privacy-source")
	command := exec.Command("/bin/bash", fixtureScript)
	command.Dir = fixture
	command.Env = []string{"LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR=" + packageDir, "PATH=" + os.Getenv("PATH")}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("export source package: %v: %s", err, output)
	}
	licensePrivacyExtractTar(t, filepath.Join(packageDir, "source.tar"), remote)
	writeLicensePrivacyFixtureFile(t, remote, "deploy/acceptance/.env", "MYSQL_DATABASE=second_hand_market_acceptance\n", 0o600)
	if _, err := os.Stat(filepath.Join(remote, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("metadata-free fixture contains .git")
	}
	return remote, packageDir, filepath.Join(remote, "deploy", "acceptance", "license-file-privacy-smoke.sh")
}

func runLicensePrivacyAcceptance(t *testing.T, remote, packageDir, script, stubDir, marker, digest string) ([]byte, error) {
	t.Helper()
	manifest, err := os.ReadFile(filepath.Join(packageDir, "package-sha256.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" {
		sum := sha256.Sum256(manifest)
		digest = hex.EncodeToString(sum[:])
	}
	command := exec.Command("/bin/bash", script)
	command.Dir = remote
	command.Env = []string{
		"LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA", "ACCEPTANCE_DB_ENGINE=mysql8.4", "COMPOSE_PROJECT_NAME=secondhand-license-privacy-acceptance", "LICENSE_FILE_PRIVACY_SOURCE_PACKAGE_DIR=" + packageDir, "LICENSE_FILE_PRIVACY_SOURCE_PACKAGE_MANIFEST_SHA256=" + digest, "DOCKER_CALLED=" + marker, "PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}
	return command.CombinedOutput()
}

func licensePrivacyDockerStub(t *testing.T, contents string) string {
	t.Helper()
	root := t.TempDir()
	writeLicensePrivacyFixtureFile(t, root, "docker", contents, 0o700)
	return root
}
func licensePrivacyFileExists(path string) bool { _, err := os.Stat(path); return err == nil }

func licensePrivacyRequiredPaths() []string {
	return []string{
		"Makefile", "backend/Dockerfile", "backend/go.mod", "backend/go.sum",
		"backend/migrations/0007_license_file_privacy.preflight.sql",
		"backend/migrations/0007_license_file_privacy.up.sql",
		"backend/migrations/0007_license_file_privacy.postflight.sql",
		"backend/migrations/license_file_privacy_migration_test.go",
		"backend/tests/file_schema_mysql_test.go", "backend/tests/license_file_privacy_test.go",
		"backend/tests/license_file_privacy_acceptance_contract_test.go",
		"deploy/acceptance/docker-compose.yml", "deploy/acceptance/license-file-privacy-smoke.sh",
	}
}

func newLicensePrivacyFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	script, err := os.ReadFile(filepath.Join(licensePrivacyAcceptanceRepoDir(t), "deploy/acceptance/license-file-privacy-smoke.sh"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Makefile": "fixture:\n\t@true\n", "backend/Dockerfile": "FROM scratch\n", "backend/go.mod": "module fixture.invalid/license\n\ngo 1.22\n", "backend/go.sum": "",
		"backend/migrations/0007_license_file_privacy.preflight.sql": "SELECT 1;\n", "backend/migrations/0007_license_file_privacy.up.sql": "SELECT 1;\n", "backend/migrations/0007_license_file_privacy.postflight.sql": "SELECT 1;\n",
		"backend/migrations/license_file_privacy_migration_test.go": "package migrations\n", "backend/tests/file_schema_mysql_test.go": "package tests\n", "backend/tests/license_file_privacy_test.go": "package tests\n", "backend/tests/license_file_privacy_acceptance_contract_test.go": "package tests\n", "deploy/acceptance/docker-compose.yml": "services: {}\n",
	}
	for path, data := range files {
		writeLicensePrivacyFixtureFile(t, repo, path, data, 0o600)
	}
	writeLicensePrivacyFixtureFile(t, repo, "deploy/acceptance/license-file-privacy-smoke.sh", string(script), 0o700)
	runLicensePrivacyGit(t, repo, "init", "-q")
	runLicensePrivacyGit(t, repo, "add", "--", ".")
	runLicensePrivacyGit(t, repo, "-c", "user.name=Acceptance Contract", "-c", "user.email=acceptance@example.invalid", "commit", "-q", "-m", "fixture")
	return repo, filepath.Join(repo, "deploy/acceptance/license-file-privacy-smoke.sh")
}

func writeLicensePrivacyFixtureFile(t *testing.T, root, path, contents string, mode os.FileMode) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func runLicensePrivacyGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func licensePrivacyAcceptanceRepoDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func licensePrivacySplitNULPaths(t *testing.T, raw []byte) []string {
	t.Helper()
	if len(raw) == 0 || raw[len(raw)-1] != 0 {
		t.Fatal("source list must be non-empty NUL-delimited")
	}
	parts := bytes.Split(raw[:len(raw)-1], []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		path := string(part)
		if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.Contains(path, "//") || strings.HasPrefix(path, "../") {
			t.Fatalf("unsafe source path %q", path)
		}
		paths = append(paths, path)
	}
	return paths
}

func licensePrivacyForbiddenPath(path string) bool {
	lower := strings.ToLower(path)
	if path == "backend/app.db" || strings.HasPrefix(path, "docs/") {
		return true
	}
	for _, suffix := range []string{".db", ".sqlite", ".sqlite3"} {
		if strings.HasSuffix(lower, suffix) || strings.Contains(lower, suffix+".") {
			return true
		}
	}
	for _, component := range strings.Split(lower, "/") {
		switch component {
		case ".env", ".git", ".tmp", ".cache", "cache", "caches", "secret", "secrets", "database", "databases", "upload", "uploads", "evidence", "backup", "backups", "node_modules":
			return true
		}
	}
	return false
}

func licensePrivacyExtractTar(t *testing.T, archivePath, destination string) {
	t.Helper()
	archive, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeXHeader || header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		path := filepath.Clean(filepath.FromSlash(header.Name))
		if path == "." || filepath.IsAbs(path) || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			t.Fatalf("unsafe archive path %q", header.Name)
		}
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(filepath.Join(destination, path), 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			t.Fatalf("unexpected archive entry %q", header.Name)
		}
		target := filepath.Join(destination, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(f, reader); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func licensePrivacyDirectoryFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
