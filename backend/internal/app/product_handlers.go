package app

import (
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
	"second-hand-market-backend/backend/internal/stateflow"
)

func (s *Server) ensureLevel2Category(categoryID uint64) error {
	var category model.Category
	if err := s.DB.Where("id = ?", categoryID).First(&category).Error; err != nil {
		return common.ErrInvalidArgument
	}
	if category.Level != 2 || category.Status != model.CategoryEnabled {
		return common.ErrInvalidArgument
	}
	return nil
}

func (s *Server) handleCreateProduct(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.CreateProductRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	if req.OriginalPriceCent < req.PriceCent || req.Stock <= 0 {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	if err := s.ensureLevel2Category(req.CategoryID); err != nil {
		common.Fail(c, err)
		return
	}
	product := model.Product{
		ProductNo:         common.BuildBizNo("P"),
		MerchantID:        actor.MerchantID,
		Title:             req.Title,
		Description:       req.Description,
		CategoryID:        req.CategoryID,
		PriceCent:         req.PriceCent,
		OriginalPriceCent: &req.OriginalPriceCent,
		ConditionLevel:    req.ConditionLevel,
		Stock:             req.Stock,
		Status:            model.ProductDraft,
		CreatedBy:         actor.UserID,
		UpdatedBy:         actor.UserID,
		Version:           1,
	}
	if len(req.ImageFileIDs) > 0 {
		cover := req.ImageFileIDs[0]
		product.CoverFileID = &cover
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&product).Error; err != nil {
			return err
		}
		for i, fileID := range req.ImageFileIDs {
			if err := tx.Create(&model.ProductImage{ProductID: product.ID, FileID: fileID, SortOrder: i + 1}).Error; err != nil {
				return err
			}
		}
		to := product.Status
		s.writeOperationLog(c, tx, "product", product.ID, "product_create", nil, &to, common.CodeOK, &actor.MerchantID, gin.H{"title": product.Title})
		return nil
	}); err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{
		"product_id":          product.ID,
		"product_no":          product.ProductNo,
		"status":              product.Status,
		"stock":               product.Stock,
		"reserved_stock":      product.ReservedStock,
		"available_stock":     product.Stock - product.ReservedStock,
		"original_price_cent": product.OriginalPriceCent,
		"created_at":          product.CreatedAt,
	})
}

func (s *Server) handleUpdateProduct(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.UpdateProductRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		product, err := s.loadOwnedProduct(tx, id, actor.MerchantID)
		if err != nil {
			return err
		}
		allowed := stateflow.EditableFieldsByProductStatus(product.Status)
		if req.Title != nil {
			if !allowed["title"] {
				return common.ErrInvalidTransition
			}
			product.Title = *req.Title
		}
		if req.Description != nil {
			if !allowed["description"] {
				return common.ErrInvalidTransition
			}
			product.Description = *req.Description
		}
		if req.CategoryID != nil {
			if !allowed["category_id"] {
				return common.ErrInvalidTransition
			}
			if err := s.ensureLevel2Category(*req.CategoryID); err != nil {
				return err
			}
			product.CategoryID = *req.CategoryID
		}
		if req.PriceCent != nil {
			if !allowed["price_cent"] || *req.PriceCent <= 0 {
				return common.ErrInvalidTransition
			}
			product.PriceCent = *req.PriceCent
		}
		if req.OriginalPriceCent != nil {
			if !allowed["original_price_cent"] || *req.OriginalPriceCent <= 0 {
				return common.ErrInvalidTransition
			}
			product.OriginalPriceCent = req.OriginalPriceCent
		}
		if req.ConditionLevel != nil {
			if !allowed["condition_level"] {
				return common.ErrInvalidTransition
			}
			product.ConditionLevel = *req.ConditionLevel
		}
		if req.Stock != nil {
			if !allowed["stock"] || *req.Stock <= 0 {
				return common.ErrInvalidTransition
			}
			if *req.Stock < product.ReservedStock {
				return common.NewBizError(common.CodeConflict, "stock cannot be lower than reserved stock", 409)
			}
			product.Stock = *req.Stock
		}
		if product.OriginalPriceCent != nil && *product.OriginalPriceCent < product.PriceCent {
			return common.ErrInvalidArgument
		}
		if req.ImageFileIDs != nil {
			if !allowed["image_file_ids"] || len(req.ImageFileIDs) == 0 || len(req.ImageFileIDs) > 5 {
				return common.ErrInvalidTransition
			}
			cover := req.ImageFileIDs[0]
			product.CoverFileID = &cover
			if err := tx.Where("product_id = ?", product.ID).Delete(&model.ProductImage{}).Error; err != nil {
				return err
			}
			for i, fileID := range req.ImageFileIDs {
				if err := tx.Create(&model.ProductImage{ProductID: product.ID, FileID: fileID, SortOrder: i + 1}).Error; err != nil {
					return err
				}
			}
		}
		product.UpdatedBy = actor.UserID
		product.Version++
		if err := tx.Save(&product).Error; err != nil {
			return err
		}
		s.writeOperationLog(c, tx, "product", product.ID, "product_update", nil, nil, common.CodeOK, &actor.MerchantID, nil)
		return nil
	}); err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, gin.H{"product_id": id, "status": "UPDATED", "updated_at": time.Now().Format(time.RFC3339)})
}

