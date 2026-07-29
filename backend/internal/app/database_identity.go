package app

import (
	"context"
	"errors"
	"strings"
)

const remoteDevelopmentIdentityQuery = "SELECT DATABASE(), @@GLOBAL.server_uuid, CURRENT_USER();"

type databaseIdentityRow interface {
	Scan(dest ...any) error
}

type databaseIdentityQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) databaseIdentityRow
}

func verifyRemoteDevelopmentDatabaseIdentity(
	ctx context.Context,
	querier databaseIdentityQuerier,
	cfg Config,
) error {
	var database string
	var serverUUID string
	var currentUser string
	if err := querier.QueryRowContext(ctx, remoteDevelopmentIdentityQuery).Scan(
		&database,
		&serverUUID,
		&currentUser,
	); err != nil {
		return errors.New("DATABASE_IDENTITY query failed")
	}

	if database != cfg.DBExpectedDatabase {
		return errors.New("DB_EXPECTED_DATABASE identity check failed")
	}
	if serverUUID != cfg.DBExpectedServerUUID {
		return errors.New("DB_EXPECTED_SERVER_UUID identity check failed")
	}
	account, host, ok := strings.Cut(currentUser, "@")
	if !ok || host == "" || account != cfg.DBExpectedUser {
		return errors.New("DB_EXPECTED_USER identity check failed")
	}
	return nil
}
