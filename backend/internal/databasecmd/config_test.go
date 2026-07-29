package databasecmd

import (
	"strings"
	"testing"
)

const databaseConfigSecretSentinel = "database-config-secret-sentinel"

func TestLoadConfigFailsClosedWithoutLeakingValues(t *testing.T) {
	t.Run("requires_driver", func(t *testing.T) {
		t.Setenv("DB_DRIVER", "")
		t.Setenv("DB_DSN", databaseConfigSecretSentinel)
		assertConfigError(t, "DB_DRIVER", databaseConfigSecretSentinel)
	})

	t.Run("requires_dsn", func(t *testing.T) {
		t.Setenv("DB_DRIVER", "sqlite")
		t.Setenv("DB_DSN", " \t ")
		assertConfigError(t, "DB_DSN", databaseConfigSecretSentinel)
	})

	t.Run("rejects_unknown_driver", func(t *testing.T) {
		const driverSentinel = "unknown-driver-secret-sentinel"
		t.Setenv("DB_DRIVER", driverSentinel)
		t.Setenv("DB_DSN", databaseConfigSecretSentinel)
		assertConfigError(t, "DB_DRIVER", driverSentinel, databaseConfigSecretSentinel)
	})
}

func TestLoadConfigPreservesExplicitDSN(t *testing.T) {
	const dsn = " file:database-config?mode=memory&cache=shared "
	t.Setenv("DB_DRIVER", " sqlite ")
	t.Setenv("DB_DSN", dsn)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Driver != "sqlite" {
		t.Fatalf("driver = %q", cfg.Driver)
	}
	if cfg.DSN != dsn {
		t.Fatalf("DSN was modified")
	}
}

func TestOpenDatabaseDoesNotLeakInvalidDSN(t *testing.T) {
	cfg := Config{
		Driver: "mysql",
		DSN:    "user:" + databaseConfigSecretSentinel + "@tcp(%zz)/database",
	}
	_, err := OpenDatabase(cfg)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_CONNECTION") {
		t.Fatalf("expected safe database connection error, got %v", err)
	}
	if strings.Contains(err.Error(), databaseConfigSecretSentinel) {
		t.Fatalf("database connection error leaked DSN: %q", err)
	}
}

func assertConfigError(t *testing.T, field string, forbidden ...string) {
	t.Helper()
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), field) {
		t.Fatalf("expected %s error, got %v", field, err)
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("config error leaked a protected value: %q", err)
		}
	}
}
