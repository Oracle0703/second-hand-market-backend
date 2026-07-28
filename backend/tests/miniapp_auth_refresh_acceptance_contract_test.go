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

const miniappAuthRefreshConfirmation = "I_UNDERSTAND_THIS_RUNS_ONLY_ISOLATED_MINIAPP_TESTS"

func TestMiniappAuthRefreshAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T) {
	repoDir := miniappAuthRefreshAcceptanceRepoDir(t)
	script := filepath.Join(repoDir, "deploy", "acceptance", "miniapp-auth-refresh-smoke.sh")
	stubDir, marker := miniappAuthRefreshTripwires(t)
	cmd := exec.Command("/bin/bash", script)
	cmd.Dir = repoDir
	cmd.Env = []string{
		"MINIAPP_AUTH_REFRESH_SOURCE_LIST_ONLY=1",
		"TRIPWIRE_MARKER=" + marker,
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run miniapp source-list mode: %v: %s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("source-list mode wrote stderr: %q", stderr.String())
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source-list mode invoked a node, npm, or network tripwire")
	}

	paths := splitMiniappAuthRefreshNULPaths(t, stdout.Bytes())
	present := make(map[string]bool, len(paths))
	for _, path := range paths {
		if present[path] {
			t.Fatalf("source-list mode emitted duplicate path %q", path)
		}
		present[path] = true
		if !miniappAuthRefreshAllowedPath(path) {
			t.Fatalf("source-list mode emitted path outside the miniapp whitelist: %q", path)
		}
		headCheck := exec.Command("git", "cat-file", "-e", "HEAD:"+path)
		headCheck.Dir = repoDir
		if output, err := headCheck.CombinedOutput(); err != nil {
			t.Fatalf("source-list path %q is not committed at HEAD: %v: %s", path, err, output)
		}
	}
	for _, path := range miniappAuthRefreshRequiredPaths {
		if !present[path] {
			t.Errorf("source-list mode omitted required committed path %q", path)
		}
	}

	fixtureRepo, fixtureScript := newMiniappAuthRefreshFixtureRepo(t)
	for _, path := range []string{
		"miniapp/src/control\nname.ts",
		"miniapp/src/back\\slash.ts",
		"miniapp/src/nonportable-\u2603.ts",
	} {
		writeMiniappAuthRefreshFixtureFile(t, fixtureRepo, path, "export {}\n", 0o600)
	}
	runMiniappAuthRefreshGit(t, fixtureRepo, "add", "--", "miniapp/src/control\nname.ts", "miniapp/src/back\\slash.ts", "miniapp/src/nonportable-\u2603.ts")
	runMiniappAuthRefreshGit(t, fixtureRepo, "commit", "-q", "-m", "nonportable committed names")
	for _, path := range []string{
		"miniapp/project.private.config.json",
		"miniapp/.swc/cache.bin",
		"miniapp/dist/app.js",
		"miniapp/node_modules/untrusted/index.js",
		"miniapp/.env",
		"miniapp/.cache/cache.bin",
		"miniapp/staged-only.ts",
	} {
		writeMiniappAuthRefreshFixtureFile(t, fixtureRepo, path, "forbidden\n", 0o600)
	}
	writeMiniappAuthRefreshFixtureFile(t, fixtureRepo, "miniapp/src/dirty-only.ts", "dirty\n", 0o600)
	runMiniappAuthRefreshGit(t, fixtureRepo, "add", "--", "miniapp/staged-only.ts")
	fixtureCmd := exec.Command("/bin/bash", fixtureScript)
	fixtureCmd.Dir = fixtureRepo
	fixtureCmd.Env = []string{"MINIAPP_AUTH_REFRESH_SOURCE_LIST_ONLY=1", "PATH=" + os.Getenv("PATH")}
	fixtureOutput, err := fixtureCmd.Output()
	if err != nil {
		t.Fatalf("run fixture source-list mode: %v", err)
	}
	fixturePaths := splitMiniappAuthRefreshNULPaths(t, fixtureOutput)
	for _, path := range fixturePaths {
		if !miniappAuthRefreshAllowedPath(path) {
			t.Fatalf("source-list mode admitted forbidden, staged, dirty, or untracked path %q", path)
		}
	}
	joinedFixturePaths := strings.Join(fixturePaths, "\n")
	for _, forbidden := range []string{"control\nname.ts", "back\\slash.ts", "nonportable-"} {
		if strings.Contains(joinedFixturePaths, forbidden) {
			t.Fatalf("source-list mode admitted non-portable committed path containing %q: %q", forbidden, joinedFixturePaths)
		}
	}
}

