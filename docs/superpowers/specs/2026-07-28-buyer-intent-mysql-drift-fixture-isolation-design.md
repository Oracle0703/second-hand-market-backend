# F-11 MySQL Drift Fixture Isolation Design

**Date:** 2026-07-28

**Branch:** `codex/f11-buyer-intent-open-uniqueness`

**Status:** Root cause confirmed in the isolated MySQL 8.4 project; implementation pending

## 1. Problem And Evidence

After the MySQL metadata aliases were fixed, the real
`TestBuyerIntentMySQLAcceptance` passed application startup, schema assertions,
three history cycles, and synchronized uniqueness contention. It then failed at
the first request in `assertBuyerIntentMySQLDriftFailsClosed`.

The acceptance sequence consumes the first buyer's exact 5/minute creation
budget before the drift matrix:

1. three historical create requests;
2. two synchronized create attempts, one success and one uniqueness conflict.

`handleBuyerIntentCreate` checks the 5/minute rate limit before it loads the
product or calls `findOpenBuyerIntent`. The drift matrix's create request is
therefore the sixth request and returns the rate-limit response before the
intentionally malformed row can be validated. The MySQL state validator is not
failing; the acceptance fixture is sharing unrelated in-memory limiter state.

## 2. Goals

1. Exercise MySQL drift failure semantics without prior rate-limit history.
2. Keep the real authentication, rate limiter, handlers, MySQL schema, and
   state validator in the path.
3. Preserve the existing drift matrix and its no-row-mutation/no-log-mutation
   assertions.
4. Clean every extra buyer artifact created by the isolated fixture.

## 3. Non-Goals

- Do not change rate-limit values, ordering, or production behavior.
- Do not add a test-only reset hook to the application.
- Do not reorder the history, contention, independence, or drift gates.
- Do not loosen the expected HTTP 500 / code `20001` drift response.
- Do not operate on production data, configuration, logs, services, or Docker.

## 4. Approaches Considered

### A. Dedicated Drift Buyer And Device (Selected)

Create one additional isolated buyer immediately before the drift matrix. Use
that buyer's ID for injected rows and its token/device for buyer requests. The
two matrix states consume at most two of the 5/minute allowance. Merchant reads
and transitions continue to use the existing fixture merchant.

This keeps every production component real and isolates only test ownership.

### B. Reset The In-Memory Rate Limiter

Expose or reach into limiter state from tests. This creates a test-only
application interface and couples acceptance to limiter internals.

### C. Reorder Drift Before Contention

This avoids the current count incidentally, but makes independent gates depend
on execution order and can move the same pollution into later assertions.

Approach A is selected because it is explicit, local to the fixture, and does
not weaken any runtime behavior.

## 5. Data And Cleanup Flow

`assertBuyerIntentMySQLDriftFailsClosed` creates a unique buyer session with a
unique device. Each malformed row uses that buyer ID and the existing second
product. Buyer create/list/detail calls use the new token and device; merchant
list/detail/transition calls keep the merchant headers.

A `t.Cleanup` registered inside the helper removes, in dependency order:

1. buyer intents owned by the drift buyer;
2. buyer auth sessions;
3. buyer device bindings;
4. the buyer user row with an unscoped delete.

Cleanup errors remain best-effort, matching the existing isolated fixture
cleanup. No identifier or token is logged.

## 6. Error Semantics

Both malformed states (`CLOSED/open` and `BOGUS/closed`) must return HTTP 500 /
code `20001` for every buyer and merchant operation in the matrix. Each request
must preserve the row digest and operation-log count. A rate-limit response is
a fixture failure, not an accepted alternative.

## 7. Verification

RED is the retained MySQL 8.4 run after metadata aliases: history and contention
passed, then the drift matrix failed after the exact five prior create checks.

GREEN requires:

1. `TestBuyerIntentMySQLAcceptance` passes with `AUTO_MIGRATE=false` and true;
2. the complete F-11 isolated harness passes migrations `0008` and `0009`, API
   schema, full tests, race tests, and vet;
3. evidence reports `forbidden_matches=0` with valid SHA-256 hashes;
4. fixed-field production before/after snapshots are byte-identical.

## 8. Rollback

Revert the fixture-isolation commit. No database rollback is required. The
revert restores order-dependent rate-limit pollution and reopens the MySQL
acceptance failure.

## 9. Traceability

- Implementation plan:
  `docs/superpowers/plans/2026-07-28-buyer-intent-mysql-drift-fixture-isolation.md`.
- Preceding compatibility fix:
  `docs/superpowers/specs/2026-07-28-buyer-intent-mysql-metadata-alias-design.md`.
- Runtime scope: isolated Compose project
  `secondhand-buyer-intent-acceptance` only; production unchanged.
