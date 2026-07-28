# F-15 Atomic Idempotency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Commit each protected business mutation and its successful idempotency replay record atomically, while preserving retryable rollback semantics and rejecting a different request hash with HTTP 409 / `10011`.

**Architecture:** The existing unique index on `(idem_key, operator_id, path)` is the serialization point. A wrapper-owned GORM transaction inserts an uncommitted JSON `null` claim, passes the same `*gorm.DB` to the business callback, writes a non-nil successful JSON object into that row, and commits both units together. MySQL startup verifies that every table participating in these transactions uses InnoDB; isolated MySQL 8.4 acceptance exercises real unique-index contention.

**Task 4 approved revision (2026-07-28):** The historical four-connection SQLite concurrency fixture uses a file-backed database under `t.TempDir()` with private-cache behavior, `_pragma=busy_timeout(5000)`, and `_txlock=immediate`. The earlier shared-memory `_txlock=immediate` proposal was rejected by a 19/20 failure reproduction of `SQLITE_LOCKED_SHAREDCACHE` at transaction start; the file-backed diagnostic passed 20/20 and 50/50. This is test-only; MySQL and application DSNs remain unchanged.

**Task 6 review revision (2026-07-28):** The transferable source package is
generated locally from immutable `HEAD` objects with `git ls-tree` and
`git archive`, then bound by a NUL path list, per-file SHA-256 manifest, archive
hashes, and one out-of-band package-manifest digest. Normal remote execution
consumes and verifies that package without `.git`. Failed runs retain only
validated classified checkpoints, authorized production snapshots, leak-scan
status, and hashes; raw output remains temporary and is deleted.

**Task 8 first-run correction (2026-07-28):** The authorized `ce8787a` package
passed source and MySQL 8.4 gates, then failed at `test_metadata` because the
metadata-only tools container wrote the host-owned mode-0700 workspace as root.
Commit `f46bb3c` adds a behavioral UID:GID/`HOME=/tmp` contract and passed
independent review plus fresh focused/full/static gates. This does not approve
the test-server result; a fresh package and a separately authorized rerun are
required.

**Local status (2026-07-28):** Code-side implemented and locally verified;
isolated MySQL 8.4 test-server review pending; production unchanged.

**Tech Stack:** Go 1.22, Gin 1.10, GORM 1.30 with `TranslateError`, `glebarez/sqlite`, MySQL 8.4, Docker Compose, Bash.

## Global Constraints

- Scope remains exactly `(Idempotency-Key, operator_id, path)` and request hashing remains SHA-256 of the existing JSON serialization.
- Replay only committed successful terminal results. Do not persist failed or pending terminal states.
- A callback, serialization, operation-log, or replay-finalization failure before commit rolls back the claim and all transactional writes.
- After a committed result, the same hash replays with `idempotent=true`; a different hash returns HTTP 409 / `10011` without running the callback.
- A commit-time connection error has an unknown outcome; a later same-key retry must converge to replay if committed or execution if rolled back.
- Do not add TTL, lease, cleanup worker, `PENDING`, `SUCCEEDED`, Redis, or a process-local mutex.
- Do not edit `0001_init` and do not add an F-15 migration. F-12 retains migration number `0010`.
- MySQL correctness requires `idempotency_records`, `buyer_intents`, `products`, `orders`, `order_events`, and `operation_logs` to use InnoDB.
- Do not log payloads, stored responses, credentials, tokens, buyer contact fields, database connection values, or raw test identifiers.
- Do not read or modify production SQL, data, logs, environment, mounts, configuration, services, migrations, or deployments. Production checks remain limited to the three authorized container name/ID/state/restart-count snapshots.
- Do not read, stage, or commit `.tmp/`, `backend/app.db`, `.env`, secrets, databases, uploads, backups, evidence directories, caches, `node_modules`, `.git`, or the protected review documents.

## File Responsibility Map

- `backend/internal/app/idempotency.go`: claim transaction, terminal serialization, replay lookup, and claim-conflict sentinel.
- `backend/internal/app/idempotency_test.go`: focused RED/GREEN coverage for wrapper semantics and failure injection.
- `backend/internal/app/idempotency_schema.go`: MySQL transactional-table engine inspection and fail-closed validation.
- `backend/internal/app/idempotency_schema_test.go`: pure engine-state matrix and SQLite bypass coverage.
- `backend/internal/app/server.go`: startup engine gate and operation-log error propagation.
- `backend/internal/app/merchant_intent_handlers.go`: two merchant intent callbacks use the wrapper transaction.
- `backend/internal/app/product_handlers.go`: product status callback uses the wrapper transaction.
- `backend/internal/app/order_handlers.go`: order action callback and both operation logs use the wrapper transaction.
- `backend/internal/app/buyer_handlers.go`: rate limits, product lock, open-intent check, and insert run only for first execution using the wrapper transaction.
- `backend/tests/idempotency_handlers_test.go`: HTTP-level rollback/replay compatibility across all five call sites.
- `backend/tests/idempotency_mysql_test.go`: opt-in MySQL 8.4 contention, failure, retry, and engine evidence.
- `backend/tests/idempotency_acceptance_contract_test.go`: controlled behavioral tests for source-list and fail-before-Docker safety.
- `deploy/acceptance/idempotency-atomicity-smoke.sh`: isolated Compose run, committed-source manifest, sanitized evidence, and production snapshot equality.
- `Makefile`: guarded F-15 acceptance target.
- `deploy/acceptance/README.md`: exact local and server-side isolated execution contract.
- `docs/release-readiness.md`: separate code-side, test-server, and production status.
- `docs/superpowers/plans/2026-07-28-idempotency-atomicity.md`: RED/GREEN command and commit traceability.

---

### Task 1: Implement The Atomic Wrapper Without Breaking Intermediate Commits

**Files:**
- Modify: `backend/internal/app/idempotency.go`
- Create: `backend/internal/app/idempotency_test.go`
- Modify mechanically for the temporary legacy name: `backend/internal/app/merchant_intent_handlers.go`
- Modify mechanically for the temporary legacy name: `backend/internal/app/product_handlers.go`
- Modify mechanically for the temporary legacy name: `backend/internal/app/order_handlers.go`
- Modify mechanically for the temporary legacy name: `backend/internal/app/buyer_handlers.go`

