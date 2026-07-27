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
	"testing"
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
	for _, forbidden := range []string{
		".env", "secrets/", "evidence/", "uploads/", "app.db", ".tmp/",
		"architecture-evolution-plan-2026-07-24.md",
		"first-round-fix-review-2026-07-24.md",
		"second-round-fix-review-2026-07-24.md",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("manifest exposed forbidden path %q", forbidden)
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
