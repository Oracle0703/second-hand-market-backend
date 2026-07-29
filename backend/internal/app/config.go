package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

const (
	appEnvDevelopment = "development"
	appEnvTest        = "test"
	appEnvProduction  = "production"

	dbTargetLocal             = "local"
	dbTargetRemoteDevelopment = "remote-development"

	remoteDevelopmentDBAddr = "127.0.0.1:13307"
	remoteDevelopmentDBName = "second_hand_market_dev"
	remoteDevelopmentDBUser = "shm_dev_app"

	buyerLoginModeMock     = "mock"
	buyerLoginModeReal     = "real"
	buyerLoginModeDisabled = "disabled"

	wechatCode2SessionURL = "https://api.weixin.qq.com/sns/jscode2session"
	douyinCode2SessionURL = "https://developer.toutiao.com/api/apps/v2/jscode2session"

	minProductionJWTSecretBytes         = 32
	minProductionJWTSecretDistinctBytes = 12
	maxProviderHTTPTimeout              = 60 * time.Second
)

var knownUnsafeProductionJWTSecrets = map[string]struct{}{
	"dev-access-secret":                           {},
	"dev-refresh-secret":                          {},
	"replace-access-secret":                       {},
	"replace-refresh-secret":                      {},
	"replace-with-a-strong-access-secret":         {},
	"replace-with-a-strong-refresh-secret":        {},
	"replace-with-a-strong-random-access-secret":  {},
	"replace-with-a-strong-random-refresh-secret": {},
}

type Config struct {
	AppEnv                     string
	Addr                       string
	DBTarget                   string
	DBDriver                   string
	DBDSN                      string
	DBExpectedDatabase         string
	DBExpectedServerUUID       string
	DBExpectedUser             string
	JWTAccessSecret            string
	JWTRefreshSecret           string
	AccessTTL                  time.Duration
	RefreshTTL                 time.Duration
	AutoMigrate                bool
	SeedDefaults               bool
	FileStorageProvider        string
	FileUploadLocalDir         string
	FilePublicBaseURL          string
	FileUploadMaxBytes         int64
	ImageCompressTargetBytes   int64
	ImageProcessorDriver       string
	ImageProcessorBin          string
	BuyerWechatLoginMode       string
	BuyerWechatAppID           string
	BuyerWechatAppSecret       string
	BuyerWechatCode2SessionURL string
	BuyerWechatHTTPTimeout     time.Duration
	BuyerDouyinLoginMode       string
	BuyerDouyinAppID           string
	BuyerDouyinAppSecret       string
	BuyerDouyinCode2SessionURL string
	BuyerDouyinHTTPTimeout     time.Duration

	runtimeLoadErr error
}

