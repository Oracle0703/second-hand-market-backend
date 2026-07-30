//go:build mysqlacceptance

package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const mysqlIdentityAcceptanceTimeout = 10 * time.Second

var mysqlAcceptanceIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type mysqlIdentityAcceptanceFixture struct {
	appDSN             string
	wrongUserDSN       string
	expectedServerUUID string
	decoySchema        string
	businessTable      string
	migrationTable     string
	bootstrapTable     string
	seedTable          string
	crudTable          string
	deniedCreateTable  string
	decoyCreateTable   string
}

func TestRemoteDevelopmentDatabaseIdentityMySQLAcceptance(t *testing.T) {
	fixture := loadMySQLIdentityAcceptanceFixture(t)

	t.Run("exact_identity", func(t *testing.T) {
		database := openMySQLIdentityAcceptanceDatabase(t, fixture.appDSN)
		defer database.Close()

		ctx, cancel := context.WithTimeout(context.Background(), mysqlIdentityAcceptanceTimeout)
		defer cancel()
		if err := verifyRemoteDevelopmentDatabaseIdentity(
			ctx,
			&sqlDatabaseIdentityQuerier{db: database},
			fixture.identityConfig(),
		); err != nil {
			t.Fatal("exact isolated MySQL identity was rejected")
		}
	})

	t.Run("production_startup_failures_close_with_zero_writes", func(t *testing.T) {
		assertMySQLIdentityAcceptanceMarkersEmpty(t, fixture)

		parsedAppDSN, err := mysqldriver.ParseDSN(fixture.appDSN)
		if err != nil {
			t.Fatal("isolated MySQL application connection configuration is invalid")
		}
		wrongDatabaseDSN := parsedAppDSN.Clone()
		wrongDatabaseDSN.DBName = "information_schema"
		emptyDatabaseDSN := parsedAppDSN.Clone()
		emptyDatabaseDSN.DBName = ""

		tests := []struct {
			name       string
			dsn        string
			mutate     func(*Config)
			closeFirst bool
			field      string
		}{
			{
				name:  "wrong_database",
				dsn:   wrongDatabaseDSN.FormatDSN(),
				field: "DB_EXPECTED_DATABASE",
			},
			{
				name:  "empty_database",
				dsn:   emptyDatabaseDSN.FormatDSN(),
				field: "DATABASE_IDENTITY",
			},
			{
				name: "wrong_server_uuid",
				dsn:  fixture.appDSN,
				mutate: func(cfg *Config) {
					cfg.DBExpectedServerUUID = alternateAcceptanceServerUUID(
						cfg.DBExpectedServerUUID,
					)
				},
				field: "DB_EXPECTED_SERVER_UUID",
			},
			{
				name:  "wrong_user",
				dsn:   fixture.wrongUserDSN,
				field: "DB_EXPECTED_USER",
			},
			{
				name:       "identity_query_error",
				dsn:        fixture.appDSN,
				closeFirst: true,
				field:      "DATABASE_IDENTITY",
			},
		}
		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				sqlDatabase := openMySQLIdentityAcceptanceDatabase(t, testCase.dsn)
				defer sqlDatabase.Close()
				cfg := fixture.identityConfig()
				cfg.AppEnv = appEnvTest
				cfg.DBTarget = dbTargetRemoteDevelopment
				cfg.DBDriver = "mysql"
				cfg.DBDSN = fixture.appDSN
				cfg.BuyerWechatLoginMode = buyerLoginModeMock
				cfg.BuyerDouyinLoginMode = buyerLoginModeMock
				cfg.FileStorageProvider = "local"
				cfg.ImageProcessorDriver = "passthrough"
				if testCase.mutate != nil {
					testCase.mutate(&cfg)
				}
				if testCase.closeFirst {
					if err := sqlDatabase.Close(); err != nil {
						t.Fatal("failed to prepare isolated identity query failure")
					}
				}
				gormDatabase := newMySQLIdentityAcceptanceGORM(t, sqlDatabase)
				openCalls := 0
				verifyCalls := 0
				closeAttempts := 0
				deps := serverStartupDependencies{
					openDB: func(Config) (*gorm.DB, error) {
						openCalls++
						return gormDatabase, nil
					},
					verifyDatabaseIdentity: func(db *gorm.DB, cfg Config) error {
						verifyCalls++
						return verifyConnectedDatabaseIdentity(db, cfg)
					},
					closeDB: func(db *gorm.DB) {
						closeAttempts++
						closeDatabase(db)
					},
				}

				server, err := newServer(cfg, deps)
				if err == nil || !strings.Contains(err.Error(), testCase.field) {
					t.Fatal("isolated MySQL identity failure was not rejected")
				}
				if server != nil {
					t.Fatal("isolated MySQL identity failure returned a server")
				}
				if openCalls != 1 || verifyCalls != 1 || closeAttempts != 1 {
					t.Fatal("production startup did not fail closed at the identity phase")
				}
				ctx, cancel := context.WithTimeout(
					context.Background(),
					mysqlIdentityAcceptanceTimeout,
				)
				pingErr := sqlDatabase.PingContext(ctx)
				cancel()
				if !errors.Is(pingErr, sql.ErrConnDone) {
					t.Fatal("identity failure left its SQL connection open")
				}
				assertMySQLIdentityAcceptanceErrorRedacted(t, err, fixture)
				assertMySQLIdentityAcceptanceMarkersEmpty(t, fixture)
			})
		}

		assertMySQLIdentityAcceptanceMarkersEmpty(t, fixture)
	})

	t.Run("application_user_crud", func(t *testing.T) {
		database := openMySQLIdentityAcceptanceDatabase(t, fixture.appDSN)
		defer database.Close()
		ctx, cancel := context.WithTimeout(context.Background(), mysqlIdentityAcceptanceTimeout)
		defer cancel()
		table := mysqlAcceptanceQualifiedName(remoteDevelopmentDatabase, fixture.crudTable)

		var initialCount int
		if err := database.QueryRowContext(
			ctx,
			"SELECT COUNT(*) FROM "+table,
		).Scan(&initialCount); err != nil || initialCount != 0 {
			t.Fatal("application user could not read the isolated CRUD fixture")
		}
		if _, err := database.ExecContext(
			ctx,
			"INSERT INTO "+table+" (id, value) VALUES (?, ?)",
			1,
			"created",
		); err != nil {
			t.Fatal("application user INSERT was rejected")
		}
		if _, err := database.ExecContext(
			ctx,
			"UPDATE "+table+" SET value = ? WHERE id = ?",
			"updated",
			1,
		); err != nil {
			t.Fatal("application user UPDATE was rejected")
		}
		var value string
		if err := database.QueryRowContext(
			ctx,
			"SELECT value FROM "+table+" WHERE id = ?",
			1,
		).Scan(&value); err != nil || value != "updated" {
			t.Fatal("application user could not verify the isolated UPDATE")
		}
		if _, err := database.ExecContext(
			ctx,
			"DELETE FROM "+table+" WHERE id = ?",
			1,
		); err != nil {
			t.Fatal("application user DELETE was rejected")
		}
		var finalCount int
		if err := database.QueryRowContext(
			ctx,
			"SELECT COUNT(*) FROM "+table,
		).Scan(&finalCount); err != nil || finalCount != 0 {
			t.Fatal("application user CRUD fixture was not restored")
		}
	})

	t.Run("application_user_ddl_denied", func(t *testing.T) {
		database := openMySQLIdentityAcceptanceDatabase(t, fixture.appDSN)
		defer database.Close()
		queries := []string{
			"CREATE TABLE " + mysqlAcceptanceQualifiedName(
				remoteDevelopmentDatabase,
				fixture.deniedCreateTable,
			) + " (id BIGINT PRIMARY KEY)",
			"ALTER TABLE " + mysqlAcceptanceQualifiedName(
				remoteDevelopmentDatabase,
				fixture.crudTable,
			) + " ADD COLUMN denied_column BIGINT",
			"DROP TABLE " + mysqlAcceptanceQualifiedName(
				remoteDevelopmentDatabase,
				fixture.crudTable,
			),
		}
		for _, query := range queries {
			assertMySQLIdentityAcceptancePermissionDenied(
				t,
				database,
				fixture,
				query,
			)
		}
		assertMySQLIdentityAcceptanceDevelopmentDDLUnchanged(t, database, fixture)
	})

	t.Run("production_schema_decoy_denied", func(t *testing.T) {
		database := openMySQLIdentityAcceptanceDatabase(t, fixture.appDSN)
		defer database.Close()
		decoyTable := mysqlAcceptanceQualifiedName(fixture.decoySchema, fixture.crudTable)
		decoyCreateTable := mysqlAcceptanceQualifiedName(
			fixture.decoySchema,
			fixture.decoyCreateTable,
		)
		queries := []string{
			"SELECT value FROM " + decoyTable + " WHERE id = 1",
			"INSERT INTO " + decoyTable + " (id, value) VALUES (2, 'denied')",
			"UPDATE " + decoyTable + " SET value = 'denied' WHERE id = 1",
			"DELETE FROM " + decoyTable + " WHERE id = 1",
			"CREATE TABLE " + decoyCreateTable + " (id BIGINT PRIMARY KEY)",
			"ALTER TABLE " + decoyTable + " ADD COLUMN denied_column BIGINT",
			"DROP TABLE " + decoyTable,
		}
		for _, query := range queries {
			assertMySQLIdentityAcceptancePermissionDenied(
				t,
				database,
				fixture,
				query,
			)
		}
	})
}

