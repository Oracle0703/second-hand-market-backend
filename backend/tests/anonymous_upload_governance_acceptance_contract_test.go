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

func TestAnonymousUploadGovernanceAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T) {
	fixtureRepo, fixtureScript := newAnonymousUploadGovernanceFixtureRepo(t)
	for _, path := range []string{
		"backend/control\nname.go",
		"backend/back\\slash.go",
		"frontend/src/nonportable-\u2603.ts",
	} {
		writeAnonymousUploadGovernanceFixtureFile(t, fixtureRepo, path, "fixture\n", 0o600)
	}
	runAnonymousUploadGovernanceGit(t, fixtureRepo, "add", "--", "backend/control\nname.go", "backend/back\\slash.go", "frontend/src/nonportable-\u2603.ts")
	runAnonymousUploadGovernanceGit(t, fixtureRepo, "-c", "user.name=Acceptance Contract", "-c", "user.email=acceptance-contract@example.invalid", "commit", "-q", "-m", "nonportable committed names")
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
	for _, forbidden := range []string{"backend/control\nname.go", "backend/back\\slash.go", "frontend/src/nonportable-\u2603.ts"} {
		if present[forbidden] {
			t.Fatalf("source-list accepted non-portable committed path %q", forbidden)
		}
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

	t.Run("final chmod failure removes incomplete export", func(t *testing.T) {
		repo, script := newAnonymousUploadGovernanceFixtureRepo(t)
		stubDir := t.TempDir()
		writeAnonymousUploadGovernanceFixtureFile(t, stubDir, "chmod", `#!/bin/sh
case "$*" in
  *package-sha256.txt*) exit 73 ;;
esac
exec /bin/chmod "$@"
`, 0o700)
		destination := filepath.Join(t.TempDir(), "failed-export")
		command := exec.Command("/bin/bash", script)
		command.Dir = repo
		command.Env = []string{
			"ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR=" + destination,
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

func TestAnonymousUploadGovernanceAcceptanceMetadataFreePackageRefusesOrProgressesBeforeDocker(t *testing.T) {
	t.Run("missing explicit package directory refuses before Docker", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		manifest, err := os.ReadFile(filepath.Join(packageDir, "package-sha256.txt"))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(manifest)
		command := exec.Command("/bin/bash", script)
		command.Dir = remoteRepo
		command.Env = []string{
			"ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA",
			"ACCEPTANCE_DB_ENGINE=mysql8.4",
			"COMPOSE_PROJECT_NAME=secondhand-upload-governance-acceptance",
			"ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_MANIFEST_SHA256=" + hex.EncodeToString(digest[:]),
			"DOCKER_CALLED=" + marker,
			"DOCKER_STATE=" + filepath.Join(stubDir, "state"),
			"PATH=" + stubDir + ":" + os.Getenv("PATH"),
		}
		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatalf("missing package directory unexpectedly succeeded: %s", output)
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing explicit package directory reached Docker: %q", output)
		}
	})

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

	t.Run("malformed archive diagnostics stay private", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		writeAnonymousUploadGovernanceFixtureFile(t, packageDir, "source.tar", "malformed archive\n", 0o600)
		anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		writeAnonymousUploadGovernanceFixtureFile(t, stubDir, "tar", `#!/bin/sh
case "$*" in
  *source.tar*)
    printf 'Authorization: Bearer upload-archive-secret\n' >&2
    exit 64
    ;;
esac
exec /usr/bin/tar "$@"
`, 0o700)
		output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, "")
		if err == nil {
			t.Fatal("malformed re-authorized archive unexpectedly succeeded")
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("malformed re-authorized archive reached Docker")
		}
		for _, secret := range []string{"Authorization", "Bearer", "upload-archive-secret"} {
			if bytes.Contains(output, []byte(secret)) {
				t.Fatalf("archive validation diagnostic leaked %q to caller output: %q", secret, output)
			}
		}
	})

	t.Run("archive link refuses before extraction", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		anonymousUploadGovernanceRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "", "", "Makefile")
		anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		extractMarker := filepath.Join(stubDir, "tar-extract-called")
		writeAnonymousUploadGovernanceFixtureFile(t, stubDir, "tar", fmt.Sprintf(`#!/bin/sh
case " $* " in *" -xf "*source.tar*) : >%q ;; esac
exec /usr/bin/tar "$@"
`, extractMarker), 0o700)
		if output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, ""); err == nil {
			t.Fatalf("archive link unexpectedly succeeded: %s", output)
		}
		if _, err := os.Stat(extractMarker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("archive link reached extraction before type refusal")
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("archive link reached Docker")
		}
	})

	for _, tamper := range []struct {
		name  string
		apply func(t *testing.T, remoteRepo, packageDir string)
	}{
		{"wrong authorized package digest", func(t *testing.T, _, _ string) {}},
		{"extra package file", func(t *testing.T, _, packageDir string) {
			writeAnonymousUploadGovernanceFixtureFile(t, packageDir, "unexpected.txt", "unexpected\n", 0o600)
		}},
		{"extra package directory", func(t *testing.T, _, packageDir string) {
			if err := os.Mkdir(filepath.Join(packageDir, "unexpected"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"noncanonical package manifest whitespace", func(t *testing.T, _, packageDir string) {
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, " ")
		}},
		{"changed package artifact", func(t *testing.T, _, packageDir string) {
			writeAnonymousUploadGovernanceFixtureFile(t, packageDir, "source.tar", "not a tar archive", 0o600)
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
		{"changed received file", func(t *testing.T, remoteRepo, _ string) {
			writeAnonymousUploadGovernanceFixtureFile(t, remoteRepo, "Makefile", "fixture:\n\t@false\n", 0o600)
		}},
		{"received source symlink", func(t *testing.T, remoteRepo, _ string) {
			if err := os.Remove(filepath.Join(remoteRepo, "Makefile")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("backend/go.mod", filepath.Join(remoteRepo, "Makefile")); err != nil {
				t.Fatal(err)
			}
		}},
		{"received ancestor directory symlink", func(t *testing.T, remoteRepo, _ string) {
			backend := filepath.Join(remoteRepo, "backend")
			relocated := filepath.Join(remoteRepo, "received-backend")
			if err := os.Rename(backend, relocated); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("received-backend", backend); err != nil {
				t.Fatal(err)
			}
		}},
		{"archive extra file", func(t *testing.T, _, packageDir string) {
			anonymousUploadGovernanceRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "", "backend/archive-extra.go", "")
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		}},
		{"archive missing file", func(t *testing.T, _, packageDir string) {
			anonymousUploadGovernanceRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "Makefile", "", "")
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		}},
		{"archive symlink entry", func(t *testing.T, _, packageDir string) {
			anonymousUploadGovernanceRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "", "", "Makefile")
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		}},
		{"archive duplicate directory entry", func(t *testing.T, _, packageDir string) {
			anonymousUploadGovernanceRewriteArchiveMember(t, filepath.Join(packageDir, "source.tar"), "backend/", "duplicate", "")
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		}},
		{"archive omitted directory entry", func(t *testing.T, _, packageDir string) {
			anonymousUploadGovernanceRewriteArchiveMember(t, filepath.Join(packageDir, "source.tar"), "backend/", "omit", "")
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		}},
		{"archive special entry", func(t *testing.T, _, packageDir string) {
			anonymousUploadGovernanceRewriteArchiveMember(t, filepath.Join(packageDir, "source.tar"), "Makefile", "fifo", "")
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		}},
		{"archive unsafe entry", func(t *testing.T, _, packageDir string) {
			anonymousUploadGovernanceRewriteArchiveMember(t, filepath.Join(packageDir, "source.tar"), "Makefile", "rename", "../Makefile")
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		}},
		{"archive noncanonical entry", func(t *testing.T, _, packageDir string) {
			anonymousUploadGovernanceRewriteArchiveMember(t, filepath.Join(packageDir, "source.tar"), "Makefile", "rename", "./Makefile")
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		}},
		{"unsorted source list", func(t *testing.T, _, packageDir string) {
			paths := anonymousUploadGovernanceReadSourcePaths(t, packageDir)
			paths[0], paths[1] = paths[1], paths[0]
			anonymousUploadGovernanceWriteSourcePaths(t, packageDir, paths)
		}},
		{"duplicate source list", func(t *testing.T, _, packageDir string) {
			paths := anonymousUploadGovernanceReadSourcePaths(t, packageDir)
			paths = append(paths, paths[0])
			sort.Strings(paths)
			anonymousUploadGovernanceWriteSourcePaths(t, packageDir, paths)
		}},
		{"forbidden source list", func(t *testing.T, _, packageDir string) {
			paths := append(anonymousUploadGovernanceReadSourcePaths(t, packageDir), "backend/.env")
			sort.Strings(paths)
			anonymousUploadGovernanceWriteSourcePaths(t, packageDir, paths)
		}},
		{"nonportable source list", func(t *testing.T, _, packageDir string) {
			paths := append(anonymousUploadGovernanceReadSourcePaths(t, packageDir), "backend/control\nname.go")
			sort.Strings(paths)
			anonymousUploadGovernanceWriteSourcePaths(t, packageDir, paths)
		}},
		{"dot component source list", func(t *testing.T, _, packageDir string) {
			paths := append(anonymousUploadGovernanceReadSourcePaths(t, packageDir), "backend/./dot.go")
			sort.Strings(paths)
			anonymousUploadGovernanceWriteSourcePaths(t, packageDir, paths)
		}},
		{"leading dash source list", func(t *testing.T, _, packageDir string) {
			paths := append(anonymousUploadGovernanceReadSourcePaths(t, packageDir), "-backend/leading.go")
			sort.Strings(paths)
			anonymousUploadGovernanceWriteSourcePaths(t, packageDir, paths)
		}},
		{"repeated separator source list", func(t *testing.T, _, packageDir string) {
			paths := append(anonymousUploadGovernanceReadSourcePaths(t, packageDir), "backend//repeated.go")
			sort.Strings(paths)
			anonymousUploadGovernanceWriteSourcePaths(t, packageDir, paths)
		}},
		{"missing required source path", func(t *testing.T, _, packageDir string) {
			paths := anonymousUploadGovernanceReadSourcePaths(t, packageDir)
			anonymousUploadGovernanceWriteSourcePaths(t, packageDir, paths[1:])
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
			anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
		}},
	} {
		t.Run("tampered "+tamper.name+" refuses before Docker", func(t *testing.T) {
			remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
			tamper.apply(t, remoteRepo, packageDir)
			marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
			digest := ""
			if tamper.name == "wrong authorized package digest" {
				digest = strings.Repeat("0", 64)
			}
			if _, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, digest); err == nil {
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
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("evidence reuse reached Docker before refusal: %q", output)
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
  *" container ls "*"name=^/secondhand-market-api$"*) printf 'secondhand-market-api\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-web$"*) printf 'secondhand-market-web\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-mysql$"*) printf 'secondhand-market-mysql\n'; exit 0 ;;
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect "*)
    case "$*" in
      *secondhand-market-api*) name=secondhand-market-api ;;
      *secondhand-market-web*) name=secondhand-market-web ;;
      *) name=secondhand-market-mysql ;;
    esac
    case "$*" in *" --format "*) printf '/%s|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|running|0\n' "$name" ;; esac
    exit 0 ;;
  *" compose "*) printf 'Authorization: Bearer injected-secret object_key=unsafe /var/lib/second-hand-market/uploads\n' >&2; exit 42 ;;
