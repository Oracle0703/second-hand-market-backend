package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
)

var buyerDetailVisibleStatuses = []string{model.ProductOnShelf, model.ProductLocked, model.ProductOffShelf, model.ProductSold}

type buyerOwner struct {
	OwnerType string
	OwnerKey  string
	BuyerID   *uint64
	DeviceID  *string
}

func ownerKeyForBuyer(buyerID uint64) string {
	return fmt.Sprintf("B:%d", buyerID)
}

func ownerKeyForDevice(deviceID string) string {
	return fmt.Sprintf("D:%s", deviceID)
}

func buyerStatusText(status string) string {
	switch status {
	case model.IntentContacted:
		return "已联系"
	case model.IntentClosed:
		return "已关闭"
	default:
		return "处理中"
	}
}

func getDeviceID(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("X-Device-Id"))
}

func isBuyerProductDetailVisible(status string) bool {
	for _, st := range buyerDetailVisibleStatuses {
		if st == status {
			return true
		}
	}
	return false
}

func (s *Server) ensureBuyerOrGuest(c *gin.Context) (*common.Actor, error) {
	actor, ok := common.GetActor(c)
	if !ok {
		return nil, nil
	}
	if actor.UserType != model.UserTypeBuyer {
		return nil, common.ErrForbidden
	}
	return &actor, nil
}

func (s *Server) resolveBuyerOwner(c *gin.Context, requireDeviceForGuest bool) (buyerOwner, error) {
	actor, err := s.ensureBuyerOrGuest(c)
	if err != nil {
		return buyerOwner{}, err
	}
	if actor != nil {
		id := actor.UserID
		return buyerOwner{OwnerType: model.OwnerTypeBuyer, OwnerKey: ownerKeyForBuyer(id), BuyerID: &id}, nil
	}
	deviceID := getDeviceID(c)
	if requireDeviceForGuest && deviceID == "" {
		return buyerOwner{}, common.ErrInvalidArgument
	}
	if deviceID == "" {
		return buyerOwner{}, nil
	}
	device := deviceID
	return buyerOwner{OwnerType: model.OwnerTypeDevice, OwnerKey: ownerKeyForDevice(deviceID), DeviceID: &device}, nil
}

func (s *Server) checkRateLimit(scope, key string, limit int, window time.Duration) error {
	if !s.limiter.allow(scope, key, limit, window) {
		return common.ErrRateLimit
	}
	return nil
}

func (s *Server) loadProductCoverURLMap(productIDs []uint64) (map[uint64]string, error) {
	result := map[uint64]string{}
	if len(productIDs) == 0 {
		return result, nil
	}

	products := make([]model.Product, 0, len(productIDs))
	if err := s.DB.Select("id", "cover_file_id").Where("id IN ?", productIDs).Find(&products).Error; err != nil {
		return nil, err
	}

	coverFileIDs := make([]uint64, 0, len(products))
	productByCoverID := map[uint64][]uint64{}
	for _, product := range products {
		if product.CoverFileID == nil || *product.CoverFileID == 0 {
			continue
		}
		coverFileIDs = append(coverFileIDs, *product.CoverFileID)
		productByCoverID[*product.CoverFileID] = append(productByCoverID[*product.CoverFileID], product.ID)
	}

	if len(coverFileIDs) > 0 {
		var files []model.FileRecord
		if err := s.DB.Where("id IN ?", coverFileIDs).Find(&files).Error; err != nil {
			return nil, err
		}
		for _, file := range files {
			url := strings.TrimSpace(file.URL)
			if url == "" && strings.TrimSpace(file.ObjectKey) != "" {
				url = s.publicFileURL(file.ObjectKey)
			}
			if url == "" {
				continue
			}
			for _, productID := range productByCoverID[file.ID] {
				result[productID] = url
			}
		}
	}

	missing := make([]uint64, 0)
	for _, productID := range productIDs {
		if strings.TrimSpace(result[productID]) == "" {
			missing = append(missing, productID)
		}
	}
	if len(missing) == 0 {
		return result, nil
	}

	var productImages []model.ProductImage
	if err := s.DB.Where("product_id IN ?", missing).Order("product_id ASC, sort_order ASC, id ASC").Find(&productImages).Error; err != nil {
		return nil, err
	}

	fileIDs := make([]uint64, 0, len(productImages))
	for _, image := range productImages {
		fileIDs = append(fileIDs, image.FileID)
	}
	fileURLMap := map[uint64]string{}
	if len(fileIDs) > 0 {
		var files []model.FileRecord
		if err := s.DB.Where("id IN ?", fileIDs).Find(&files).Error; err != nil {
			return nil, err
		}
		for _, file := range files {
			url := strings.TrimSpace(file.URL)
			if url == "" && strings.TrimSpace(file.ObjectKey) != "" {
				url = s.publicFileURL(file.ObjectKey)
			}
			if url != "" {
				fileURLMap[file.ID] = url
			}
		}
	}

	for _, image := range productImages {
		if strings.TrimSpace(result[image.ProductID]) != "" {
			continue
		}
		url := strings.TrimSpace(fileURLMap[image.FileID])
		if url != "" {
			result[image.ProductID] = url
		}
	}
	return result, nil
}