**Interfaces:**
- Produces: `type idempotentOperation func(tx *gorm.DB) (map[string]interface{}, error)`.
- Produces: `type idempotencyScope struct { Key string; OperatorID uint64; Path string; RequestHash string }`.
- Produces: `runWithIdempotency(c *gin.Context, payload interface{}, fn idempotentOperation) (map[string]interface{}, error)`.
- Produces: `func (s *Server) runIdempotentTransaction(fn idempotentOperation) (map[string]interface{}, error)`.
- Produces: `func (s *Server) replayIdempotencyResult(scope idempotencyScope) (map[string]interface{}, error)`.
- Temporary: `runWithLegacyIdempotency` contains the old implementation until Tasks 3 and 4 migrate all five call sites; it must be deleted in Task 4.

- [x] **Step 1: Add focused tests for the final callback contract**

Create an isolated shared-memory SQLite database with `TranslateError: true`, migrate `model.IdempotencyRecord` and a test-only effect model, and invoke the helper through a Gin route so `c.FullPath()` is populated:

```go
type idempotencyTestEffect struct {
    ID    uint64 `gorm:"primaryKey"`
    Scope string `gorm:"size:64;uniqueIndex"`
}

type idempotencyTestDuplicate struct {
    ID   uint64 `gorm:"primaryKey"`
    Name string `gorm:"size:64;uniqueIndex"`
}

func invokeIdempotencyTest(
    t *testing.T,
    server *Server,
    key string,
    payload interface{},
    fn idempotentOperation,
) (map[string]interface{}, error) {
    t.Helper()
    gin.SetMode(gin.TestMode)
    router := gin.New()
    var data map[string]interface{}
    var runErr error
    router.POST("/idempotency/test", func(c *gin.Context) {
        common.SetActor(c, common.Actor{UserID: 42, UserType: model.UserTypeMerchant})
        data, runErr = server.runWithIdempotency(c, payload, fn)
    })
    req := httptest.NewRequest(http.MethodPost, "/idempotency/test", nil)
    req.Header.Set("Idempotency-Key", key)
    router.ServeHTTP(httptest.NewRecorder(), req)
    return data, runErr
}
```

Add these tests, each asserting both returned error semantics and database state:

```go
func TestRunWithIdempotencyCommitsAndReplaysSuccessfulObject(t *testing.T)
func TestRunWithIdempotencyRejectsDifferentHashAfterSuccess(t *testing.T)
func TestRunWithIdempotencyRollsBackCallbackAndClaimThenAllowsRetry(t *testing.T)
func TestRunWithIdempotencyAcceptsEmptyObjectAndRejectsNilObject(t *testing.T)
func TestRunWithIdempotencyRollsBackUnsupportedResponseValue(t *testing.T)
func TestRunWithIdempotencyRollsBackWhenTerminalUpdateFails(t *testing.T)
func TestRunWithIdempotencyDoesNotTreatBusinessDuplicateAsClaimConflict(t *testing.T)
func TestRunWithIdempotencyWithoutKeyIsTransactional(t *testing.T)
func TestRunWithIdempotencyFailsClosedForNullOrCorruptStoredResponse(t *testing.T)
func TestRunWithIdempotencyRetryConvergesForBothCommitOutcomes(t *testing.T)
```

For terminal-update failure, register a test-only GORM callback that calls `tx.AddError(errForcedFinalize)` only when `tx.Statement.Table == "idempotency_records"`, then assert the effect and claim counts remain zero.

- [x] **Step 2: Run the focused tests and capture RED**

Run:

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./internal/app -run '^TestRunWithIdempotency' -count=1 -v
```

Expected: compilation fails because the old callback is `func()`, or behavioral assertions fail because the old read/action/insert sequence commits effects before replay persistence.

- [x] **Step 3: Preserve a green intermediate history while introducing the final helper**

Rename the current function to `runWithLegacyIdempotency` and mechanically change the five existing calls to that temporary name. Then implement the final helper in the same file:

```go
var errIdempotencyClaimConflict = errors.New("idempotency claim conflict")

type idempotentOperation func(tx *gorm.DB) (map[string]interface{}, error)

func (s *Server) runWithIdempotency(
    c *gin.Context,
    payload interface{},
    fn idempotentOperation,
) (map[string]interface{}, error) {
    key := c.GetHeader("Idempotency-Key")
    if key == "" {
        return s.runIdempotentTransaction(fn)
    }
    actor, ok := common.GetActor(c)
    if !ok {
        return nil, common.ErrInternal
    }
    raw, err := json.Marshal(payload)
    if err != nil {
        return nil, common.ErrInternal
    }
    scope := idempotencyScope{
        Key: key, OperatorID: actor.UserID, Path: c.FullPath(),
        RequestHash: common.SHA256(string(raw)),
    }

    var data map[string]interface{}
    err = s.DB.Transaction(func(tx *gorm.DB) error {
        record := model.IdempotencyRecord{
            IdemKey: scope.Key, OperatorID: scope.OperatorID, Path: scope.Path,
            RequestHash: scope.RequestHash, ResultCode: common.CodeOK,
            ResponseRaw: datatypes.JSON([]byte("null")),
        }
        if createErr := tx.Create(&record).Error; createErr != nil {
            if errors.Is(createErr, gorm.ErrDuplicatedKey) {
                return errIdempotencyClaimConflict
            }
            return common.ErrInternal
        }
        result, runErr := fn(tx)
        if runErr != nil {
            return runErr
        }
        if result == nil {
            return common.ErrInternal
        }
        encoded, marshalErr := json.Marshal(result)
        if marshalErr != nil {
            return common.ErrInternal
        }
        update := tx.Model(&model.IdempotencyRecord{}).
            Where("id = ?", record.ID).
            Updates(map[string]interface{}{
                "result_code": common.CodeOK,
                "response_raw": datatypes.JSON(encoded),
            })
        if update.Error != nil || update.RowsAffected != 1 {
            return common.ErrInternal
        }
        data = result
        return nil
    })
    if errors.Is(err, errIdempotencyClaimConflict) {
        return s.replayIdempotencyResult(scope)
    }
    if err != nil {
        return nil, err
    }
    return data, nil
}
```

`runIdempotentTransaction` must reject nil results inside its transaction. `replayIdempotencyResult` must query by all three scope columns, compare the hash, require `result_code == common.CodeOK`, unmarshal a non-nil JSON object, copy/set `idempotent=true`, and return `common.ErrInternal` for missing/null/corrupt/non-success records.

- [x] **Step 4: Run focused and package tests for GREEN**

Run:

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./internal/app -run '^TestRunWithIdempotency' -count=1 -v
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./internal/app ./tests -count=1
```

