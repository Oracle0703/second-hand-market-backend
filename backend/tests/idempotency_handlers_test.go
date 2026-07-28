package tests

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

type idempotencyTransitionFixture struct {
	srv       *app.Server
	token     string
	path      string
	body      map[string]interface{}
	productID uint64
	intentID  uint64
	orderID   uint64
}

type idempotencyTransitionSnapshot struct {
	product            model.Product
	intent             model.BuyerIntent
	order              model.Order
	orderEventCount    int64
	operationLogCount  int64
	idempotencyRecords int64
}

type buyerIntentIdempotencyFixture struct {
	srv       *app.Server
	headers   map[string]string
	body      map[string]interface{}
	buyerID   uint64
	productID uint64
}

func TestBuyerIntentIdempotencyReplayBypassesChangedProductAndRateLimit(t *testing.T) {
	fixture := newBuyerIntentIdempotencyFixture(t)
	first := requestBuyerIntentIdempotency(t, fixture, "buyer-intent-replay-key", fixture.body)
	assertSuccessfulBuyerIntentIdempotency(t, first, false)

	if err := fixture.srv.DB.Model(&model.Product{}).Where("id = ?", fixture.productID).Update("status", model.ProductOffShelf).Error; err != nil {
		t.Fatalf("change product status after commit: %v", err)
	}
	for index := 0; index < 6; index++ {
		replay := requestBuyerIntentIdempotency(t, fixture, "buyer-intent-replay-key", fixture.body)
		assertSuccessfulBuyerIntentIdempotency(t, replay, true)
		assertSameIdempotencyBusinessPayload(t, first, replay)
	}
	assertSingleBuyerIntentMutation(t, fixture, "buyer-intent-replay-key")
}

func TestBuyerIntentIdempotencyTerminalFailureRollsBackAndRetrySucceeds(t *testing.T) {
	fixture := newBuyerIntentIdempotencyFixture(t)
	failGORMTableOperation(t, fixture.srv.DB, "update", "idempotency_records", errors.New("forced buyer intent terminal failure"))

	failed := requestBuyerIntentIdempotency(t, fixture, "buyer-intent-terminal-key", fixture.body)
	if failed.HTTPStatus != http.StatusInternalServerError || failed.Code != common.CodeInternal {
		t.Fatalf("terminal failure response = status %d, code %d", failed.HTTPStatus, failed.Code)
	}
	assertNoBuyerIntentMutation(t, fixture, "buyer-intent-terminal-key")

	removeGORMTableOperation(t, fixture.srv.DB, "update", "idempotency_records")
	retry := requestBuyerIntentIdempotency(t, fixture, "buyer-intent-terminal-key", fixture.body)
	assertSuccessfulBuyerIntentIdempotency(t, retry, false)
	assertSingleBuyerIntentMutation(t, fixture, "buyer-intent-terminal-key")
}

func TestBuyerIntentIdempotencyDifferentHashReturns10011(t *testing.T) {
	fixture := newBuyerIntentIdempotencyFixture(t)
	first := requestBuyerIntentIdempotency(t, fixture, "buyer-intent-hash-key", fixture.body)
	assertSuccessfulBuyerIntentIdempotency(t, first, false)

	different := map[string]interface{}{
		"product_id":     fixture.productID,
		"contact_wechat": "different-contact",
		"message":        "idempotent buyer intent",
	}
	mismatch := requestBuyerIntentIdempotency(t, fixture, "buyer-intent-hash-key", different)
	if mismatch.HTTPStatus != http.StatusConflict || mismatch.Code != common.CodeDuplicateSubmit {
		t.Fatalf("different-hash response = status %d, code %d", mismatch.HTTPStatus, mismatch.Code)
	}
	assertSingleBuyerIntentMutation(t, fixture, "buyer-intent-hash-key")
}

