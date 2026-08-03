package main

import (
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
	} {
		spec, err := parseMigrationSelection(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		if spec.ID == "" || spec.FileName == "" || spec.SHA256 == "" {
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

func TestMigrationCatalogMatchesExistingSources(t *testing.T) {
	wantStatements := map[string]int{
		"0001_init":                  13,
		"0002_buyer_domain":          5,
		"0003_buyer_auth_provider":   2,
		"0004_image_backfill_ledger": 2,
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

func TestLoadMigrationStatementsRejectsModifiedOrLinkedSources(t *testing.T) {
	directory := t.TempDir()
	spec := migrationSpec{
		ID:       "test",
		FileName: "test.up.sql",
		SHA256:   "b4e0497804e46e0a0b0b8c31975b062152d551bac49c3c2e80932567b4085dcd",
	}
	path := directory + string(os.PathSeparator) + spec.FileName
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
