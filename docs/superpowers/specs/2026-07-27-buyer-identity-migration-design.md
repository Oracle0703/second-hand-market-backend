# F-12 Buyer Identity Migration Design

**Date:** 2026-07-27

**Branch:** `codex/reconcile-code-reviews`

**Status:** Design and written specification approved; implementation not started

**Finding:** F-12 - production buyer login accepts synthetic mock identities

## 1. Goal

Close the production mock-identity vulnerability without discarding or silently
splitting the data owned by existing experience users.

The completed change must:

- prevent every production mini-program provider from running in `mock` mode;
- support an explicit, fail-closed migration window before normal real-account
  creation is enabled;
- preserve one manually approved existing buyer as the canonical buyer;
- merge only explicitly approved same-provider mock buyers into that canonical
  buyer;
- require either an active approved old session or a separately issued,
  short-lived recovery credential;
- verify the real identity with the platform server before changing ownership;
- merge buyer-owned data, revoke old sessions, and create the replacement
  canonical session atomically;
- retain mandatory, non-secret audit evidence;
- keep production data changes behind a separate maintenance authorization.

Code-side closure, isolated MySQL acceptance, real-platform test-environment
acceptance, and production migration are separate states. This branch must not
claim that the 19 production rows or the two real experience users were
migrated.

## 2. Current State and Confirmed Constraints

The backend already implements real WeChat and Douyin code exchange. The defect
is not the absence of a platform HTTP client. It is the unsafe mode and account
transition behavior around that client.

Current facts established by source and the protected review evidence are:

- `BUYER_WECHAT_LOGIN_MODE` and `BUYER_DOUYIN_LOGIN_MODE` default to `mock`;
- mock mode derives an identity from caller-controlled temporary code;
- production WeChat was explicitly configured as mock and Douyin fell back to
  mock;
- production contained 19 WeChat mock buyer rows, representing two known
  experience users rather than 19 distinct people;
- repeated platform login code values are not stable, so a mock OpenID cannot
  be mapped automatically to the later real OpenID;
- current login looks up only `(auth_provider, openid)` and creates a new buyer
  when no row exists;
- buyer-owned data spans device bindings, favorites, histories, intents, and
  auth sessions;
- production had zero buyer intents at review time, but the migration must
  still be correct for non-empty isolated fixtures and future environments;
- `buyer_intents` currently has the F-11 uniqueness defect, so F-11 is a
  prerequisite for general closed-history reassignment;
- `APP_ENV` now exists due earlier hardening, but production validation does
  not yet reject buyer mock modes;
- the miniapp builds for both WeChat and Douyin and sends the provider selected
  by its runtime platform.

An active old session is acceptable evidence that the caller controls one
manually approved mock account. A device ID is not account-recovery evidence.

## 3. Non-goals

- Do not modify, migrate, disable, or revoke any production buyer or session in
  this branch.
- Do not transfer source or run a remote project without separate authorization.
- Do not obtain, store, commit, print, or return production platform secrets.
- Do not infer a real identity from nickname, avatar, phone, device ID, mock
  OpenID, UnionID, IP address, or behavioral similarity.
- Do not automatically merge a real identity that is already owned by another
  buyer.
- Do not merge WeChat and Douyin buyers into a cross-provider account. The
  current buyer model owns one provider identity; cross-provider account
  linking needs a separate product and data-model design.
- Do not use UnionID as an automatic merge key.
- Do not add an administrator frontend for the two-user migration. Audited
  administrator APIs and a runbook are sufficient.
- Do not implement F-05, F-10, F-15, or unrelated authentication changes.
- Do not weaken F-14 active-session and current-account enforcement.
- Do not add a background migration that changes accounts without an approved
  task and a real platform code exchange.

## 4. Approaches Considered

### 4.1 Adopted: keep an approved existing buyer as the canonical buyer

A super administrator prepares a task containing one canonical mock buyer and
the exact same-provider mock buyers allowed to merge. A member with a valid old
session, or a user holding a separately issued recovery token, submits a real
platform code. The backend verifies the platform identity and performs the
complete merge in one database transaction.

This preserves the canonical buyer ID and minimizes foreign-key and owner-key
rewrites. Manual membership approval prevents device-based or heuristic account
takeover.

### 4.2 Rejected: create a new real buyer and move all old data to it

This gives the new identity a visually clean row but rewrites every business
reference, changes the canonical ID, and expands rollback and session handoff.
It provides no security benefit over a verified in-place identity replacement.

### 4.3 Rejected: maintenance-window SQL only

