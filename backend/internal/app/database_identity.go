package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const remoteDevelopmentIdentityQuery = "SELECT DATABASE(), @@GLOBAL.server_uuid, CURRENT_USER();"

const remoteDevelopmentIdentityTimeout = 5 * time.Second

type databaseIdentityRow interface {
	Scan(dest ...any) error
}

type databaseIdentityQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) databaseIdentityRow
	databaseIdentityAvailable() bool
}

type sqlDatabaseIdentityQuerier struct {
	db *sql.DB
}

func (q *sqlDatabaseIdentityQuerier) QueryRowContext(
	ctx context.Context,
	query string,
	args ...any,
) databaseIdentityRow {
	return q.db.QueryRowContext(ctx, query, args...)
}

func (q *sqlDatabaseIdentityQuerier) databaseIdentityAvailable() bool {
	return q != nil && q.db != nil
}

func verifyConnectedDatabaseIdentity(db *gorm.DB, cfg Config) error {
	if normalizeDBTarget(cfg.DBTarget) != dbTargetRemoteDevelopment {
		return nil
	}
	if db == nil || db.Config == nil {
		return errors.New("DATABASE_IDENTITY connection unavailable")
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		return errors.New("DATABASE_IDENTITY connection unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteDevelopmentIdentityTimeout)
	defer cancel()
	return verifyRemoteDevelopmentDatabaseIdentity(ctx, &sqlDatabaseIdentityQuerier{db: sqlDB}, cfg)
}

func verifyRemoteDevelopmentDatabaseIdentity(
	ctx context.Context,
	querier databaseIdentityQuerier,
	cfg Config,
) error {
	if ctx == nil || querier == nil || !querier.databaseIdentityAvailable() {
		return errors.New("DATABASE_IDENTITY connection unavailable")
	}

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
	parsedServerUUID, err := uuid.Parse(serverUUID)
	if err != nil ||
		parsedServerUUID == uuid.Nil ||
		parsedServerUUID.String() != serverUUID ||
		serverUUID != cfg.DBExpectedServerUUID {
		return errors.New("DB_EXPECTED_SERVER_UUID identity check failed")
	}
	if !validCurrentDatabaseUser(currentUser, cfg.DBExpectedUser) {
		return errors.New("DB_EXPECTED_USER identity check failed")
	}
	return nil
}

func validCurrentDatabaseUser(currentUser string, expectedAccount string) bool {
	if strings.Count(currentUser, "@") != 1 {
		return false
	}
	account, host, _ := strings.Cut(currentUser, "@")
	if account != expectedAccount || host == "" || len(host) > 255 || !utf8.ValidString(host) {
		return false
	}
	for _, character := range host {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	return true
}
