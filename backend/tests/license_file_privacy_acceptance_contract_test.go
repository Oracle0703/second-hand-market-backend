package tests

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	writeLicensePrivacyFixtureFile(t, fixtureRepo, "backend/control\nname.go", "package backend\n", 0o600)
	writeLicensePrivacyFixtureFile(t, fixtureRepo, "backend/back\\slash.go", "package backend\n", 0o600)
	writeLicensePrivacyFixtureFile(t, fixtureRepo, "backend/nonportable-\u2603.go", "package backend\n", 0o600)
	runLicensePrivacyGit(t, fixtureRepo, "add", "--", "backend/control\nname.go", "backend/back\\slash.go", "backend/nonportable-\u2603.go")
	runLicensePrivacyGit(t, fixtureRepo, "commit", "-q", "-m", "nonportable source names")
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
	if !strings.Contains(paths, "backend/committed.go") || strings.Contains(paths, "backend/untracked.go") ||
		strings.Contains(paths, "backend/staged_only.go") || strings.Contains(paths, "control\nname.go") ||
		strings.Contains(paths, "back\\slash.go") || strings.Contains(paths, "nonportable-") {
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

	t.Run("final chmod failure removes incomplete export", func(t *testing.T) {
		stubDir := t.TempDir()
		writeLicensePrivacyFixtureFile(t, stubDir, "chmod", `#!/bin/sh
case " $* " in
  *"source-files.z "*) exit 73;;
esac
exec /bin/chmod "$@"
`, 0o700)
		destination := filepath.Join(t.TempDir(), "interrupted-package")
		command := exec.Command("/bin/bash", fixtureScript)
		command.Dir = fixtureRepo
		command.Env = []string{
			"LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR=" + destination,
			"PATH=" + stubDir + ":" + os.Getenv("PATH"),
		}
		if output, err := command.CombinedOutput(); err == nil {
			t.Fatalf("export unexpectedly survived final chmod failure: %s", output)
		}
		if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed export destination was retained: %v", err)
		}
	})
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
	t.Run("malformed archive diagnostics stay private", func(t *testing.T) {
		remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
		writeLicensePrivacyFixtureFile(t, packageDir, "source.tar", "malformed archive\n", 0o600)
		licensePrivacyWritePackageManifest(t, packageDir, "  ")
		marker := filepath.Join(t.TempDir(), "docker-called")
		stubDir := licensePrivacyDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		writeLicensePrivacyFixtureFile(t, stubDir, "tar", `#!/bin/sh
case " $* " in
  *" -tvf "*)
    printf 'Authorization: Bearer archive-diagnostic-secret\n' >&2
    exit 64
    ;;
esac
exec /usr/bin/tar "$@"
`, 0o700)
		output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, stubDir, marker, "")
		if err == nil {
			t.Fatal("malformed re-authorized archive unexpectedly succeeded")
		}
		if licensePrivacyFileExists(marker) {
			t.Fatal("malformed re-authorized archive reached Docker")
		}
		for _, secret := range []string{"Authorization", "Bearer", "archive-diagnostic-secret"} {
			if bytes.Contains(output, []byte(secret)) {
				t.Fatalf("archive validation diagnostic leaked %q to caller output: %q", secret, output)
			}
		}
	})
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, remote, packageDir string)
	}{
		{"wrong authorization digest", func(t *testing.T, _, _ string) {}},
		{"extra package file", func(t *testing.T, _, packageDir string) {
			writeLicensePrivacyFixtureFile(t, packageDir, "unexpected.txt", "unexpected\n", 0o600)
		}},
		{"extra package directory", func(t *testing.T, _, packageDir string) {
			if err := os.Mkdir(filepath.Join(packageDir, "unexpected"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"noncanonical package manifest whitespace", func(t *testing.T, _, packageDir string) {
			licensePrivacyWritePackageManifest(t, packageDir, " ")
		}},
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
		{"received source symlink", func(t *testing.T, remote, _ string) {
			if err := os.Remove(filepath.Join(remote, "Makefile")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("backend/go.mod", filepath.Join(remote, "Makefile")); err != nil {
				t.Fatal(err)
			}
		}},
		{"archive extra file", func(t *testing.T, _, packageDir string) {
			licensePrivacyRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "", "backend/archive-extra.go", "")
			licensePrivacyWritePackageManifest(t, packageDir, "  ")
		}},
		{"archive missing file", func(t *testing.T, _, packageDir string) {
			licensePrivacyRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "Makefile", "", "")
			licensePrivacyWritePackageManifest(t, packageDir, "  ")
		}},
		{"archive symlink entry", func(t *testing.T, _, packageDir string) {
			licensePrivacyRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "", "", "Makefile")
			licensePrivacyWritePackageManifest(t, packageDir, "  ")
		}},
		{"unsorted source list", func(t *testing.T, _, packageDir string) {
			paths := licensePrivacyReadSourcePaths(t, packageDir)
			paths[0], paths[1] = paths[1], paths[0]
			licensePrivacyWriteSourcePaths(t, packageDir, paths)
		}},
		{"duplicate source list", func(t *testing.T, _, packageDir string) {
			paths := licensePrivacyReadSourcePaths(t, packageDir)
			paths = append(paths, paths[0])
			sort.Strings(paths)
			licensePrivacyWriteSourcePaths(t, packageDir, paths)
		}},
		{"forbidden source list", func(t *testing.T, _, packageDir string) {
			paths := append(licensePrivacyReadSourcePaths(t, packageDir), "backend/.env")
			sort.Strings(paths)
			licensePrivacyWriteSourcePaths(t, packageDir, paths)
		}},
		{"nonportable source list", func(t *testing.T, _, packageDir string) {
			paths := append(licensePrivacyReadSourcePaths(t, packageDir), "backend/control\nname.go")
			sort.Strings(paths)
			licensePrivacyWriteSourcePaths(t, packageDir, paths)
		}},
		{"dot component source list", func(t *testing.T, _, packageDir string) {
			paths := append(licensePrivacyReadSourcePaths(t, packageDir), "backend/./dot.go")
			sort.Strings(paths)
			licensePrivacyWriteSourcePaths(t, packageDir, paths)
		}},
		{"missing required source list", func(t *testing.T, _, packageDir string) {
			paths := licensePrivacyReadSourcePaths(t, packageDir)
			licensePrivacyWriteSourcePaths(t, packageDir, paths[1:])
		}},
		{"mismatched per-file hash", func(t *testing.T, _, packageDir string) {
			manifestPath := filepath.Join(packageDir, "source-sha256.txt")
			raw, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			copy(raw[:64], strings.Repeat("0", 64))
			if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			licensePrivacyWritePackageManifest(t, packageDir, "  ")
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
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{
			name: "existing evidence",
			setup: func(t *testing.T, evidence string) {
				if err := os.MkdirAll(evidence, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "dangling evidence symlink",
			setup: func(t *testing.T, evidence string) {
				if err := os.MkdirAll(filepath.Dir(evidence), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(filepath.Dir(evidence), "missing-evidence"), evidence); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "stale publication lock",
			setup: func(t *testing.T, evidence string) {
				if err := os.MkdirAll(evidence+".publish.lock", 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
			evidence := filepath.Join(remote, "deploy", "acceptance", "evidence", "license-file-privacy")
			tc.setup(t, evidence)
			marker := filepath.Join(t.TempDir(), "docker-called")
			output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, licensePrivacyDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n"), marker, "")
			if err == nil || licensePrivacyFileExists(marker) {
				t.Fatalf("%s was not refused before Docker: %v: %s", tc.name, err, output)
			}
		})
	}
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
	for _, forbidden := range []string{"Authorization", "Bearer", "license-privacy-secret"} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("controlled raw failure leaked %q to caller output: %q", forbidden, output)
		}
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
	results, err := os.ReadFile(filepath.Join(evidence, "acceptance-results.txt"))
	if err != nil {
		t.Fatal(err)
	}
	resultLines := strings.Split(strings.TrimSuffix(string(results), "\n"), "\n")
	if len(resultLines) != 2 || !strings.HasPrefix(resultLines[0], "classification=source_package|result=PASS|count=") ||
		resultLines[1] != "classification=acceptance_failure|result=FAIL|stage=mysql_start|count=1" {
		t.Fatalf("retained failure classifications are not fixed and ordered: %q", results)
	}
	if scan, err := os.ReadFile(filepath.Join(evidence, "evidence-leak-scan.txt")); err != nil || string(scan) != "classification=evidence_scan|result=PASS|count=0\n" {
		t.Fatalf("evidence scan result = %q, %v", scan, err)
	}
	check := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
	check.Dir = evidence
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("verify evidence hashes: %v: %s", err, output)
	}

	t.Run("unsafe snapshot uses hardcoded fallback", func(t *testing.T) {
		remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
		stateDir := filepath.Join(t.TempDir(), "docker-state")
		stubDir := licensePrivacyDockerStub(t, `#!/bin/sh
args=" $* "
mkdir -p "$DOCKER_STATE"
case "$args" in
  *" container ls "*"label=com.docker.compose.project="*|*" volume ls "*|*" network ls "*) exit 0;;
  *" container ls "*"name=^/secondhand-market-"*)
    count=0
    [ ! -f "$DOCKER_STATE/lookups" ] || count="$(cat "$DOCKER_STATE/lookups")"
    count=$((count + 1)); printf '%s\n' "$count" >"$DOCKER_STATE/lookups"
    [ "$count" -ne 4 ] || printf 'secondhand-market-api\n'
    exit 0
    ;;
  *" inspect "*) printf '/secondhand-market-api|Authorization|unsafe|0\n'; exit 0;;
  *" compose "*" stop "*) exit 0;;
  *" compose "*) printf 'Bearer fallback-secret\n' >&2; exit 42;;
esac
exit 0
`)
		output, err := runLicensePrivacyAcceptanceWithEnv(t, remote, packageDir, script, stubDir,
			filepath.Join(t.TempDir(), "docker-called"), "", []string{"DOCKER_STATE=" + stateDir})
		if err == nil {
			t.Fatal("unsafe controlled failure unexpectedly succeeded")
		}
		if bytes.Contains(output, []byte("fallback-secret")) {
			t.Fatalf("fallback raw output leaked to caller: %q", output)
		}
		evidence := filepath.Join(remote, "deploy", "acceptance", "evidence", "license-file-privacy")
		entries, err := os.ReadDir(evidence)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 3 {
			t.Fatalf("fallback evidence entries = %v, want exactly three", entries)
		}
		results, err := os.ReadFile(filepath.Join(evidence, "acceptance-results.txt"))
		if err != nil || string(results) != "classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n" {
			t.Fatalf("fallback classification = %q, %v", results, err)
		}
		check := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
		check.Dir = evidence
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("verify fallback evidence hashes: %v: %s", err, output)
		}
	})
}

func TestLicenseFilePrivacyAcceptancePreservesBehaviorMatrix(t *testing.T) {
	remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
	marker := filepath.Join(t.TempDir(), "docker-called")
	logPath := filepath.Join(t.TempDir(), "docker-sequence")
	stubDir := licensePrivacyBehaviorDockerStub(t)
	output, err := runLicensePrivacyAcceptanceWithEnv(t, remote, packageDir, script, stubDir, marker, "", []string{
		"DOCKER_SEQUENCE=" + logPath,
		"DOCKER_STATE=" + filepath.Join(t.TempDir(), "docker-state"),
	})
	if err != nil {
		t.Fatalf("complete behavior matrix failed against deterministic Docker boundary: %v: %s", err, output)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(raw))
	want := []string{
		"production-before",
		"dirty-preflight-01", "dirty-preflight-02", "dirty-preflight-03", "dirty-preflight-04",
		"dirty-preflight-05", "dirty-preflight-06", "dirty-preflight-07", "dirty-preflight-08",
		"dirty-preflight-09", "dirty-preflight-10", "dirty-preflight-11", "dirty-preflight-12",
		"dirty-preflight-13", "dirty-preflight-14",
		"clean-0007-preflight", "clean-0007-up", "clean-0007-postflight",
		"clean-0008-preflight", "clean-0008-up", "clean-0008-postflight",
		"clean-0009-preflight", "clean-0009-up", "clean-0009-postflight",
		"focused-auto-migrate-false", "focused-auto-migrate-true", "production-after",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("executed behavior matrix sequence =\n%s\nwant =\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	evidence := filepath.Join(remote, "deploy", "acceptance", "evidence", "license-file-privacy")
	results, err := os.ReadFile(filepath.Join(evidence, "acceptance-results.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(results), "\n"), "\n")
	if len(lines) != 7 || !strings.HasPrefix(lines[0], "classification=source_package|result=PASS|count=") ||
		lines[1] != "classification=mysql_version|result=PASS|count=1" ||
		lines[2] != "classification=license_preflight_failures|result=PASS|count=14" ||
		lines[3] != "classification=clean_migration|result=PASS|count=1" ||
		lines[4] != "classification=api_auto_migrate_false|result=PASS|count=1" ||
		lines[5] != "classification=api_auto_migrate_true|result=PASS|count=1" ||
		lines[6] != "classification=production_snapshot|result=PASS|count=3" {
		t.Fatalf("success classifications are not exact and ordered: %q", results)
	}

	t.Run("publication failure leaves no partial evidence", func(t *testing.T) {
		remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
		marker := filepath.Join(t.TempDir(), "docker-called")
		stubDir := licensePrivacyDockerStub(t, `#!/bin/sh
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0;;
  *" compose "*" stop "*) exit 0;;
  *" compose "*) exit 42;;
esac
exit 0
`)
		writeLicensePrivacyFixtureFile(t, stubDir, "tar", `#!/bin/sh
if [ "$*" = "-cf - ." ]; then exit 74; fi
exec /usr/bin/tar "$@"
`, 0o700)
		output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, stubDir, marker, "")
		if err == nil {
			t.Fatalf("forced publication failure unexpectedly succeeded: %s", output)
		}
		retained := filepath.Join(remote, "deploy", "acceptance", "evidence", "license-file-privacy")
		if _, err := os.Lstat(retained); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("partial retained evidence survived publication failure: %v", err)
		}
		parent := filepath.Dir(retained)
		entries, err := os.ReadDir(parent)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "license-file-privacy.publish.") {
				t.Fatalf("partial sibling publication directory survived: %q", entry.Name())
			}
		}
	})
}

func TestLicenseFilePrivacyAcceptanceRejectsStagedEvidenceTamper(t *testing.T) {
	remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
	marker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := licensePrivacyDockerStub(t, `#!/bin/sh
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0;;
  *" compose "*" stop "*) exit 0;;
  *" compose "*) exit 42;;
esac
exit 0
`)
	writeLicensePrivacyFixtureFile(t, stubDir, "tar", `#!/bin/sh
previous=
destination=
for argument in "$@"; do
  [ "$previous" != -C ] || destination="$argument"
  previous="$argument"
done
case "$destination| $* " in
  *license-file-privacy.publish.*'| '*' -xf - '*)
    /usr/bin/tar "$@" || exit $?
    printf 'unexpected staged evidence\n' >"$destination/unexpected.txt"
    exit 0 ;;
esac
exec /usr/bin/tar "$@"
`, 0o700)
	if output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, stubDir, marker, ""); err == nil {
		t.Fatalf("staged evidence tamper unexpectedly succeeded: %s", output)
	}
	assertLicensePrivacyNoPublicationState(t, remote)
}

func TestLicenseFilePrivacyAcceptanceRejectsPublicationCollision(t *testing.T) {
	remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
	marker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := licensePrivacyDockerStub(t, `#!/bin/sh
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0;;
  *" compose "*" stop "*) exit 0;;
  *" compose "*) exit 42;;
esac
exit 0
`)
	writeLicensePrivacyFixtureFile(t, stubDir, "mv", `#!/bin/sh
case " $* " in
  *license-file-privacy.publish.*)
    while [ "$#" -gt 0 ]; do
      case "$1" in -n|--) shift ;; *) break ;; esac
    done
    source=$1
    target=$2
    case "$source|$target" in
      *license-file-privacy.publish.*'|'*/license-file-privacy)
        [ -d "${target}.publish.lock" ] || exit 91
        mkdir "$target" || exit $?
        printf 'concurrent-owner\n' >"$target/concurrent-owner.txt" || exit $?
        exec /bin/mv "$source" "$target" ;;
    esac ;;
esac
exec /bin/mv "$@"
`, 0o700)
	if output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, stubDir, marker, ""); err == nil {
		t.Fatalf("publication collision unexpectedly succeeded: %s", output)
	}
	assertLicensePrivacyConcurrentPublicationPreserved(t, remote)
}

func TestLicenseFilePrivacyAcceptancePreservesPostRenamePublicationAmbiguity(t *testing.T) {
	remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
	marker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := licensePrivacyDockerStub(t, `#!/bin/sh
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0;;
  *" compose "*) exit 42;;
esac
exit 0
`)
	writeLicensePrivacyFixtureFile(t, stubDir, "mv", `#!/bin/sh
while [ "$#" -gt 0 ]; do case "$1" in -n|--) shift;; *) break;; esac; done
source=$1
target=$2
[ -d "${target}.publish.lock" ] || exit 91
/bin/mv "$source" "$target" || exit $?
printf 'classification=post_rename_tamper|result=PASS|count=1\n' >>"$target/acceptance-results.txt"
`, 0o700)
	if output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, stubDir, marker, ""); err == nil {
		t.Fatalf("post-rename mutation unexpectedly succeeded: %s", output)
	}
	assertLicensePrivacyAmbiguousPublicationPreserved(t, remote)
}

