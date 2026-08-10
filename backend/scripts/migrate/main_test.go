package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/databasecmd"
)

const migrationSecretSentinel = "migration-secret-sentinel"

func TestParseMigrationSelectionRequiresExactlyOneAllowlistedMigration(t *testing.T) {
	for _, args := range [][]string{
		{"--migration", "0001_init"},
		{"--migration=0002_buyer_domain"},
		{"--migration=0005_legacy_file_records_table"},
		{"--migration=0006_product_stock_adjustments"},
		{"--migration=0007_product_sold_out_state"},
	} {
		spec, err := parseMigrationSelection(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		if spec.ID == "" || len(spec.Sources) != 1 || spec.Sources[0].FileName == "" || spec.Sources[0].SHA256 == "" {
			t.Fatalf("incomplete migration spec: %+v", spec)
		}
	}

	invalid := []struct {
		name string
		args []string
	}{
		{name: "missing"},
		{name: "empty", args: []string{"--migration="}},
		{name: "unknown", args: []string{"--migration", migrationSecretSentinel}},
		{name: "down migration", args: []string{"--migration", "0001_init.down.sql"}},
		{name: "path traversal", args: []string{"--migration", "../../" + migrationSecretSentinel}},
		{name: "duplicate", args: []string{"--migration", "0001_init", "--migration", "0002_buyer_domain"}},
		{name: "extra positional", args: []string{"--migration", "0001_init", migrationSecretSentinel}},
		{name: "unknown flag", args: []string{"--source=" + migrationSecretSentinel}},
		{name: "surrounding whitespace", args: []string{"--migration", " 0001_init "}},
	}
	for _, testCase := range invalid {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseMigrationSelection(testCase.args)
			if err == nil {
				t.Fatal("expected selection to fail")
			}
			if strings.Contains(err.Error(), migrationSecretSentinel) {
				t.Fatalf("selection error leaked input: %q", err)
			}
		})
	}
}

func Test0004MigrationLoadsAllThreeStages(t *testing.T) {
	spec, err := parseMigrationSelection([]string{"--migration", "0004_merchant_multi_stock"})
	if err != nil {
		t.Fatalf("select 0004: %v", err)
	}

	statements, err := loadMigrationStatementsFromDir("../../migrations", spec)
	if err != nil {
		t.Fatalf("load 0004: %v", err)
	}
	if len(statements) != 23 {
		t.Fatalf("0004 statements = %d, want 23", len(statements))
	}
}

func Test0004MigrationSourcesHaveFixedOrderAndHashes(t *testing.T) {
	spec, err := parseMigrationSelection([]string{"--migration", "0004_merchant_multi_stock"})
	if err != nil {
		t.Fatalf("select 0004: %v", err)
	}
	want := []migrationSource{
		{FileName: "0004_merchant_multi_stock.preflight.sql", SHA256: "9b00fd6d32ef8e73d74fedbad154d99a584ebc5ef292d849b8776fddedf95865"},
		{FileName: "0004_merchant_multi_stock.up.sql", SHA256: "ec2713616fb266ba653d3babfa738e896525e53cad6d87417dc8e629b092b3f2"},
		{FileName: "0004_merchant_multi_stock.postflight.sql", SHA256: "c17ebf6c0595f15c9f7de8749b216cde2bc86fe57a3cd9b4984b8c6404288ae2"},
	}
	if len(spec.Sources) != len(want) {
		t.Fatalf("0004 sources = %d, want %d", len(spec.Sources), len(want))
	}
	for index, source := range spec.Sources {
		if source != want[index] {
			t.Fatalf("0004 source %d = %+v, want %+v", index, source, want[index])
		}
	}
}

func TestRunFailsBeforeDatabaseAccessWithoutValidSelection(t *testing.T) {
	dependencies := migrationDependencies{
		loadConfig: func() (databasecmd.Config, error) {
			t.Fatal("database config must not be loaded")
			return databasecmd.Config{}, nil
		},
		openDatabase: func(databasecmd.Config) (*gorm.DB, error) {
			t.Fatal("database must not be opened")
			return nil, nil
		},
		closeDatabase: func(*gorm.DB) {
			t.Fatal("database must not be closed")
		},
		loadStatements: func(migrationSpec) ([]string, error) {
			t.Fatal("migration source must not be loaded")
			return nil, nil
		},
		execute: func(*gorm.DB, string) error {
			t.Fatal("migration statement must not execute")
			return nil
		},
	}

	if _, err := run(nil, dependencies); err == nil {
		t.Fatal("missing migration selection succeeded")
	}
	if _, err := run([]string{"--migration", migrationSecretSentinel}, dependencies); err == nil {
		t.Fatal("unknown migration selection succeeded")
	}
}