Expected: all focused tests and both packages pass while the production handlers still use the explicitly named legacy path.

- [x] **Step 5: Commit the atomic core**

```bash
git add backend/internal/app/idempotency.go backend/internal/app/idempotency_test.go \
  backend/internal/app/merchant_intent_handlers.go backend/internal/app/product_handlers.go \
  backend/internal/app/order_handlers.go backend/internal/app/buyer_handlers.go
git commit -m "fix(idempotency): add atomic transaction wrapper"
```

---

### Task 2: Fail Closed When MySQL Transaction Tables Are Not InnoDB

**Files:**
- Create: `backend/internal/app/idempotency_schema.go`
- Create: `backend/internal/app/idempotency_schema_test.go`
- Modify: `backend/internal/app/server.go`

**Interfaces:**
- Produces: `verifyIdempotencyTransactionalTables(db *gorm.DB) error`.
- Produces: `validateIdempotencyTableEngines(rows []mysqlTableEngine) error` for a deterministic unit-test matrix.

- [x] **Step 1: Write the engine-state RED tests**

Use the exact six-table set and test canonical, reordered, lowercase engine, missing table, duplicate table, extra/unknown table, empty engine, and MyISAM cases:

```go
func TestValidateIdempotencyTableEnginesAcceptsExactInnoDBSet(t *testing.T)
func TestValidateIdempotencyTableEnginesRejectsMissingDuplicateAndNonInnoDB(t *testing.T)
func TestVerifyIdempotencyTransactionalTablesSkipsSQLite(t *testing.T)
```

The rejected cases must assert only the stable message `idempotency transactional tables are missing or drifted`, not driver text.

- [x] **Step 2: Run RED**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./internal/app -run 'IdempotencyTableEngine|IdempotencyTransactionalTables' -count=1 -v
```

Expected: compilation fails because the validator does not exist.

- [x] **Step 3: Implement the MySQL-only startup gate**

```go
var idempotencyTransactionalTables = []string{
    "buyer_intents",
    "idempotency_records",
    "operation_logs",
    "order_events",
    "orders",
    "products",
}

type mysqlTableEngine struct {
    TableName string `gorm:"column:table_name"`
    Engine    string `gorm:"column:engine"`
}

func verifyIdempotencyTransactionalTables(db *gorm.DB) error {
    if db.Dialector.Name() != "mysql" {
        return nil
    }
    var rows []mysqlTableEngine
    if err := db.Raw(`
        SELECT table_name, engine
        FROM information_schema.tables
        WHERE table_schema = DATABASE() AND table_name IN ?
        ORDER BY table_name`, idempotencyTransactionalTables).Scan(&rows).Error; err != nil {
        return fmt.Errorf("inspect idempotency transactional tables: %w", err)
    }
    return validateIdempotencyTableEngines(rows)
}
```

The pure validator must require each exact table once and `strings.EqualFold(engine, "InnoDB")`. Call it in `NewServer` after migration and `verifyBuyerIntentOpenUniqueness`, before seeding or route registration:

```go
if err := verifyIdempotencyTransactionalTables(db); err != nil {
    return nil, err
}
```

- [x] **Step 4: Run GREEN and startup regressions**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./internal/app -run 'IdempotencyTableEngine|IdempotencyTransactionalTables|NewServer' -count=1 -v
```

Expected: validator tests pass; SQLite server tests remain unchanged because the MySQL query is skipped.

- [x] **Step 5: Commit the storage prerequisite**

```bash
git add backend/internal/app/idempotency_schema.go \
  backend/internal/app/idempotency_schema_test.go backend/internal/app/server.go
git commit -m "fix(idempotency): require transactional MySQL tables"
```

---

### Task 3: Convert Merchant Intent, Product, And Order Writes

**Files:**
- Modify: `backend/internal/app/server.go`
- Modify: `backend/internal/app/merchant_intent_handlers.go`
- Modify: `backend/internal/app/product_handlers.go`
- Modify: `backend/internal/app/order_handlers.go`
- Create: `backend/tests/idempotency_handlers_test.go`

**Interfaces:**
- Changes: `writeOperationLog(c *gin.Context, tx *gorm.DB, resourceType string, resourceID uint64, action string, fromStatus, toStatus *string, code int, merchantID *uint64, detail map[string]interface{}) error` returns `insertOperationLog` failure.
- Consumes: `runWithIdempotency(c *gin.Context, payload interface{}, fn idempotentOperation) (map[string]interface{}, error)` from Task 1.

- [x] **Step 1: Write HTTP-level RED tests for four protected transitions**

Add table-driven subtests that register narrowly scoped GORM callbacks and always remove them with `t.Cleanup`:

```go
func failGORMTableOperation(
    t *testing.T,
    db *gorm.DB,
    operation string,
    table string,
    forced error,
)

func TestIdempotentMerchantAndOrderTransitionsRollbackWhenTerminalWriteFails(t *testing.T)
func TestIdempotentMerchantAndOrderTransitionsRollbackWhenOperationLogFails(t *testing.T)
func TestIdempotentMerchantAndOrderTransitionsPreserveSuccessPayloads(t *testing.T)
```

Cover exactly:

- merchant intent `NEW -> CONTACTED`;
- merchant intent `CONTACTED -> CLOSED`;
- product `DRAFT -> ON_SHELF`;
- order `CREATED -> COMPLETED` (including product inventory, order event, and both operation logs).

For terminal failure, intercept only the `idempotency_records` update. For log failure, intercept only `operation_logs` create. After each 500 response, assert the domain row, inventory/reservation, event count, operation-log count, and idempotency-record count match the before snapshot. Remove the callback and retry the same key; assert one successful mutation and one terminal record.

- [x] **Step 2: Run RED**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^TestIdempotentMerchantAndOrderTransitions' -count=1 -v
```

Expected: old legacy handlers either commit the domain mutation despite terminal failure or ignore the operation-log error.

- [x] **Step 3: Return operation-log errors**

```go
func (s *Server) writeOperationLog(
    c *gin.Context,
    tx *gorm.DB,
    resourceType string,
    resourceID uint64,
    action string,
    fromStatus, toStatus *string,
    code int,
    merchantID *uint64,
    detail map[string]interface{},
) error {
    logItem := s.buildOperationLog(
        c, resourceType, resourceID, action,
        fromStatus, toStatus, code, merchantID, detail,
    )
    return s.insertOperationLog(tx, &logItem)
}
```

Existing non-F-15 call statements may discard this returned error as permitted by Go. The four protected callbacks must explicitly return it.

- [x] **Step 4: Flatten the four callbacks onto the supplied transaction**

Replace each legacy call and nested `s.DB.Transaction` with this shape:

```go
data, err := s.runWithIdempotency(c, payload, func(tx *gorm.DB) (map[string]interface{}, error) {
    result := map[string]interface{}{}
    // Existing locks, compare-and-set updates, inventory writes, events,
    // and response construction all use tx directly.
    if err := s.writeOperationLog(c, tx, resourceType, resourceID, action,
        fromStatus, toStatus, common.CodeOK, &actor.MerchantID, detail); err != nil {
        return nil, err
    }
    return result, nil
})
```

For order actions, propagate each of the two log insert errors separately. Do not reorder inventory, order update, event creation, or response-field construction beyond removing the nested transaction.

- [x] **Step 5: Run GREEN and focused domain regressions**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^(TestIdempotentMerchantAndOrderTransitions|TestIntegrationFlow|TestBuyerIntentCreateConflictAndMerchantStatusFlow|TestMultiStock)' -count=1 -v
```

Expected: forced failures roll back completely; retry succeeds once; response fields and existing state transitions pass.

- [x] **Step 6: Commit the four converted writes**

```bash
git add backend/internal/app/server.go backend/internal/app/merchant_intent_handlers.go \
  backend/internal/app/product_handlers.go backend/internal/app/order_handlers.go \
  backend/tests/idempotency_handlers_test.go
git commit -m "fix(idempotency): share transactions across merchant writes"
```

---

### Task 4: Convert Buyer Intent Creation And Remove The Legacy Wrapper

**Files:**
- Modify: `backend/internal/app/buyer_handlers.go`
- Modify: `backend/internal/app/idempotency.go`
- Modify: `backend/tests/idempotency_handlers_test.go`
- Modify: `backend/tests/buyer_flow_test.go`

**Interfaces:**
- Consumes: `findOpenBuyerIntent(tx, buyerID, productID)` and `classifyBuyerIntentCreateError(tx, err, buyerID, productID)`.
- Final state: `runWithLegacyIdempotency` no longer exists or has callers.

- [x] **Step 1: Write buyer replay and rollback RED tests**

```go
func TestBuyerIntentIdempotencyReplayBypassesChangedProductAndRateLimit(t *testing.T)
func TestBuyerIntentIdempotencyTerminalFailureRollsBackAndRetrySucceeds(t *testing.T)
func TestBuyerIntentIdempotencyDifferentHashReturns10011(t *testing.T)
```

The first test creates with a key, changes the product status directly after commit, then replays the exact request at least six times. Every replay must return the stored intent fields with `idempotent=true`, proving it does not run product validation or consume the 5/minute buyer rate limit. The terminal-failure test intercepts `idempotency_records` update and asserts no buyer intent or claim survives; after removing the callback, the same request/key succeeds exactly once. The mismatch test changes a contact field under the same key and asserts HTTP 409 / `10011` with one intent row.

- [x] **Step 2: Run RED**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^TestBuyerIntentIdempotency' -count=1 -v
```

Expected: the old handler validates/rate-limits before replay and its direct `s.DB` writes survive replay-finalization failure.

- [x] **Step 3: Move first-execution work into the supplied transaction**

Keep authentication, actor type, JSON binding, and contact-field validation before the wrapper. Remove the transaction-external product query and move these operations into the callback in this order:

```go
data, err := s.runWithIdempotency(c, payload, func(tx *gorm.DB) (map[string]interface{}, error) {
    if err := s.checkRateLimit("buyer:intent:min", fmt.Sprintf("%d", actor.UserID), 5, time.Minute); err != nil {
        return nil, err
    }
    if err := s.checkRateLimit("buyer:intent:day", fmt.Sprintf("%d", actor.UserID), 20, 24*time.Hour); err != nil {
        return nil, err
    }
    if deviceID != "" {
        if err := s.checkRateLimit("buyer:intent:device_day", deviceID, 30, 24*time.Hour); err != nil {
            return nil, err
        }
    }

    var product model.Product
    if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
        Where("id = ?", req.ProductID).First(&product).Error; err != nil {
        return nil, s.dbError(err)
    }
    if product.Status != model.ProductOnShelf {
        return nil, common.ErrInvalidTransition
    }
    found, err := findOpenBuyerIntent(tx, actor.UserID, req.ProductID)
    if err != nil {
        return nil, err
    }
    if found {
        return nil, common.ErrConflict
    }
    intent := model.BuyerIntent{
        IntentNo:       common.BuildBizNo("I"),
        BuyerID:        actor.UserID,
        SourceDeviceID: &deviceID,
        ProductID:      req.ProductID,
        MerchantID:     product.MerchantID,
        Status:         model.IntentNew,
        IsOpen:         true,
        ContactName:    req.ContactName,
        ContactPhone:   req.ContactPhone,
        ContactWechat:  req.ContactWechat,
        Message:        req.Message,
    }
    if err := tx.Create(&intent).Error; err != nil {
        return nil, classifyBuyerIntentCreateError(tx, err, actor.UserID, req.ProductID)
    }
    return map[string]interface{}{
        "intent_id": intent.ID, "intent_no": intent.IntentNo,
        "status": intent.Status, "created_at": intent.CreatedAt,
    }, nil
})
```

- [x] **Step 4: Delete the temporary legacy helper and prove all five callers migrated**

```bash
rg -n 'runWithLegacyIdempotency|func\(\) \(map\[string\]interface\{\}, error\)' \
  backend/internal/app/idempotency.go backend/internal/app/merchant_intent_handlers.go \
  backend/internal/app/product_handlers.go backend/internal/app/order_handlers.go \
  backend/internal/app/buyer_handlers.go
