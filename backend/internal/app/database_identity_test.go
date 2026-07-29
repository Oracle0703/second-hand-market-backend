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
}

func (q *fakeDatabaseIdentityQuerier) QueryRowContext(_ context.Context, query string, _ ...any) databaseIdentityRow {
	q.query = query
	return fakeDatabaseIdentityRow{querier: q}
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
			t.Fatalf("identity query = %q", querier.query)
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
			{name: "server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(q *fakeDatabaseIdentityQuerier) { q.serverUUID = "wrong-server-uuid" }},
			{name: "empty_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(q *fakeDatabaseIdentityQuerier) { q.serverUUID = "" }},
			{name: "user", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = "root@localhost" }},
			{name: "empty_user", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = "" }},
			{name: "user_without_host", field: "DB_EXPECTED_USER", mutate: func(q *fakeDatabaseIdentityQuerier) { q.currentUser = remoteDevelopmentExpectedUser }},
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
	for _, value := range forbidden {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("identity error leaked a protected value: %q", err)
		}
	}
}
