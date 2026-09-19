package app

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func validateBuyerIntentState(intent model.BuyerIntent) error {
	switch intent.Status {
	case model.IntentNew, model.IntentContacted:
		if intent.IsOpen {
			return nil
		}
	case model.IntentClosed:
		if !intent.IsOpen {
			return nil
		}
	}
	return common.ErrInternal
}

// Called inside the idempotency transaction. The product lock serializes
// competing creates and rechecks availability, while the unique index also
// protects against writers outside this application.
func createBuyerIntent(tx *gorm.DB, intent *model.BuyerIntent) error {
	var product model.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND merchant_id = ?", intent.ProductID, intent.MerchantID).
		First(&product).Error; err != nil {
		return mapDatabaseError(err)
	}
	if product.Status != model.ProductOnShelf {
		return common.ErrInvalidTransition
	}
	var existing []model.BuyerIntent
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("buyer_id = ? AND product_id = ? AND is_open = ?", intent.BuyerID, intent.ProductID, true).
		Limit(1).Find(&existing).Error; err != nil {
		return common.ErrInternal
	}
	if len(existing) != 0 {
		if err := validateBuyerIntentState(existing[0]); err != nil {
			return err
		}
		return common.ErrConflict
	}
	if err := tx.Create(intent).Error; err != nil {
		return common.ErrInternal
	}
	return nil
}
