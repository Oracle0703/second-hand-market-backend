# F-11 Buyer Intent Open Uniqueness Design

**Date:** 2026-07-27

**Branch:** `codex/reconcile-code-reviews`

**Status:** F-11 fixed and passed isolated MySQL 8.4.8 test-server review at `6f84cc6`; production `0009` not executed; the F-12 prerequisite is satisfied but F-12 is not implemented.

**Finding:** F-11 - the buyer-intent unique index permits only one closed
history for a buyer and product

## 1. Goal

Enforce the actual buyer-intent invariant at the database boundary:

- one buyer may retain any number of closed intent histories for one product;
- the same buyer and product may have at most one open intent;
- repeated create, contact, and close cycles must remain valid;
- concurrent intent creation must have one database-enforced winner;
- schema drift and inconsistent intent state must fail closed;
- MySQL 8.4 migrations, GORM startup, and SQLite tests must express the same
  business rule without pretending the dialects have the same DDL features.

Code-side closure, isolated MySQL 8.4 test-server review, and production
migration are independent states. This design does not authorize source
transfer, remote execution, production migration, deployment, or production
data changes.

## 2. Current State and Failure

`BuyerIntent` and `0002_buyer_domain.up.sql` currently create this key:

```text
uk_buyer_product_open(buyer_id, product_id, is_open)
```

The key prevents two `is_open=true` rows, but it also prevents two
`is_open=false` rows. The first intent can be created and closed, and a second
intent can then be created, but closing the second collides with the first
closed history.

The existing flow has two additional correctness gaps:

- the regression test stops after creating the second intent and therefore
  never exercises the failing second close;
- intent creation maps any database error whose text contains `"unique"` to
  conflict, so an unrelated unique-key failure can be misreported as an
  already-open intent.

The tracked review observed zero production buyer-intent rows at review time.
That observation is not a migration shortcut: all migration gates must handle
non-empty isolated fixtures, reject unsafe data, and never alter production
rows automatically.

## 3. Non-goals

- Do not enable, register, redesign, or release the hidden miniapp intent UI.
- Do not change buyer-intent rate limits, contact requirements, ownership, or
  product eligibility.
- Do not redesign the `NEW -> CONTACTED -> CLOSED` business workflow.
- Do not delete or compact closed intent history.
- Do not rewrite `0002`; it remains the immutable historical baseline.
- Do not add a database trigger or a new business table.
- Do not repair malformed business rows with migration `INSERT`, `UPDATE`,
  `DELETE`, `REPLACE`, or `TRUNCATE` statements.
- Do not provide a destructive down migration.
- Do not start F-12 implementation until F-11 code and isolated MySQL 8.4
  acceptance are complete.
- Do not modify or stage `.tmp/` or the three protected untracked review
  documents.

## 4. Approaches Considered

### 4.1 Adopted: nullable open marker plus a unique key

MySQL derives a nullable marker from `is_open`. Open rows receive `1`; closed
rows receive `NULL`. MySQL unique keys reject duplicate non-null values but
permit multiple rows containing `NULL`, so a unique key on
`(buyer_id, product_id, open_marker)` expresses the exact invariant.

SQLite uses its native partial unique index on `(buyer_id, product_id) WHERE
is_open = 1`. This gives local tests the same behavior without adding a fake
ordinary column that application code could write.

This approach is adopted because it is database-enforced, deterministic under
concurrency, additive before the legacy key is removed, and small enough to
verify precisely.

### 4.2 Rejected: application pre-check only

The current count-before-create check is useful as a fast path but cannot be
the authority. Two requests can both observe zero rows and then insert. A
database constraint is required for correctness.

### 4.3 Rejected: trigger-maintained marker or history table split

A trigger introduces hidden write behavior and a second source of status
truth. Moving closed intents to another table expands API, reporting, and
migration scope without improving the invariant. Neither is justified for
this finding.

## 5. Canonical Intent-State Contract

Only these persisted combinations are legal:

| `status` | `is_open` | Meaning | Runtime treatment |
| --- | --- | --- | --- |
| `NEW` | `true` | Newly submitted open intent | Valid |
| `CONTACTED` | `true` | Merchant-contacted open intent | Valid |
| `CLOSED` | `false` | Closed history | Valid; repeated close is idempotent |
| `NEW` | `false` | State drift | Internal error |
| `CONTACTED` | `false` | State drift | Internal error |
| `CLOSED` | `true` | State drift | Internal error |
| any unknown status | either value | State drift | Internal error |