func TestLicenseFilePrivacyAcceptancePreservesPublicationLockReleaseFailure(t *testing.T) {
	remote, packageDir, script := prepareMetadataFreeLicensePrivacyAcceptance(t)
	marker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := licensePrivacyDockerStub(t, `#!/bin/sh
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0;;
  *" compose "*) exit 42;;
esac
exit 0
`)
	writeLicensePrivacyFixtureFile(t, stubDir, "rmdir", `#!/bin/sh
case "$1" in
  *.publish.lock)
    target=${1%.publish.lock}
    [ -d "$target" ] || exit 92
    : >"$0.called"
    exit 73 ;;
esac
exec /bin/rmdir "$@"
`, 0o700)
	if output, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, stubDir, marker, ""); err == nil {
		t.Fatalf("lock-release failure unexpectedly succeeded: %s", output)
	}
	if _, err := os.Stat(filepath.Join(stubDir, "rmdir.called")); err != nil {
		t.Fatalf("publication lock release was not attempted after rename: %v", err)
	}
	assertLicensePrivacyAmbiguousPublicationPreserved(t, remote)
	tripwire := filepath.Join(t.TempDir(), "docker-called")
	if _, err := runLicensePrivacyAcceptance(t, remote, packageDir, script, stubDir, tripwire, ""); err == nil {
		t.Fatal("preserved publication state allowed a later invocation")
	}
	if _, err := os.Stat(tripwire); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preserved publication state reached Docker")
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
	return runLicensePrivacyAcceptanceWithEnv(t, remote, packageDir, script, stubDir, marker, digest, nil)
}

