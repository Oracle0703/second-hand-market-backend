package tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

func TestSessionRevocationAcceptanceMetadataFreePackageRefusesOrProgressesBeforeDocker(t *testing.T) {
	t.Run("valid package reaches Docker without Git metadata", func(t *testing.T) {
		remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
		dockerMarker := filepath.Join(t.TempDir(), "docker-called")
		stubDir := writeIdempotencyAcceptanceDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
		output, err := runMetadataFreeSessionRevocationAcceptance(t,
			remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
		if err == nil {
			t.Fatal("fake Docker tripwire unexpectedly allowed acceptance to succeed")
		}
		if _, err := os.Stat(filepath.Join(remoteRepo, ".git")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("metadata-free fixture unexpectedly contains Git metadata")
		}
		if _, err := os.Stat(dockerMarker); err != nil {
			t.Fatalf("valid metadata-free package did not progress to Docker: %v; output = %q", err, output)
		}
	})

	tamperCases := []struct {
		name   string
		mutate func(*testing.T, string, string)
		digest string
	}{
		{
			name:   "wrong authorized digest",
			digest: strings.Repeat("0", 64),
		},
		{
			name: "changed received source",
			mutate: func(t *testing.T, remoteRepo string, _ string) {
				writeIdempotencyAcceptanceFixtureFile(t, remoteRepo, "Makefile", "fixture:\n\t@false\n", 0o600)
			},
		},
		{
			name: "changed package artifact",
			mutate: func(t *testing.T, _ string, packageDir string) {
				file, err := os.OpenFile(filepath.Join(packageDir, "source.tar"), os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatalf("open source archive for tamper: %v", err)
				}
				if _, err := file.Write([]byte("tampered")); err != nil {
					_ = file.Close()
					t.Fatalf("tamper source archive: %v", err)
				}
				if err := file.Close(); err != nil {
					t.Fatalf("close tampered source archive: %v", err)
				}
			},
		},
		{
			name: "missing package artifact",
			mutate: func(t *testing.T, _ string, packageDir string) {
				if err := os.Remove(filepath.Join(packageDir, "source.tar")); err != nil {
					t.Fatalf("remove source archive: %v", err)
				}
			},
		},
		{
			name: "received source symlink",
			mutate: func(t *testing.T, remoteRepo string, _ string) {
				makefile := filepath.Join(remoteRepo, "Makefile")
				if err := os.Remove(makefile); err != nil {
					t.Fatalf("remove received Makefile: %v", err)
				}
				if err := os.Symlink("backend/go.mod", makefile); err != nil {
					t.Fatalf("replace received Makefile with symlink: %v", err)
				}
			},
		},
		{
			name: "missing required source path",
			mutate: func(t *testing.T, _ string, packageDir string) {
				rewriteSessionRevocationSourceList(t, packageDir, func(paths []string) []string {
					return removeSessionRevocationPath(paths, "Makefile")
				})
			},
		},
		{
			name: "duplicate source path",
			mutate: func(t *testing.T, _ string, packageDir string) {
				rewriteSessionRevocationSourceList(t, packageDir, func(paths []string) []string {
					return append(paths, paths[0])
				})
			},
		},
		{
			name: "forbidden source path",
			mutate: func(t *testing.T, _ string, packageDir string) {
				rewriteSessionRevocationSourceList(t, packageDir, func(paths []string) []string {
					return append(paths, "../escape")
				})
			},
		},
		{
			name: "mismatched per-file hash",
			mutate: func(t *testing.T, _ string, packageDir string) {
				manifest := filepath.Join(packageDir, "source-sha256.txt")
				if err := os.WriteFile(manifest,
					[]byte(strings.Repeat("0", 64)+"  Makefile\n"), 0o600); err != nil {
					t.Fatalf("write mismatched source manifest: %v", err)
				}
				rewriteSessionRevocationPackageManifest(t, packageDir)
			},
		},
		{
			name: "additional archive path",
			mutate: func(t *testing.T, _ string, packageDir string) {
				extraRoot := t.TempDir()
				writeIdempotencyAcceptanceFixtureFile(t, extraRoot, "backend/unexpected.go", "package backend\n", 0o600)
				command := exec.Command("tar", "-rf", filepath.Join(packageDir, "source.tar"),
					"-C", extraRoot, "backend/unexpected.go")
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("append unexpected archive path: %v: %s", err, output)
				}
				rewriteSessionRevocationPackageManifest(t, packageDir)
			},
		},
	}

	for _, tc := range tamperCases {
		t.Run(tc.name, func(t *testing.T) {
			remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
			if tc.mutate != nil {
				tc.mutate(t, remoteRepo, packageDir)
			}
			dockerMarker := filepath.Join(t.TempDir(), "docker-called")
			stubDir := writeIdempotencyAcceptanceDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
			var output []byte
			var err error
			if tc.digest == "" {
				output, err = runMetadataFreeSessionRevocationAcceptance(t,
					remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
			} else {
				output, err = runMetadataFreeSessionRevocationAcceptanceWithDigest(t,
					remoteRepo, packageDir, remoteScript, stubDir, dockerMarker, tc.digest, nil)
			}
			if err == nil {
				t.Fatalf("tampered metadata-free package unexpectedly succeeded: %q", output)
			}
			if _, err := os.Stat(dockerMarker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("tampered metadata-free package reached Docker; output = %q", output)
			}
			evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "session-access-revocation")
			if _, err := os.Stat(evidenceDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("provenance preflight failure retained evidence")
			}
		})
	}
}

