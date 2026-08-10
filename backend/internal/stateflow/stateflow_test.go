package stateflow

import (
	"testing"

	"second-hand-market-backend/backend/internal/model"
)

func TestProductTransition(t *testing.T) {
	if !CanTransitionProduct(model.ProductDraft, model.ProductOnShelf) {
		t.Fatal("expected DRAFT -> ON_SHELF allowed")
	}
	if !CanTransitionProduct(model.ProductOnShelf, model.ProductOffShelf) {
		t.Fatal("expected ON_SHELF -> OFF_SHELF allowed")
	}
	if !CanTransitionProduct(model.ProductOnShelf, model.ProductLocked) {
		t.Fatal("expected ON_SHELF -> LOCKED allowed")
	}
	if !CanTransitionProduct(model.ProductOffShelf, model.ProductOnShelf) {
		t.Fatal("expected OFF_SHELF -> ON_SHELF allowed")
	}
	if CanTransitionProduct(model.ProductLocked, model.ProductOffShelf) {
		t.Fatal("expected LOCKED -> OFF_SHELF denied outside order close")
	}
	if CanTransitionProduct(model.ProductLocked, model.ProductSold) {
		t.Fatal("expected LOCKED -> SOLD denied outside order completion")
	}
	if CanTransitionProduct(model.ProductSold, model.ProductOffShelf) {
		t.Fatal("expected SOLD -> OFF_SHELF denied outside stock recovery")
	}
	if CanTransitionProduct(model.ProductSold, model.ProductOnShelf) {
		t.Fatal("expected SOLD -> ON_SHELF denied")
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