`status` remains the workflow state and `is_open` remains the queryable open
flag. `open_marker` is derived storage only. Application code must never set,
update, or accept it from a request.

All buyer and merchant intent endpoints validate every loaded intent before
returning success or applying an idempotent/transition result. In particular,
a malformed `CLOSED + true` row must not be accepted as an idempotent close,
and a malformed open row must not be reported as an ordinary duplicate.

## 6. Dialect-Specific Schema Contract

### 6.1 MySQL 8.4

The final `buyer_intents` table contains this generated column:

```sql
open_marker TINYINT
  GENERATED ALWAYS AS (
    CASE WHEN is_open = 1 THEN 1 ELSE NULL END
  ) STORED
```

The final unique key is:

```text
uk_buyer_intent_open(buyer_id, product_id, open_marker)
```

The final schema does not contain `uk_buyer_product_open`. Existing unrelated
indexes, including `idx_buyer_intent_product_open`, remain unchanged.

The generated column must be non-writable, stored, and semantically identical
to the approved expression. Schema validation may normalize harmless MySQL
formatting such as whitespace, case, parentheses, and identifier quoting, but
must reject a different mapping, virtual generation, an ordinary column, a
different type, or nullability drift.

### 6.2 SQLite

SQLite test and development databases use:

```sql
CREATE UNIQUE INDEX uk_buyer_intent_open
  ON buyer_intents (buyer_id, product_id)
  WHERE is_open = 1;
```

The SQLite final state has no physical `open_marker` column and no
`uk_buyer_product_open` index. Validation uses `PRAGMA index_list`,
`PRAGMA index_info`, and the index SQL to require the exact unique partial
index rather than accepting a similarly named or non-partial index.

### 6.3 GORM model ownership

Remove the legacy `uniqueIndex:uk_buyer_product_open` tags from `BuyerID`,
`ProductID`, and `IsOpen`. GORM must not recreate the defective key.

Represent `open_marker` on `BuyerIntent` exactly as a nullable, read-only,
migration-ignored field:

```go
OpenMarker *uint8 `gorm:"->;-:migration"`
```

This field is never present in a create/update map and never appears in an API
DTO. The dialect helper, not GORM `AutoMigrate`, owns creation and validation
of the MySQL generated column and both dialect-specific unique indexes.

## 7. Startup and Schema Convergence

The application has one schema helper with explicit dialect branches. It runs
after GORM has created ordinary tables when `AUTO_MIGRATE=true`, and a
read-only verifier runs on every startup before defaults are seeded or routes
are served.

### 7.1 `AUTO_MIGRATE=true`

For development and tests, startup may converge only a recognized state:

1. GORM migrates ordinary model columns and indexes without managing
   `open_marker` or either F-11 dialect index.
2. The F-11 helper validates table shape and intent-state data.
3. The helper adds the dialect-specific final constraint.
4. It verifies the new constraint before removing the legacy key.
5. A final verifier requires the exact final state.

A new, empty GORM-created table with neither old nor new F-11 index is a
recognized development state. The same unconstrained shape with any business
row is drift and must fail rather than being silently adopted.

### 7.2 `AUTO_MIGRATE=false`

Startup performs no DDL. It accepts only the final dialect-specific schema and
valid intent-state data. A legacy, intermediate, missing, duplicated, or
drifted F-11 structure prevents the server from starting.

This closes the current gap where disabling GORM migration also disables
schema compatibility checks.

### 7.3 Recognized MySQL resume states

| State | `open_marker` | Legacy key | New key | Allowed action |
| --- | --- | --- | --- | --- |
| Empty GORM table | absent | absent | absent | Add marker and new key |
| Legacy `0002` | absent | exact | absent | Add marker, add/verify new key, drop legacy key |
| Column added | exact | exact | absent | Add/verify new key, drop legacy key |
| Both keys | exact | exact | exact | Verify new key, drop legacy key |
| Final | exact | absent | exact | Verified no-op |

The empty GORM-table state is allowed only by the application helper with
`AUTO_MIGRATE=true`, never by the formal MySQL `0009` migration. Any other
combination, unexpected relevant key, or drifted column/key definition fails
closed.

### 7.4 Recognized SQLite resume states