func (s *Server) loadFavoritedSet(ownerKey string, productIDs []uint64) map[uint64]bool {
	set := map[uint64]bool{}
	if ownerKey == "" || len(productIDs) == 0 {
		return set
	}
	var rows []model.BuyerFavorite
	if err := s.DB.Where("owner_key = ? AND is_active = ? AND product_id IN ?", ownerKey, true, productIDs).Find(&rows).Error; err != nil {
		return set
	}
	for _, it := range rows {
		set[it.ProductID] = true
	}
	return set
}

func parseViewedAt(raw *string) (time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return time.Now(), nil
	}
	ts, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return time.Time{}, common.ErrInvalidArgument
	}
	return ts, nil
}

func (s *Server) handleBuyerCategories(c *gin.Context) {
	if _, err := s.ensureBuyerOrGuest(c); err != nil {
		common.Fail(c, err)
		return
	}
	query := s.DB.Model(&model.Category{}).Where("status = ?", model.CategoryEnabled)
	if v := c.Query("level"); v != "" {
		query = query.Where("level = ?", v)
	}
	if v := c.Query("parent_id"); v != "" {
		query = query.Where("parent_id = ?", v)
	}
	var items []model.Category
	if err := query.Order("sort ASC, id ASC").Find(&items).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"items": items})
}

func (s *Server) handleBuyerProducts(c *gin.Context) {
	if _, err := s.ensureBuyerOrGuest(c); err != nil {
		common.Fail(c, err)
		return
	}
	owner, err := s.resolveBuyerOwner(c, false)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Model(&model.Product{}).Where("status = ?", model.ProductOnShelf)
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		query = query.Where("title LIKE ?", "%"+kw+"%")
	}
	if v := strings.TrimSpace(c.Query("category_id")); v != "" {
		query = query.Where("category_id = ?", v)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}

	sort := strings.TrimSpace(c.Query("sort"))
	orderBy := "updated_at DESC"
	if sort == "price_asc" {
		orderBy = "price_cent ASC, id DESC"
	}
	if sort == "price_desc" {
		orderBy = "price_cent DESC, id DESC"
	}

	rows := make([]model.Product, 0, size)
	if err := query.Order(orderBy).Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}

	productIDs := make([]uint64, 0, len(rows))
	merchantIDs := make([]uint64, 0, len(rows))
	for _, it := range rows {
		productIDs = append(productIDs, it.ID)
		merchantIDs = append(merchantIDs, it.MerchantID)
	}
	favorited := s.loadFavoritedSet(owner.OwnerKey, productIDs)
	coverMap, _ := s.loadProductCoverURLMap(productIDs)
	merchantMap := map[uint64]string{}
	if len(merchantIDs) > 0 {
		var merchants []model.Merchant
		if err := s.DB.Model(&model.Merchant{}).Where("id IN ?", merchantIDs).Find(&merchants).Error; err == nil {
			for _, merchant := range merchants {
				merchantMap[merchant.ID] = merchant.MerchantName
			}
		}
	}

	items := make([]gin.H, 0, len(rows))
	for _, it := range rows {
		items = append(items, gin.H{
			"id":                  it.ID,
			"title":               it.Title,
			"price_cent":          it.PriceCent,
			"original_price_cent": it.OriginalPriceCent,
			"stock":               it.Stock - it.ReservedStock,
			"condition_level":     it.ConditionLevel,
			"cover_url":           coverMap[it.ID],
			"status":              it.Status,
			"merchant_id":         it.MerchantID,
			"merchant_name":       merchantMap[it.MerchantID],
			"is_favorited":        favorited[it.ID],
		})
	}
	common.Success(c, common.PageResult[gin.H]{Items: items, Total: total, Page: page, PageSize: size})
}

