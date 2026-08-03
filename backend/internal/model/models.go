package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	UserTypeAdmin    = "ADMIN"
	UserTypeMerchant = "MERCHANT"
	UserTypeBuyer    = "BUYER"
	UserTypePublic   = "PUBLIC"

	AdminRoleSuper = "SUPER_ADMIN"
	AdminRoleAdmin = "ADMIN"

	AccountRoleOwner = "OWNER"
	AccountRoleStaff = "STAFF"

	AccountStatusActive   = "ACTIVE"
	AccountStatusDisabled = "DISABLED"

	ReviewPending  = "PENDING"
	ReviewApproved = "APPROVED"
	ReviewRejected = "REJECTED"
	ReviewDisabled = "DISABLED"

	CategoryEnabled  = "ENABLED"
	CategoryDisabled = "DISABLED"

	ProductDraft    = "DRAFT"
	ProductOnShelf  = "ON_SHELF"
	ProductLocked   = "LOCKED"
	ProductOffShelf = "OFF_SHELF"
	ProductSold     = "SOLD"
	ProductClosed   = "CLOSED"

	OrderCreated   = "CREATED"
	OrderCompleted = "COMPLETED"
	OrderClosed    = "CLOSED"

	FileScanPending = "PENDING"
	FileScanPass    = "PASS"
	FileScanBlocked = "BLOCKED"

	FileBizMerchantLicense = "MERCHANT_LICENSE"
	FileBizProductImage    = "PRODUCT_IMAGE"
	FileBizOther           = "OTHER"

	BuyerStatusActive   = "ACTIVE"
	BuyerStatusDisabled = "DISABLED"

	OwnerTypeBuyer  = "BUYER"
	OwnerTypeDevice = "DEVICE"

	IntentNew       = "NEW"
	IntentContacted = "CONTACTED"
	IntentClosed    = "CLOSED"
)

