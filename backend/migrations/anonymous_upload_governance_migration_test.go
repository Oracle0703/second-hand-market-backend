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
			"upload governance preflight: file_records must use InnoDB",
			"upload governance preflight: quota guard table must use InnoDB",
			"upload governance preflight: 0007 merchant license URL remains public",
			"upload governance preflight: 0007 merchant license object key is invalid",
			"upload governance preflight: 0007 completed product image URL is empty",
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
			"upload governance migration: file_records must use InnoDB",
			"upload governance migration: quota guard table must use InnoDB",
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
			"upload governance postflight: file_records must use InnoDB",
			"upload governance postflight: quota guard table must use InnoDB",
			"upload governance postflight: 0007 merchant license URL remains public",
			"upload governance postflight: 0007 merchant license object key is invalid",
			"upload governance postflight: 0007 completed product image URL is empty",
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

func TestAnonymousUploadGovernanceGroupedIndexHavingProjectsNonUnique(t *testing.T) {
	tests := []struct {
		name            string
		groupedQueries  int
		aliasPredicates int
	}{
		{"0008_anonymous_upload_governance.preflight.sql", 2, 4},
		{"0008_anonymous_upload_governance.up.sql", 1, 2},
		{"0008_anonymous_upload_governance.postflight.sql", 1, 2},
	}
	aliasPredicate := regexp.MustCompile(`\bis_non_unique\s*=\s*[01]\b`)
	boundProjection := regexp.MustCompile(
		`select index_name, non_unique as is_non_unique from information_schema\.statistics[^;]*` +
			`group by index_name, non_unique having`,
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := os.ReadFile(tt.name)
			if err != nil {
				t.Fatalf("read %s: %v", tt.name, err)
			}
			normalized := strings.ToLower(strings.Join(strings.Fields(string(raw)), " "))
			grouped := strings.Count(normalized, "group by index_name, non_unique")
			if grouped != tt.groupedQueries {
				t.Fatalf("grouped index queries = %d, want %d", grouped, tt.groupedQueries)
			}
			projected := strings.Count(normalized,
				"select index_name, non_unique as is_non_unique from information_schema.statistics")
			if projected != tt.groupedQueries {
				t.Fatalf("projected grouped queries = %d, want %d", projected, tt.groupedQueries)
			}
			bound := len(boundProjection.FindAllString(normalized, -1))
			if bound != tt.groupedQueries {
				t.Fatalf("bound grouped projections = %d, want %d", bound, tt.groupedQueries)
			}
			if predicates := len(aliasPredicate.FindAllString(normalized, -1)); predicates != tt.aliasPredicates {
				t.Fatalf("HAVING alias predicates = %d, want %d", predicates, tt.aliasPredicates)
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

func TestAnonymousUploadGovernanceAcceptanceScriptContracts(t *testing.T) {
	tests := map[string][]string{
		"../../deploy/acceptance/anonymous-upload-governance-smoke.sh": {
			"ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM",
			"I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA",
			"secondhand-upload-governance-acceptance",
			"evidence/anonymous-upload-governance",
			"docker container ls -a --filter",
			"docker volume ls --filter",
			"docker network ls --filter",
			`[[ "$mysql_version" == 8.4.* ]]`,
			"0001_init.up.sql",
			"0002_buyer_domain.up.sql",
			"0003_buyer_auth_provider.up.sql",
			"0004_merchant_multi_stock.preflight.sql",
			"0005_file_records_table.preflight.sql",
			"0006_file_binding_ownership.preflight.sql",
			"0007_license_file_privacy.preflight.sql",
			"0008_anonymous_upload_governance.preflight.sql",
			"0008_anonymous_upload_governance.up.sql",
			"0008_anonymous_upload_governance.postflight.sql",
			`ERROR 1644 \(45000\)`,
			"skipped-0007-error.txt",
			"drifted-guard-engine",
			"historical-before.txt",
			"historical-after.txt",
			"TestUploadGovernanceMySQLConcurrencyAndCleanup",
			"UPLOAD_GOVERNANCE_MYSQL_TEST=1",
			"AUTO_MIGRATE=false",
			"AUTO_MIGRATE=true",
			"production-before.txt",
			"production-after.txt",
			"source-sha256.txt",
			"frontend/index.html",
			"frontend/tsconfig.json",
			"frontend/vite.config.ts",
			"frontend/vitest.config.ts",
			"sha256sum",
			"isolated anonymous upload governance acceptance passed",
			"resources retained for inspection under Compose project",
		},
		"../../Makefile": {
			"acceptance-anonymous-upload-governance-smoke:",
			"ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM",
			"I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA",
			"ACCEPTANCE_DB_ENGINE=mysql8.4",
			"./deploy/acceptance/anonymous-upload-governance-smoke.sh",
		},
		"../../deploy/acceptance/docker-compose.yml": {
			"FILE_UPLOAD_MAX_MB: \"10\"",
			"FILE_UPLOAD_MULTIPART_MAX_MB: \"11\"",
			"FILE_UPLOAD_IP_HASH_SECRET:",
			"FILE_UPLOAD_ANON_PRESIGN_PER_HOUR: \"20\"",
			"FILE_UPLOAD_ANON_ACTIVE_FILES: \"5\"",
			"FILE_UPLOAD_ANON_ACTIVE_MB: \"50\"",
			"FILE_UPLOAD_MERCHANT_QUOTA_MB: \"2048\"",
			"FILE_UPLOAD_GLOBAL_QUOTA_MB: \"20480\"",
			"TRUSTED_PROXY_CIDRS: none",
		},
		"../../deploy/acceptance/nginx.conf": {
			"client_max_body_size 11m;",
			"error_page 413 = @upload_too_large;",
			"location @upload_too_large",
			"default_type application/json;",
			`return 413 '{"code":10008,"message":"upload file too large","request_id":"$request_id"}';`,
		},
		"../../deploy/acceptance/prepare.sh": {
			"file_upload_ip_hash_secret",
			"openssl rand -hex 32",
			"FILE_UPLOAD_IP_HASH_SECRET=",
			"unset mysql_password mysql_root_password jwt_access_secret jwt_refresh_secret file_upload_ip_hash_secret",
		},
	}

	for path, snippets := range tests {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		text := string(raw)
		for _, snippet := range snippets {
			if !strings.Contains(text, snippet) {
				t.Errorf("%s missing %q", path, snippet)
			}
		}
	}
}

func TestAnonymousUploadGovernanceAcceptanceUsesCurrentMigrationChain(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/acceptance/anonymous-upload-governance-smoke.sh")
	if err != nil {
		t.Fatalf("read anonymous upload governance script: %v", err)
	}

	requireOrderedScriptSnippets(t, string(raw), []string{
		`run_0008 | tee "$evidence_dir/clean-migration.txt"`,
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql",
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql",
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql",
		`-e AUTO_MIGRATE=false \`,
		`bootstrap-admin go test ./internal/app -run '^TestUploadGovernanceMySQLConcurrencyAndCleanup$' -count=1 -v`,
	})
	// Catches adding a false-mode upload API run before 0009 postflight in the
	// clean historical-fixture phase.
	requireCurrentChainBeforeFirstFalseModeFocusedAPI(t, string(raw),
		"apply_chain_0001_0007\nseed_historical_rows",
		`mysql_file /acceptance/migrations/0008_anonymous_upload_governance.postflight.sql \`,
		"mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql",
		"-e AUTO_MIGRATE=false",
		`bootstrap-admin go test ./internal/app -run '^TestUploadGovernanceMySQLConcurrencyAndCleanup$' -count=1 -v`,
	)
}
