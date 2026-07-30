package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites(t *testing.T) {
	failureTests := []struct {
		name   string
		field  string
		mutate func(*startupIdentitySQLState)
	}{
		{name: "wrong_database", field: "DB_EXPECTED_DATABASE", mutate: func(state *startupIdentitySQLState) {
			state.database = "identity-database-sentinel"
		}},
		{name: "empty_database", field: "DB_EXPECTED_DATABASE", mutate: func(state *startupIdentitySQLState) {
			state.database = ""
		}},
		{name: "wrong_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(state *startupIdentitySQLState) {
			state.serverUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
		}},
		{name: "empty_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(state *startupIdentitySQLState) {
			state.serverUUID = ""
		}},
		{name: "malformed_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(state *startupIdentitySQLState) {
			state.serverUUID = "identity-server-uuid-sentinel"
		}},
		{name: "zero_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(state *startupIdentitySQLState) {
			state.serverUUID = "00000000-0000-0000-0000-000000000000"
		}},
		{name: "wrong_user", field: "DB_EXPECTED_USER", mutate: func(state *startupIdentitySQLState) {
			state.currentUser = "identity-user-sentinel@identity-host-sentinel"
		}},
		{name: "empty_user", field: "DB_EXPECTED_USER", mutate: func(state *startupIdentitySQLState) {
			state.currentUser = ""
		}},
		{name: "malformed_current_user", field: "DB_EXPECTED_USER", mutate: func(state *startupIdentitySQLState) {
			state.currentUser = remoteDevelopmentExpectedUser + "@@identity-host-sentinel"
		}},
		{name: "identity_query_error", field: "DATABASE_IDENTITY", mutate: func(state *startupIdentitySQLState) {
			state.queryErr = errors.New(identityErrorSentinel)
		}},
	}
	for _, testCase := range failureTests {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := validRemoteDevelopmentConfig()
			state := &startupIdentitySQLState{
				database:    cfg.DBExpectedDatabase,
				serverUUID:  cfg.DBExpectedServerUUID,
				currentUser: cfg.DBExpectedUser + "@%",
			}
			testCase.mutate(state)
			database := newStartupIdentityGORM(t, state)
			calls := []string{}
			closeAttempts := 0
			deps := serverStartupDependencies{
				openDB: func(Config) (*gorm.DB, error) {
					calls = append(calls, "open")
					return database, nil
				},
				verifyDatabaseIdentity: func(db *gorm.DB, cfg Config) error {
					calls = append(calls, "identity")
					return verifyConnectedDatabaseIdentity(db, cfg)
				},
				closeDB: func(db *gorm.DB) {
					calls = append(calls, "close")
					closeAttempts++
					closeDatabase(db)
				},
			}

			server, err := newServer(cfg, deps)
			if err == nil || !strings.Contains(err.Error(), testCase.field) {
				t.Fatal("expected database identity rejection")
			}
			if server != nil {
				t.Fatal("identity failure returned a server")
			}
			if !reflect.DeepEqual(calls, []string{"open", "identity", "close"}) {
				t.Fatal("database identity was not the only phase between open and close")
			}
			if closeAttempts != 1 {
				t.Fatal("identity failure did not close the opened database exactly once")
			}
			snapshot := state.snapshot()
			if snapshot.queryCalls != 1 ||
				snapshot.query != remoteDevelopmentIdentityQuery ||
				snapshot.queryArgs != 0 {
				t.Fatal("identity failure did not execute exactly the required read-only query")
			}
			if snapshot.execCalls != 0 {
				t.Fatal("identity failure executed a business, migration, bootstrap, or seed write")
			}
			if snapshot.closeCalls != 1 {
				t.Fatal("identity failure did not close the underlying SQL connection")
			}
			assertIdentityStartupErrorIsRedacted(t, err, state)
		})
	}

	t.Run("nil_or_unavailable_connection_fails_closed", func(t *testing.T) {
		for _, testCase := range []struct {
			name     string
			database *gorm.DB
		}{
			{name: "nil", database: nil},
			{name: "unavailable", database: &gorm.DB{}},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				cfg := validRemoteDevelopmentConfig()
				calls := []string{}
				closeAttempts := 0
				deps := serverStartupDependencies{
					openDB: func(Config) (*gorm.DB, error) {
						calls = append(calls, "open")
						return testCase.database, nil
					},
					verifyDatabaseIdentity: func(db *gorm.DB, cfg Config) error {
						calls = append(calls, "identity")
						return verifyConnectedDatabaseIdentity(db, cfg)
					},
					closeDB: func(*gorm.DB) {
						calls = append(calls, "close")
						closeAttempts++
					},
				}

				server, err := newServer(cfg, deps)
				if err == nil || !strings.Contains(err.Error(), "DATABASE_IDENTITY") {
					t.Fatal("unavailable identity connection was not rejected")
				}
				if server != nil {
					t.Fatal("unavailable identity connection returned a server")
				}
				if !reflect.DeepEqual(calls, []string{"open", "identity", "close"}) ||
					closeAttempts != 1 {
					t.Fatal("unavailable identity connection did not fail closed")
				}
				assertIdentityStartupErrorIsRedacted(t, err, nil)
			})
		}
	})

	t.Run("successful_identity_check_does_not_run_database_writes", func(t *testing.T) {
		cfg := validRemoteDevelopmentConfig()
		cfg.FileStorageProvider = "local"
		cfg.FileUploadLocalDir = t.TempDir()
		cfg.ImageProcessorDriver = "passthrough"
		calls := []string{}
		deps := serverStartupDependencies{
			openDB: func(Config) (*gorm.DB, error) {
				calls = append(calls, "open")
				return &gorm.DB{}, nil
			},
			verifyDatabaseIdentity: func(*gorm.DB, Config) error {
				calls = append(calls, "identity")
				return nil
			},
			closeDB: func(*gorm.DB) {
				calls = append(calls, "close")
			},
		}

		server, err := newServer(cfg, deps)
		if err != nil {
			t.Fatalf("new server failed: %v", err)
		}
		if server == nil {
			t.Fatal("new server returned nil")
		}
		if !reflect.DeepEqual(calls, []string{"open", "identity"}) {
			t.Fatalf("startup calls = %v", calls)
		}
	})
}