func TestMiniappAuthRefreshAcceptanceSourceExportUsesImmutableHEAD(t *testing.T) {
	fixtureRepo, fixtureScript := newMiniappAuthRefreshFixtureRepo(t)
	writeMiniappAuthRefreshFixtureFile(t, fixtureRepo, "miniapp/package.json", "{\"name\":\"dirty\"}\n", 0o600)
	writeMiniappAuthRefreshFixtureFile(t, fixtureRepo, "miniapp/staged-only.ts", "staged\n", 0o600)
	runMiniappAuthRefreshGit(t, fixtureRepo, "add", "--", "miniapp/staged-only.ts")

	stubDir, marker := miniappAuthRefreshTripwires(t)
	exportDir := filepath.Join(t.TempDir(), "source-package")
	cmd := exec.Command("/bin/bash", fixtureScript)
	cmd.Dir = fixtureRepo
	cmd.Env = []string{
		"MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR=" + exportDir,
		"TRIPWIRE_MARKER=" + marker,
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export immutable HEAD miniapp package: %v: %s", err, output)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source export invoked a node, npm, or network tripwire")
	}
	entries, err := os.ReadDir(exportDir)
	if err != nil || len(entries) != 4 {
		t.Fatalf("source export must contain exactly four artifacts: %v, %v", entries, err)
	}
	for _, name := range []string{"source-files.z", "source-sha256.txt", "source.tar", "package-sha256.txt"} {
		if info, err := os.Lstat(filepath.Join(exportDir, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("source export artifact %q must be a regular file: %v", name, err)
		} else if info.Mode().Perm() != 0o600 {
			t.Fatalf("source export artifact %q mode = %o, want 0600", name, info.Mode().Perm())
		}
	}
	packageManifest, err := os.ReadFile(filepath.Join(exportDir, "package-sha256.txt"))
	if err != nil {
		t.Fatal(err)
	}
	manifestLines := strings.Split(strings.TrimSuffix(string(packageManifest), "\n"), "\n")
	if len(manifestLines) != 3 || !strings.HasSuffix(manifestLines[0], "  source-files.z") ||
		!strings.HasSuffix(manifestLines[1], "  source-sha256.txt") ||
		!strings.HasSuffix(manifestLines[2], "  source.tar") {
		t.Fatalf("package manifest has noncanonical names or order: %q", packageManifest)
	}
	rawList, err := os.ReadFile(filepath.Join(exportDir, "source-files.z"))
	if err != nil {
		t.Fatalf("read exported source list: %v", err)
	}
	for _, path := range splitMiniappAuthRefreshNULPaths(t, rawList) {
		if !miniappAuthRefreshAllowedPath(path) {
			t.Fatalf("source export admitted path outside the whitelist: %q", path)
		}
	}
	extracted := t.TempDir()
	extractMiniappAuthRefreshTar(t, filepath.Join(exportDir, "source.tar"), extracted)
	gotPackage, err := os.ReadFile(filepath.Join(extracted, "miniapp", "package.json"))
	if err != nil {
		t.Fatalf("read exported package.json: %v", err)
	}
	if string(gotPackage) != "{\"name\":\"fixture-miniapp\"}\n" {
		t.Fatalf("exported package.json = %q, want committed HEAD bytes", gotPackage)
	}
	if _, err := os.Stat(filepath.Join(extracted, "miniapp", "staged-only.ts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source export included staged-only path")
	}
	manifestCheck := exec.Command("sha256sum", "-c", filepath.Join(exportDir, "source-sha256.txt"))
	manifestCheck.Dir = extracted
	if output, err := manifestCheck.CombinedOutput(); err != nil {
		t.Fatalf("verify exported miniapp source manifest: %v: %s", err, output)
	}

	for _, tc := range []struct{ name, destination string }{
		{"relative", "source-package"},
		{"root", "/"},
		{"pre-existing", t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := exec.Command("/bin/bash", fixtureScript)
			command.Dir = fixtureRepo
			command.Env = []string{"MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR=" + tc.destination, "PATH=" + os.Getenv("PATH")}
			if err := command.Run(); err == nil {
				t.Fatalf("source export destination %q unexpectedly succeeded", tc.destination)
			}
		})
	}
	command := exec.Command("/bin/bash", fixtureScript)
	command.Dir = fixtureRepo
	command.Env = []string{
		"MINIAPP_AUTH_REFRESH_SOURCE_LIST_ONLY=1",
		"MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR=" + filepath.Join(t.TempDir(), "both"),
		"PATH=" + os.Getenv("PATH"),
	}
	if err := command.Run(); err == nil {
		t.Fatal("combined source modes unexpectedly succeeded")
	}

	t.Run("final chmod failure removes incomplete export", func(t *testing.T) {
		stubDir := t.TempDir()
		writeMiniappAuthRefreshFixtureFile(t, stubDir, "chmod", `#!/bin/sh
case " $* " in
  *"source-files.z "*) exit 73;;
esac
exec /bin/chmod "$@"
`, 0o700)
		destination := filepath.Join(t.TempDir(), "interrupted-package")
		command := exec.Command("/bin/bash", fixtureScript)
		command.Dir = fixtureRepo
		command.Env = []string{
			"MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR=" + destination,
			"PATH=" + stubDir + ":" + os.Getenv("PATH"),
		}
		if output, err := command.CombinedOutput(); err == nil {
			t.Fatalf("source export unexpectedly survived final chmod failure: %s", output)
		}
		if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed source export retained incomplete destination: %v", err)
		}
	})
}

