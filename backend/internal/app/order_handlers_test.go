package app

import (
	"math"
	"testing"
)

func TestOrderTotalDealPriceCentRejectsOverflow(t *testing.T) {
	if _, err := orderTotalDealPriceCent(2, math.MaxInt); err == nil {
		t.Fatal("expected overflowing order total to be rejected")
	}
	total, err := orderTotalDealPriceCent(5, 1200)
	if err != nil || total != 6000 {
		t.Fatalf("unexpected total=%d err=%v", total, err)
	}
}