func (s *Server) handleBuyerProductDetail(c *gin.Context) {
	if _, err := s.ensureBuyerOrGuest(c); err != nil {
		common.Fail(c, err)
		return
	}
	owner, err := s.resolveBuyerOwner(c, false)
	if err != nil {
		common.Fail(c, err)
		return
	}
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	var product model.Product
	if err := s.DB.Where("id = ?", id).First(&product).Error; err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	if !isBuyerProductDetailVisible(product.Status) {
		common.Fail(c, common.ErrNotFound)
		return
	}
	var merchant model.Merchant
	if err := s.DB.Where("id = ?", product.MerchantID).First(&merchant).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}

	var imgs []model.ProductImage
	if err := s.DB.Where("product_id = ?", product.ID).Order("sort_order ASC").Find(&imgs).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	fileIDs := make([]uint64, 0, len(imgs))
	for _, it := range imgs {
		fileIDs = append(fileIDs, it.FileID)
	}
	urlMap := map[uint64]string{}
	if len(fileIDs) > 0 {
		var files []model.FileRecord
		if err := s.DB.Where("id IN ?", fileIDs).Find(&files).Error; err != nil {
			common.Fail(c, common.ErrInternal)
			return
		}
		for _, file := range files {
			urlMap[file.ID] = file.URL
		}
	}
	images := make([]string, 0, len(imgs))
	for _, img := range imgs {
		images = append(images, urlMap[img.FileID])
	}

	isFavorited := false
	if owner.OwnerKey != "" {
		var cnt int64
		if err := s.DB.Model(&model.BuyerFavorite{}).Where("owner_key = ? AND product_id = ? AND is_active = ?", owner.OwnerKey, product.ID, true).Count(&cnt).Error; err == nil {
			isFavorited = cnt > 0
		}
	}

	common.Success(c, gin.H{"product": gin.H{
		"id":                  product.ID,
		"title":               product.Title,
		"description":         product.Description,
		"price_cent":          product.PriceCent,
		"original_price_cent": product.OriginalPriceCent,
		"stock":               product.Stock - product.ReservedStock,
		"condition_level":     product.ConditionLevel,
		"status":              product.Status,
		"images":              images,
		"merchant":            gin.H{"id": merchant.ID, "name": merchant.MerchantName},
		"is_favorited":        isFavorited,
		"can_submit_intent":   product.Status == model.ProductOnShelf,
	}})
}

func (s *Server) loadBuyerVisibleProduct(productID uint64) (model.Product, error) {
	var product model.Product
	if err := s.DB.Where("id = ?", productID).First(&product).Error; err != nil {
		return model.Product{}, s.dbError(err)
	}
	if !isBuyerProductDetailVisible(product.Status) {
		return model.Product{}, common.ErrNotFound
	}
	return product, nil
}

func (s *Server) handleBuyerMiniProgramLogin(c *gin.Context) {
	var req dto.BuyerMiniProgramLoginRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.handleBuyerMiniProgramLoginRequest(c, req); err != nil {
		common.Fail(c, err)
	}
}

func (s *Server) handleBuyerWechatLogin(c *gin.Context) {
	var req dto.BuyerMiniProgramLoginRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	req.Provider = miniProgramProviderWechat
	if err := s.handleBuyerMiniProgramLoginRequest(c, req); err != nil {
		common.Fail(c, err)
	}
}

