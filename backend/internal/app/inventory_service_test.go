package app

import (
	"errors"
	"gorm.io/gorm"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
	"testing"
)

func TestInventoryServiceRollsBackWhenAuditFails(t *testing.T) {
	for _, operation := range []string{"adjust", "create", "complete", "close"} {
		t.Run(operation, func(t *testing.T) {
			s := newIdempotencyTestServer(t)
			if err := s.DB.AutoMigrate(&model.Product{}, &model.ProductStockAdjustment{}, &model.Order{}, &model.OrderEvent{}); err != nil {
				t.Fatal(err)
			}
			p := model.Product{ProductNo: "audit-test", MerchantID: 7, Status: model.ProductOnShelf, Stock: 5, Version: 1}
			if err := s.DB.Create(&p).Error; err != nil {
				t.Fatal(err)
			}
			actor := common.Actor{UserID: 42, MerchantID: 7, UserType: model.UserTypeMerchant}
			fail := errors.New("audit unavailable")
			audit := func(*gorm.DB, string, uint64, string, *string, *string, int, *uint64, map[string]interface{}) error {
				return fail
			}
			svc := inventoryService{}
			var order model.Order
			if operation == "complete" || operation == "close" {
				ok := func(*gorm.DB, string, uint64, string, *string, *string, int, *uint64, map[string]interface{}) error {
					return nil
				}
				var err error
				order, err = svc.createOrder(s.DB, actor, ok, dto.CreateOrderRequest{ProductID: p.ID, DealPriceCent: 100})
				if err != nil {
					t.Fatal(err)
				}
			}
			var before model.Product
			s.DB.First(&before, p.ID)
			err := s.DB.Transaction(func(tx *gorm.DB) error {
				switch operation {
				case "adjust":
					_, err := svc.adjustStock(tx, actor, audit, p.ID, dto.AdjustProductStockRequest{AdjustmentType: model.StockAdjustmentIncrease, Quantity: 2, Reason: "audit test"})
					return err
				case "create":
					_, err := svc.createOrder(tx, actor, audit, dto.CreateOrderRequest{ProductID: p.ID, DealPriceCent: 100})
					return err
				default:
					status := model.OrderCompleted
					if operation == "close" {
						status = model.OrderClosed
					}
					_, err := svc.transitionOrder(tx, actor, audit, order.ID, status, "test", nil)
					return err
				}
			})
			if !errors.Is(err, fail) {
				t.Fatalf("error = %v", err)
			}
			var after model.Product
			if err := s.DB.First(&after, p.ID).Error; err != nil {
				t.Fatal(err)
			}
			if after.Stock != before.Stock || after.ReservedStock != before.ReservedStock || after.Status != before.Status || after.Version != before.Version {
				t.Fatalf("product changed despite audit failure: %+v", after)
			}
			if countIdempotencyTestRows(t, s.DB, &model.ProductStockAdjustment{}) != 0 {
				t.Fatal("stock movement leaked")
			}
			expected := int64(0)
			if order.ID != 0 {
				expected = 1
				s.DB.First(&order, order.ID)
				if order.Status != model.OrderCreated {
					t.Fatal("order status leaked")
				}
			}
			if countIdempotencyTestRows(t, s.DB, &model.Order{}) != expected || countIdempotencyTestRows(t, s.DB, &model.OrderEvent{}) != expected {
				t.Fatal("order/event leaked")
			}
		})
	}
}
