# F-15 Atomic Idempotency Design

**Date:** 2026-07-28

**Branch:** `codex/f15-idempotency-atomicity`

**Status:** Code-side implemented and locally verified on 2026-07-28;
isolated MySQL 8.4 test-server review pending; production unchanged

**Original finding:** The pre-fix idempotency wrapper read before the business
action, ran the action in a separate transaction, inserted the replay record
afterward, and ignored insertion errors. Concurrent requests could therefore
execute the same side effect more than once, and a successful response could be
returned without a durable replay record.

## 1. Goal

For the scope `(Idempotency-Key, operator_id, path)`, execute a successful write
at most once and durably replay only its successful terminal result. Couple the
business mutation and replay record in one database transaction so neither can
commit without the other.

The approved failure contract is:

1. Only successful terminal results are replayed.
2. A callback, serialization, or terminal-record failure before commit rolls
   back the claim and all transactional business writes, so the same request
   can retry.
3. Once a scope has a committed successful result, reusing it with a different
   request hash returns HTTP 409 and business code `10011` without executing the
   second action.

## 2. Authority And Current Defect

`docs/backend-api-checklist.md` defines the scope and requires one execution,
first-result replay, and `10011` for different parameters. The existing
`runWithIdempotency` sequence violates that contract in three ways:

- read-before-write allows two concurrent misses;
- the business callback owns an independent transaction;
- replay-record creation occurs after the business commit and discards errors.

The existing unique key `uk_idem_scope(idem_key, operator_id, path)` is already
the correct serialization primitive. F-15 changes transaction ownership rather
than adding another lock service.

## 3. Scope

F-15 covers the current five idempotent write call sites:

- merchant intent transition to `CONTACTED`;
- merchant intent transition to `CLOSED`;
- product status changes;
- order completion and closure;
- buyer intent creation.

It also covers the shared helper, focused SQLite tests, opt-in MySQL 8.4
concurrency tests, acceptance automation, and status/evidence documentation.

## 4. Non-Goals

- Do not introduce Redis, an external lock service, or an in-process mutex.
- Do not replay failed responses or persist failed claims.
- Do not make the existing in-memory rate-limit counters transactional. They
  remain admission controls rather than database business writes.
- Do not change `Idempotency-Key` scope, request hashing, response envelopes,
  existing business error codes, or domain state-machine rules.
- Do not add a TTL, lease, cleanup worker, `PENDING` column, or `SUCCEEDED`
  column. Successful records retain the existing long-lived replay behavior.
- Do not modify `0001_init` or add an F-15 migration. F-12 retains the reserved
  `0010` migration number and F-15 introduces no numbering conflict.
- Do not execute production SQL, deploy production code, or modify production
  data, containers, sessions, or idempotency rows.

## 5. Approaches Considered

### 5.1 Adopted: transaction-scoped insert claim using the existing schema

Insert a placeholder replay row inside the same transaction that receives every
business write. The unique index serializes contenders. The row is finalized
with the successful response before commit. Any callback, serialization, or
pre-commit database error rolls back both the placeholder and business writes.

An uncommitted placeholder is never a replayable state. Other transactions
cannot observe it as a terminal result; they wait on the unique-key conflict
until the first transaction commits or rolls back.

### 5.2 Rejected: persisted `PENDING` / `SUCCEEDED` state with TTL

Persisted claims require leases, stale-owner recovery, cleanup ownership, and a
definition for requests that outlive the lease. They also change the existing
long-lived replay contract and require a migration. None of that is needed to
satisfy the approved failure semantics.

### 5.3 Rejected: external or process-local locks

Process-local locks do not coordinate multiple API instances. External locks
still cannot atomically commit the MySQL business mutation and replay record,
and introduce a second availability and recovery domain.

## 6. API And Transaction Contract

The callback becomes transaction-aware:

```go
type idempotentOperation func(tx *gorm.DB) (map[string]interface{}, error)

func (s *Server) runWithIdempotency(
    c *gin.Context,
    payload interface{},
    fn idempotentOperation,
) (map[string]interface{}, error)
```