func loadMySQLIdentityAcceptanceFixture(t *testing.T) mysqlIdentityAcceptanceFixture {
	t.Helper()
	fixture := mysqlIdentityAcceptanceFixture{
		appDSN:             readMySQLIdentityAcceptanceFile(t, "ISSUE9_MYSQL_APP_DSN_FILE"),
		wrongUserDSN:       readMySQLIdentityAcceptanceFile(t, "ISSUE9_MYSQL_WRONG_USER_DSN_FILE"),
		expectedServerUUID: readMySQLIdentityAcceptanceFile(t, "ISSUE9_MYSQL_SERVER_UUID_FILE"),
		decoySchema:        os.Getenv("ISSUE9_MYSQL_DECOY_SCHEMA"),
		businessTable:      os.Getenv("ISSUE9_MYSQL_BUSINESS_TABLE"),
		migrationTable:     os.Getenv("ISSUE9_MYSQL_MIGRATION_TABLE"),
		bootstrapTable:     os.Getenv("ISSUE9_MYSQL_BOOTSTRAP_TABLE"),
		seedTable:          os.Getenv("ISSUE9_MYSQL_SEED_TABLE"),
		crudTable:          os.Getenv("ISSUE9_MYSQL_CRUD_TABLE"),
		deniedCreateTable:  os.Getenv("ISSUE9_MYSQL_DENIED_CREATE_TABLE"),
		decoyCreateTable:   os.Getenv("ISSUE9_MYSQL_DECOY_CREATE_TABLE"),
	}
	for _, identifier := range []string{
		fixture.decoySchema,
		fixture.businessTable,
		fixture.migrationTable,
		fixture.bootstrapTable,
		fixture.seedTable,
		fixture.crudTable,
		fixture.deniedCreateTable,
		fixture.decoyCreateTable,
	} {
		if !mysqlAcceptanceIdentifierPattern.MatchString(identifier) {
			t.Fatal("isolated MySQL acceptance identifier is missing or invalid")
		}
	}
	if !canonicalNonZeroAcceptanceUUID(fixture.expectedServerUUID) {
		t.Fatal("isolated MySQL server identity is not a canonical non-zero UUID")
	}
	return fixture
}