esac
exit 0
`)
	output, runErr := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, "")
	if runErr == nil {
		t.Fatal("controlled Docker failure unexpectedly succeeded")
	}
	for _, forbidden := range []string{"Authorization", "Bearer", "injected-secret", "object_key", "/var/lib/second-hand-market/uploads"} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("controlled raw failure leaked %q to caller output: %q", forbidden, output)
		}
	}
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		t.Fatalf("read retained evidence: %v", err)
	}
	allowed := map[string]bool{
		"acceptance-results.txt": true,
		"production-before.txt":  true,
		"production-after.txt":   true,
		"evidence-leak-scan.txt": true,
		"evidence-sha256.txt":    true,
	}
	var retained bytes.Buffer
	for _, entry := range entries {
		if entry.IsDir() || !allowed[entry.Name()] {
			t.Fatalf("retained evidence contains unexpected entry %q", entry.Name())
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
	results, err := os.ReadFile(filepath.Join(evidenceDir, "acceptance-results.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(results), "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "classification=source_package|result=PASS|count=") ||
		lines[1] != "classification=acceptance_failure|result=FAIL|stage=mysql_start|count=1" {
		t.Fatalf("failure classifications are not strict and ordered: %q", results)
	}
	check := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
	check.Dir = evidenceDir
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("verify retained evidence hashes: %v: %s", err, output)
	}

	t.Run("unsafe after snapshot uses hardcoded fallback", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, `#!/bin/sh
