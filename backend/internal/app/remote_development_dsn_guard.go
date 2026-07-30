package app

import (
	"fmt"
	"net/url"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

var allowedRemoteDevelopmentDSNParameters = map[string]string{
	"charset":   "utf8mb4",
	"loc":       "Local",
	"parseTime": "true",
}

func validateRemoteDevelopmentDatabase(c Config) error {
	if c.DBDriver != "mysql" {
		return fmt.Errorf("DB_DRIVER must be mysql when DB_TARGET is remote-development")
	}
	if err := validateRemoteDevelopmentDSNParameters(c.DBDSN); err != nil {
		return err
	}

	dsn, err := mysqldriver.ParseDSN(c.DBDSN)
	if err != nil {
		return fmt.Errorf("DB_DSN must be a valid MySQL DSN for remote-development")
	}
	if dsn.Net != "tcp" ||
		dsn.Addr != remoteDevelopmentDBAddr ||
		dsn.DBName != remoteDevelopmentDBName ||
		dsn.User != remoteDevelopmentDBUser ||
		strings.TrimSpace(dsn.Passwd) == "" {
		return fmt.Errorf(
			"DB_DSN must use the required MySQL TCP target, database, user, and credential for remote-development",
		)
	}
	if dsn.MultiStatements ||
		dsn.AllowAllFiles ||
		dsn.AllowCleartextPasswords ||
		dsn.AllowOldPasswords ||
		dsn.AllowFallbackToPlaintext {
		return fmt.Errorf("DB_DSN must not enable unsafe MySQL options for remote-development")
	}

	if c.DBExpectedDatabase != remoteDevelopmentDBName {
		return fmt.Errorf("DB_EXPECTED_DATABASE must be %s for remote-development", remoteDevelopmentDBName)
	}
	expectedServerUUID, err := uuid.Parse(c.DBExpectedServerUUID)
	if err != nil ||
		expectedServerUUID == uuid.Nil ||
		expectedServerUUID.String() != c.DBExpectedServerUUID {
		return fmt.Errorf("DB_EXPECTED_SERVER_UUID must be a canonical non-zero UUID for remote-development")
	}
	if c.DBExpectedUser != remoteDevelopmentDBUser {
		return fmt.Errorf("DB_EXPECTED_USER must be %s for remote-development", remoteDevelopmentDBUser)
	}
	return nil
}

func validateRemoteDevelopmentDSNParameters(rawDSN string) error {
	lastSlash := strings.LastIndexByte(rawDSN, '/')
	if lastSlash < 0 {
		return nil
	}
	queryOffset := strings.IndexByte(rawDSN[lastSlash+1:], '?')
	if queryOffset < 0 {
		return nil
	}
	rawQuery := rawDSN[lastSlash+1+queryOffset+1:]
	if rawQuery == "" {
		return fmt.Errorf("DB_DSN must use well-formed MySQL parameters for remote-development")
	}

	seen := make(map[string]struct{})
	for _, parameter := range strings.Split(rawQuery, "&") {
		rawKey, rawValue, found := strings.Cut(parameter, "=")
		if !found || rawKey == "" {
			return fmt.Errorf("DB_DSN must use well-formed MySQL parameters for remote-development")
		}
		decodedKey, err := url.QueryUnescape(rawKey)
		if err != nil || decodedKey != rawKey {
			return fmt.Errorf("DB_DSN must use canonical MySQL parameter names for remote-development")
		}
		normalizedKey := strings.ToLower(decodedKey)
		if _, duplicate := seen[normalizedKey]; duplicate {
			return fmt.Errorf("DB_DSN must not contain duplicate MySQL parameters for remote-development")
		}
		seen[normalizedKey] = struct{}{}

		allowedValue, allowed := allowedRemoteDevelopmentDSNParameters[decodedKey]
		decodedValue, err := url.QueryUnescape(rawValue)
		if err != nil || !allowed || decodedValue != allowedValue || decodedValue != rawValue {
			return fmt.Errorf("DB_DSN contains a MySQL parameter that is not allowed for remote-development")
		}
	}
	return nil
}