func (s *Server) handleBuyerMiniProgramLoginRequest(c *gin.Context, req dto.BuyerMiniProgramLoginRequest) error {
	if err := s.checkRateLimit("buyer:miniapp_login:device", req.DeviceID, 20, time.Minute); err != nil {
		return err
	}
	if err := s.checkRateLimit("buyer:miniapp_login:ip", c.ClientIP(), 120, time.Minute); err != nil {
		return err
	}

	now := time.Now()
	provider, openid, unionid, resolveErr := s.resolveMiniProgramIdentity(req.Provider, req.Code)
	if resolveErr != nil {
		return s.wrapMiniProgramLoginError(resolveErr)
	}

	var buyer model.BuyerUser
	err := s.DB.Where("auth_provider = ? AND openid = ?", provider, openid).First(&buyer).Error
	if err == gorm.ErrRecordNotFound {
		buyer = model.BuyerUser{
			BuyerNo:      common.BuildBizNo("B"),
			AuthProvider: provider,
			OpenID:       openid,
			UnionID:      unionid,
			Status:       model.BuyerStatusActive,
			Nickname:     trimOptional(req.Nickname),
			AvatarURL:    trimOptional(req.AvatarURL),
		}
		if err := s.DB.Create(&buyer).Error; err != nil {
			return common.ErrInternal
		}
	} else if err != nil {
		return common.ErrInternal
	} else {
		if buyer.Status == model.BuyerStatusDisabled {
			return common.ErrAccountDisabled
		}
		updates := map[string]interface{}{"last_login_at": &now}
		setOptionalUpdate(updates, "nickname", req.Nickname)
		setOptionalUpdate(updates, "avatar_url", req.AvatarURL)
		setOptionalUpdate(updates, "unionid", unionid)
		if err := s.DB.Model(&model.BuyerUser{}).Where("id = ?", buyer.ID).Updates(updates).Error; err != nil {
			return common.ErrInternal
		}
		setOptionalModelField(&buyer.Nickname, req.Nickname)
		setOptionalModelField(&buyer.AvatarURL, req.AvatarURL)
		setOptionalModelField(&buyer.UnionID, unionid)
	}
	_ = s.DB.Model(&model.BuyerUser{}).Where("id = ?", buyer.ID).Update("last_login_at", &now).Error

	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		var binding model.BuyerDeviceBinding
		err := tx.Where("device_id = ? AND buyer_id = ?", req.DeviceID, buyer.ID).First(&binding).Error
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&model.BuyerDeviceBinding{DeviceID: req.DeviceID, BuyerID: buyer.ID, FirstBindAt: now, LastBindAt: now}).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&model.BuyerDeviceBinding{}).Where("id = ?", binding.ID).Update("last_bind_at", now).Error
	}); err != nil {
		return common.ErrInternal
	}

	data, err := s.issueTokens(c, model.UserTypeBuyer, buyer.ID, model.UserTypeBuyer, 0, "full")
	if err != nil {
		return err
	}
	data["user"] = gin.H{
		"id": buyer.ID, "buyer_no": buyer.BuyerNo, "auth_provider": buyer.AuthProvider,
		"nickname": buyer.Nickname, "avatar_url": buyer.AvatarURL, "phone": buyer.Phone,
	}
	common.Success(c, data)
	return nil
}

func (s *Server) handleBuyerGuestMerge(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if actor.UserType != model.UserTypeBuyer {
		common.Fail(c, common.ErrForbidden)
		return
	}
	if err := s.checkRateLimit("buyer:guest_merge", fmt.Sprintf("%d", actor.UserID), 10, time.Minute); err != nil {
		common.Fail(c, err)
		return
	}

	var req dto.BuyerGuestMergeRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}

	now := time.Now()
	favoritesMerged := 0
	historiesMerged := 0

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		deviceOwner := ownerKeyForDevice(req.DeviceID)
		buyerOwner := ownerKeyForBuyer(actor.UserID)

		var favorites []model.BuyerFavorite
		if err := tx.Where("owner_key = ? AND is_active = ? AND merge_target_buyer_id IS NULL", deviceOwner, true).Find(&favorites).Error; err != nil {
			return err
		}
		for _, src := range favorites {
			var dst model.BuyerFavorite
			err := tx.Where("owner_key = ? AND product_id = ?", buyerOwner, src.ProductID).First(&dst).Error
			if err == gorm.ErrRecordNotFound {
				buyerID := actor.UserID
				if err := tx.Create(&model.BuyerFavorite{
					OwnerType:  model.OwnerTypeBuyer,
					OwnerKey:   buyerOwner,
					BuyerID:    &buyerID,
					ProductID:  src.ProductID,
					MerchantID: src.MerchantID,
					IsActive:   true,
				}).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else if !dst.IsActive {
				if err := tx.Model(&model.BuyerFavorite{}).Where("id = ?", dst.ID).Updates(map[string]interface{}{"is_active": true, "merchant_id": src.MerchantID}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.BuyerFavorite{}).Where("id = ?", src.ID).Updates(map[string]interface{}{"is_active": false, "merge_target_buyer_id": actor.UserID, "merged_at": &now}).Error; err != nil {
				return err
			}
			favoritesMerged++
		}

		var histories []model.BuyerHistory
		if err := tx.Where("owner_key = ? AND is_active = ? AND merge_target_buyer_id IS NULL", deviceOwner, true).Find(&histories).Error; err != nil {
			return err
		}
		for _, src := range histories {
			var dst model.BuyerHistory
			err := tx.Where("owner_key = ? AND product_id = ?", buyerOwner, src.ProductID).First(&dst).Error
			if err == gorm.ErrRecordNotFound {
				buyerID := actor.UserID
				if err := tx.Create(&model.BuyerHistory{
					OwnerType:     model.OwnerTypeBuyer,
					OwnerKey:      buyerOwner,
					BuyerID:       &buyerID,
					ProductID:     src.ProductID,
					MerchantID:    src.MerchantID,
					FirstViewedAt: src.FirstViewedAt,
					LastViewedAt:  src.LastViewedAt,
					ViewCount:     src.ViewCount,
					IsActive:      true,
				}).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				firstViewed := dst.FirstViewedAt
				if src.FirstViewedAt.Before(firstViewed) {
					firstViewed = src.FirstViewedAt
				}
				lastViewed := dst.LastViewedAt
				if src.LastViewedAt.After(lastViewed) {
					lastViewed = src.LastViewedAt
				}
				if err := tx.Model(&model.BuyerHistory{}).Where("id = ?", dst.ID).Updates(map[string]interface{}{
					"is_active":       true,
					"merchant_id":     src.MerchantID,
					"first_viewed_at": firstViewed,
					"last_viewed_at":  lastViewed,
					"view_count":      dst.ViewCount + src.ViewCount,
				}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.BuyerHistory{}).Where("id = ?", src.ID).Updates(map[string]interface{}{"is_active": false, "merge_target_buyer_id": actor.UserID, "merged_at": &now}).Error; err != nil {
				return err
			}
			historiesMerged++
		}

		var binding model.BuyerDeviceBinding
		err = tx.Where("device_id = ? AND buyer_id = ?", req.DeviceID, actor.UserID).First(&binding).Error
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&model.BuyerDeviceBinding{DeviceID: req.DeviceID, BuyerID: actor.UserID, FirstBindAt: now, LastBindAt: now, LastMergeAt: &now}).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&model.BuyerDeviceBinding{}).Where("id = ?", binding.ID).Updates(map[string]interface{}{"last_bind_at": now, "last_merge_at": &now}).Error
	})
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}

	common.Success(c, gin.H{
		"merged": gin.H{
			"favorites_count": favoritesMerged,
			"histories_count": historiesMerged,
		},
		"merged_at": now.Format(time.RFC3339),
	})
}