: >>"$DOCKER_CALLED"
mkdir -p "$DOCKER_STATE"
case " $* " in
  *" container ls "*"name=^/secondhand-market-api$"*) printf 'secondhand-market-api\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-web$"*) printf 'secondhand-market-web\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-mysql$"*) printf 'secondhand-market-mysql\n'; exit 0 ;;
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect "*)
    case "$*" in
      *secondhand-market-api*) name=secondhand-market-api ;;
      *secondhand-market-web*) name=secondhand-market-web ;;
      *) name=secondhand-market-mysql ;;
    esac
    case "$*" in
      *" --format "*)
        count=0; [ ! -f "$DOCKER_STATE/formatted" ] || count="$(cat "$DOCKER_STATE/formatted")"
        count=$((count + 1)); printf '%s\n' "$count" >"$DOCKER_STATE/formatted"
        if [ "$count" -eq 4 ]; then
          printf '/%s|Authorization|unsafe|0\n' "$name"
        else
          printf '/%s|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|running|0\n' "$name"
        fi
        ;;
    esac
    exit 0 ;;
  *" compose "*) printf 'Bearer fallback-secret\n' >&2; exit 42 ;;
esac
exit 0
`)
		output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, "")
		if err == nil {
			t.Fatal("unsafe controlled failure unexpectedly succeeded")
		}
		if bytes.Contains(output, []byte("fallback-secret")) {
			t.Fatalf("fallback raw output leaked to caller: %q", output)
		}
		evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
		entries, err := os.ReadDir(evidenceDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 3 {
			t.Fatalf("fallback evidence entries = %v, want exactly three", entries)
		}
		results, err := os.ReadFile(filepath.Join(evidenceDir, "acceptance-results.txt"))
		if err != nil || string(results) != "classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n" {
			t.Fatalf("fallback classification = %q, %v", results, err)
		}
		scan, err := os.ReadFile(filepath.Join(evidenceDir, "evidence-leak-scan.txt"))
		if err != nil || string(scan) != "classification=evidence_scan|result=FAIL|count=1\n" {
			t.Fatalf("fallback scan classification = %q, %v", scan, err)
		}
		check := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
		check.Dir = evidenceDir
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("verify fallback evidence hashes: %v: %s", err, output)
		}
	})

	t.Run("malformed checkpoint uses hardcoded fallback", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, `#!/bin/sh