func TestMiniappAuthRefreshAcceptanceMetadataFreePackageRefusesOrProgressesBeforeNPM(t *testing.T) {
	t.Run("missing explicit package directory refuses before node or npm", func(t *testing.T) {
		remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
		stubDir, marker, commandLog := writeMiniappAuthRefreshRuntimeStubs(t, "success")
		nodeMarker := miniappAuthRefreshNodeMarker(stubDir)
		cmd := exec.Command("/bin/bash", remoteScript)
		cmd.Dir = remoteRepo
		cmd.Env = []string{
			"MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM=" + miniappAuthRefreshConfirmation,
			"MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_MANIFEST_SHA256=" + miniappAuthRefreshPackageDigest(t, packageDir),
			"NODE_CALLED=" + nodeMarker,
			"NPM_CALLED=" + marker,
			"NPM_COMMAND_LOG=" + commandLog,
			"NPM_STUB_MODE_FILE=" + filepath.Join(stubDir, "mode"),
			"PATH=" + stubDir + ":" + os.Getenv("PATH"),
		}
		output, err := cmd.CombinedOutput()
		if err == nil || !errors.Is(func() error { _, statErr := os.Stat(marker); return statErr }(), os.ErrNotExist) {
			t.Fatalf("missing explicit package directory reached node or npm: %v: %q", err, output)
		}
		if _, err := os.Stat(nodeMarker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing explicit package directory reached node: %v: %q", err, output)
		}
	})

	t.Run("missing package manifest digest refuses before node or npm", func(t *testing.T) {
		remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
		stubDir, marker, commandLog := writeMiniappAuthRefreshRuntimeStubs(t, "success")
		nodeMarker := miniappAuthRefreshNodeMarker(stubDir)
		cmd := exec.Command("/bin/bash", remoteScript)
		cmd.Dir = remoteRepo
		cmd.Env = []string{
			"MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM=" + miniappAuthRefreshConfirmation,
			"MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_DIR=" + packageDir,
			"NODE_CALLED=" + nodeMarker,
			"NPM_CALLED=" + marker,
			"NPM_COMMAND_LOG=" + commandLog,
			"NPM_STUB_MODE_FILE=" + filepath.Join(stubDir, "mode"),
			"PATH=" + stubDir + ":" + os.Getenv("PATH"),
		}
		output, err := cmd.CombinedOutput()
		if err == nil || !errors.Is(func() error { _, statErr := os.Stat(marker); return statErr }(), os.ErrNotExist) {
			t.Fatalf("missing package manifest digest reached node or npm: %v: %q", err, output)
		}
		if _, err := os.Stat(nodeMarker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing package manifest digest reached node: %v: %q", err, output)
		}
	})

	t.Run("valid package reaches npm after exact toolchain checks", func(t *testing.T) {
		remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
		stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "fail-ci")
		output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker)
		if err == nil {
			t.Fatal("fake npm failure unexpectedly allowed acceptance to succeed")
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("valid metadata-free package did not reach npm: %v; output = %q", err, output)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, remoteRepo, packageDir string)
	}{
		{
			name:   "wrong authorized package digest",
			mutate: func(t *testing.T, _ string, _ string) {},
		},
		{
			name: "extra package file",
			mutate: func(t *testing.T, _ string, packageDir string) {
				writeMiniappAuthRefreshFixtureFile(t, packageDir, "unexpected.txt", "unexpected\n", 0o600)
			},
		},
		{
			name: "extra package directory",
			mutate: func(t *testing.T, _ string, packageDir string) {
				if err := os.Mkdir(filepath.Join(packageDir, "unexpected"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "noncanonical package manifest whitespace",
			mutate: func(t *testing.T, _ string, packageDir string) {
				miniappAuthRefreshWritePackageManifest(t, packageDir, " ")
			},
		},
		{
			name: "tampered source list",
			mutate: func(t *testing.T, _ string, packageDir string) {
				appendMiniappAuthRefreshFixtureFile(t, packageDir, "source-files.z", []byte("miniapp/src/tampered.ts\x00"))
			},
		},
		{
			name: "tampered source archive",
			mutate: func(t *testing.T, _ string, packageDir string) {
				appendMiniappAuthRefreshFixtureFile(t, packageDir, "source.tar", []byte("tampered"))
			},
		},
		{
			name: "tampered received file",
			mutate: func(t *testing.T, remoteRepo, _ string) {
				writeMiniappAuthRefreshFixtureFile(t, remoteRepo, "miniapp/package.json", "{\"tampered\":true}\n", 0o600)
			},
		},
		{
			name: "missing package artifact",
			mutate: func(t *testing.T, _ string, packageDir string) {
				if err := os.Remove(filepath.Join(packageDir, "source.tar")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink package artifact",
			mutate: func(t *testing.T, _ string, packageDir string) {
				if err := os.Remove(filepath.Join(packageDir, "source.tar")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("source-files.z", filepath.Join(packageDir, "source.tar")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "received source symlink",
			mutate: func(t *testing.T, remoteRepo, _ string) {
				if err := os.Remove(filepath.Join(remoteRepo, "Makefile")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("miniapp/package.json", filepath.Join(remoteRepo, "Makefile")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "archive extra file",
			mutate: func(t *testing.T, _ string, packageDir string) {
				miniappAuthRefreshRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "", "miniapp/src/archive-extra.ts", "")
				miniappAuthRefreshWritePackageManifest(t, packageDir, "  ")
			},
		},
		{
			name: "archive missing file",
			mutate: func(t *testing.T, _ string, packageDir string) {
				miniappAuthRefreshRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "Makefile", "", "")
				miniappAuthRefreshWritePackageManifest(t, packageDir, "  ")
			},
		},
		{
			name: "archive symlink entry",
			mutate: func(t *testing.T, _ string, packageDir string) {
				miniappAuthRefreshRewriteArchive(t, filepath.Join(packageDir, "source.tar"), "", "", "Makefile")
				miniappAuthRefreshWritePackageManifest(t, packageDir, "  ")
			},
		},
		{
			name: "unsorted source list",
			mutate: func(t *testing.T, _ string, packageDir string) {
				paths := miniappAuthRefreshReadSourcePaths(t, packageDir)
				paths[0], paths[1] = paths[1], paths[0]
				miniappAuthRefreshWriteSourcePaths(t, packageDir, paths)
			},
		},
		{
			name: "duplicate source list",
			mutate: func(t *testing.T, _ string, packageDir string) {
				paths := miniappAuthRefreshReadSourcePaths(t, packageDir)
				paths = append(paths, paths[0])
				sort.Strings(paths)
				miniappAuthRefreshWriteSourcePaths(t, packageDir, paths)
			},
		},
		{
			name: "forbidden source list",
			mutate: func(t *testing.T, _ string, packageDir string) {
				paths := append(miniappAuthRefreshReadSourcePaths(t, packageDir), "miniapp/.env")
				sort.Strings(paths)
				miniappAuthRefreshWriteSourcePaths(t, packageDir, paths)
			},
		},
		{
			name: "missing required source path",
			mutate: func(t *testing.T, _ string, packageDir string) {
				paths := miniappAuthRefreshReadSourcePaths(t, packageDir)
				miniappAuthRefreshWriteSourcePaths(t, packageDir, paths[1:])
			},
		},
		{
			name: "mismatched per-file hash",
			mutate: func(t *testing.T, _ string, packageDir string) {
				manifestPath := filepath.Join(packageDir, "source-sha256.txt")
				raw, err := os.ReadFile(manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				copy(raw[:64], strings.Repeat("0", 64))
				if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
					t.Fatal(err)
				}
				miniappAuthRefreshWritePackageManifest(t, packageDir, "  ")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
			tc.mutate(t, remoteRepo, packageDir)
			stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "success")
			digest := miniappAuthRefreshPackageDigest(t, packageDir)
			if tc.name == "wrong authorized package digest" {
				digest = strings.Repeat("0", 64)
			}
			output, err := runMetadataFreeMiniappAuthRefreshWithDigest(t, remoteRepo, packageDir, remoteScript, stubDir, marker, digest)
			if err == nil {
				t.Fatalf("%s unexpectedly succeeded", tc.name)
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s reached npm before refusing; output = %q", tc.name, output)
			}
			if _, err := os.Stat(miniappAuthRefreshNodeMarker(stubDir)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s reached node before refusing; output = %q", tc.name, output)
			}
		})
	}
}

func TestMiniappAuthRefreshAcceptancePublishesEvidenceAtomically(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "fail-ci")
	writeMiniappAuthRefreshFixtureFile(t, stubDir, "tar", `#!/bin/sh
if [ "$*" = "-cf - ." ]; then exit 74; fi
exec /usr/bin/tar "$@"
`, 0o700)
	if output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker); err == nil {
		t.Fatalf("forced publication failure unexpectedly succeeded: %s", output)
	}
	retained := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "miniapp-auth-refresh")
	if _, err := os.Lstat(retained); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial retained evidence survived publication failure: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(retained))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "miniapp-auth-refresh.publish.") {
			t.Fatalf("partial sibling publication directory survived: %q", entry.Name())
		}
	}
}

func TestMiniappAuthRefreshAcceptanceRejectsStagedEvidenceTamper(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "fail-ci")
	writeMiniappAuthRefreshFixtureFile(t, stubDir, "tar", `#!/bin/sh
previous=
destination=
for argument in "$@"; do
  [ "$previous" != -C ] || destination="$argument"
  previous="$argument"
done
case "$destination| $* " in
  *miniapp-auth-refresh.publish.*'| '*' -xf - '*)
    /usr/bin/tar "$@" || exit $?
    printf 'unexpected staged evidence\n' >"$destination/unexpected.txt"
    exit 0 ;;
esac
exec /usr/bin/tar "$@"
`, 0o700)
	if output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker); err == nil {
		t.Fatalf("staged evidence tamper unexpectedly succeeded: %s", output)
	}
	assertMiniappAuthRefreshNoPublicationState(t, remoteRepo)
}