SQLite accepts an empty unconstrained GORM table, the exact legacy named
index, both exact old and new indexes, or the exact final partial index. The
helper creates and verifies the partial index before dropping the old named
index. An unconstrained non-empty table, generated-marker column, unnamed
lookalike constraint, or drifted partial predicate is rejected.

## 8. Formal MySQL Migration `0009`

Add exactly these migration artifacts:

- `backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql`
- `backend/migrations/0009_buyer_intent_open_uniqueness.up.sql`
- `backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql`

There is intentionally no
`0009_buyer_intent_open_uniqueness.down.sql`.

### 8.1 Preflight

Preflight is read-only. It requires:

- one canonical InnoDB `buyer_intents` table with the required existing
  columns and primary/intent-number indexes;
- only a recognized formal-migration state: legacy, generated-column-only
  with the legacy key, both exact keys, or final;
- exact definitions for every present F-11 column and key;
- no unknown status and no illegal `status/is_open` combination;
- no `(buyer_id, product_id)` group with more than one open row.

Any failure uses `SIGNAL SQLSTATE '45000'` with a stable non-sensitive reason.
Preflight does not fix rows or schema.

### 8.2 Up migration

The up migration repeats the relevant checks so drift between preflight and
up cannot pass. Its resumable order is fixed:

1. add the exact generated `open_marker` when absent;
2. add `uk_buyer_intent_open` when absent;
3. query `information_schema` and prove the generated expression and ordered
   unique-key columns are exact;
4. drop `uk_buyer_product_open` only after the new key is proven;
5. verify the exact final state and emit a stable success marker.

MySQL DDL can commit between statements, so every interruption point must be a
recognized input to the next run. The procedure must fail before removing the
legacy key if the new key cannot be created or verified.

Preflight and up must contain no business-table `INSERT`, `UPDATE`, `DELETE`,
`REPLACE`, or `TRUNCATE`. Creating/dropping their temporary stored procedure
wrappers is allowed; mutating buyer-intent rows is not.

### 8.3 Postflight

Postflight is read-only and requires:

- the exact stored generated column;
- exactly one `uk_buyer_intent_open` with ordered columns
  `buyer_id,product_id,open_marker`;
- no `uk_buyer_product_open`;
- valid intent-state data and at most one open row per buyer/product;
- no duplicate or lookalike F-11 unique index.

It emits a stable success marker only after every condition passes. F-12
migration `0010` must require this exact postflight shape.

## 9. Runtime Data Flow

### 9.1 Database error translation

The application-owned `gorm.Open` configuration enables
`TranslateError: true`. Runtime code identifies duplicate keys with
`errors.Is(err, gorm.ErrDuplicatedKey)`. The existing broad
`strings.Contains(strings.ToLower(err.Error()), "unique")` branch is removed.

### 9.2 Creating an intent

The create flow remains idempotency-aware and follows this sequence:

1. authenticate the buyer and apply existing rate limits;
2. validate the request, contact method, and on-shelf product;
3. query the existing open row for `(buyer_id, product_id)`;
4. if a valid open row exists, return conflict; if the row is malformed or
   the query fails, return internal error;
5. create a `NEW + is_open=true` intent without writing `open_marker`;
6. if create returns a translated duplicate-key error, re-query the open row;
7. return conflict only when that re-query finds one valid open row; return
   internal error when no open row exists, it is malformed, or re-query fails.

The re-query is mandatory because `buyer_intents` also has an independent
unique intent number. A duplicate from that key, or from future unrelated
keys, must not be mislabeled as an open-intent conflict.

The pre-check improves the common response path; the database unique key is
the concurrency authority.

### 9.3 Contact and close transitions

The merchant endpoints validate the loaded state before idempotency handling:

- `NEW + true -> CONTACTED + true` is valid;
- `CONTACTED + true -> CONTACTED + true` is idempotent;
- `NEW + true` or `CONTACTED + true -> CLOSED + false` is valid;
- `CLOSED + false -> CLOSED + false` is idempotent;
- every other persisted combination returns internal error without mutation
  or a success operation log.

Closing a second or later valid intent now succeeds because closed rows are
outside the open-only unique constraint.

### 9.4 Read paths

Buyer and merchant list/detail paths load enough state to apply the canonical
validator. If any selected row is malformed, the request fails with internal
error rather than returning contradictory workflow data. Normal response
shapes remain unchanged and never expose `open_marker`.

