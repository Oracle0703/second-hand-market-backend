package app

import (
	"math"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
	"second-hand-market-backend/backend/internal/stateflow"
)

func orderTotalDealPriceCent(quantity, dealPriceCent int) (int64, error) {
	if quantity <= 0 || dealPriceCent <= 0 || int64(dealPriceCent) > math.MaxInt64/int64(quantity) {
		return 0, common.ErrInvalidArgument
	}
	return int64(quantity) * int64(dealPriceCent), nil
}

func (s *Server) handleCreateOrder(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.CreateOrderRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	totalDealPriceCent, err := orderTotalDealPriceCent(req.Quantity, req.DealPriceCent)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var order model.Order
	var product model.Product
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		ownedProduct, err := s.loadOwnedProduct(tx, req.ProductID, actor.MerchantID)
		if err != nil {
			return err
		}
		if ownedProduct.Status != model.ProductOnShelf {
			return common.ErrInvalidTransition
		}
		reserveResult := tx.Model(&model.Product{}).
			Where("id = ? AND merchant_id = ? AND status = ? AND stock - reserved_stock >= ?", ownedProduct.ID, actor.MerchantID, model.ProductOnShelf, req.Quantity).
			Updates(map[string]interface{}{
				"reserved_stock": gorm.Expr("reserved_stock + ?", req.Quantity),
				"updated_by":     actor.UserID,
				"version":        gorm.Expr("version + 1"),
			})
		if reserveResult.Error != nil {
			return reserveResult.Error
		}
		if reserveResult.RowsAffected != 1 {
			return common.NewBizError(common.CodeConflict, "insufficient available stock", 409)
		}
		order = model.Order{
			OrderNo:            common.BuildBizNo("O"),
			MerchantID:         actor.MerchantID,
			ProductID:          ownedProduct.ID,
			Quantity:           req.Quantity,
			DealPriceCent:      req.DealPriceCent,
			BuyerContactMasked: req.BuyerContactMasked,
			Remark:             req.Remark,
			Status:             model.OrderCreated,
			IsActive:           true,
			CreatedBy:          actor.UserID,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", ownedProduct.ID).First(&product).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.OrderEvent{OrderID: order.ID, EventType: "CREATE", ToStatus: model.OrderCreated, OperatorType: model.UserTypeMerchant, OperatorID: actor.UserID, Note: req.Remark}).Error; err != nil {
			return err
		}
		s.writeOperationLog(c, tx, "order", order.ID, "order_create", nil, &order.Status, common.CodeOK, &actor.MerchantID, nil)
		s.writeOperationLog(c, tx, "product", product.ID, "product_reserve_stock", nil, nil, common.CodeOK, &actor.MerchantID, gin.H{"order_id": order.ID, "quantity": order.Quantity})
		return nil
	})
	if err != nil {
		if bizErr, ok := err.(*common.BizError); ok && bizErr.Code == common.CodeConflict {
			common.Fail(c, bizErr)
			return
		}
		common.Fail(c, err)
		return
	}
	common.Success(c, gin.H{
		"order_id":              order.ID,
		"order_no":              order.OrderNo,
		"status":                order.Status,
		"quantity":              order.Quantity,
		"deal_price_cent":       order.DealPriceCent,
		"total_deal_price_cent": totalDealPriceCent,
		"product_status":        product.Status,
		"stock":                 product.Stock,
		"reserved_stock":        product.ReservedStock,
		"available_stock":       product.Stock - product.ReservedStock,
	})
}