func runLicensePrivacyAcceptanceWithEnv(t *testing.T, remote, packageDir, script, stubDir, marker, digest string, extraEnv []string) ([]byte, error) {
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
	command.Env = append([]string{
		"LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA", "ACCEPTANCE_DB_ENGINE=mysql8.4", "COMPOSE_PROJECT_NAME=secondhand-license-privacy-acceptance", "LICENSE_FILE_PRIVACY_SOURCE_PACKAGE_DIR=" + packageDir, "LICENSE_FILE_PRIVACY_SOURCE_PACKAGE_MANIFEST_SHA256=" + digest, "DOCKER_CALLED=" + marker, "PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}, extraEnv...)
	return command.CombinedOutput()
}

func licensePrivacyDockerStub(t *testing.T, contents string) string {
	t.Helper()
	root := t.TempDir()
	writeLicensePrivacyFixtureFile(t, root, "docker", contents, 0o700)
	return root
}
func licensePrivacyFileExists(path string) bool { _, err := os.Stat(path); return err == nil }

func assertLicensePrivacyNoPublicationState(t *testing.T, remote string) {
	t.Helper()
	retained := filepath.Join(remote, "deploy", "acceptance", "evidence", "license-file-privacy")
	if _, err := os.Lstat(retained); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial retained evidence survived: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(retained))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "license-file-privacy.publish.") || entry.Name() == "license-file-privacy.publish.lock" {
			t.Fatalf("partial sibling publication state survived: %q", entry.Name())
		}
	}
}