## 10. Error Semantics

| Condition | HTTP | Code | Behavior |
| --- | ---: | ---: | --- |
| Valid open intent already exists | 409 | `10010` | No new row |
| Concurrent create loses to open-intent key | 409 | `10010` | Re-query proves the winner |
| Duplicate key but no valid open row is found | 500 | `20001` | Do not misclassify another key |
| Duplicate reconciliation query fails | 500 | `20001` | Fail closed |
| Illegal or unknown persisted intent state | 500 | `20001` | No mutation/success response |
| Schema missing, intermediate, or drifted at startup | startup failure | n/a | Do not serve traffic |
| Migration data/schema precondition fails | SQLSTATE `45000` | n/a | No business-row mutation |

Existing invalid request, authentication, authorization, product-state,
rate-limit, idempotency, and not-found semantics are unchanged.

## 11. Migration Recovery and Operational Rollback

`0009` is a forward-only schema correction. There is no automated down
migration because restoring the legacy key would reintroduce the defect and
could fail after multiple closed histories legitimately exist.

If execution stops after adding the column or new key, rerun preflight, up,
and postflight. The recognized-state machine resumes without touching rows.
If a definition is drifted or data is invalid, stop and investigate; do not
rename, drop, rewrite, or delete anything automatically.

Application rollback after `0009` keeps the corrected additive column and key.
The old application writes `is_open`, so the generated marker remains
compatible. Once more than one closed history exists, recreating the old key
is both incorrect and potentially impossible.

Any future repair for an unrecognized production state is a new forward
migration with its own design, evidence, backup plan, and explicit production
authorization. This specification does not authorize it.

## 12. Test Strategy

Behavioral changes use RED -> GREEN tests.

### 12.1 Static migration contract

Migration artifact tests require:

- all three `0009` files and their stable gate markers;
- the exact generated-column and ordered unique-key contract;
- `SIGNAL SQLSTATE '45000'` failure paths;
- recognition of the approved intermediate states;
- no business DML token in preflight or up;
- absence of a `0009` down script;
- acceptance-script isolation guards and MySQL 8.4 enforcement.

### 12.2 SQLite schema tests

Focused tests cover:

- a new empty database converges to the exact partial unique index;
- an exact legacy index converges without changing rows;
- both-index and final states rerun idempotently;
- a non-empty unconstrained table, wrong columns, wrong uniqueness, wrong
  order, wrong predicate, lookalike index, and unexpected marker fail closed;
- `AUTO_MIGRATE=false` accepts only the final partial-index state;
- `AUTO_MIGRATE=true` creates no legacy or duplicate index.

### 12.3 Intent behavior and errors

Real Gin/GORM tests cover:

- create first, close first, create second, close second, then repeat for at
  least one additional cycle while retaining all closed rows;
- a new open intent after multiple closed histories;
- two concurrent creates for the same buyer/product have one success and one
  `409 / 10010`, with exactly one open row;
- different buyers or products remain independent;
- valid contact and close idempotency;
- every illegal/unknown status combination returns `500 / 20001` on affected
  read and transition paths without mutation;
- translated duplicate-key reconciliation returns conflict only when a valid
  open row exists and returns internal error for absent, malformed, or failed
  re-query results;
- an unrelated unique-key failure is never reported as an open-intent
  conflict;
- API response DTOs and create/update SQL do not expose or write
  `open_marker`.

### 12.4 Local quality gates

Run focused tests first, then all of:

```bash
cd backend && go test ./...
cd backend && go test -race ./...
cd backend && go vet ./...
git diff --check
```

Repository-local Go module and build caches may be used. A skipped MySQL test
is not evidence of isolated MySQL acceptance.

## 13. Isolated MySQL 8.4 Acceptance

The proposed dedicated environment is:

```text
Remote directory: /home/yu/services/secondhand-buyer-intent-acceptance-20260727
Compose project:  secondhand-buyer-intent-acceptance
```

Design approval and written-specification approval do not authorize transfer
or remote execution. Before either action, obtain separate user authorization
for this exact directory, Compose project, action, and source whitelist.

The future transfer whitelist is limited to the minimum reviewed source needed
for the F-11 harness:

- `backend/` source, tests, migrations, module files, and Dockerfile, excluding
  databases, caches, uploads, and evidence;
