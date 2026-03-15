package app

import (
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
	if req.Stock != nil && *req.Stock != 1 {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	if err := s.ensureLevel2Category(req.CategoryID); err != nil {
		common.Fail(c, err)
		return
	}
	product := model.Product{
		ProductNo:      common.BuildBizNo("P"),
		MerchantID:     actor.MerchantID,
		Title:          req.Title,
		Description:    req.Description,
		CategoryID:     req.CategoryID,
		PriceCent:      req.PriceCent,
		ConditionLevel: req.ConditionLevel,
		Stock:          1,
		Status:         model.ProductDraft,
		CreatedBy:      actor.UserID,
		UpdatedBy:      actor.UserID,
		Version:        1,
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
	common.Success(c, gin.H{"product_id": product.ID, "product_no": product.ProductNo, "status": product.Status, "stock": product.Stock, "created_at": product.CreatedAt})
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
		if req.ConditionLevel != nil {
			if !allowed["condition_level"] {
				return common.ErrInvalidTransition
			}
			product.ConditionLevel = *req.ConditionLevel
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
	common.Success(c, gin.H{"product": gin.H{
		"id":              product.ID,
		"title":           product.Title,
		"description":     product.Description,
		"status":          product.Status,
		"category_id":     product.CategoryID,
		"price_cent":      product.PriceCent,
		"condition_level": product.ConditionLevel,
		"stock":           product.Stock,
		"images":          imgIDs,
		"active_order_id": product.ActiveOrderID,
	}})
}

func (s *Server) handleProductList(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Model(&model.Product{}).Where("merchant_id = ?", actor.MerchantID)
	if v := c.Query("status"); v != "" {
		query = query.Where("status = ?", v)
	}
	if kw := c.Query("keyword"); kw != "" {
		query = query.Where("title LIKE ?", "%"+kw+"%")
	}
	if st := c.Query("start_at"); st != "" {
		query = query.Where("created_at >= ?", st)
	}
	if et := c.Query("end_at"); et != "" {
		query = query.Where("created_at <= ?", et)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	type item struct {
		ID        uint64    `json:"id"`
		Title     string    `json:"title"`
		Status    string    `json:"status"`
		PriceCent int       `json:"price_cent"`
		Stock     int       `json:"stock"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	var rows []model.Product
	if err := query.Order("updated_at DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	items := make([]item, 0, len(rows))
	for _, it := range rows {
		items = append(items, item{ID: it.ID, Title: it.Title, Status: it.Status, PriceCent: it.PriceCent, Stock: it.Stock, UpdatedAt: it.UpdatedAt})
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
