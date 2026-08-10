package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
	"second-hand-market-backend/backend/internal/stateflow"
)

func isUniqueActiveOrderErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "uk_product_active") || strings.Contains(msg, "unique")
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
	var order model.Order
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		product, err := s.loadOwnedProductForUpdate(tx, req.ProductID, actor.MerchantID)
		if err != nil {
			return err
		}
		if product.Status == model.ProductLocked && product.ActiveOrderID != nil {
			return common.NewBizError(common.CodeConflict, "product has active order", 409)
		}
		if !stateflow.CanTransitionProduct(product.Status, model.ProductLocked) {
			return common.ErrInvalidTransition
		}
		if product.Stock-product.ReservedStock < 1 {
			return common.ErrInvalidTransition
		}
		order = model.Order{
			OrderNo:            common.BuildBizNo("O"),
			MerchantID:         actor.MerchantID,
			ProductID:          product.ID,
			Quantity:           1,
			DealPriceCent:      req.DealPriceCent,
			BuyerContactMasked: req.BuyerContactMasked,
			Remark:             req.Remark,
			Status:             model.OrderCreated,
			IsActive:           true,
			CreatedBy:          actor.UserID,
		}
		if err := tx.Create(&order).Error; err != nil {
			if isUniqueActiveOrderErr(err) {
				return common.ErrConflict
			}
			return err
		}
		now := time.Now()
		fromStatus := product.Status
		product.Status = model.ProductLocked
		product.ReservedStock += order.Quantity
		product.ActiveOrderID = &order.ID
		product.LockedAt = &now
		product.UpdatedBy = actor.UserID
		product.Version++
		if err := tx.Save(&product).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.OrderEvent{OrderID: order.ID, EventType: "CREATE", ToStatus: model.OrderCreated, OperatorType: model.UserTypeMerchant, OperatorID: actor.UserID, Note: req.Remark}).Error; err != nil {
			return err
		}
		from, to := fromStatus, model.ProductLocked
		s.writeOperationLog(c, tx, "order", order.ID, "order_create", nil, &order.Status, common.CodeOK, &actor.MerchantID, nil)
		s.writeOperationLog(c, tx, "product", product.ID, "product_lock", &from, &to, common.CodeOK, &actor.MerchantID, gin.H{"order_id": order.ID})
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
	common.Success(c, gin.H{"order_id": order.ID, "order_no": order.OrderNo, "status": order.Status, "product_status": model.ProductLocked})
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
		Status             string    `json:"status"`
		DealPriceCent      int       `json:"deal_price_cent"`
		CreatedAt          time.Time `json:"created_at"`
		CategoryLevel1ID   *uint64   `json:"category_level1_id"`
		CategoryLevel1Name *string   `json:"category_level1_name"`
		CategoryLevel2ID   *uint64   `json:"category_level2_id"`
		CategoryLevel2Name *string   `json:"category_level2_name"`
	}
	items := make([]item, 0, size)
	if err := query.Select(
		"o.id, o.order_no, o.product_id, o.status, o.deal_price_cent, o.created_at, c1.id AS category_level1_id, c1.name AS category_level1_name, c2.id AS category_level2_id, c2.name AS category_level2_name",
	).Order("o.id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
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
	common.Success(c, gin.H{"order_detail": gin.H{"id": order.ID, "order_no": order.OrderNo, "status": order.Status, "deal_price_cent": order.DealPriceCent, "product": gin.H{"id": product.ID, "title": product.Title, "status": product.Status}}, "events": events})
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
			order, err := s.loadOwnedOrderForUpdate(tx, id, actor.MerchantID)
			if err != nil {
				return err
			}
			fromOrder := order.Status
			if order.Status == toStatus {
				product, err := s.loadOwnedProductForUpdate(tx, order.ProductID, actor.MerchantID)
				if err != nil {
					return err
				}
				resp["order_id"] = order.ID
				resp["from_status"] = order.Status
				resp["to_status"] = order.Status
				resp["idempotent"] = true
				resp["product_status"] = product.Status
				return nil
			}
			if !stateflow.CanTransitionOrder(order.Status, toStatus) {
				return common.ErrInvalidTransition
			}
			product, err := s.loadOwnedProductForUpdate(tx, order.ProductID, actor.MerchantID)
			if err != nil {
				return err
			}
			if product.Status != model.ProductLocked || product.ActiveOrderID == nil || *product.ActiveOrderID != order.ID {
				return common.ErrInvalidTransition
			}
			if order.Quantity <= 0 || product.ReservedStock < order.Quantity || product.Stock < order.Quantity {
				return common.ErrInvalidTransition
			}

			order.Status = toStatus
			order.IsActive = false
			now := time.Now()
			if toStatus == model.OrderCompleted {
				order.CompletedAt = &now
				product.Stock -= order.Quantity
				product.ReservedStock -= order.Quantity
				if product.Stock == 0 {
					product.Status = model.ProductSold
					product.SoldAt = &now
				} else {
					product.Status = model.ProductOnShelf
				}
				resp["completed_at"] = now.Format(time.RFC3339)
			} else {
				order.ClosedAt = &now
				order.CloseReason = note
				product.ReservedStock -= order.Quantity
				product.Status = model.ProductOffShelf
				product.OffShelfAt = &now
				resp["closed_at"] = now.Format(time.RFC3339)
			}
			product.ActiveOrderID = nil
			product.UpdatedBy = actor.UserID
			product.Version++
			if err := tx.Save(&order).Error; err != nil {
				return err
			}
			if err := tx.Save(&product).Error; err != nil {
				return err
			}
			eventType := "COMPLETE"
			if toStatus == model.OrderClosed {
				eventType = "CLOSE"
			}
			if err := tx.Create(&model.OrderEvent{OrderID: order.ID, EventType: eventType, FromStatus: &fromOrder, ToStatus: toStatus, OperatorType: model.UserTypeMerchant, OperatorID: actor.UserID, Note: note}).Error; err != nil {
				return err
			}
			fromProduct := model.ProductLocked
			s.writeOperationLog(c, tx, "order", order.ID, action, &fromOrder, &toStatus, common.CodeOK, &actor.MerchantID, nil)
			s.writeOperationLog(c, tx, "product", product.ID, "product_order_link", &fromProduct, &product.Status, common.CodeOK, &actor.MerchantID, gin.H{"order_id": order.ID})

			resp["order_id"] = order.ID
			resp["from_status"] = fromOrder
			resp["to_status"] = toStatus
			resp["product_status"] = product.Status
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
