package migrations

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

const buyerIntentAcceptanceConfirmation = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA"

func TestBuyerIntentOpenUniquenessAcceptanceHarnessContract(t *testing.T) {
	scriptPath := "../../deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh"
	scriptInfo, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat acceptance harness: %v", err)
	}
	if !scriptInfo.Mode().IsRegular() || scriptInfo.Mode()&0o111 == 0 {
		t.Fatal("acceptance harness must be a regular executable file")
	}
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read acceptance harness: %v", err)
	}
	for _, required := range []string{
		"BUYER_INTENT_ACCEPTANCE_CONFIRM",
		buyerIntentAcceptanceConfirmation,
		"BUYER_INTENT_SOURCE_LIST_ONLY",
		"secondhand-buyer-intent-acceptance",
		"evidence/buyer-intent-open-uniqueness",
		"ACCEPTANCE_DB_ENGINE=mysql8.4",
		"MySQL 8.4 version check",
		"0009_buyer_intent_open_uniqueness.preflight.sql",
		"0009_buyer_intent_open_uniqueness.up.sql",
		"0009_buyer_intent_open_uniqueness.postflight.sql",
		"legacy",
		"marker-only",
		"both-key",
		"final-rerun",
		"invalid-state",
		"duplicate-open",
		"drifted-marker",
		"drifted-key",
		"ERROR 1644 (45000)",
		"before/after row-summary comparisons",
		"BUYER_INTENT_MYSQL_TEST=1",
		"AUTO_MIGRATE=false",
		"AUTO_MIGRATE=true",
		"go test ./...",
		"go test -race ./...",
		"go vet ./...",
		"source-sha256.txt",
		"evidence-sha256.txt",
		"production-before.txt",
		"production-after.txt",
		"resource retention marker",
	} {
		if !strings.Contains(string(script), required) {
			t.Errorf("acceptance harness missing %q", required)
		}
	}

	makefile, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	for _, required := range []string{
		"acceptance-buyer-intent-smoke:",
		"BUYER_INTENT_ACCEPTANCE_CONFIRM",
		buyerIntentAcceptanceConfirmation,
		"ACCEPTANCE_DB_ENGINE",
		"mysql8.4",
		"./deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh",
	} {
		if !strings.Contains(string(makefile), required) {
			t.Errorf("Makefile acceptance target missing %q", required)
		}
	}

	readme, err := os.ReadFile("../../deploy/acceptance/README.md")
	if err != nil {
		t.Fatalf("read acceptance README: %v", err)
	}
	for _, required := range []string{
		"/home/yu/services/secondhand-buyer-intent-acceptance-20260727",
		"secondhand-buyer-intent-acceptance",
		"acceptance-buyer-intent-smoke",
		"evidence/buyer-intent-open-uniqueness",
		"production-before.txt",
		"production-after.txt",
		"does not execute production 0009",
	} {
		if !strings.Contains(string(readme), required) {
			t.Errorf("acceptance README missing %q", required)
		}
	}
}

func TestBuyerIntentOpenUniquenessAcceptanceRejectsUnsafeEnvironmentBeforeDocker(t *testing.T) {
	script := "../../deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh"
	stubDir := t.TempDir()
	dockerCalled := filepath.Join(stubDir, "docker-called")
	dockerStub := filepath.Join(stubDir, "docker")
	stub := "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n"
	if err := os.WriteFile(dockerStub, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, confirm, engine, project string
	}{
		{name: "missing confirmation", engine: "mysql8.4"},
		{name: "wrong confirmation", confirm: "unsafe", engine: "mysql8.4"},
		{name: "wrong engine", confirm: buyerIntentAcceptanceConfirmation, engine: "mysql8.0"},
		{name: "wrong project", confirm: buyerIntentAcceptanceConfirmation, engine: "mysql8.4", project: "secondhand-market"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Remove(dockerCalled); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/bash", script)
			cmd.Env = []string{
				"PATH=" + stubDir + ":/usr/bin:/bin",
				"DOCKER_CALLED=" + dockerCalled,
				"BUYER_INTENT_ACCEPTANCE_CONFIRM=" + tc.confirm,
				"ACCEPTANCE_DB_ENGINE=" + tc.engine,
				"COMPOSE_PROJECT_NAME=" + tc.project,
			}
			if err := cmd.Run(); err == nil {
				t.Fatal("unsafe acceptance environment succeeded")
			}
			if _, err := os.Stat(dockerCalled); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unsafe environment reached Docker")
			}
		})
	}
}