func (s *Server) handleOrderList(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Table("orders AS o").
		Joins("LEFT JOIN products AS p ON p.id = o.product_id").
		Joins("LEFT JOIN categories AS c2 ON c2.id = p.category_id").
		Joins("LEFT JOIN categories AS c1 ON c1.id = c2.parent_id").
		Where("o.merchant_id = ?", actor.MerchantID)
	if v := strings.TrimSpace(c.Query("status")); v != "" {
		query = query.Where("o.status = ?", v)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		query = query.Where("o.order_no LIKE ?", "%"+kw+"%")
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
		OrderNo            string    `json:"order_no"`
		ProductID          uint64    `json:"product_id"`
		ProductTitle       string    `json:"product_title"`
		Status             string    `json:"status"`
		Quantity           int       `json:"quantity"`
		DealPriceCent      int       `json:"deal_price_cent"`
		TotalDealPriceCent int64     `json:"total_deal_price_cent" gorm:"-"`
		CreatedAt          time.Time `json:"created_at"`
		CategoryLevel1ID   *uint64   `json:"category_level1_id"`
		CategoryLevel1Name *string   `json:"category_level1_name"`
		CategoryLevel2ID   *uint64   `json:"category_level2_id"`
		CategoryLevel2Name *string   `json:"category_level2_name"`
	}
	items := make([]item, 0, size)
	if err := query.Select(
		"o.id, o.order_no, o.product_id, p.title AS product_title, o.status, o.quantity, o.deal_price_cent, o.created_at, c1.id AS category_level1_id, c1.name AS category_level1_name, c2.id AS category_level2_id, c2.name AS category_level2_name",
	).Order("o.id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	for i := range items {
		items[i].TotalDealPriceCent = int64(items[i].Quantity) * int64(items[i].DealPriceCent)
	}
	common.Success(c, common.PageResult[item]{Items: items, Total: total, Page: page, PageSize: size})
}

func (s *Server) handleOrderDetail(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	order, err := s.loadOwnedOrder(nil, id, actor.MerchantID)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var product model.Product
	_ = s.DB.Where("id = ?", order.ProductID).First(&product).Error
	var events []model.OrderEvent
	_ = s.DB.Where("order_id = ?", order.ID).Order("id ASC").Find(&events).Error
	totalDealPriceCent := int64(order.Quantity) * int64(order.DealPriceCent)
	common.Success(c, gin.H{"order_detail": gin.H{
		"id":                    order.ID,
		"order_no":              order.OrderNo,
		"status":                order.Status,
		"quantity":              order.Quantity,
		"deal_price_cent":       order.DealPriceCent,
		"total_deal_price_cent": totalDealPriceCent,
		"product": gin.H{
			"id":              product.ID,
			"title":           product.Title,
			"status":          product.Status,
			"stock":           product.Stock,
			"reserved_stock":  product.ReservedStock,
			"available_stock": product.Stock - product.ReservedStock,
		},
	}, "events": events})
}