func readMySQLIdentityAcceptanceFile(t *testing.T, environmentName string) string {
	t.Helper()
	path := os.Getenv(environmentName)
	if path == "" || !filepath.IsAbs(path) {
		t.Fatal("isolated MySQL acceptance file path is missing or invalid")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
		info.Size() <= 0 || info.Size() > 4096 {
		t.Fatal("isolated MySQL acceptance file is unavailable or has unsafe permissions")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("isolated MySQL acceptance file could not be read")
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		t.Fatal("isolated MySQL acceptance file is empty")
	}
	return value
}

func (fixture mysqlIdentityAcceptanceFixture) identityConfig() Config {
	return Config{
		DBExpectedDatabase:   remoteDevelopmentDatabase,
		DBExpectedServerUUID: fixture.expectedServerUUID,
		DBExpectedUser:       remoteDevelopmentExpectedUser,
	}
}

func openMySQLIdentityAcceptanceDatabase(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	database, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal("isolated MySQL connection could not be created")
	}
	database.SetMaxOpenConns(2)
	database.SetMaxIdleConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), mysqlIdentityAcceptanceTimeout)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		t.Fatal("isolated MySQL connection is unavailable")
	}
	return database
}

func newMySQLIdentityAcceptanceGORM(t *testing.T, sqlDatabase *sql.DB) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(
		gormmysql.New(gormmysql.Config{
			Conn:                      sqlDatabase,
			SkipInitializeWithVersion: true,
		}),
		&gorm.Config{DisableAutomaticPing: true},
	)
	if err != nil {
		t.Fatal("isolated MySQL GORM connection could not be created")
	}
	return database
}

