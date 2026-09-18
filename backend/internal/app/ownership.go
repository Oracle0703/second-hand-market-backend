package app

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func (s *Server) loadOwnedProduct(tx *gorm.DB, productID, merchantID uint64) (model.Product, error) {
	target := s.DB
	if tx != nil {
		target = tx
	}
	var product model.Product
	if err := target.Where("id = ?", productID).First(&product).Error; err != nil {
		return model.Product{}, mapDatabaseError(err)
	}
	if product.MerchantID != merchantID {
		return model.Product{}, common.ErrForbidden
	}
	return product, nil
}

func (s *Server) loadOwnedOrder(tx *gorm.DB, orderID, merchantID uint64) (model.Order, error) {
	target := s.DB
	if tx != nil {
		target = tx
	}
	var order model.Order
	if err := target.Where("id = ?", orderID).First(&order).Error; err != nil {
		return model.Order{}, mapDatabaseError(err)
	}
	if order.MerchantID != merchantID {
		return model.Order{}, common.ErrForbidden
	}
	return order, nil
}

func loadOwnedOrderForUpdate(tx *gorm.DB, orderID, merchantID uint64) (model.Order, error) {
	var order model.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&order).Error; err != nil {
		return model.Order{}, mapDatabaseError(err)
	}
	if order.MerchantID != merchantID {
		return model.Order{}, common.ErrForbidden
	}
	return order, nil
}
func loadOwnedProductForUpdate(tx *gorm.DB, productID uint64, merchantID uint64) (model.Product, error) {
	var product model.Product
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", productID).
		First(&product).Error
	if err != nil {
		return model.Product{}, mapDatabaseError(err)
	}
	if product.MerchantID != merchantID {
		return model.Product{}, common.ErrForbidden
	}
	return product, nil
}

func (s *Server) loadOwnedProductForUpdate(tx *gorm.DB, id, merchantID uint64) (model.Product, error) {
	return loadOwnedProductForUpdate(tx, id, merchantID)
}
func (s *Server) loadOwnedOrderForUpdate(tx *gorm.DB, id, merchantID uint64) (model.Order, error) {
	return loadOwnedOrderForUpdate(tx, id, merchantID)
}
