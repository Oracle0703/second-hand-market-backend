package app

import (
	"strings"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/model"
)

func securityTestConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		AppEnv:                   "development",
		Addr:                     ":0",
		DBDriver:                 "sqlite",
		DBDSN:                    "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		JWTAccessSecret:          "test-access",
		JWTRefreshSecret:         "test-refresh",
		AccessTTL:                time.Hour,
		RefreshTTL:               24 * time.Hour,
		AutoMigrate:              true,
		FileStorageProvider:      "local",
		FileUploadLocalDir:       t.TempDir(),
		ImageProcessorDriver:     "passthrough",
		FileUploadMaxBytes:       1024,
		ImageCompressTargetBytes: 512,
	}
}

func TestNewServerDoesNotSeedAdministrators(t *testing.T) {
	srv, err := NewServer(securityTestConfig(t))
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	var count int64
	if err := srv.DB.Model(&model.AdminUser{}).Count(&count).Error; err != nil {
		t.Fatalf("count administrators: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no seeded administrators, got %d", count)
	}
}

func TestProductionConfigRejectsDefaultsAndRequiresBootstrap(t *testing.T) {
	cfg := securityTestConfig(t)
	cfg.AppEnv = "production"
	cfg.JWTAccessSecret = defaultJWTAccessSecret
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected default access secret to be rejected")
	}

	cfg.JWTAccessSecret = "production-access-secret-not-default"
	cfg.JWTRefreshSecret = "production-refresh-secret-not-default"
	_, err := NewServer(cfg)
	if err == nil || !strings.Contains(err.Error(), "no administrator") {
		t.Fatalf("expected bootstrap error, got %v", err)
	}
}

func TestProductionConfigRejectsExampleSecretsAndDatabasePasswords(t *testing.T) {
	cfg := securityTestConfig(t)
	cfg.AppEnv = "production"
	cfg.JWTAccessSecret = "replace-with-a-strong-access-secret"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected example access secret to be rejected")
	}

	cfg.JWTAccessSecret = "production-access-secret-not-default"
	cfg.JWTRefreshSecret = "replace-with-a-strong-refresh-secret"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected example refresh secret to be rejected")
	}

	cfg.JWTRefreshSecret = "production-refresh-secret-not-default"
	cfg.DBDriver = "mysql"
	for index, dsn := range []string{
		defaultDBDSN,
		"shm:" + defaultDBPassword + "@tcp(db.internal:3306)/second_hand_market?parseTime=true",
		"shm:replace-with-a-strong-db-password@tcp(db.internal:3306)/second_hand_market?parseTime=true",
	} {
		cfg.DBDSN = dsn
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected unsafe production DSN case %d to be rejected", index)
		}
	}
}

func TestOrderModelUsesNonUniqueProductActiveIndex(t *testing.T) {
	srv, err := NewServer(securityTestConfig(t))
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	indexes, err := srv.DB.Migrator().GetIndexes(&model.Order{})
	if err != nil {
		t.Fatalf("load order indexes: %v", err)
	}
	foundActiveIndex := false
	for _, index := range indexes {
		switch index.Name() {
		case "uk_product_active":
			t.Fatal("legacy unique product-active index must not be recreated")
		case "idx_order_product_active":
			foundActiveIndex = true
			if unique, ok := index.Unique(); ok && unique {
				t.Fatal("product-active lookup index must be non-unique")
			}
		}
	}
	if !foundActiveIndex {
		t.Fatal("product-active lookup index was not created")
	}
}
