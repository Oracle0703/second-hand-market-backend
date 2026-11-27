package tests

import (
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mysqlconfig "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"second-hand-market-backend/backend/internal/databasecmd"
)

// Opt-in only. The loopback analytics_ci connection must be able to create
// temporary databases. Run the actual migrations and HTTP suite in a new schema.
func TestCustomAnalyticsExploreMySQL(t *testing.T) {
	if os.Getenv("ANALYTICS_MYSQL_TEST") != "1" {
		t.Skip("set ANALYTICS_MYSQL_TEST=1 and ANALYTICS_MYSQL_DSN for a local MySQL 8 fixture")
	}
	connection, err := mysqlconfig.ParseDSN(os.Getenv("ANALYTICS_MYSQL_DSN"))
	if err != nil {
		t.Fatal("invalid local analytics fixture DSN")
	}
	host, _, err := net.SplitHostPort(connection.Addr)
	if err != nil || connection.Net != "tcp" || (host != "127.0.0.1" && host != "localhost" && host != "::1") || connection.DBName != "analytics_ci" {
		t.Fatal("analytics tests require a loopback TCP connection to the analytics_ci database")
	}
	control, err := sql.Open("mysql", connection.FormatDSN())
	if err != nil {
		t.Fatal("cannot open analytics fixture")
	}
	t.Cleanup(func() { _ = control.Close() })
	name := "analytics_ci_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = control.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatalf("could not create isolated analytics schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := control.Exec("DROP DATABASE " + name); err != nil {
			t.Errorf("remove test-created schema: %v", err)
		}
	})
	connection.DBName = name
	connection.ParseTime = true
	db, err := sql.Open("mysql", connection.FormatDSN())
	if err != nil {
		t.Fatal("cannot open isolated schema")
	}
	t.Cleanup(func() { _ = db.Close() })
	migrate := func(file string) {
		t.Helper()
		src, err := os.ReadFile(filepath.Join("..", "migrations", file))
		if err != nil {
			t.Fatal(err)
		}
		statements, err := databasecmd.SplitSQLStatements(string(src))
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", file, err)
			}
		}
	}
	migrate("0012_miniapp_analytics.up.sql")
	if _, err := db.Exec(`INSERT INTO miniapp_analytics_events
        (merchant_id,event_id,platform,device_key,name,page,source,occurred_at_ms,event_date,created_at)
        VALUES(98765,'before','tt','device','page_view','pages/home/index','',0,'1970-01-01',NOW())`); err != nil {
		t.Fatal(err)
	}
	migrate("0013_analytics_properties.up.sql")
	migrate("0013_analytics_properties.up.sql")
	var legacyRows int
	if err := db.QueryRow("SELECT COUNT(*) FROM miniapp_analytics_events WHERE event_id='before' AND properties IS NULL").Scan(&legacyRows); err != nil || legacyRows != 1 {
		t.Fatalf("legacy rows %d, err %v", legacyRows, err)
	}
	cfg := newTestConfig(t, t.TempDir())
	cfg.DBDriver = "mysql"
	cfg.DBDSN = connection.FormatDSN()
	server := newTestServerWithConfig(t, cfg)
	pool, err := server.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	checkAnalyticsExplorer(t, server)
}