func assertMySQLIdentityAcceptancePermissionDenied(
	t *testing.T,
	database *sql.DB,
	fixture mysqlIdentityAcceptanceFixture,
	query string,
	args ...any,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), mysqlIdentityAcceptanceTimeout)
	_, err := database.ExecContext(ctx, query, args...)
	cancel()
	if err == nil {
		t.Fatal("application user unexpectedly executed a denied database operation")
	}
	var mysqlError *mysqldriver.MySQLError
	if !errors.As(err, &mysqlError) ||
		(mysqlError.Number != 1044 && mysqlError.Number != 1142) {
		t.Fatal("database operation did not fail with a MySQL permission denial")
	}

	pingContext, pingCancel := context.WithTimeout(
		context.Background(),
		mysqlIdentityAcceptanceTimeout,
	)
	pingErr := database.PingContext(pingContext)
	pingCancel()
	if pingErr != nil {
		t.Fatal("database connection became unhealthy after a permission denial")
	}

	identityContext, identityCancel := context.WithTimeout(
		context.Background(),
		mysqlIdentityAcceptanceTimeout,
	)
	identityErr := verifyRemoteDevelopmentDatabaseIdentity(
		identityContext,
		&sqlDatabaseIdentityQuerier{db: database},
		fixture.identityConfig(),
	)
	identityCancel()
	if identityErr != nil {
		t.Fatal("database identity changed after a permission denial")
	}
}

func assertMySQLIdentityAcceptanceDevelopmentDDLUnchanged(
	t *testing.T,
	database *sql.DB,
	fixture mysqlIdentityAcceptanceFixture,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), mysqlIdentityAcceptanceTimeout)
	defer cancel()
	var rowCount int
	if err := database.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM "+mysqlAcceptanceQualifiedName(
			remoteDevelopmentDatabase,
			fixture.crudTable,
		),
	).Scan(&rowCount); err != nil || rowCount != 0 {
		t.Fatal("denied DDL changed the isolated CRUD fixture")
	}
	var deniedColumnCount int
	if err := database.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM information_schema.columns "+
			"WHERE table_schema = ? AND table_name = ? AND column_name = ?",
		remoteDevelopmentDatabase,
		fixture.crudTable,
		"denied_column",
	).Scan(&deniedColumnCount); err != nil || deniedColumnCount != 0 {
		t.Fatal("denied ALTER changed the isolated CRUD fixture")
	}
	var deniedTableCount int
	if err := database.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM information_schema.tables "+
			"WHERE table_schema = ? AND table_name = ?",
		remoteDevelopmentDatabase,
		fixture.deniedCreateTable,
	).Scan(&deniedTableCount); err != nil || deniedTableCount != 0 {
		t.Fatal("denied CREATE changed the isolated development schema")
	}
}

func assertMySQLIdentityAcceptanceMarkersEmpty(
	t *testing.T,
	fixture mysqlIdentityAcceptanceFixture,
) {
	t.Helper()
	database := openMySQLIdentityAcceptanceDatabase(t, fixture.appDSN)
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), mysqlIdentityAcceptanceTimeout)
	defer cancel()
	for _, table := range []string{
		fixture.businessTable,
		fixture.migrationTable,
		fixture.bootstrapTable,
		fixture.seedTable,
	} {
		var count int
		if err := database.QueryRowContext(
			ctx,
			"SELECT COUNT(*) FROM "+mysqlAcceptanceQualifiedName(
				remoteDevelopmentDatabase,
				table,
			),
		).Scan(&count); err != nil || count != 0 {
			t.Fatal("identity failure marker table is not empty")
		}
	}
}

func assertMySQLIdentityAcceptanceErrorRedacted(
	t *testing.T,
	err error,
	fixture mysqlIdentityAcceptanceFixture,
) {
	t.Helper()
	logLine := "failed to init server: " + err.Error()
	for _, protected := range []string{
		fixture.appDSN,
		fixture.wrongUserDSN,
		fixture.expectedServerUUID,
		fixture.decoySchema,
		remoteDevelopmentDatabase,
		remoteDevelopmentExpectedUser,
		remoteDevelopmentIdentityQuery,
		"127.0.0.1",
		"13307",
	} {
		if protected != "" && strings.Contains(logLine, protected) {
			t.Fatal("isolated MySQL identity error exposed protected connection details")
		}
	}
}

func mysqlAcceptanceQualifiedName(schema string, table string) string {
	return "`" + schema + "`.`" + table + "`"
}

func alternateAcceptanceServerUUID(actual string) string {
	const first = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	if actual != first {
		return first
	}
	return "11111111-2222-4333-8444-555555555555"
}

func canonicalNonZeroAcceptanceUUID(value string) bool {
	cfg := validRemoteDevelopmentConfig()
	cfg.DBExpectedServerUUID = value
	return validateRemoteDevelopmentDatabase(cfg) == nil
}
