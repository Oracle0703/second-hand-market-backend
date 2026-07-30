package app

import (
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
)

const remoteDSNQuerySentinel = "remote-query-value-sentinel"

func TestRemoteDevelopmentDSNGuardAcceptsOnlyCanonicalSafeParameters(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "no_parameters"},
		{name: "charset", query: "charset=utf8mb4"},
		{name: "parse_time", query: "parseTime=true"},
		{name: "local_timezone", query: "loc=Local"},
		{name: "complete_example", query: "charset=utf8mb4&parseTime=true&loc=Local"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validRemoteDevelopmentConfig()
			cfg.DBDSN = remoteDevelopmentDSNWithQuery(tc.query)
			if err := cfg.ValidateRuntime(); err != nil {
				t.Fatal("canonical remote-development DSN was rejected")
			}
		})
	}
}

func TestRemoteDevelopmentDSNGuardRejectsDuplicateParameters(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "same_safe_value", query: "parseTime=true&parseTime=true"},
		{name: "conflicting_safe_value", query: "parseTime=true&parseTime=false"},
		{name: "case_variant", query: "parseTime=true&PARSETIME=true"},
		{name: "custom_parameter", query: "sql_mode=ANSI&sql_mode=TRADITIONAL"},
		{name: "dangerous_false_then_true", query: "multiStatements=false&multiStatements=true"},
		{name: "dangerous_true_then_false", query: "allowAllFiles=true&allowAllFiles=false"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validRemoteDevelopmentConfig()
			cfg.DBDSN = remoteDevelopmentDSNWithQuery(tc.query)
			assertRemoteDevelopmentRejectedBeforeOpenDB(t, cfg, "DB_DSN")
		})
	}
}

func TestRemoteDevelopmentDSNGuardRejectsUnsafeOrUnapprovedParameters(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "multi_statements_true", query: "multiStatements=true"},
		{name: "multi_statements_false", query: "multiStatements=false"},
		{name: "allow_all_files_true", query: "allowAllFiles=true"},
		{name: "allow_all_files_false", query: "allowAllFiles=false"},
		{name: "cleartext_passwords", query: "allowCleartextPasswords=true"},
		{name: "old_passwords", query: "allowOldPasswords=true"},
		{name: "plaintext_fallback", query: "allowFallbackToPlaintext=true"},
		{name: "local_infile_camel", query: "localInfile=true"},
		{name: "local_infile_snake", query: "local_infile=ON"},
		{name: "allow_local_infile", query: "allowLocalInfile=true"},
		{name: "load_local_infile", query: "loadLocalInfile=true"},
		{name: "allow_load_local_infile", query: "allowLoadLocalInfile=true"},
		{name: "strict_parser_panic", query: "strict=true"},
		{name: "session_variable", query: "sql_mode=" + remoteDSNQuerySentinel},
		{name: "autocommit", query: "autocommit=0"},
		{name: "time_zone", query: "time_zone=" + remoteDSNQuerySentinel},
		{name: "tls_skip_verify", query: "tls=skip-verify"},
		{name: "server_public_key", query: "serverPubKey=" + remoteDSNQuerySentinel},
		{name: "unsafe_charset", query: "charset=latin1"},
		{name: "disabled_parse_time", query: "parseTime=false"},
		{name: "unapproved_timezone", query: "loc=UTC"},
		{name: "noncanonical_key_case", query: "PARSETIME=true"},
		{name: "encoded_key", query: "parse%54ime=true"},
		{name: "encoded_value", query: "charset=utf8%6d%62%34"},
		{name: "missing_equals", query: "parseTime"},
		{name: "empty_key", query: "=true"},
		{name: "trailing_separator", query: "parseTime=true&"},
		{name: "empty_segment", query: "parseTime=true&&loc=Local"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validRemoteDevelopmentConfig()
			cfg.DBDSN = remoteDevelopmentDSNWithQuery(tc.query)
			assertRemoteDevelopmentRejectedBeforeOpenDB(t, cfg, "DB_DSN")
		})
	}
}

