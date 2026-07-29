package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/media"
	"second-hand-market-backend/backend/internal/middleware"
	"second-hand-market-backend/backend/internal/model"
)

type Server struct {
	cfg            Config
	DB             *gorm.DB
	Router         *gin.Engine
	limiter        *memoryRateLimiter
	imageProcessor media.Processor
}

func NewServer(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.FileStorageProvider) == "" {
		cfg.FileStorageProvider = "local"
	}
	if !strings.EqualFold(cfg.FileStorageProvider, "local") {
		return nil, fmt.Errorf("unsupported file storage provider: %q", cfg.FileStorageProvider)
	}
	cfg.FileStorageProvider = "local"
	if strings.TrimSpace(cfg.FileUploadLocalDir) == "" {
		cfg.FileUploadLocalDir = "uploads"
	}
	if cfg.FileUploadMaxBytes <= 0 {
		cfg.FileUploadMaxBytes = media.DefaultMaxOriginalBytes
	}
	if cfg.ImageCompressTargetBytes <= 0 {
		cfg.ImageCompressTargetBytes = media.DefaultTargetBytes
	}
	if strings.EqualFold(cfg.FileStorageProvider, "local") &&
		strings.TrimSpace(cfg.FilePublicBaseURL) != "" {
		return nil, errors.New("FILE_PUBLIC_BASE_URL must be empty for local storage so /uploads security checks cannot be bypassed")
	}
	imageProcessor := buildImageProcessor(cfg)
	if imageProcessor == nil {
		return nil, fmt.Errorf("unsupported image processor driver: %q", cfg.ImageProcessorDriver)
	}
	db, err := openDB(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.AutoMigrate {
		if err := migrate(db); err != nil {
			return nil, err
		}
	}
	if err := seedDefaults(db); err != nil {
		return nil, err
	}
	if err := ensureUploadStorage(cfg); err != nil {
		return nil, err
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), middleware.RequestID(), middleware.OptionalAuth(cfg.JWTAccessSecret))
	s := &Server{
		cfg:            cfg,
		DB:             db,
		Router:         r,
		limiter:        newMemoryRateLimiter(),
		imageProcessor: imageProcessor,
	}
	if strings.EqualFold(cfg.FileStorageProvider, "local") {
		r.GET("/uploads/*object_key", s.handlePublicUpload)
		r.HEAD("/uploads/*object_key", s.handlePublicUpload)
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) SetImageProcessor(processor media.Processor) {
	if processor == nil {
		s.imageProcessor = buildImageProcessor(s.cfg)
		return
	}
	s.imageProcessor = processor
}

func buildImageProcessor(cfg Config) media.Processor {
	policy := media.DefaultUploadPolicy()
	policy.MaxOriginalBytes = cfg.FileUploadMaxBytes
	policy.TargetBytes = cfg.ImageCompressTargetBytes

	switch strings.ToLower(strings.TrimSpace(cfg.ImageProcessorDriver)) {
	case "", "vips":
		return media.NewVipsCLIProcessor(cfg.ImageProcessorBin, policy)
	case "passthrough":
		return media.NewPassthroughProcessor(policy)
	default:
		return nil
	}
}

func ensureUploadStorage(cfg Config) error {
	if !strings.EqualFold(cfg.FileStorageProvider, "local") {
		return nil
	}
	return os.MkdirAll(cfg.FileUploadLocalDir, 0o755)
}

func openDB(cfg Config) (*gorm.DB, error) {
	var dial gorm.Dialector
	switch cfg.DBDriver {
	case "mysql":
		dial = mysql.Open(cfg.DBDSN)
	case "sqlite":
		dial = sqlite.Open(cfg.DBDSN)
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", cfg.DBDriver)
	}
	return gorm.Open(dial, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

func migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.Merchant{},
		&model.MerchantAccount{},
		&model.AdminUser{},
		&model.MerchantAuditLog{},
		&model.Category{},
		&model.Product{},
		&model.ProductImage{},
		&model.Order{},
		&model.OrderEvent{},
		&model.FileRecord{},
		&model.OperationLog{},
		&model.AuthSession{},
		&model.IdempotencyRecord{},
		&model.BuyerUser{},
		&model.BuyerDeviceBinding{},
		&model.BuyerFavorite{},
		&model.BuyerHistory{},
		&model.BuyerIntent{},
	)
}

func seedDefaults(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.AdminUser{}).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte("Admin@123456"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		admins := []model.AdminUser{
			{Username: "superadmin", DisplayName: "Super Admin", Role: model.AdminRoleSuper, Status: model.AccountStatusActive, PasswordHash: string(hash)},
			{Username: "admin", DisplayName: "Admin", Role: model.AdminRoleAdmin, Status: model.AccountStatusActive, PasswordHash: string(hash)},
		}
		if err := db.Create(&admins).Error; err != nil {
			return err
		}
	}
	if err := db.Model(&model.Category{}).Count(&count).Error; err != nil {
		return err
	}
	return ensureDefaultCategories(db)
}

type categorySeed struct {
	Name     string
	Children []string
}

