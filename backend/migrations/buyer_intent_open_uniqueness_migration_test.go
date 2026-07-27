package migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

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
