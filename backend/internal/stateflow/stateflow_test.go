package stateflow

import (
	"testing"

	"second-hand-market-backend/backend/internal/model"
)

func TestProductTransition(t *testing.T) {
	if !CanTransitionProduct(model.ProductOnShelf, model.ProductLocked) {
		t.Fatal("expected ON_SHELF -> LOCKED allowed")
	}
	if CanTransitionProduct(model.ProductSold, model.ProductOnShelf) {
		t.Fatal("expected SOLD terminal")
	}
}

func TestMerchantTransition(t *testing.T) {
	if !CanTransitionMerchant(model.ReviewRejected, model.ReviewPending) {
		t.Fatal("expected REJECTED -> PENDING allowed")
	}
}

func TestEditableFields(t *testing.T) {
	fields := EditableFieldsByProductStatus(model.ProductOnShelf)
	if !fields["description"] || fields["title"] {
		t.Fatal("ON_SHELF editable fields mismatch")
	}
}