func TestNewServerRejectsInvalidRemoteDevelopmentConfigurationBeforeOpenDB(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		mutate func(*Config)
	}{
		{name: "non_mysql_driver", field: "DB_DRIVER", mutate: func(cfg *Config) { cfg.DBDriver = "sqlite" }},
		{name: "auto_migrate", field: "AUTO_MIGRATE", mutate: func(cfg *Config) { cfg.AutoMigrate = true }},
		{name: "seed_defaults", field: "SEED_DEFAULTS", mutate: func(cfg *Config) { cfg.SeedDefaults = true }},
		{name: "hostname", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(localhost:13307)/second_hand_market_dev"
		}},
		{name: "ipv6", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp([::1]:13307)/second_hand_market_dev"
		}},
		{name: "unix_socket", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = "shm_dev_app:" + remoteDSNSentinelPassword + "@unix(/tmp/mysql.sock)/second_hand_market_dev"
		}},
		{name: "wrong_port", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:3306)/second_hand_market_dev"
		}},
		{name: "empty_database", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:13307)/"
		}},
		{name: "production_database", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = "shm_dev_app:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:13307)/second_hand_market"
		}},
		{name: "wrong_dsn_user", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = "root:" + remoteDSNSentinelPassword + "@tcp(127.0.0.1:13307)/second_hand_market_dev"
		}},
		{name: "empty_dsn_password", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = "shm_dev_app@tcp(127.0.0.1:13307)/second_hand_market_dev"
		}},
		{name: "whitespace_dsn_password", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = "shm_dev_app:   @tcp(127.0.0.1:13307)/second_hand_market_dev"
		}},
		{name: "duplicate_parameter", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = remoteDevelopmentDSNWithQuery("parseTime=true&parseTime=false")
		}},
		{name: "empty_query", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = remoteDevelopmentDSNWithQuery("") + "?"
		}},
		{name: "dangerous_parameter", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = remoteDevelopmentDSNWithQuery("multiStatements=true")
		}},
		{name: "local_infile_parameter", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = remoteDevelopmentDSNWithQuery("local_infile=ON")
		}},
		{name: "unapproved_parameter", field: "DB_DSN", mutate: func(cfg *Config) {
			cfg.DBDSN = remoteDevelopmentDSNWithQuery("sql_mode=" + remoteDSNQuerySentinel)
		}},
		{name: "expected_database", field: "DB_EXPECTED_DATABASE", mutate: func(cfg *Config) {
			cfg.DBExpectedDatabase = "second_hand_market"
		}},
		{name: "expected_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) {
			cfg.DBExpectedServerUUID = "not-a-server-uuid"
		}},
		{name: "noncanonical_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) {
			cfg.DBExpectedServerUUID = "11111111222243338444555555555555"
		}},
		{name: "uppercase_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) {
			cfg.DBExpectedServerUUID = "11111111-2222-4333-8444-AAAAAAAAAAAA"
		}},
		{name: "braced_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) {
			cfg.DBExpectedServerUUID = "{11111111-2222-4333-8444-555555555555}"
		}},
		{name: "whitespace_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) {
			cfg.DBExpectedServerUUID = " " + remoteDevelopmentServerUUID
		}},
		{name: "nil_server_uuid", field: "DB_EXPECTED_SERVER_UUID", mutate: func(cfg *Config) {
			cfg.DBExpectedServerUUID = "00000000-0000-0000-0000-000000000000"
		}},
		{name: "expected_user", field: "DB_EXPECTED_USER", mutate: func(cfg *Config) {
			cfg.DBExpectedUser = "root"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validRemoteDevelopmentConfig()
			tc.mutate(&cfg)
			assertRemoteDevelopmentRejectedBeforeOpenDB(t, cfg, tc.field)
		})
	}
}

