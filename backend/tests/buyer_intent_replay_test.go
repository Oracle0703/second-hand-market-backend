package tests

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestBuyerIntentReplayPreservesScopeAndResult(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "replay")
	otherID, _, _ := registerMerchant(t, srv, "other-replay")
	merchantNo := merchantNoByID(t, srv, merchantID)
	otherNo := merchantNoByID(t, srv, otherID)
	productID := createAndOnShelfProduct(t, srv, str(merchantLogin(t, srv, username, password).Data["access_token"]))
	buyer := buyerLogin(t, serverAdapter{h: srv.Router}, "wx-replay", "replay-device")
	if buyer.Code != 0 {
		t.Fatalf("buyer login: %+v", buyer)
	}
	headers := map[string]string{"Authorization": "Bearer " + str(buyer.Data["access_token"]), "X-Device-Id": "replay-device"}
	body := map[string]interface{}{"product_id": productID, "contact_phone": "13800000000"}
	post := func(shop, key string) apiResp {
		headers["Idempotency-Key"] = key
		return requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/intents", shop), body, headers)
	}
	first := post(merchantNo, "original-key")
	if first.Code != 0 || first.Data["intent_id"] == nil || first.Data["intent_no"] == nil {
		t.Fatalf("first creation: %+v", first)
	}
	expectReplay := func(key string) {
		t.Helper()
		replay := post(merchantNo, key)
		if replay.Code != 0 || replay.Data["idempotent"] != true {
			t.Fatalf("replay: %+v", replay)
		}
		delete(replay.Data, "idempotent")
		if !reflect.DeepEqual(first.Data, replay.Data) {
			t.Fatalf("original=%+v replay=%+v", first.Data, replay.Data)
		}
	}
	expectConflict := func(shop, key string) {
		t.Helper()
		resp := post(shop, key)
		if resp.Code != common.CodeDuplicateSubmit || resp.HTTPStatus != http.StatusConflict || len(resp.Data) != 0 {
			t.Fatalf("expected a conflict without original data: %+v", resp)
		}
	}

	// Recreate a pre-change record: merchant_no was never included in the hash.
	legacyBody, err := json.Marshal(map[string]interface{}{"product_id": productID, "contact_name": nil, "contact_phone": "13800000000", "contact_wechat": nil, "message": nil})
	if err != nil {
		t.Fatal(err)
	}
	var record model.IdempotencyRecord
	if err := srv.DB.Where("idem_key = ?", "original-key").First(&record).Error; err != nil {
		t.Fatal(err)
	}
	record.ID, record.IdemKey, record.RequestHash = 0, "legacy-key", common.SHA256(string(legacyBody))
	if err := srv.DB.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.Model(&model.Product{}).Where("id = ?", productID).Update("status", model.ProductOffShelf).Error; err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.Model(&model.BuyerIntent{}).Where("product_id = ?", productID).Updates(map[string]interface{}{"status": model.IntentClosed, "is_open": false}).Error; err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"original-key", "legacy-key"} {
		expectConflict(otherNo, key)
		body["message"] = "different"
		expectConflict(merchantNo, key)
		delete(body, "message")
		for i := 0; i < 6; i++ {
			expectReplay(key)
		}
	}
	// Four more fresh attempts fit in the five-per-minute budget, despite all replays.
	for i := 0; i < 4; i++ {
		if resp := post(merchantNo, "fresh-key"); resp.Code != common.CodeInvalidTransition {
			t.Fatalf("fresh creation against off-shelf product: %+v", resp)
		}
	}
	if resp := post(merchantNo, "fresh-key"); resp.Code != common.CodeRateLimit {
		t.Fatalf("fresh request beyond rate limit: %+v", resp)
	}
	expectReplay("original-key")
	if err := srv.DB.Where("intent_no = ?", first.Data["intent_no"]).Delete(&model.BuyerIntent{}).Error; err != nil {
		t.Fatal(err)
	}
	expectConflict(merchantNo, "legacy-key")
}