func assertLicensePrivacyConcurrentPublicationPreserved(t *testing.T, remote string) {
	t.Helper()
	retained := filepath.Join(remote, "deploy", "acceptance", "evidence", "license-file-privacy")
	entries, err := os.ReadDir(retained)
	if err != nil {
		t.Fatalf("read concurrent publication directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "concurrent-owner.txt" || !entries[0].Type().IsRegular() {
		t.Fatalf("harness changed concurrent publication directory: %v", entries)
	}
	owner, err := os.ReadFile(filepath.Join(retained, "concurrent-owner.txt"))
	if err != nil || string(owner) != "concurrent-owner\n" {
		t.Fatalf("concurrent publication marker changed: %q, %v", owner, err)
	}
	parentEntries, err := os.ReadDir(filepath.Dir(retained))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range parentEntries {
		if strings.HasPrefix(entry.Name(), "license-file-privacy.publish.") || entry.Name() == "license-file-privacy.publish.lock" {
			t.Fatalf("partial sibling publication state survived: %q", entry.Name())
		}
	}
}

func assertLicensePrivacyAmbiguousPublicationPreserved(t *testing.T, remote string) {
	t.Helper()
	retained := filepath.Join(remote, "deploy", "acceptance", "evidence", "license-file-privacy")
	if info, err := os.Stat(retained); err != nil || !info.IsDir() {
		t.Fatalf("ambiguous retained evidence was removed: %v", err)
	}
	if info, err := os.Stat(retained + ".publish.lock"); err != nil || !info.IsDir() {
		t.Fatalf("ambiguous publication lock was removed: %v", err)
	}
}

func licensePrivacyReadSourcePaths(t *testing.T, packageDir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(packageDir, "source-files.z"))
	if err != nil {
		t.Fatal(err)
	}
	return licensePrivacySplitNULPaths(t, raw)
}

