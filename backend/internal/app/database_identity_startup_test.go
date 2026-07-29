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
			migrate: func(*gorm.DB) error {
				calls = append(calls, "migrate")
				return nil
			},
			seedDefaults: func(*gorm.DB) error {
				calls = append(calls, "seed")
				return nil
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

	t.Run("successful_identity_check_precedes_current_seed_path", func(t *testing.T) {
		cfg := validRemoteDevelopmentConfig()
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
			migrate: func(*gorm.DB) error {
				calls = append(calls, "migrate")
				return nil
			},
			seedDefaults: func(*gorm.DB) error {
				calls = append(calls, "seed")
				return errors.New("seed probe")
			},
		}

		_, err := newServer(cfg, deps)
		if err == nil || !strings.Contains(err.Error(), "seed probe") {
			t.Fatalf("expected seed probe, got %v", err)
		}
		if !reflect.DeepEqual(calls, []string{"open", "identity", "seed"}) {
			t.Fatalf("startup calls = %v", calls)
		}
	})
}

func TestVerifyConnectedDatabaseIdentitySkipsLocalTarget(t *testing.T) {
	cfg := localTestRuntimeConfig()
	if err := verifyConnectedDatabaseIdentity(nil, cfg); err != nil {
		t.Fatalf("local database unexpectedly required remote identity: %v", err)
	}
}