func (s *Server) handleBuyerFavoriteList(c *gin.Context) {
	owner, err := s.resolveBuyerOwner(c, true)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Model(&model.BuyerFavorite{}).Where("owner_key = ? AND is_active = ?", owner.OwnerKey, true)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	var favs []model.BuyerFavorite
	if err := query.Order("updated_at DESC").Offset((page - 1) * size).Limit(size).Find(&favs).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	productIDs := make([]uint64, 0, len(favs))
	for _, it := range favs {
		productIDs = append(productIDs, it.ProductID)
	}
	coverMap, _ := s.loadProductCoverURLMap(productIDs)

	type productRow struct {
		ID                uint64
		Title             string
		PriceCent         int
		OriginalPriceCent *int
		Stock             int
		ReservedStock     int
		Status            string
	}
	rows := make([]productRow, 0, len(productIDs))
	if len(productIDs) > 0 {
		if err := s.DB.Model(&model.Product{}).Where("id IN ?", productIDs).Find(&rows).Error; err != nil {
			common.Fail(c, common.ErrInternal)
			return
		}
	}
	products := map[uint64]productRow{}
	for _, it := range rows {
		if !isBuyerProductDetailVisible(it.Status) {
			continue
		}
		products[it.ID] = it
	}
	items := make([]gin.H, 0, len(favs))
	for _, fav := range favs {
		p, ok := products[fav.ProductID]
		if !ok {
			continue
		}
		items = append(items, gin.H{
			"product_id":          fav.ProductID,
			"title":               p.Title,
			"cover_url":           coverMap[fav.ProductID],
			"price_cent":          p.PriceCent,
			"original_price_cent": p.OriginalPriceCent,
			"stock":               p.Stock - p.ReservedStock,
			"status":              p.Status,
			"favorited_at":        fav.CreatedAt,
		})
	}
	common.Success(c, common.PageResult[gin.H]{Items: items, Total: total, Page: page, PageSize: size})
}