func LoadConfig() Config {
	cfg := Config{
		AppEnv:                     strings.TrimSpace(os.Getenv("APP_ENV")),
		Addr:                       getEnv("ADDR", ":8080"),
		DBTarget:                   normalizeDBTarget(getEnv("DB_TARGET", dbTargetLocal)),
		DBDriver:                   getEnv("DB_DRIVER", "mysql"),
		DBDSN:                      getEnv("DB_DSN", ""),
		DBExpectedDatabase:         getEnv("DB_EXPECTED_DATABASE", ""),
		DBExpectedServerUUID:       getEnv("DB_EXPECTED_SERVER_UUID", ""),
		DBExpectedUser:             getEnv("DB_EXPECTED_USER", ""),
		JWTAccessSecret:            getEnv("JWT_ACCESS_SECRET", "dev-access-secret"),
		JWTRefreshSecret:           getEnv("JWT_REFRESH_SECRET", "dev-refresh-secret"),
		AccessTTL:                  2 * time.Hour,
		RefreshTTL:                 7 * 24 * time.Hour,
		AutoMigrate:                false,
		SeedDefaults:               false,
		FileStorageProvider:        getEnv("FILE_STORAGE_PROVIDER", "local"),
		FileUploadLocalDir:         getEnv("FILE_UPLOAD_LOCAL_DIR", "uploads"),
		FilePublicBaseURL:          getEnv("FILE_PUBLIC_BASE_URL", ""),
		FileUploadMaxBytes:         int64(getEnvInt("FILE_UPLOAD_MAX_MB", 40)) * 1024 * 1024,
		ImageCompressTargetBytes:   int64(getEnvInt("IMAGE_COMPRESS_TARGET_MB", 20)) * 1024 * 1024,
		ImageProcessorDriver:       getEnv("IMAGE_PROCESSOR_DRIVER", "vips"),
		ImageProcessorBin:          getEnv("IMAGE_PROCESSOR_BIN", "vips"),
		BuyerWechatLoginMode:       getEnv("BUYER_WECHAT_LOGIN_MODE", "mock"),
		BuyerWechatAppID:           getEnv("BUYER_WECHAT_APP_ID", ""),
		BuyerWechatAppSecret:       getEnv("BUYER_WECHAT_APP_SECRET", ""),
		BuyerWechatCode2SessionURL: getEnv("BUYER_WECHAT_CODE2SESSION_URL", wechatCode2SessionURL),
		BuyerWechatHTTPTimeout:     5 * time.Second,
		BuyerDouyinLoginMode:       getEnv("BUYER_DOUYIN_LOGIN_MODE", "mock"),
		BuyerDouyinAppID:           getEnv("BUYER_DOUYIN_APP_ID", ""),
		BuyerDouyinAppSecret:       getEnv("BUYER_DOUYIN_APP_SECRET", ""),
		BuyerDouyinCode2SessionURL: getEnv("BUYER_DOUYIN_CODE2SESSION_URL", douyinCode2SessionURL),
		BuyerDouyinHTTPTimeout:     5 * time.Second,
	}
	cfg.loadRuntimeBool("AUTO_MIGRATE", &cfg.AutoMigrate)
	cfg.loadRuntimeBool("SEED_DEFAULTS", &cfg.SeedDefaults)
	if value := os.Getenv("ACCESS_TTL_SECONDS"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			cfg.AccessTTL = time.Duration(seconds) * time.Second
		}
	}
	if value := os.Getenv("REFRESH_TTL_SECONDS"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			cfg.RefreshTTL = time.Duration(seconds) * time.Second
		}
	}
	if normalizeBuyerLoginMode(cfg.BuyerWechatLoginMode) == buyerLoginModeReal {
		cfg.loadProviderTimeout("BUYER_WECHAT_HTTP_TIMEOUT_SECONDS", &cfg.BuyerWechatHTTPTimeout)
	}
	if normalizeBuyerLoginMode(cfg.BuyerDouyinLoginMode) == buyerLoginModeReal {
		cfg.loadProviderTimeout("BUYER_DOUYIN_HTTP_TIMEOUT_SECONDS", &cfg.BuyerDouyinHTTPTimeout)
	}
	return cfg
}

func (c *Config) loadRuntimeBool(name string, target *bool) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		c.runtimeLoadErr = errors.Join(c.runtimeLoadErr, fmt.Errorf("%s must be a valid boolean", name))
		return
	}
	*target = value
}

func (c *Config) loadProviderTimeout(name string, target *time.Duration) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	maximumSeconds := int64(maxProviderHTTPTimeout / time.Second)
	if err != nil || value <= 0 || value > maximumSeconds {
		if c.runtimeLoadErr == nil {
			c.runtimeLoadErr = fmt.Errorf(
				"%s must be an integer between 1 and %d",
				name,
				maximumSeconds,
			)
		}
		return
	}
	*target = time.Duration(value) * time.Second
}

func (c Config) IsProduction() bool {
	return normalizeAppEnv(c.AppEnv) == appEnvProduction
}