: >>"$DOCKER_CALLED"
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect "*)
    case "$*" in
      *secondhand-market-api*) name=secondhand-market-api ;;
      *secondhand-market-web*) name=secondhand-market-web ;;
      *) name=secondhand-market-mysql ;;
    esac
    case "$*" in *" --format "*) printf '/%s|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|running|0\n' "$name" ;; esac
    exit 0 ;;
  *" compose "*)
    previous=
    runtime=
    for argument in "$@"; do
      if [ "$previous" = "--file" ]; then
        case "$argument" in *anonymous-upload-governance-compose.yml) runtime="$(dirname "$argument")" ;; esac
      fi
      previous="$argument"
    done
    if [ -n "$runtime" ]; then
      printf 'classification=mysql_version|result=PASS|count=999\n' >>"$runtime/raw-evidence/acceptance-results.txt"
    fi
    exit 42 ;;
esac
exit 0
`)
		if output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, ""); err == nil {
			t.Fatalf("malformed checkpoint failure unexpectedly succeeded: %s", output)
		}
		evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
		entries, err := os.ReadDir(evidenceDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 3 {
			t.Fatalf("checkpoint fallback entries = %v, want exactly three", entries)
		}
		results, err := os.ReadFile(filepath.Join(evidenceDir, "acceptance-results.txt"))
		if err != nil || string(results) != "classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n" {
			t.Fatalf("checkpoint fallback classification = %q, %v", results, err)
		}
		scan, err := os.ReadFile(filepath.Join(evidenceDir, "evidence-leak-scan.txt"))
		if err != nil || string(scan) != "classification=evidence_scan|result=FAIL|count=1\n" {
			t.Fatalf("checkpoint fallback scan classification = %q, %v", scan, err)
		}
		check := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
		check.Dir = evidenceDir
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("verify checkpoint fallback hashes: %v: %s", err, output)
		}
	})

	t.Run("publication failure leaves no partial evidence", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, `#!/bin/sh
case " $* " in
  *" container ls "*"name=^/secondhand-market-api$"*) printf 'secondhand-market-api\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-web$"*) printf 'secondhand-market-web\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-mysql$"*) printf 'secondhand-market-mysql\n'; exit 0 ;;
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect "*)
    case "$*" in
      *secondhand-market-api*) name=secondhand-market-api ;;
      *secondhand-market-web*) name=secondhand-market-web ;;
      *) name=secondhand-market-mysql ;;
    esac
    case "$*" in *" --format "*) printf '/%s|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|running|0\n' "$name" ;; esac
    exit 0 ;;
  *" compose "*) exit 42 ;;