func TestMiniappAuthRefreshAcceptanceRejectsPublicationCollision(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "fail-ci")
	writeMiniappAuthRefreshFixtureFile(t, stubDir, "mv", `#!/bin/sh
case " $* " in
  *miniapp-auth-refresh.publish.*)
    while [ "$#" -gt 0 ]; do
      case "$1" in -n|--) shift ;; *) break ;; esac
    done
    source=$1
    target=$2
    case "$source|$target" in
      *miniapp-auth-refresh.publish.*'|'*/miniapp-auth-refresh)
        [ -d "${target}.publish.lock" ] || exit 91
        mkdir "$target" || exit $?
        printf 'concurrent-owner\n' >"$target/concurrent-owner.txt" || exit $?
        exec /bin/mv "$source" "$target" ;;
    esac ;;
esac
exec /bin/mv "$@"
`, 0o700)
	if output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker); err == nil {
		t.Fatalf("publication collision unexpectedly succeeded: %s", output)
	}
	assertMiniappAuthRefreshConcurrentPublicationPreserved(t, remoteRepo)
}

func TestMiniappAuthRefreshAcceptancePreservesPostRenamePublicationAmbiguity(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "fail-ci")
	writeMiniappAuthRefreshFixtureFile(t, stubDir, "mv", `#!/bin/sh
while [ "$#" -gt 0 ]; do case "$1" in -n|--) shift;; *) break;; esac; done
source=$1
target=$2
[ -d "${target}.publish.lock" ] || exit 91
/bin/mv "$source" "$target" || exit $?
printf 'classification=post_rename_tamper|result=PASS|count=1\n' >>"$target/acceptance-results.txt"
`, 0o700)
	if output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker); err == nil {
		t.Fatalf("post-rename mutation unexpectedly succeeded: %s", output)
	}
	assertMiniappAuthRefreshAmbiguousPublicationPreserved(t, remoteRepo)
}

