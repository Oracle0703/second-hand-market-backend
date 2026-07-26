package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"second-hand-market-backend/backend/internal/model"
)

func securityTestConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		AppEnv:                       "development",
		Addr:                         ":0",
		DBDriver:                     "sqlite",
		DBDSN:                        "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared",
		JWTAccessSecret:              "test-access",
		JWTRefreshSecret:             "test-refresh",
		AccessTTL:                    time.Hour,
		RefreshTTL:                   24 * time.Hour,
		AutoMigrate:                  true,
		FileStorageProvider:          "local",
		FileUploadLocalDir:           t.TempDir(),
		ImageProcessorDriver:         "passthrough",
		FileUploadMaxBytes:           10 * 1024 * 1024,
		FileUploadMultipartMaxBytes:  11 * 1024 * 1024,
		FileUploadIPHashSecret:       "test-only-upload-ip-hmac-secret-32-bytes",
		FileUploadAnonPresignPerHour: 20,
		FileUploadAnonActiveFiles:    5,
		FileUploadAnonActiveBytes:    50 * 1024 * 1024,
		FileUploadMerchantQuotaBytes: 2 * 1024 * 1024 * 1024,
		FileUploadGlobalQuotaBytes:   20 * 1024 * 1024 * 1024,
		FileUploadCleanupInterval:    5 * time.Minute,
		FileUploadCleanupBatchSize:   50,
		FileUploadCleanupClaimTTL:    10 * time.Minute,
		FileUploadCleanupGrace:       30 * time.Minute,
		TrustedProxyCIDRs:            nil,
		ImageCompressTargetBytes:     512,
	}
}

var uploadGovernanceEnv = map[string]string{
	"FILE_UPLOAD_MAX_MB":                    "10",
	"FILE_UPLOAD_MULTIPART_MAX_MB":          "11",
	"FILE_UPLOAD_IP_HASH_SECRET":            "production-upload-ip-hmac-secret-32-bytes",
	"FILE_UPLOAD_ANON_PRESIGN_PER_HOUR":     "20",
	"FILE_UPLOAD_ANON_ACTIVE_FILES":         "5",
	"FILE_UPLOAD_ANON_ACTIVE_MB":            "50",
	"FILE_UPLOAD_MERCHANT_QUOTA_MB":         "2048",
	"FILE_UPLOAD_GLOBAL_QUOTA_MB":           "20480",
	"FILE_UPLOAD_CLEANUP_INTERVAL_SECONDS":  "300",
	"FILE_UPLOAD_CLEANUP_BATCH_SIZE":        "50",
	"FILE_UPLOAD_CLEANUP_CLAIM_TTL_SECONDS": "600",
	"FILE_UPLOAD_CLEANUP_GRACE_SECONDS":     "1800",
	"TRUSTED_PROXY_CIDRS":                   "none",
}

func setUploadGovernanceEnv(t *testing.T) {
	t.Helper()
	for key, value := range uploadGovernanceEnv {
		t.Setenv(key, value)
	}
}

func TestProductionRequiresExplicitUploadGovernance(t *testing.T) {
	cfg := securityTestConfig(t)
	cfg.AppEnv = "production"
	cfg.uploadGovernanceEnvExplicit = false
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "upload governance") {
		t.Fatalf("expected explicit governance error, got %v", err)
	}

	for missing := range uploadGovernanceEnv {
		t.Run(missing, func(t *testing.T) {
			setUploadGovernanceEnv(t)
			t.Setenv("APP_ENV", "production")
			if err := os.Unsetenv(missing); err != nil {
				t.Fatalf("unset %s: %v", missing, err)
			}
			cfg := LoadConfig()
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "upload governance") {
				t.Fatalf("missing %s error = %v", missing, err)
			}
		})
	}
}

func TestProductionRejectsInvalidUploadGovernance(t *testing.T) {
	tests := map[string]func(*Config){
		"file max differs from 10 MiB":      func(cfg *Config) { cfg.FileUploadMaxBytes-- },
		"multipart max differs from 11 MiB": func(cfg *Config) { cfg.FileUploadMultipartMaxBytes-- },
		"31 byte HMAC secret":               func(cfg *Config) { cfg.FileUploadIPHashSecret = strings.Repeat("x", 31) },
		"example HMAC secret": func(cfg *Config) {
			cfg.FileUploadIPHashSecret = "replace-with-a-strong-random-upload-ip-hmac-secret"
		},
		"development example HMAC secret": func(cfg *Config) {
			cfg.FileUploadIPHashSecret = "replace-with-a-local-random-upload-ip-hmac-secret"
		},
		"zero anonymous rate":    func(cfg *Config) { cfg.FileUploadAnonPresignPerHour = 0 },
		"zero anonymous files":   func(cfg *Config) { cfg.FileUploadAnonActiveFiles = 0 },
		"zero anonymous bytes":   func(cfg *Config) { cfg.FileUploadAnonActiveBytes = 0 },
		"zero merchant bytes":    func(cfg *Config) { cfg.FileUploadMerchantQuotaBytes = 0 },
		"zero global bytes":      func(cfg *Config) { cfg.FileUploadGlobalQuotaBytes = 0 },
		"zero cleanup interval":  func(cfg *Config) { cfg.FileUploadCleanupInterval = 0 },
		"zero cleanup batch":     func(cfg *Config) { cfg.FileUploadCleanupBatchSize = 0 },
		"zero cleanup claim TTL": func(cfg *Config) { cfg.FileUploadCleanupClaimTTL = 0 },
		"zero cleanup grace":     func(cfg *Config) { cfg.FileUploadCleanupGrace = 0 },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := securityTestConfig(t)
			cfg.AppEnv = "production"
			cfg.uploadGovernanceEnvExplicit = true
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected invalid upload governance to be rejected")
			}
		})
	}
}