func TestBuyerIntentOpenUniquenessAcceptanceManifestModeIsReadOnly(t *testing.T) {
	script := "../../deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh"
	stubDir := t.TempDir()
	dockerCalled := filepath.Join(stubDir, "docker-called")
	dockerStub := filepath.Join(stubDir, "docker")
	if err := os.WriteFile(dockerStub, []byte(
		"#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n",
	), 0o700); err != nil {
		t.Fatal(err)
	}
	sha256sum, err := exec.LookPath("sha256sum")
	if err != nil {
		t.Fatal("sha256sum is required for the manifest contract")
	}
	utilityPath := stubDir + ":" + filepath.Dir(sha256sum) +
		":/usr/bin:/bin:/usr/sbin:/sbin"
	cmd := exec.Command("/bin/bash", script)
	cmd.Env = []string{
		"PATH=" + utilityPath,
		"DOCKER_CALLED=" + dockerCalled,
		"BUYER_INTENT_SOURCE_MANIFEST_ONLY=1",
	}
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("manifest-only mode: %v", err)
	}
	if _, err := os.Stat(dockerCalled); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("manifest-only mode reached Docker")
	}
	text := string(raw)
	for _, required := range []string{
		"  Makefile\n",
		"  backend/internal/app/server.go\n",
		"  backend/migrations/0009_buyer_intent_open_uniqueness.up.sql\n",
		"  deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh\n",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("manifest missing %q", required)
		}
	}
	var paths []string
	linePattern := regexp.MustCompile(`^[0-9a-f]{64}  (.+)$`)
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		match := linePattern.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("invalid manifest line %q", line)
		}
		paths = append(paths, match[1])
	}
	want := append([]string(nil), paths...)
	sort.Strings(want)
	if !slices.Equal(paths, want) {
		t.Fatal("source manifest paths are not sorted")
	}
	listCmd := exec.Command("/bin/bash", script)
	listCmd.Env = []string{
		"PATH=" + utilityPath,
		"DOCKER_CALLED=" + dockerCalled,
		"BUYER_INTENT_SOURCE_LIST_ONLY=1",
	}
	listRaw, err := listCmd.Output()
	if err != nil {
		t.Fatalf("source-list mode: %v", err)
	}
	listText := strings.TrimSuffix(string(listRaw), "\x00")
	listPaths := strings.Split(listText, "\x00")
	if !slices.Equal(listPaths, paths) {
		t.Fatal("transfer list and hash manifest select different paths")
	}
	if _, err := os.Stat(dockerCalled); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source-list mode reached Docker")
	}
}

func TestBuyerIntentOpenUniquenessAcceptanceSourceModesPruneForbiddenSyntheticPaths(t *testing.T) {
	repoDir := t.TempDir()
	allowed := map[string]string{
		"Makefile":                                                     "synthetic\n",
		"backend/Dockerfile":                                           "synthetic\n",
		"backend/go.mod":                                               "module synthetic\n\ngo 1.22\n",
		"backend/go.sum":                                               "",
		"backend/main.go":                                              "package main\n",
		"backend/internal/allowed_test.go":                             "package internal\n",
		"backend/migrations/0009_allowed.sql":                          "SELECT 1;\n",
		"deploy/acceptance/allowed.sh":                                 "#!/bin/sh\n",
		"deploy/acceptance/allowed.yml":                                "services: {}\n",
		"deploy/acceptance/config/allowed.yaml":                        "enabled: true\n",
		"deploy/acceptance/config/allowed.conf":                        "enabled=true\n",
		"deploy/acceptance/README.md":                                  "synthetic\n",
		"deploy/acceptance/tool.Dockerfile":                            "FROM scratch\n",
		"deploy/acceptance/sql/allowed.sql":                            "SELECT 1;\n",
		"backend/migrations/nested/not-top-level.sql":                  "SELECT 2;\n",
		"deploy/acceptance/config/nested/not-within-approved-depth.sh": "#!/bin/sh\n",
	}
	for path, content := range allowed {
		writeBuyerIntentAcceptanceFixtureFile(t, repoDir, path, content)
	}
	script := copyBuyerIntentAcceptanceScript(t, repoDir)

	forbiddenDirectories := []string{
		".tmp", ".git", "node_modules", ".cache", "cache", "caches",
		"uploads", "evidence", "secrets", "backups", "databases",
	}
	for _, directory := range forbiddenDirectories {
		writeBuyerIntentAcceptanceFixtureFile(t, repoDir,
			filepath.Join("backend", "internal", directory, "forbidden.go"), "package forbidden\n")
		writeBuyerIntentAcceptanceFixtureFile(t, repoDir,
			filepath.Join("deploy", "acceptance", directory, "forbidden.sh"), "#!/bin/sh\n")
	}
	writeBuyerIntentAcceptanceFixtureFile(t, repoDir, "backend/internal/nested/.tmp/forbidden_test.go", "package forbidden\n")
	writeBuyerIntentAcceptanceFixtureFile(t, repoDir, "deploy/acceptance/config/.env.yml", "secret: forbidden\n")
	writeBuyerIntentAcceptanceFixtureFile(t, repoDir, "deploy/acceptance/.env.conf", "secret=forbidden\n")
	writeBuyerIntentAcceptanceFixtureFile(t, repoDir, "deploy/acceptance/data.sqlite.Dockerfile", "FROM forbidden\n")

	before := buyerIntentAcceptanceFixturePaths(t, repoDir)
	want := []string{
		"Makefile",
		"backend/Dockerfile",
		"backend/go.mod",
		"backend/go.sum",
		"backend/internal/allowed_test.go",
		"backend/main.go",
		"backend/migrations/0009_allowed.sql",
		"deploy/acceptance/README.md",
		"deploy/acceptance/allowed.sh",
		"deploy/acceptance/allowed.yml",
		"deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh",
		"deploy/acceptance/config/allowed.conf",
		"deploy/acceptance/config/allowed.yaml",
		"deploy/acceptance/sql/allowed.sql",
		"deploy/acceptance/tool.Dockerfile",
	}
	sort.Strings(want)

	listCmd := exec.Command("/bin/bash", script)
	listCmd.Env = []string{"PATH=" + os.Getenv("PATH"), "BUYER_INTENT_SOURCE_LIST_ONLY=1"}
	listRaw, err := listCmd.Output()
	if err != nil {
		t.Fatalf("synthetic source-list mode: %v", err)
	}
	listPaths := strings.Split(strings.TrimSuffix(string(listRaw), "\x00"), "\x00")
	if !slices.Equal(listPaths, want) {
		t.Fatalf("synthetic source list = %q, want %q", listPaths, want)
	}

	manifestCmd := exec.Command("/bin/bash", script)
	manifestCmd.Env = []string{"PATH=" + os.Getenv("PATH"), "BUYER_INTENT_SOURCE_MANIFEST_ONLY=1"}
	manifestRaw, err := manifestCmd.Output()
	if err != nil {
		t.Fatalf("synthetic manifest mode: %v", err)
	}
	var manifestPaths []string
	linePattern := regexp.MustCompile(`^[0-9a-f]{64}  (.+)$`)
	for _, line := range strings.Split(strings.TrimSpace(string(manifestRaw)), "\n") {
		match := linePattern.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("invalid synthetic manifest line %q", line)
		}
		manifestPaths = append(manifestPaths, match[1])
	}
	if !slices.Equal(manifestPaths, want) {
		t.Fatalf("synthetic manifest paths = %q, want %q", manifestPaths, want)
	}
	if after := buyerIntentAcceptanceFixturePaths(t, repoDir); !slices.Equal(after, before) {
		t.Fatalf("read-only source modes changed fixture paths: before=%q after=%q", before, after)
	}
}