func newBuyerIntentIdempotencyFixture(t *testing.T) buyerIntentIdempotencyFixture {
	t.Helper()
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "buyer_intent_idempotency")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	merchant := merchantLogin(t, srv, username, password)
	if merchant.Code != common.CodeOK {
		t.Fatalf("merchant login: code %d", merchant.Code)
	}
	productID := createAndOnShelfProduct(t, srv, str(merchant.Data["access_token"]))
	buyer := buyerLogin(t, serverAdapter{h: srv.Router}, "buyer-intent-idempotency", "buyer-intent-idempotency-device")
	if buyer.Code != common.CodeOK {
		t.Fatalf("buyer login: code %d", buyer.Code)
	}
	return buyerIntentIdempotencyFixture{
		srv: srv,
		headers: map[string]string{
			"Authorization": "Bearer " + str(buyer.Data["access_token"]),
			"X-Device-Id":   "buyer-intent-idempotency-device",
		},
		body: map[string]interface{}{
			"product_id":     productID,
			"contact_wechat": "idempotent-contact",
			"message":        "idempotent buyer intent",
		},
		buyerID:   numToUint64(buyer.Data["user"].(map[string]interface{})["id"]),
		productID: productID,
	}
}

func requestBuyerIntentIdempotency(t *testing.T, fixture buyerIntentIdempotencyFixture, key string, body map[string]interface{}) apiResp {
	t.Helper()
	headers := make(map[string]string, len(fixture.headers)+1)
	for name, value := range fixture.headers {
		headers[name] = value
	}
	headers["Idempotency-Key"] = key
	return requestJSON(t, fixture.srv.Router, http.MethodPost, "/api/v1/buyer/intents", body, headers)
}

func assertSuccessfulBuyerIntentIdempotency(t *testing.T, response apiResp, replay bool) {
	t.Helper()
	if response.HTTPStatus != http.StatusOK || response.Code != common.CodeOK {
		t.Fatalf("buyer intent response = status %d, code %d", response.HTTPStatus, response.Code)
	}
	if numToUint64(response.Data["intent_id"]) == 0 || str(response.Data["intent_no"]) == "" || str(response.Data["status"]) != model.IntentNew || str(response.Data["created_at"]) == "" {
		t.Fatal("buyer intent success payload changed")
	}
	if replay {
		if got, ok := response.Data["idempotent"].(bool); !ok || !got {
			t.Fatalf("replay idempotent flag = %v, want true", response.Data["idempotent"])
		}
	} else if _, ok := response.Data["idempotent"]; ok {
		t.Fatalf("first execution unexpectedly included idempotent flag: %v", response.Data["idempotent"])
	}
}

func assertNoBuyerIntentMutation(t *testing.T, fixture buyerIntentIdempotencyFixture, key string) {
	t.Helper()
	assertBuyerIntentMutationCounts(t, fixture, key, 0, 0)
}

func assertSingleBuyerIntentMutation(t *testing.T, fixture buyerIntentIdempotencyFixture, key string) {
	t.Helper()
	assertBuyerIntentMutationCounts(t, fixture, key, 1, 1)
}

func assertBuyerIntentMutationCounts(t *testing.T, fixture buyerIntentIdempotencyFixture, key string, wantIntents, wantRecords int64) {
	t.Helper()
	var intents int64
	if err := fixture.srv.DB.Model(&model.BuyerIntent{}).Where("buyer_id = ? AND product_id = ?", fixture.buyerID, fixture.productID).Count(&intents).Error; err != nil {
		t.Fatalf("count buyer intents: %v", err)
	}
	var records int64
	if err := fixture.srv.DB.Model(&model.IdempotencyRecord{}).Where("idem_key = ?", key).Count(&records).Error; err != nil {
		t.Fatalf("count buyer intent idempotency records: %v", err)
	}
	if intents != wantIntents || records != wantRecords {
		t.Fatalf("buyer intent/idempotency counts = %d/%d, want %d/%d", intents, records, wantIntents, wantRecords)
	}
}

