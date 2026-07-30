package app

import (
	"strings"
	"testing"
)

const (
	remoteDevelopmentTarget       = "remote-development"
	remoteDevelopmentDatabase     = "second_hand_market_dev"
	remoteDevelopmentServerUUID   = "11111111-2222-4333-8444-555555555555"
	remoteDevelopmentExpectedUser = "shm_dev_app"
	remoteDSNSentinelPassword     = "remote-dsn-sentinel-password"
)

func TestLoadConfigDatabaseSettings(t *testing.T) {
	t.Run("defaults_to_local_with_destructive_flags_off", func(t *testing.T) {
		for _, name := range []string{
			"DB_TARGET",
			"AUTO_MIGRATE",
			"SEED_DEFAULTS",
			"DB_EXPECTED_DATABASE",
			"DB_EXPECTED_SERVER_UUID",
			"DB_EXPECTED_USER",
		} {
			unsetEnvForTest(t, name)
		}
		t.Setenv("APP_ENV", appEnvTest)

		cfg := LoadConfig()
		if cfg.DBTarget != "local" {
			t.Fatalf("DB_TARGET = %q, want local", cfg.DBTarget)
		}
		if cfg.AutoMigrate {
			t.Fatal("AUTO_MIGRATE defaulted to true")
		}
		if cfg.SeedDefaults {
			t.Fatal("SEED_DEFAULTS defaulted to true")
		}
		if cfg.DBExpectedDatabase != "" || cfg.DBExpectedServerUUID != "" || cfg.DBExpectedUser != "" {
			t.Fatal("database identity defaults were not empty")
		}
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("local test defaults were rejected: %v", err)
		}
	})

	t.Run("loads_and_normalizes_remote_development_settings", func(t *testing.T) {
		t.Setenv("APP_ENV", appEnvTest)
		t.Setenv("DB_TARGET", "  REMOTE-DEVELOPMENT  ")
		t.Setenv("DB_DRIVER", "mysql")
		t.Setenv("DB_DSN", validRemoteDevelopmentDSN(""))
		t.Setenv("AUTO_MIGRATE", "false")
		t.Setenv("SEED_DEFAULTS", "FALSE")
		t.Setenv("DB_EXPECTED_DATABASE", remoteDevelopmentDatabase)
		t.Setenv("DB_EXPECTED_SERVER_UUID", remoteDevelopmentServerUUID)
		t.Setenv("DB_EXPECTED_USER", remoteDevelopmentExpectedUser)

		cfg := LoadConfig()
		if cfg.DBTarget != remoteDevelopmentTarget {
			t.Fatalf("DB_TARGET = %q", cfg.DBTarget)
		}
		if cfg.SeedDefaults {
			t.Fatal("SEED_DEFAULTS parsed FALSE as true")
		}
		if cfg.DBExpectedDatabase != remoteDevelopmentDatabase {
			t.Fatalf("DB_EXPECTED_DATABASE = %q", cfg.DBExpectedDatabase)
		}
		if cfg.DBExpectedServerUUID != remoteDevelopmentServerUUID {
			t.Fatalf("DB_EXPECTED_SERVER_UUID = %q", cfg.DBExpectedServerUUID)
		}
		if cfg.DBExpectedUser != remoteDevelopmentExpectedUser {
			t.Fatalf("DB_EXPECTED_USER = %q", cfg.DBExpectedUser)
		}
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("valid remote-development configuration was rejected: %v", err)
		}
	})
}

func TestLoadConfigRejectsInvalidDatabaseBooleansWithoutLeakingValues(t *testing.T) {
	t.Run("each_invalid_field", func(t *testing.T) {
		tests := []struct {
			name  string
			field string
			value string
		}{
			{name: "auto_migrate", field: "AUTO_MIGRATE", value: "invalid-auto-migrate-sentinel"},
			{name: "seed_defaults", field: "SEED_DEFAULTS", value: "invalid-seed-defaults-sentinel"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				unsetEnvForTest(t, "AUTO_MIGRATE")
				unsetEnvForTest(t, "SEED_DEFAULTS")
				t.Setenv("APP_ENV", appEnvTest)
				t.Setenv(tc.field, tc.value)

				assertDatabaseRuntimeError(t, LoadConfig(), tc.field, tc.value)
			})
		}
	})

	t.Run("reports_both_invalid_fields", func(t *testing.T) {
		const autoSentinel = "invalid-auto-migrate-sentinel"
		const seedSentinel = "invalid-seed-defaults-sentinel"
		t.Setenv("APP_ENV", appEnvTest)
		t.Setenv("AUTO_MIGRATE", autoSentinel)
		t.Setenv("SEED_DEFAULTS", seedSentinel)

		err := LoadConfig().ValidateRuntime()
		if err == nil {
			t.Fatal("expected invalid database booleans to be rejected")
		}
		for _, field := range []string{"AUTO_MIGRATE", "SEED_DEFAULTS"} {
			if !strings.Contains(err.Error(), field) {
				t.Fatalf("error %q does not identify %s", err, field)
			}
		}
		for _, sentinel := range []string{autoSentinel, seedSentinel} {
			if strings.Contains(err.Error(), sentinel) {
				t.Fatalf("boolean error leaked a value: %q", err)
			}
		}
	})
}