esac
exit 0
`)
		writeAnonymousUploadGovernanceFixtureFile(t, stubDir, "tar", `#!/bin/sh
if [ "$*" = "-cf - ." ]; then exit 74; fi
exec /usr/bin/tar "$@"
`, 0o700)
		if output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, ""); err == nil {
			t.Fatalf("forced publication failure unexpectedly succeeded: %s", output)
		}
		retained := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
		if _, err := os.Lstat(retained); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("partial retained evidence survived publication failure: %v", err)
		}
		entries, err := os.ReadDir(filepath.Dir(retained))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "anonymous-upload-governance.publish.") {
				t.Fatalf("partial sibling publication directory survived: %q", entry.Name())
			}
		}
	})

	t.Run("malformed initial snapshot uses hardcoded fallback without caller leakage", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, `#!/bin/sh
case " $* " in
  *" container ls "*"name=^/secondhand-market-api$"*) printf 'secondhand-market-api\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-web$"*) printf 'secondhand-market-web\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-mysql$"*) printf 'secondhand-market-mysql\n'; exit 0 ;;
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect "*)
    case "$*" in
      *secondhand-market-api*) name=secondhand-market-api ;;
      *secondhand-market-web*) name=secondhand-market-web ;;
      *) name=secondhand-market-mysql ;;
    esac
    case "$*" in
      *" --format "*)
        printf '/%s|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|running|\n' "$name"
        printf 'Authorization: Bearer initial-inspect-secret\n' >&2
        ;;
    esac
    exit 0 ;;
  *" compose "*) : >"$DOCKER_CALLED"; exit 99 ;;
esac
exit 0
`)
		output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, "")
		if err == nil {
			t.Fatal("malformed initial snapshot unexpectedly succeeded")
		}
		if bytes.Contains(output, []byte("initial-inspect-secret")) {
			t.Fatalf("formatted inspect diagnostics leaked to caller: %q", output)
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("malformed initial snapshot reached Compose")
		}
		assertAnonymousUploadGovernanceSanitizationFallback(t, remoteRepo)
	})

	t.Run("staged publication tamper leaves no partial evidence", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, `#!/bin/sh
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect "*)
    case "$*" in
      *secondhand-market-api*) name=secondhand-market-api ;;
      *secondhand-market-web*) name=secondhand-market-web ;;
      *) name=secondhand-market-mysql ;;
    esac
    case "$*" in *" --format "*) printf '/%s|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|running|0\n' "$name" ;; esac
    exit 0 ;;
  *" compose "*) exit 42 ;;
esac
exit 0
`)
		writeAnonymousUploadGovernanceFixtureFile(t, stubDir, "tar", `#!/bin/sh
previous=
destination=
for argument in "$@"; do
  [ "$previous" != -C ] || destination="$argument"
  previous="$argument"
done
case "$destination| $* " in
  *anonymous-upload-governance.publish.*'| '*' -xf - '*)
    /usr/bin/tar "$@" || exit $?
    printf 'classification=staging_tamper|result=PASS|count=1\n' >"$destination/acceptance-results.txt"
    exit 0 ;;
esac
exec /usr/bin/tar "$@"
`, 0o700)
		if output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, ""); err == nil {
			t.Fatalf("staged evidence tamper unexpectedly succeeded: %s", output)
		}
		assertAnonymousUploadGovernanceNoPublicationState(t, remoteRepo)
	})

	t.Run("publication no-clobber collision leaves no partial evidence", func(t *testing.T) {
		remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
		marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, `#!/bin/sh
case " $* " in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect "*)
    case "$*" in
      *secondhand-market-api*) name=secondhand-market-api ;;
      *secondhand-market-web*) name=secondhand-market-web ;;
      *) name=secondhand-market-mysql ;;
    esac
    case "$*" in *" --format "*) printf '/%s|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|running|0\n' "$name" ;; esac
    exit 0 ;;
  *" compose "*) exit 42 ;;
esac
exit 0
`)
		writeAnonymousUploadGovernanceFixtureFile(t, stubDir, "mv", `#!/bin/sh
case " $* " in
  *" -n "*anonymous-upload-governance.publish.*)
    while [ "$#" -gt 0 ]; do
      case "$1" in -n|--) shift ;; *) break ;; esac
    done
    source=$1
    target=$2
    mkdir "$target" || exit $?
    printf 'concurrent-owner\n' >"$target/concurrent-owner.txt" || exit $?
    exec /bin/mv -n -- "$source" "$target" ;;