func (s *Server) handleProductDetail(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	product, err := s.loadOwnedProduct(nil, id, actor.MerchantID)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var imgs []model.ProductImage
	_ = s.DB.Where("product_id = ?", product.ID).Order("sort_order ASC").Find(&imgs).Error
	imgIDs := make([]uint64, 0, len(imgs))
	for _, it := range imgs {
		imgIDs = append(imgIDs, it.FileID)
	}
	urlMap := map[uint64]string{}
	if len(imgIDs) > 0 {
		var files []model.FileRecord
		if err := s.DB.Where("id IN ?", imgIDs).Find(&files).Error; err != nil {
			common.Fail(c, common.ErrInternal)
			return
		}
		for _, file := range files {
			url := strings.TrimSpace(file.URL)
			if url == "" && strings.TrimSpace(file.ObjectKey) != "" {
				url = s.publicFileURL(file.ObjectKey)
			}
			if url != "" {
				urlMap[file.ID] = url
			}
		}
	}
	imgURLs := make([]string, 0, len(imgs))
	for _, img := range imgs {
		if url, ok := urlMap[img.FileID]; ok && strings.TrimSpace(url) != "" {
			imgURLs = append(imgURLs, url)
		}
	}
	common.Success(c, gin.H{"product": gin.H{
		"id":                  product.ID,
		"title":               product.Title,
		"description":         product.Description,
		"status":              product.Status,
		"category_id":         product.CategoryID,
		"price_cent":          product.PriceCent,
		"original_price_cent": product.OriginalPriceCent,
		"condition_level":     product.ConditionLevel,
		"stock":               product.Stock,
		"reserved_stock":      product.ReservedStock,
		"available_stock":     product.Stock - product.ReservedStock,
		"images":              imgIDs,
		"image_urls":          imgURLs,
	}})
}

