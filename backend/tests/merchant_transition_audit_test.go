package tests

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestMerchantTransitionsRollbackOnAuditFailure(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "audit-rollback")
	token := str(merchantLogin(t, srv, username, password).Data["access_token"])
	draft := createDraftProduct(t, srv, token)
	onShelf := createAndOnShelfProduct(t, srv, token)
	intent := model.BuyerIntent{IntentNo: "audit-contact", BuyerID: 10, MerchantID: merchantID, ProductID: onShelf, Status: model.IntentNew, IsOpen: true}
	if err := srv.DB.Create(&intent).Error; err != nil {
		t.Fatal(err)
	}
	contacted := model.BuyerIntent{IntentNo: "audit-close", BuyerID: 11, MerchantID: merchantID, ProductID: onShelf, Status: model.IntentContacted, IsOpen: true}
	if err := srv.DB.Create(&contacted).Error; err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		action, path, table string
		id                  uint64
	}{
		{"product_on_shelf", fmt.Sprintf("products/%d/on-shelf", draft), "products", draft},
		{"product_off_shelf", fmt.Sprintf("products/%d/off-shelf", onShelf), "products", onShelf},
		{"merchant_intent_contacted", fmt.Sprintf("intents/%d/contacted", intent.ID), "buyer_intents", intent.ID},
		{"merchant_intent_close", fmt.Sprintf("intents/%d/close", contacted.ID), "buyer_intents", contacted.ID},
	} {
		t.Run(tc.action, func(t *testing.T) {
			readState := func() map[string]interface{} {
				t.Helper()
				var row map[string]interface{}
				if err := srv.DB.Table(tc.table).Where("id = ?", tc.id).Take(&row).Error; err != nil {
					t.Fatal(err)
				}
				return row
			}
			headers := map[string]string{"Authorization": "Bearer " + token, "Idempotency-Key": tc.action}
			post := func() apiResp {
				return requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/"+tc.path, map[string]string{}, headers)
			}
			before := readState()
			failAudit := true
			callback := "test:fail-" + tc.action
			if err := srv.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if failAudit && tx.Statement.Table == "operation_logs" {
					tx.AddError(errors.New("audit unavailable"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer srv.DB.Callback().Create().Remove(callback)
			if failed := post(); failed.Code != common.CodeInternal {
				t.Fatalf("audit failure = %+v", failed)
			}
			if after := readState(); !reflect.DeepEqual(before, after) {
				t.Fatalf("transition escaped rollback: before=%v after=%v", before, after)
			}
			var claims int64
			if err := srv.DB.Model(&model.IdempotencyRecord{}).Where("idem_key = ?", tc.action).Count(&claims).Error; err != nil || claims != 0 {
				t.Fatalf("claims after rollback = %d, err=%v", claims, err)
			}
			failAudit = false
			if retry := post(); retry.Code != 0 || retry.Data["idempotent"] != false {
				t.Fatalf("retry = %+v", retry)
			}
			if replay := post(); replay.Code != 0 || replay.Data["idempotent"] != true {
				t.Fatalf("replay = %+v", replay)
			}
			var logs int64
			if err := srv.DB.Model(&model.OperationLog{}).Where("resource_id = ? AND action = ?", tc.id, tc.action).Count(&logs).Error; err != nil || logs != 1 {
				t.Fatalf("audit count = %d, err=%v", logs, err)
			}
		})
	}
}
