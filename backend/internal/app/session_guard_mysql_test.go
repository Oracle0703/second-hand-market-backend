//go:build mysqlacceptance

package app

import (
	"net"
	"os"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// This matrix uses only a disposable loopback MySQL 8.4 service. Missing or
// unexpected fixture configuration fails rather than silently skipping tests.
func TestSessionAuthorityMySQL(t *testing.T) {
	if os.Getenv("SESSION_AUTHORITY_MYSQL") != "1" {
		t.Fatal("explicit disposable MySQL fixture opt-in is required")
	}
	dsn := os.Getenv("SESSION_AUTHORITY_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid authority fixture DSN")
	}
	host, port, err := net.SplitHostPort(parsed.Addr)
	if err != nil || parsed.Net != "tcp" || host != "127.0.0.1" || port == "" ||
		parsed.DBName != "session_authority_ci" || parsed.User != "session_authority_ci" ||
		!parsed.ParseTime || parsed.Loc.String() != "UTC" ||
		parsed.Timeout <= 0 || parsed.ReadTimeout <= 0 || parsed.WriteTimeout <= 0 ||
		parsed.MultiStatements || parsed.AllowAllFiles || parsed.AllowCleartextPasswords ||
		parsed.TLSConfig != "" || len(parsed.Params) != 0 {
		t.Fatal("authority fixture must be the isolated loopback test database")
	}
	cfg := authorityTestConfig(t)
	cfg.DBDriver, cfg.DBDSN = "mysql", dsn
	db, err := openDB(cfg)
	if err != nil {
		t.Fatal("open authority MySQL fixture")
	}
	var identity struct{ Version, Database, User string }
	err = db.Raw("SELECT VERSION() AS version, DATABASE() AS `database`, CURRENT_USER() AS user").Scan(&identity).Error
	closeDatabase(db)
	if err != nil || !strings.HasPrefix(identity.Version, "8.4.") || identity.Database != "session_authority_ci" || strings.Split(identity.User, "@")[0] != "session_authority_ci" {
		t.Fatal("authority fixture identity or MySQL version mismatch")
	}
	runSessionAuthoritySuite(t, func(t *testing.T) *Server {
		cfg := authorityTestConfig(t)
		cfg.DBDriver, cfg.DBDSN = "mysql", dsn
		return newAuthorityTestServer(t, cfg)
	})
}