func TestMiniappAuthRefreshAcceptancePreservesPublicationLockReleaseFailure(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "fail-ci")
	writeMiniappAuthRefreshFixtureFile(t, stubDir, "rmdir", `#!/bin/sh
case "$1" in
  *.publish.lock)
    target=${1%.publish.lock}
    [ -d "$target" ] || exit 92
    : >"$0.called"
    exit 73 ;;
esac
exec /bin/rmdir "$@"
`, 0o700)
	if output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker); err == nil {
		t.Fatalf("lock-release failure unexpectedly succeeded: %s", output)
	}
	if _, err := os.Stat(filepath.Join(stubDir, "rmdir.called")); err != nil {
		t.Fatalf("publication lock release was not attempted after rename: %v", err)
	}
	assertMiniappAuthRefreshAmbiguousPublicationPreserved(t, remoteRepo)
	tripwire := filepath.Join(t.TempDir(), "npm-called")
	if _, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, tripwire); err == nil {
		t.Fatal("preserved publication state allowed a later invocation")
	}
	if _, err := os.Stat(tripwire); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preserved publication state reached npm")
	}
}

func TestMiniappAuthRefreshAcceptanceFailsClosedOnMalformedCheckpoint(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "fail-ci")
	writeMiniappAuthRefreshFixtureFile(t, stubDir, "npm", `#!/bin/sh
if [ "${1:-}" = "--version" ]; then printf '10.9.7\n'; exit 0; fi
: >"$NPM_CALLED"
printf 'classification=npm_ci|result=PASS|count=999\n' >>"$PWD/../../evidence/acceptance-results.txt"
exit 42
`, 0o700)
	if output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker); err == nil {
		t.Fatalf("malformed checkpoint failure unexpectedly succeeded: %s", output)
	}
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "miniapp-auth-refresh")
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		t.Fatalf("read sanitization fallback: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("sanitization fallback entries = %v, want exactly three files", entries)
	}
	results, err := os.ReadFile(filepath.Join(evidenceDir, "acceptance-results.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(results) != "classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1\n" {
		t.Fatalf("sanitization fallback results = %q", results)
	}
	leakScan, err := os.ReadFile(filepath.Join(evidenceDir, "evidence-leak-scan.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(leakScan) != "classification=evidence_scan|result=FAIL|count=1\n" {
		t.Fatalf("sanitization fallback leak scan = %q", leakScan)
	}
}

func TestMiniappAuthRefreshAcceptanceRefusesExistingEvidenceBeforeNPM(t *testing.T) {
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
			remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
			evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "miniapp-auth-refresh")
			tc.setup(t, evidenceDir)
			stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "success")
			nodeMarker := miniappAuthRefreshNodeMarker(stubDir)
			output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker)
			if err == nil || !strings.Contains(string(output), "refusing to overwrite existing miniapp auth refresh evidence") {
				t.Fatalf("%s did not produce a stable refusal: %v: %q", tc.name, err, output)
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s reached npm", tc.name)
			}
			if _, err := os.Stat(nodeMarker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s reached node", tc.name)
			}
		})
	}
}

func TestMiniappAuthRefreshAcceptanceUsesTemporaryVerifiedTree(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	before := miniappAuthRefreshDirectoryDigest(t, filepath.Join(remoteRepo, "miniapp"))
	stubDir, marker, commandLog := writeMiniappAuthRefreshRuntimeStubs(t, "fail-ci")
	output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker)
	if err == nil {
		t.Fatal("controlled npm failure unexpectedly succeeded")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("controlled failure did not reach npm: %v; output = %q", err, output)
	}
	rawLog, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatalf("read npm command log: %v", err)
	}
	var runtimeDir string
	for _, line := range strings.Split(strings.TrimSpace(string(rawLog)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 || !strings.HasSuffix(parts[0], "/build-context/miniapp") {
			t.Fatalf("npm ran outside the temporary verified miniapp tree: %q", line)
		}
		if strings.HasPrefix(parts[0], filepath.Join(remoteRepo, "miniapp")) {
			t.Fatalf("npm ran inside the received source tree: %q", line)
		}
		candidateRuntimeDir := filepath.Dir(filepath.Dir(parts[0]))
		if runtimeDir == "" {
			runtimeDir = candidateRuntimeDir
		} else if runtimeDir != candidateRuntimeDir {
			t.Fatalf("npm commands used multiple runtime directories: %q and %q", runtimeDir, candidateRuntimeDir)
		}
	}
	if runtimeDir == "" {
		t.Fatal("npm command log did not identify a temporary runtime directory")
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(runtimeDir); err != nil {
			t.Errorf("clean temporary runtime fixture %q: %v", runtimeDir, err)
		}
	})
	if _, err := os.Lstat(runtimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("controlled failure retained temporary runtime directory %q: %v", runtimeDir, err)
	}
	after := miniappAuthRefreshDirectoryDigest(t, filepath.Join(remoteRepo, "miniapp"))
	if after != before {
		t.Fatal("received miniapp source changed after temporary runtime execution")
	}
	for _, path := range []string{"node_modules", "dist"} {
		if _, err := os.Stat(filepath.Join(remoteRepo, "miniapp", path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("received source retained generated path %q", path)
		}
	}
}