func (c Config) ValidateRuntime() error {
	if c.runtimeLoadErr != nil {
		return c.runtimeLoadErr
	}

	env := normalizeAppEnv(c.AppEnv)
	switch env {
	case appEnvDevelopment, appEnvTest, appEnvProduction:
	default:
		return fmt.Errorf("APP_ENV must be one of development, test, or production")
	}

	target := normalizeDBTarget(c.DBTarget)
	switch target {
	case dbTargetLocal, dbTargetRemoteDevelopment:
	default:
		return fmt.Errorf("DB_TARGET must be one of local or remote-development")
	}

	if (env == appEnvProduction || target == dbTargetRemoteDevelopment) && strings.TrimSpace(c.DBDSN) == "" {
		return fmt.Errorf("DB_DSN is required when APP_ENV is production or DB_TARGET is remote-development")
	}
	if env == appEnvProduction || target == dbTargetRemoteDevelopment {
		if c.AutoMigrate {
			return fmt.Errorf("AUTO_MIGRATE must be false when APP_ENV is production or DB_TARGET is remote-development")
		}
		if c.SeedDefaults {
			return fmt.Errorf("SEED_DEFAULTS must be false when APP_ENV is production or DB_TARGET is remote-development")
		}
	}
	if target == dbTargetRemoteDevelopment {
		if err := validateRemoteDevelopmentDatabase(c); err != nil {
			return err
		}
	}

	if err := validateBuyerLoginConfig(
		"BUYER_WECHAT",
		c.BuyerWechatLoginMode,
		c.BuyerWechatAppID,
		c.BuyerWechatAppSecret,
		c.BuyerWechatCode2SessionURL,
		c.BuyerWechatHTTPTimeout,
		wechatCode2SessionURL,
		env == appEnvProduction,
	); err != nil {
		return err
	}
	if err := validateBuyerLoginConfig(
		"BUYER_DOUYIN",
		c.BuyerDouyinLoginMode,
		c.BuyerDouyinAppID,
		c.BuyerDouyinAppSecret,
		c.BuyerDouyinCode2SessionURL,
		c.BuyerDouyinHTTPTimeout,
		douyinCode2SessionURL,
		env == appEnvProduction,
	); err != nil {
		return err
	}

	if env != appEnvProduction {
		return nil
	}

	if err := validateProductionJWTSecret("JWT_ACCESS_SECRET", c.JWTAccessSecret); err != nil {
		return err
	}
	if err := validateProductionJWTSecret("JWT_REFRESH_SECRET", c.JWTRefreshSecret); err != nil {
		return err
	}
	if c.JWTAccessSecret == c.JWTRefreshSecret {
		return fmt.Errorf("JWT_ACCESS_SECRET and JWT_REFRESH_SECRET must be different in production")
	}
	return nil
}