esac
exec /bin/mv "$@"
`, 0o700)
		if output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, ""); err == nil {
			t.Fatalf("publication collision unexpectedly succeeded: %s", output)
		}
		retained := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
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
			t.Fatalf("read evidence parent: %v", err)
		}
		for _, entry := range parentEntries {
			if strings.HasPrefix(entry.Name(), "anonymous-upload-governance.publish.") || entry.Name() == "anonymous-upload-governance.publish.lock" {
				t.Fatalf("partial sibling publication state survived: %q", entry.Name())
			}
		}
	})
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
	sequence, err := os.ReadFile(filepath.Join(stubDir, "sequence"))
	if err != nil {
		t.Fatalf("read F-06 behavior sequence: %v", err)
	}
	gotSequence := strings.Fields(string(sequence))
	wantSequence := []string{
		"production-before", "mysql-start", "mysql-version", "build-test-images",
		"skipped-0007", "dirty-0008-01", "dirty-0008-02", "dirty-0008-03", "dirty-0008-04",
		"clean-migration", "auto-migrate-false", "auto-migrate-true", "backend-tests", "frontend-tests-build",
		"api-web", "upload-01", "upload-02", "upload-03", "upload-04", "upload-05", "upload-06", "upload-07",
		"production-after",
	}
	if strings.Join(gotSequence, "\n") != strings.Join(wantSequence, "\n") {
		t.Fatalf("F-06 behavior sequence =\n%s\nwant =\n%s", strings.Join(gotSequence, "\n"), strings.Join(wantSequence, "\n"))
	}
	evidence, err := os.ReadFile(filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance", "acceptance-results.txt"))
	if err != nil {
		t.Fatalf("read F-06 acceptance results: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(evidence), "\n"), "\n")
	if len(lines) != 12 || !strings.HasPrefix(lines[0], "classification=source_package|result=PASS|count=") ||
		lines[1] != "classification=mysql_version|result=PASS|count=1" ||
		lines[2] != "classification=skipped_0007_preflight|result=PASS|count=1" ||
		lines[3] != "classification=dirty_0008_preflights|result=PASS|count=4" ||
		lines[4] != "classification=clean_migration|result=PASS|count=1" ||
		lines[5] != "classification=mysql_auto_migrate_false|result=PASS|count=1" ||
		lines[6] != "classification=mysql_auto_migrate_true|result=PASS|count=1" ||
		lines[7] != "classification=backend_tests|result=PASS|count=1" ||
		lines[8] != "classification=frontend_tests_build|result=PASS|count=1" ||
		lines[9] != "classification=upload_boundaries|result=PASS|count=7" ||
		lines[10] != "classification=historical_rows_files|result=PASS|count=2" ||
		lines[11] != "classification=production_snapshot|result=PASS|count=3" {
		t.Fatalf("published results are not exact and ordered: %q", evidence)
	}
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
	check := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
	check.Dir = evidenceDir
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("verify successful evidence hashes: %v: %s", err, output)
	}
}

func TestAnonymousUploadGovernanceAcceptanceRefusesProductionInspectionErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		docker string
	}{
		{
			name: "exact-name listing error",
			docker: `#!/bin/sh
case " $* " in
  *" container ls "*"label=com.docker.compose.project="*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" container ls "*"name=^/secondhand-market-"*) printf 'production-listing-secret\n' >&2; exit 125 ;;
  *" inspect "*) exit 125 ;;
  *" compose "*) : >"$DOCKER_CALLED"; exit 99 ;;
esac
exit 0
`,
		},
		{
			name: "present container formatted inspect error",
			docker: `#!/bin/sh
case " $* " in
  *" container ls "*"label=com.docker.compose.project="*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" container ls "*"name=^/secondhand-market-api$"*) printf 'secondhand-market-api\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-"*) exit 0 ;;
  *" inspect "*"secondhand-market-api"*) printf 'production-inspect-secret\n' >&2; exit 125 ;;
  *" inspect "*) exit 125 ;;
  *" compose "*) : >"$DOCKER_CALLED"; exit 99 ;;