var defaultCategorySeeds = []categorySeed{
	{Name: "家具类", Children: []string{"家具", "家电", "麻将机", "商铺用品"}},
	{Name: "办公类", Children: []string{"老板桌", "办公桌", "老板椅", "老板办公座椅套装", "会议桌", "办公沙发", "会议桌椅套装", "文件柜书柜"}},
	{Name: "麻将机类", Children: []string{"旧麻将机", "新麻将机", "麻将椅", "茶几", "麻将机维修"}},
}

func ensureDefaultCategories(db *gorm.DB) error {
	for i, seed := range defaultCategorySeeds {
		root, err := findOrCreateCategory(db, nil, 1, seed.Name, i+1)
		if err != nil {
			return err
		}
		seen := map[string]struct{}{}
		sortOrder := 1
		for _, childName := range seed.Children {
			name := strings.TrimSpace(childName)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			if _, err := findOrCreateCategory(db, &root.ID, 2, name, sortOrder); err != nil {
				return err
			}
			sortOrder++
		}
	}
	return nil
}

func findOrCreateCategory(db *gorm.DB, parentID *uint64, level int8, name string, sort int) (model.Category, error) {
	var cat model.Category
	if err := db.Model(&model.Category{}).Where("name = ?", name).First(&cat).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cat = model.Category{
				ParentID: parentID,
				Level:    level,
				Name:     name,
				Status:   model.CategoryEnabled,
				Sort:     sort,
			}
			return cat, db.Create(&cat).Error
		}
		return model.Category{}, err
	}
	updates := map[string]interface{}{}
	if !sameParentID(cat.ParentID, parentID) {
		updates["parent_id"] = parentID
	}
	if cat.Level != level {
		updates["level"] = level
	}
	if cat.Status != model.CategoryEnabled {
		updates["status"] = model.CategoryEnabled
	}
	if cat.Sort != sort {
		updates["sort"] = sort
	}
	if len(updates) > 0 {
		if err := db.Model(&model.Category{}).Where("id = ?", cat.ID).Updates(updates).Error; err != nil {
			return model.Category{}, err
		}
		cat.Status = model.CategoryEnabled
		cat.Sort = sort
	}
	return cat, nil
}