func TestRunExecutesOnlyTheExplicitlySelectedMigration(t *testing.T) {
	var (
		loadedSpec migrationSpec
		executed   []string
		closed     int
	)
	database := &gorm.DB{}
	dependencies := migrationDependencies{
		loadConfig: func() (databasecmd.Config, error) {
			return databasecmd.Config{Driver: "mysql", DSN: migrationSecretSentinel}, nil
		},
		openDatabase: func(config databasecmd.Config) (*gorm.DB, error) {
			if config.Driver != "mysql" || config.DSN != migrationSecretSentinel {
				t.Fatalf("unexpected database config: %+v", config)
			}
			return database, nil
		},
		closeDatabase: func(actual *gorm.DB) {
			if actual != database {
				t.Fatal("closed a different database")
			}
			closed++
		},
		loadStatements: func(spec migrationSpec) ([]string, error) {
			loadedSpec = spec
			return []string{"selected statement one", "selected statement two"}, nil
		},
		execute: func(actual *gorm.DB, statement string) error {
			if actual != database {
				t.Fatal("executed against a different database")
			}
			executed = append(executed, statement)
			return nil
		},
	}

	migrationID, err := run([]string{"--migration", "0002_buyer_domain"}, dependencies)
	if err != nil {
		t.Fatalf("run selected migration: %v", err)
	}
	if migrationID != "0002_buyer_domain" || loadedSpec.ID != migrationID {
		t.Fatalf("migration selection changed: result=%q spec=%+v", migrationID, loadedSpec)
	}
	if strings.Join(executed, "|") != "selected statement one|selected statement two" {
		t.Fatalf("executed statements = %v", executed)
	}
	if closed != 1 {
		t.Fatalf("database close calls = %d, want 1", closed)
	}
}

func TestRunExecutes0004StagesInOrder(t *testing.T) {
	var executed []string
	dependencies := migrationDependencies{
		loadConfig: func() (databasecmd.Config, error) {
			return databasecmd.Config{Driver: "mysql", DSN: migrationSecretSentinel}, nil
		},
		openDatabase:  func(databasecmd.Config) (*gorm.DB, error) { return &gorm.DB{}, nil },
		closeDatabase: func(*gorm.DB) {},
		loadStatements: func(spec migrationSpec) ([]string, error) {
			want := []string{
				"0004_merchant_multi_stock.preflight.sql",
				"0004_merchant_multi_stock.up.sql",
				"0004_merchant_multi_stock.postflight.sql",
			}
			if len(spec.Sources) != len(want) {
				t.Fatalf("0004 sources = %d, want %d", len(spec.Sources), len(want))
			}
			for index, source := range spec.Sources {
				if source.FileName != want[index] {
					t.Fatalf("0004 source %d = %q, want %q", index, source.FileName, want[index])
				}
			}
			return []string{"preflight statement", "up statement", "postflight statement"}, nil
		},
		execute: func(_ *gorm.DB, statement string) error {
			executed = append(executed, statement)
			return nil
		},
	}

	if _, err := run([]string{"--migration", "0004_merchant_multi_stock"}, dependencies); err != nil {
		t.Fatalf("run 0004: %v", err)
	}
	if got := strings.Join(executed, "|"); got != "preflight statement|up statement|postflight statement" {
		t.Fatalf("0004 execution order = %q", got)
	}
}

func TestRunRejectsSQLiteWithoutOpeningDatabase(t *testing.T) {
	opened := false
	dependencies := migrationDependencies{
		loadConfig: func() (databasecmd.Config, error) {
			return databasecmd.Config{Driver: "sqlite", DSN: migrationSecretSentinel}, nil
		},
		openDatabase: func(databasecmd.Config) (*gorm.DB, error) {
			opened = true
			return nil, nil
		},
		closeDatabase: func(*gorm.DB) {},
		loadStatements: func(migrationSpec) ([]string, error) {
			return []string{"statement"}, nil
		},
		execute: func(*gorm.DB, string) error {
			t.Fatal("migration statement must not execute")
			return nil
		},
	}

	_, err := run([]string{"--migration", "0001_init"}, dependencies)
	if err == nil || !strings.Contains(err.Error(), "DB_DRIVER") {
		t.Fatalf("SQLite migration error = %v", err)
	}
	if opened {
		t.Fatal("SQLite database was opened")
	}
}