func TestBuyerIntentOpenUniquenessAcceptanceDoesNotRetainLeakedStagedEvidence(t *testing.T) {
	script, environment := prepareBuyerIntentAcceptanceHarness(t, buyerIntentAcceptanceComposeConfigStub+`case "$1 $2" in
  "container ls"|"volume ls"|"network ls")
    exit 0
    ;;
esac
exit 1
`)
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "sha256sum"), []byte(
		"#!/bin/sh\nprintf 'Authorization: injected-forbidden-value\\n'\n",
	), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "tar"), []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	environment = prependBuyerIntentAcceptancePath(environment, stubDir)

	cmd := exec.Command("/bin/bash", script)
	cmd.Env = environment
	if err := cmd.Run(); err == nil {
		t.Fatal("synthetic evidence leak unexpectedly succeeded")
	}
	evidenceDir := filepath.Join(filepath.Dir(script), "evidence", "buyer-intent-open-uniqueness")
	paths := buyerIntentAcceptanceFixturePaths(t, evidenceDir)
	want := []string{"evidence-leak-failure.txt"}
	if !slices.Equal(paths, want) {
		t.Fatalf("retained leaked-evidence paths = %q, want %q", paths, want)
	}
	raw, err := os.ReadFile(filepath.Join(evidenceDir, want[0]))
	if err != nil {
		t.Fatalf("read categorical leak marker: %v", err)
	}
	if strings.Contains(string(raw), "injected-forbidden-value") || strings.Contains(string(raw), "Authorization") {
		t.Fatalf("categorical leak marker retained forbidden content: %q", raw)
	}
}

func TestBuyerIntentOpenUniquenessAcceptanceFailsClosedWhenEvidenceScanErrors(t *testing.T) {
	script, environment := prepareBuyerIntentAcceptanceHarness(t, buyerIntentAcceptanceComposeConfigStub+`case "$1 $2" in
  "container ls"|"volume ls"|"network ls")
    exit 0
    ;;
esac
exit 1
`)
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "sha256sum"), []byte(
		"#!/bin/sh\nprintf 'Authorization: injected-forbidden-value\\n'\n",
	), 0o700); err != nil {
		t.Fatal(err)
	}
	grepPath, err := exec.LookPath("grep")
	if err != nil {
		t.Fatal(err)
	}
	grepStub := fmt.Sprintf(`#!/bin/sh
case "$1" in
  -ERn) exit 72 ;;
esac
exec %q "$@"
`, grepPath)
	if err := os.WriteFile(filepath.Join(stubDir, "grep"), []byte(grepStub), 0o700); err != nil {
		t.Fatal(err)
	}
	environment = prependBuyerIntentAcceptancePath(environment, stubDir)

	cmd := exec.Command("/bin/bash", script)
	cmd.Env = environment
	if err := cmd.Run(); err == nil {
		t.Fatal("synthetic evidence scan error unexpectedly succeeded")
	}
	evidenceDir := filepath.Join(filepath.Dir(script), "evidence", "buyer-intent-open-uniqueness")
	paths := buyerIntentAcceptanceFixturePaths(t, evidenceDir)
	want := []string{"evidence-scan-failure.txt"}
	if !slices.Equal(paths, want) {
		t.Fatalf("retained scan-error paths = %q, want %q", paths, want)
	}
}