func (s *Server) handleProductList(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Table("products AS p").
		Joins("LEFT JOIN categories AS c2 ON c2.id = p.category_id").
		Joins("LEFT JOIN categories AS c1 ON c1.id = c2.parent_id").
		Where("p.merchant_id = ? AND p.deleted_at IS NULL", actor.MerchantID)
	if v := strings.TrimSpace(c.Query("status")); v != "" {
		query = query.Where("p.status = ?", v)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		query = query.Where("p.title LIKE ?", "%"+kw+"%")
	}
	if st := strings.TrimSpace(c.Query("start_at")); st != "" {
		query = query.Where("p.created_at >= ?", st)
	}
	if et := strings.TrimSpace(c.Query("end_at")); et != "" {
		query = query.Where("p.created_at <= ?", et)
	}
	if lv1 := strings.TrimSpace(c.Query("category_level1_id")); lv1 != "" {
		query = query.Where("c1.id = ?", lv1)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	type item struct {
		ID                 uint64    `json:"id"`
		Title              string    `json:"title"`
		Status             string    `json:"status"`
		PriceCent          int       `json:"price_cent"`
		OriginalPriceCent  *int      `json:"original_price_cent"`
		Stock              int       `json:"stock"`
		ReservedStock      int       `json:"reserved_stock"`
		AvailableStock     int       `json:"available_stock" gorm:"->"`
		UpdatedAt          time.Time `json:"updated_at"`
		CategoryLevel1ID   *uint64   `json:"category_level1_id"`
		CategoryLevel1Name *string   `json:"category_level1_name"`
		CategoryLevel2ID   *uint64   `json:"category_level2_id"`
		CategoryLevel2Name *string   `json:"category_level2_name"`
	}
	items := make([]item, 0, size)
	if err := query.Select(
		"p.id, p.title, p.status, p.price_cent, p.original_price_cent, p.stock, p.reserved_stock, (p.stock - p.reserved_stock) AS available_stock, p.updated_at, c1.id AS category_level1_id, c1.name AS category_level1_name, c2.id AS category_level2_id, c2.name AS category_level2_name",
	).Order("p.updated_at DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, common.PageResult[item]{Items: items, Total: total, Page: page, PageSize: size})
}

func (s *Server) checkProductReadyForOnShelf(tx *gorm.DB, productID uint64) error {
	var count int64
	if err := tx.Model(&model.ProductImage{}).Where("product_id = ?", productID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return common.ErrInvalidArgument
	}
	return nil
}

func canDeleteProductByStatus(status string) bool {
	return status == model.ProductDraft || status == model.ProductOffShelf || status == model.ProductClosed
}

func (s *Server) cleanupLocalFilesByObjectKeys(keys []string) {
	if !strings.EqualFold(strings.TrimSpace(s.cfg.FileStorageProvider), "local") {
		return
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" {
			continue
		}
		if _, ok := seen[cleanKey]; ok {
			continue
		}
		seen[cleanKey] = struct{}{}
		path, err := s.localUploadPath(cleanKey)
		if err != nil {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			continue
		}
		_ = os.Remove(path + ".tmp")
	}
}

func (s *Server) handleDeleteProduct(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}

	deletedFileIDs := make([]uint64, 0)
	objectKeys := make([]string, 0)
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		product, err := s.loadOwnedProduct(tx, id, actor.MerchantID)
		if err != nil {
			return err
		}
		if !canDeleteProductByStatus(product.Status) || product.ReservedStock != 0 {
			return common.ErrInvalidTransition
		}
		var activeOrderCount int64
		if err := tx.Model(&model.Order{}).Where("product_id = ? AND is_active = ?", product.ID, true).Count(&activeOrderCount).Error; err != nil {
			return err
		}
		if activeOrderCount > 0 {
			return common.ErrInvalidTransition
		}

		var orderCount int64
		if err := tx.Model(&model.Order{}).Where("product_id = ?", product.ID).Count(&orderCount).Error; err != nil {
			return err
		}
		if orderCount > 0 {
			return common.ErrInvalidTransition
		}
		var intentCount int64
		if err := tx.Model(&model.BuyerIntent{}).Where("product_id = ?", product.ID).Count(&intentCount).Error; err != nil {
			return err
		}
		if intentCount > 0 {
			return common.ErrInvalidTransition
		}

		var productImages []model.ProductImage
		if err := tx.Where("product_id = ?", product.ID).Find(&productImages).Error; err != nil {
			return err
		}
		fileIDSet := map[uint64]struct{}{}
		for _, img := range productImages {
			if img.FileID > 0 {
				fileIDSet[img.FileID] = struct{}{}
			}
		}
		if product.CoverFileID != nil && *product.CoverFileID > 0 {
			fileIDSet[*product.CoverFileID] = struct{}{}
		}
		fileIDs := make([]uint64, 0, len(fileIDSet))
		for fileID := range fileIDSet {
			fileIDs = append(fileIDs, fileID)
		}

		deletableFileIDs := make([]uint64, 0, len(fileIDs))
		if len(fileIDs) > 0 {
			type refRow struct {
				FileID   uint64
				RefCount int64
			}
			refs := make([]refRow, 0)
			if err := tx.Model(&model.ProductImage{}).
				Select("file_id, COUNT(*) AS ref_count").
				Where("file_id IN ? AND product_id <> ?", fileIDs, product.ID).
				Group("file_id").
				Scan(&refs).Error; err != nil {
				return err
			}
			referenced := make(map[uint64]struct{}, len(refs))
			for _, row := range refs {
				if row.RefCount > 0 {
					referenced[row.FileID] = struct{}{}
				}
			}
			for _, fileID := range fileIDs {
				if _, inUse := referenced[fileID]; !inUse {
					deletableFileIDs = append(deletableFileIDs, fileID)
				}
			}
		}

		if len(deletableFileIDs) > 0 {
			files := make([]model.FileRecord, 0, len(deletableFileIDs))
			if err := tx.Where(
				"id IN ? AND biz_type = ? AND uploader_type = ? AND uploader_id = ?",
				deletableFileIDs,
				model.FileBizProductImage,
				model.UserTypeMerchant,
				actor.UserID,
			).Find(&files).Error; err != nil {
				return err
			}
			if len(files) > 0 {
				ids := make([]uint64, 0, len(files))
				for _, file := range files {
					ids = append(ids, file.ID)
					objectKey := strings.TrimSpace(file.ObjectKey)
					if objectKey != "" {
						objectKeys = append(objectKeys, objectKey)
					}
				}
				if err := tx.Where("id IN ?", ids).Delete(&model.FileRecord{}).Error; err != nil {
					return err
				}
				deletedFileIDs = append(deletedFileIDs, ids...)
			}
		}

		if err := tx.Where("product_id = ?", product.ID).Delete(&model.ProductImage{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&product).Error; err != nil {
			return err
		}

		s.writeOperationLog(c, tx, "product", product.ID, "product_delete", &product.Status, nil, common.CodeOK, &actor.MerchantID, gin.H{
			"deleted_file_count": len(deletedFileIDs),
		})
		return nil
	}); err != nil {
		common.Fail(c, err)
		return
	}

	s.cleanupLocalFilesByObjectKeys(objectKeys)
	common.Success(c, gin.H{
		"product_id":         id,
		"deleted":            true,
		"deleted_file_ids":   deletedFileIDs,
		"deleted_file_count": len(deletedFileIDs),
	})
}