func TestNewServerRejectsDatabaseWriteFlagsBeforeOpeningDatabase(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		mutate func(*Config)
	}{
		{
			name:  "auto_migrate",
			field: "AUTO_MIGRATE",
			mutate: func(cfg *Config) {
				cfg.AutoMigrate = true
			},
		},
		{
			name:  "seed_defaults",
			field: "SEED_DEFAULTS",
			mutate: func(cfg *Config) {
				cfg.SeedDefaults = true
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := safeProductionRuntimeConfig()
			testCase.mutate(&cfg)
			openCalls := 0
			deps := serverStartupDependencies{
				openDB: func(Config) (*gorm.DB, error) {
					openCalls++
					return nil, errors.New("database must not be opened")
				},
				verifyDatabaseIdentity: func(*gorm.DB, Config) error {
					t.Fatal("database identity must not be checked")
					return nil
				},
				closeDB: func(*gorm.DB) {
					t.Fatal("database close must not be called")
				},
			}

			_, err := newServer(cfg, deps)
			if err == nil || !strings.Contains(err.Error(), testCase.field) {
				t.Fatalf("expected %s rejection, got %v", testCase.field, err)
			}
			if strings.Contains(err.Error(), cfg.DBDSN) {
				t.Fatalf("%s rejection leaked DB_DSN: %q", testCase.field, err)
			}
			if openCalls != 0 {
				t.Fatalf("database opened %d times", openCalls)
			}
		})
	}
}

func TestNewServerRedactsDatabaseOpenErrors(t *testing.T) {
	const (
		passwordSentinel = "database-open-password-sentinel"
		dsnSentinel      = "user:" + passwordSentinel + "@tcp(127.0.0.1:3306)/database"
	)
	cfg := localTestRuntimeConfig()
	cfg.DBDSN = dsnSentinel
	cfg.FileStorageProvider = "local"
	cfg.FileUploadLocalDir = t.TempDir()
	cfg.ImageProcessorDriver = "passthrough"
	closeCalls := 0
	deps := serverStartupDependencies{
		openDB: func(Config) (*gorm.DB, error) {
			return &gorm.DB{}, errors.New("driver rejected " + dsnSentinel)
		},
		verifyDatabaseIdentity: func(*gorm.DB, Config) error {
			t.Fatal("database identity must not be checked after an open error")
			return nil
		},
		closeDB: func(*gorm.DB) {
			closeCalls++
		},
	}

	_, err := newServer(cfg, deps)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_CONNECTION") {
		t.Fatalf("expected redacted database connection error, got %v", err)
	}
	for _, forbidden := range []string{dsnSentinel, passwordSentinel} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("database connection error leaked a protected value: %q", err)
		}
	}
	if closeCalls != 1 {
		t.Fatalf("partially opened database closed %d times, want 1", closeCalls)
	}
}

