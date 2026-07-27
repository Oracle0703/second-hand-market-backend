package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestFileBindingMigrationArtifacts(t *testing.T) {
	tests := map[string][]string{
		"0006_file_binding_ownership.preflight.sql": {
			"file_binding_ownership_preflight",
			"table_name = 'file_records'",
			"table_name = 'files'",
			"product_images",
			"license_file_id",
			"COUNT(DISTINCT merchant_id)",
			"file_binding_ownership_preflight_passed",
			"SIGNAL SQLSTATE '45000'",
		},
		"0006_file_binding_ownership.up.sql": {
			"owner_merchant_id",
			"capability_token_hash",
			"capability_expires_at",
			"idx_file_owner_biz_scan",
			"uk_file_capability_token",
			"idx_file_capability_expires",
			"file_binding_ownership_migration_applied",
		},
		"0006_file_binding_ownership.postflight.sql": {
			"file_binding_ownership_postflight",
			"owner_merchant_id",
			"file_binding_ownership_postflight_passed",
			"SIGNAL SQLSTATE '45000'",
		},
	}

	for name, required := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			text := string(raw)
			for _, snippet := range required {
				if !strings.Contains(text, snippet) {
					t.Errorf("%s missing %q", name, snippet)
				}
			}
		})
	}
}

func TestFileBindingMigrationHasNoDestructiveDownScript(t *testing.T) {
	_, err := os.Stat("0006_file_binding_ownership.down.sql")
	if !os.IsNotExist(err) {
		t.Fatalf("0006 down migration must not exist; stat error = %v", err)
	}
}

func TestFileBindingAcceptanceScriptContracts(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/acceptance/file-binding-authorization-smoke.sh")
	if err != nil {
		t.Fatalf("read file binding acceptance script: %v", err)
	}
	text := string(raw)
	for _, snippet := range []string{
		"FILE_BINDING_ACCEPTANCE_CONFIRM",
		"I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_BINDING_DATA",
		"file_binding_ownership_preflight_passed",
		"file_binding_ownership_postflight_passed",
		"TestFileFlowWithMigrationOnlyMySQL",
		"docker container ls -a --filter",
		"file binding preflight: invalid product image references",
		"file binding preflight: file is referenced by multiple merchants",
		"file binding preflight: merchant uploader ownership mismatch",
		`ERROR 1644 \(45000\)`,
		`[[ "$mysql_version" == 8.4.* ]]`,
	} {
		if !strings.Contains(text, snippet) {
			t.Errorf("acceptance script missing %q", snippet)
		}
	}
}

func TestFileBindingAcceptanceUsesCurrentMigrationChain(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/acceptance/file-binding-authorization-smoke.sh")
	if err != nil {
		t.Fatalf("read file binding acceptance script: %v", err)
	}

	requireOrderedScriptSnippets(t, string(raw), []string{
		`run_0006 | tee "$evidence_dir/full-chain.txt"`,
		"mysql_file /acceptance/migrations/0007_license_file_privacy.preflight.sql",
		"mysql_file /acceptance/migrations/0007_license_file_privacy.up.sql",
		"mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql",
		"mysql_file /acceptance/migrations/0008_anonymous_upload_governance.preflight.sql",
		"mysql_file /acceptance/migrations/0008_anonymous_upload_governance.up.sql",
		"mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql",
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql",
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql",
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql",
		"-e AUTO_MIGRATE=false",
		`bootstrap-admin go test ./tests -run '^TestFileFlowWithMigrationOnlyMySQL$' -count=1 -v`,
	})
	// Catches adding a false-mode file API run before 0009 postflight in the
	// full-chain phase.
	requireCurrentChainBeforeFirstFalseModeFocusedAPI(t, string(raw),
		"# Full chain plus real API registration, product binding, concurrent claim, and AutoMigrate compatibility.",
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql",
		"-e AUTO_MIGRATE=false",
		`bootstrap-admin go test ./tests -run '^TestFileFlowWithMigrationOnlyMySQL$' -count=1 -v`,
	)
}