func TestIdempotentMerchantAndOrderTransitionsRollbackWhenTerminalWriteFails(t *testing.T) {
	for _, transition := range idempotencyTransitionCases() {
		t.Run(transition.name, func(t *testing.T) {
			fixture := transition.newFixture(t)
			before := snapshotIdempotencyTransition(t, fixture)
			failGORMTableOperation(t, fixture.srv.DB, "update", "idempotency_records", errors.New("forced terminal write failure"))

			failed := requestIdempotencyTransition(t, fixture, "terminal-write-key")
			if failed.HTTPStatus != http.StatusInternalServerError || failed.Code != common.CodeInternal {
				t.Fatalf("terminal write failure response = status %d, code %d", failed.HTTPStatus, failed.Code)
			}
			assertIdempotencyTransitionSnapshot(t, fixture, before)

			removeGORMTableOperation(t, fixture.srv.DB, "update", "idempotency_records")
			retry := requestIdempotencyTransition(t, fixture, "terminal-write-key")
			assertSuccessfulIdempotencyTransition(t, transition, fixture, retry, false)
			assertSingleTerminalRecord(t, fixture, "terminal-write-key")
			assertCommittedIdempotencyTransition(t, transition, fixture, before)
		})
	}
}

func TestIdempotentMerchantAndOrderTransitionsRollbackWhenOperationLogFails(t *testing.T) {
	for _, transition := range idempotencyTransitionCases() {
		t.Run(transition.name, func(t *testing.T) {
			fixture := transition.newFixture(t)
			before := snapshotIdempotencyTransition(t, fixture)
			failGORMTableOperation(t, fixture.srv.DB, "create", "operation_logs", errors.New("forced operation log failure"))

			failed := requestIdempotencyTransition(t, fixture, "operation-log-key")
			if failed.HTTPStatus != http.StatusInternalServerError || failed.Code != common.CodeInternal {
				t.Fatalf("operation log failure response = status %d, code %d", failed.HTTPStatus, failed.Code)
			}
			assertIdempotencyTransitionSnapshot(t, fixture, before)

			removeGORMTableOperation(t, fixture.srv.DB, "create", "operation_logs")
			retry := requestIdempotencyTransition(t, fixture, "operation-log-key")
			assertSuccessfulIdempotencyTransition(t, transition, fixture, retry, false)
			assertSingleTerminalRecord(t, fixture, "operation-log-key")
			assertCommittedIdempotencyTransition(t, transition, fixture, before)
		})
	}
}

func TestIdempotentMerchantAndOrderTransitionsPreserveSuccessPayloads(t *testing.T) {
	for _, transition := range idempotencyTransitionCases() {
		t.Run(transition.name, func(t *testing.T) {
			fixture := transition.newFixture(t)
			before := snapshotIdempotencyTransition(t, fixture)
			first := requestIdempotencyTransition(t, fixture, "success-payload-key")
			assertSuccessfulIdempotencyTransition(t, transition, fixture, first, false)
			committed := assertCommittedIdempotencyTransition(t, transition, fixture, before)

			replay := requestIdempotencyTransition(t, fixture, "success-payload-key")
			assertSuccessfulIdempotencyTransition(t, transition, fixture, replay, true)
			assertSameIdempotencyBusinessPayload(t, first, replay)
			assertSingleTerminalRecord(t, fixture, "success-payload-key")
			assertIdempotencyTransitionSnapshot(t, fixture, committed)
		})
	}
}

type idempotencyTransitionCase struct {
	name              string
	newFixture        func(*testing.T) idempotencyTransitionFixture
	operationLogDelta int64
	orderEventDelta   int64
	assertData        func(*testing.T, idempotencyTransitionFixture, apiResp)
	assertPersisted   func(*testing.T, idempotencyTransitionSnapshot)
}

