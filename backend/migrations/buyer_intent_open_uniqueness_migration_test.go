package migrations

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestBuyerIntentOpenUniquenessMigrationArtifacts(t *testing.T) {
	common := []string{
		"buyer_intents",
		"open_marker",
		"uk_buyer_product_open",
		"uk_buyer_intent_open",
		"buyer_id,product_id,is_open",
		"buyer_id,product_id,open_marker",
		"generation_expression",
		"is_generated",
		"STORED GENERATED",
		"status IN ('NEW', 'CONTACTED') AND is_open = 1",
		"status = 'CLOSED' AND is_open = 0",
		"GROUP BY buyer_id, product_id",
		"SIGNAL SQLSTATE '45000'",
	}
	tests := map[string][]string{
		"0009_buyer_intent_open_uniqueness.preflight.sql": {
			"buyer_intent_open_uniqueness_preflight_passed",
			"casewhenis_open=1then1elsenullend",
			"v_state = 'legacy'",
			"v_state = 'marker_only'",
			"v_state = 'both_keys'",
			"v_state = 'final'",
		},
		"0009_buyer_intent_open_uniqueness.up.sql": {
			"buyer_intent_open_uniqueness_migration_applied",
			"ADD COLUMN open_marker TINYINT",
			"GENERATED ALWAYS AS",
			"CASE WHEN is_open = 1 THEN 1 ELSE NULL END",
			"STORED AFTER is_open",
			"ADD UNIQUE KEY uk_buyer_intent_open",
			"(buyer_id, product_id, open_marker)",
			"DROP INDEX uk_buyer_product_open",
		},
		"0009_buyer_intent_open_uniqueness.postflight.sql": {
			"buyer_intent_open_uniqueness_postflight_passed",
			"casewhenis_open=1then1elsenullend",
		},
	}

	for name, required := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			text := string(raw)
			for _, snippet := range append(common, required...) {
				if !strings.Contains(text, snippet) {
					t.Errorf("%s missing %q", name, snippet)
				}
			}
		})
	}
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
