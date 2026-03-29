package app

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
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
		Addr:                       getEnv("ADDR", ":8080"),
		DBDriver:                   getEnv("DB_DRIVER", "mysql"),
		DBDSN:                      getEnv("DB_DSN", "shm:Shm@123456@tcp(127.0.0.1:3306)/second_hand_market?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"),
		JWTAccessSecret:            getEnv("JWT_ACCESS_SECRET", "dev-access-secret"),
		JWTRefreshSecret:           getEnv("JWT_REFRESH_SECRET", "dev-refresh-secret"),
		AccessTTL:                  2 * time.Hour,
		RefreshTTL:                 7 * 24 * time.Hour,
		AutoMigrate:                getEnvBool("AUTO_MIGRATE", true),
		FileStorageProvider:        getEnv("FILE_STORAGE_PROVIDER", "local"),
		FileUploadLocalDir:         getEnv("FILE_UPLOAD_LOCAL_DIR", "uploads"),
		FilePublicBaseURL:          getEnv("FILE_PUBLIC_BASE_URL", ""),
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