Operator SQL can merge the known rows, but it cannot prove that the user
controls the old account and real platform identity in one auditable workflow.
It is also harder to test for retries, concurrent attempts, and session handoff.

## 5. Provider Mode Contract

Each provider has one explicit mode:

| Mode | Allowed environment | Login behavior |
| --- | --- | --- |
| `mock` | development/test only | Existing synthetic behavior for automated tests |
| `migration` | production/test | Real code exchange; existing real identity may log in; an unbound identity is not created |
| `real` | production/test/development | Real code exchange; existing identity logs in and an unbound identity may create a buyer |
| `disabled` | production/test/development | Provider login is unavailable |

Production validation must reject blank, unknown, or `mock` values. At least
one provider must be `migration` or `real`; an accidentally all-disabled buyer
deployment is invalid.

For every production provider in `migration` or `real`, startup requires:

- non-empty AppID and AppSecret;
- a positive HTTP timeout;
- the exact official HTTPS code-exchange endpoint for that provider.

Non-production tests may use an `httptest` endpoint. Production cannot point a
platform secret at an arbitrary URL.

The intended F-12 rollout configuration is:

```text
initial migration window:
  WeChat = migration
  Douyin = disabled

after both experience users are verified:
  WeChat = real
  Douyin = disabled
```

Douyin may move from disabled to migration/real only after its own credential,
platform, and real-device acceptance.

Both `/api/v1/buyer/auth/wechat-login` and
`/api/v1/buyer/auth/miniapp-login` consume the same mode policy and identity
resolver. In migration mode, an unbound real identity returns conflict and no
buyer row or session is created.

## 6. Data Model

Reserve the next formal migrations as:

```text
0009_buyer_intent_open_uniqueness     F-11 prerequisite
0010_buyer_identity_migration         F-12 additive audit/control tables
```

The detailed F-11 design owns the exact `0009` generated-column and index
contract. `0010` preflight must refuse to run unless the F-11 postflight shape
is present.

### 6.1 `buyer_identity_migrations`

One row represents one irreversible same-provider identity transition:

```text
id
migration_no                     unique, non-secret business identifier
provider                         wechat or douyin
canonical_buyer_id
status                           PENDING, SUCCEEDED, or CANCELLED
created_by_admin_id
recovery_token_hash              nullable SHA-256 hash
recovery_expires_at              nullable
recovery_used_at                 nullable
last_attempt_at                  nullable
last_failure_code                nullable categorical value only
favorites_merged                 result count
histories_merged                 result count
device_bindings_merged           result count
intents_moved                    result count
source_buyers_disabled           result count
completed_at                     nullable
cancelled_at                     nullable
created_at
updated_at
```

No platform code, raw OpenID, UnionID, AppSecret, access token, refresh token,
or recovery token is stored in this table.

### 6.2 `buyer_identity_migration_members`

```text
id
migration_id
buyer_id
member_role                      CANONICAL or SOURCE
created_at
```

Exactly one member is CANONICAL and it must equal the task's
`canonical_buyer_id`. The table has a unique key on
`(migration_id, buyer_id)` and an index on `buyer_id`.

Task creation runs in a transaction, locks every proposed buyer in stable ID
order, and rejects any buyer already present in another PENDING task. Concurrent
task creation for an overlapping buyer therefore serializes on the buyer row.
A cancelled task retains its members and events for audit, but the same buyers
may enter a newly approved task because no partial business mutation occurred.

All members must:

- exist and not be soft deleted;
- use the task provider;
- have an active account status when the task is approved;
- have the provider's recognized mock OpenID prefix;
- be explicitly selected by the super administrator.

### 6.3 `buyer_identity_migration_events`

Every task state change inserts a mandatory event in the same transaction:

```text
id
migration_id
event_type                       CREATED, RECOVERY_ISSUED, COMPLETED, CANCELLED
actor_type                       ADMIN or BUYER
actor_id                         nullable only for recovery completion
result_code                      non-secret categorical/API result
created_at
```

This is the authoritative operation audit for identity migration. The existing
generic operation-log helper is not sufficient because its write error is
intentionally ignored by callers.

## 7. API and Authorization Boundaries

The administrator control plane is restricted to the current
`SUPER_ADMIN` role:

```text
POST /api/v1/admin/buyer-identity-migrations
GET  /api/v1/admin/buyer-identity-migrations/:id
POST /api/v1/admin/buyer-identity-migrations/:id/recovery-token
POST /api/v1/admin/buyer-identity-migrations/:id/cancel
```

