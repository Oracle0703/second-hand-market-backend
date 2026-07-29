package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const remoteDevelopmentIdentityQuery = "SELECT DATABASE(), @@GLOBAL.server_uuid, CURRENT_USER();"

const remoteDevelopmentIdentityTimeout = 5 * time.Second

type databaseIdentityRow interface {
	Scan(dest ...any) error
}

type databaseIdentityQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) databaseIdentityRow
}

type sqlDatabaseIdentityQuerier struct {
	db *sql.DB
}

func (q sqlDatabaseIdentityQuerier) QueryRowContext(
	ctx context.Context,
	query string,
	args ...any,
) databaseIdentityRow {
	return q.db.QueryRowContext(ctx, query, args...)
}

func verifyConnectedDatabaseIdentity(db *gorm.DB, cfg Config) error {
	if normalizeDBTarget(cfg.DBTarget) != dbTargetRemoteDevelopment {
		return nil
	}
	if db == nil {
		return errors.New("DATABASE_IDENTITY connection unavailable")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.New("DATABASE_IDENTITY connection unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteDevelopmentIdentityTimeout)
	defer cancel()
	return verifyRemoteDevelopmentDatabaseIdentity(ctx, sqlDatabaseIdentityQuerier{db: sqlDB}, cfg)
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
