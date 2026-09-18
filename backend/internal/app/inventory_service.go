package app

import (
	"gorm.io/gorm"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
	"second-hand-market-backend/backend/internal/stateflow"
	"strings"
	"time"
)

// inventoryService centralizes inventory/order writes without depending on Gin.
// Adjustment and transition calls receive the caller's idempotency transaction;
// nested transactions are savepoints and cannot commit independently.
// Business rules remain one unit per order and one active order per product.
type inventoryService struct{}
type inventoryAudit func(tx *gorm.DB, resourceType string, resourceID uint64, action string, fromStatus, toStatus *string, code int, merchantID *uint64, detail map[string]interface{}) error

func canAdjustProductStock(status string) bool {
	return status == model.ProductDraft || status == model.ProductOnShelf || status == model.ProductOffShelf || status == model.ProductSold
}

func calculateStockAdjustment(product model.Product, adjustmentType string, quantity int, allRemaining bool) (int, int, string, error) {
	if !canAdjustProductStock(product.Status) || product.ActiveOrderID != nil {
		return 0, 0, "", common.ErrInvalidTransition
	}
	if product.Status == model.ProductSold {
		if adjustmentType != model.StockAdjustmentIncrease || quantity <= 0 || allRemaining {
			return 0, 0, "", common.ErrInvalidTransition
		}
		return quantity, product.Stock + quantity, model.ProductOffShelf, nil
	}
	if allRemaining && adjustmentType != model.StockAdjustmentMarkSold {
		return 0, 0, "", common.ErrInvalidArgument
	}
	if quantity <= 0 && !allRemaining {
		return 0, 0, "", common.ErrInvalidArgument
	}

	stockAfter := product.Stock
	statusAfter := product.Status
	switch adjustmentType {
	case model.StockAdjustmentIncrease:
		stockAfter = product.Stock + quantity
	case model.StockAdjustmentDecrease:
		if quantity > product.Stock {
			return 0, 0, "", common.ErrInvalidTransition
		}
		stockAfter = product.Stock - quantity
		if product.Status == model.ProductOnShelf && stockAfter == 0 {
			statusAfter = model.ProductOffShelf
		}
	case model.StockAdjustmentMarkSold:
		if (product.Status != model.ProductOnShelf && product.Status != model.ProductOffShelf) || product.Stock <= 0 || product.ReservedStock != 0 {
			return 0, 0, "", common.ErrInvalidTransition
		}
		if allRemaining {
			quantity = product.Stock
		}
		if quantity > product.Stock {
			return 0, 0, "", common.ErrInvalidTransition
		}
		stockAfter = product.Stock - quantity
		if stockAfter == 0 {
			statusAfter = model.ProductSold
		}
	default:
		return 0, 0, "", common.ErrInvalidArgument
	}
	return quantity, stockAfter, statusAfter, nil
}

func isUniqueActiveOrderErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "uk_product_active") || strings.Contains(msg, "unique")
}

func (inventoryService) adjustStock(tx *gorm.DB, actor common.Actor, audit inventoryAudit, id uint64, req dto.AdjustProductStockRequest) (map[string]interface{}, error) {
	resp := map[string]interface{}{}
	err := tx.Transaction(func(tx *gorm.DB) error {
		product, err := loadOwnedProductForUpdate(tx, id, actor.MerchantID)
		if err != nil {
			return err
		}
		stockBefore := product.Stock
		statusBefore := product.Status
		appliedQuantity, stockAfter, statusAfter, err := calculateStockAdjustment(product, req.AdjustmentType, req.Quantity, req.AllRemaining)
		if err != nil {
			return err
		}

		now := time.Now()
		product.Stock = stockAfter
		product.Status = statusAfter
		product.UpdatedBy = actor.UserID
		product.Version++
		if statusAfter == model.ProductOffShelf && statusBefore != model.ProductOffShelf {
			product.OffShelfAt = &now
		}
		if statusAfter == model.ProductSold && statusBefore != model.ProductSold {
			product.SoldAt = &now
			product.ActiveOrderID = nil
		}
		if err := tx.Save(&product).Error; err != nil {
			return err
		}

		movement := model.ProductStockAdjustment{
			ProductID:      product.ID,
			MerchantID:     actor.MerchantID,
			AdjustmentType: req.AdjustmentType,
			Quantity:       appliedQuantity,
			StockBefore:    stockBefore,
			StockAfter:     stockAfter,
			StatusBefore:   statusBefore,
			StatusAfter:    statusAfter,
			Reason:         req.Reason,
			OperatorID:     actor.UserID,
			CreatedAt:      now,
		}
		if err := tx.Create(&movement).Error; err != nil {
			return err
		}

		from, to := statusBefore, statusAfter
		if err := audit(tx, "product", product.ID, "product_stock_adjust", &from, &to, common.CodeOK, &actor.MerchantID, map[string]interface{}{
			"adjustment_type": req.AdjustmentType,
			"quantity":        appliedQuantity,
			"stock_before":    stockBefore,
			"stock_after":     stockAfter,
			"reason":          req.Reason,
			"movement_id":     movement.ID,
		}); err != nil {
			return err
		}

		resp["product_id"] = product.ID
		resp["movement_id"] = movement.ID
		resp["adjustment_type"] = req.AdjustmentType
		resp["quantity"] = appliedQuantity
		resp["stock_before"] = stockBefore
		resp["stock_after"] = stockAfter
		resp["status_before"] = statusBefore
		resp["status_after"] = statusAfter
		resp["adjusted_at"] = now.Format(time.RFC3339)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (inventoryService) transitionOrder(tx *gorm.DB, actor common.Actor, audit inventoryAudit, id uint64, toStatus, action string, note *string) (map[string]interface{}, error) {
	resp := map[string]interface{}{}
	err := tx.Transaction(func(tx *gorm.DB) error {
		order, err := loadOwnedOrderForUpdate(tx, id, actor.MerchantID)
		if err != nil {
			return err
		}
		fromOrder := order.Status
		if order.Status == toStatus {
			product, err := loadOwnedProductForUpdate(tx, order.ProductID, actor.MerchantID)
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
		product, err := loadOwnedProductForUpdate(tx, order.ProductID, actor.MerchantID)
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
		if err := audit(tx, "order", order.ID, action, &fromOrder, &toStatus, common.CodeOK, &actor.MerchantID, nil); err != nil {
			return err
		}
		if err := audit(tx, "product", product.ID, "product_order_link", &fromProduct, &product.Status, common.CodeOK, &actor.MerchantID, map[string]interface{}{"order_id": order.ID}); err != nil {
			return err
		}

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
}

func (inventoryService) createOrder(tx *gorm.DB, actor common.Actor, audit inventoryAudit, req dto.CreateOrderRequest) (model.Order, error) {
	var order model.Order
	err := tx.Transaction(func(tx *gorm.DB) error {
		product, err := loadOwnedProductForUpdate(tx, req.ProductID, actor.MerchantID)
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
		if err := audit(tx, "order", order.ID, "order_create", nil, &order.Status, common.CodeOK, &actor.MerchantID, nil); err != nil {
			return err
		}
		if err := audit(tx, "product", product.ID, "product_lock", &from, &to, common.CodeOK, &actor.MerchantID, map[string]interface{}{"order_id": order.ID}); err != nil {
			return err
		}
		return nil
	})
	return order, err
}