```

Expected: no matches. Delete `runWithLegacyIdempotency` from `idempotency.go`.

- [x] **Step 5: Run GREEN and buyer concurrency regressions**

The pre-fix historical concurrency regression returned HTTP 500 / `20001` in
4 of 20 repeated runs:

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^TestBuyerIntentConcurrentCreateHasOneWinner$' -count=20 -v
```

Root cause of the original fixture: the shared-memory SQLite DSN uses deferred
transactions; both requests can read before either writes, SQLite ignores
`FOR UPDATE`, and one transaction can fail its write upgrade with
`SQLITE_BUSY`. Retaining shared cache and adding `_txlock=immediate` is also
invalid: a diagnostic run failed 19/20 times at transaction start with
`SQLITE_LOCKED_SHAREDCACHE` (262), which `busy_timeout` does not retry.

Change only this test fixture to import `path/filepath` and use a file-backed
private-cache database under `t.TempDir()`:

```go
cfg.DBDSN = "file:" + filepath.Join(t.TempDir(), "buyer-intent.db") +
    "?_pragma=busy_timeout(5000)&_txlock=immediate"
```

Keep `SetMaxOpenConns(4)`, `SetMaxIdleConns(2)`, the five-second busy timeout,
and the strict expected one-success/one-conflict result. Do not add application
retries, a process-local mutex, a global SQLite DSN rewrite, shared cache, or
weaker assertions. The diagnostic file-backed candidate passed 20/20 and
50/50; the committed implementation must produce a fresh 20-run result.

First require 20 consecutive GREEN runs:

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^TestBuyerIntentConcurrentCreateHasOneWinner$' -count=20 -v
```

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^(TestBuyerIntentIdempotency|TestBuyerIntentCreateConflictAndMerchantStatusFlow|TestBuyerIntentConcurrentCreateHasOneWinner|TestBuyerIntentStateDriftFailsClosed)' -count=1 -v
```

Expected: replay bypasses changed product/rate limits; failure leaves no row; historical F-11 uniqueness/state tests remain green.

- [x] **Step 6: Commit buyer conversion and legacy removal**

```bash
git add backend/internal/app/buyer_handlers.go backend/internal/app/idempotency.go \
  backend/tests/idempotency_handlers_test.go backend/tests/buyer_flow_test.go
git commit -m "fix(idempotency): atomically create buyer intents"
```

---

### Task 5: Add Real MySQL 8.4 Contention Coverage

**Files:**
- Create: `backend/tests/idempotency_mysql_test.go`

**Interfaces:**
- Test gate: `IDEMPOTENCY_MYSQL_TEST=1`.
- Environment contract: `DB_DSN` must parse as TCP `mysql:3306/second_hand_market_acceptance`.
- Produces: `requireIsolatedIdempotencyMySQLDSN(t *testing.T) string`, which skips unless opted in and returns the validated DSN without logging it.
- Produces: `newIdempotencyMySQLServer(t *testing.T, dsn string) *app.Server`, which configures the isolated server and bounded connection pool.
- Produces: `assertIdempotencyMySQLTableEngines(t *testing.T, db *gorm.DB)`, which requires the exact six InnoDB rows without printing identifiers.
- Produces: `runIdempotencySameHashMySQL(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture)`.
- Produces: `runIdempotencyDifferentHashMySQL(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture)`.
- Produces: `runIdempotencyFailureReleaseMySQL(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture)`.
- Produces: `runIdempotencyTerminalRollbackMySQL(t *testing.T, srv *app.Server, fixture buyerIntentMySQLFixture)`.

- [x] **Step 1: Write the opt-in MySQL acceptance test**

```go
func TestIdempotencyMySQLAcceptance(t *testing.T) {
    dsn := requireIsolatedIdempotencyMySQLDSN(t)
    srv := newIdempotencyMySQLServer(t, dsn)
    assertIdempotencyMySQLTableEngines(t, srv.DB)
    fixture := createBuyerIntentMySQLFixture(t, srv)
    t.Cleanup(func() { cleanupBuyerIntentMySQLFixture(srv, fixture) })

    t.Run("same hash executes one product transition", func(t *testing.T) {
        runIdempotencySameHashMySQL(t, srv, fixture)
    })
    t.Run("different hash returns 10011", func(t *testing.T) {
        runIdempotencyDifferentHashMySQL(t, srv, fixture)
    })
    t.Run("failed callback releases claim for waiter", func(t *testing.T) {
        runIdempotencyFailureReleaseMySQL(t, srv, fixture)
    })
    t.Run("terminal failure rolls back business mutation", func(t *testing.T) {
        runIdempotencyTerminalRollbackMySQL(t, srv, fixture)
    })
}
```

`runIdempotencySameHashMySQL` creates one draft product for the fixture merchant, sends two synchronized `on-shelf` requests with the same key, and asserts one state transition/log, identical stored business fields, one original `idempotent=false`, and one replay `idempotent=true`.

It registers cleanup for the additional product, its images, logs, and idempotency row inside the helper so the shared F-11 fixture cleanup remains complete.