Task creation accepts provider, canonical buyer ID, and the complete source
buyer ID list. The server loads and validates every row; it never trusts client
labels or prefixes. The creation transaction locks proposed buyers in stable
ID order before checking for another PENDING membership. The response never
includes member OpenIDs.

A recovery-token response contains a 32-byte cryptographically random token
exactly once. The database stores only its SHA-256 hash. The token expires in
15 minutes, issuing another token invalidates the previous one, and successful
use marks it consumed. Delivery and user verification occur outside the
application under the production runbook.

The buyer completion endpoints are:

```text
POST /api/v1/buyer/auth/identity-migrations/:id/complete
POST /api/v1/buyer/auth/identity-migrations/:id/recover
```

`complete` requires an active F-14-validated buyer session whose buyer ID is a
task member. `recover` is unauthenticated but requires the migration ID, the
valid single-use recovery token, provider, and platform code. It has strict IP
and task-level rate limits. Neither endpoint accepts device ID as proof.

Both endpoints use the real provider exchange even if a non-production server
otherwise runs in mock mode. The submitted provider must equal the task
provider.

## 8. Transactional Completion Flow

Before calling the platform, the handler applies rate limits and performs a
read-only credential precheck. The session path requires the F-14 actor to be a
member of the pending task. The recovery path compares the supplied token hash
to an unexpired, unused task credential before it consumes a one-time platform
code. This precheck is not authorization to mutate; every condition is checked
again under row locks.

After successful real code exchange, one database transaction performs:

1. Lock the migration row with `FOR UPDATE`.
2. Require `PENDING`; a cancelled task cannot run.
3. Lock the canonical buyer and every member buyer in stable ID order.
4. Revalidate task membership, account state, provider, mock prefix, and
   absence of soft deletion.
5. For a session completion, reload the exact `auth_sessions.id` and require an
   active, unexpired, identity-matched buyer session for the member actor. For
   recovery, compare and consume the matching, unexpired recovery-token hash.
6. Require a non-mock real OpenID and reject a real `(provider, openid)` owned
   by any other buyer.
7. Run all merge preflight checks, including the open-intent conflict check,
   before the first business-row mutation.
8. Merge favorites, histories, device bindings, and intents as defined below.
9. Replace the canonical buyer's mock provider identity with the verified real
   provider/OpenID/UnionID.
10. Mark every SOURCE buyer `DISABLED`; do not delete it or rewrite its mock
    identity.
11. Revoke every non-revoked auth session for all migration members.
12. Create one new canonical buyer auth session and its refresh-token hash
    through a transaction-aware token-issuance helper.
13. Persist result counts, mark the migration `SUCCEEDED`, consume any recovery
    credential, and insert the mandatory COMPLETED event.
14. Commit before returning the new access/refresh pair.

Any failure rolls back all business rows, account states, session changes,
result counts, and the completion event. A safe categorical failure code may be
recorded on the still-pending task after rollback; failure evidence must not
contain identity or credential values.

### 8.1 Favorites

For every product present under any member buyer:

- the canonical owner row is active if any member row is active;
- an existing canonical row is updated rather than duplicated;
- if no canonical row exists, create one using the established canonical
  `owner_key` contract;
- all SOURCE rows are made inactive and receive
  `merge_target_buyer_id=canonical` and `merged_at`;
- merchant/product ownership fields remain consistent with the product record.

### 8.2 Histories

For every product:

- `first_viewed_at` is the minimum member timestamp;
- `last_viewed_at` is the maximum member timestamp;
- `view_count` is the sum of all member counts;
- the canonical row is active if any member row is active;
- SOURCE rows are made inactive and marked with the canonical merge target.

### 8.3 Device bindings

For each distinct device:

- upsert a canonical binding;
- preserve the earliest first-bind time and latest last-bind/last-merge time;
- delete SOURCE binding rows after the canonical row exists.

The audit task and members retain the source-to-canonical trace. Device
bindings remain metadata, not identity proof.

### 8.4 Buyer intents

F-11 must first allow any number of closed histories and at most one open row
per `(buyer_id, product_id)`.

Before reassignment, if two or more member buyers have an open intent for the
same product, completion returns conflict with no mutation. An operator must
resolve the business state and retry. Otherwise, every SOURCE intent is
reassigned to the canonical buyer without changing its intent number, status,
contact fields, merchant handling history, or source device ID.

### 8.5 Profile and identity fields

Migration preserves the canonical nickname, avatar, and phone. Completion does
not accept profile replacement fields. UnionID is stored when the real provider
returns it but is not used to select or merge accounts.

## 9. Session Handoff, Retries, and Concurrency