func sameParentID(a, b *uint64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func (s *Server) registerRoutes() {
	r := s.Router
	r.GET("/healthz", func(c *gin.Context) {
		common.Success(c, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})
	v1 := r.Group("/api/v1")
	{
		v1.POST("/auth/register", s.handleRegister)
		v1.POST("/auth/login", s.handleLogin)
		v1.POST("/auth/refresh", s.handleRefresh)
		v1.POST("/files/presign", s.handlePresign)
		v1.POST("/files/upload", s.handleUploadFile)
		v1.POST("/files/confirm", s.handleConfirmUpload)

		v1.POST("/auth/logout", middleware.RequireAuth(model.UserTypeAdmin, model.UserTypeMerchant, model.UserTypeBuyer), s.handleLogout)

		merchant := v1.Group("/merchant")
		merchant.Use(middleware.RequireAuth(model.UserTypeMerchant))
		{
			merchant.GET("/profile", middleware.RequireMerchantScope("full", "onboarding"), s.handleMerchantProfile)
			merchant.POST("/reapply", middleware.RequireMerchantScope("onboarding"), s.handleMerchantReapply)

			merchant.GET("/account", middleware.RequireFullMerchantScope(), s.handleMerchantAccount)
			merchant.PUT("/account/password", middleware.RequireFullMerchantScope(), s.handleMerchantChangePassword)
			merchant.GET("/categories", middleware.RequireFullMerchantScope(), s.handleCategories)
			merchant.GET("/dashboard", middleware.RequireFullMerchantScope(), s.handleDashboard)

			merchant.POST("/products", middleware.RequireFullMerchantScope(), s.handleCreateProduct)
			merchant.PUT("/products/:id", middleware.RequireFullMerchantScope(), s.handleUpdateProduct)
			merchant.DELETE("/products/:id", middleware.RequireFullMerchantScope(), s.handleDeleteProduct)
			merchant.GET("/products/:id", middleware.RequireFullMerchantScope(), s.handleProductDetail)
			merchant.GET("/products", middleware.RequireFullMerchantScope(), s.handleProductList)
			merchant.POST("/products/:id/on-shelf", middleware.RequireFullMerchantScope(), s.handleProductOnShelf)
			merchant.POST("/products/:id/off-shelf", middleware.RequireFullMerchantScope(), s.handleProductOffShelf)
			merchant.POST("/products/:id/close", middleware.RequireFullMerchantScope(), s.handleProductClose)

			merchant.POST("/orders", middleware.RequireFullMerchantScope(), s.handleCreateOrder)
			merchant.GET("/orders", middleware.RequireFullMerchantScope(), s.handleOrderList)
			merchant.GET("/orders/:id", middleware.RequireFullMerchantScope(), s.handleOrderDetail)
			merchant.POST("/orders/:id/complete", middleware.RequireFullMerchantScope(), s.handleOrderComplete)
			merchant.POST("/orders/:id/close", middleware.RequireFullMerchantScope(), s.handleOrderClose)
			merchant.GET("/intents", middleware.RequireFullMerchantScope(), s.handleMerchantIntentList)
			merchant.GET("/intents/:id", middleware.RequireFullMerchantScope(), s.handleMerchantIntentDetail)
			merchant.POST("/intents/:id/contacted", middleware.RequireFullMerchantScope(), s.handleMerchantIntentContacted)
			merchant.POST("/intents/:id/close", middleware.RequireFullMerchantScope(), s.handleMerchantIntentClose)

			merchant.GET("/logs", middleware.RequireFullMerchantScope(), s.handleMerchantLogs)
		}

		buyer := v1.Group("/buyer")
		{
			buyer.GET("/categories", s.handleBuyerCategories)
			buyer.GET("/products", s.handleBuyerProducts)
			buyer.GET("/products/:id", s.handleBuyerProductDetail)

			buyer.POST("/auth/miniapp-login", s.handleBuyerMiniProgramLogin)
			buyer.POST("/auth/wechat-login", s.handleBuyerWechatLogin)
			buyer.POST("/auth/refresh", s.handleRefresh)
			buyer.POST("/auth/logout", middleware.RequireAuth(model.UserTypeBuyer), s.handleLogout)

			buyer.GET("/favorites", s.handleBuyerFavoriteList)
			buyer.POST("/favorites", s.handleBuyerFavoriteAdd)
			buyer.DELETE("/favorites/:product_id", s.handleBuyerFavoriteDelete)

			buyer.GET("/histories", s.handleBuyerHistoryList)
			buyer.POST("/histories/views", s.handleBuyerHistoryView)
			buyer.DELETE("/histories", s.handleBuyerHistoryDelete)
			buyer.GET("/me/summary", s.handleBuyerSummary)
		}

		buyerAuth := v1.Group("/buyer")
		buyerAuth.Use(middleware.RequireAuth(model.UserTypeBuyer))
		{
			buyerAuth.POST("/guest/merge", s.handleBuyerGuestMerge)
			buyerAuth.POST("/intents", s.handleBuyerIntentCreate)
			buyerAuth.GET("/intents", s.handleBuyerIntentList)
			buyerAuth.GET("/intents/:id", s.handleBuyerIntentDetail)
		}

		admin := v1.Group("/admin")
		admin.Use(middleware.RequireAuth(model.UserTypeAdmin))
		{
			admin.GET("/merchants", s.handleAdminMerchantList)
			admin.GET("/merchants/:id", s.handleAdminMerchantDetail)
			admin.POST("/merchants/:id/approve", s.handleAdminMerchantApprove)
			admin.POST("/merchants/:id/reject", s.handleAdminMerchantReject)
			admin.GET("/logs", s.handleAdminLogs)
		}
	}
}

func (s *Server) Run() error {
	log.Printf("server listening on %s", s.cfg.Addr)
	return s.Router.Run(s.cfg.Addr)
}

func actorFromContext(c *gin.Context) (common.Actor, error) {
	actor, ok := common.GetActor(c)
	if !ok {
		return common.Actor{}, common.ErrUnauthorized
	}
	return actor, nil
}

func bindJSON(c *gin.Context, req interface{}) error {
	if err := c.ShouldBindJSON(req); err != nil {
		return common.ErrInvalidArgument
	}
	return nil
}

func parseUintParam(c *gin.Context, name string) (uint64, error) {
	var id uint64
	if _, err := fmt.Sscan(c.Param(name), &id); err != nil || id == 0 {
		return 0, common.ErrInvalidArgument
	}
	return id, nil
}

func parsePage(c *gin.Context) (int, int) {
	page := 1
	size := 20
	if v := c.Query("page"); v != "" {
		_, _ = fmt.Sscan(v, &page)
	}
	if v := c.Query("page_size"); v != "" {
		_, _ = fmt.Sscan(v, &size)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func (s *Server) dbError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return common.ErrNotFound
	}
	return err
}

func (s *Server) writeOperationLog(c *gin.Context, tx *gorm.DB, resourceType string, resourceID uint64, action string, fromStatus, toStatus *string, code int, merchantID *uint64, detail map[string]interface{}) {
	actor, _ := common.GetActor(c)
	target := s.DB
	if tx != nil {
		target = tx
	}
	payload, _ := json.Marshal(detail)
	_ = target.Create(&model.OperationLog{
		RequestID:    common.RequestIDFromContext(c),
		OperatorType: actor.UserType,
		OperatorID:   actor.UserID,
		MerchantID:   merchantID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		FromStatus:   fromStatus,
		ToStatus:     toStatus,
		Method:       c.Request.Method,
		Path:         c.FullPath(),
		IP:           c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		ResultCode:   code,
		DetailJSON:   payload,
	}).Error
}

func abortWithErr(c *gin.Context, err error) {
	common.Fail(c, err)
	c.Abort()
}

func (s *Server) health(c *gin.Context) {
	common.Success(c, gin.H{"status": "ok"})
}

func (s *Server) ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