func TestDatabaseTargetRuntimeGuardrails(t *testing.T) {
	t.Run("rejects_unknown_target_without_echoing_it", func(t *testing.T) {
		const sentinel = "unknown-target-sentinel"
		cfg := localTestRuntimeConfig()
		cfg.DBTarget = sentinel
		assertDatabaseRuntimeError(t, cfg, "DB_TARGET", sentinel)
	})

	t.Run("accepts_normalized_remote_target", func(t *testing.T) {
		cfg := validRemoteDevelopmentConfig()
		cfg.DBTarget = "  REMOTE-DEVELOPMENT  "
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("normalized remote-development target was rejected: %v", err)
		}
	})

	t.Run("local_nonproduction_allows_empty_dsn", func(t *testing.T) {
		cfg := localTestRuntimeConfig()
		cfg.DBDSN = "  "
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("local test configuration with empty DB_DSN was rejected: %v", err)
		}
	})

	t.Run("production_requires_nonempty_dsn", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.DBDSN = "  "
		assertDatabaseRuntimeError(t, cfg, "DB_DSN", remoteDSNSentinelPassword)
	})

	t.Run("load_config_production_requires_explicit_dsn", func(t *testing.T) {
		unsetEnvForTest(t, "DB_DSN")
		t.Setenv("APP_ENV", appEnvProduction)
		t.Setenv("DB_TARGET", "local")
		t.Setenv("JWT_ACCESS_SECRET", testProductionAccessSecret)
		t.Setenv("JWT_REFRESH_SECRET", testProductionRefreshSecret)
		t.Setenv("BUYER_WECHAT_LOGIN_MODE", buyerLoginModeDisabled)
		t.Setenv("BUYER_DOUYIN_LOGIN_MODE", buyerLoginModeDisabled)

		assertDatabaseRuntimeError(t, LoadConfig(), "DB_DSN", remoteDSNSentinelPassword)
	})

	t.Run("remote_development_requires_nonempty_dsn", func(t *testing.T) {
		cfg := validRemoteDevelopmentConfig()
		cfg.DBDSN = "\t"
		assertDatabaseRuntimeError(t, cfg, "DB_DSN", remoteDSNSentinelPassword)
	})

	t.Run("production_rejects_auto_migrate_before_connecting", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.AutoMigrate = true
		assertDatabaseRuntimeError(t, cfg, "AUTO_MIGRATE", cfg.DBDSN)
	})

	t.Run("production_rejects_seed_defaults_before_connecting", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.SeedDefaults = true
		assertDatabaseRuntimeError(t, cfg, "SEED_DEFAULTS", cfg.DBDSN)
	})
}