func TestSessionRevocationAcceptanceRefusesEvidenceAndProjectReuse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, string, string)
	}{
		{
			name: "existing evidence",
			setup: func(t *testing.T, evidenceDir, _ string) {
				if err := os.MkdirAll(evidenceDir, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "dangling evidence symlink",
			setup: func(t *testing.T, evidenceDir, _ string) {
				if err := os.MkdirAll(filepath.Dir(evidenceDir), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(filepath.Dir(evidenceDir), "missing-evidence"), evidenceDir); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "stale publication lock",
			setup: func(t *testing.T, _, lock string) {
				if err := os.MkdirAll(lock, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "dangling publication lock",
			setup: func(t *testing.T, evidenceDir, lock string) {
				if err := os.MkdirAll(filepath.Dir(evidenceDir), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(filepath.Dir(evidenceDir), "missing-lock"), lock); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name+" refuses before Docker", func(t *testing.T) {
			remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
			evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "session-access-revocation")
			lock := filepath.Join(filepath.Dir(evidenceDir), ".session-access-revocation.publish.lock")
			tc.setup(t, evidenceDir, lock)
			dockerMarker := filepath.Join(t.TempDir(), "docker-called")
			stubDir := writeIdempotencyAcceptanceDockerStub(t, "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n")
			if output, err := runMetadataFreeSessionRevocationAcceptance(t,
				remoteRepo, packageDir, remoteScript, stubDir, dockerMarker); err == nil {
				t.Fatalf("%s unexpectedly succeeded: %q", tc.name, output)
			}
			if _, err := os.Stat(dockerMarker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s reached Docker", tc.name)
			}
		})
	}

	for _, resource := range []string{"container", "volume", "network"} {
		t.Run("existing "+resource, func(t *testing.T) {
			remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
			composeMarker := filepath.Join(t.TempDir(), "compose-called")
			stub := "#!/bin/sh\n" +
				"case \"$1 $2\" in\n" +
				"  \"" + resource + " ls\") printf 'existing-resource\\n'; exit 0 ;;\n" +
				"  \"compose \"*) : >\"$COMPOSE_CALLED\"; exit 99 ;;\n" +
				"esac\nexit 0\n"
			stubDir := writeIdempotencyAcceptanceDockerStub(t, stub)
			output, err := runMetadataFreeSessionRevocationAcceptanceWithDigest(t,
				remoteRepo, packageDir, remoteScript, stubDir, filepath.Join(t.TempDir(), "docker-called"),
				sessionRevocationPackageDigest(t, packageDir), []string{"COMPOSE_CALLED=" + composeMarker})
			if err == nil || !strings.Contains(string(output), "refusing to reuse existing secondhand-session-revocation-acceptance resources") {
				t.Fatalf("existing %s did not fail with stable collision: %v: %q", resource, err, output)
			}
			if _, err := os.Stat(composeMarker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("existing %s reached Compose", resource)
			}
		})
	}
}

func TestSessionRevocationAcceptanceRetainsSanitizedFailureEvidence(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
	dockerMarker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := writeIdempotencyAcceptanceDockerStub(t, `#!/bin/sh
: >>"$DOCKER_CALLED"
args=" $* "
case "$args" in
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect --type container "*) exit 1 ;;
  *" compose "*" stop "*|*" compose "*" up "*|*" compose "*" build "*) exit 0 ;;
  *" compose "*" exec "*)
    case "$args" in *"SELECT VERSION()"*) printf '8.4.0\n' ;; esac
    exit 0
    ;;
  *" compose "*" run "*)
    printf 'Authorization: Bearer raw-session-secret\n' >&2
    exit 42
    ;;
esac
exit 0
`)
	output, err := runMetadataFreeSessionRevocationAcceptance(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
	if err == nil {
		t.Fatal("controlled focused-test failure unexpectedly succeeded")
	}
	if _, err := os.Stat(dockerMarker); err != nil {
		t.Fatalf("controlled failure did not exercise fake Docker: %v", err)
	}

	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "session-access-revocation")
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		t.Fatalf("read retained failure evidence: %v; output = %q", err, output)
	}
	allowed := map[string]bool{
		"acceptance-results.txt": true,
		"evidence-leak-scan.txt": true,
		"evidence-sha256.txt":    true,
		"failure-status.txt":     true,
		"production-before.txt":  true,
		"production-after.txt":   true,
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
	for _, required := range []string{
		"classification=acceptance_failure|result=FAIL|stage=session_auto_migrate_false|count=1",
		"classification=source_package|result=PASS|count=",
		"classification=mysql_version|result=PASS|count=1",
		"classification=migration_chain|result=PASS|count=1",
		"classification=evidence_scan|result=PASS|count=0",
	} {
		if !strings.Contains(retained.String(), required) {
			t.Fatalf("retained failure evidence omitted %q", required)
		}
	}
	for _, forbidden := range []string{"Authorization", "Bearer", "raw-session-secret", "TestSessionRevocation"} {
		if strings.Contains(retained.String(), forbidden) {
			t.Fatalf("retained failure evidence leaked %q", forbidden)
		}
	}
	before, err := os.ReadFile(filepath.Join(evidenceDir, "production-before.txt"))
	if err != nil {
		t.Fatalf("read production-before snapshot: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(evidenceDir, "production-after.txt"))
	if err != nil {
		t.Fatalf("read production-after snapshot: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("failure production snapshots differ: before=%q after=%q", before, after)
	}
	hashCheck := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
	hashCheck.Dir = evidenceDir
	if output, err := hashCheck.CombinedOutput(); err != nil {
		t.Fatalf("verify retained failure evidence hashes: %v: %s", err, output)
	}
}

func TestSessionRevocationAcceptanceFailsClosedOnEvidenceScannerError(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
	dockerMarker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := writeIdempotencyAcceptanceDockerStub(t, sessionRevocationControlledFailureDockerStub)
	writeIdempotencyAcceptanceFixtureFile(t, stubDir, "grep", `#!/bin/sh
case " $* " in
  *" -ERn "*) exit 2 ;;
esac
exec /usr/bin/grep "$@"
`, 0o700)
	output, err := runMetadataFreeSessionRevocationAcceptance(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
	if err == nil {
		t.Fatal("evidence scanner error unexpectedly succeeded")
	}
	assertSessionRevocationSanitizationFallback(t, remoteRepo, output)
}

func TestSessionRevocationAcceptancePublicationFailureLeavesNoPartialEvidence(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
	dockerMarker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := writeIdempotencyAcceptanceDockerStub(t, sessionRevocationControlledFailureDockerStub)
	writeIdempotencyAcceptanceFixtureFile(t, stubDir, "chmod", `#!/bin/sh
case " $* " in
  *"/evidence-candidate/"*".txt"*|*".session-access-revocation."*".txt"*) exit 74 ;;
esac
exec /bin/chmod "$@"
`, 0o700)
	if output, err := runMetadataFreeSessionRevocationAcceptance(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker); err == nil {
		t.Fatalf("forced evidence publication failure unexpectedly succeeded: %s", output)
	}
	assertSessionRevocationNoPublicationState(t, remoteRepo)
}

func TestSessionRevocationAcceptanceRejectsStagedEvidenceTamper(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
	dockerMarker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := writeIdempotencyAcceptanceDockerStub(t, sessionRevocationControlledFailureDockerStub)
	writeIdempotencyAcceptanceFixtureFile(t, stubDir, "tar", `#!/bin/sh
previous=
destination=
for argument in "$@"; do
  [ "$previous" != -C ] || destination="$argument"
  previous="$argument"
done
case "$destination| $* " in
  *.session-access-revocation.publish.*'| '*' -xf - '*)
    /usr/bin/tar "$@" || exit $?
    printf 'unexpected staged evidence\n' >"$destination/unexpected.txt"
    exit 0 ;;
esac
exec /usr/bin/tar "$@"
`, 0o700)
	output, err := runMetadataFreeSessionRevocationAcceptance(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
	if err == nil {
		t.Fatalf("staged evidence tamper unexpectedly succeeded: %s", output)
	}
	assertSessionRevocationNoPublicationState(t, remoteRepo)
}

func TestSessionRevocationAcceptancePreservesPostRenamePublicationAmbiguity(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
	dockerMarker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := writeIdempotencyAcceptanceDockerStub(t, sessionRevocationControlledFailureDockerStub)
	writeIdempotencyAcceptanceFixtureFile(t, stubDir, "mv", `#!/bin/sh
case " $* " in
  *" -n "*.session-access-revocation.publish.*) ;;
  *) exec /bin/mv "$@" ;;
esac
while [ "$#" -gt 0 ]; do case "$1" in -n|--) shift;; *) break;; esac; done
source=$1
target=$2
lock="$(dirname "$target")/.session-access-revocation.publish.lock"
[ -d "$lock" ] || exit 91
/bin/mv "$source" "$target" || exit $?
printf 'classification=post_rename_tamper|result=PASS|count=1\n' >>"$target/acceptance-results.txt"
`, 0o700)
	output, err := runMetadataFreeSessionRevocationAcceptance(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
	if err == nil {
		t.Fatalf("post-rename mutation unexpectedly succeeded: %s", output)
	}
	assertSessionRevocationAmbiguousPublicationPreserved(t, remoteRepo, output)
}

func TestSessionRevocationAcceptancePreservesPublicationLockReleaseFailure(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
	dockerMarker := filepath.Join(t.TempDir(), "docker-called")
	stubDir := writeIdempotencyAcceptanceDockerStub(t, sessionRevocationControlledFailureDockerStub)
	writeIdempotencyAcceptanceFixtureFile(t, stubDir, "rmdir", `#!/bin/sh
case "$1" in
  *.session-access-revocation.publish.lock)
    target="$(dirname "$1")/session-access-revocation"
    [ -d "$target" ] || exit 92
    : >"$0.called"
    exit 73 ;;
esac
exec /bin/rmdir "$@"
`, 0o700)
	output, err := runMetadataFreeSessionRevocationAcceptance(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
	if err == nil {
		t.Fatalf("lock-release failure unexpectedly succeeded: %s", output)
	}
	if _, err := os.Stat(filepath.Join(stubDir, "rmdir.called")); err != nil {
		t.Fatalf("publication lock release was not attempted after rename: %v", err)
	}
	assertSessionRevocationAmbiguousPublicationPreserved(t, remoteRepo, output)
	tripwire := filepath.Join(t.TempDir(), "docker-called")
	if _, err := runMetadataFreeSessionRevocationAcceptance(t,
		remoteRepo, packageDir, remoteScript, stubDir, tripwire); err == nil {
		t.Fatal("preserved publication state allowed a later invocation")
	}
	if _, err := os.Stat(tripwire); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preserved publication state reached Docker")
	}
}

func TestSessionRevocationAcceptanceRefusesProductionInspectionError(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
	stateDir := t.TempDir()
	dockerMarker := filepath.Join(stateDir, "docker-called")
	stubDir := writeIdempotencyAcceptanceDockerStub(t, `#!/bin/sh
: >>"$DOCKER_CALLED"
args=" $* "
case "$args" in
  *" container ls "*"name=^/secondhand-market-api$"*) printf 'secondhand-market-api\n'; exit 0 ;;
  *" container ls "*"name=^/secondhand-market-"*) exit 0 ;;
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect --type container "*" secondhand-market-api "*) printf 'production-inspection-secret\n' >&2; exit 125 ;;
  *" compose "*" up "*) exit 0 ;;
  *" compose "*" exec "*)
    case "$args" in *"SELECT VERSION()"*) printf '8.4.0\n' ;; esac
    exit 0 ;;
  *" compose "*" run "*"TestSessionRevocationMySQLAcceptance"*)
    printf '%s\n' '--- PASS: TestSessionRevocationMySQLAcceptance (0.01s)' 'PASS' 'ok  fixture/tests 0.01s'
    exit 0 ;;
  *" compose "*" run "*"go test ./..."*) printf 'ok  fixture/all 0.01s\n'; exit 0 ;;
  *" compose "*" run "*"go vet ./..."*) exit 0 ;;
  *" compose "*) exit 0 ;;
esac
exit 0
`)
	output, err := runMetadataFreeSessionRevocationAcceptance(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker)
	if err == nil {
		t.Fatalf("production inspection error produced a false PASS: %s", output)
	}
	if bytes.Contains(output, []byte("production-inspection-secret")) {
		t.Fatalf("production inspection diagnostics leaked to caller: %q", output)
	}
	if bytes.Contains(output, []byte("classification=production_snapshot|result=PASS")) {
		t.Fatalf("production inspection error published normal PASS evidence: %q", output)
	}
	assertSessionRevocationSanitizationFallback(t, remoteRepo, output)
}

func TestSessionRevocationAcceptancePreservesRuntimeGateOrder(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeSessionRevocationAcceptance(t)
	stateDir := t.TempDir()
	dockerLog := filepath.Join(stateDir, "docker.log")
	cmpLog := filepath.Join(stateDir, "cmp.log")
	projectStarted := filepath.Join(stateDir, "project-started")
	stubDir := writeIdempotencyAcceptanceDockerStub(t, `#!/bin/sh
printf '%s\n' "$*" >>"$DOCKER_LOG"
args=" $* "
case "$args" in
  *" container ls "*"name=^/secondhand-market-"*) exit 0 ;;
  *" container ls "*)
    [ -f "$PROJECT_STARTED" ] && printf 'isolated-container\n'
    exit 0
    ;;
  *" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect --type container "*) exit 1 ;;
  *" compose "*" up "*) : >"$PROJECT_STARTED"; exit 0 ;;
  *" compose "*" exec "*)
    case "$args" in *"SELECT VERSION()"*) printf '8.4.0\n' ;; esac
    exit 0
    ;;
  *" compose "*" run "*"TestSessionRevocationMySQLAcceptance"*)
    printf '%s\n' '--- PASS: TestSessionRevocationMySQLAcceptance (0.01s)' 'PASS' 'ok  fixture/tests 0.01s'
    exit 0
    ;;
  *" compose "*" run "*"go test ./..."*) printf 'ok  fixture/all 0.01s\n'; exit 0 ;;
  *" compose "*" run "*"go vet ./..."*) exit 0 ;;
  *" compose "*) exit 0 ;;
esac
exit 0
`)
	writeIdempotencyAcceptanceFixtureFile(t, stubDir, "cmp", `#!/bin/sh
case " $* " in
  *"production-before.txt "*"production-after.txt "*) printf '%s\n' "$*" >>"$CMP_LOG" ;;
esac
exec /usr/bin/cmp "$@"
`, 0o700)
	output, err := runMetadataFreeSessionRevocationAcceptanceWithDigest(t,
		remoteRepo, packageDir, remoteScript, stubDir, filepath.Join(stateDir, "docker-called"),
		sessionRevocationPackageDigest(t, packageDir), []string{
			"DOCKER_LOG=" + dockerLog,
			"CMP_LOG=" + cmpLog,
			"PROJECT_STARTED=" + projectStarted,
		})
	if err != nil {
		t.Fatalf("run complete stubbed session revocation matrix: %v: %s", err, output)
	}
	rawLog, err := os.ReadFile(dockerLog)
	if err != nil {
		t.Fatalf("read Docker command log: %v", err)
	}
	logText := string(rawLog)
	expectedMigrationChain := []string{
		"0001_init.up.sql",
		"0002_buyer_domain.up.sql",
		"0003_buyer_auth_provider.up.sql",
		"0004_merchant_multi_stock.preflight.sql",
		"0004_merchant_multi_stock.up.sql",
		"0004_merchant_multi_stock.postflight.sql",
		"0005_file_records_table.preflight.sql",
		"0005_file_records_table.up.sql",
		"0005_file_records_table.postflight.sql",
		"0006_file_binding_ownership.preflight.sql",
		"0006_file_binding_ownership.up.sql",
		"0006_file_binding_ownership.postflight.sql",
		"0007_license_file_privacy.preflight.sql",
		"0007_license_file_privacy.up.sql",
		"0007_license_file_privacy.postflight.sql",
		"0008_anonymous_upload_governance.preflight.sql",
		"0008_anonymous_upload_governance.up.sql",
		"0008_anonymous_upload_governance.postflight.sql",
		"0009_buyer_intent_open_uniqueness.preflight.sql",
		"0009_buyer_intent_open_uniqueness.up.sql",
		"0009_buyer_intent_open_uniqueness.postflight.sql",
	}
	ordered := []string{
		"name=^/secondhand-market-api$",
		"compose --project-name secondhand-session-revocation-acceptance",
		" up -d --wait mysql",
		"SELECT VERSION()",
		"DROP TABLE IF EXISTS file_quota_guards",
	}
	ordered = append(ordered, expectedMigrationChain...)
	ordered = append(ordered,
		"AUTO_MIGRATE=false",
		"DROP TABLE IF EXISTS file_quota_guards",
	)
	ordered = append(ordered, expectedMigrationChain...)
	ordered = append(ordered,
		"AUTO_MIGRATE=true",
		"go test ./... -count=1",
		"go vet ./...",
		"name=^/secondhand-market-api$",
		" stop",
	)
	requireOrderedSessionSnippets(t, logText, ordered)
	for _, migration := range expectedMigrationChain {
		if count := strings.Count(logText, migration); count != 2 {
			t.Fatalf("migration %q count = %d, want 2: %s", migration, count, logText)
		}
	}
	if count := strings.Count(logText, "name=^/secondhand-market-api$"); count != 2 {
		t.Fatalf("production API snapshot count = %d, want 2: %s", count, logText)
	}
	if count := strings.Count(logText, "DROP TABLE IF EXISTS file_quota_guards"); count != 2 {
		t.Fatalf("clean schema reset count = %d, want 2: %s", count, logText)
	}
	cmpBytes, err := os.ReadFile(cmpLog)
	if err != nil {
		t.Fatalf("read production snapshot comparison log: %v", err)
	}
	cmpLines := strings.Split(strings.TrimSpace(string(cmpBytes)), "\n")
	if len(cmpLines) != 2 {
		t.Fatalf("production snapshot byte comparison count = %d, want final and publication checks: %q", len(cmpLines), cmpBytes)
	}
	for _, line := range cmpLines {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "-s" ||
			!strings.HasSuffix(fields[1], "/production-before.txt") ||
			!strings.HasSuffix(fields[2], "/production-after.txt") {
			t.Fatalf("production snapshot byte comparison = %q, want cmp -s before after", line)
		}
	}
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "session-access-revocation")
	results, err := os.ReadFile(filepath.Join(evidenceDir, "acceptance-results.txt"))
	if err != nil {
		t.Fatalf("read success evidence classifications: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(results)), "\n")
	if len(lines) != 8 || !strings.HasPrefix(lines[0], "classification=source_package|result=PASS|count=") {
		t.Fatalf("success evidence classifications = %q", results)
	}
	for index, expected := range []string{
		"classification=mysql_version|result=PASS|count=1",
		"classification=migration_chain|result=PASS|count=1",
		"classification=session_auto_migrate_false|result=PASS|count=1",
		"classification=session_auto_migrate_true|result=PASS|count=1",
		"classification=backend_tests|result=PASS|count=1",
		"classification=go_vet|result=PASS|count=1",
		"classification=production_snapshot|result=PASS|count=3",
	} {
		if lines[index+1] != expected {
			t.Fatalf("success classification %d = %q, want %q", index+1, lines[index+1], expected)
		}
	}
	before, err := os.ReadFile(filepath.Join(evidenceDir, "production-before.txt"))
	if err != nil {
		t.Fatalf("read production-before evidence: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(evidenceDir, "production-after.txt"))
	if err != nil {
		t.Fatalf("read production-after evidence: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("published production snapshots differ: before=%q after=%q", before, after)
	}
	runtimePath := ""
	for _, field := range strings.Fields(logText) {
		if strings.HasSuffix(field, "/session-revocation-compose.yml") {
			runtimePath = filepath.Dir(field)
			break
		}
	}
	if runtimePath == "" {
		t.Fatal("runtime Compose override path was not recorded")
	}
	if _, err := os.Lstat(runtimePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary runtime directory survived: %s: %v", runtimePath, err)
	}
	for _, forbidden := range []string{" down", " prune", "secondhand-market-api stop", "secondhand-market-web stop", "secondhand-market-mysql stop"} {
		if strings.Contains(logText, forbidden) {
			t.Fatalf("runtime used forbidden Docker operation %q: %s", forbidden, logText)
		}
	}
}

const sessionRevocationControlledFailureDockerStub = `#!/bin/sh
: >>"$DOCKER_CALLED"
args=" $* "
case "$args" in
  *" container ls "*"name=^/secondhand-market-"*) exit 0 ;;
  *" container ls "*|*" volume ls "*|*" network ls "*) exit 0 ;;
  *" inspect --type container "*) exit 1 ;;
  *" compose "*" stop "*|*" compose "*" up "*|*" compose "*" build "*) exit 0 ;;
  *" compose "*" exec "*)
    case "$args" in *"SELECT VERSION()"*) printf '8.4.0\n' ;; esac
    exit 0 ;;
  *" compose "*" run "*)
    printf 'Authorization: Bearer raw-session-secret\n' >&2
    exit 42 ;;
esac
exit 0
`

func assertSessionRevocationSanitizationFallback(t *testing.T, remoteRepo string, commandOutput []byte) {
	t.Helper()
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "session-access-revocation")
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		t.Fatalf("read sanitization fallback: %v; output = %q", err, commandOutput)
	}
	if len(entries) != 3 {
		t.Fatalf("sanitization fallback entries = %v, want exactly three", entries)
	}
	results, err := os.ReadFile(filepath.Join(evidenceDir, "acceptance-results.txt"))
	if err != nil || string(results) != "classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n" {
		t.Fatalf("sanitization fallback result = %q, %v", results, err)
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

func assertSessionRevocationNoPublicationState(t *testing.T, remoteRepo string) {
	t.Helper()
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "session-access-revocation")
	if _, err := os.Lstat(evidenceDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial retained evidence survived: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(evidenceDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".session-access-revocation.") {
			t.Fatalf("partial sibling publication state survived: %q", entry.Name())
		}
	}
}

func assertSessionRevocationAmbiguousPublicationPreserved(t *testing.T, remoteRepo string, output []byte) {
	t.Helper()
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "session-access-revocation")
	lock := filepath.Join(filepath.Dir(evidenceDir), ".session-access-revocation.publish.lock")
	if info, err := os.Stat(evidenceDir); err != nil || !info.IsDir() {
		t.Fatalf("ambiguous retained evidence was removed: %v; output=%q", err, output)
	}
	if info, err := os.Stat(lock); err != nil || !info.IsDir() {
		t.Fatalf("ambiguous publication lock was removed: %v; output=%q", err, output)
	}
}

func prepareMetadataFreeSessionRevocationAcceptance(t *testing.T) (string, string, string) {
	t.Helper()
	fixtureRepo, fixtureScript := newSessionRevocationAcceptanceFixtureRepo(t)
	remoteRepo := t.TempDir()
	packageDir := filepath.Join(remoteRepo, ".session-revocation-source")
	exportCmd := exec.Command("/bin/bash", fixtureScript)
	exportCmd.Dir = fixtureRepo
	exportCmd.Env = []string{
		"SESSION_REVOCATION_SOURCE_EXPORT_DIR=" + packageDir,
		"PATH=" + os.Getenv("PATH"),
	}
	if output, err := exportCmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare metadata-free session revocation package: %v: %s", err, output)
	}
	extractIdempotencyAcceptanceTar(t, filepath.Join(packageDir, "source.tar"), remoteRepo)
	writeIdempotencyAcceptanceFixtureFile(t, remoteRepo, "deploy/acceptance/.env",
		"MYSQL_DATABASE=second_hand_market_acceptance\n", 0o600)
	return remoteRepo, packageDir,
		filepath.Join(remoteRepo, "deploy", "acceptance", "session-revocation-smoke.sh")
}

func runMetadataFreeSessionRevocationAcceptance(
	t *testing.T,
	remoteRepo string,
	packageDir string,
	remoteScript string,
	stubDir string,
	dockerMarker string,
) ([]byte, error) {
	t.Helper()
	return runMetadataFreeSessionRevocationAcceptanceWithDigest(t,
		remoteRepo, packageDir, remoteScript, stubDir, dockerMarker,
		sessionRevocationPackageDigest(t, packageDir), nil)
}

func runMetadataFreeSessionRevocationAcceptanceWithDigest(
	t *testing.T,
	remoteRepo string,
	packageDir string,
	remoteScript string,
	stubDir string,
	dockerMarker string,
	manifestDigest string,
	extraEnv []string,
) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("/bin/bash", remoteScript)
	cmd.Dir = remoteRepo
	cmd.Env = append([]string{
		"SESSION_REVOCATION_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA",
		"ACCEPTANCE_DB_ENGINE=mysql8.4",
		"COMPOSE_PROJECT_NAME=secondhand-session-revocation-acceptance",
		"SESSION_REVOCATION_SOURCE_PACKAGE_DIR=" + packageDir,
		"SESSION_REVOCATION_SOURCE_PACKAGE_MANIFEST_SHA256=" + manifestDigest,
		"DOCKER_CALLED=" + dockerMarker,
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}, extraEnv...)
	return cmd.CombinedOutput()
}

func sessionRevocationPackageDigest(t *testing.T, packageDir string) string {
	t.Helper()
	manifest, err := os.ReadFile(filepath.Join(packageDir, "package-sha256.txt"))
	if err != nil {
		t.Fatalf("read session package manifest: %v", err)
	}
	digest := sha256.Sum256(manifest)
	return hex.EncodeToString(digest[:])
}

func rewriteSessionRevocationSourceList(
	t *testing.T,
	packageDir string,
	rewrite func([]string) []string,
) {
	t.Helper()
	listPath := filepath.Join(packageDir, "source-files.z")
	raw, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("read session source list: %v", err)
	}
	paths := rewrite(splitNULPaths(t, raw))
	var output bytes.Buffer
	for _, path := range paths {
		output.WriteString(path)
		output.WriteByte(0)
	}
	if err := os.WriteFile(listPath, output.Bytes(), 0o600); err != nil {
		t.Fatalf("rewrite session source list: %v", err)
	}
	rewriteSessionRevocationPackageManifest(t, packageDir)
}

func removeSessionRevocationPath(paths []string, remove string) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != remove {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func rewriteSessionRevocationPackageManifest(t *testing.T, packageDir string) {
	t.Helper()
	var manifest strings.Builder
	for _, name := range []string{"source-files.z", "source-sha256.txt", "source.tar"} {
		raw, err := os.ReadFile(filepath.Join(packageDir, name))
		if err != nil {
			t.Fatalf("read package artifact %q: %v", name, err)
		}
		digest := sha256.Sum256(raw)
		manifest.WriteString(hex.EncodeToString(digest[:]))
		manifest.WriteString("  ")
		manifest.WriteString(name)
		manifest.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(packageDir, "package-sha256.txt"),
		[]byte(manifest.String()), 0o600); err != nil {
		t.Fatalf("rewrite package checksum manifest: %v", err)
	}
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