A successful callback must return a non-nil JSON-object map. An empty map is a
valid success result; a nil map is an internal contract error and rolls back.

The wrapper owns the only transaction around the callback. Callers must use the
provided `tx` for every read, lock, mutation, and operation-log write. They must
not call `s.DB.Transaction` or use `s.DB` from inside the callback.

Requests without an `Idempotency-Key` still run the callback in one transaction.
This preserves the atomicity previously supplied by four caller-owned
transactions and adds the missing transaction around buyer-intent creation.

## 7. Data Flow

### 7.1 Request preparation

1. Resolve the header key without changing its current semantics.
2. If the key is empty, skip payload serialization and hashing and use the
   no-key execution path.
3. For a keyed request, resolve the current actor; absence is an internal
   contract failure.
4. Serialize the payload and hash the exact JSON bytes with the existing
   SHA-256 helper. Serialization failure returns the stable internal error
   before the callback runs.
5. Resolve `c.FullPath()` without changing the current scope semantics.

### 7.2 No-key execution

Run `fn(tx)` inside `s.DB.Transaction`. Callback errors roll back all writes and
are returned unchanged. No idempotency record is created.

### 7.3 Keyed execution and claim

Inside `s.DB.Transaction`:

1. Insert one `IdempotencyRecord` containing the scope, request hash,
   `result_code=0`, and the JSON placeholder `null`. The placeholder is visible
   only inside the uncommitted transaction and is distinct from every permitted
   non-nil object result, including `{}`.
2. If insertion succeeds, call `fn(tx)`.
3. Reject a nil result map. Serialize the non-nil successful result;
   serialization failure returns the internal error and rolls back the
   transaction.
4. Update exactly the inserted row with `result_code=0` and the serialized
   response. An update error or `RowsAffected != 1` returns the internal error
   and rolls back the transaction.
5. Commit. Only this commit makes the business mutation and terminal replay
   record visible together.

The production database is opened with GORM `TranslateError: true`, so the
claim insert identifies a scope collision with
`errors.Is(err, gorm.ErrDuplicatedKey)` rather than matching database error
strings. At that exact insertion point the wrapper converts the error to a
private idempotency-claim sentinel before returning from the transaction. Only
that sentinel enters contender resolution; a duplicate-key error from a
business table or another callback operation remains a callback error and must
not be mistaken for an idempotency replay.

### 7.4 Contender resolution

When the claim transaction returns the private idempotency-claim sentinel, the
transaction is ended and the wrapper reads the committed record outside it:

- different `request_hash`: return `common.ErrDuplicateSubmit` (`10011`);
- same hash and a valid `result_code=0` JSON object: replay it and set
  `idempotent=true` in the returned copy;
- missing, corrupt, or non-success record: return the stable internal error.

InnoDB unique-index locking provides the required ordering:

- if the first transaction commits, a contender receives duplicate-key and
  reads the terminal response;
- if the first transaction rolls back, a waiting insert can acquire the scope
  and execute the action itself;
- a different-hash contender never executes after the successful first commit.

Because a failed transaction leaves no record by design, there is no durable
hash to compare after rollback. The next waiter or retry, whether it carries the
same or a different hash, competes as a new first execution. This is the direct
consequence of the approved rule that failures are not persisted. HTTP 409 /
`10011` applies whenever a committed successful record already occupies the
scope.

There is no retry loop around arbitrary database errors. Deadlocks, lock
timeouts, connection failures, and corrupt records return the internal error.
A failure before the commit attempt proves that the enclosing transaction did
not commit. A connection failure while `COMMIT` is in flight has an unknown
outcome: the initial caller receives the internal error, and a later same-key
retry safely converges by replaying the record if commit succeeded or acquiring
the claim and executing if it did not.

### 7.5 Transactional storage prerequisite

