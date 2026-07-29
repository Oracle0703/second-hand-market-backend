package app

import (
	"bufio"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/common"
)

const (
	testProductionAccessSecret  = "G7mQ2vX9pL4sN8cR1tY6kD3fH5jW0zB7aE9uI2oP"
	testProductionRefreshSecret = "R8cV1nM6qT3xK9dF2hJ7sW4pY0bL5gZ8uA1eN6iC"
)

func safeProductionRuntimeConfig() Config {
	return Config{
		AppEnv:                     appEnvProduction,
		DBTarget:                   "local",
		DBDriver:                   "guardrail-probe",
		DBDSN:                      "guardrail-probe-dsn",
		JWTAccessSecret:            testProductionAccessSecret,
		JWTRefreshSecret:           testProductionRefreshSecret,
		BuyerWechatLoginMode:       buyerLoginModeReal,
		BuyerWechatAppID:           "wx-test-app-id",
		BuyerWechatAppSecret:       "wx-test-app-secret",
		BuyerWechatCode2SessionURL: wechatCode2SessionURL,
		BuyerWechatHTTPTimeout:     5 * time.Second,
		BuyerDouyinLoginMode:       buyerLoginModeDisabled,
		BuyerDouyinCode2SessionURL: douyinCode2SessionURL,
		BuyerDouyinHTTPTimeout:     5 * time.Second,
		FileStorageProvider:        "local",
		FileUploadMaxBytes:         40 * 1024 * 1024,
		ImageCompressTargetBytes:   20 * 1024 * 1024,
		ImageProcessorDriver:       "passthrough",
		AccessTTL:                  2 * time.Hour,
		RefreshTTL:                 7 * 24 * time.Hour,
	}
}

func assertRuntimeConfigError(t *testing.T, cfg Config, field string) {
	t.Helper()
	err := cfg.ValidateRuntime()
	if err == nil {
		t.Fatalf("expected %s to be rejected", field)
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("error %q does not identify %s", err, field)
	}
}

