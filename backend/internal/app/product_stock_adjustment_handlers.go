package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
)

func canAdjustProductStock(status string) bool {
	return status == model.ProductDraft || status == model.ProductOnShelf || status == model.ProductOffShelf
}

func calculateStockAdjustment(product model.Product, adjustmentType string, quantity int) (int, string, error) {
	if !canAdjustProductStock(product.Status) || product.ActiveOrderID != nil {
		return 0, "", common.ErrInvalidTransition
	}
	if quantity <= 0 {
		return 0, "", common.ErrInvalidArgument
	}

	stockAfter := product.Stock
	statusAfter := product.Status
	switch adjustmentType {
	case model.StockAdjustmentIncrease:
		stockAfter = product.Stock + quantity
	case model.StockAdjustmentDecrease:
		if quantity > product.Stock {
			return 0, "", common.ErrInvalidTransition
		}
		stockAfter = product.Stock - quantity
		if product.Status == model.ProductOnShelf && stockAfter == 0 {
			statusAfter = model.ProductOffShelf
		}
	case model.StockAdjustmentMarkSold:
		if quantity > product.Stock {
			return 0, "", common.ErrInvalidTransition
		}
		stockAfter = product.Stock - quantity
		if stockAfter == 0 {
			statusAfter = model.ProductSold
		}
	default:
		return 0, "", common.ErrInvalidArgument
	}
	return stockAfter, statusAfter, nil
}

func (s *Server) loadOwnedProductForUpdate(tx *gorm.DB, productID uint64, merchantID uint64) (model.Product, error) {
	var product model.Product
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", productID).
		First(&product).Error
	if err != nil {
		return model.Product{}, s.dbError(err)
	}
	if product.MerchantID != merchantID {
		return model.Product{}, common.ErrForbidden
	}
	return product, nil
}

func (s *Server) handleProductStockAdjustment(c *gin.Context) {
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
	var req dto.AdjustProductStockRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) < 2 || len(req.Reason) > 255 {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}

	payload := gin.H{"id": id, "adjustment_type": req.AdjustmentType, "quantity": req.Quantity, "reason": req.Reason}
	data, err := s.runWithIdempotency(c, payload, func() (map[string]interface{}, error) {
		resp := map[string]interface{}{}
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			product, err := s.loadOwnedProductForUpdate(tx, id, actor.MerchantID)
			if err != nil {
				return err
			}
			stockBefore := product.Stock
			statusBefore := product.Status
			stockAfter, statusAfter, err := calculateStockAdjustment(product, req.AdjustmentType, req.Quantity)
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
				Quantity:       req.Quantity,
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
			s.writeOperationLog(c, tx, "product", product.ID, "product_stock_adjust", &from, &to, common.CodeOK, &actor.MerchantID, gin.H{
				"adjustment_type": req.AdjustmentType,
				"quantity":        req.Quantity,
				"stock_before":    stockBefore,
				"stock_after":     stockAfter,
				"reason":          req.Reason,
				"movement_id":     movement.ID,
			})

			resp["product_id"] = product.ID
			resp["movement_id"] = movement.ID
			resp["adjustment_type"] = req.AdjustmentType
			resp["quantity"] = req.Quantity
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
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, data)
}
