package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestFileRecordsMigrationArtifacts(t *testing.T) {
	tests := map[string][]string{
		"0005_file_records_table.preflight.sql": {
			"file_records_preflight",
			"table_name IN ('files', 'file_records')",
			"required columns are missing",
			"primary key on id is missing or drifted",
			"object_key uniqueness is missing",
			"biz_type,created_at index is missing",
			"file_records_preflight_passed",
			"SIGNAL SQLSTATE '45000'",
		},
		"0005_file_records_table.up.sql": {
			"file_records_migration",
			"table_name IN ('files', 'file_records')",
			"required columns are missing",
			"primary key on id is missing or drifted",
			"object_key uniqueness is missing",
			"biz_type,created_at index is missing",
			"RENAME TABLE files TO file_records",
			"file_records_migration_renamed",
			"file_records_migration_noop",
			"SIGNAL SQLSTATE '45000'",
		},
		"0005_file_records_table.postflight.sql": {
			"file_records_postflight",
			"table_name = 'file_records'",
			"required columns are missing",
			"primary key on id is missing or drifted",
			"object_key uniqueness is missing",
			"biz_type,created_at index is missing",
			"file_records_postflight_passed",
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

func TestFileRecordsUpRepeatsShapeValidationBeforeMutation(t *testing.T) {
	raw, err := os.ReadFile("0005_file_records_table.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	text := string(raw)

	renameAt := strings.Index(text, "RENAME TABLE files TO file_records")
	if renameAt < 0 {
		t.Fatal("up migration missing RENAME TABLE")
	}
	noopAt := strings.Index(text, "file_records_migration_noop")
	if noopAt < 0 {
		t.Fatal("up migration missing noop marker")
	}

	// Shape checks must appear before either approved mutation/success path.
	for _, snippet := range []string{
		"required columns are missing",
		"primary key on id is missing or drifted",
		"object_key uniqueness is missing",
		"biz_type,created_at index is missing",
	} {
		at := strings.Index(text, snippet)
		if at < 0 {
			t.Fatalf("up migration missing shape check %q", snippet)
		}
		if at > renameAt || at > noopAt {
			t.Fatalf("shape check %q must appear before rename/noop markers", snippet)
		}
	}
}

func TestFileRecordsMigrationHasNoDestructiveDownScript(t *testing.T) {
	_, err := os.Stat("0005_file_records_table.down.sql")
	if !os.IsNotExist(err) {
		t.Fatalf("0005 down migration must not exist; stat error = %v", err)
	}
}

func TestFileSchemaSmokeResetsBeforeCleanChain(t *testing.T) {
	// Historical 0001 down drops only `files`. The acceptance harness must
	// clear both candidate tables after drift cases and before rebuilding the
	// clean migration chain, or 0005 preflight sees both tables and fails.
	raw, err := os.ReadFile("../../deploy/acceptance/file-record-schema-smoke.sh")
	if err != nil {
		t.Fatalf("read acceptance smoke script: %v", err)
	}
	text := string(raw)

	driftMarker := "drift-canonical-index.txt"
	driftAt := strings.LastIndex(text, driftMarker)
	if driftAt < 0 {
		t.Fatal("smoke script missing canonical drift evidence path")
	}

	cleanChainAt := strings.Index(text[driftAt:], "# Clean full migration chain")
	if cleanChainAt < 0 {
		t.Fatal("smoke script missing clean full migration chain phase after drift cases")
	}
	cleanChainAt += driftAt
	phase := text[cleanChainAt:]

	resetAt := strings.Index(phase, "reset_file_tables")
	if resetAt < 0 {
		t.Fatal("smoke script must call reset_file_tables at the start of the clean chain phase")
	}
	apply0001At := strings.Index(phase, "apply_0001")
	if apply0001At < 0 {
		t.Fatal("smoke script missing apply_0001 in clean chain phase")
	}
	if resetAt > apply0001At {
		t.Fatal("reset_file_tables must run before apply_0001 in the clean chain phase")
	}

	// Fail closed if a leftover drift table remains after reset.
	between := phase[resetAt:apply0001At]
	if !strings.Contains(between, "table_name IN ('files','file_records')") || !strings.Contains(between, `== "0"`) {
		t.Fatal("clean chain must assert neither files nor file_records exists after reset")
	}
}