func TestRuntimeGuardrails(t *testing.T) {
	t.Run("production_accepts_real_plus_disabled", func(t *testing.T) {
		if err := safeProductionRuntimeConfig().ValidateRuntime(); err != nil {
			t.Fatalf("safe production configuration was rejected: %v", err)
		}
	})

	t.Run("production_accepts_disabled_plus_real", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatLoginMode = buyerLoginModeDisabled
		cfg.BuyerWechatAppID = ""
		cfg.BuyerWechatAppSecret = ""
		cfg.BuyerDouyinLoginMode = buyerLoginModeReal
		cfg.BuyerDouyinAppID = "tt-test-app-id"
		cfg.BuyerDouyinAppSecret = "tt-test-app-secret"
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("safe production configuration was rejected: %v", err)
		}
	})

	t.Run("production_accepts_all_disabled", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatLoginMode = buyerLoginModeDisabled
		cfg.BuyerWechatAppID = ""
		cfg.BuyerWechatAppSecret = ""
		cfg.BuyerWechatHTTPTimeout = 0
		cfg.BuyerDouyinHTTPTimeout = 0
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("fail-closed production configuration was rejected: %v", err)
		}
	})

	t.Run("rejects_one_byte_access_secret", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.JWTAccessSecret = "a"
		assertRuntimeConfigError(t, cfg, "JWT_ACCESS_SECRET")
	})

	t.Run("rejects_31_byte_refresh_secret", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.JWTRefreshSecret = strings.Repeat("r", 31)
		assertRuntimeConfigError(t, cfg, "JWT_REFRESH_SECRET")
	})

	t.Run("rejects_known_long_placeholder", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.JWTAccessSecret = "replace-with-a-strong-random-access-secret"
		assertRuntimeConfigError(t, cfg, "JWT_ACCESS_SECRET")
	})

	t.Run("rejects_equal_jwt_secrets", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.JWTRefreshSecret = cfg.JWTAccessSecret
		assertRuntimeConfigError(t, cfg, "JWT_ACCESS_SECRET")
	})

	t.Run("rejects_secret_whitespace", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.JWTAccessSecret = " " + cfg.JWTAccessSecret
		assertRuntimeConfigError(t, cfg, "JWT_ACCESS_SECRET")
	})

	t.Run("rejects_repeated_secret", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.JWTAccessSecret = strings.Repeat("a", minProductionJWTSecretBytes)
		assertRuntimeConfigError(t, cfg, "JWT_ACCESS_SECRET")
	})

	t.Run("rejects_low_diversity_secret", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.JWTAccessSecret = strings.Repeat("password", 4)
		assertRuntimeConfigError(t, cfg, "JWT_ACCESS_SECRET")
	})

	t.Run("rejects_repeating_pattern_secret", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.JWTAccessSecret = strings.Repeat("1234567890abcdef", 2)
		assertRuntimeConfigError(t, cfg, "JWT_ACCESS_SECRET")
	})

	t.Run("rejects_wechat_mock", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatLoginMode = buyerLoginModeMock
		assertRuntimeConfigError(t, cfg, "BUYER_WECHAT_LOGIN_MODE")
	})

	t.Run("rejects_douyin_mock", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerDouyinLoginMode = buyerLoginModeMock
		assertRuntimeConfigError(t, cfg, "BUYER_DOUYIN_LOGIN_MODE")
	})

	t.Run("rejects_empty_mode", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatLoginMode = ""
		assertRuntimeConfigError(t, cfg, "BUYER_WECHAT_LOGIN_MODE")
	})

	t.Run("rejects_unknown_mode", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatLoginMode = "automatic"
		assertRuntimeConfigError(t, cfg, "BUYER_WECHAT_LOGIN_MODE")
	})

	t.Run("rejects_real_provider_without_credentials", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatAppSecret = ""
		assertRuntimeConfigError(t, cfg, "BUYER_WECHAT_APP_SECRET")
	})

	t.Run("rejects_real_provider_without_positive_timeout", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatHTTPTimeout = 0
		assertRuntimeConfigError(t, cfg, "BUYER_WECHAT_HTTP_TIMEOUT_SECONDS")
	})

	t.Run("rejects_real_provider_excessive_timeout", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatHTTPTimeout = maxProviderHTTPTimeout + time.Second
		assertRuntimeConfigError(t, cfg, "BUYER_WECHAT_HTTP_TIMEOUT_SECONDS")
	})

	t.Run("accepts_real_provider_timeout_boundary", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatHTTPTimeout = maxProviderHTTPTimeout
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("maximum provider timeout was rejected: %v", err)
		}
	})

	t.Run("rejects_nonofficial_production_endpoint", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.BuyerWechatCode2SessionURL = "https://example.invalid/sns/jscode2session"
		assertRuntimeConfigError(t, cfg, "BUYER_WECHAT_CODE2SESSION_URL")
	})

	t.Run("rejects_empty_app_env", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.AppEnv = ""
		assertRuntimeConfigError(t, cfg, "APP_ENV")
	})

	t.Run("rejects_unknown_app_env", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.AppEnv = "prodution"
		assertRuntimeConfigError(t, cfg, "APP_ENV")
	})

	t.Run("development_allows_mock", func(t *testing.T) {
		cfg := Config{
			AppEnv:               appEnvDevelopment,
			JWTAccessSecret:      "dev-access-secret",
			JWTRefreshSecret:     "dev-refresh-secret",
			AccessTTL:            2 * time.Hour,
			RefreshTTL:           7 * 24 * time.Hour,
			BuyerWechatLoginMode: buyerLoginModeMock,
			BuyerDouyinLoginMode: buyerLoginModeMock,
		}
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("development mock configuration was rejected: %v", err)
		}
	})

	t.Run("test_allows_mock", func(t *testing.T) {
		cfg := Config{
			AppEnv:               appEnvTest,
			JWTAccessSecret:      "test-access",
			JWTRefreshSecret:     "test-refresh",
			AccessTTL:            2 * time.Hour,
			RefreshTTL:           7 * 24 * time.Hour,
			BuyerWechatLoginMode: buyerLoginModeMock,
			BuyerDouyinLoginMode: buyerLoginModeMock,
		}
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("test mock configuration was rejected: %v", err)
		}
	})
}

func TestNewServerValidatesRuntimeBeforeDatabase(t *testing.T) {
	t.Run("unsafe_configuration_stops_before_database", func(t *testing.T) {
		cfg := safeProductionRuntimeConfig()
		cfg.JWTAccessSecret = "a"
		_, err := NewServer(cfg)
		if err == nil || !strings.Contains(err.Error(), "JWT_ACCESS_SECRET") {
			t.Fatalf("expected access-secret error, got %v", err)
		}
		if strings.Contains(err.Error(), "unsupported db driver") {
			t.Fatalf("database was reached before runtime validation: %v", err)
		}
	})

	t.Run("safe_configuration_reaches_database_probe", func(t *testing.T) {
		_, err := NewServer(safeProductionRuntimeConfig())
		if err == nil || !strings.Contains(err.Error(), "unsupported db driver: guardrail-probe") {
			t.Fatalf("safe configuration did not reach the database probe: %v", err)
		}
	})
}