- non-sensitive `deploy/acceptance/` source scripts and manifests;
- `Makefile`.

Always exclude `.env` files, credentials, secrets, databases, upload files,
evidence directories, `.git`, caches, `node_modules`, `backend/app.db`,
`.tmp/`, and the three protected review documents.

The isolated matrix uses synthetic rows only and proves:

1. the database reports MySQL `8.4.x` and belongs to the dedicated Compose
   project;
2. local and remote committed-source SHA-256 manifests are identical;
3. legacy `0002..0008`, generated-column-only, both-key, and final states all
   converge or no-op as specified;
4. every recognized interruption point reruns successfully;
5. drifted columns/keys, duplicate open rows, unknown statuses, and illegal
   status/open combinations fail with SQLSTATE `45000`;
6. each rejected fixture has an identical before/after row summary and no
   hidden migration DML;
7. multiple closed histories and exactly one open intent are permitted;
8. repeated close cycles and concurrent create behavior match the API
   contract;
9. `AUTO_MIGRATE=false` accepts the final SQL shape and rejects every other
   shape without DDL;
10. `AUTO_MIGRATE=true` is idempotent and creates no legacy/duplicate key;
11. focused tests, the complete backend suite, race tests, and `go vet ./...`
    pass in the isolated source tree;
12. production container identity, running state, and restart counts are
    identical before and after acceptance.

The harness must fail closed when the isolation confirmation, project name,
database engine/version, source hash, or production snapshot check is absent.
It must never print a DSN, password, token, secret, production row, or buyer
contact field.

Retained evidence contains only committed source hashes, tool versions, stable
migration markers, sanitized test names/counts and exit codes, aggregate
synthetic row summaries, non-secret schema descriptions, Compose identity,
and the unchanged production-container snapshot comparison. A sanitized
tracked review report is added only after the authorized matrix passes.

## 14. Documentation and Status Rules

The authoritative status must distinguish:

| State | Required wording |
| --- | --- |
| Design/spec only | Design approved; implementation not started |
| Implementation and local gates pass | Code-side fixed; isolated test-server review pending; production `0009` not executed |
| Authorized isolated MySQL acceptance passes | Fixed and passed isolated test-server review; production `0009` not executed |
| Separately authorized production migration passes | Production F-11 closed, only with retained migration evidence |

After local evidence exists, append a follow-up status to tracked current
review/release documents with the design and plan paths, commit range, exact
commands/results, and remaining gates. After authorized isolated acceptance,
add a sanitized report at
`docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md`
and record the server-review state.

Do not rewrite historical finding text. Do not modify the three protected
untracked review documents.

## 15. F-12 Dependency

F-12 reserves `0010_buyer_identity_migration` and may reassign source buyer
intents to a canonical buyer. It depends on F-11 because reassignment must
retain any number of closed histories while detecting a conflicting open row.

F-12 implementation may begin only after all of these are true:

- F-11 written specification and implementation plan are approved;
- F-11 code and `0009` artifacts are committed and locally verified;
- the authorized isolated MySQL 8.4 F-11 matrix passes;
- the retained F-11 report identifies the exact accepted commit range.

F-12 `0010` preflight and startup verification must require the exact F-11
MySQL final state. F-12 must not duplicate or weaken F-11 schema ownership.

## 16. Expected File Scope

Implementation is expected to remain within:

- `backend/internal/model/models.go`;
- `backend/internal/app/server.go` and focused schema-helper tests;
- `backend/internal/app/buyer_handlers.go`;
- `backend/internal/app/merchant_intent_handlers.go`;
- focused buyer-intent integration/MySQL tests;
- the three `0009` migration artifacts and their contract test;
- an F-11 acceptance script, Makefile target, and acceptance README entry;
- the F-11 design, implementation plan, sanitized acceptance report, and
  tracked status documents.

No frontend, miniapp, unrelated model/domain, production configuration,
credential, database, upload artifact, or protected review document is changed
under F-11.

## 17. Acceptance Criteria

- MySQL final schema has the exact stored nullable generated marker and one
  exact `uk_buyer_intent_open` key, with no legacy key.
- SQLite final schema has one exact partial unique index and no marker column
  or legacy key.
- GORM never writes or migrates `open_marker` and never recreates the legacy
  unique index.
- `AUTO_MIGRATE=true` converges only recognized development/test states;
  `AUTO_MIGRATE=false` accepts only the final state.