func idempotencyTransitionCases() []idempotencyTransitionCase {
	return []idempotencyTransitionCase{
		{
			name:              "merchant intent new to contacted",
			newFixture:        newContactedIntentFixture,
			operationLogDelta: 1,
			assertData: func(t *testing.T, fixture idempotencyTransitionFixture, response apiResp) {
				t.Helper()
				if numToUint64(response.Data["intent_id"]) != fixture.intentID || str(response.Data["from_status"]) != model.IntentNew || str(response.Data["to_status"]) != model.IntentContacted || str(response.Data["handled_at"]) == "" {
					t.Fatalf("contacted payload changed")
				}
			},
			assertPersisted: func(t *testing.T, snapshot idempotencyTransitionSnapshot) {
				t.Helper()
				if snapshot.intent.Status != model.IntentContacted || !snapshot.intent.IsOpen {
					t.Fatal("contacted intent state was not committed exactly once")
				}
			},
		},
		{
			name:              "merchant intent contacted to closed",
			newFixture:        newClosedIntentFixture,
			operationLogDelta: 1,
			assertData: func(t *testing.T, fixture idempotencyTransitionFixture, response apiResp) {
				t.Helper()
				if numToUint64(response.Data["intent_id"]) != fixture.intentID || str(response.Data["from_status"]) != model.IntentContacted || str(response.Data["to_status"]) != model.IntentClosed || str(response.Data["closed_at"]) == "" {
					t.Fatalf("closed intent payload changed")
				}
			},
			assertPersisted: func(t *testing.T, snapshot idempotencyTransitionSnapshot) {
				t.Helper()
				if snapshot.intent.Status != model.IntentClosed || snapshot.intent.IsOpen {
					t.Fatal("closed intent state was not committed exactly once")
				}
			},
		},
		{
			name:              "product draft to on shelf",
			newFixture:        newOnShelfProductFixture,
			operationLogDelta: 1,
			assertData: func(t *testing.T, fixture idempotencyTransitionFixture, response apiResp) {
				t.Helper()
				if numToUint64(response.Data["product_id"]) != fixture.productID || str(response.Data["from_status"]) != model.ProductDraft || str(response.Data["to_status"]) != model.ProductOnShelf || str(response.Data["changed_at"]) == "" {
					t.Fatalf("on-shelf payload changed")
				}
			},
			assertPersisted: func(t *testing.T, snapshot idempotencyTransitionSnapshot) {
				t.Helper()
				if snapshot.product.Status != model.ProductOnShelf {
					t.Fatal("on-shelf product state was not committed exactly once")
				}
			},
		},
		{
			name:              "order created to completed",
			newFixture:        newCompletedOrderFixture,
			operationLogDelta: 2,
			orderEventDelta:   1,
			assertData: func(t *testing.T, fixture idempotencyTransitionFixture, response apiResp) {
				t.Helper()
				if numToUint64(response.Data["order_id"]) != fixture.orderID || str(response.Data["from_status"]) != model.OrderCreated || str(response.Data["to_status"]) != model.OrderCompleted || str(response.Data["product_status"]) != model.ProductSold || numToUint64(response.Data["quantity"]) != 1 || numToUint64(response.Data["deal_price_cent"]) != 1000 || numToUint64(response.Data["total_deal_price_cent"]) != 1000 || numToUint64(response.Data["stock"]) != 0 || numToUint64(response.Data["reserved_stock"]) != 0 || numToUint64(response.Data["available_stock"]) != 0 || str(response.Data["completed_at"]) == "" {
					t.Fatalf("completed order payload changed")
				}
			},
			assertPersisted: func(t *testing.T, snapshot idempotencyTransitionSnapshot) {
				t.Helper()
				if snapshot.order.Status != model.OrderCompleted || snapshot.order.IsActive ||
					snapshot.product.Status != model.ProductSold || snapshot.product.Stock != 0 || snapshot.product.ReservedStock != 0 {
					t.Fatal("completed order inventory state was not committed exactly once")
				}
			},
		},
	}
}

func newContactedIntentFixture(t *testing.T) idempotencyTransitionFixture {
	t.Helper()
	fixture := newIntentTransitionFixture(t)
	fixture.path = fmt.Sprintf("/api/v1/merchant/intents/%d/contacted", fixture.intentID)
	fixture.body = map[string]interface{}{}
	return fixture
}

func newClosedIntentFixture(t *testing.T) idempotencyTransitionFixture {
	t.Helper()
	fixture := newContactedIntentFixture(t)
	contacted := requestJSON(t, fixture.srv.Router, http.MethodPost, fixture.path, fixture.body, map[string]string{"Authorization": "Bearer " + fixture.token})
	if contacted.Code != common.CodeOK {
		t.Fatalf("prepare contacted intent: code %d", contacted.Code)
	}
	fixture.path = fmt.Sprintf("/api/v1/merchant/intents/%d/close", fixture.intentID)
	fixture.body = map[string]interface{}{"reason": "NO_RESPONSE", "merchant_note": "closed"}
	return fixture
}