func TestLoadConfigProductionDefaultsFailClosed(t *testing.T) {
	t.Setenv("APP_ENV", appEnvProduction)
	t.Setenv("DB_TARGET", dbTargetLocal)
	t.Setenv("DB_DSN", "production-test-dsn")
	t.Setenv("JWT_ACCESS_SECRET", "dev-access-secret")
	t.Setenv("JWT_REFRESH_SECRET", "dev-refresh-secret")
	t.Setenv("BUYER_WECHAT_LOGIN_MODE", buyerLoginModeMock)
	t.Setenv("BUYER_DOUYIN_LOGIN_MODE", buyerLoginModeMock)

	cfg := LoadConfig()
	assertRuntimeConfigError(t, cfg, "BUYER_WECHAT_LOGIN_MODE")
}

func TestLoadConfigRejectsInvalidRealProviderTimeout(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "rejects_zero_wechat_timeout", key: "BUYER_WECHAT_HTTP_TIMEOUT_SECONDS", value: "0"},
		{name: "rejects_douyin_timeout_above_limit", key: "BUYER_DOUYIN_HTTP_TIMEOUT_SECONDS", value: "61"},
		{name: "rejects_nonnumeric_wechat_timeout", key: "BUYER_WECHAT_HTTP_TIMEOUT_SECONDS", value: "forever"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_ENV", appEnvTest)
			t.Setenv("BUYER_WECHAT_LOGIN_MODE", buyerLoginModeReal)
			t.Setenv("BUYER_WECHAT_APP_ID", "wx-test-app-id")
			t.Setenv("BUYER_WECHAT_APP_SECRET", "wx-test-app-secret")
			t.Setenv("BUYER_DOUYIN_LOGIN_MODE", buyerLoginModeReal)
			t.Setenv("BUYER_DOUYIN_APP_ID", "tt-test-app-id")
			t.Setenv("BUYER_DOUYIN_APP_SECRET", "tt-test-app-secret")
			t.Setenv("BUYER_WECHAT_HTTP_TIMEOUT_SECONDS", "5")
			t.Setenv("BUYER_DOUYIN_HTTP_TIMEOUT_SECONDS", "5")
			t.Setenv(tc.key, tc.value)

			cfg := LoadConfig()
			assertRuntimeConfigError(t, cfg, tc.key)
		})
	}
}

func TestLoadConfigIgnoresUnusedProviderTimeout(t *testing.T) {
	t.Setenv("APP_ENV", appEnvTest)
	t.Setenv("BUYER_WECHAT_LOGIN_MODE", buyerLoginModeMock)
	t.Setenv("BUYER_DOUYIN_LOGIN_MODE", buyerLoginModeDisabled)
	t.Setenv("BUYER_WECHAT_HTTP_TIMEOUT_SECONDS", "invalid-but-unused")
	t.Setenv("BUYER_DOUYIN_HTTP_TIMEOUT_SECONDS", "0")

	cfg := LoadConfig()
	if err := cfg.ValidateRuntime(); err != nil {
		t.Fatalf("unused provider timeout was validated: %v", err)
	}
}

func TestLoadConfigAutoMigrateIsStrictAndDefaultsOff(t *testing.T) {
	t.Run("defaults_off", func(t *testing.T) {
		unsetEnvForTest(t, "AUTO_MIGRATE")
		t.Setenv("APP_ENV", appEnvTest)

		cfg := LoadConfig()
		if cfg.AutoMigrate {
			t.Fatal("AUTO_MIGRATE defaulted to true")
		}
		if err := cfg.ValidateRuntime(); err != nil {
			t.Fatalf("default AUTO_MIGRATE was rejected: %v", err)
		}
	})

	t.Run("rejects_invalid_value_without_leaking_it", func(t *testing.T) {
		const sentinel = "auto-migrate-invalid-sentinel"
		t.Setenv("APP_ENV", appEnvTest)
		t.Setenv("AUTO_MIGRATE", sentinel)

		err := LoadConfig().ValidateRuntime()
		if err == nil || !strings.Contains(err.Error(), "AUTO_MIGRATE") {
			t.Fatalf("expected AUTO_MIGRATE error, got %v", err)
		}
		if strings.Contains(err.Error(), sentinel) {
			t.Fatalf("AUTO_MIGRATE error leaked its value: %q", err)
		}
	})
}

func unsetEnvForTest(t *testing.T, name string) {
	t.Helper()
	original, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, original)
			return
		}
		_ = os.Unsetenv(name)
	})
}

func TestDisabledProviderReturnsForbidden(t *testing.T) {
	t.Run("wechat", func(t *testing.T) {
		srv := &Server{cfg: Config{BuyerWechatLoginMode: buyerLoginModeDisabled}}
		_, _, err := srv.resolveWechatIdentity("code")
		assertDisabledProviderError(t, err)
	})
	t.Run("douyin", func(t *testing.T) {
		srv := &Server{cfg: Config{BuyerDouyinLoginMode: buyerLoginModeDisabled}}
		_, _, err := srv.resolveDouyinIdentity("code")
		assertDisabledProviderError(t, err)
	})
}

