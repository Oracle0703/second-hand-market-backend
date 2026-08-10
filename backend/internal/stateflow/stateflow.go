package stateflow

import "second-hand-market-backend/backend/internal/model"

var productTransitions = map[string]map[string]bool{
	model.ProductDraft: {
		model.ProductOnShelf: true,
	},
	model.ProductOnShelf: {
		model.ProductOffShelf: true,
		model.ProductLocked:   true,
	},
	model.ProductOffShelf: {
		model.ProductOnShelf: true,
	},
}

var orderTransitions = map[string]map[string]bool{
	model.OrderCreated: {
		model.OrderCompleted: true,
		model.OrderClosed:    true,
	},
}

var merchantTransitions = map[string]map[string]bool{
	model.ReviewPending: {
		model.ReviewApproved: true,
		model.ReviewRejected: true,
	},
	model.ReviewRejected: {
		model.ReviewPending: true,
	},
}

func CanTransitionProduct(from, to string) bool {
	if nexts, ok := productTransitions[from]; ok {
		return nexts[to]
	}
	return false
}

func CanTransitionOrder(from, to string) bool {
	if nexts, ok := orderTransitions[from]; ok {
		return nexts[to]
	}
	return false
}

func CanTransitionMerchant(from, to string) bool {
	if nexts, ok := merchantTransitions[from]; ok {
		return nexts[to]
	}
	return false
}

func EditableFieldsByProductStatus(status string) map[string]bool {
	switch status {
	case model.ProductDraft, model.ProductOffShelf:
		return map[string]bool{
			"title": true, "description": true, "category_id": true, "price_cent": true, "original_price_cent": true, "condition_level": true, "stock": true, "image_file_ids": true,
		}
	case model.ProductOnShelf:
		return map[string]bool{"description": true, "image_file_ids": true}
	default:
		return map[string]bool{}
	}
}