func TestMiniappAuthRefreshAcceptanceRetainsSanitizedFailureEvidence(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "fail-ci")
	output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker)
	if err == nil {
		t.Fatal("controlled npm failure unexpectedly succeeded")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("controlled failure did not reach npm: %v; output = %q", err, output)
	}
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "miniapp-auth-refresh")
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		t.Fatalf("read retained failure evidence: %v; command output = %q", err, output)
	}
	allowed := map[string]bool{
		"acceptance-results.txt": true,
		"evidence-leak-scan.txt": true,
		"evidence-sha256.txt":    true,
		"failure-status.txt":     true,
	}
	var retained bytes.Buffer
	for _, entry := range entries {
		if entry.IsDir() || !allowed[entry.Name()] {
			t.Fatalf("retained failure evidence contains unexpected entry %q", entry.Name())
		}
		raw, err := os.ReadFile(filepath.Join(evidenceDir, entry.Name()))
		if err != nil {
			t.Fatalf("read retained evidence %q: %v", entry.Name(), err)
		}
		retained.Write(raw)
	}
	retainedText := retained.String()
	for _, required := range []string{
		"classification=source_package|result=PASS|count=",
		"classification=toolchain|result=PASS|count=2",
		"classification=acceptance_failure|result=FAIL|stage=npm_ci|count=1",
		"classification=evidence_scan|result=PASS|count=0",
	} {
		if !strings.Contains(retainedText, required) {
			t.Fatalf("retained failure evidence omitted %q", required)
		}
	}
	for _, forbidden := range []string{"Authorization", "Bearer", "raw-miniapp-secret", "node_modules", "dist"} {
		if strings.Contains(retainedText, forbidden) {
			t.Fatalf("retained failure evidence leaked %q", forbidden)
		}
	}
	hashCheck := exec.Command("sha256sum", "-c", "evidence-sha256.txt")
	hashCheck.Dir = evidenceDir
	if output, err := hashCheck.CombinedOutput(); err != nil {
		t.Fatalf("verify retained evidence hashes: %v: %s", err, output)
	}
}

func TestMiniappAuthRefreshAcceptancePreservesCommandMatrix(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	stubDir, marker, commandLog := writeMiniappAuthRefreshRuntimeStubs(t, "success")
	output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker)
	if err != nil {
		t.Fatalf("run controlled successful miniapp acceptance: %v: %s", err, output)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("successful acceptance did not invoke npm: %v", err)
	}
	rawLog, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatalf("read npm command log: %v", err)
	}
	var commands []string
	var runtimeDir string
	for _, line := range strings.Split(strings.TrimSpace(string(rawLog)), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			t.Fatalf("malformed command log entry %q", line)
		}
		if parts[2] != "https://example.invalid/api/v1" {
			t.Fatalf("npm command did not receive isolated example.invalid API URL: %q", line)
		}
		candidateRuntimeDir := filepath.Dir(filepath.Dir(parts[0]))
		if runtimeDir == "" {
			runtimeDir = candidateRuntimeDir
		} else if runtimeDir != candidateRuntimeDir {
			t.Fatalf("npm commands used multiple runtime directories: %q and %q", runtimeDir, candidateRuntimeDir)
		}
		commands = append(commands, parts[1])
	}
	want := []string{
		"ci --registry=https://registry.npmmirror.com --replace-registry-host=always",
		"test -- --run tests/request-refresh.test.ts",
		"test",
		"run build:weapp",
		"run build:tt",
	}
	if strings.Join(commands, "\n") != strings.Join(want, "\n") {
		t.Fatalf("npm command matrix = %q, want %q", commands, want)
	}
	if runtimeDir == "" {
		t.Fatal("successful npm command log did not identify a temporary runtime directory")
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(runtimeDir); err != nil {
			t.Errorf("clean temporary runtime fixture %q: %v", runtimeDir, err)
		}
	})
	if _, err := os.Lstat(runtimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful run retained temporary runtime directory %q: %v", runtimeDir, err)
	}
}

var miniappAuthRefreshRequiredPaths = []string{
	"Makefile",
	"miniapp/.nvmrc",
	"miniapp/package.json",
	"miniapp/package-lock.json",
	"miniapp/project.config.json",
	"miniapp/project.tt.json",
	"miniapp/src/services/request.ts",
	"miniapp/tests/request-refresh.test.ts",
	"deploy/acceptance/miniapp-auth-refresh-smoke.sh",
}

func miniappAuthRefreshAllowedPath(path string) bool {
	if path == "Makefile" || path == "deploy/acceptance/miniapp-auth-refresh-smoke.sh" {
		return true
	}
	for _, prefix := range []string{"miniapp/config/", "miniapp/src/", "miniapp/tests/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	for _, allowed := range []string{
		"miniapp/.nvmrc", "miniapp/babel.config.js", "miniapp/package.json", "miniapp/package-lock.json",
		"miniapp/project.config.json", "miniapp/project.tt.json", "miniapp/tsconfig.json", "miniapp/vitest.config.mjs",
	} {
		if path == allowed {
			return true
		}
	}
	return false
}

func miniappAuthRefreshAcceptanceRepoDir(t *testing.T) string {
	t.Helper()
	repoDir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repository directory: %v", err)
	}
	return repoDir
}

func newMiniappAuthRefreshFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	fixtureRepo := t.TempDir()
	realScript := filepath.Join(miniappAuthRefreshAcceptanceRepoDir(t), "deploy", "acceptance", "miniapp-auth-refresh-smoke.sh")
	scriptBytes, err := os.ReadFile(realScript)
	if err != nil {
		t.Fatalf("read real miniapp acceptance script: %v", err)
	}
	fixtureFiles := map[string]string{
		"Makefile":                              "fixture:\n\t@true\n",
		"miniapp/.nvmrc":                        "v22.22.2\n",
		"miniapp/babel.config.js":               "module.exports = {}\n",
		"miniapp/config/index.ts":               "export default {}\n",
		"miniapp/package.json":                  "{\"name\":\"fixture-miniapp\"}\n",
		"miniapp/package-lock.json":             "{\"name\":\"fixture-miniapp\",\"lockfileVersion\":3}\n",
		"miniapp/project.config.json":           "{}\n",
		"miniapp/project.tt.json":               "{}\n",
		"miniapp/src/services/request.ts":       "export {}\n",
		"miniapp/tests/request-refresh.test.ts": "export {}\n",
		"miniapp/tsconfig.json":                 "{}\n",
		"miniapp/vitest.config.mjs":             "export default {}\n",
	}
	for path, content := range fixtureFiles {
		writeMiniappAuthRefreshFixtureFile(t, fixtureRepo, path, content, 0o600)
	}
	fixtureScript := filepath.Join(fixtureRepo, "deploy", "acceptance", "miniapp-auth-refresh-smoke.sh")
	writeMiniappAuthRefreshFixtureFile(t, fixtureRepo, "deploy/acceptance/miniapp-auth-refresh-smoke.sh", string(scriptBytes), 0o700)
	runMiniappAuthRefreshGit(t, fixtureRepo, "init", "-q")
	runMiniappAuthRefreshGit(t, fixtureRepo, "add", "--", ".")
	runMiniappAuthRefreshGit(t, fixtureRepo, "-c", "user.name=Acceptance Contract", "-c", "user.email=acceptance-contract@example.invalid", "commit", "-q", "-m", "fixture")
	return fixtureRepo, fixtureScript
}

func writeMiniappAuthRefreshFixtureFile(t *testing.T, repoDir, path, content string, mode os.FileMode) {
	t.Helper()
	fullPath := filepath.Join(repoDir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatalf("create fixture parent for %q: %v", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), mode); err != nil {
		t.Fatalf("write fixture file %q: %v", path, err)
	}
}

func runMiniappAuthRefreshGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func splitMiniappAuthRefreshNULPaths(t *testing.T, raw []byte) []string {
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

func miniappAuthRefreshTripwires(t *testing.T) (string, string) {
	t.Helper()
	stubDir := t.TempDir()
	marker := filepath.Join(stubDir, "tripwire-called")
	for _, name := range []string{"node", "npm", "curl", "wget"} {
		stub := "#!/bin/sh\n: >\"$TRIPWIRE_MARKER\"\nexit 99\n"
		if err := os.WriteFile(filepath.Join(stubDir, name), []byte(stub), 0o700); err != nil {
			t.Fatalf("write %s tripwire: %v", name, err)
		}
	}
	return stubDir, marker
}

func extractMiniappAuthRefreshTar(t *testing.T, archivePath, destination string) {
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

func miniappAuthRefreshPackageDigest(t *testing.T, packageDir string) string {
	t.Helper()
	manifest, err := os.ReadFile(filepath.Join(packageDir, "package-sha256.txt"))
	if err != nil {
		t.Fatalf("read source package manifest: %v", err)
	}
	digest := sha256.Sum256(manifest)
	return hex.EncodeToString(digest[:])
}

func miniappAuthRefreshReadSourcePaths(t *testing.T, packageDir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(packageDir, "source-files.z"))
	if err != nil {
		t.Fatal(err)
	}
	return splitMiniappAuthRefreshNULPaths(t, raw)
}

func miniappAuthRefreshWriteSourcePaths(t *testing.T, packageDir string, paths []string) {
	t.Helper()
	var raw bytes.Buffer
	for _, path := range paths {
		raw.WriteString(path)
		raw.WriteByte(0)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "source-files.z"), raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	miniappAuthRefreshWritePackageManifest(t, packageDir, "  ")
}

func miniappAuthRefreshWritePackageManifest(t *testing.T, packageDir, separator string) {
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

func miniappAuthRefreshRewriteArchive(t *testing.T, archivePath, omit, extra, symlink string) {
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
			cloned.Linkname = "miniapp/package.json"
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
		contents := []byte("export {}\n")
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

func prepareMetadataFreeMiniappAuthRefresh(t *testing.T) (string, string, string) {
	t.Helper()
	fixtureRepo, fixtureScript := newMiniappAuthRefreshFixtureRepo(t)
	remoteRepo := t.TempDir()
	packageDir := filepath.Join(remoteRepo, ".miniapp-auth-refresh-source")
	exportCmd := exec.Command("/bin/bash", fixtureScript)
	exportCmd.Dir = fixtureRepo
	exportCmd.Env = []string{
		"MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR=" + packageDir,
		"PATH=" + os.Getenv("PATH"),
	}
	if output, err := exportCmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare metadata-free miniapp source package: %v: %s", err, output)
	}
	extractMiniappAuthRefreshTar(t, filepath.Join(packageDir, "source.tar"), remoteRepo)
	if _, err := os.Stat(filepath.Join(remoteRepo, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("metadata-free fixture unexpectedly contains Git metadata")
	}
	return remoteRepo, packageDir,
		filepath.Join(remoteRepo, "deploy", "acceptance", "miniapp-auth-refresh-smoke.sh")
}

func appendMiniappAuthRefreshFixtureFile(t *testing.T, directory, name string, data []byte) {
	t.Helper()
	file, err := os.OpenFile(filepath.Join(directory, name), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open fixture %q for append: %v", name, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		t.Fatalf("append fixture %q: %v", name, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close fixture %q: %v", name, err)
	}
}

func writeMiniappAuthRefreshRuntimeStubs(t *testing.T, mode string) (string, string, string) {
	t.Helper()
	stubDir := t.TempDir()
	marker := filepath.Join(stubDir, "npm-called")
	commandLog := filepath.Join(stubDir, "npm-commands")
	node := `#!/bin/sh
: >"$NODE_CALLED"
case "${1:-}" in
  --version) printf 'v22.22.2\n' ;;
  *) exit 91 ;;
esac
`
	npm := `#!/bin/sh
if [ "${1:-}" = "--version" ]; then
  printf '10.9.7\n'
  exit 0
fi
: >"$NPM_CALLED"
printf '%s|%s|%s\n' "$PWD" "$*" "${TARO_APP_API_BASE_URL:-}" >>"$NPM_COMMAND_LOG"
mkdir -p node_modules/controlled dist
printf 'Authorization: Bearer raw-miniapp-secret\n' >raw-command.log
case "$(cat "$NPM_STUB_MODE_FILE")" in
  fail-ci)
    [ "${1:-}" = "ci" ] && exit 42
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(stubDir, "node"), []byte(node), 0o700); err != nil {
		t.Fatalf("write node stub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "npm"), []byte(npm), 0o700); err != nil {
		t.Fatalf("write npm stub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "mode"), []byte(mode+"\n"), 0o600); err != nil {
		t.Fatalf("write npm stub mode: %v", err)
	}
	return stubDir, marker, commandLog
}

func miniappAuthRefreshNodeMarker(stubDir string) string {
	return filepath.Join(stubDir, "node-called")
}

func assertMiniappAuthRefreshNoPublicationState(t *testing.T, remoteRepo string) {
	t.Helper()
	retained := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "miniapp-auth-refresh")
	if _, err := os.Lstat(retained); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial retained evidence survived: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(retained))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "miniapp-auth-refresh.publish.") || entry.Name() == "miniapp-auth-refresh.publish.lock" {
			t.Fatalf("partial sibling publication state survived: %q", entry.Name())
		}
	}
}

func assertMiniappAuthRefreshConcurrentPublicationPreserved(t *testing.T, remoteRepo string) {
	t.Helper()
	retained := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "miniapp-auth-refresh")
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
		if strings.HasPrefix(entry.Name(), "miniapp-auth-refresh.publish.") || entry.Name() == "miniapp-auth-refresh.publish.lock" {
			t.Fatalf("partial sibling publication state survived: %q", entry.Name())
		}
	}
}

func assertMiniappAuthRefreshAmbiguousPublicationPreserved(t *testing.T, remoteRepo string) {
	t.Helper()
	retained := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "miniapp-auth-refresh")
	if info, err := os.Stat(retained); err != nil || !info.IsDir() {
		t.Fatalf("ambiguous retained evidence was removed: %v", err)
	}
	if info, err := os.Stat(retained + ".publish.lock"); err != nil || !info.IsDir() {
		t.Fatalf("ambiguous publication lock was removed: %v", err)
	}
}

func runMetadataFreeMiniappAuthRefresh(
	t *testing.T,
	remoteRepo, packageDir, remoteScript, stubDir, marker string,
) ([]byte, error) {
	t.Helper()
	return runMetadataFreeMiniappAuthRefreshWithDigest(t, remoteRepo, packageDir, remoteScript, stubDir,
		marker, miniappAuthRefreshPackageDigest(t, packageDir))
}

func runMetadataFreeMiniappAuthRefreshWithDigest(
	t *testing.T,
	remoteRepo, packageDir, remoteScript, stubDir, marker, manifestDigest string,
) ([]byte, error) {
	t.Helper()
	commandLog := filepath.Join(stubDir, "npm-commands")
	stubMode := filepath.Join(stubDir, "mode")
	cmd := exec.Command("/bin/bash", remoteScript)
	cmd.Dir = remoteRepo
	cmd.Env = []string{
		"MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM=" + miniappAuthRefreshConfirmation,
		"MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_DIR=" + packageDir,
		"MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_MANIFEST_SHA256=" + manifestDigest,
		"NODE_CALLED=" + miniappAuthRefreshNodeMarker(stubDir),
		"NPM_CALLED=" + marker,
		"NPM_COMMAND_LOG=" + commandLog,
		"NPM_STUB_MODE_FILE=" + stubMode,
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
	}
	return cmd.CombinedOutput()
}

func miniappAuthRefreshDirectoryDigest(t *testing.T, directory string) string {
	t.Helper()
	cmd := exec.Command("/usr/bin/find", ".", "-type", "f", "-exec", "sha256sum", "{}", ";")
	cmd.Dir = directory
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("hash source directory: %v", err)
	}
	digest := sha256.Sum256(output)
	return hex.EncodeToString(digest[:])
}
