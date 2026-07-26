package app

import (
	"fmt"
	"math"
	"net"
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

var knownUnsafeUploadIPHashSecrets = map[string]bool{
	"replace-with-a-strong-random-upload-ip-hmac-secret": true,
	"replace-with-a-local-random-upload-ip-hmac-secret":  true,
}

type Config struct {
	AppEnv                       string
	Addr                         string
	DBDriver                     string
	DBDSN                        string
	JWTAccessSecret              string
	JWTRefreshSecret             string
	AccessTTL                    time.Duration
	RefreshTTL                   time.Duration
	AutoMigrate                  bool
	FileStorageProvider          string
	FileUploadLocalDir           string
	FilePublicBaseURL            string
	FileUploadMaxBytes           int64
	FileUploadMultipartMaxBytes  int64
	FileUploadIPHashSecret       string
	FileUploadAnonPresignPerHour int64
	FileUploadAnonActiveFiles    int64
	FileUploadAnonActiveBytes    int64
	FileUploadMerchantQuotaBytes int64
	FileUploadGlobalQuotaBytes   int64
	FileUploadCleanupInterval    time.Duration
	FileUploadCleanupBatchSize   int
	FileUploadCleanupClaimTTL    time.Duration
	FileUploadCleanupGrace       time.Duration
	TrustedProxyCIDRs            []string
	ImageCompressTargetBytes     int64
	ImageProcessorDriver         string
	ImageProcessorBin            string
	BuyerWechatLoginMode         string
	BuyerWechatAppID             string
	BuyerWechatAppSecret         string
	BuyerWechatCode2SessionURL   string
	BuyerWechatHTTPTimeout       time.Duration
	BuyerDouyinLoginMode         string
	BuyerDouyinAppID             string
	BuyerDouyinAppSecret         string
	BuyerDouyinCode2SessionURL   string
	BuyerDouyinHTTPTimeout       time.Duration

	uploadGovernanceEnvExplicit bool
	loadErr                     error
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
	parser := uploadGovernanceParser{allExplicit: true}
	cfg.FileUploadMaxBytes = parser.positiveMiB("FILE_UPLOAD_MAX_MB", 10)
	cfg.FileUploadMultipartMaxBytes = parser.positiveMiB("FILE_UPLOAD_MULTIPART_MAX_MB", 11)
	cfg.FileUploadIPHashSecret = parser.requiredString("FILE_UPLOAD_IP_HASH_SECRET")
	cfg.FileUploadAnonPresignPerHour = parser.positiveInt64("FILE_UPLOAD_ANON_PRESIGN_PER_HOUR", 20)
	cfg.FileUploadAnonActiveFiles = parser.positiveInt64("FILE_UPLOAD_ANON_ACTIVE_FILES", 5)
	cfg.FileUploadAnonActiveBytes = parser.positiveMiB("FILE_UPLOAD_ANON_ACTIVE_MB", 50)
	cfg.FileUploadMerchantQuotaBytes = parser.positiveMiB("FILE_UPLOAD_MERCHANT_QUOTA_MB", 2048)
	cfg.FileUploadGlobalQuotaBytes = parser.positiveMiB("FILE_UPLOAD_GLOBAL_QUOTA_MB", 20480)
	cfg.FileUploadCleanupInterval = parser.positiveDuration("FILE_UPLOAD_CLEANUP_INTERVAL_SECONDS", 300)
	cfg.FileUploadCleanupBatchSize = parser.positiveInt("FILE_UPLOAD_CLEANUP_BATCH_SIZE", 50)
	cfg.FileUploadCleanupClaimTTL = parser.positiveDuration("FILE_UPLOAD_CLEANUP_CLAIM_TTL_SECONDS", 600)
	cfg.FileUploadCleanupGrace = parser.positiveDuration("FILE_UPLOAD_CLEANUP_GRACE_SECONDS", 1800)
	cfg.TrustedProxyCIDRs = parser.trustedProxyCIDRs("TRUSTED_PROXY_CIDRS")
	cfg.uploadGovernanceEnvExplicit = parser.allExplicit
	cfg.loadErr = parser.err
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
	if c.loadErr != nil {
		return c.loadErr
	}
	if c.IsProduction() && !c.uploadGovernanceEnvExplicit {
		return fmt.Errorf("production upload governance environment must be explicitly configured")
	}
	if err := c.validateUploadGovernance(); err != nil {
		return err
	}
	if !c.IsProduction() {
		return nil
	}
	if c.FileUploadMaxBytes != 10*1024*1024 || c.FileUploadMultipartMaxBytes != 11*1024*1024 {
		return fmt.Errorf("production upload governance requires exact 10 MiB file and 11 MiB multipart limits")
	}
	if knownUnsafeUploadIPHashSecrets[strings.TrimSpace(c.FileUploadIPHashSecret)] {
		return fmt.Errorf("production upload IP hash secret must not use an example value")
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

func (c Config) validateUploadGovernance() error {
	positiveInt64 := []struct {
		name  string
		value int64
	}{
		{name: "FILE_UPLOAD_MAX_MB", value: c.FileUploadMaxBytes},
		{name: "FILE_UPLOAD_MULTIPART_MAX_MB", value: c.FileUploadMultipartMaxBytes},
		{name: "FILE_UPLOAD_ANON_PRESIGN_PER_HOUR", value: c.FileUploadAnonPresignPerHour},
		{name: "FILE_UPLOAD_ANON_ACTIVE_FILES", value: c.FileUploadAnonActiveFiles},
		{name: "FILE_UPLOAD_ANON_ACTIVE_MB", value: c.FileUploadAnonActiveBytes},
		{name: "FILE_UPLOAD_MERCHANT_QUOTA_MB", value: c.FileUploadMerchantQuotaBytes},
		{name: "FILE_UPLOAD_GLOBAL_QUOTA_MB", value: c.FileUploadGlobalQuotaBytes},
	}
	for _, field := range positiveInt64 {
		if field.value <= 0 {
			return fmt.Errorf("%s must be positive", field.name)
		}
	}
	if c.FileUploadCleanupInterval <= 0 {
		return fmt.Errorf("FILE_UPLOAD_CLEANUP_INTERVAL_SECONDS must be positive")
	}
	if c.FileUploadCleanupBatchSize <= 0 {
		return fmt.Errorf("FILE_UPLOAD_CLEANUP_BATCH_SIZE must be positive")
	}
	if c.FileUploadCleanupClaimTTL <= 0 {
		return fmt.Errorf("FILE_UPLOAD_CLEANUP_CLAIM_TTL_SECONDS must be positive")
	}
	if c.FileUploadCleanupGrace <= 0 {
		return fmt.Errorf("FILE_UPLOAD_CLEANUP_GRACE_SECONDS must be positive")
	}
	if c.FileUploadMultipartMaxBytes <= c.FileUploadMaxBytes {
		return fmt.Errorf("FILE_UPLOAD_MULTIPART_MAX_MB must exceed FILE_UPLOAD_MAX_MB")
	}
	secret := strings.TrimSpace(c.FileUploadIPHashSecret)
	if len([]byte(secret)) < 32 {
		return fmt.Errorf("FILE_UPLOAD_IP_HASH_SECRET must be at least 32 bytes")
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("trusted proxy CIDR is invalid")
		}
	}
	return nil
}

type uploadGovernanceParser struct {
	allExplicit bool
	err         error
}

func (p *uploadGovernanceParser) setError(name, message string) {
	if p.err == nil {
		p.err = fmt.Errorf("%s %s", name, message)
	}
}

func (p *uploadGovernanceParser) positiveInt64(name string, defaultValue int64) int64 {
	raw, ok := os.LookupEnv(name)
	if !ok {
		p.allExplicit = false
		return defaultValue
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		p.setError(name, "must be a positive integer")
		return 0
	}
	return value
}

func (p *uploadGovernanceParser) positiveInt(name string, defaultValue int) int {
	value := p.positiveInt64(name, int64(defaultValue))
	if value > int64(^uint(0)>>1) {
		p.setError(name, "is too large")
		return 0
	}
	return int(value)
}

func (p *uploadGovernanceParser) positiveMiB(name string, defaultValue int64) int64 {
	value := p.positiveInt64(name, defaultValue)
	if value == 0 {
		return 0
	}
	bytes, err := mibToBytes(value)
	if err != nil {
		p.setError(name, "must be a positive MiB quantity within range")
		return 0
	}
	return bytes
}

func (p *uploadGovernanceParser) positiveDuration(name string, defaultSeconds int64) time.Duration {
	seconds := p.positiveInt64(name, defaultSeconds)
	if seconds == 0 {
		return 0
	}
	if seconds > math.MaxInt64/int64(time.Second) {
		p.setError(name, "is too large")
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func (p *uploadGovernanceParser) requiredString(name string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		p.allExplicit = false
		return ""
	}
	return strings.TrimSpace(value)
}

func (p *uploadGovernanceParser) trustedProxyCIDRs(name string) []string {
	raw, ok := os.LookupEnv(name)
	if !ok {
		p.allExplicit = false
		return nil
	}
	raw = strings.TrimSpace(raw)
	if strings.EqualFold(raw, "none") {
		return nil
	}
	if raw == "" {
		p.setError(name, "must be none or a comma-separated CIDR list")
		return nil
	}
	parts := strings.Split(raw, ",")
	cidrs := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		_, network, err := net.ParseCIDR(part)
		if err != nil {
			p.setError(name, "contains an invalid CIDR")
			return nil
		}
		cidrs = append(cidrs, network.String())
	}
	return cidrs
}

func mibToBytes(value int64) (int64, error) {
	const mib = int64(1024 * 1024)
	if value <= 0 || value > math.MaxInt64/mib {
		return 0, fmt.Errorf("value must be a positive MiB quantity")
	}
	return value * mib, nil
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