func TestNewServerRejectsInvalidRemoteDevelopmentBooleansBeforeOpenDB(t *testing.T) {
	for _, field := range []string{"AUTO_MIGRATE", "SEED_DEFAULTS"} {
		t.Run(strings.ToLower(field), func(t *testing.T) {
			t.Setenv("APP_ENV", appEnvTest)
			t.Setenv("DB_TARGET", dbTargetRemoteDevelopment)
			t.Setenv("DB_DRIVER", "mysql")
			t.Setenv("DB_DSN", validRemoteDevelopmentDSN(""))
			t.Setenv("DB_EXPECTED_DATABASE", remoteDevelopmentDatabase)
			t.Setenv("DB_EXPECTED_SERVER_UUID", remoteDevelopmentServerUUID)
			t.Setenv("DB_EXPECTED_USER", remoteDevelopmentExpectedUser)
			t.Setenv("AUTO_MIGRATE", "false")
			t.Setenv("SEED_DEFAULTS", "false")
			t.Setenv("BUYER_WECHAT_LOGIN_MODE", buyerLoginModeMock)
			t.Setenv("BUYER_DOUYIN_LOGIN_MODE", buyerLoginModeMock)
			t.Setenv(field, "invalid-boolean-sentinel")

			assertRemoteDevelopmentRejectedBeforeOpenDB(
				t,
				LoadConfig(),
				field,
				"invalid-boolean-sentinel",
			)
		})
	}
}

func TestNewServerValidRemoteDevelopmentConfigurationReachesOpenDBOnce(t *testing.T) {
	cfg := validRemoteDevelopmentConfig()
	openCalls := 0
	deps := serverStartupDependencies{
		openDB: func(Config) (*gorm.DB, error) {
			openCalls++
			return nil, errors.New("openDB control tripwire")
		},
		verifyDatabaseIdentity: func(*gorm.DB, Config) error {
			t.Fatal("identity check must not run after the control open error")
			return nil
		},
		closeDB: func(*gorm.DB) {
			t.Fatal("close must not run for a nil control database")
		},
	}

	_, err := newServer(cfg, deps)
	if err == nil {
		t.Fatal("expected the openDB control tripwire to stop startup")
	}
	if openCalls != 1 {
		t.Fatalf("openDB control calls = %d, want 1", openCalls)
	}
}

func remoteDevelopmentDSNWithQuery(query string) string {
	dsn := "shm_dev_app:" + remoteDSNSentinelPassword +
		"@tcp(127.0.0.1:13307)/second_hand_market_dev"
	if query != "" {
		dsn += "?" + query
	}
	return dsn
}

func assertRemoteDevelopmentRejectedBeforeOpenDB(
	t *testing.T,
	cfg Config,
	field string,
	additionalForbidden ...string,
) {
	t.Helper()
	openCalls := 0
	deps := serverStartupDependencies{
		openDB: func(Config) (*gorm.DB, error) {
			openCalls++
			return nil, errors.New("openDB rejection tripwire")
		},
		verifyDatabaseIdentity: func(*gorm.DB, Config) error {
			t.Fatal("identity check must not run for rejected remote-development configuration")
			return nil
		},
		closeDB: func(*gorm.DB) {
			t.Fatal("close must not run before openDB")
		},
	}

	_, err := newServer(cfg, deps)
	if err == nil {
		t.Fatalf("expected %s to be rejected", field)
	}
	forbidden := []string{
		cfg.DBDSN,
		remoteDSNSentinelPassword,
		remoteDSNQuerySentinel,
	}
	if queryIndex := strings.LastIndexByte(cfg.DBDSN, '?'); queryIndex >= 0 {
		forbidden = append(forbidden, cfg.DBDSN[queryIndex+1:])
	}
	forbidden = append(forbidden, additionalForbidden...)
	for _, value := range forbidden {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("error for %s leaked a protected value", field)
		}
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("error does not identify %s", field)
	}
	if openCalls != 0 {
		t.Fatalf("openDB was called for rejected %s configuration", field)
	}
}