func TestRunStopsAtFirstFailureAndRedactsDetails(t *testing.T) {
	var executed []string
	closed := 0
	dependencies := migrationDependencies{
		loadConfig: func() (databasecmd.Config, error) {
			return databasecmd.Config{Driver: "mysql", DSN: migrationSecretSentinel}, nil
		},
		openDatabase: func(databasecmd.Config) (*gorm.DB, error) {
			return &gorm.DB{}, nil
		},
		closeDatabase: func(*gorm.DB) {
			closed++
		},
		loadStatements: func(migrationSpec) ([]string, error) {
			return []string{"first statement", migrationSecretSentinel, "third statement"}, nil
		},
		execute: func(_ *gorm.DB, statement string) error {
			executed = append(executed, statement)
			if statement == migrationSecretSentinel {
				return errors.New("driver exposed " + migrationSecretSentinel)
			}
			return nil
		},
	}

	_, err := run([]string{"--migration=0003_buyer_auth_provider"}, dependencies)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_MIGRATION") {
		t.Fatalf("migration failure = %v", err)
	}
	if strings.Contains(err.Error(), migrationSecretSentinel) {
		t.Fatalf("migration failure leaked details: %q", err)
	}
	if strings.Join(executed, "|") != "first statement|"+migrationSecretSentinel {
		t.Fatalf("execution did not stop at first failure: %v", executed)
	}
	if closed != 1 {
		t.Fatalf("database close calls = %d, want 1", closed)
	}
}

func TestRunRejectsInvalid0004SourcesBeforeDatabaseAccess(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		setup func(t *testing.T, directory string, spec migrationSpec)
	}{
		{
			name:  "missing",
			setup: func(t *testing.T, directory string, spec migrationSpec) {},
		},
		{
			name: "tampered",
			setup: func(t *testing.T, directory string, spec migrationSpec) {
				path := directory + string(os.PathSeparator) + spec.Sources[0].FileName
				if err := os.WriteFile(path, []byte("SELECT 1;\n"), 0o600); err != nil {
					t.Fatalf("write tampered source: %v", err)
				}
			},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, directory string, spec migrationSpec) {
				target := directory + string(os.PathSeparator) + "target.sql"
				path := directory + string(os.PathSeparator) + spec.Sources[0].FileName
				if err := os.WriteFile(target, []byte("SELECT 1;\n"), 0o600); err != nil {
					t.Fatalf("write symlink target: %v", err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			directory := t.TempDir()
			spec := migrationCatalog["0004_merchant_multi_stock"]
			testCase.setup(t, directory, spec)
			configLoaded := false
			dependencies := migrationDependencies{
				loadConfig: func() (databasecmd.Config, error) {
					configLoaded = true
					return databasecmd.Config{}, nil
				},
				openDatabase:  func(databasecmd.Config) (*gorm.DB, error) { t.Fatal("database must not be opened"); return nil, nil },
				closeDatabase: func(*gorm.DB) { t.Fatal("database must not be closed") },
				loadStatements: func(spec migrationSpec) ([]string, error) {
					return loadMigrationStatementsFromDir(directory, spec)
				},
				execute: func(*gorm.DB, string) error { t.Fatal("statement must not execute"); return nil },
			}

			if _, err := run([]string{"--migration", "0004_merchant_multi_stock"}, dependencies); err == nil {
				t.Fatal("invalid 0004 sources were accepted")
			}
			if configLoaded {
				t.Fatal("database config was loaded before source validation")
			}
		})
	}
}

func TestMigrationCatalogMatchesExistingSources(t *testing.T) {
	wantStatements := map[string]int{
		"0001_init":                      13,
		"0002_buyer_domain":              5,
		"0003_buyer_auth_provider":       2,
		"0004_image_backfill_ledger":     2,
		"0005_legacy_file_records_table": 1,
		"0006_product_stock_adjustments": 1,
		"0007_product_sold_out_state":    2,
	}
	for migrationID, expectedCount := range wantStatements {
		spec := migrationCatalog[migrationID]
		statements, err := loadMigrationStatementsFromDir("../../migrations", spec)
		if err != nil {
			t.Fatalf("load %s: %v", migrationID, err)
		}
		if len(statements) != expectedCount {
			t.Fatalf("%s statements = %d, want %d", migrationID, len(statements), expectedCount)
		}
	}
}