func TestProductionRejectsInvalidUploadGovernanceParsing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "nonnumeric", key: "FILE_UPLOAD_ANON_ACTIVE_FILES", value: "five"},
		{name: "negative", key: "FILE_UPLOAD_GLOBAL_QUOTA_MB", value: "-1"},
		{name: "overflow", key: "FILE_UPLOAD_MERCHANT_QUOTA_MB", value: "9223372036854775807"},
		{name: "invalid CIDR", key: "TRUSTED_PROXY_CIDRS", value: "not-a-cidr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setUploadGovernanceEnv(t)
			t.Setenv("APP_ENV", "development")
			t.Setenv(tc.key, tc.value)
			cfg := LoadConfig()
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("invalid %s error = %v", tc.key, err)
			}
		})
	}

	t.Run("non-production defaults", func(t *testing.T) {
		for key := range uploadGovernanceEnv {
			t.Setenv(key, "test-cleanup-placeholder")
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
		}
		t.Setenv("FILE_UPLOAD_IP_HASH_SECRET", "development-upload-ip-hmac-secret-32-bytes")
		cfg := LoadConfig()
		if cfg.FileUploadMaxBytes != 10*1024*1024 || cfg.FileUploadMultipartMaxBytes != 11*1024*1024 {
			t.Fatalf("upload defaults = %d/%d", cfg.FileUploadMaxBytes, cfg.FileUploadMultipartMaxBytes)
		}
		if cfg.FileUploadAnonPresignPerHour != 20 || cfg.FileUploadAnonActiveFiles != 5 ||
			cfg.FileUploadAnonActiveBytes != 50*1024*1024 ||
			cfg.FileUploadMerchantQuotaBytes != 2*1024*1024*1024 ||
			cfg.FileUploadGlobalQuotaBytes != 20*1024*1024*1024 ||
			cfg.FileUploadCleanupInterval != 5*time.Minute || cfg.FileUploadCleanupBatchSize != 50 ||
			cfg.FileUploadCleanupClaimTTL != 10*time.Minute || cfg.FileUploadCleanupGrace != 30*time.Minute ||
			len(cfg.TrustedProxyCIDRs) != 0 {
			t.Fatalf("unexpected upload governance defaults: %+v", cfg)
		}
	})

	t.Run("none is explicit", func(t *testing.T) {
		setUploadGovernanceEnv(t)
		t.Setenv("APP_ENV", "development")
		cfg := LoadConfig()
		if !cfg.uploadGovernanceEnvExplicit || len(cfg.TrustedProxyCIDRs) != 0 {
			t.Fatalf("TRUSTED_PROXY_CIDRS=none was not explicit: %+v", cfg.TrustedProxyCIDRs)
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("validate explicit no-proxy configuration: %v", err)
		}
	})
}

func TestTrustedProxyConfigurationRejectsSpoofedForwardedFor(t *testing.T) {
	cfg := securityTestConfig(t)
	cfg.TrustedProxyCIDRs = nil
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv.Router.GET("/test-client-ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	req := httptest.NewRequest(http.MethodGet, "/test-client-ip", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	out := httptest.NewRecorder()
	srv.Router.ServeHTTP(out, req)
	if out.Body.String() != "192.0.2.10" {
		t.Fatalf("client IP = %q, want direct peer", out.Body.String())
	}
}

func TestTrustedProxyConfigurationRejectsInvalidCIDR(t *testing.T) {
	cfg := securityTestConfig(t)
	cfg.TrustedProxyCIDRs = []string{"not-a-cidr"}
	if _, err := NewServer(cfg); err == nil || !strings.Contains(err.Error(), "trusted prox") {
		t.Fatalf("invalid trusted proxy error = %v", err)
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
	cfg.uploadGovernanceEnvExplicit = true
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
	cfg.uploadGovernanceEnvExplicit = true
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