func newIntentTransitionFixture(t *testing.T) idempotencyTransitionFixture {
	t.Helper()
	fixture := newMerchantTransitionFixture(t)
	fixture.productID = createAndOnShelfProduct(t, fixture.srv, fixture.token)
	buyer := buyerLogin(t, serverAdapter{h: fixture.srv.Router}, "idempotency-intent", "intent-device")
	if buyer.Code != common.CodeOK {
		t.Fatalf("buyer login: code %d", buyer.Code)
	}
	created := requestJSON(t, fixture.srv.Router, http.MethodPost, "/api/v1/buyer/intents", map[string]interface{}{"product_id": fixture.productID, "contact_wechat": "contact", "message": "request"}, map[string]string{"Authorization": "Bearer " + str(buyer.Data["access_token"]), "X-Device-Id": "intent-device"})
	if created.Code != common.CodeOK {
		t.Fatalf("create intent: code %d", created.Code)
	}
	fixture.intentID = numToUint64(created.Data["intent_id"])
	return fixture
}

func newOnShelfProductFixture(t *testing.T) idempotencyTransitionFixture {
	t.Helper()
	fixture := newMerchantTransitionFixture(t)
	fixture.productID = createDraftProduct(t, fixture.srv, fixture.token)
	fixture.path = fmt.Sprintf("/api/v1/merchant/products/%d/on-shelf", fixture.productID)
	fixture.body = map[string]interface{}{}
	return fixture
}

func newCompletedOrderFixture(t *testing.T) idempotencyTransitionFixture {
	t.Helper()
	fixture := newMerchantTransitionFixture(t)
	fixture.productID = createAndOnShelfProduct(t, fixture.srv, fixture.token)
	created := createMerchantOrder(t, fixture.token, fixture.productID, 1, 1000, fixture.srv.Router)
	if created.Code != common.CodeOK {
		t.Fatalf("create order: code %d", created.Code)
	}
	fixture.orderID = numToUint64(created.Data["order_id"])
	fixture.path = fmt.Sprintf("/api/v1/merchant/orders/%d/complete", fixture.orderID)
	fixture.body = map[string]interface{}{"note": "complete"}
	return fixture
}

func newMerchantTransitionFixture(t *testing.T) idempotencyTransitionFixture {
	t.Helper()
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "idempotency_transition")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	login := merchantLogin(t, srv, username, password)
	if login.Code != common.CodeOK {
		t.Fatalf("merchant login: code %d", login.Code)
	}
	return idempotencyTransitionFixture{srv: srv, token: str(login.Data["access_token"])}
}

func requestIdempotencyTransition(t *testing.T, fixture idempotencyTransitionFixture, key string) apiResp {
	t.Helper()
	return requestJSON(t, fixture.srv.Router, http.MethodPost, fixture.path, fixture.body, map[string]string{"Authorization": "Bearer " + fixture.token, "Idempotency-Key": key})
}

func snapshotIdempotencyTransition(t *testing.T, fixture idempotencyTransitionFixture) idempotencyTransitionSnapshot {
	t.Helper()
	var snapshot idempotencyTransitionSnapshot
	if fixture.productID != 0 {
		if err := fixture.srv.DB.First(&snapshot.product, fixture.productID).Error; err != nil {
			t.Fatalf("load product snapshot: %v", err)
		}
	}
	if fixture.intentID != 0 {
		if err := fixture.srv.DB.First(&snapshot.intent, fixture.intentID).Error; err != nil {
			t.Fatalf("load intent snapshot: %v", err)
		}
	}
	if fixture.orderID != 0 {
		if err := fixture.srv.DB.First(&snapshot.order, fixture.orderID).Error; err != nil {
			t.Fatalf("load order snapshot: %v", err)
		}
	}
	for _, count := range []struct {
		model interface{}
		dest  *int64
	}{
		{&model.OrderEvent{}, &snapshot.orderEventCount},
		{&model.OperationLog{}, &snapshot.operationLogCount},
		{&model.IdempotencyRecord{}, &snapshot.idempotencyRecords},
	} {
		if err := fixture.srv.DB.Model(count.model).Count(count.dest).Error; err != nil {
			t.Fatalf("count transition snapshot: %v", err)
		}
	}
	return snapshot
}