Approach A requires transactional tables. On MySQL startup, after migrations,
the server queries `information_schema.tables` and fails closed unless
`idempotency_records`, `buyer_intents`, `products`, `orders`, `order_events`,
and `operation_logs` all exist with `ENGINE=InnoDB`. SQLite startup skips this
MySQL-specific check.

The isolated MySQL 8.4 gate records the same six-table engine result. Production
engine inspection and deployment remain separate, explicitly authorized work;
F-15 implementation and isolated acceptance do not query production.

### 7.6 SQLite concurrency-fixture transaction mode

The historical `TestBuyerIntentConcurrentCreateHasOneWinner` regression used a
four-connection, shared-memory SQLite database. After buyer-intent creation is
moved into the wrapper-owned transaction, SQLite's default deferred transaction
mode allows both requests to read the product and open-intent state before
either owns the single writer path. SQLite ignores GORM's `FOR UPDATE` clause,
so one read transaction can then fail while upgrading to a writer with
`SQLITE_BUSY` instead of reaching the open-intent uniqueness result.

Adding `_txlock=immediate` while retaining shared cache is not sufficient. The
modernc/glebarez driver maps that option to `BEGIN IMMEDIATE`, but the second
shared-cache connection receives `SQLITE_LOCKED_SHAREDCACHE` instead of the
`SQLITE_BUSY` condition covered by the five-second timeout. The approved
fixture-only correction moves this test to a file under `t.TempDir()`, omits
shared cache, and keeps `_txlock=immediate`. The first request acquires the
writer path at transaction start; the second waits under the existing timeout,
then reads the committed open intent and returns HTTP 409. The test keeps four
open connections and still asserts exactly one success and one conflict.

This option is not injected by `openDB`, is not added to application
configuration, and does not change MySQL behavior. The MySQL implementation
continues to rely on InnoDB transaction and unique-index locking plus
`SELECT ... FOR UPDATE`. Application-level `SQLITE_BUSY` retries and
process-local mutexes remain out of scope because they would broaden runtime
error semantics or fail to coordinate multiple processes.

## 8. Caller Conversion

Each existing callback is flattened so it uses the supplied transaction:

```go
data, err := s.runWithIdempotency(c, payload, func(tx *gorm.DB) (map[string]interface{}, error) {
    // All reads, row locks, mutations, and logs use tx.
})
```

The conversion must preserve every current response field and domain error.
Existing same-target business behavior may still return `idempotent=true`; the
shared wrapper independently sets that field to true for a stored replay.

Buyer-intent prechecks that protect the mutation move inside the supplied
transaction. The handler performs no transaction-external product status
precheck: the callback loads and locks the product row with the supplied `tx`,
then its status and merchant ID determine the insert. This also lets an already
committed same-key result replay even if the product status later changes. The
database open-intent uniqueness constraint remains the final concurrency guard
and is not weakened by F-15.

Buyer-intent authentication, actor-type checks, JSON binding, and contact-field
validation remain before the wrapper. Its three rate-limit checks move into the
callback so a committed same-key replay does not consume quota or return
`10009` instead of the stored success. Rate-limit counters are in-memory and
therefore are not rolled back if a later transactional step fails; a retry is
still subject to the documented admission limits.

`writeOperationLog` changes to return the `insertOperationLog` error. Each of
the four idempotent transition callbacks that writes logs must propagate that
error through the supplied transaction. Other existing callers may continue to
discard the returned error until their own separately scoped review; F-15 does
not silently broaden their behavior.

## 9. Error Semantics

| Condition | Result | Durable claim | Business writes |
| --- | --- | --- | --- |
| First execution succeeds | Original success payload | Terminal success row | Committed once |
| Same hash after success | Stored payload plus `idempotent=true` | Unchanged | Not executed |
| Different hash after success | HTTP 409 / `10011` | Unchanged | Not executed |
| Callback returns business error | Original business error | None | Rolled back |
| Callback returns database/internal error | Stable internal error where already mapped | None | Rolled back |
| New request after an earlier rollback | Executes as a new first attempt | Created only on success | Commits only on success |
| Response serialization fails | Stable internal error | None | Rolled back |
| Replay-row finalization fails | Stable internal error | None | Rolled back |
| Connection loss during commit | Stable internal error; retry converges | Unknown until retry/read | Atomically all or none |
| Stored response is corrupt | Stable internal error | Unchanged for investigation | Not executed |