esac
exit 0
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remoteRepo, packageDir, script := prepareAnonymousUploadGovernanceMetadataFreeRepo(t)
			marker, stubDir := anonymousUploadGovernanceDockerTripwire(t, tc.docker)
			output, err := runAnonymousUploadGovernanceAcceptance(t, remoteRepo, packageDir, script, stubDir, marker, "")
			if err == nil {
				t.Fatalf("production inspection error produced false PASS: %s", output)
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("production inspection error reached Compose: %v", err)
			}
			for _, forbidden := range [][]byte{[]byte("production-listing-secret"), []byte("production-inspect-secret")} {
				if bytes.Contains(output, forbidden) {
					t.Fatalf("production inspection diagnostics leaked to caller: %q", output)
				}
			}
			assertAnonymousUploadGovernanceSanitizationFallback(t, remoteRepo)
		})
	}
}

const anonymousUploadGovernanceHappyDockerStub = `#!/bin/sh
: >>"$DOCKER_CALLED"
mkdir -p "$DOCKER_STATE"
log() { printf '%s\n' "$1" >>"$DOCKER_SEQUENCE"; }
if [ "$1" = "container" ]; then
  case " $* " in
    *" name=^/secondhand-market-"*)
      count=0; [ ! -f "$DOCKER_STATE/production-lookups" ] || count=$(cat "$DOCKER_STATE/production-lookups")
      count=$((count + 1)); printf '%s' "$count" >"$DOCKER_STATE/production-lookups"
      [ "$count" -ne 1 ] || log production-before
      [ "$count" -ne 4 ] || log production-after
      case "$*" in
        *secondhand-market-api*) printf 'secondhand-market-api\n' ;;
        *secondhand-market-web*) printf 'secondhand-market-web\n' ;;
        *) printf 'secondhand-market-mysql\n' ;;
      esac ;;
  esac
  exit 0
fi
if [ "$1" = "volume" ] || [ "$1" = "network" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then
  case "$*" in
    *secondhand-market-api*) name=secondhand-market-api ;;
    *secondhand-market-web*) name=secondhand-market-web ;;
    *) name=secondhand-market-mysql ;;
  esac
  case " $* " in
    *" --format "*)
      printf '/%s|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|running|0\n' "$name"
      ;;
  esac
  exit 0
fi
if [ "$1" != "compose" ]; then exit 0; fi
case " $* " in
  *" up -d --wait mysql "*) log mysql-start ;;
  *" --profile tools build "*) log build-test-images ;;
  *" up -d --wait api web "*) log api-web ;;
esac
case " $* " in
  *" exec "*)
    case " $* " in
      *"SELECT VERSION()"*) log mysql-version; printf '8.4.0\n' ;;
      *"0008_anonymous_upload_governance.preflight.sql"*)
        n=0; [ -f "$DOCKER_STATE/0008" ] && n=$(cat "$DOCKER_STATE/0008")
        n=$((n + 1)); printf '%s' "$n" >"$DOCKER_STATE/0008"
        case "$n" in
          1) log skipped-0007; printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: 0007 merchant license URL remains public' >&2; exit 1 ;;
          2) log dirty-0008-01; printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: partial 0008 schema exists' >&2; exit 1 ;;
          4) log dirty-0008-02; printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: 0008 columns are drifted' >&2; exit 1 ;;
          6) log dirty-0008-03; printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: fixed quota guard row is missing or drifted' >&2; exit 1 ;;
          8) log dirty-0008-04; printf '%s\n' 'ERROR 1644 (45000) upload governance preflight: quota guard table must use InnoDB' >&2; exit 1 ;;
          9) log clean-migration ;;
        esac ;;
      *"COUNT(*)"*) printf '0\n' ;;
      *"SELECT CONCAT"*) printf 'historical\n' ;;
      *"postflight.sql"*) printf 'anonymous_upload_governance_preflight_passed\nanonymous_upload_governance_migration_applied\nanonymous_upload_governance_postflight_passed\n' ;;
    esac
    exit 0 ;;
  *" run "*)
    case " $* " in
      *"sha256sum /var/lib"*) printf 'license=fixture\nproduct=fixture\n' ;;
      *"AUTO_MIGRATE=false"*) log auto-migrate-false; printf '%s\n' '--- PASS: TestUploadGovernanceMySQLConcurrencyAndCleanup' ;;
      *"AUTO_MIGRATE=true"*) log auto-migrate-true; printf '%s\n' '--- PASS: TestUploadGovernanceMySQLConcurrencyAndCleanup' ;;
      *"go test ./..."*) log backend-tests ;;
      *" frontend-test "*) log frontend-tests-build ;;
    esac
    exit 0 ;;
esac
exit 0
`

