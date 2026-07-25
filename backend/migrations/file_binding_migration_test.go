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
