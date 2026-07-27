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
			"LEFT(object_key, 17) <> 'merchant_license/'",
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
			"LEFT(object_key, 17) <> 'merchant_license/'",
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

func TestLicenseFilePrivacyAcceptanceScriptContracts(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/acceptance/license-file-privacy-smoke.sh")
	if err != nil {
		t.Fatalf("read license file privacy acceptance script: %v", err)
	}
	text := string(raw)
	for _, snippet := range []string{
		"LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM",
		"I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA",
		"secondhand-license-privacy-acceptance",
		`[[ "$mysql_version" == 8.4.* ]]`,
		"license_file_privacy_preflight_passed",
		"license_file_privacy_postflight_passed",
		`ERROR 1644 \(45000\)`,
		"TestLicenseFilePrivacyWithMigrationOnlyMySQL",
		"AUTO_MIGRATE=false",
		"AUTO_MIGRATE=true",
		"isolated license file privacy acceptance passed",
	} {
		if !strings.Contains(text, snippet) {
			t.Errorf("acceptance script missing %q", snippet)
		}
	}
}

func TestLicenseFilePrivacyAcceptanceUsesCurrentMigrationChain(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/acceptance/license-file-privacy-smoke.sh")
	if err != nil {
		t.Fatalf("read license file privacy acceptance script: %v", err)
	}

	requireOrderedScriptSnippets(t, string(raw), []string{
		`mysql_file /acceptance/migrations/0007_license_file_privacy.postflight.sql`,
		"mysql_file /acceptance/migrations/0008_anonymous_upload_governance.preflight.sql",
		"mysql_file /acceptance/migrations/0008_anonymous_upload_governance.up.sql",
		"mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql",
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql",
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql",
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql",
		`-e AUTO_MIGRATE=false \`,
		`bootstrap-admin go test ./tests -run '^TestLicenseFilePrivacyWithMigrationOnlyMySQL$' -count=1 -v`,
	})
}
