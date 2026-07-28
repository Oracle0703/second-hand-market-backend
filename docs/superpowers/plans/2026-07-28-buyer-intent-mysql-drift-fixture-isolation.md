# F-11 MySQL Drift Fixture Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Isolate the real MySQL drift matrix from rate-limit state consumed by earlier acceptance scenarios.

**Architecture:** Create one dedicated buyer/device inside the drift helper, route all buyer-side drift calls through it, and register complete test-data cleanup. Keep production rate limiting and drift validation unchanged.

**Tech Stack:** Go 1.22, Gin 1.10, GORM 1.30, MySQL 8.4, Docker Compose.

## Global Constraints

- Modify only `backend/tests/buyer_intent_mysql_test.go` plus F-11 traceability/status documents after verified GREEN.
- Do not change rate-limit thresholds, ordering, implementation, or production handlers.
- Do not reorder or weaken the MySQL acceptance scenarios or expected drift errors.
- Do not log tokens, identifiers, contact fields, DSN values, or credentials.
- Remote execution remains restricted to `/home/yu/services/secondhand-buyer-intent-acceptance-20260727` and Compose project `secondhand-buyer-intent-acceptance`.
- Transfer only the committed source whitelist with matching local and remote SHA-256 manifests.
- Preserve every existing prohibition on protected files and production operations.

---

### Task 1: Isolate Drift Requests And Prove The Full Gate

**Files:**
- Modify: `backend/tests/buyer_intent_mysql_test.go`
- Modify after GREEN: `docs/superpowers/specs/2026-07-28-buyer-intent-mysql-drift-fixture-isolation-design.md`
- Modify after GREEN: `docs/superpowers/plans/2026-07-28-buyer-intent-mysql-drift-fixture-isolation.md`

**Interfaces:**
- Consumes: `createBuyerIntentMySQLBuyer`, `buyerIntentMySQLBuyer`, and the existing fixture cleanup pattern.
- Produces: a drift matrix whose buyer requests have an independent 5/minute limiter scope.

- [x] **Step 1: Capture RED and establish the boundary**

Run the real isolated test after the metadata alias commits:

```bash
BUYER_INTENT_MYSQL_TEST=1 AUTO_MIGRATE=false \
  go test ./tests -run '^TestBuyerIntentMySQLAcceptance$' -count=1 -v
```

Observed: schema, history, and contention passed; the drift matrix failed. The
first buyer had exactly five prior create checks, and the handler performs the
5/minute rate check before drift validation.

- [ ] **Step 2: Create a dedicated drift buyer and cleanup**

At the start of `assertBuyerIntentMySQLDriftFailsClosed`, create a unique buyer:

```go
suffix := fmt.Sprintf("%d", time.Now().UnixNano())
driftBuyer := createBuyerIntentMySQLBuyer(
    t, srv, "f11-drift-buyer-"+suffix, "f11-drift-device-"+suffix,
)
driftBuyerHeaders := map[string]string{
    "Authorization": "Bearer " + driftBuyer.token,
    "X-Device-Id":   driftBuyer.device,
}
```

Register `t.Cleanup` that deletes this buyer's intents, auth sessions, device
bindings, and user row in that order. Do not print any field values.

- [ ] **Step 3: Route drift ownership through the dedicated buyer**

Use `driftBuyer.id` in the injected row and pass `driftBuyerHeaders` to the
buyer create/list/detail calls. Keep merchant calls on `merchantHeaders`. Keep
the two malformed states and every digest/log assertion unchanged.

- [ ] **Step 4: Run local compile and non-opt-in regressions**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./tests -run '^(TestBuyerIntentMySQLAcceptance|TestBuyerIntentStateDriftFailsClosed)$' -count=1 -v
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./internal/app ./tests ./migrations -count=1
```

Expected: the MySQL test skips locally without opt-in; drift regressions and all
three packages pass.

- [ ] **Step 5: Commit the isolated fixture**

```bash
gofmt -w backend/tests/buyer_intent_mysql_test.go
git diff --check
git add backend/tests/buyer_intent_mysql_test.go
git commit -m "test(buyer): isolate MySQL drift fixture"
```

- [ ] **Step 6: Run the committed source on isolated MySQL 8.4**

Regenerate the committed whitelist, require identical local/remote manifests,
clean only verified resources from the fixed test project, and run the complete
acceptance command once. Require both AutoMigrate modes, full/race/vet gates,
leak scan, evidence hashes, and production snapshot equality.

- [ ] **Step 7: Record GREEN and server status**

Update this plan and its design with exact commit, commands, MySQL version,
evidence classification, retained resource state, and unchanged-production
result. Do not mark production migration or deployment complete.

## Plan Self-Review

- Every spec requirement maps to Task 1.
- No placeholder, production hook, limiter reset, threshold change, or test
  reordering is present.
- The helper uses existing fixture types and cleanup patterns with exact field
  names.