func licensePrivacyWriteSourcePaths(t *testing.T, packageDir string, paths []string) {
	t.Helper()
	var raw bytes.Buffer
	for _, path := range paths {
		raw.WriteString(path)
		raw.WriteByte(0)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "source-files.z"), raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	licensePrivacyWritePackageManifest(t, packageDir, "  ")
}

func licensePrivacyWritePackageManifest(t *testing.T, packageDir, separator string) {
	t.Helper()
	var manifest strings.Builder
	for _, name := range []string{"source-files.z", "source-sha256.txt", "source.tar"} {
		raw, err := os.ReadFile(filepath.Join(packageDir, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		fmt.Fprintf(&manifest, "%s%s%s\n", hex.EncodeToString(digest[:]), separator, name)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "package-sha256.txt"), []byte(manifest.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func licensePrivacyRewriteArchive(t *testing.T, archivePath, omit, extra, symlink string) {
	t.Helper()
	raw, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(bytes.NewReader(raw))
	var rewritten bytes.Buffer
	writer := tar.NewWriter(&rewritten)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == omit {
			continue
		}
		cloned := *header
		if header.Name == symlink {
			cloned.Typeflag = tar.TypeSymlink
			cloned.Linkname = "backend/go.mod"
			cloned.Size = 0
			if err := writer.WriteHeader(&cloned); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := writer.WriteHeader(&cloned); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if extra != "" {
		contents := []byte("package tests\n")
		if err := writer.WriteHeader(&tar.Header{Name: extra, Mode: 0o600, Size: int64(len(contents)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, rewritten.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func licensePrivacyBehaviorDockerStub(t *testing.T) string {
	t.Helper()
	return licensePrivacyDockerStub(t, `#!/bin/sh
args=" $* "
state="$DOCKER_STATE"
mkdir -p "$state"
log() { printf '%s\n' "$1" >>"$DOCKER_SEQUENCE"; }
increment() {
  file="$state/$1"
  value=0
  [ ! -f "$file" ] || value="$(cat "$file")"
  value=$((value + 1))
  printf '%s\n' "$value" >"$file"
  printf '%s\n' "$value"
}
case "$args" in
  *" container ls "*"label=com.docker.compose.project="*|*" volume ls "*|*" network ls "*) exit 0;;
esac
case "$args" in
  *" container ls "*"name=^/secondhand-market-"*)
    count="$(increment production_lookup)"
    [ "$count" -ne 1 ] || log production-before
    [ "$count" -ne 4 ] || log production-after
    exit 0
    ;;
  *" inspect "*) exit 0;;
  *" compose "*" stop "*|*" compose "*" up "*|*" compose "*" build "*) exit 0;;
  *"SELECT VERSION()"*) printf '8.4.0\n'; exit 0;;
  *"table_name='file_records'"*) printf '1\n'; exit 0;;
  *"table_name='files'"*) printf '0\n'; exit 0;;
  *"SELECT CONCAT("*) printf 'rows=2|licenses=1|digest=fixture\n'; exit 0;;
  *"SELECT url FROM file_records WHERE id=302"*) printf '/uploads/product.jpg\n'; exit 0;;
  *"SELECT url FROM file_records WHERE id=301"*)
    count="$(increment license_url)"
    [ "$count" -ne 1 ] || printf '/uploads/license.jpg\n'
    exit 0
    ;;
  *"SELECT COUNT(*) FROM file_records WHERE biz_type='MERCHANT_LICENSE'"*) printf '1\n'; exit 0;;
  *"SELECT COUNT(*) FROM file_records"*) printf '2\n'; exit 0;;
  *"/acceptance/migrations/0007_license_file_privacy.preflight.sql"*)
    count="$(increment privacy_preflight)"
    if [ "$count" -le 14 ]; then
      printf 'dirty-preflight-%02d\n' "$count" >>"$DOCKER_SEQUENCE"
      printf '%s\n' 'ERROR 1644 (45000)' \
        'license privacy preflight: canonical file_records table is required' \
        'license privacy preflight: legacy files table must not exist' \
        'license privacy preflight: owner_merchant_id is missing or drifted' \
        'license privacy preflight: capability_token_hash is missing or drifted' \
        'license privacy preflight: capability_expires_at is missing or drifted' \
        'license privacy preflight: owner/biz/scan index is missing or drifted' \
        'license privacy preflight: capability token index is missing or drifted' \
        'license privacy preflight: capability expiry index is missing or drifted' \
        'license privacy preflight: invalid merchant license record' \
        'license privacy preflight: invalid bound merchant license' >&2
      exit 1
    fi
    log clean-0007-preflight
    printf 'license_file_privacy_preflight_passed\n'
    exit 0
    ;;
  *"/acceptance/migrations/0007_license_file_privacy.up.sql"*) log clean-0007-up; exit 0;;
  *"/acceptance/migrations/0007_license_file_privacy.postflight.sql"*)
    count="$(increment privacy_postflight)"
    [ "$count" -ne 1 ] || log clean-0007-postflight
    printf 'license_file_privacy_postflight_passed\n'
    exit 0
    ;;
  *"/acceptance/migrations/0008_anonymous_upload_governance.preflight.sql"*) log clean-0008-preflight; exit 0;;
  *"/acceptance/migrations/0008_anonymous_upload_governance.up.sql"*) log clean-0008-up; exit 0;;
  *"/acceptance/migrations/0008_anonymous_upload_governance.postflight.sql"*) log clean-0008-postflight; exit 0;;
  *"/acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql"*) log clean-0009-preflight; exit 0;;
  *"/acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql"*) log clean-0009-up; exit 0;;
  *"/acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql"*) log clean-0009-postflight; exit 0;;
  *" AUTO_MIGRATE=false "*) log focused-auto-migrate-false; printf '%s\n' '--- PASS: TestLicenseFilePrivacyWithMigrationOnlyMySQL'; exit 0;;
  *" AUTO_MIGRATE=true "*) log focused-auto-migrate-true; printf '%s\n' '--- PASS: TestLicenseFilePrivacyWithMigrationOnlyMySQL'; exit 0;;
esac
exit 0
`)
}

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