The implementation must not log payloads, stored response bodies, credentials,
tokens, buyer contact data, or database connection values as part of error
handling or acceptance evidence.

## 10. Test Strategy

### 10.1 Focused helper tests

Tests must first fail against the old implementation and then prove:

1. a successful request persists one replay row and a repeat does not call the
   callback;
2. after a successful first request, a different payload under the same scope
   returns `10011` without a second callback;
3. a callback that writes and then fails leaves neither business data nor a
   replay row, and the same request can subsequently succeed;
4. an empty object result succeeds and replays, while a nil map or JSON `null`
   terminal result fails closed;
5. response serialization failure rolls back callback writes and the claim;
6. forced replay-row update failure rolls back callback writes and the claim;
7. an operation-log insert failure rolls back the protected business write and
   claim;
8. a business-table duplicate-key error is not treated as an idempotency claim
   collision;
9. a no-key callback still runs transactionally and rolls back on error;
10. corrupt stored JSON fails closed without executing the callback;
11. retry resolution handles both possible outcomes of an unknown commit by
    replaying an existing success or executing when no claim exists.

### 10.2 Caller regression tests

Cover all five call sites through focused handler or integration tests. The
tests must prove response compatibility and that a forced terminal-record
failure cannot leave each protected business mutation committed.

The SQLite historical concurrency regression is a dialect fixture, not the
production contention gate. The original shared-memory deferred-transaction
fixture returned HTTP 500 / `20001` in 4 of 20 repeated runs. The first
proposed correction retained `mode=memory&cache=shared` and added
`_txlock=immediate`; runtime diagnostics rejected that design because 19 of 20
runs failed at `BEGIN IMMEDIATE` with `SQLITE_LOCKED_SHAREDCACHE` (extended
code 262), before callback SQL ran. SQLite `busy_timeout` handles
`SQLITE_BUSY`, not shared-cache table-lock failures.

The approved fixture therefore uses a file-backed database under
`t.TempDir()` with private-cache behavior, `_pragma=busy_timeout(5000)`, and
`_txlock=immediate`:

```go
cfg.DBDSN = "file:" + filepath.Join(t.TempDir(), "buyer-intent.db") +
    "?_pragma=busy_timeout(5000)&_txlock=immediate"
```

This keeps four open connections and the strict one-success/one-conflict
result. It changes only this historical test fixture: application and MySQL
DSNs remain unchanged, and no retry, process-local mutex, or assertion
weakening is allowed. The diagnostic candidate passed 20 consecutive runs and
then 50 consecutive runs. The committed implementation must repeat at least 20
runs, then run the complete Task 4 focused set and
`go test ./internal/app ./tests -count=1`.

### 10.3 MySQL 8.4 concurrency gate

An opt-in test in an isolated MySQL 8.4 database must run concurrent same-scope
requests and prove:

- same hash: exactly one callback side effect and all successful callers receive
  identical stored business fields; the original execution reports its original
  `idempotent` value and each replay reports `idempotent=true`;
- different hash: exactly one hash wins, the other receives `10011`, and only
  one side effect exists;
- first callback failure: its writes and claim are absent, then a waiting or
  retried same request can succeed exactly once;
- terminal-record write failure: the business side effect is absent;
- all six transaction-participating tables report `ENGINE=InnoDB`.

The acceptance project must use a dedicated directory and Compose project,
verify source manifests, retain sanitized evidence, and compare only the allowed
production container name/ID/state/restart-count snapshots. It must not run
production SQL or inspect production logs, environment, mounts, or data.

### 10.4 Metadata-free acceptance package and failure evidence

