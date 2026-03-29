package dto

type RegisterRequest struct {
	MerchantName  string `json:"merchant_name" binding:"required,min=2,max=128"`
	ContactName   string `json:"contact_name" binding:"required,min=2,max=64"`
	Phone         string `json:"phone" binding:"required,min=6,max=20"`
	Username      string `json:"username" binding:"required,min=3,max=64"`
	Password      string `json:"password" binding:"required,min=8,max=128"`
	LicenseFileID uint64 `json:"license_file_id" binding:"required"`
}

type LoginRequest struct {
	LoginType string `json:"login_type" binding:"required,oneof=ADMIN MERCHANT"`
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type ReapplyRequest struct {
	MerchantName  *string `json:"merchant_name"`
	ContactName   *string `json:"contact_name"`
	Phone         *string `json:"phone"`
	LicenseFileID *uint64 `json:"license_file_id"`
}

type MerchantReviewRejectRequest struct {
	Reason string `json:"reason" binding:"required,min=2,max=255"`
}

type MerchantReviewApproveRequest struct {
	Comment *string `json:"comment"`
}

type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,min=8,max=128"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=128"`
}

type CreateProductRequest struct {
	Title             string   `json:"title" binding:"required,min=2,max=128"`
	Description       string   `json:"description" binding:"required,min=2,max=5000"`
	CategoryID        uint64   `json:"category_id" binding:"required"`
	PriceCent         int      `json:"price_cent" binding:"required,gt=0"`
	OriginalPriceCent int      `json:"original_price_cent" binding:"required,gt=0"`
	ConditionLevel    string   `json:"condition_level" binding:"required,oneof=LIKE_NEW GOOD FAIR POOR"`
	Stock             int      `json:"stock" binding:"required,gt=0"`
	ImageFileIDs      []uint64 `json:"image_file_ids" binding:"required,min=1,max=5,dive,gt=0"`
}

type UpdateProductRequest struct {
	Title             *string  `json:"title"`
	Description       *string  `json:"description"`
	CategoryID        *uint64  `json:"category_id"`
	PriceCent         *int     `json:"price_cent"`
	OriginalPriceCent *int     `json:"original_price_cent"`
	ConditionLevel    *string  `json:"condition_level"`
	Stock             *int     `json:"stock"`
	ImageFileIDs      []uint64 `json:"image_file_ids"`
}

type CreateOrderRequest struct {
	ProductID          uint64  `json:"product_id" binding:"required"`
	DealPriceCent      int     `json:"deal_price_cent" binding:"required,gt=0"`
	BuyerContactMasked *string `json:"buyer_contact_masked"`
	Remark             *string `json:"remark"`
}

type OrderActionRequest struct {
	Note   *string `json:"note"`
	Reason *string `json:"reason"`
}

type PresignRequest struct {
	BizType  string `json:"biz_type" binding:"required,min=2,max=32"`
	FileName string `json:"file_name" binding:"required,min=1,max=255"`
	FileSize int64  `json:"file_size" binding:"required,gt=0"`
	MIMEType string `json:"mime_type" binding:"required"`
}

type ConfirmUploadRequest struct {
	FileID    uint64 `json:"file_id" binding:"required"`
	ObjectKey string `json:"object_key" binding:"required"`
}

type BuyerMiniProgramLoginRequest struct {
	Provider  string  `json:"provider"`
	Code      string  `json:"code" binding:"required"`
	DeviceID  string  `json:"device_id" binding:"required,min=8,max=64"`
	Nickname  *string `json:"nickname"`
	AvatarURL *string `json:"avatar_url"`
}

type BuyerGuestMergeRequest struct {
	DeviceID string `json:"device_id" binding:"required,min=8,max=64"`
}

type BuyerFavoriteRequest struct {
	ProductID uint64 `json:"product_id" binding:"required"`
}

type BuyerHistoryViewRequest struct {
	ProductID uint64  `json:"product_id" binding:"required"`
	ViewedAt  *string `json:"viewed_at"`
}

type BuyerIntentCreateRequest struct {
	ProductID     uint64  `json:"product_id" binding:"required"`
	ContactName   *string `json:"contact_name"`
	ContactPhone  *string `json:"contact_phone"`
	ContactWechat *string `json:"contact_wechat"`
	Message       *string `json:"message"`
}

type MerchantIntentCloseRequest struct {
	Reason       *string `json:"reason"`
	MerchantNote *string `json:"merchant_note"`
}