func TestBuyerIntentOpenUniquenessAcceptanceDistinguishesContainerAbsenceFromInspectFailure(t *testing.T) {
	t.Run("proven absent", func(t *testing.T) {
		script, environment := prepareBuyerIntentAcceptanceHarness(t, buyerIntentAcceptanceComposeConfigStub+`case "$1 $2" in
  "container ls"|"volume ls"|"network ls")
    exit 0
    ;;
  "inspect --type")
    exit 1
    ;;
esac
exit 1
`)
		cmd := exec.Command("/bin/bash", script)
		cmd.Env = environment
		if err := cmd.Run(); err == nil {
			t.Fatal("synthetic absent-container harness unexpectedly completed")
		}
		evidence := filepath.Join(filepath.Dir(script), "evidence", "buyer-intent-open-uniqueness", "production-before.txt")
		raw, err := os.ReadFile(evidence)
		if err != nil {
			t.Fatalf("read proven-absence snapshot: %v", err)
		}
		if !strings.Contains(string(raw), "/secondhand-market-api|absent|absent|absent\n") {
			t.Fatalf("proven absence snapshot = %q", raw)
		}
	})

	t.Run("inspect failure", func(t *testing.T) {
		script, environment := prepareBuyerIntentAcceptanceHarness(t, buyerIntentAcceptanceComposeConfigStub+`case "$1 $2" in
  "container ls")
    case "$*" in
      *"name=^/secondhand-market-api$"*) printf 'secondhand-market-api\n' ;;
      *"name=^/secondhand-market-web$"*) printf 'secondhand-market-web\n' ;;
      *"name=^/secondhand-market-mysql$"*) printf 'secondhand-market-mysql\n' ;;
    esac
    exit 0
    ;;
  "volume ls"|"network ls")
    exit 0
    ;;
  "inspect --type")
    echo 'synthetic daemon inspection failure' >&2
    exit 71
    ;;
esac
exit 1
`)
		cmd := exec.Command("/bin/bash", script)
		cmd.Env = environment
		raw, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("synthetic inspect failure unexpectedly completed")
		}
		if !strings.Contains(string(raw), "failed to inspect production container secondhand-market-api") {
			t.Fatalf("inspect failure did not fail closed: %s", raw)
		}
	})
}

func writeBuyerIntentAcceptanceFixtureFile(t *testing.T, root, path, content string) {
	t.Helper()
	target := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

func copyBuyerIntentAcceptanceScript(t *testing.T, repoDir string) string {
	t.Helper()
	raw, err := os.ReadFile("../../deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repoDir, "deploy", "acceptance", "buyer-intent-open-uniqueness-smoke.sh")
	writeBuyerIntentAcceptanceFixtureFile(t, repoDir,
		"deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh", string(raw))
	return path
}

func buyerIntentAcceptanceFixturePaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	return paths
}

func prependBuyerIntentAcceptancePath(environment []string, directory string) []string {
	result := append([]string(nil), environment...)
	for index, variable := range result {
		if strings.HasPrefix(variable, "PATH=") {
			result[index] = "PATH=" + directory + ":" + strings.TrimPrefix(variable, "PATH=")
			return result
		}
	}
	return append(result, "PATH="+directory)
}

func TestBuyerIntentOpenUniquenessAcceptanceSnapshotsWithFormattedInspectOnly(t *testing.T) {
	script, environment := prepareBuyerIntentAcceptanceHarness(t, buyerIntentAcceptanceComposeConfigStub+`case "$1 $2" in
  "container ls")
    case "$*" in
      *"name=^/secondhand-market-api$"*) printf 'secondhand-market-api\n' ;;
      *"name=^/secondhand-market-web$"*) printf 'secondhand-market-web\n' ;;
      *"name=^/secondhand-market-mysql$"*) printf 'secondhand-market-mysql\n' ;;
    esac
    exit 0
    ;;
  "volume ls"|"network ls")
    exit 0
    ;;
  "inspect --type")
    shift
    printf '%s\n' "$*" >>"$DOCKER_INSPECT_LOG"
    if [ "$3" != "--format" ]; then
      exit 1
    fi
    printf '/synthetic|synthetic|running|0\n'
    exit 0
    ;;
esac
exit 1
`)
	inspectLog := filepath.Join(t.TempDir(), "docker-inspect.log")
	environment = append(environment, "DOCKER_INSPECT_LOG="+inspectLog)

	cmd := exec.Command("/bin/bash", script)
	cmd.Env = environment
	if err := cmd.Run(); err == nil {
		t.Fatal("synthetic harness unexpectedly completed")
	}

	raw, err := os.ReadFile(inspectLog)
	if err != nil {
		t.Fatalf("read inspect log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		"--type container --format {{.Name}}|{{.Id}}|{{.State.Status}}|{{.RestartCount}} secondhand-market-api",
		"--type container --format {{.Name}}|{{.Id}}|{{.State.Status}}|{{.RestartCount}} secondhand-market-web",
		"--type container --format {{.Name}}|{{.Id}}|{{.State.Status}}|{{.RestartCount}} secondhand-market-mysql",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("production snapshots used inspect arguments %q, want %q", got, want)
	}
}

func TestBuyerIntentOpenUniquenessAcceptanceSignalsExitNonzero(t *testing.T) {
	for _, tc := range []struct {
		name   string
		signal syscall.Signal
		want   int
	}{
		{name: "interrupt", signal: syscall.SIGINT, want: 130},
		{name: "terminate", signal: syscall.SIGTERM, want: 143},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := filepath.Join(t.TempDir(), "release")
			ready := filepath.Join(t.TempDir(), "ready")
			script, environment := prepareBuyerIntentAcceptanceHarness(t, buyerIntentAcceptanceComposeConfigStub+`case "$1 $2" in
  "container ls")
    case "$*" in
      *"name=^/secondhand-market-api$"*) printf 'secondhand-market-api\n' ;;
      *"name=^/secondhand-market-web$"*) printf 'secondhand-market-web\n' ;;
      *"name=^/secondhand-market-mysql$"*) printf 'secondhand-market-mysql\n' ;;
    esac
    exit 0
    ;;
  "volume ls"|"network ls")
    exit 0
    ;;
  "inspect --type")
    : >"$DOCKER_READY"
    while [ ! -e "$DOCKER_RELEASE" ]; do
      sleep 0.01
    done
    printf '/synthetic|synthetic|running|0\n'
    exit 0
    ;;
esac
exit 1
`)
			environment = append(environment,
				"DOCKER_READY="+ready,
				"DOCKER_RELEASE="+release,
			)
			cmd := exec.Command("/bin/bash", script)
			cmd.Env = environment
			if err := cmd.Start(); err != nil {
				t.Fatalf("start synthetic harness: %v", err)
			}
			defer func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}()
			waitForBuyerIntentAcceptanceFile(t, ready)
			if err := cmd.Process.Signal(tc.signal); err != nil {
				t.Fatalf("send %s: %v", tc.name, err)
			}
			if err := os.WriteFile(release, nil, 0o600); err != nil {
				t.Fatalf("release inspection: %v", err)
			}
			err := cmd.Wait()
			if err == nil {
				t.Fatalf("%s exited successfully", tc.name)
			}
			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != tc.want {
				t.Fatalf("%s exit = %v, want exit status %d", tc.name, err, tc.want)
			}
		})
	}
}

