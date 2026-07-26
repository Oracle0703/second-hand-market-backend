package migrations

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestLicenseFilePrivacyMigrationArtifacts(t *testing.T) {
	tests := map[string][]string{
		"0007_license_file_privacy.preflight.sql": {
			"license_file_privacy_preflight",
			"table_name = 'file_records'",
			"table_name = 'files'",
			"owner_merchant_id",
			"capability_token_hash",
			"capability_expires_at",
			"idx_file_owner_biz_scan",
			"uk_file_capability_token",
			"idx_file_capability_expires",
			"MERCHANT_LICENSE",
			"object_key",
			"license_file_id",
			"SIGNAL SQLSTATE '45000'",
			"license_file_privacy_preflight_passed",
		},
		"0007_license_file_privacy.up.sql": {
			"UPDATE file_records",
			"SET url = ''",
			"biz_type = 'MERCHANT_LICENSE'",
			"url <> ''",
			"license_file_privacy_migration_applied",
		},
		"0007_license_file_privacy.postflight.sql": {
			"license_file_privacy_postflight",
			"MERCHANT_LICENSE",
			"PRODUCT_IMAGE",
			"file_record_count",
			"merchant_license_count",
			"license_file_privacy_postflight_passed",
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

func TestLicenseFilePrivacyMigrationOnlyClearsLicenseURLs(t *testing.T) {
	raw, err := os.ReadFile("0007_license_file_privacy.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	normalized := strings.Join(strings.Fields(string(raw)), " ")
	updates := regexp.MustCompile(`(?i)\bUPDATE\s+file_records\b`).FindAllStringIndex(normalized, -1)
	if len(updates) != 1 {
		t.Fatalf("file_records update count = %d, want 1", len(updates))
	}
	for _, forbidden := range []string{
		"PRODUCT_IMAGE",
		"owner_merchant_id =",
		"object_key =",
		"scan_status =",
		"updated_at =",
	} {
		if strings.Contains(normalized, forbidden) {
			t.Errorf("up migration contains forbidden mutation %q", forbidden)
		}
	}
}

func TestLicenseFilePrivacyMigrationHasNoDownScript(t *testing.T) {
	_, err := os.Stat("0007_license_file_privacy.down.sql")
	if !os.IsNotExist(err) {
		t.Fatalf("0007 down migration must not exist; stat error = %v", err)
	}
}
