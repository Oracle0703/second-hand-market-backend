package migrations

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAnonymousUploadGovernanceMigrationArtifacts(t *testing.T) {
	tests := map[string][]string{
		"0008_anonymous_upload_governance.preflight.sql": {
			"anonymous_upload_governance_preflight",
			"table_name = 'file_records'",
			"table_name = 'files'",
			"owner_merchant_id",
			"capability_token_hash",
			"capability_expires_at",
			"size_bytes < 0",
			"source_ip_hash",
			"cleanup_after",
			"cleanup_claimed_at",
			"cleanup_claim_token",
			"cleanup_attempts",
			"idx_file_source_created",
			"idx_file_cleanup_candidate",
			"file_quota_guards",
			"SIGNAL SQLSTATE '45000'",
			"anonymous_upload_governance_preflight_passed",
		},
		"0008_anonymous_upload_governance.up.sql": {
			"ADD COLUMN source_ip_hash CHAR(64) NULL",
			"ADD COLUMN cleanup_after DATETIME(3) NULL",
			"ADD COLUMN cleanup_claimed_at DATETIME(3) NULL",
			"ADD COLUMN cleanup_claim_token CHAR(64) NULL",
			"ADD COLUMN cleanup_attempts INT UNSIGNED NOT NULL DEFAULT 0",
			"ADD INDEX idx_file_source_created (source_ip_hash, created_at)",
			"(uploader_type, owner_merchant_id, cleanup_after, cleanup_claimed_at)",
			"CREATE TABLE file_quota_guards",
			"PRIMARY KEY (id)",
			"UNIQUE KEY uk_file_quota_guard_name (guard_name)",
			"INSERT INTO file_quota_guards (id, guard_name) VALUES (1, 'file_records')",
			"SIGNAL SQLSTATE '45000'",
			"anonymous_upload_governance_migration_applied",
		},
		"0008_anonymous_upload_governance.postflight.sql": {
			"anonymous_upload_governance_postflight",
			"table_name = 'file_records'",
			"table_name = 'files'",
			"source_ip_hash",
			"cleanup_after",
			"cleanup_claimed_at",
			"cleanup_claim_token",
			"cleanup_attempts",
			"idx_file_source_created",
			"source_ip_hash,created_at",
			"idx_file_cleanup_candidate",
			"uploader_type,owner_merchant_id,cleanup_after,cleanup_claimed_at",
			"file_quota_guards",
			"guard_name = 'file_records'",
			"SIGNAL SQLSTATE '45000'",
			"anonymous_upload_governance_postflight_passed",
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

func TestAnonymousUploadGovernanceMigrationPreservesHistoricalRows(t *testing.T) {
	raw, err := os.ReadFile("0008_anonymous_upload_governance.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(string(raw)), " "))
	for _, forbidden := range []string{
		"update file_records set source_ip_hash",
		"update file_records set cleanup_after",
		"update file_records set cleanup_claimed_at",
		"update file_records set cleanup_claim_token",
	} {
		if strings.Contains(normalized, forbidden) {
			t.Errorf("up migration contains forbidden historical enrollment %q", forbidden)
		}
	}
	if regexp.MustCompile(`(?i)\bUPDATE\s+file_records\b`).Match(raw) {
		t.Error("up migration must not update historical file_records")
	}
}

func TestAnonymousUploadGovernanceMigrationHasNoDownScript(t *testing.T) {
	matches, err := filepath.Glob("0008*.down.sql")
	if err != nil {
		t.Fatalf("glob 0008 down migrations: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("0008 down migrations must not exist: %v", matches)
	}
}