`runIdempotencyDifferentHashMySQL` creates one open buyer intent, sends two synchronized close requests on the same path/key with `reason=NO_RESPONSE` but different `merchant_note` values, and asserts one success, one HTTP 409 / `10011`, one close transition, and one stored hash.

`runIdempotencyFailureReleaseMySQL` registers an update callback guarded by an `atomic.Int32` that fails only the first matching buyer-intent transition after the claim is inserted. Two same-key requests start together; the first rolls back and the waiter or subsequent retry succeeds. Assert one final transition and one terminal claim.

`runIdempotencyTerminalRollbackMySQL` fails only the first `idempotency_records` finalization update, asserts the domain mutation, log/event, and claim are absent, removes the callback, retries the same key, and asserts one commit.

Use `sync.WaitGroup`, a closed start channel, and `atomic.Int32` only inside test failure callbacks. Do not print keys, payloads, contact fields, user IDs, tokens, or DSN values.

- [x] **Step 2: Prove the test is opt-in locally**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^TestIdempotencyMySQLAcceptance$' -count=1 -v
```

Expected: `SKIP` with the stable isolated-project instruction and no network connection attempt.

- [x] **Step 3: Run all local non-opt-in tests**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./... -count=1
```

Expected: all local tests pass; MySQL F-15 test is skipped unless the exact opt-in flag is set.

- [x] **Step 4: Commit the MySQL gate**

```bash
git add backend/tests/idempotency_mysql_test.go
git commit -m "test(idempotency): cover MySQL transaction contention"
```

---

### Task 6: Build The Isolated Acceptance Project And Behavioral Safety Contract

**Files:**
- Create: `deploy/acceptance/idempotency-atomicity-smoke.sh`
- Create: `backend/tests/idempotency_acceptance_contract_test.go`
- Modify: `Makefile`
- Modify: `deploy/acceptance/README.md`

**Interfaces:**
- Compose project: `secondhand-idempotency-acceptance`.
- Evidence directory: `deploy/acceptance/evidence/idempotency-atomicity`.
- Confirmation: `IDEMPOTENCY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_IDEMPOTENCY_DATA`.
- Engine gate: `ACCEPTANCE_DB_ENGINE=mysql8.4`.
- Source-list mode: `IDEMPOTENCY_SOURCE_LIST_ONLY=1` emits only the NUL-delimited `HEAD` whitelist and performs no Docker or remote action.
- Export mode: `IDEMPOTENCY_SOURCE_EXPORT_DIR=<absent-absolute-directory>` writes the immutable `HEAD` package without Docker or remote action.
- Normal mode consumes `IDEMPOTENCY_SOURCE_PACKAGE_DIR` (default `.idempotency-source`) and requires the authorized `IDEMPOTENCY_SOURCE_PACKAGE_MANIFEST_SHA256` digest; it does not require remote Git metadata.
- Planned remote directory, subject to exact authorization: `/home/yu/services/secondhand-idempotency-acceptance-20260728`.

- [x] **Step 1: Write behavioral safety RED tests**

Run the real script and Make target with controlled inputs. Do not assert that source files contain particular strings:

```go
func TestIdempotencyAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T)
func TestIdempotencyAcceptanceSourceExportUsesImmutableHEAD(t *testing.T)
func TestIdempotencyAcceptanceMetadataFreePackageRefusesOrProgressesBeforeDocker(t *testing.T)
func TestIdempotencyAcceptanceRetainsSanitizedFailureEvidence(t *testing.T)
func TestIdempotencyAcceptanceRefusesBeforeDockerWithoutConfirmation(t *testing.T)
func TestIdempotencyAcceptanceMakeTargetRefusesBeforeDockerWithoutConfirmation(t *testing.T)
```

The source-list test runs the script with only `IDEMPOTENCY_SOURCE_LIST_ONLY=1`, parses its NUL-delimited stdout, and asserts literal required paths are present, every entry exists in `HEAD`, staged-only paths are absent, and no entry is `.env`, a secret, database, upload, evidence, backup, `.git`, cache, `node_modules`, `backend/app.db`, `.tmp`, or a protected review document. The exporter test proves dirty and staged working-tree bytes cannot enter the package. Metadata-free and failure-evidence tests execute the real script with controlled fake Docker behavior rather than asserting source text.

The two refusal tests prepend a temporary fake `docker` executable that writes a marker if invoked. Run the real script or Make target with confirmation variables absent; assert a nonzero exit, the stable missing-confirmation message, and absence of the Docker marker. This proves the guard executes before Docker rather than merely checking source text.

