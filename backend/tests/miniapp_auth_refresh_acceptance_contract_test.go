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
	for _, path := range splitMiniappAuthRefreshNULPaths(t, fixtureOutput) {
		if !miniappAuthRefreshAllowedPath(path) {
			t.Fatalf("source-list mode admitted forbidden, staged, dirty, or untracked path %q", path)
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
	for _, name := range []string{"source-files.z", "source-sha256.txt", "source.tar", "package-sha256.txt"} {
		if info, err := os.Lstat(filepath.Join(exportDir, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("source export artifact %q must be a regular file: %v", name, err)
		}
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
}

func TestMiniappAuthRefreshAcceptanceMetadataFreePackageRefusesOrProgressesBeforeNPM(t *testing.T) {
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
		})
	}
}

func TestMiniappAuthRefreshAcceptanceRefusesExistingEvidenceBeforeNPM(t *testing.T) {
	remoteRepo, packageDir, remoteScript := prepareMetadataFreeMiniappAuthRefresh(t)
	evidenceDir := filepath.Join(remoteRepo, "deploy", "acceptance", "evidence", "miniapp-auth-refresh")
	if err := os.MkdirAll(evidenceDir, 0o700); err != nil {
		t.Fatalf("create retained evidence fixture: %v", err)
	}
	stubDir, marker, _ := writeMiniappAuthRefreshRuntimeStubs(t, "success")
	output, err := runMetadataFreeMiniappAuthRefresh(t, remoteRepo, packageDir, remoteScript, stubDir, marker)
	if err == nil || !strings.Contains(string(output), "refusing to overwrite existing miniapp auth refresh evidence") {
		t.Fatalf("existing evidence did not produce a stable refusal: %v: %q", err, output)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("existing retained evidence reached node or npm")
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
	for _, line := range strings.Split(strings.TrimSpace(string(rawLog)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 || !strings.HasSuffix(parts[0], "/build-context/miniapp") {
			t.Fatalf("npm ran outside the temporary verified miniapp tree: %q", line)
		}
		if strings.HasPrefix(parts[0], filepath.Join(remoteRepo, "miniapp")) {
			t.Fatalf("npm ran inside the received source tree: %q", line)
		}
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
	for _, line := range strings.Split(strings.TrimSpace(string(rawLog)), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			t.Fatalf("malformed command log entry %q", line)
		}
		if parts[2] != "https://example.invalid/api/v1" {
			t.Fatalf("npm command did not receive isolated example.invalid API URL: %q", line)
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