const anonymousUploadGovernanceCurlStub = `#!/bin/sh
log() { printf '%s\n' "$1" >>"$DOCKER_SEQUENCE"; }
output=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) output=$2; shift 2 ;;
    *) shift ;;
  esac
done
case "$output" in
  *presign-exact*) log upload-01; status=200; body='{"code":0,"data":{"file_id":"fixture","object_key":"fixture","file_token":"fixture"}}' ;;
  *upload-exact*) log upload-02; status=200; body='{"code":0}' ;;
  *presign-over*) log upload-03; status=400; body='{"code":10008}' ;;
  *direct_exact_11*) log upload-04; status=400; body='{"code":10001,"request_id":"fixture"}' ;;
  *direct_over_11*) log upload-05; status=413; body='{"code":10008,"request_id":"fixture"}' ;;
  *proxy_exact_11*) log upload-06; status=400; body='{"code":10001,"request_id":"fixture"}' ;;
  *proxy_over_11*) log upload-07; status=413; body='{"code":10008,"request_id":"fixture"}' ;;
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

func anonymousUploadGovernanceReadSourcePaths(t *testing.T, packageDir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(packageDir, "source-files.z"))
	if err != nil {
		t.Fatal(err)
	}
	return splitAnonymousUploadGovernanceNULPaths(t, raw)
}

func anonymousUploadGovernanceWriteSourcePaths(t *testing.T, packageDir string, paths []string) {
	t.Helper()
	var raw bytes.Buffer
	for _, path := range paths {
		raw.WriteString(path)
		raw.WriteByte(0)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "source-files.z"), raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	anonymousUploadGovernanceWritePackageManifest(t, packageDir, "  ")
}

func anonymousUploadGovernanceWritePackageManifest(t *testing.T, packageDir, separator string) {
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

func anonymousUploadGovernanceRewriteArchive(t *testing.T, archivePath, omit, extra, symlink string) {
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
		contents := []byte("package fixture\n")
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

func anonymousUploadGovernanceRewriteArchiveMember(t *testing.T, archivePath, target, operation, replacement string) {
	t.Helper()
	raw, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(bytes.NewReader(raw))
	var rewritten bytes.Buffer
	writer := tar.NewWriter(&rewritten)
	found := false
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
		cloned := *header
		if header.Name == target {
			found = true
			switch operation {
			case "omit":
				continue
			case "duplicate":
				if err := anonymousUploadGovernanceWriteTarMember(writer, &cloned, contents); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				cloned.Typeflag = tar.TypeFifo
				cloned.Size = 0
				contents = nil
			case "rename":
				cloned.Name = replacement
			default:
				t.Fatalf("unknown archive mutation %q", operation)
			}
		}
		if err := anonymousUploadGovernanceWriteTarMember(writer, &cloned, contents); err != nil {
			t.Fatal(err)
		}
	}
	if !found {
		t.Fatalf("archive member %q was not found", target)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, rewritten.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func anonymousUploadGovernanceWriteTarMember(writer *tar.Writer, header *tar.Header, contents []byte) error {
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
		_, err := writer.Write(contents)
		return err
	}
	return nil
}

func assertAnonymousUploadGovernanceSanitizationFallback(t *testing.T, remoteRepo string) {
	t.Helper()
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		t.Fatalf("read sanitization fallback: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("sanitization fallback entries = %v, want exactly three", entries)
	}
	results, err := os.ReadFile(filepath.Join(evidenceDir, "acceptance-results.txt"))
	if err != nil || string(results) != "classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n" {
		t.Fatalf("sanitization fallback classification = %q, %v", results, err)
	}
	scan, err := os.ReadFile(filepath.Join(evidenceDir, "evidence-leak-scan.txt"))
	if err != nil || string(scan) != "classification=evidence_scan|result=FAIL|count=1\n" {
		t.Fatalf("sanitization fallback scan = %q, %v", scan, err)
	}
	check := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
	check.Dir = evidenceDir
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("verify sanitization fallback hashes: %v: %s", err, output)
	}
}

func assertAnonymousUploadGovernanceNoPublicationState(t *testing.T, remoteRepo string) {
	t.Helper()
	retained := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "anonymous-upload-governance")
	if _, err := os.Lstat(retained); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial retained evidence survived: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(retained))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "anonymous-upload-governance.publish.") || entry.Name() == "anonymous-upload-governance.publish.lock" {
			t.Fatalf("partial sibling publication state survived: %q", entry.Name())
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
		"DOCKER_SEQUENCE=" + filepath.Join(stubDir, "sequence"),
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}
	return cmd.CombinedOutput()
}