func (s *Server) doProductStatusChange(c *gin.Context, id uint64, toStatus, action string) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	payload := gin.H{"id": id, "to_status": toStatus}
	data, err := s.runWithIdempotency(c, payload, func() (map[string]interface{}, error) {
		resp := map[string]interface{}{}
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			product, err := s.loadOwnedProduct(tx, id, actor.MerchantID)
			if err != nil {
				return err
			}
			fromStatus := product.Status
			if product.Status == toStatus {
				resp["product_id"] = product.ID
				resp["from_status"] = product.Status
				resp["to_status"] = product.Status
				resp["changed_at"] = time.Now().Format(time.RFC3339)
				resp["idempotent"] = true
				return nil
			}
			if !stateflow.CanTransitionProduct(product.Status, toStatus) {
				return common.ErrInvalidTransition
			}
			if toStatus == model.ProductOnShelf {
				if err := s.checkProductReadyForOnShelf(tx, product.ID); err != nil {
					return err
				}
				now := time.Now()
				product.ShelfAt = &now
			}
			if toStatus == model.ProductOffShelf {
				now := time.Now()
				product.OffShelfAt = &now
			}
			if toStatus == model.ProductClosed {
				if product.ReservedStock != 0 {
					return common.ErrInvalidTransition
				}
				var activeOrderCount int64
				if err := tx.Model(&model.Order{}).Where("product_id = ? AND is_active = ?", product.ID, true).Count(&activeOrderCount).Error; err != nil {
					return err
				}
				if activeOrderCount > 0 {
					return common.ErrInvalidTransition
				}
				now := time.Now()
				product.ClosedAt = &now
			}
			product.Status = toStatus
			product.UpdatedBy = actor.UserID
			product.Version++
			if err := tx.Save(&product).Error; err != nil {
				return err
			}
			from, to := fromStatus, toStatus
			s.writeOperationLog(c, tx, "product", product.ID, action, &from, &to, common.CodeOK, &actor.MerchantID, nil)
			resp["product_id"] = product.ID
			resp["from_status"] = fromStatus
			resp["to_status"] = toStatus
			resp["changed_at"] = time.Now().Format(time.RFC3339)
			resp["idempotent"] = false
			return nil
		})
		if err != nil {
			return nil, err
		}
		return resp, nil
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, data)
}

func (s *Server) handleProductOnShelf(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	s.doProductStatusChange(c, id, model.ProductOnShelf, "product_on_shelf")
}

func (s *Server) handleProductOffShelf(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	s.doProductStatusChange(c, id, model.ProductOffShelf, "product_off_shelf")
}

func (s *Server) handleProductClose(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	s.doProductStatusChange(c, id, model.ProductClosed, "product_close")
}
