package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const identityErrorSentinel = "identity-error-secret-sentinel"

type fakeDatabaseIdentityQuerier struct {
	database    string
	serverUUID  string
	currentUser string
	err         error
	query       string
	queryCalls  int
	argsCount   int
}

func (q *fakeDatabaseIdentityQuerier) QueryRowContext(_ context.Context, query string, args ...any) databaseIdentityRow {
	q.query = query
	q.queryCalls++
	q.argsCount = len(args)
	return fakeDatabaseIdentityRow{querier: q}
}

func (q *fakeDatabaseIdentityQuerier) databaseIdentityAvailable() bool {
	return q != nil
}

type fakeDatabaseIdentityRow struct {
	querier *fakeDatabaseIdentityQuerier
}

func (r fakeDatabaseIdentityRow) Scan(dest ...any) error {
	if r.querier.err != nil {
		return r.querier.err
	}
	if len(dest) != 3 {
		return errors.New("unexpected identity destination count")
	}
	*dest[0].(*string) = r.querier.database
	*dest[1].(*string) = r.querier.serverUUID
	*dest[2].(*string) = r.querier.currentUser
	return nil
}

func TestVerifyRemoteDevelopmentDatabaseIdentity(t *testing.T) {
	t.Run("queries_and_accepts_exact_identity", func(t *testing.T) {
		cfg := validRemoteDevelopmentConfig()
		querier := validDatabaseIdentityQuerier(cfg)

		if err := verifyRemoteDevelopmentDatabaseIdentity(context.Background(), querier, cfg); err != nil {
			t.Fatalf("valid database identity was rejected: %v", err)
		}
		if querier.query != remoteDevelopmentIdentityQuery {
			t.Fatal("identity verification did not use the required read-only query")
		}
		if querier.queryCalls != 1 || querier.argsCount != 0 {
			t.Fatal("identity verification did not execute exactly one argument-free query")
		}
	})

	t.Run("rejects_query_failure_without_leaking_details", func(t *testing.T) {
		cfg := validRemoteDevelopmentConfig()
		querier := validDatabaseIdentityQuerier(cfg)
		querier.err = errors.New(identityErrorSentinel)

		assertDatabaseIdentityError(t, querier, cfg, "DATABASE_IDENTITY", identityErrorSentinel)
	})

	t.Run("rejects_each_identity_mismatch_without_echoing_values", func(t *testing.T) {
		tests := []struct {
			name   string
			field  string
			mutate func(*fakeDatabaseIdentityQuerier)
		}{
			{name: "database", field: "DB_EXPECTED_DATABASE", mutate: func(q *fakeDatabaseIdentityQuerier) { q.database = "second_hand_market" }},
			{name: "empty_database", field: "DB_EXPECTED_DATABASE", mutate: func(q *fakeDatabaseIdentityQuerier) { q.database = "" }},
			{name: "server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(q *fakeDatabaseIdentityQuerier) { q.serverUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee" }},
			{name: "empty_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(q *fakeDatabaseIdentityQuerier) { q.serverUUID = "" }},
			{name: "malformed_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(q *fakeDatabaseIdentityQuerier) { q.serverUUID = "invalid-server-uuid" }},
			{name: "zero_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(q *fakeDatabaseIdentityQuerier) { q.serverUUID = "00000000-0000-0000-0000-000000000000" }},
			{name: "noncanonical_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(q *fakeDatabaseIdentityQuerier) { q.serverUUID = "11111111222243338444555555555555" }},
			{name: "uppercase_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(q *fakeDatabaseIdentityQuerier) { q.serverUUID = "11111111-2222-4333-8444-AAAAAAAAAAAA" }},
			{name: "user", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = "root@localhost" }},
			{name: "empty_user", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = "" }},
			{name: "user_without_host", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = remoteDevelopmentExpectedUser }},
			{name: "user_with_empty_host", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = remoteDevelopmentExpectedUser + "@" }},
			{name: "user_with_multiple_separators", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = remoteDevelopmentExpectedUser + "@@localhost" }},
			{name: "user_with_whitespace_host", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = remoteDevelopmentExpectedUser + "@ \t" }},
			{name: "user_with_control_host", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = remoteDevelopmentExpectedUser + "@local\nhost" }},
			{name: "user_with_invalid_utf8_host", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) {
				q.currentUser = remoteDevelopmentExpectedUser + "@" + string([]byte{0xff})
			}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				cfg := validRemoteDevelopmentConfig()
				querier := validDatabaseIdentityQuerier(cfg)
				tc.mutate(querier)
				assertDatabaseIdentityError(
					t,
					querier,
					cfg,
					tc.field,
					querier.database,
					querier.serverUUID,
					querier.currentUser,
				)
			})
		}
	})

	t.Run("rejects_nil_context_or_connection", func(t *testing.T) {
		cfg := validRemoteDevelopmentConfig()
		for _, testCase := range []struct {
			name    string
			ctx     context.Context
			querier databaseIdentityQuerier
		}{
			{name: "nil_context", ctx: nil, querier: validDatabaseIdentityQuerier(cfg)},
			{name: "nil_connection", ctx: context.Background(), querier: nil},
			{name: "nil_typed_connection", ctx: context.Background(), querier: (*fakeDatabaseIdentityQuerier)(nil)},
			{name: "nil_sql_connection", ctx: context.Background(), querier: &sqlDatabaseIdentityQuerier{}},
			{name: "nil_typed_sql_connection", ctx: context.Background(), querier: (*sqlDatabaseIdentityQuerier)(nil)},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				err := verifyRemoteDevelopmentDatabaseIdentity(testCase.ctx, testCase.querier, cfg)
				if err == nil || !strings.Contains(err.Error(), "DATABASE_IDENTITY") {
					t.Fatal("unavailable identity connection was not rejected safely")
				}
			})
		}
	})
}

func validDatabaseIdentityQuerier(cfg Config) *fakeDatabaseIdentityQuerier {
	return &fakeDatabaseIdentityQuerier{
		database:    cfg.DBExpectedDatabase,
		serverUUID:  cfg.DBExpectedServerUUID,
		currentUser: cfg.DBExpectedUser + "@%",
	}
}

func assertDatabaseIdentityError(
	t *testing.T,
	querier databaseIdentityQuerier,
	cfg Config,
	field string,
	forbidden ...string,
) {
	t.Helper()
	err := verifyRemoteDevelopmentDatabaseIdentity(context.Background(), querier, cfg)
	if err == nil {
		t.Fatalf("expected %s identity check to fail", field)
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("identity error %q does not identify %s", err, field)
	}
	if fake, ok := querier.(*fakeDatabaseIdentityQuerier); ok && fake.queryCalls != 1 {
		t.Fatal("identity failure did not come from exactly one identity query")
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("identity error leaked a protected value: %q", err)
		}
	}
}
