package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	mysqlcfg "github.com/go-sql-driver/mysql"
)

const (
	defaultDBDSN            = "shm:Shm@123456@tcp(127.0.0.1:3306)/second_hand_market?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
	defaultDBPassword       = "Shm@123456"
	defaultJWTAccessSecret  = "dev-access-secret"
	defaultJWTRefreshSecret = "dev-refresh-secret"
)

var knownUnsafeProductionSecrets = map[string]bool{
	defaultJWTAccessSecret:                        true,
	defaultJWTRefreshSecret:                       true,
	"replace-access-secret":                       true,
	"replace-refresh-secret":                      true,
	"replace-with-a-strong-access-secret":         true,
	"replace-with-a-strong-refresh-secret":        true,
	"replace-with-a-strong-random-access-secret":  true,
	"replace-with-a-strong-random-refresh-secret": true,
}

type Config struct {
	AppEnv                     string
	Addr                       string
	DBDriver                   string
	DBDSN                      string
	JWTAccessSecret            string
	JWTRefreshSecret           string
	AccessTTL                  time.Duration
	RefreshTTL                 time.Duration
	AutoMigrate                bool
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
}

func LoadConfig() Config {
	cfg := Config{
		AppEnv:                     getEnv("APP_ENV", "development"),
		Addr:                       getEnv("ADDR", ":8080"),
		DBDriver:                   getEnv("DB_DRIVER", "mysql"),
		DBDSN:                      getEnv("DB_DSN", defaultDBDSN),
		JWTAccessSecret:            getEnv("JWT_ACCESS_SECRET", defaultJWTAccessSecret),
		JWTRefreshSecret:           getEnv("JWT_REFRESH_SECRET", defaultJWTRefreshSecret),
		AccessTTL:                  2 * time.Hour,
		RefreshTTL:                 7 * 24 * time.Hour,
		AutoMigrate:                getEnvBool("AUTO_MIGRATE", true),
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
		BuyerWechatCode2SessionURL: getEnv("BUYER_WECHAT_CODE2SESSION_URL", "https://api.weixin.qq.com/sns/jscode2session"),
		BuyerWechatHTTPTimeout:     5 * time.Second,
		BuyerDouyinLoginMode:       getEnv("BUYER_DOUYIN_LOGIN_MODE", "mock"),
		BuyerDouyinAppID:           getEnv("BUYER_DOUYIN_APP_ID", ""),
		BuyerDouyinAppSecret:       getEnv("BUYER_DOUYIN_APP_SECRET", ""),
		BuyerDouyinCode2SessionURL: getEnv("BUYER_DOUYIN_CODE2SESSION_URL", "https://developer.toutiao.com/api/apps/v2/jscode2session"),
		BuyerDouyinHTTPTimeout:     5 * time.Second,
	}
	if v := os.Getenv("ACCESS_TTL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.AccessTTL = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("REFRESH_TTL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.RefreshTTL = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("BUYER_WECHAT_HTTP_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.BuyerWechatHTTPTimeout = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("BUYER_DOUYIN_HTTP_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.BuyerDouyinHTTPTimeout = time.Duration(n) * time.Second
		}
	}
	return cfg
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(c.AppEnv), "production")
}

func (c Config) Validate() error {
	if !c.IsProduction() {
		return nil
	}
	if isKnownUnsafeProductionSecret(c.JWTAccessSecret) {
		return fmt.Errorf("production JWT access secret must be explicitly configured")
	}
	if isKnownUnsafeProductionSecret(c.JWTRefreshSecret) {
		return fmt.Errorf("production JWT refresh secret must be explicitly configured")
	}
	if strings.EqualFold(strings.TrimSpace(c.DBDriver), "mysql") {
		dsn := strings.TrimSpace(c.DBDSN)
		if dsn == "" || dsn == defaultDBDSN {
			return fmt.Errorf("production database DSN must be explicitly configured")
		}
		parsed, err := mysqlcfg.ParseDSN(dsn)
		if err != nil {
			return fmt.Errorf("production database DSN is invalid")
		}
		if parsed.Passwd == "" || parsed.Passwd == defaultDBPassword || parsed.Passwd == "replace-with-a-strong-db-password" {
			return fmt.Errorf("production database password must not use an empty, default, or example value")
		}
	}
	return nil
}

func isKnownUnsafeProductionSecret(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || knownUnsafeProductionSecrets[value]
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getEnvBool(k string, d bool) bool {
	if v := os.Getenv(k); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
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