The reviewed local commit is the only source authority. Source-list mode uses
`git ls-tree HEAD`, and export mode obtains the listed bytes with
`git archive HEAD`; neither the index nor working-tree contents can add or
replace transferable source. The exporter writes a NUL path list, a per-file
SHA-256 manifest, the source archive, and a checksum manifest. The checksum
manifest's SHA-256 digest is recorded out of band.

The remote directory intentionally has no `.git`. Before Docker access, normal
mode requires that recorded digest, verifies all package artifacts, verifies
each received path is a regular non-symlink file with the expected hash, and
reconstructs the Docker build context from the verified archive. A staged-only
path, dirty working-tree byte, missing file, symlink, unexpected path, changed
archive, or changed manifest fails closed.

After the authorized production-before snapshot, failure handling stops the
isolated project containers and constructs a separate sanitized evidence set.
Only validated classified checkpoints, complete authorized snapshots already
captured, a fixed failure stage, leak-scan status, and hashes may be copied.
Raw test, migration, race, vet, and Docker output remains under the temporary
runtime directory and is deleted. If checkpoint/snapshot validation or the leak
scan cannot be proven, only hardcoded sanitization-failure classifications and
their hashes are retained.

## 11. Documentation And Evidence

The implementation plan, RED/GREEN commands, commit range, local full/race/vet
results, isolated MySQL version and concurrency results, evidence hashes, and
production snapshot equality must be recorded. Status reporting always
separates:

1. code-side closure;
2. isolated test-server approval;
3. production migration/deployment status.

F-15 has no production migration. Production remains unchanged until a separate
deployment authorization is executed and verified.

## 12. Acceptance Criteria

- The successful business mutation and terminal replay row commit atomically.
- Concurrent same-scope/same-hash requests execute the callback exactly once.
- Same scope with a committed successful result and a different hash returns
  HTTP 409 / `10011` and does not run the second callback.
- Every callback, serialization, log-write, and terminal-record failure before
  commit rolls back the claim and transactional business writes and permits a
  safe same-request retry.
- An unknown commit outcome returns an internal error and a later same-key retry
  converges without duplicating a committed business mutation.
- Only successful terminal results replay; no persisted pending or failed state
  exists.
- All five callers use the wrapper-supplied transaction for protected work.
- Existing response fields, business state transitions, and error codes do not
  regress.
- The four-connection SQLite buyer-intent concurrency regression passes at
  least 20 consecutive runs with one success and one conflict; no HTTP 500 is
  accepted.
- MySQL startup fails closed unless all six transaction-participating tables use
  InnoDB; isolated acceptance records that verification.
- Focused, full, race, and vet gates pass locally.
- The isolated MySQL 8.4 concurrency gate passes with sanitized, hashed evidence
  before test-server status is marked approved.
- No production data, SQL, service, container, deployment, or idempotency row is
  modified during implementation and isolated acceptance.

## 13. Local Verification Record

Code-side commits through `15f57dd` passed the focused idempotency/buyer/order
selection, a fresh serial `go test ./... -count=1`,
`go test -race ./internal/app ./tests -count=1`, and `go vet ./...` on
2026-07-28. The final race run of the `tests` package completed in 368.402
seconds. Shell syntax, gofmt, diff checks, metadata-free package behavior, and a
127-path `HEAD` export audit also passed with zero forbidden paths.

An initial full run executed concurrently with race was not accepted after one
signal fixture missed its five-second ready-file deadline. The exact fixture
passed alone in 2.99 seconds, and the subsequent serial full suite passed every
package, establishing resource contention rather than a code regression.

The second `product_order_inventory` operation-log rollback branch has an
explicit regression. A temporary, uncommitted mutation that discarded that
error made the regression fail with HTTP 200/code 0; restoring error
propagation made both operation-log rollback suites pass. No production source
mutation remained.

These results close only local code status. The opt-in MySQL 8.4 contention
matrix, evidence audit, and production snapshot equality remain pending the
separately authorized test-server run. F-15 has no migration, and no production
deployment or data change has occurred.