func assertDisabledProviderError(t *testing.T, err error) {
	t.Helper()
	var bizErr *common.BizError
	if !errors.As(err, &bizErr) {
		t.Fatalf("disabled provider error = %v", err)
	}
	if bizErr.Code != common.CodeForbidden || bizErr.HTTPStatus != http.StatusForbidden {
		t.Fatalf("disabled provider error = %+v", bizErr)
	}
}

func TestWechatTransportErrorDoesNotExposeAppSecret(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpointURL := endpoint.URL
	endpoint.Close()

	const sentinel = "wechat-transport-secret-must-not-leak"
	srv := &Server{cfg: Config{
		BuyerWechatLoginMode:       buyerLoginModeReal,
		BuyerWechatAppID:           "wx-test-app-id",
		BuyerWechatAppSecret:       sentinel,
		BuyerWechatCode2SessionURL: endpointURL,
		BuyerWechatHTTPTimeout:     time.Second,
	}}
	_, _, err := srv.resolveWechatIdentity("one-time-code")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "one-time-code") {
		t.Fatalf("transport error leaked a credential: %q", err)
	}
}

func TestDouyinTransportErrorDoesNotExposeAppSecret(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpointURL := endpoint.URL
	endpoint.Close()

	const sentinel = "douyin-transport-secret-must-not-leak"
	srv := &Server{cfg: Config{
		BuyerDouyinLoginMode:       buyerLoginModeReal,
		BuyerDouyinAppID:           "tt-test-app-id",
		BuyerDouyinAppSecret:       sentinel,
		BuyerDouyinCode2SessionURL: endpointURL,
		BuyerDouyinHTTPTimeout:     time.Second,
	}}
	_, _, err := srv.resolveDouyinIdentity("one-time-code")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "one-time-code") {
		t.Fatalf("transport error leaked a credential: %q", err)
	}
}

func TestLoadConfigRequiresExplicitAppEnv(t *testing.T) {
	original, existed := os.LookupEnv("APP_ENV")
	if err := os.Unsetenv("APP_ENV"); err != nil {
		t.Fatalf("unset APP_ENV: %v", err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("APP_ENV", original)
			return
		}
		_ = os.Unsetenv("APP_ENV")
	})

	cfg := LoadConfig()
	assertRuntimeConfigError(t, cfg, "APP_ENV")
}

func TestProductionEnvExamplesEnableRuntimeGuardrails(t *testing.T) {
	for _, name := range []string{".env.production.mysql.example", ".env.production.sqlite.example"} {
		t.Run(name, func(t *testing.T) {
			values := readEnvExample(t, filepath.Join("..", "..", "configs", name))
			if values["APP_ENV"] != appEnvProduction {
				t.Fatalf("APP_ENV = %q", values["APP_ENV"])
			}
			if values["BUYER_WECHAT_LOGIN_MODE"] != buyerLoginModeDisabled {
				t.Fatalf("BUYER_WECHAT_LOGIN_MODE = %q", values["BUYER_WECHAT_LOGIN_MODE"])
			}
			if values["BUYER_DOUYIN_LOGIN_MODE"] != buyerLoginModeDisabled {
				t.Fatalf("BUYER_DOUYIN_LOGIN_MODE = %q", values["BUYER_DOUYIN_LOGIN_MODE"])
			}
			if values["BUYER_WECHAT_CODE2SESSION_URL"] != wechatCode2SessionURL {
				t.Fatalf("BUYER_WECHAT_CODE2SESSION_URL = %q", values["BUYER_WECHAT_CODE2SESSION_URL"])
			}
			if values["BUYER_DOUYIN_CODE2SESSION_URL"] != douyinCode2SessionURL {
				t.Fatalf("BUYER_DOUYIN_CODE2SESSION_URL = %q", values["BUYER_DOUYIN_CODE2SESSION_URL"])
			}
			if err := validateProductionJWTSecret("JWT_ACCESS_SECRET", values["JWT_ACCESS_SECRET"]); err == nil {
				t.Fatal("the access-secret placeholder must remain fail-closed")
			}
			if err := validateProductionJWTSecret("JWT_REFRESH_SECRET", values["JWT_REFRESH_SECRET"]); err == nil {
				t.Fatal("the refresh-secret placeholder must remain fail-closed")
			}
		})
	}
}

func readEnvExample(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Fatalf("close %s: %v", path, err)
		}
	}()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("invalid env line in %s: %q", path, line)
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return values
}