- `0009` is resumable across every DDL interruption point, contains no
  business DML, and has no down migration.
- Any number of closed histories and at most one open row exist per
  buyer/product.
- At least three complete create/close cycles pass and retain every history.
- Concurrent duplicate creation has one winner and one verified conflict.
- Only a proven valid open intent maps a translated duplicate error to
  `409 / 10010`; ambiguous cases map to `500 / 20001`.
- Illegal persisted intent states fail closed on reads and transitions.
- Focused/full/race/vet and diff checks pass locally before code-side closure.
- Isolated MySQL 8.4 evidence passes before test-server approval is recorded.
- F-12 remains blocked until the F-11 accepted commit range is recorded.
- No source transfer, remote execution, production schema/data change, or
  production deployment occurs without separate exact authorization.

## 18. Approval Record

- The user approved the F-11 architecture design on 2026-07-27.
- The user approved the F-11 data-flow design on 2026-07-27.
- The user approved the F-11 runtime error-semantics design on 2026-07-27.
- The user approved the F-11 migration and rollback design on 2026-07-27.
- The user approved the F-11 test and acceptance design on 2026-07-27.
- The user explicitly approved the F-11 written specification on 2026-07-27.
- This approval authorizes preparation of the implementation plan. It does not
  authorize source transfer, remote execution, production inspection,
  production migration, deployment, or data changes.

## 19. 2026-07-27 Code-Side Follow-Up

F-11 code-side fixed; isolated MySQL 8.4 test-server review pending; production 0009 not executed.

The approved design is
`docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md`;
the implementation plan is
`docs/superpowers/plans/2026-07-27-buyer-intent-open-uniqueness.md`.

- `77771d379ce260b548b54c45882c8173747467fe..0f2cf7b5db9bbe7f00c18490dc523b09709d8467`
  is the implementation-only F-11 commit range.
- `4e8ea92d9fd0206abae3e000b92123ae23a20254..0f2cf7b5db9bbe7f00c18490dc523b09709d8467`
  is the independent whole-branch code-review range from the F-11 written-design
  baseline through the pre-documentation implementation HEAD. Task 8 reviews
  the subsequent documentation commits separately.

All local gates recorded PASS with these results:

- `git status --short --branch --untracked-files=no` printed only
  `## codex/f11-buyer-intent-open-uniqueness`; the tracked worktree was clean.
- `mkdir -p backend/.cache/go/mod backend/.cache/go/build` exited 0.
- `cd backend && go test ./internal/app -run 'Test.*BuyerIntent' -count=1`
  exited 0 with `ok .../internal/app`.
- `cd backend && go test ./tests -run 'TestBuyerIntent' -count=1` exited 0
  with `ok .../tests`.
- `cd backend && go test ./migrations -run 'TestBuyerIntentOpenUniqueness' -count=1`
  exited 0 with `ok .../migrations`.
- `bash -n deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh` and
  `git diff --check` both exited 0 with no error output.
- `cd backend && env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test ./... -count=1`
  exited 0 with every package passing.
- `cd backend && env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test -race ./... -count=1`
  exited 0 with every package passing; `backend/tests` reported `ok` in
  147.287s.
- `cd backend && env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go vet ./...`
  exited 0 with no error output.

The authorized isolated MySQL 8.4 acceptance now records the accepted F-11
snapshot and repair range in
`docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md`.
F-12's F-11 prerequisite is satisfied; F-12 implementation and validation have
not started. Production `0009` execution and deployment remain separately
authorized future work.

## 20. 2026-07-28 Isolated MySQL 8.4 Acceptance

F-11 fixed and passed isolated test-server review at source commit
`6f84cc68c2f6dd870e2e6943f240d9b8589d6396`; production `0009` was not
executed.

The dedicated `secondhand-buyer-intent-acceptance` project passed MySQL 8.4.8,
the complete 0008/0009 success and rejection matrix, final/API schema checks,
both AutoMigrate modes, full tests, race tests, and vet. The retained evidence
reported `forbidden_matches=0`; all 26 SHA-256 entries verified; the fixed-field
production before/after snapshots were byte-identical. The accepted evidence
and exact commit ranges are recorded in the sanitized report above.

This satisfies the four F-12 dependency bullets in section 15. It does not
claim that F-12 is implemented, tested, deployed, or production-authorized.