func prepareBuyerIntentAcceptanceHarness(t *testing.T, dockerStub string) (string, []string) {
	t.Helper()
	repoDir := t.TempDir()
	acceptanceDir := filepath.Join(repoDir, "deploy", "acceptance")
	if err := os.MkdirAll(filepath.Join(acceptanceDir, "secrets"), 0o700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"Makefile":             "",
		"backend/Dockerfile":   "",
		"backend/go.mod":       "module synthetic\n\ngo 1.22\n",
		"backend/go.sum":       "",
		"backend/migrations/x": "",
	} {
		file := filepath.Join(repoDir, path)
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	source, err := os.ReadFile("../../deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(acceptanceDir, "buyer-intent-open-uniqueness-smoke.sh")
	if err := os.WriteFile(script, source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acceptanceDir, ".env"),
		[]byte("MYSQL_DATABASE=second_hand_market_acceptance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acceptanceDir, "secrets", "control-admin-password"),
		[]byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "docker"), []byte(dockerStub), 0o700); err != nil {
		t.Fatal(err)
	}
	return script, []string{
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
		"BUYER_INTENT_ACCEPTANCE_CONFIRM=" + buyerIntentAcceptanceConfirmation,
		"ACCEPTANCE_DB_ENGINE=mysql8.4",
	}
}

const buyerIntentAcceptanceComposeConfigStub = `#!/bin/sh
if [ "$1" = compose ]; then
  previous=""
  compose_file=""
  env_file=""
  for argument in "$@"; do
    if [ "$previous" = --file ]; then
      compose_file="$argument"
    fi
    if [ "$previous" = --env-file ]; then
      env_file="$argument"
    fi
    previous="$argument"
  done
  source_dir=$(sed -n 's/.*context: "\(.*\)"/\1/p' "$compose_file")
  acceptance_dir=$(dirname "$env_file")
  repo_dir=$(dirname "$(dirname "$acceptance_dir")")
  secret="$acceptance_dir/secrets/control-admin-password"
  migrations="$repo_dir/backend/migrations"
  printf '%s\n' "{\"services\":{\"bootstrap-admin\":{\"working_dir\":\"/workspace/backend\",\"build\":{\"context\":\"$source_dir\"},\"volumes\":[{\"target\":\"/workspace\",\"source\":\"$source_dir\",\"read_only\":true},{\"target\":\"/run/secrets/admin-password\",\"source\":\"$secret\",\"read_only\":true}],\"environment\":{\"DB_DSN\":\"\${MYSQL_USER}\${MYSQL_PASSWORD}\"}},\"mysql\":{\"volumes\":[{\"target\":\"/acceptance/migrations\",\"source\":\"$migrations\",\"read_only\":true}],\"environment\":{\"MYSQL_DATABASE\":\"\${MYSQL_DATABASE}\",\"MYSQL_USER\":\"\${MYSQL_USER}\",\"MYSQL_PASSWORD\":\"\${MYSQL_PASSWORD}\",\"MYSQL_ROOT_PASSWORD\":\"\${MYSQL_ROOT_PASSWORD}\"}},\"api\":{\"environment\":{\"DB_DSN\":\"\${MYSQL_USER}\${MYSQL_PASSWORD}\",\"JWT_ACCESS_SECRET\":\"\${JWT_ACCESS_SECRET}\",\"JWT_REFRESH_SECRET\":\"\${JWT_REFRESH_SECRET}\",\"FILE_UPLOAD_IP_HASH_SECRET\":\"\${FILE_UPLOAD_IP_HASH_SECRET}\"}}}}"
  exit 0
fi
`

func waitForBuyerIntentAcceptanceFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func TestBuyerIntentOpenUniquenessMigrationArtifacts(t *testing.T) {
	for _, name := range []string{
		"0009_buyer_intent_open_uniqueness.preflight.sql",
		"0009_buyer_intent_open_uniqueness.up.sql",
		"0009_buyer_intent_open_uniqueness.postflight.sql",
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if err := validateBuyerIntentOpenUniquenessArtifact(name, string(raw)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type migrationSQLContract struct {
	name    string
	pattern string
}

func validateBuyerIntentOpenUniquenessArtifact(name, sql string) error {
	common := []migrationSQLContract{
		{"buyer intents table", anchoredMigrationSQL(`    AND table_name = 'buyer_intents'`)},
		{"generated marker catalog definition", anchoredMigrationSQL(`    SELECT data_type, column_type, is_nullable, generation_expression, extra,
      CASE
        WHEN generation_expression IS NOT NULL AND generation_expression <> '' THEN 'ALWAYS'
        ELSE 'NEVER'
      END AS is_generated
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'buyer_intents'
      AND column_name = 'open_marker'`)},
		{"stored marker predicate", anchoredMigrationSQL(`    AND is_generated = 'ALWAYS'
    AND UPPER(extra) LIKE '%STORED GENERATED%'
    AND LOWER(
      REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(
        generation_expression,
        '` + "`" + `', ''), ' ', ''), CHAR(9), ''), CHAR(10), ''), CHAR(13), ''),
        '(', ''), ')', '')
    ) = 'casewhenis_open=1then1elsenullend';`)},
		{"legacy ordered unique key", anchoredMigrationSQL(`      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'buyer_id,product_id,is_open'`)},
		{"open ordered unique key", anchoredMigrationSQL(`      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'buyer_id,product_id,open_marker'`)},
		{"invalid state rows", anchoredMigrationSQL(invalidBuyerIntentRowsSQL())},
		{"duplicate open groups", anchoredMigrationSQL(duplicateBuyerIntentGroupsSQL())},
		{"fail closed signal", `(?m)^    SIGNAL SQLSTATE '45000'$`},
	}
	if err := requireMigrationSQLContracts(sql, common); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	switch name {
	case "0009_buyer_intent_open_uniqueness.preflight.sql":
		return validateBuyerIntentPreflightContract(sql)
	case "0009_buyer_intent_open_uniqueness.up.sql":
		return validateBuyerIntentUpContract(sql)
	case "0009_buyer_intent_open_uniqueness.postflight.sql":
		return validateBuyerIntentPostflightContract(sql)
	default:
		return fmt.Errorf("unsupported buyer intent migration artifact %q", name)
	}
}

func validateBuyerIntentPreflightContract(sql string) error {
	formalState := anchoredMigrationSQL(buyerIntentFormalStateSQL("preflight"))
	contracts := []migrationSQLContract{
		{"relevant lookalike computation", anchoredMigrationSQL(`  SET v_relevant_lookalikes = v_relevant_keys - v_legacy_exact - v_open_exact;
  IF v_relevant_lookalikes <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent preflight: relevant unique key is drifted';
  END IF;`)},
		{"exact formal state ladder", formalState},
		{"success marker", anchoredMigrationSQL(`  SELECT 'buyer_intent_open_uniqueness_preflight_passed' AS migration_gate;`)},
	}
	if err := requireMigrationSQLContracts(sql, contracts); err != nil {
		return fmt.Errorf("preflight contract: %w", err)
	}
	return requireMigrationSQLOrder(sql, []migrationSQLContract{
		{"invalid state rows", anchoredMigrationSQL(invalidBuyerIntentRowsSQL())},
		{"duplicate open groups", anchoredMigrationSQL(duplicateBuyerIntentGroupsSQL())},
		{"formal state ladder", formalState},
		{"success marker", contracts[2].pattern},
	})
}

func validateBuyerIntentUpContract(sql string) error {
	formalState := anchoredMigrationSQL(buyerIntentFormalStateSQL("migration"))
	finalState := anchoredMigrationSQL(`  IF v_marker_exact <> 1
      OR v_legacy_key <> 0
      OR v_open_key <> 1
      OR v_open_exact <> 1
      OR v_relevant_keys <> 1
      OR v_relevant_lookalikes <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent migration: final schema is missing or drifted';
  END IF;`)
	contracts := []migrationSQLContract{
		{"relevant lookalike computation", anchoredMigrationSQL(`  SET v_relevant_lookalikes = v_relevant_keys - v_legacy_exact - v_open_exact;
  IF v_relevant_lookalikes <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent migration: relevant unique key is drifted';
  END IF;`)},
		{"exact formal state ladder", formalState},
		{"add generated marker", anchoredMigrationSQL(`  IF v_marker = 0 THEN
    ALTER TABLE buyer_intents
      ADD COLUMN open_marker TINYINT
        GENERATED ALWAYS AS (
          CASE WHEN is_open = 1 THEN 1 ELSE NULL END
        ) STORED AFTER is_open;
  END IF;`)},
		{"marker exact recheck", anchoredMigrationSQL(`  IF v_marker <> 1 OR v_marker_exact <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent migration: generated marker is missing or drifted';
  END IF;`)},
		{"add open unique key", anchoredMigrationSQL(`  IF v_open_key = 0 THEN
    ALTER TABLE buyer_intents
      ADD UNIQUE KEY uk_buyer_intent_open
        (buyer_id, product_id, open_marker);
  END IF;`)},
		{"open key exact recheck", anchoredMigrationSQL(`  IF v_open_key <> 1 OR v_open_exact <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent migration: open key is missing or drifted';
  END IF;`)},
		{"drop legacy unique key", anchoredMigrationSQL(`  IF v_legacy_key = 1 THEN
    ALTER TABLE buyer_intents
      DROP INDEX uk_buyer_product_open;
  END IF;`)},
		{"final exact state", finalState},
		{"success marker", anchoredMigrationSQL(`  SELECT 'buyer_intent_open_uniqueness_migration_applied' AS migration_gate;`)},
	}
	if err := requireMigrationSQLContracts(sql, contracts); err != nil {
		return fmt.Errorf("up contract: %w", err)
	}
	if err := requireMigrationSQLOrder(sql, []migrationSQLContract{
		{"invalid state rows", anchoredMigrationSQL(invalidBuyerIntentRowsSQL())},
		{"duplicate open groups", anchoredMigrationSQL(duplicateBuyerIntentGroupsSQL())},
		{"formal state ladder", formalState},
		{"add generated marker", contracts[2].pattern},
		{"marker catalog reinspection", `(?m)^  \) AS marker_definition_after_column$`},
		{"marker exact recheck", contracts[3].pattern},
		{"add open unique key", contracts[4].pattern},
		{"open key catalog reinspection", `(?m)^  \) AS exact_open_key_before_legacy_drop;$`},
		{"open key exact recheck", contracts[5].pattern},
		{"drop legacy unique key", contracts[6].pattern},
		{"final marker reinspection", `(?m)^  \) AS final_marker_definition$`},
		{"final exact state", finalState},
		{"success marker", contracts[8].pattern},
	}); err != nil {
		return fmt.Errorf("up DDL sequence: %w", err)
	}
	return nil
}

func validateBuyerIntentPostflightContract(sql string) error {
	contracts := []migrationSQLContract{
		{"exact generated marker", anchoredMigrationSQL(`  IF v_marker <> 1 OR v_marker_exact <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: generated marker is missing or drifted';
  END IF;`)},
		{"zero legacy key", anchoredMigrationSQL(`  IF v_legacy_key <> 0 OR v_legacy_exact <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: legacy key remains';
  END IF;`)},
		{"exact open key", anchoredMigrationSQL(`  IF v_open_key <> 1 OR v_open_exact <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: open key is missing or drifted';
  END IF;`)},
		{"zero relevant lookalikes", anchoredMigrationSQL(`  SET v_relevant_lookalikes = v_relevant_keys - v_open_exact;
  IF v_relevant_keys <> 1 OR v_relevant_lookalikes <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: relevant unique key is drifted';
  END IF;`)},
		{"success marker", anchoredMigrationSQL(`  SELECT 'buyer_intent_open_uniqueness_postflight_passed' AS migration_gate;`)},
	}
	if err := requireMigrationSQLContracts(sql, contracts); err != nil {
		return fmt.Errorf("postflight contract: %w", err)
	}
	return requireMigrationSQLOrder(sql, []migrationSQLContract{
		contracts[0],
		contracts[1],
		contracts[2],
		contracts[3],
		{"invalid state rows", anchoredMigrationSQL(invalidBuyerIntentRowsSQL())},
		{"duplicate open groups", anchoredMigrationSQL(duplicateBuyerIntentGroupsSQL())},
		contracts[4],
	})
}

func requireMigrationSQLContracts(sql string, contracts []migrationSQLContract) error {
	for _, contract := range contracts {
		matched, err := regexp.MatchString(contract.pattern, sql)
		if err != nil {
			return fmt.Errorf("invalid %s pattern: %w", contract.name, err)
		}
		if !matched {
			return fmt.Errorf("missing or drifted %s", contract.name)
		}
	}
	return nil
}

func requireMigrationSQLOrder(sql string, steps []migrationSQLContract) error {
	offset := 0
	for _, step := range steps {
		pattern, err := regexp.Compile(step.pattern)
		if err != nil {
			return fmt.Errorf("invalid %s pattern: %w", step.name, err)
		}
		match := pattern.FindStringIndex(sql[offset:])
		if match == nil {
			return fmt.Errorf("missing or out-of-order %s", step.name)
		}
		offset += match[1]
	}
	return nil
}

func anchoredMigrationSQL(sql string) string {
	return `(?m)^` + regexp.QuoteMeta(sql) + `$`
}

func invalidBuyerIntentRowsSQL() string {
	return `  SELECT COUNT(*) INTO v_invalid_rows
  FROM buyer_intents
  WHERE CASE
    WHEN status IN ('NEW', 'CONTACTED') AND is_open = 1 THEN 0
    WHEN status = 'CLOSED' AND is_open = 0 THEN 0
    ELSE 1
  END = 1;`
}

func duplicateBuyerIntentGroupsSQL() string {
	return `  SELECT COUNT(*) INTO v_duplicate_groups
  FROM (
    SELECT buyer_id, product_id
    FROM buyer_intents
    WHERE is_open = 1
    GROUP BY buyer_id, product_id
    HAVING COUNT(*) > 1
  ) AS duplicate_open_intents;`
}

func buyerIntentFormalStateSQL(scope string) string {
	return fmt.Sprintf(`  IF v_marker = 0 AND v_legacy_exact = 1 AND v_open_key = 0 THEN
    SET v_state = 'legacy';
  ELSEIF v_marker_exact = 1 AND v_legacy_exact = 1 AND v_open_key = 0 THEN
    SET v_state = 'marker_only';
  ELSEIF v_marker_exact = 1 AND v_legacy_exact = 1 AND v_open_exact = 1 THEN
    SET v_state = 'both_keys';
  ELSEIF v_marker_exact = 1 AND v_legacy_key = 0 AND v_open_exact = 1 THEN
    SET v_state = 'final';
  ELSE
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent %s: schema is partial or drifted';
  END IF;`, scope)
}

func TestBuyerIntentOpenUniquenessMigrationRejectsBehavioralMutations(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		want   string
		mutate func(*testing.T, string) string
	}{
		{
			name: "loosened formal state ladder",
			file: "0009_buyer_intent_open_uniqueness.preflight.sql",
			want: "missing or drifted exact formal state ladder",
			mutate: func(t *testing.T, sql string) string {
				return replaceMigrationSQL(t, sql,
					"IF v_marker = 0 AND v_legacy_exact = 1 AND v_open_key = 0 THEN",
					"IF v_marker = 0 THEN",
				)
			},
		},
		{
			name: "state classification moved before data checks",
			file: "0009_buyer_intent_open_uniqueness.preflight.sql",
			want: "missing or out-of-order formal state ladder",
			mutate: func(t *testing.T, sql string) string {
				return moveMigrationSQLBefore(t, sql,
					"  IF v_marker = 0 AND v_legacy_exact = 1 AND v_open_key = 0 THEN",
					"\n\n  SELECT 'buyer_intent_open_uniqueness_preflight_passed'",
					"  SELECT COUNT(*) INTO v_invalid_rows",
				)
			},
		},
		{
			name: "relevant lookalike count zeroed",
			file: "0009_buyer_intent_open_uniqueness.preflight.sql",
			want: "missing or drifted relevant lookalike computation",
			mutate: func(t *testing.T, sql string) string {
				return replaceMigrationSQL(t, sql,
					"SET v_relevant_lookalikes = v_relevant_keys - v_legacy_exact - v_open_exact;",
					"SET v_relevant_lookalikes = 0;",
				)
			},
		},
		{
			name: "open key added before marker recheck",
			file: "0009_buyer_intent_open_uniqueness.up.sql",
			want: "missing or out-of-order add open unique key",
			mutate: func(t *testing.T, sql string) string {
				return moveMigrationSQLBefore(t, sql,
					"  IF v_open_key = 0 THEN",
					"\n\n  SELECT COUNT(DISTINCT index_name) INTO v_open_key",
					"  SELECT COUNT(*) INTO v_marker",
				)
			},
		},
		{
			name: "final lookalike predicate removed",
			file: "0009_buyer_intent_open_uniqueness.up.sql",
			want: "missing or drifted final exact state",
			mutate: func(t *testing.T, sql string) string {
				return replaceMigrationSQL(t, sql,
					"      OR v_relevant_lookalikes <> 0 THEN",
					"      OR v_relevant_keys < 0 THEN",
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := os.ReadFile(test.file)
			if err != nil {
				t.Fatal(err)
			}
			mutated := test.mutate(t, string(raw))
			err = validateBuyerIntentOpenUniquenessArtifact(test.file, mutated)
			if err == nil {
				t.Fatalf("contract accepted %s", test.name)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("contract rejected %s for the wrong reason: %v", test.name, err)
			}
		})
	}
}

func replaceMigrationSQL(t *testing.T, sql, old, replacement string) string {
	t.Helper()
	mutated := strings.Replace(sql, old, replacement, 1)
	if mutated == sql {
		t.Fatalf("mutation target missing %q", old)
	}
	return mutated
}

func moveMigrationSQLBefore(t *testing.T, sql, start, end, before string) string {
	t.Helper()
	startIndex := strings.Index(sql, start)
	if startIndex < 0 {
		t.Fatalf("mutation start missing %q", start)
	}
	endOffset := strings.Index(sql[startIndex:], end)
	if endOffset < 0 {
		t.Fatalf("mutation end missing %q", end)
	}
	endIndex := startIndex + endOffset
	segment := sql[startIndex:endIndex]
	withoutSegment := sql[:startIndex] + sql[endIndex:]
	beforeIndex := strings.Index(withoutSegment, before)
	if beforeIndex < 0 {
		t.Fatalf("mutation destination missing %q", before)
	}
	return withoutSegment[:beforeIndex] + segment + "\n\n" + withoutSegment[beforeIndex:]
}

func TestBuyerIntentOpenUniquenessMigrationHasNoBusinessDML(t *testing.T) {
	for _, name := range []string{
		"0009_buyer_intent_open_uniqueness.preflight.sql",
		"0009_buyer_intent_open_uniqueness.up.sql",
		"0009_buyer_intent_open_uniqueness.postflight.sql",
	} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		forbidden := regexp.MustCompile(
			"(?is)\\b(?:INSERT\\s+INTO|UPDATE\\s+[`a-z0-9_]+|DELETE\\s+FROM|" +
				"REPLACE\\s+INTO|TRUNCATE(?:\\s+TABLE)?)\\b",
		)
		if match := forbidden.Find(raw); match != nil {
			t.Fatalf("%s contains business DML %q", name, match)
		}
	}
}

func TestBuyerIntentOpenUniquenessMigrationHasNoHiddenMutationPath(t *testing.T) {
	for _, name := range []string{
		"0009_buyer_intent_open_uniqueness.preflight.sql",
		"0009_buyer_intent_open_uniqueness.up.sql",
		"0009_buyer_intent_open_uniqueness.postflight.sql",
	} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		forbidden := regexp.MustCompile(
			"(?is)\\b(?:PREPARE|EXECUTE|RENAME\\s+TABLE|CREATE\\s+TABLE\\b[\\s\\S]*?\\bSELECT)\\b",
		)
		if match := forbidden.Find(raw); match != nil {
			t.Fatalf("%s contains forbidden migration path %q", name, match)
		}
	}
}

func TestBuyerIntentOpenUniquenessMigrationHasNoDownScript(t *testing.T) {
	matches, err := filepath.Glob("0009*.down.sql")
	if err != nil {
		t.Fatalf("glob 0009 down migrations: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("0009 down migrations must not exist: %v", matches)
	}
}