func (s *Server) handleBuyerFavoriteAdd(c *gin.Context) {
	owner, err := s.resolveBuyerOwner(c, true)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.checkRateLimit("buyer:favorites:add", owner.OwnerKey, 60, time.Minute); err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.BuyerFavoriteRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	product, err := s.loadBuyerVisibleProduct(req.ProductID)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		var fav model.BuyerFavorite
		err := tx.Where("owner_key = ? AND product_id = ?", owner.OwnerKey, req.ProductID).First(&fav).Error
		if err == gorm.ErrRecordNotFound {
			fav = model.BuyerFavorite{
				OwnerType:  owner.OwnerType,
				OwnerKey:   owner.OwnerKey,
				BuyerID:    owner.BuyerID,
				DeviceID:   owner.DeviceID,
				ProductID:  req.ProductID,
				MerchantID: product.MerchantID,
				IsActive:   true,
			}
			return tx.Create(&fav).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&model.BuyerFavorite{}).Where("id = ?", fav.ID).Updates(map[string]interface{}{
			"owner_type":  owner.OwnerType,
			"buyer_id":    owner.BuyerID,
			"device_id":   owner.DeviceID,
			"merchant_id": product.MerchantID,
			"is_active":   true,
		}).Error
	}); err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"product_id": req.ProductID, "is_favorited": true})
}

func (s *Server) handleBuyerFavoriteDelete(c *gin.Context) {
	owner, err := s.resolveBuyerOwner(c, true)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.checkRateLimit("buyer:favorites:delete", owner.OwnerKey, 60, time.Minute); err != nil {
		common.Fail(c, err)
		return
	}
	productID, err := parseUintParam(c, "product_id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.DB.Model(&model.BuyerFavorite{}).Where("owner_key = ? AND product_id = ?", owner.OwnerKey, productID).Update("is_active", false).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"product_id": productID, "is_favorited": false})
}

func (s *Server) handleBuyerHistoryView(c *gin.Context) {
	owner, err := s.resolveBuyerOwner(c, true)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.checkRateLimit("buyer:histories:view", owner.OwnerKey, 120, time.Minute); err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.BuyerHistoryViewRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	viewedAt, err := parseViewedAt(req.ViewedAt)
	if err != nil {
		common.Fail(c, err)
		return
	}
	product, err := s.loadBuyerVisibleProduct(req.ProductID)
	if err != nil {
		common.Fail(c, err)
		return
	}

	respViewCount := 0
	respLastViewed := viewedAt
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		var history model.BuyerHistory
		err := tx.Where("owner_key = ? AND product_id = ?", owner.OwnerKey, req.ProductID).First(&history).Error
		if err == gorm.ErrRecordNotFound {
			h := model.BuyerHistory{
				OwnerType:     owner.OwnerType,
				OwnerKey:      owner.OwnerKey,
				BuyerID:       owner.BuyerID,
				DeviceID:      owner.DeviceID,
				ProductID:     req.ProductID,
				MerchantID:    product.MerchantID,
				FirstViewedAt: viewedAt,
				LastViewedAt:  viewedAt,
				ViewCount:     1,
				IsActive:      true,
			}
			if err := tx.Create(&h).Error; err != nil {
				return err
			}
			respViewCount = 1
			return nil
		}
		if err != nil {
			return err
		}
		if history.IsActive && viewedAt.Sub(history.LastViewedAt) < 30*time.Second {
			respViewCount = history.ViewCount
			respLastViewed = history.LastViewedAt
			return nil
		}
		updates := map[string]interface{}{
			"is_active":             true,
			"merchant_id":           product.MerchantID,
			"last_viewed_at":        viewedAt,
			"view_count":            history.ViewCount + 1,
			"merge_target_buyer_id": nil,
			"merged_at":             nil,
		}
		if !history.IsActive {
			updates["first_viewed_at"] = viewedAt
			updates["view_count"] = 1
		}
		if err := tx.Model(&model.BuyerHistory{}).Where("id = ?", history.ID).Updates(updates).Error; err != nil {
			return err
		}
		if !history.IsActive {
			respViewCount = 1
		} else {
			respViewCount = history.ViewCount + 1
		}
		respLastViewed = viewedAt
		return nil
	})
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"product_id": req.ProductID, "last_viewed_at": respLastViewed.Format(time.RFC3339), "view_count": respViewCount})
}

