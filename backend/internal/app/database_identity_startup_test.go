package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestNewServerVerifiesRemoteIdentityBeforeDatabaseWrites(t *testing.T) {
	t.Run("identity_failure_stops_before_migrate_and_seed", func(t *testing.T) {
		cfg := validRemoteDevelopmentConfig()
		calls := []string{}
		deps := serverStartupDependencies{
			openDB: func(Config) (*gorm.DB, error) {
				calls = append(calls, "open")
				return &gorm.DB{}, nil
			},
			verifyDatabaseIdentity: func(*gorm.DB, Config) error {
				calls = append(calls, "identity")
				return errors.New("DB_EXPECTED_SERVER_UUID identity check failed")
			},
			closeDB: func(*gorm.DB) {
				calls = append(calls, "close")
			},
		}

		_, err := newServer(cfg, deps)
		if err == nil || !strings.Contains(err.Error(), "DB_EXPECTED_SERVER_UUID") {
			t.Fatalf("expected database identity rejection, got %v", err)
		}
		if !reflect.DeepEqual(calls, []string{"open", "identity", "close"}) {
			t.Fatalf("startup calls = %v", calls)
		}
	})

	t.Run("successful_identity_check_does_not_run_database_writes", func(t *testing.T) {
		cfg := validRemoteDevelopmentConfig()
		cfg.FileStorageProvider = "local"
		cfg.FileUploadLocalDir = t.TempDir()
		cfg.ImageProcessorDriver = "passthrough"
		calls := []string{}
		deps := serverStartupDependencies{
			openDB: func(Config) (*gorm.DB, error) {
				calls = append(calls, "open")
				return &gorm.DB{}, nil
			},
			verifyDatabaseIdentity: func(*gorm.DB, Config) error {
				calls = append(calls, "identity")
				return nil
			},
			closeDB: func(*gorm.DB) {
				calls = append(calls, "close")
			},
		}

		srv, err := newServer(cfg, deps)
		if err != nil {
			t.Fatalf("new server failed: %v", err)
		}
		if srv == nil {
			t.Fatal("new server returned nil")
		}
		if !reflect.DeepEqual(calls, []string{"open", "identity"}) {
			t.Fatalf("startup calls = %v", calls)
		}
	})
}

func TestNewServerRejectsDatabaseWriteFlagsBeforeOpeningDatabase(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		mutate func(*Config)
	}{
		{
			name:  "auto_migrate",
			field: "AUTO_MIGRATE",
			mutate: func(cfg *Config) {
				cfg.AutoMigrate = true
			},
		},
		{
			name:  "seed_defaults",
			field: "SEED_DEFAULTS",
			mutate: func(cfg *Config) {
				cfg.SeedDefaults = true
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := safeProductionRuntimeConfig()
			tc.mutate(&cfg)
			openCalls := 0
			deps := serverStartupDependencies{
				openDB: func(Config) (*gorm.DB, error) {
					openCalls++
					return nil, errors.New("database must not be opened")
				},
				verifyDatabaseIdentity: func(*gorm.DB, Config) error {
					t.Fatal("database identity must not be checked")
					return nil
				},
				closeDB: func(*gorm.DB) {
					t.Fatal("database close must not be called")
				},
			}

			_, err := newServer(cfg, deps)
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("expected %s rejection, got %v", tc.field, err)
			}
			if strings.Contains(err.Error(), cfg.DBDSN) {
				t.Fatalf("%s rejection leaked DB_DSN: %q", tc.field, err)
			}
			if openCalls != 0 {
				t.Fatalf("database opened %d times", openCalls)
			}
		})
	}
}

func TestNewServerRedactsDatabaseOpenErrors(t *testing.T) {
	const (
		passwordSentinel = "database-open-password-sentinel"
		dsnSentinel      = "user:" + passwordSentinel + "@tcp(127.0.0.1:3306)/database"
	)
	cfg := localTestRuntimeConfig()
	cfg.DBDSN = dsnSentinel
	cfg.FileStorageProvider = "local"
	cfg.FileUploadLocalDir = t.TempDir()
	cfg.ImageProcessorDriver = "passthrough"
	closeCalls := 0
	deps := serverStartupDependencies{
		openDB: func(Config) (*gorm.DB, error) {
			return &gorm.DB{}, errors.New("driver rejected " + dsnSentinel)
		},
		verifyDatabaseIdentity: func(*gorm.DB, Config) error {
			t.Fatal("database identity must not be checked after an open error")
			return nil
		},
		closeDB: func(*gorm.DB) {
			closeCalls++
		},
	}

	_, err := newServer(cfg, deps)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_CONNECTION") {
		t.Fatalf("expected redacted database connection error, got %v", err)
	}
	for _, forbidden := range []string{dsnSentinel, passwordSentinel} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("database connection error leaked a protected value: %q", err)
		}
	}
	if closeCalls != 1 {
		t.Fatalf("partially opened database closed %d times, want 1", closeCalls)
	}
}

func TestVerifyConnectedDatabaseIdentitySkipsLocalTarget(t *testing.T) {
	cfg := localTestRuntimeConfig()
	if err := verifyConnectedDatabaseIdentity(nil, cfg); err != nil {
		t.Fatalf("local database unexpectedly required remote identity: %v", err)
	}
}