func TestProductSoldOutStateMigrationCatalog(t *testing.T) {
	const source = "UPDATE products SET status = 'OFF_SHELF' WHERE status = 'CLOSED';\n" +
		"UPDATE products SET status = 'OFF_SHELF' WHERE status = 'SOLD' AND stock > 0;\n"
	wantHash := sha256.Sum256([]byte(source))
	want := migrationSource{
		FileName: "0007_product_sold_out_state.up.sql",
		SHA256:   hex.EncodeToString(wantHash[:]),
	}

	spec, err := parseMigrationSelection([]string{"--migration", "0007_product_sold_out_state"})
	if err != nil {
		t.Fatalf("select 0007: %v", err)
	}
	if len(spec.Sources) != 1 || spec.Sources[0] != want {
		t.Fatalf("0007 sources = %+v, want [%+v]", spec.Sources, want)
	}

	statements, err := loadMigrationStatementsFromDir("../../migrations", spec)
	if err != nil {
		t.Fatalf("load 0007: %v", err)
	}
	wantStatements := []string{
		"UPDATE products SET status = 'OFF_SHELF' WHERE status = 'CLOSED'",
		"UPDATE products SET status = 'OFF_SHELF' WHERE status = 'SOLD' AND stock > 0",
	}
	if strings.Join(statements, "|") != strings.Join(wantStatements, "|") {
		t.Fatalf("0007 statements = %q, want %q", statements, wantStatements)
	}
}

func TestLegacyFileRecordsMigrationRenamesTableForCurrentModel(t *testing.T) {
	spec := migrationCatalog["0005_legacy_file_records_table"]
	statements, err := loadMigrationStatementsFromDir("../../migrations", spec)
	if err != nil {
		t.Fatalf("load legacy file records migration: %v", err)
	}
	source := strings.Join(statements, "\n")
	if !strings.Contains(source, "RENAME TABLE file_records TO files") {
		t.Fatalf("legacy migration must rename file_records to files, got: %s", source)
	}
}

func TestLoadMigrationStatementsRejectsModifiedOrLinkedSources(t *testing.T) {
	directory := t.TempDir()
	spec := migrationSpec{
		ID: "test",
		Sources: []migrationSource{{
			FileName: "test.up.sql",
			SHA256:   "b4e0497804e46e0a0b0b8c31975b062152d551bac49c3c2e80932567b4085dcd",
		}},
	}
	path := directory + string(os.PathSeparator) + spec.Sources[0].FileName
	if err := os.WriteFile(path, []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatalf("write migration fixture: %v", err)
	}
	if _, err := loadMigrationStatementsFromDir(directory, spec); err != nil {
		t.Fatalf("load exact migration fixture: %v", err)
	}

	if err := os.WriteFile(path, []byte("SELECT 2;\n"), 0o600); err != nil {
		t.Fatalf("modify migration fixture: %v", err)
	}
	if _, err := loadMigrationStatementsFromDir(directory, spec); err == nil {
		t.Fatal("modified migration source was accepted")
	}

	target := directory + string(os.PathSeparator) + "target.sql"
	if err := os.WriteFile(target, []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove migration fixture: %v", err)
	}
	if err := os.Symlink(target, path); err == nil {
		if _, err := loadMigrationStatementsFromDir(directory, spec); err == nil {
			t.Fatal("linked migration source was accepted")
		}
	}
}

func TestSplitSQLStatementsHandlesQuotedSemicolonsAndComments(t *testing.T) {
	source := `
-- comment containing ;
CREATE TABLE test (value VARCHAR(32) DEFAULT 'a;b');
/* another ; comment */
INSERT INTO test(value) VALUES ("c;d");
# final comment ;
SELECT ` + "`semi;colon`" + ` FROM test;
`
	statements, err := splitSQLStatements(source)
	if err != nil {
		t.Fatalf("split SQL: %v", err)
	}
	if len(statements) != 3 {
		t.Fatalf("statement count = %d, want 3: %v", len(statements), statements)
	}
	if _, err := splitSQLStatements("SELECT 'unterminated;"); err == nil {
		t.Fatal("unterminated SQL string was accepted")
	}
	if _, err := splitSQLStatements("/* unterminated"); err == nil {
		t.Fatal("unterminated SQL comment was accepted")
	}
}