func (s *Server) doOrderAction(c *gin.Context, id uint64, toStatus, action string, note *string) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	payload := gin.H{"id": id, "to_status": toStatus, "note": note}
	data, err := s.runWithIdempotency(c, payload, func() (map[string]interface{}, error) {
		resp := map[string]interface{}{}
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			order, err := s.loadOwnedOrder(tx, id, actor.MerchantID)
			if err != nil {
				return err
			}
			fromOrder := order.Status
			if order.Status == toStatus {
				product, err := s.loadOwnedProduct(tx, order.ProductID, actor.MerchantID)
				if err != nil {
					return err
				}
				resp["order_id"] = order.ID
				resp["from_status"] = order.Status
				resp["to_status"] = order.Status
				resp["idempotent"] = true
				resp["quantity"] = order.Quantity
				resp["deal_price_cent"] = order.DealPriceCent
				resp["total_deal_price_cent"] = int64(order.Quantity) * int64(order.DealPriceCent)
				resp["product_status"] = product.Status
				resp["stock"] = product.Stock
				resp["reserved_stock"] = product.ReservedStock
				resp["available_stock"] = product.Stock - product.ReservedStock
				return nil
			}
			if !stateflow.CanTransitionOrder(order.Status, toStatus) {
				return common.ErrInvalidTransition
			}
			if order.Quantity <= 0 {
				return common.ErrInternal
			}
			product, err := s.loadOwnedProduct(tx, order.ProductID, actor.MerchantID)
			if err != nil {
				return err
			}
			if product.Status != model.ProductOnShelf && product.Status != model.ProductOffShelf {
				return common.ErrInvalidTransition
			}

			now := time.Now()
			var inventoryResult *gorm.DB
			if toStatus == model.OrderCompleted {
				inventoryResult = tx.Model(&model.Product{}).
					Where("id = ? AND merchant_id = ? AND stock >= ? AND reserved_stock >= ?", product.ID, actor.MerchantID, order.Quantity, order.Quantity).
					Updates(map[string]interface{}{
						"stock":          gorm.Expr("stock - ?", order.Quantity),
						"reserved_stock": gorm.Expr("reserved_stock - ?", order.Quantity),
						"updated_by":     actor.UserID,
						"version":        gorm.Expr("version + 1"),
					})
				resp["completed_at"] = now.Format(time.RFC3339)
			} else {
				inventoryResult = tx.Model(&model.Product{}).
					Where("id = ? AND merchant_id = ? AND reserved_stock >= ?", product.ID, actor.MerchantID, order.Quantity).
					Updates(map[string]interface{}{
						"reserved_stock": gorm.Expr("reserved_stock - ?", order.Quantity),
						"updated_by":     actor.UserID,
						"version":        gorm.Expr("version + 1"),
					})
				resp["closed_at"] = now.Format(time.RFC3339)
			}
			if inventoryResult.Error != nil {
				return inventoryResult.Error
			}
			if inventoryResult.RowsAffected != 1 {
				return common.NewBizError(common.CodeConflict, "inventory state changed", 409)
			}

			orderUpdates := map[string]interface{}{
				"status":    toStatus,
				"is_active": false,
			}
			if toStatus == model.OrderCompleted {
				orderUpdates["completed_at"] = &now
			} else {
				orderUpdates["closed_at"] = &now
				orderUpdates["close_reason"] = note
			}
			orderResult := tx.Model(&model.Order{}).
				Where("id = ? AND merchant_id = ? AND status = ? AND is_active = ?", order.ID, actor.MerchantID, model.OrderCreated, true).
				Updates(orderUpdates)
			if orderResult.Error != nil {
				return orderResult.Error
			}
			if orderResult.RowsAffected != 1 {
				return common.NewBizError(common.CodeConflict, "order state changed", 409)
			}

			if err := tx.Where("id = ?", product.ID).First(&product).Error; err != nil {
				return err
			}
			if toStatus == model.OrderCompleted && product.Stock == 0 {
				statusResult := tx.Model(&model.Product{}).
					Where("id = ? AND stock = 0 AND status IN ?", product.ID, []string{model.ProductOnShelf, model.ProductOffShelf}).
					Updates(map[string]interface{}{
						"status":     model.ProductSold,
						"sold_at":    &now,
						"updated_by": actor.UserID,
						"version":    gorm.Expr("version + 1"),
					})
				if statusResult.Error != nil {
					return statusResult.Error
				}
				if statusResult.RowsAffected != 1 {
					return common.ErrInvalidTransition
				}
				product.Status = model.ProductSold
				product.SoldAt = &now
			}
			eventType := "COMPLETE"
			if toStatus == model.OrderClosed {
				eventType = "CLOSE"
			}
			if err := tx.Create(&model.OrderEvent{OrderID: order.ID, EventType: eventType, FromStatus: &fromOrder, ToStatus: toStatus, OperatorType: model.UserTypeMerchant, OperatorID: actor.UserID, Note: note}).Error; err != nil {
				return err
			}
			s.writeOperationLog(c, tx, "order", order.ID, action, &fromOrder, &toStatus, common.CodeOK, &actor.MerchantID, nil)
			s.writeOperationLog(c, tx, "product", product.ID, "product_order_inventory", nil, &product.Status, common.CodeOK, &actor.MerchantID, gin.H{"order_id": order.ID, "quantity": order.Quantity, "order_status": toStatus})

			resp["order_id"] = order.ID
			resp["from_status"] = fromOrder
			resp["to_status"] = toStatus
			resp["product_status"] = product.Status
			resp["quantity"] = order.Quantity
			resp["deal_price_cent"] = order.DealPriceCent
			resp["total_deal_price_cent"] = int64(order.Quantity) * int64(order.DealPriceCent)
			resp["stock"] = product.Stock
			resp["reserved_stock"] = product.ReservedStock
			resp["available_stock"] = product.Stock - product.ReservedStock
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

func (s *Server) handleOrderComplete(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.OrderActionRequest
	if c.Request.ContentLength > 0 {
		if err := bindJSON(c, &req); err != nil {
			common.Fail(c, err)
			return
		}
	}
	s.doOrderAction(c, id, model.OrderCompleted, "order_complete", req.Note)
}

func (s *Server) handleOrderClose(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.OrderActionRequest
	if c.Request.ContentLength > 0 {
		if err := bindJSON(c, &req); err != nil {
			common.Fail(c, err)
			return
		}
	}
	s.doOrderAction(c, id, model.OrderClosed, "order_close", req.Reason)
}