- [x] **Step 2: Run RED**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^TestIdempotencyAcceptance' -count=1 -v
```

Expected: the source-list test fails because the script does not exist, and the refusal tests fail because the Make target/script behavior is absent.

- [x] **Step 3: Implement the guarded source manifest and one-shot smoke**

The script must:

1. Exit before Docker access unless both confirmation values and the exact project name match.
2. Refuse any existing project container, volume, network, or evidence directory.
3. Produce the whitelist from `git ls-tree HEAD` for `Makefile`, `backend/Dockerfile`, `backend/go.mod`, `backend/go.sum`, committed `.go` files under `backend/`, committed migrations, and committed acceptance files, excluding every protected path.
4. In `IDEMPOTENCY_SOURCE_LIST_ONLY=1` mode, print only that list and exit.
5. Export the actual listed `HEAD` bytes with `git archive`; bind the NUL list, file manifest, and archive with SHA-256, without reading working-tree source bytes.
6. In normal mode, verify the out-of-band package-manifest digest, every package checksum, every received regular file, and the reconstructed context without requiring `.git`.
7. Snapshot only `secondhand-market-api`, `secondhand-market-web`, and `secondhand-market-mysql` as `name|ID|state|restart-count`.
8. Start only the fixed Compose project's MySQL 8.4 service, apply `0001` through `0009`, and run `TestIdempotencyMySQLAcceptance` with both AutoMigrate values.
9. Run `go test ./... -count=1`, `go test -race ./internal/app ./tests -count=1`, and `go vet ./...` with the MySQL opt-in disabled.
10. Compare before/after production snapshots byte-for-byte.
11. On success, retain only classified PASS lines, counts, authorized snapshots, leak-scan status, and hashes.
12. On failure after the first authorized snapshot, stop isolated containers and retain only validated classified checkpoints/snapshots/hashes; if sanitization cannot be proven, publish only a hardcoded sanitization-failure classification. Never retain raw command output.

Add the Make target:

```make
acceptance-idempotency-smoke:
	@test "$${IDEMPOTENCY_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_IDEMPOTENCY_DATA" || { echo "set IDEMPOTENCY_ACCEPTANCE_CONFIRM for isolated idempotency tests" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	./deploy/acceptance/idempotency-atomicity-smoke.sh
```

- [x] **Step 4: Run contract GREEN, shell syntax, and source-list leak checks**

```bash
bash -n deploy/acceptance/idempotency-atomicity-smoke.sh
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^TestIdempotencyAcceptance' -count=1 -v
cd ..
git add Makefile deploy/acceptance/idempotency-atomicity-smoke.sh \
  deploy/acceptance/README.md backend/tests/idempotency_acceptance_contract_test.go
IDEMPOTENCY_SOURCE_LIST_ONLY=1 \
  ./deploy/acceptance/idempotency-atomicity-smoke.sh > /tmp/f15-source-list.z
```

Inspect the NUL-delimited list only for forbidden path names, verify every entry
with `git cat-file -e HEAD:<path>`, and compare it byte-for-byte with export
mode's list. Do not read protected files or any evidence content. Post-commit
verification exported 127 `HEAD` paths, found zero forbidden paths, verified
every `HEAD:<path>` object, and passed all three package checksums before any
Docker or remote action.

- [x] **Step 5: Document exact execution and commit**

Document the local command, fixed remote directory to be separately authorized, fixed project name, source-list transfer rule, SHA-256 manifest comparison, one-run rule, evidence classification, cleanup boundary, and production restrictions.

```bash
git add Makefile deploy/acceptance/idempotency-atomicity-smoke.sh \
  deploy/acceptance/README.md backend/tests/idempotency_acceptance_contract_test.go
git commit -m "test(idempotency): add isolated MySQL acceptance"
```

---

### Task 7: Run Local Quality Gates And Record Code-Side Closure

**Files:**
- Modify: `docs/release-readiness.md`
- Modify: `docs/superpowers/specs/2026-07-28-idempotency-atomicity-design.md`
- Modify: `docs/superpowers/plans/2026-07-28-idempotency-atomicity.md`

**Interfaces:**
- Status columns remain: code-side closure, isolated test-server approval, production migration/deployment.

- [x] **Step 1: Format and inspect the complete diff**

```bash
gofmt -w backend/internal/app/idempotency.go \
  backend/internal/app/idempotency_test.go \
  backend/internal/app/idempotency_schema.go \
  backend/internal/app/idempotency_schema_test.go \
  backend/internal/app/server.go \
  backend/internal/app/merchant_intent_handlers.go \
  backend/internal/app/product_handlers.go \
  backend/internal/app/order_handlers.go \
  backend/internal/app/buyer_handlers.go \
  backend/tests/idempotency_handlers_test.go \
  backend/tests/idempotency_mysql_test.go \
  backend/tests/idempotency_acceptance_contract_test.go
git diff --check
```

- [x] **Step 2: Run fresh focused, full, race, and vet gates**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./internal/app ./tests -run 'Idempotency|BuyerIntent|IntegrationFlow|MultiStock' -count=1 -v
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./... -count=1
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test -race ./internal/app ./tests -count=1
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go vet ./...
```

Expected: zero failures. Record exact commands, timestamps, and package summaries; never record payloads or identifiers.

Local gate record, 2026-07-28 14:32-15:04 +0800:

- focused `./internal/app ./tests` selection: exit 0;
- `go vet ./...`: exit 0;
- `go test -race ./internal/app ./tests -count=1`: exit 0 (`internal/app`
  and `tests`, with the final run of the latter completing in 368.402 seconds);
- the first full run was intentionally not accepted because it ran in parallel
  with race and one signal fixture missed its five-second ready-file deadline;
  the exact fixture then passed alone in 2.99 seconds and a fresh serial
  `go test ./... -count=1` passed every package;
- shell syntax, gofmt diff, `git diff --check`, metadata-free package tests, and
  the 127-path `HEAD` export audit all passed.

- [x] **Step 3: Request two-stage code review and resolve every finding**

Run specification compliance review first, then code-quality review. For every finding, reproduce it, add a failing regression test when behavioral, implement the minimum fix, and rerun the relevant focused and full gates. Do not accept reviewer summaries without inspecting the diff and rerunning commands.

Task-level independent reviews approved Tasks 1-5. Task 6 review found the
remote `.git` dependency, index-only provenance, and failure-evidence deletion;
commit `f1b8a93` addressed them and the controller reran focused/full/export
gates. The final specification and quality pass found no open Critical or
Important issue. Its only deferred minor was the second order operation-log
failure branch; commit `15f57dd` added a regression. A temporary mutation that
discarded that error made the test fail with HTTP 200/code 0, and restoring the
real propagation made both operation-log rollback suites pass.

- [x] **Step 4: Update status without overstating server or production completion**

Set the F-15 spec status to `code-side implemented and locally verified; isolated MySQL 8.4 test-server review pending; production unchanged`. Add the same three-way row to `docs/release-readiness.md`. Mark completed plan checkboxes and record commit hashes plus RED/GREEN outputs.

- [x] **Step 5: Commit local closure evidence**

```bash
git add docs/release-readiness.md \
  docs/superpowers/specs/2026-07-28-idempotency-atomicity-design.md \
  docs/superpowers/plans/2026-07-28-idempotency-atomicity.md
git commit -m "docs(idempotency): record local verification"
```

Implementation traceability at local closure:

- Task 1: `a0ef4b3`
- Task 2: `58e697b`
- Task 3: `d18c760`
- Task 4: `f218832`
- accepted F-11 prerequisite merge: `79da573`
- Task 5: `8d838a6`
- Task 6: `017600f`, review fix `f1b8a93`
- final deferred-minor regression: `15f57dd`

---

### Task 8: Run The Separately Authorized MySQL 8.4 Server Gate

**Files:**
- Modify after successful review only: `docs/release-readiness.md`
- Modify after successful review only: `docs/superpowers/specs/2026-07-28-idempotency-atomicity-design.md`
- Modify after successful review only: `docs/superpowers/plans/2026-07-28-idempotency-atomicity.md`

**Interfaces:**
- Requires a separate exact remote-directory/source-transfer authorization unless an existing authorization explicitly names the F-15 directory and Compose project.
- Must not proceed while SSH fails before banner exchange.

- [x] **Step 1: Generate and verify the committed source manifest locally for the first run**

```bash
IDEMPOTENCY_SOURCE_LIST_ONLY=1 \
  ./deploy/acceptance/idempotency-atomicity-smoke.sh > /tmp/f15-source-list.z
f15_export_root="$(mktemp -d)"
IDEMPOTENCY_SOURCE_EXPORT_DIR="$f15_export_root/.idempotency-source" \
  ./deploy/acceptance/idempotency-atomicity-smoke.sh
cmp /tmp/f15-source-list.z \
  "$f15_export_root/.idempotency-source/source-files.z"
(
  cd "$f15_export_root/.idempotency-source"
  sha256sum -c package-sha256.txt
)
sha256sum "$f15_export_root/.idempotency-source/package-sha256.txt"
```

Prove every source-list item exists as `HEAD:<path>`, count files, reject every
forbidden path name, require the three package checksum lines to pass, and
record the final package-manifest digest out of band without reading protected
content.

The first-run package at `ce8787a` contained 127 paths with zero forbidden
matches. Its out-of-band `package-sha256.txt` digest was
`c42edc4d72210d5551d7261c93da9b90e6bf28509e7add0b88b17bd1fdfdcbe3`.
A new package must be generated from the corrective final `HEAD` before rerun.

- [x] **Step 2: Obtain one consolidated authorization for the first run**

The authorization must name `/home/yu/services/secondhand-idempotency-acceptance-20260728`, the fixed Compose project `secondhand-idempotency-acceptance`, transfer of only the immutable `HEAD` whitelist package (`source-files.z`, `source-sha256.txt`, `source.tar`, and `package-sha256.txt`), remote-only generated secrets, one isolated MySQL 8.4 run, retained sanitized evidence/resources, and only the three fixed production container snapshots. It must preserve every existing prohibition, including no `.git` transfer.

- [x] **Step 3: Run one bounded SSH probe before the first transfer**

```bash
ssh -o BatchMode=yes -o ConnectTimeout=8 -o ConnectionAttempts=1 \
  -o ServerAliveInterval=5 -o ServerAliveCountMax=1 aliyun-server true
```

Expected: exit 0 before any remote action. Banner timeout means stop remote work and continue other local tasks.

- [x] **Step 4: Transfer, verify hashes, generate remote secrets, and run the authorized first attempt once**

Only after authorization and successful SSH: create the exact directory with
mode `0700`, transfer only `.idempotency-source`, compare the out-of-band
package-manifest digest before extraction, verify `package-sha256.txt`, extract
`source.tar`, generate remote `.env` and secrets, then invoke the guarded Make
target exactly once with the authorized digest in
`IDEMPOTENCY_SOURCE_PACKAGE_MANIFEST_SHA256`.

The first attempt reached `test_metadata` and failed before the application
matrix. It was not rerun. The MySQL 8.4 classification passed, the evidence
leak scan passed with zero matches, and the authorized production snapshot was
byte-identical before and after. The retained project resources and root-owned
runtime are documented in
`docs/superpowers/reviews/2026-07-28-idempotency-atomicity-isolated-acceptance-failure.md`.

- [ ] **Step 5: Audit sanitized evidence and update server status**

Read only the new sanitized evidence after the run and leak scan pass. Verify MySQL 8.4, both AutoMigrate modes, concurrency cases, full/race/vet gates, evidence hashes, and production snapshot equality. Update status to test-server approved while leaving production unchanged, then commit:

```bash
git add docs/release-readiness.md \
  docs/superpowers/specs/2026-07-28-idempotency-atomicity-design.md \
  docs/superpowers/plans/2026-07-28-idempotency-atomicity.md
git commit -m "docs(idempotency): record isolated MySQL acceptance"
```

The first run produced valid sanitized failure evidence but did not satisfy
this approval step. Before retrying:

1. export and verify a fresh immutable package from the corrective final
   `HEAD`;
2. obtain exact authorization to remove and recreate only the fixed remote
   directory and this Compose project's resources, including a separate
   decision for the root-owned `/tmp/tmp.AJaD782XNx` runtime;
3. transfer only the four package artifacts, verify the new out-of-band digest,
   regenerate remote-only secrets, and run the fixed Compose project only once;
4. mark test-server approval only after the complete classified matrix and
   evidence hashes pass and the fixed production snapshot is unchanged.

## Plan Self-Review Traceability

| Specification requirement | Implemented by |
| --- | --- |
| Atomic claim, mutation, and successful replay row | Tasks 1, 3, 4 |
| Same-hash replay and different-hash `10011` | Tasks 1, 4, 5 |
| Failure rollback and safe retry | Tasks 1, 3, 4, 5 |
| Unknown commit convergence | Tasks 1 and 5 |
| No persisted failed/pending state or migration | Tasks 1 and 7 |
| Five callback transactions and strict operation logs | Tasks 3 and 4 |
| Buyer product lock and replay-before-rate-limit behavior | Task 4 |
| SQLite deferred/shared-cache locking regression without weakening concurrency assertions | Task 4 approved revision |
| Six-table InnoDB prerequisite | Tasks 2, 5, 6 |
| SQLite focused and MySQL 8.4 contention tests | Tasks 1 through 6 |
| Sanitized evidence and production isolation | Tasks 6 and 8 |
| Separate code/server/production statuses | Tasks 7 and 8 |

The plan contains no migration task, no production write/deploy task, and no operation that reads a protected review document or local private artifact.