Token issuance must be refactored internally to accept a GORM transaction. The
new session row, refresh-token hash, account merge, old-session revocation, and
COMPLETED event therefore commit or roll back together.

All old sessions for CANONICAL and SOURCE members are revoked before the new
canonical session is created. The new session is excluded by construction
because it is inserted after the revocation update.

Two concurrent completion attempts serialize on the migration row. Exactly one
can change PENDING to SUCCEEDED and return the new token pair. The other cannot
repeat business mutations.

Plaintext tokens are never stored for response replay. If the successful HTTP
response is lost, the old session is already revoked. The client obtains a new
platform code and uses normal login. Because the verified real identity now
belongs to the canonical buyer, login creates a new session for that buyer and
does not create another account.

## 10. Error Semantics

| Condition | HTTP | Code | Behavior |
| --- | ---: | ---: | --- |
| Production blank, unknown, or mock mode | startup failure | n/a | Do not serve traffic |
| Production real/migration missing credentials or official URL | startup failure | n/a | Do not serve traffic |
| Disabled provider login | 403 | `10003` | Provider unavailable |
| Migration-mode identity not yet bound | 409 | `10010` | No buyer/session creation |
| Task absent or authenticated actor not a member | 404 | `10004` | Do not disclose membership |
| Invalid/expired/used recovery token | 401 | `10002` | No mutation |
| Member session revoked during platform exchange | 401 | `10002` | Transaction recheck rejects it |
| Invalid or expired platform code | existing 400/401 mapping | `10001`/`10002` | No mutation |
| Explicitly disabled member account | 403 | `10007` | No migration |
| Existing real identity belongs to another buyer | 409 | `10010` | Require manual review |
| Multiple open intents for one target product | 409 | `10010` | Resolve business state first |
| Cancelled/already completed state transition | 409 | `10010` | No repeated mutation |
| Database/platform format/internal invariant failure | 500 | `20001` | Fail closed and roll back |

Error responses, application logs, operation events, test output, and retained
evidence must not contain platform code, raw OpenID, UnionID, recovery token,
access token, refresh token, AppID, AppSecret, or buyer profile PII.

## 11. Deployment and Rollback

The execution order is fixed:

```text
F-11 design/implementation and 0009 MySQL 8.4 acceptance
-> F-12 0010 and code-side isolated MySQL acceptance
-> real WeChat test-environment credentials and iOS/Android acceptance
-> production read-only inventory, recoverable backup, and canonical mapping
-> deploy with WeChat=migration and Douyin=disabled
-> prepare and complete one experience-user task
-> verify identity, session, and aggregate counts; exercise recovery/rollback plan
-> prepare and complete the second experience-user task
-> verify no pending approved user and no new mock buyer creation
-> change WeChat=migration to WeChat=real
-> observe and retain sanitized evidence
```

Before the first successful production identity transition, the application
deployment may be rolled back while additive tables remain. Existing mock
sessions continue to work during the bounded migration window, but no new mock
login or mock buyer creation is allowed.

After the first success, identity data is forward-only. Automated rollback
must not restore a mock OpenID, reactivate SOURCE buyers, or revive revoked
sessions. A production anomaly stops further migrations and account creation;
recovery uses the pre-change backup and an explicitly approved forward repair.

No production step occurs as part of code review, local tests, or isolated
acceptance. Production backup, configuration, task creation, data mutation,
and mode switch each require the maintenance authorization defined by the
later runbook.

## 12. Test Strategy

Behavior changes use RED -> GREEN tests.

### 12.1 Configuration and provider policy

Cover every environment/mode combination, including:

- development/test mock remains available;
- production rejects blank, unknown, and mock modes;
- production rejects all-disabled providers;
- real/migration requires credentials, positive timeout, and the official
  HTTPS endpoint;
- disabled requires no credential;
- migration and real perform actual HTTP exchange in controlled tests;
- migration login accepts an already-bound real identity but never creates an
  unbound buyer;
- both public login endpoints share the same provider policy.

### 12.2 Task authorization and recovery

Cover:

- SUPER_ADMIN can prepare/read/cancel a valid task;
- ordinary ADMIN, merchant, buyer, and anonymous callers cannot use the
  control plane;
- invalid member, duplicate member, cross-provider member, non-mock member,
  disabled/deleted member, multiple canonical members, and overlapping task
  membership fail closed;
- concurrent task creation with an overlapping buyer has one winner, while a
  cancelled task does not permanently consume its members;
- only a member's active session can complete;
- a session revoked while platform exchange is in flight fails the locked
  transaction recheck without changing identity or ownership;
- recovery token has 32 random bytes, only its hash is stored, expires after
  15 minutes, is invalidated by reissue, and succeeds once;
