package app

import (
	"errors"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func validateBuyerIntentStatus(status string, isOpen bool) error {
	switch status {
	case model.IntentNew, model.IntentContacted:
		if isOpen {
			return nil
		}
	case model.IntentClosed:
		if !isOpen {
			return nil
		}
	}
	return common.ErrInternal
}

func validateBuyerIntentState(intent model.BuyerIntent) error {
	return validateBuyerIntentStatus(intent.Status, intent.IsOpen)
}

func findOpenBuyerIntent(
	db *gorm.DB,
	buyerID uint64,
	productID uint64,
) (bool, error) {
	var intents []model.BuyerIntent
	if err := db.Select("status", "is_open").Where(
		"buyer_id = ? AND product_id = ?",
		buyerID, productID,
	).Find(&intents).Error; err != nil {
		return false, common.ErrInternal
	}
	found := false
	for _, intent := range intents {
		if err := validateBuyerIntentState(intent); err != nil {
			return false, common.ErrInternal
		}
		if intent.IsOpen {
			if found {
				return false, common.ErrInternal
			}
			found = true
		}
	}
	return found, nil
}

func classifyBuyerIntentCreateError(
	db *gorm.DB,
	createErr error,
	buyerID uint64,
	productID uint64,
) error {
	if !errors.Is(createErr, gorm.ErrDuplicatedKey) {
		return common.ErrInternal
	}
	found, err := findOpenBuyerIntent(db, buyerID, productID)
	if err != nil || !found {
		return common.ErrInternal
	}
	return common.ErrConflict
}