func TestRemoteDevelopmentDatabaseRuntimeGuardrails(t *testing.T) {
	t.Run("accepts_exact_safe_configuration", func(t *testing.T) {
		if err := validRemoteDevelopmentConfig().ValidateRuntime(); err != nil {
			t.Fatalf("valid remote-development configuration was rejected: %v", err)
		}
	})

	t.Run("rejects_unsafe_non_dsn_settings", func(t *testing.T) {
		tests := []struct {
			name   string
			field  string
			mutate func(*Config)
		}{
			{name: "non_mysql_driver", field: "DB_DRIVER", mutate: func(cfg *Config) { cfg.DBDriver = "sqlite" }},
			{name: "non_exact_mysql_driver", field: "DB_DRIVER", mutate: func(cfg *Config) { cfg.DBDriver = "MySQL" }},
			{name: "auto_migrate_enabled", field: "AUTO_MIGRATE", mutate: func(cfg *Config) { cfg.AutoMigrate = true }},
			{name: "seed_defaults_enabled", field: "SEED_DEFAULTS", mutate: func(cfg *Config) { cfg.SeedDefaults = true }},
			{name: "wrong_expected_database", field: "DB_EXPECTED_DATABASE", mutate: func(cfg *Config) { cfg.DBExpectedDatabase = "second_hand_market" }},
			{name: "whitespace_expected_database", field: "DB_EXPECTED_DATABASE", mutate: func(cfg *Config) { cfg.DBExpectedDatabase = " second_hand_market_dev " }},
			{name: "empty_expected_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) { cfg.DBExpectedServerUUID = " \t" }},
			{name: "malformed_expected_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) { cfg.DBExpectedServerUUID = "not-a-server-uuid" }},
			{name: "noncanonical_expected_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) { cfg.DBExpectedServerUUID = "11111111222243338444555555555555" }},
			{name: "uppercase_expected_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) { cfg.DBExpectedServerUUID = "11111111-2222-4333-8444-AAAAAAAAAAAA" }},
			{name: "braced_expected_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) { cfg.DBExpectedServerUUID = "{11111111-2222-4333-8444-555555555555}" }},
			{name: "urn_expected_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) { cfg.DBExpectedServerUUID = "urn:uuid:11111111-2222-4333-8444-555555555555" }},
			{name: "whitespace_expected_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) { cfg.DBExpectedServerUUID = " " + remoteDevelopmentServerUUID }},
			{name: "zero_expected_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) { cfg.DBExpectedServerUUID = "00000000-0000-0000-0000-000000000000" }},
			{name: "wrong_expected_user", field: "DB_EXPECTED_USER", mutate: func(cfg *Config) { cfg.DBExpectedUser = "root" }},
			{name: "whitespace_expected_user", field: "DB_EXPECTED_USER", mutate: func(cfg *Config) { cfg.DBExpectedUser = " shm_dev_app " }},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				cfg := validRemoteDevelopmentConfig()
				tc.mutate(&cfg)
				assertDatabaseRuntimeError(t, cfg, tc.field, cfg.DBDSN, remoteDSNSentinelPassword)
			})
		}
	})

	t.Run("rejects_unsafe_dsn_shape", func(t *testing.T) {
		tests := []struct {
			name string
			dsn  string
		}{
			{name: "malformed", dsn: "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:13307)/second_hand_market_dev?tls=%zz"},
			{name: "unix_socket", dsn: "shm_dev_app:" + remoteDSNSentinelPassword + "@unix(/tmp/mysql.sock)/second_hand_market_dev"},
			{name: "empty_database", dsn: "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:13307)/"},
			{name: "wrong_host", dsn: "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(localhost:13307)/second_hand_market_dev"},
			{name: "ipv6_loopback", dsn: "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp([::1]:13307)/second_hand_market_dev"},
			{name: "wrong_port", dsn: "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:3306)/second_hand_market_dev"},
			{name: "missing_port", dsn: "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1)/second_hand_market_dev"},
			{name: "production_database", dsn: "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:13307)/second_hand_market"},
			{name: "wrong_user", dsn: "root:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:13307)/second_hand_market_dev"},
			{name: "empty_user", dsn: ":" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:13307)/second_hand_market_dev"},
			{name: "empty_password", dsn: "shm_dev_app@tcp(127.0.0.1:13307)/second_hand_market_dev"},
			{name: "whitespace_password", dsn: "shm_dev_app:   @tcp(127.0.0.1:13307)/second_hand_market_dev"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				cfg := validRemoteDevelopmentConfig()
				cfg.DBDSN = tc.dsn
				assertDatabaseRuntimeError(t, cfg, "DB_DSN", tc.dsn, remoteDSNSentinelPassword)
			})
		}
	})

	t.Run("rejects_dangerous_dsn_options", func(t *testing.T) {
		for _, option := range []string{
			"multiStatements=true",
			"allowAllFiles=true",
			"allowCleartextPasswords=true",
			"allowOldPasswords=true",
			"allowFallbackToPlaintext=true",
		} {
			t.Run(strings.TrimSuffix(option, "=true"), func(t *testing.T) {
				cfg := validRemoteDevelopmentConfig()
				cfg.DBDSN = validRemoteDevelopmentDSN(option)
				assertDatabaseRuntimeError(t, cfg, "DB_DSN", cfg.DBDSN, remoteDSNSentinelPassword)
			})
		}
	})
}

func localTestRuntimeConfig() Config {
	return Config{
		AppEnv:               appEnvTest,
		DBTarget:             "local",
		DBDriver:             "mysql",
		BuyerWechatLoginMode: buyerLoginModeMock,
		BuyerDouyinLoginMode: buyerLoginModeMock,
	}
}

func validRemoteDevelopmentConfig() Config {
	return Config{
		AppEnv:               appEnvTest,
		DBTarget:             remoteDevelopmentTarget,
		DBDriver:             "mysql",
		DBDSN:                validRemoteDevelopmentDSN(""),
		AutoMigrate:          false,
		SeedDefaults:         false,
		DBExpectedDatabase:   remoteDevelopmentDatabase,
		DBExpectedServerUUID: remoteDevelopmentServerUUID,
		DBExpectedUser:       remoteDevelopmentExpectedUser,
		BuyerWechatLoginMode: buyerLoginModeMock,
		BuyerDouyinLoginMode: buyerLoginModeMock,
	}
}

func validRemoteDevelopmentDSN(option string) string {
	dsn := "shm_dev_app:" + remoteDSNSentinelPassword +
		"@tcp(127.0.0.1:13307)/second_hand_market_dev?charset=utf8mb4&parseTime=true"
	if option != "" {
		dsn += "&" + option
	}
	return dsn
}

func assertDatabaseRuntimeError(t *testing.T, cfg Config, field string, forbidden ...string) {
	t.Helper()
	err := cfg.ValidateRuntime()
	if err == nil {
		t.Fatalf("expected %s to be rejected", field)
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("error for %s leaked a protected value", field)
		}
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("error does not identify %s", field)
	}
}