func TestVerifyConnectedDatabaseIdentitySkipsLocalTarget(t *testing.T) {
	cfg := localTestRuntimeConfig()
	if err := verifyConnectedDatabaseIdentity(nil, cfg); err != nil {
		t.Fatalf("local database unexpectedly required remote identity: %v", err)
	}
}

type startupIdentitySQLState struct {
	mu          sync.Mutex
	database    string
	serverUUID  string
	currentUser string
	queryErr    error
	query       string
	queryCalls  int
	queryArgs   int
	execCalls   int
	closeCalls  int
}

type startupIdentitySQLSnapshot struct {
	query      string
	queryCalls int
	queryArgs  int
	execCalls  int
	closeCalls int
}

func (state *startupIdentitySQLState) snapshot() startupIdentitySQLSnapshot {
	state.mu.Lock()
	defer state.mu.Unlock()
	return startupIdentitySQLSnapshot{
		query:      state.query,
		queryCalls: state.queryCalls,
		queryArgs:  state.queryArgs,
		execCalls:  state.execCalls,
		closeCalls: state.closeCalls,
	}
}

type startupIdentityConnector struct {
	state *startupIdentitySQLState
}

func (connector startupIdentityConnector) Connect(context.Context) (driver.Conn, error) {
	return &startupIdentityConnection{state: connector.state}, nil
}

func (connector startupIdentityConnector) Driver() driver.Driver {
	return startupIdentityDriver{}
}

type startupIdentityDriver struct{}

func (startupIdentityDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("DATABASE_IDENTITY test connector unavailable")
}

type startupIdentityConnection struct {
	state *startupIdentitySQLState
}

func (connection *startupIdentityConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("DATABASE_IDENTITY unexpected prepared statement")
}

func (connection *startupIdentityConnection) Close() error {
	connection.state.mu.Lock()
	defer connection.state.mu.Unlock()
	connection.state.closeCalls++
	return nil
}

func (connection *startupIdentityConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("DATABASE_IDENTITY unexpected transaction")
}

func (connection *startupIdentityConnection) Ping(context.Context) error {
	return nil
}

func (connection *startupIdentityConnection) QueryContext(
	_ context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Rows, error) {
	connection.state.mu.Lock()
	defer connection.state.mu.Unlock()
	connection.state.query = query
	connection.state.queryCalls++
	connection.state.queryArgs = len(args)
	if connection.state.queryErr != nil {
		return nil, connection.state.queryErr
	}
	return &startupIdentityRows{
		values: []driver.Value{
			connection.state.database,
			connection.state.serverUUID,
			connection.state.currentUser,
		},
	}, nil
}

func (connection *startupIdentityConnection) ExecContext(
	context.Context,
	string,
	[]driver.NamedValue,
) (driver.Result, error) {
	connection.state.mu.Lock()
	defer connection.state.mu.Unlock()
	connection.state.execCalls++
	return nil, errors.New("DATABASE_IDENTITY unexpected write")
}

type startupIdentityRows struct {
	values []driver.Value
	done   bool
}

func (rows *startupIdentityRows) Columns() []string {
	return []string{"database", "server_uuid", "current_user"}
}

func (rows *startupIdentityRows) Close() error {
	return nil
}

func (rows *startupIdentityRows) Next(destination []driver.Value) error {
	if rows.done {
		return io.EOF
	}
	copy(destination, rows.values)
	rows.done = true
	return nil
}

func newStartupIdentityGORM(t *testing.T, state *startupIdentitySQLState) *gorm.DB {
	t.Helper()
	sqlDB := sql.OpenDB(startupIdentityConnector{state: state})
	database, err := gorm.Open(
		gormmysql.New(gormmysql.Config{
			Conn:                      sqlDB,
			SkipInitializeWithVersion: true,
		}),
		&gorm.Config{DisableAutomaticPing: true},
	)
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal("failed to create isolated identity startup database")
	}
	return database
}

func assertIdentityStartupErrorIsRedacted(
	t *testing.T,
	err error,
	state *startupIdentitySQLState,
) {
	t.Helper()
	logLine := "failed to init server: " + err.Error()
	forbidden := []string{
		remoteDevelopmentIdentityQuery,
		remoteDevelopmentDatabase,
		remoteDevelopmentServerUUID,
		remoteDevelopmentExpectedUser,
		remoteDSNSentinelPassword,
		"127.0.0.1",
		"13307",
		identityErrorSentinel,
	}
	if state != nil {
		forbidden = append(
			forbidden,
			state.database,
			state.serverUUID,
			state.currentUser,
		)
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(logLine, value) {
			t.Fatal("database identity startup error exposed protected connection details")
		}
	}
}