- task and IP rate limits reject abuse without logging secrets.

### 12.3 Merge contracts

Use real Gin/GORM integration tests with nontrivial fixtures to prove exact
favorite, history, binding, intent, account, session, and audit outcomes.
Inject failures before identity replacement, source disablement, session
revocation, new-session creation, and event insertion; every injection must
leave the pre-transaction database state unchanged.

### 12.4 Concurrency and retry

Prove:

- two simultaneous session completions have one winner;
- session and recovery completion racing have one winner;
- no duplicate canonical owner rows or sessions are created;
- response-loss simulation followed by a fresh real login returns the same
  canonical buyer;
- cancelled and succeeded tasks cannot mutate again.

### 12.5 MySQL 8.4 isolated acceptance

Use a dedicated directory and Compose project proposed as:

```text
Remote directory: /home/yu/services/secondhand-buyer-identity-acceptance-20260727
Compose project:  secondhand-buyer-identity-acceptance
```

No transfer or remote execution is authorized by this design. A later plan
must request an exact source whitelist and separate authorization.

The isolated matrix uses synthetic buyers only and proves:

- `0009` prerequisite and `0010` preflight/up/postflight behavior;
- SQL and GORM schema agreement without duplicate indexes;
- row-lock concurrency and one-winner completion;
- complete merge and rollback invariants with non-empty intent histories;
- mode policy and secret-safe evidence;
- source SHA-256 equality;
- full backend tests and `go vet ./...`;
- isolated Compose identity distinct from production;
- production container identity, state, and restart counts remain unchanged.

Real platform acceptance is a separate test-environment gate because synthetic
MySQL acceptance cannot prove WeChat or Douyin credential/platform behavior.
Real codes and credentials must never enter retained repository evidence.

## 13. Documentation and Status Rules

Status documents must distinguish:

| State | Required wording |
| --- | --- |
| Implementation and local gates pass | Code-side fixed; F-11/MySQL/real-platform/production gates stated explicitly |
| Isolated MySQL acceptance passes | Passed isolated test-server review; real-platform and production migration pending |
| Real test-environment acceptance passes | Real-platform test environment approved; production migration pending |
| Production users migrated and mode is real | Production F-12 closed, only after separately authorized evidence |

The three protected review documents remain untouched. Historical finding text
in tracked current-status documents is preserved and receives an appended
follow-up status.

## 14. Expected File Scope

Expected implementation scope is limited to:

- backend config validation and examples;
- mini-program identity policy/resolver and buyer login handler;
- transaction-aware internal token issuance;
- buyer identity migration models, DTOs, handlers, and focused service logic;
- `0010` preflight/up/postflight and migration artifact tests;
- focused local/MySQL acceptance tests and guarded acceptance harness;
- F-12 design, plan, runbook, review evidence, and tracked status documents.

F-11 owns `0009` and its model/index tests. No production configuration,
secret, database, uploaded file, frontend, or unrelated domain implementation
is changed under F-12.

## 15. Acceptance Criteria

- Production cannot start with a blank, unknown, or mock buyer provider mode.
- Production real/migration providers cannot send secrets to a non-official
  endpoint.
- Migration mode performs real exchange, permits existing real identities, and
  creates no unbound buyer.
- Only a SUPER_ADMIN-approved, same-provider membership can migrate.
- Completion requires an active approved session or a valid one-time recovery
  token; device ID is never sufficient.
- The verified real identity stays on the approved canonical buyer ID.
- Favorites, histories, device bindings, and intents follow the exact merge
  contracts without silent loss.
- F-11 is complete before intent reassignment is enabled.
- SOURCE buyers are disabled, all old member sessions are revoked, and exactly
  one new canonical session is committed atomically with the merge.
- Concurrent completion has one winner and retries do not repeat data mutation.
- Mandatory audit records commit with every state transition and contain no
  identity or credential secret.
- Local focused/full/race/vet and isolated MySQL 8.4 gates pass before code-side
  or server-review closure is recorded.
- Real-platform and production status are never inferred from synthetic tests.
- No production data or service changes occur without separate authorization.

## 16. Approval Record

- Provider strategy, canonical-buyer approach, architecture, data flow, error
  semantics, deployment order, and test strategy were presented in the active
  goal sequence on 2026-07-27.
- No contrary design direction was received during the continuation sequence.
- The user explicitly approved the F-12 written specification on 2026-07-27.
- This approval authorizes preparation of the implementation plan. It does not
  authorize source transfer, remote execution, real-platform credentials,
  production configuration, production data migration, or deployment.