func (s *Server) handleBuyerHistoryList(c *gin.Context) {
	owner, err := s.resolveBuyerOwner(c, true)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Model(&model.BuyerHistory{}).Where("owner_key = ? AND is_active = ?", owner.OwnerKey, true)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	var histories []model.BuyerHistory
	if err := query.Order("last_viewed_at DESC").Offset((page - 1) * size).Limit(size).Find(&histories).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	productIDs := make([]uint64, 0, len(histories))
	for _, it := range histories {
		productIDs = append(productIDs, it.ProductID)
	}
	coverMap, _ := s.loadProductCoverURLMap(productIDs)
	type productRow struct {
		ID                uint64
		Title             string
		PriceCent         int
		OriginalPriceCent *int
		Stock             int
		ReservedStock     int
		Status            string
	}
	rows := make([]productRow, 0, len(productIDs))
	if len(productIDs) > 0 {
		if err := s.DB.Model(&model.Product{}).Where("id IN ?", productIDs).Find(&rows).Error; err != nil {
			common.Fail(c, common.ErrInternal)
			return
		}
	}
	products := map[uint64]productRow{}
	for _, it := range rows {
		if !isBuyerProductDetailVisible(it.Status) {
			continue
		}
		products[it.ID] = it
	}
	items := make([]gin.H, 0, len(histories))
	for _, it := range histories {
		p, ok := products[it.ProductID]
		if !ok {
			continue
		}
		items = append(items, gin.H{
			"product_id":          it.ProductID,
			"title":               p.Title,
			"cover_url":           coverMap[it.ProductID],
			"price_cent":          p.PriceCent,
			"original_price_cent": p.OriginalPriceCent,
			"stock":               p.Stock - p.ReservedStock,
			"status":              p.Status,
			"last_viewed_at":      it.LastViewedAt,
			"view_count":          it.ViewCount,
		})
	}
	common.Success(c, common.PageResult[gin.H]{Items: items, Total: total, Page: page, PageSize: size})
}

func (s *Server) handleBuyerHistoryDelete(c *gin.Context) {
	owner, err := s.resolveBuyerOwner(c, true)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.checkRateLimit("buyer:histories:delete", owner.OwnerKey, 30, time.Minute); err != nil {
		common.Fail(c, err)
		return
	}
	query := s.DB.Model(&model.BuyerHistory{}).Where("owner_key = ?", owner.OwnerKey)
	if v := strings.TrimSpace(c.Query("product_id")); v != "" {
		query = query.Where("product_id = ?", v)
	}
	if err := query.Update("is_active", false).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"success": true})
}

func (s *Server) handleBuyerIntentCreate(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if actor.UserType != model.UserTypeBuyer {
		common.Fail(c, common.ErrForbidden)
		return
	}
	deviceID := getDeviceID(c)

	var req dto.BuyerIntentCreateRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	if (req.ContactPhone == nil || strings.TrimSpace(*req.ContactPhone) == "") && (req.ContactWechat == nil || strings.TrimSpace(*req.ContactWechat) == "") {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}

	payload := map[string]interface{}{"product_id": req.ProductID, "contact_name": req.ContactName, "contact_phone": req.ContactPhone, "contact_wechat": req.ContactWechat, "message": req.Message}
	data, err := s.runWithIdempotency(c, payload, func(tx *gorm.DB) (map[string]interface{}, error) {
		if err := s.checkRateLimit("buyer:intent:min", fmt.Sprintf("%d", actor.UserID), 5, time.Minute); err != nil {
			return nil, err
		}
		if err := s.checkRateLimit("buyer:intent:day", fmt.Sprintf("%d", actor.UserID), 20, 24*time.Hour); err != nil {
			return nil, err
		}
		if deviceID != "" {
			if err := s.checkRateLimit("buyer:intent:device_day", deviceID, 30, 24*time.Hour); err != nil {
				return nil, err
			}
		}

		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", req.ProductID).First(&product).Error; err != nil {
			return nil, s.dbError(err)
		}
		if product.Status != model.ProductOnShelf {
			return nil, common.ErrInvalidTransition
		}

		found, err := findOpenBuyerIntent(tx, actor.UserID, req.ProductID)
		if err != nil {
			return nil, err
		}
		if found {
			return nil, common.ErrConflict
		}
		intent := model.BuyerIntent{
			IntentNo:       common.BuildBizNo("I"),
			BuyerID:        actor.UserID,
			SourceDeviceID: &deviceID,
			ProductID:      req.ProductID,
			MerchantID:     product.MerchantID,
			Status:         model.IntentNew,
			IsOpen:         true,
			ContactName:    req.ContactName,
			ContactPhone:   req.ContactPhone,
			ContactWechat:  req.ContactWechat,
			Message:        req.Message,
		}
		if err := tx.Create(&intent).Error; err != nil {
			return nil, classifyBuyerIntentCreateError(
				tx, err, actor.UserID, req.ProductID,
			)
		}
		return map[string]interface{}{
			"intent_id":  intent.ID,
			"intent_no":  intent.IntentNo,
			"status":     intent.Status,
			"created_at": intent.CreatedAt,
		}, nil
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, data)
}