func assertIdempotencyTransitionSnapshot(t *testing.T, fixture idempotencyTransitionFixture, want idempotencyTransitionSnapshot) {
	t.Helper()
	if got := snapshotIdempotencyTransition(t, fixture); !reflect.DeepEqual(got, want) {
		t.Fatal("failed idempotent transition changed persisted state")
	}
}

func assertSuccessfulIdempotencyTransition(t *testing.T, transition idempotencyTransitionCase, fixture idempotencyTransitionFixture, response apiResp, replay bool) {
	t.Helper()
	if response.HTTPStatus != http.StatusOK || response.Code != common.CodeOK {
		t.Fatalf("successful idempotent transition response = status %d, code %d", response.HTTPStatus, response.Code)
	}
	transition.assertData(t, fixture, response)
	if got, ok := response.Data["idempotent"].(bool); !ok || got != replay {
		t.Fatalf("idempotent flag = %v, want %t", response.Data["idempotent"], replay)
	}
}

func assertSameIdempotencyBusinessPayload(t *testing.T, first, replay apiResp) {
	t.Helper()
	withoutFlag := func(data map[string]interface{}) map[string]interface{} {
		business := make(map[string]interface{}, len(data))
		for key, value := range data {
			if key != "idempotent" {
				business[key] = value
			}
		}
		return business
	}
	if !reflect.DeepEqual(withoutFlag(first.Data), withoutFlag(replay.Data)) {
		t.Fatal("replayed business payload changed")
	}
}

func assertCommittedIdempotencyTransition(t *testing.T, transition idempotencyTransitionCase, fixture idempotencyTransitionFixture, before idempotencyTransitionSnapshot) idempotencyTransitionSnapshot {
	t.Helper()
	after := snapshotIdempotencyTransition(t, fixture)
	if after.operationLogCount != before.operationLogCount+transition.operationLogDelta {
		t.Fatalf("operation log count = %d, want %d", after.operationLogCount, before.operationLogCount+transition.operationLogDelta)
	}
	if after.orderEventCount != before.orderEventCount+transition.orderEventDelta {
		t.Fatalf("order event count = %d, want %d", after.orderEventCount, before.orderEventCount+transition.orderEventDelta)
	}
	if after.idempotencyRecords != before.idempotencyRecords+1 {
		t.Fatalf("idempotency record count = %d, want %d", after.idempotencyRecords, before.idempotencyRecords+1)
	}
	transition.assertPersisted(t, after)
	return after
}

func assertSingleTerminalRecord(t *testing.T, fixture idempotencyTransitionFixture, key string) {
	t.Helper()
	var count int64
	if err := fixture.srv.DB.Model(&model.IdempotencyRecord{}).Where("idem_key = ?", key).Count(&count).Error; err != nil {
		t.Fatalf("count terminal records: %v", err)
	}
	if count != 1 {
		t.Fatalf("terminal record count = %d", count)
	}
}

func failGORMTableOperation(t *testing.T, db *gorm.DB, operation string, table string, forced error) {
	t.Helper()
	name := gormTableOperationCallbackName(t, operation, table)
	callback := func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == table {
			tx.AddError(forced)
		}
	}
	var err error
	switch operation {
	case "create":
		err = db.Callback().Create().Before("gorm:create").Register(name, callback)
	case "update":
		err = db.Callback().Update().Before("gorm:update").Register(name, callback)
	default:
		t.Fatalf("unsupported GORM operation %q", operation)
	}
	if err != nil {
		t.Fatalf("register %s callback for %s: %v", operation, table, err)
	}
	t.Cleanup(func() { removeGORMTableOperation(t, db, operation, table) })
}

func removeGORMTableOperation(t *testing.T, db *gorm.DB, operation string, table string) {
	t.Helper()
	switch operation {
	case "create":
		_ = db.Callback().Create().Remove(gormTableOperationCallbackName(t, operation, table))
	case "update":
		_ = db.Callback().Update().Remove(gormTableOperationCallbackName(t, operation, table))
	default:
		t.Fatalf("unsupported GORM operation %q", operation)
	}
}

func gormTableOperationCallbackName(t *testing.T, operation string, table string) string {
	return "idempotency-handlers:" + strings.ReplaceAll(t.Name(), "/", "-") + ":" + operation + ":" + table
}
