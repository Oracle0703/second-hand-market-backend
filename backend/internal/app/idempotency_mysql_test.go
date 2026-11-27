package app

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

type mysqlStockRequests struct {
	server  *Server
	router  http.Handler
	product model.Product
}

func newMySQLStockRequests(t *testing.T) mysqlStockRequests {
	t.Helper()
	s := newP1MySQLServer(t)
	p := model.Product{ProductNo: "idem-stock", MerchantID: 7, Status: model.ProductOnShelf, Stock: 5, Version: 1}
	if err := s.DB.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	return mysqlStockRequests{server: s, router: p1Router(s), product: p}
}

func (f mysqlStockRequests) increase(quantity int) p1Response {
	path := fmt.Sprintf("/products/%d/stock", f.product.ID)
	body := fmt.Sprintf(`{"adjustment_type":"INCREASE","quantity":%d,"reason":"stock count"}`, quantity)
	return p1Request(f.router, http.MethodPost, path, body, "shared-stock-key")
}

func (f mysqlStockRequests) assertState(t *testing.T, stock, version int, entries int64) {
	t.Helper()
	var product model.Product
	if err := f.server.DB.First(&product, f.product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if product.Stock != stock || product.Version != version {
		t.Fatalf("stock/version = %d/%d, want %d/%d", product.Stock, product.Version, stock, version)
	}
	for _, row := range []interface{}{&model.ProductStockAdjustment{}, &model.OperationLog{}, &model.IdempotencyRecord{}} {
		if n := countIdempotencyTestRows(t, f.server.DB, row); n != entries {
			t.Fatalf("%T has %d rows, want %d", row, n, entries)
		}
	}
}

// The owner's business writes have completed but its idempotency claim is still uncommitted.
// Wait for InnoDB to report the second connection blocked on that table before releasing the owner.
func (f mysqlStockRequests) contend(t *testing.T, waiterQuantity int, rollbackOwner bool) (p1Response, p1Response) {
	t.Helper()
	db := f.server.DB
	table := db.NamingStrategy.TableName("IdempotencyRecord")
	paused, release := make(chan struct{}), make(chan struct{})
	var owner atomic.Bool
	if err := db.Callback().Update().Before("gorm:update").Register("test:hold-idempotency-finalize", func(tx *gorm.DB) {
		if tx.Statement.Table != table || !owner.CompareAndSwap(false, true) {
			return
		}
		close(paused)
		<-release
		if rollbackOwner {
			tx.AddError(errForcedFinalize)
		}
	}); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var wg sync.WaitGroup
	defer func() { unblock(); wg.Wait() }()
	request := func(quantity int) <-chan p1Response {
		responses := make(chan p1Response, 1)
		wg.Add(1)
		go func() { defer wg.Done(); responses <- f.increase(quantity) }()
		return responses
	}
	first := request(2)
	select {
	case <-paused:
	case <-time.After(5 * time.Second):
		t.Fatal("owner did not reach the final result write")
	}
	waiter := request(waiterQuantity)
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waits int
		err := db.Raw(`SELECT COUNT(*) FROM performance_schema.data_lock_waits w
			JOIN performance_schema.data_locks l ON l.ENGINE = w.ENGINE AND l.ENGINE_LOCK_ID = w.REQUESTING_ENGINE_LOCK_ID
			WHERE l.OBJECT_SCHEMA = DATABASE() AND l.OBJECT_NAME = ?`, table).Scan(&waits).Error
		if err != nil {
			t.Fatal(err)
		}
		if waits > 0 {
			break
		}
		select {
		case response := <-waiter:
			t.Fatalf("waiter returned before the owner's commit or rollback: %+v", response)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("InnoDB did not report the waiter blocked on the idempotency claim")
		}
		time.Sleep(20 * time.Millisecond)
	}
	unblock()
	return <-first, <-waiter
}

func TestP1MySQLIdempotencyConflictingPayload(t *testing.T) {
	f := newMySQLStockRequests(t)
	owner, waiter := f.contend(t, 3, false)
	if owner.Code != common.CodeOK || waiter.Code != common.CodeDuplicateSubmit {
		t.Fatalf("owner=%+v, conflicting waiter=%+v", owner, waiter)
	}
	f.assertState(t, 7, 2, 1)
}

func TestP1MySQLIdempotencyWaiterTakesOverAfterRollback(t *testing.T) {
	f := newMySQLStockRequests(t)
	owner, waiter := f.contend(t, 2, true)
	if owner.Code != common.CodeInternal || waiter.Code != common.CodeOK || waiter.Data["movement_id"] == nil || waiter.Data["idempotent"] == true {
		t.Fatalf("rolled-back owner=%+v, waiting request=%+v", owner, waiter)
	}
	replay := f.increase(2)
	if replay.Code != common.CodeOK || replay.Data["idempotent"] != true || replay.Data["movement_id"] != waiter.Data["movement_id"] {
		t.Fatalf("waiter's committed result was not replayed: %+v", replay)
	}
	f.assertState(t, 7, 2, 1)
}

func TestP1MySQLIdempotencyFinalizeRollback(t *testing.T) {
	f := newMySQLStockRequests(t)
	db := f.server.DB
	table := db.NamingStrategy.TableName("IdempotencyRecord")
	trigger := table + "_reject_final"
	// Reject the SQL result write after the stock, movement and audit statements succeeded.
	if err := db.Exec("CREATE TRIGGER " + trigger + " BEFORE UPDATE ON " + table + " FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'test result write failure'").Error; err != nil {
		t.Fatal(err)
	}
	if failure := f.increase(2); failure.Code != common.CodeInternal {
		t.Fatalf("expected finalization failure, got %+v", failure)
	}
	f.assertState(t, 5, 1, 0)
	if err := db.Exec("DROP TRIGGER " + trigger).Error; err != nil {
		t.Fatal(err)
	}
	first, replay := f.increase(2), f.increase(2)
	if first.Code != common.CodeOK || replay.Code != common.CodeOK || replay.Data["idempotent"] != true || first.Data["movement_id"] == nil || first.Data["movement_id"] != replay.Data["movement_id"] {
		t.Fatalf("retry=%+v replay=%+v", first, replay)
	}
	f.assertState(t, 7, 2, 1)
}