func (s *Server) handleBuyerIntentList(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if actor.UserType != model.UserTypeBuyer {
		common.Fail(c, common.ErrForbidden)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Model(&model.BuyerIntent{}).Where("buyer_id = ?", actor.UserID)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	var intents []model.BuyerIntent
	if err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&intents).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	for _, intent := range intents {
		if err := validateBuyerIntentState(intent); err != nil {
			common.Fail(c, err)
			return
		}
	}
	productIDs := make([]uint64, 0, len(intents))
	for _, it := range intents {
		productIDs = append(productIDs, it.ProductID)
	}
	coverMap, _ := s.loadProductCoverURLMap(productIDs)
	type productRow struct {
		ID    uint64
		Title string
	}
	rows := make([]productRow, 0, len(productIDs))
	if len(productIDs) > 0 {
		if err := s.DB.Model(&model.Product{}).Where("id IN ?", productIDs).Find(&rows).Error; err != nil {
			common.Fail(c, common.ErrInternal)
			return
		}
	}
	productMap := map[uint64]productRow{}
	for _, it := range rows {
		productMap[it.ID] = it
	}
	items := make([]gin.H, 0, len(intents))
	for _, it := range intents {
		p := productMap[it.ProductID]
		items = append(items, gin.H{
			"id":                it.ID,
			"intent_no":         it.IntentNo,
			"product":           gin.H{"id": it.ProductID, "title": p.Title, "cover_url": coverMap[it.ProductID]},
			"status":            it.Status,
			"buyer_status_text": buyerStatusText(it.Status),
			"created_at":        it.CreatedAt,
			"updated_at":        it.UpdatedAt,
		})
	}
	common.Success(c, common.PageResult[gin.H]{Items: items, Total: total, Page: page, PageSize: size})
}

func maskPhone(phone *string) *string {
	if phone == nil {
		return nil
	}
	raw := strings.TrimSpace(*phone)
	if len(raw) < 7 {
		return phone
	}
	masked := raw[:3] + "****" + raw[len(raw)-4:]
	return &masked
}

func (s *Server) handleBuyerIntentDetail(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if actor.UserType != model.UserTypeBuyer {
		common.Fail(c, common.ErrForbidden)
		return
	}
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	var intent model.BuyerIntent
	if err := s.DB.Where("id = ?", id).First(&intent).Error; err != nil {
		common.Fail(c, s.dbError(err))
		return
	}
	if intent.BuyerID != actor.UserID {
		common.Fail(c, common.ErrForbidden)
		return
	}
	if err := validateBuyerIntentState(intent); err != nil {
		common.Fail(c, err)
		return
	}
	var product model.Product
	if err := s.DB.Where("id = ?", intent.ProductID).First(&product).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	coverMap, _ := s.loadProductCoverURLMap([]uint64{intent.ProductID})

	common.Success(c, gin.H{"intent": gin.H{
		"id":                intent.ID,
		"intent_no":         intent.IntentNo,
		"status":            intent.Status,
		"buyer_status_text": buyerStatusText(intent.Status),
		"product":           gin.H{"id": product.ID, "title": product.Title, "cover_url": coverMap[intent.ProductID]},
		"contact_masked":    maskPhone(intent.ContactPhone),
		"message":           intent.Message,
		"created_at":        intent.CreatedAt,
		"updated_at":        intent.UpdatedAt,
	}})
}

func (s *Server) handleBuyerSummary(c *gin.Context) {
	owner, err := s.resolveBuyerOwner(c, true)
	if err != nil {
		common.Fail(c, err)
		return
	}

	favorites := int64(0)
	histories := int64(0)
	_ = s.DB.Model(&model.BuyerFavorite{}).Where("owner_key = ? AND is_active = ?", owner.OwnerKey, true).Count(&favorites).Error
	_ = s.DB.Model(&model.BuyerHistory{}).Where("owner_key = ? AND is_active = ?", owner.OwnerKey, true).Count(&histories).Error

	isLogin := owner.BuyerID != nil
	profile := gin.H{}
	intentsOpen := int64(0)
	if owner.BuyerID != nil {
		var buyer model.BuyerUser
		if err := s.DB.Where("id = ?", *owner.BuyerID).First(&buyer).Error; err == nil {
			profile = gin.H{"buyer_id": buyer.ID, "nickname": buyer.Nickname, "avatar_url": buyer.AvatarURL}
		}
		_ = s.DB.Model(&model.BuyerIntent{}).Where("buyer_id = ? AND is_open = ?", *owner.BuyerID, true).Count(&intentsOpen).Error
	}

	common.Success(c, gin.H{
		"is_login": isLogin,
		"profile":  profile,
		"counters": gin.H{"favorites": favorites, "histories": histories, "intents_open": intentsOpen},
	})
}