func normalizeAppEnv(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeDBTarget(value string) string {
	target := strings.ToLower(strings.TrimSpace(value))
	if target == "" {
		return dbTargetLocal
	}
	return target
}

func normalizeBuyerLoginMode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validateRemoteDevelopmentDatabase(c Config) error {
	if c.DBDriver != "mysql" {
		return fmt.Errorf("DB_DRIVER must be mysql when DB_TARGET is remote-development")
	}

	dsn, err := mysqldriver.ParseDSN(c.DBDSN)
	if err != nil {
		return fmt.Errorf("DB_DSN must be a valid MySQL DSN for remote-development")
	}
	if dsn.Net != "tcp" || dsn.Addr != remoteDevelopmentDBAddr || dsn.DBName != remoteDevelopmentDBName {
		return fmt.Errorf(
			"DB_DSN must use tcp at %s with database %s for remote-development",
			remoteDevelopmentDBAddr,
			remoteDevelopmentDBName,
		)
	}
	if dsn.MultiStatements ||
		dsn.AllowAllFiles ||
		dsn.AllowCleartextPasswords ||
		dsn.AllowOldPasswords ||
		dsn.AllowFallbackToPlaintext {
		return fmt.Errorf("DB_DSN must not enable unsafe MySQL options for remote-development")
	}

	if c.DBExpectedDatabase != remoteDevelopmentDBName {
		return fmt.Errorf("DB_EXPECTED_DATABASE must be %s for remote-development", remoteDevelopmentDBName)
	}
	if strings.TrimSpace(c.DBExpectedServerUUID) == "" {
		return fmt.Errorf("DB_EXPECTED_SERVER_UUID must be non-empty for remote-development")
	}
	if c.DBExpectedUser != remoteDevelopmentDBUser {
		return fmt.Errorf("DB_EXPECTED_USER must be %s for remote-development", remoteDevelopmentDBUser)
	}
	return nil
}

func validateProductionJWTSecret(name, value string) error {
	trimmed := strings.TrimSpace(value)
	if value != trimmed {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", name)
	}
	if len([]byte(trimmed)) < minProductionJWTSecretBytes {
		return fmt.Errorf("%s must be at least %d bytes in production", name, minProductionJWTSecretBytes)
	}
	if _, unsafe := knownUnsafeProductionJWTSecrets[strings.ToLower(trimmed)]; unsafe {
		return fmt.Errorf("%s must not use a default or example value in production", name)
	}
	secretBytes := []byte(trimmed)
	if distinctByteCount(secretBytes) < minProductionJWTSecretDistinctBytes {
		return fmt.Errorf(
			"%s must contain at least %d distinct bytes in production",
			name,
			minProductionJWTSecretDistinctBytes,
		)
	}
	if isRepeatedBytePattern(secretBytes) {
		return fmt.Errorf("%s must not be a repeated byte pattern in production", name)
	}
	return nil
}

func distinctByteCount(value []byte) int {
	distinct := map[byte]struct{}{}
	for _, current := range value {
		distinct[current] = struct{}{}
	}
	return len(distinct)
}

func isRepeatedBytePattern(value []byte) bool {
	for patternLength := 1; patternLength <= len(value)/2; patternLength++ {
		if len(value)%patternLength != 0 {
			continue
		}
		repeated := true
		for index := patternLength; index < len(value); index++ {
			if value[index] != value[index%patternLength] {
				repeated = false
				break
			}
		}
		if repeated {
			return true
		}
	}
	return false
}

func validateBuyerLoginConfig(
	prefix string,
	modeValue string,
	appID string,
	appSecret string,
	endpoint string,
	timeout time.Duration,
	officialEndpoint string,
	production bool,
) error {
	mode := normalizeBuyerLoginMode(modeValue)
	switch mode {
	case buyerLoginModeMock:
		if production {
			return fmt.Errorf("%s_LOGIN_MODE must not use mock in production", prefix)
		}
		return nil
	case buyerLoginModeDisabled:
		return nil
	case buyerLoginModeReal:
	default:
		return fmt.Errorf("%s_LOGIN_MODE must be one of mock, real, or disabled", prefix)
	}

	if strings.TrimSpace(appID) == "" {
		return fmt.Errorf("%s_APP_ID is required in real mode", prefix)
	}
	if strings.TrimSpace(appSecret) == "" {
		return fmt.Errorf("%s_APP_SECRET is required in real mode", prefix)
	}
	if timeout < time.Second || timeout > maxProviderHTTPTimeout {
		return fmt.Errorf(
			"%s_HTTP_TIMEOUT_SECONDS must be between 1 and %d seconds in real mode",
			prefix,
			int64(maxProviderHTTPTimeout/time.Second),
		)
	}

	trimmedEndpoint := strings.TrimSpace(endpoint)
	parsed, err := url.Parse(trimmedEndpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s_CODE2SESSION_URL must be a valid HTTP(S) URL in real mode", prefix)
	}
	if production && trimmedEndpoint != officialEndpoint {
		return fmt.Errorf("%s_CODE2SESSION_URL must use the official HTTPS endpoint in production", prefix)
	}
	return nil
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getEnvInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return d
}