type Merchant struct {
	ID            uint64  `gorm:"primaryKey"`
	MerchantNo    string  `gorm:"size:32;uniqueIndex"`
	MerchantName  string  `gorm:"size:128"`
	ContactName   string  `gorm:"size:64"`
	ContactPhone  string  `gorm:"size:20;index"`
	ContactEmail  *string `gorm:"size:128"`
	LicenseNo     *string `gorm:"size:64"`
	LicenseFileID *uint64
	ReviewStatus  string  `gorm:"size:16;index"`
	RejectReason  *string `gorm:"size:255"`
	ReviewedBy    *uint64
	ReviewedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

type MerchantAccount struct {
	ID           uint64 `gorm:"primaryKey"`
	MerchantID   uint64 `gorm:"index:idx_merchant_role,priority:1"`
	Username     string `gorm:"size:64;uniqueIndex"`
	PasswordHash string `gorm:"size:255"`
	Role         string `gorm:"size:16;index:idx_merchant_role,priority:2"`
	Status       string `gorm:"size:16;index"`
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

type AdminUser struct {
	ID           uint64 `gorm:"primaryKey"`
	Username     string `gorm:"size:64;uniqueIndex"`
	PasswordHash string `gorm:"size:255"`
	DisplayName  string `gorm:"size:64"`
	Role         string `gorm:"size:16"`
	Status       string `gorm:"size:16;index"`
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

type MerchantAuditLog struct {
	ID           uint64  `gorm:"primaryKey"`
	MerchantID   uint64  `gorm:"index"`
	Action       string  `gorm:"size:32"`
	FromStatus   string  `gorm:"size:16"`
	ToStatus     string  `gorm:"size:16"`
	Reason       *string `gorm:"size:255"`
	OperatorType string  `gorm:"size:16"`
	OperatorID   uint64
	CreatedAt    time.Time `gorm:"index"`
}

type Category struct {
	ID        uint64         `gorm:"primaryKey" json:"id"`
	ParentID  *uint64        `gorm:"index:idx_parent_sort,priority:1" json:"parent_id"`
	Level     int8           `gorm:"index:idx_level_status_sort,priority:1" json:"level"`
	Name      string         `gorm:"size:64;uniqueIndex:uk_parent_name,priority:2" json:"name"`
	Status    string         `gorm:"size:16;index:idx_level_status_sort,priority:2" json:"status"`
	Sort      int            `gorm:"index:idx_parent_sort,priority:2;index:idx_level_status_sort,priority:3" json:"sort"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (c Category) TableName() string {
	return "categories"
}

type Product struct {
	ID                uint64 `gorm:"primaryKey"`
	ProductNo         string `gorm:"size:32;uniqueIndex"`
	MerchantID        uint64 `gorm:"index:idx_merchant_status_updated,priority:1;index:idx_merchant_title,priority:1"`
	Title             string `gorm:"size:128;index:idx_merchant_title,priority:2"`
	Description       string `gorm:"type:text"`
	CategoryID        uint64
	PriceCent         int
	OriginalPriceCent *int
	ConditionLevel    string `gorm:"size:16"`
	Stock             int
	CoverFileID       *uint64
	Status            string  `gorm:"size:16;index:idx_merchant_status_updated,priority:2"`
	ActiveOrderID     *uint64 `gorm:"index"`
	LockedAt          *time.Time
	ShelfAt           *time.Time
	OffShelfAt        *time.Time
	SoldAt            *time.Time
	ClosedAt          *time.Time
	CreatedBy         uint64
	UpdatedBy         uint64
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time      `gorm:"index:idx_merchant_status_updated,priority:3"`
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

type ProductImage struct {
	ID        uint64 `gorm:"primaryKey"`
	ProductID uint64 `gorm:"index:idx_product_sort,priority:1"`
	FileID    uint64
	SortOrder int `gorm:"index:idx_product_sort,priority:2"`
	CreatedAt time.Time
}

type Order struct {
	ID                 uint64 `gorm:"primaryKey"`
	OrderNo            string `gorm:"size:32;uniqueIndex"`
	MerchantID         uint64 `gorm:"index:idx_merchant_status_created,priority:1"`
	ProductID          uint64 `gorm:"index;uniqueIndex:uk_product_active,priority:1"`
	DealPriceCent      int
	BuyerContactMasked *string `gorm:"size:64"`
	Remark             *string `gorm:"size:255"`
	Status             string  `gorm:"size:16;index:idx_merchant_status_created,priority:2"`
	IsActive           bool    `gorm:"uniqueIndex:uk_product_active,priority:2"`
	CloseReason        *string `gorm:"size:255"`
	CreatedBy          uint64
	CompletedAt        *time.Time
	ClosedAt           *time.Time
	CreatedAt          time.Time `gorm:"index:idx_merchant_status_created,priority:3"`
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

type OrderEvent struct {
	ID           uint64  `gorm:"primaryKey"`
	OrderID      uint64  `gorm:"index:idx_order_created,priority:1"`
	EventType    string  `gorm:"size:32"`
	FromStatus   *string `gorm:"size:16"`
	ToStatus     string  `gorm:"size:16"`
	OperatorType string  `gorm:"size:16"`
	OperatorID   uint64
	Note         *string   `gorm:"size:255"`
	CreatedAt    time.Time `gorm:"index:idx_order_created,priority:2"`
}

type FileRecord struct {
	ID           uint64 `gorm:"primaryKey"`
	BizType      string `gorm:"size:32;index:idx_biz_type_created,priority:1"`
	ObjectKey    string `gorm:"size:255;uniqueIndex"`
	URL          string `gorm:"size:500"`
	MimeType     string `gorm:"size:64"`
	SizeBytes    int64
	UploaderType string `gorm:"size:16"`
	UploaderID   *uint64
	ScanStatus   string    `gorm:"size:16"`
	CreatedAt    time.Time `gorm:"index:idx_biz_type_created,priority:2"`
}

type ImageBackfillRun struct {
	ID             string `gorm:"primaryKey;size:64"`
	ProfileVersion string `gorm:"size:32;index"`
	CreatedAt      time.Time
	FinishedAt     *time.Time
}

type ImageBackfillItem struct {
	ID               uint64  `gorm:"primaryKey"`
	RunID            string  `gorm:"size:64;uniqueIndex:uk_backfill_run_file,priority:1;index"`
	FileID           uint64  `gorm:"uniqueIndex:uk_backfill_run_file,priority:2;index"`
	SourceObjectKey  string  `gorm:"size:255"`
	TargetObjectKey  string  `gorm:"size:255"`
	ProfileVersion   string  `gorm:"size:32;index"`
	SourceSHA256     *string `gorm:"size:64"`
	OutputSHA256     *string `gorm:"size:64"`
	SourceSizeBytes  int64
	OutputSizeBytes  *int64
	Status           string `gorm:"size:16;index"`
	Attempts         int
	ErrorCode        *string `gorm:"size:64"`
	CommittedAt      *time.Time
	CleanupAfter     *time.Time `gorm:"index"`
	CleanupStatus    string     `gorm:"size:16;index"`
	CleanupErrorCode *string    `gorm:"size:64"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type OperationLog struct {
	ID           uint64  `gorm:"primaryKey"`
	RequestID    string  `gorm:"size:64"`
	OperatorType string  `gorm:"size:16;index:idx_operator_created,priority:1"`
	OperatorID   uint64  `gorm:"index:idx_operator_created,priority:2"`
	MerchantID   *uint64 `gorm:"index:idx_merchant_created,priority:1"`
	Action       string  `gorm:"size:64"`
	ResourceType string  `gorm:"size:32;index:idx_resource_created,priority:1"`
	ResourceID   uint64  `gorm:"index:idx_resource_created,priority:2"`
	FromStatus   *string `gorm:"size:16"`
	ToStatus     *string `gorm:"size:16"`
	Method       string  `gorm:"size:8"`
	Path         string  `gorm:"size:255"`
	IP           string  `gorm:"size:64"`
	UserAgent    string  `gorm:"size:255"`
	ResultCode   int
	DetailJSON   datatypes.JSON `gorm:"type:json"`
	CreatedAt    time.Time      `gorm:"index:idx_operator_created,priority:3;index:idx_merchant_created,priority:2;index:idx_resource_created,priority:3"`
}

type AuthSession struct {
	ID               uint64    `gorm:"primaryKey"`
	UserType         string    `gorm:"size:16;index:idx_user_expired,priority:1"`
	UserID           uint64    `gorm:"index:idx_user_expired,priority:2"`
	RefreshTokenHash string    `gorm:"size:255;index"`
	DeviceInfo       *string   `gorm:"size:255"`
	IP               *string   `gorm:"size:64"`
	ExpiredAt        time.Time `gorm:"index:idx_user_expired,priority:3"`
	RevokedAt        *time.Time
	CreatedAt        time.Time
}

type BuyerUser struct {
	ID           uint64  `gorm:"primaryKey"`
	BuyerNo      string  `gorm:"size:32;uniqueIndex"`
	AuthProvider string  `gorm:"column:auth_provider;size:16;default:wechat;uniqueIndex:uk_buyer_provider_openid,priority:1"`
	OpenID       string  `gorm:"column:openid;size:64;uniqueIndex:uk_buyer_provider_openid,priority:2"`
	UnionID      *string `gorm:"column:unionid;size:64;index"`
	Nickname     *string `gorm:"size:64"`
	AvatarURL    *string `gorm:"size:500"`
	Phone        *string `gorm:"size:20"`
	Status       string  `gorm:"size:16;index:idx_status_created,priority:1"`
	LastLoginAt  *time.Time
	CreatedAt    time.Time `gorm:"index:idx_status_created,priority:2"`
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

type BuyerDeviceBinding struct {
	ID          uint64 `gorm:"primaryKey"`
	DeviceID    string `gorm:"size:64;uniqueIndex:uk_device_buyer,priority:1;index:idx_device_last_bind,priority:1"`
	BuyerID     uint64 `gorm:"uniqueIndex:uk_device_buyer,priority:2;index:idx_buyer_last_bind,priority:1"`
	FirstBindAt time.Time
	LastBindAt  time.Time `gorm:"index:idx_buyer_last_bind,priority:2;index:idx_device_last_bind,priority:2"`
	LastMergeAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type BuyerFavorite struct {
	ID                 uint64  `gorm:"primaryKey"`
	OwnerType          string  `gorm:"size:16"`
	OwnerKey           string  `gorm:"size:96;uniqueIndex:uk_buyer_favorite_owner_product,priority:1"`
	BuyerID            *uint64 `gorm:"index:idx_buyer_active_created,priority:1"`
	DeviceID           *string `gorm:"size:64;index:idx_device_active_created,priority:1"`
	ProductID          uint64  `gorm:"uniqueIndex:uk_buyer_favorite_owner_product,priority:2;index:idx_product_active_created,priority:1"`
	MerchantID         uint64
	IsActive           bool `gorm:"index:idx_buyer_active_created,priority:2;index:idx_device_active_created,priority:2;index:idx_product_active_created,priority:2"`
	MergeTargetBuyerID *uint64
	MergedAt           *time.Time
	CreatedAt          time.Time `gorm:"index:idx_buyer_active_created,priority:3;index:idx_device_active_created,priority:3;index:idx_product_active_created,priority:3"`
	UpdatedAt          time.Time
}

type BuyerHistory struct {
	ID                 uint64  `gorm:"primaryKey"`
	OwnerType          string  `gorm:"size:16"`
	OwnerKey           string  `gorm:"size:96;uniqueIndex:uk_buyer_history_owner_product,priority:1;index:idx_owner_last_view,priority:1"`
	BuyerID            *uint64 `gorm:"index:idx_buyer_last_view,priority:1"`
	DeviceID           *string `gorm:"size:64;index:idx_device_last_view,priority:1"`
	ProductID          uint64  `gorm:"uniqueIndex:uk_buyer_history_owner_product,priority:2;index:idx_product_last_view,priority:1"`
	MerchantID         uint64
	FirstViewedAt      time.Time
	LastViewedAt       time.Time `gorm:"index:idx_owner_last_view,priority:3;index:idx_buyer_last_view,priority:3;index:idx_device_last_view,priority:3;index:idx_product_last_view,priority:2"`
	ViewCount          int
	IsActive           bool `gorm:"index:idx_owner_last_view,priority:2;index:idx_buyer_last_view,priority:2;index:idx_device_last_view,priority:2"`
	MergeTargetBuyerID *uint64
	MergedAt           *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type BuyerIntent struct {
	ID             uint64  `gorm:"primaryKey"`
	IntentNo       string  `gorm:"size:32;uniqueIndex"`
	BuyerID        uint64  `gorm:"uniqueIndex:uk_buyer_product_open,priority:1;index:idx_buyer_intent_buyer_created,priority:1"`
	SourceDeviceID *string `gorm:"size:64;index:idx_buyer_intent_source_device_created,priority:1"`
	ProductID      uint64  `gorm:"uniqueIndex:uk_buyer_product_open,priority:2;index:idx_buyer_intent_product_open,priority:1"`
	MerchantID     uint64  `gorm:"index:idx_buyer_intent_merchant_status_created,priority:1"`
	Status         string  `gorm:"size:16;index:idx_buyer_intent_merchant_status_created,priority:2"`
	IsOpen         bool    `gorm:"uniqueIndex:uk_buyer_product_open,priority:3;index:idx_buyer_intent_product_open,priority:2"`
	ContactName    *string `gorm:"size:64"`
	ContactPhone   *string `gorm:"size:20"`
	ContactWechat  *string `gorm:"size:64"`
	Message        *string `gorm:"size:500"`
	HandledBy      *uint64
	HandledAt      *time.Time
	ClosedAt       *time.Time
	CloseReason    *string   `gorm:"size:32"`
	MerchantNote   *string   `gorm:"size:255"`
	CreatedAt      time.Time `gorm:"index:idx_buyer_intent_merchant_status_created,priority:3;index:idx_buyer_intent_buyer_created,priority:2;index:idx_buyer_intent_source_device_created,priority:2"`
	UpdatedAt      time.Time
}

type IdempotencyRecord struct {
	ID          uint64 `gorm:"primaryKey"`
	IdemKey     string `gorm:"size:128;uniqueIndex:uk_idem_scope,priority:1"`
	OperatorID  uint64 `gorm:"uniqueIndex:uk_idem_scope,priority:2"`
	Path        string `gorm:"size:255;uniqueIndex:uk_idem_scope,priority:3"`
	RequestHash string `gorm:"size:64"`
	ResultCode  int
	ResponseRaw datatypes.JSON `gorm:"type:json"`
	CreatedAt   time.Time
}
