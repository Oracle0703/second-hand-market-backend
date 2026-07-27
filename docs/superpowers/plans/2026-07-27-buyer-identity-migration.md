# F-12 Buyer Identity Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace production synthetic buyer identities with real mini-program identities through an explicitly approved, auditable, same-provider migration that atomically preserves buyer-owned data and session continuity.

**Architecture:** Production login uses one fail-closed provider policy with `mock`, `migration`, `real`, and `disabled` modes; only non-production may use `mock`. A SUPER_ADMIN prepares an immutable membership task, then an approved member proves control with an active session or one-time recovery credential and a real platform code. One locked database transaction merges owned data, moves the verified identity to the canonical buyer, disables sources, revokes old sessions, creates the replacement session, and records mandatory audit evidence.

**Tech Stack:** Go 1.22, Gin, GORM, glebarez SQLite, MySQL 8.4, SQL migrations, Bash, Docker Compose, Make.

## Global Constraints

- F-11 migration `0009_buyer_intent_open_uniqueness` must be code-side complete and pass isolated MySQL 8.4 acceptance before any F-12 implementation begins.
- Provider modes are exactly `mock`, `migration`, `real`, and `disabled`; production rejects blank, unknown, and `mock` values.
- At least one production provider must be `migration` or `real`; a production configuration with every provider disabled is invalid.
- A production `migration` or `real` provider requires non-empty AppID/AppSecret, a positive HTTP timeout, and its exact official HTTPS code-exchange endpoint.
- Initial production rollout is WeChat `migration` and Douyin `disabled`; later WeChat `real` and Douyin `disabled` requires a separately authorized maintenance step.
- Migration membership is explicit, immutable, same-provider, and limited to recognized mock buyers. One member is canonical and every other member is a source.
- Completion requires either an active F-14-validated member session or a 32-byte random, single-use recovery token that expires after 15 minutes.
- Device ID, nickname, avatar, phone, UnionID, IP address, behavior, and mock OpenID similarity are never identity or account-recovery evidence.
- The canonical buyer ID, nickname, avatar, and phone remain unchanged. The verified provider/OpenID/UnionID replace only the canonical identity fields.
- A real `(auth_provider, openid)` already owned by another buyer is never merged automatically.
- Favorites, histories, device bindings, intents, identity replacement, source disablement, old-session revocation, replacement-session issuance, result counts, and the COMPLETED event commit in one transaction.
- Platform code, raw OpenID, UnionID, AppID, AppSecret, recovery token, access token, refresh token, and buyer profile PII must not appear in logs, events, error responses, retained evidence, or committed fixtures.
- No destructive down migration is added. Post-success rollback is a separately approved forward repair and must never restore a mock identity or revive a revoked session.
- Code-side, isolated MySQL test-server, real-platform test-environment, and production-migration statuses remain separate.
- Do not modify, stage, commit, transfer, or rewrite `.tmp/`, `docs/architecture-evolution-plan-2026-07-24.md`, `docs/first-round-fix-review-2026-07-24.md`, or `docs/second-round-fix-review-2026-07-24.md`.
- Do not read or modify `backend/app.db`, production data, production configuration, production uploads, secrets, or production services.
- Source transfer, remote execution, real-platform credentials, production task creation, production identity mutation, deployment, and mode changes each require separate exact written authorization.
- Every behavior change follows RED -> GREEN, Go changes are formatted with `gofmt`, and each implementation task ends in a focused Conventional Commit.

---

## File Map

| Path | Responsibility |
| --- | --- |
| `backend/internal/app/config.go` | Parse and validate the four provider modes and production endpoint policy |
| `backend/internal/app/server_security_test.go` | Production/non-production provider-policy matrix |
| `backend/internal/app/miniapp_auth.go` | Shared identity value, normal-login resolver, and forced-real resolver |
| `backend/internal/app/miniapp_auth_test.go` | Provider exchange, mode, redaction, and `AllowCreate` tests |
| `backend/internal/app/auth_handlers.go` | Transaction-aware token issuance used by normal login and migration completion |
| `backend/internal/app/auth_handlers_test.go` | Token-session atomicity and rollback tests |
| `backend/internal/model/models.go` | Three `0010` control/audit models |
| `backend/internal/model/models_test.go` | SQLite/GORM schema and index contract |
| `backend/internal/dto/dto.go` | Administrator task, recovery, and completion request DTOs |
| `backend/internal/app/buyer_identity_migration.go` | Task validation, recovery issuance, locked merge, and mandatory events |
| `backend/internal/app/buyer_identity_migration_handlers.go` | SUPER_ADMIN and buyer HTTP boundaries |
| `backend/internal/app/buyer_identity_migration_test.go` | Focused service authorization, merge, failure, and concurrency tests |
| `backend/internal/app/server.go` | Model registration and six F-12 routes |
| `backend/internal/app/buyer_handlers.go` | Consume the shared identity policy and transaction-aware token helper |
| `backend/tests/buyer_identity_migration_test.go` | End-to-end SQLite API matrix |
| `backend/tests/buyer_identity_migration_mysql_test.go` | Opt-in MySQL 8.4 row-lock and migration-only acceptance |
| `backend/tests/buyer_identity_acceptance_contract_test.go` | Acceptance harness fail-closed contract |
| `backend/migrations/0010_buyer_identity_migration.preflight.sql` | Refuse missing F-11, unexpected schema, and unsafe partial state |
| `backend/migrations/0010_buyer_identity_migration.up.sql` | Add only the three control/audit tables |
| `backend/migrations/0010_buyer_identity_migration.postflight.sql` | Verify exact columns, keys, constraints, and F-11 prerequisite |
| `backend/migrations/buyer_identity_migration_test.go` | Static migration and no-down contract |
| `backend/configs/.env.example` | Development mode contract without secrets |
| `backend/configs/.env.production.mysql.example` | Fail-closed production mode documentation |
| `backend/configs/.env.production.sqlite.example` | Fail-closed production mode documentation |
| `deploy/acceptance/docker-compose.yml` | Test-only provider mode configuration for the isolated tools container |
| `deploy/acceptance/buyer-identity-migration-smoke.sh` | Dedicated MySQL 8.4 acceptance and sanitized evidence collection |
| `deploy/acceptance/README.md` | Exact guarded acceptance procedure and prohibitions |
| `Makefile` | `acceptance-buyer-identity-migration-smoke` guard target |
| `docs/runbooks/buyer-identity-migration.md` | Real-platform and production maintenance gates without credentials |
| `docs/full-project-code-review-2026-07-24.md` | Append evidence-backed F-12 status without rewriting historical finding text |
| `docs/release-readiness.md` | Keep code/server/real-platform/production statuses separate |
| `docs/superpowers/reviews/2026-07-27-buyer-identity-migration-code-review.md` | Code-side review findings, commands, and commit range |
| `docs/superpowers/reviews/2026-07-27-buyer-identity-migration-isolated-acceptance.md` | Sanitized remote evidence after separate authorization |

---

### Task 1: Enforce the F-11 prerequisite gate

**Files:**
- Verify: `docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md`
- Verify: `docs/superpowers/plans/2026-07-27-buyer-intent-open-uniqueness.md`
- Verify: `backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql`
- Verify: `backend/migrations/0009_buyer_intent_open_uniqueness.up.sql`
- Verify: `backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql`
- Verify: `docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md`

**Interfaces:**
- Consumes: the F-11 `open_marker` generated-column contract and unique key `(buyer_id, product_id, open_marker)`.
- Produces: a reviewed F-11 commit range and MySQL 8.4 evidence that make F-12 intent reassignment legal.

- [ ] **Step 1: Refuse to start while any F-11 artifact or status is missing**

Run:

```bash
test -f docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md
test -f docs/superpowers/plans/2026-07-27-buyer-intent-open-uniqueness.md
test -f backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
test -f backend/migrations/0009_buyer_intent_open_uniqueness.up.sql
test -f backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
test -f docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md
rg -n '代码侧状态：已修复|测试服务器状态：审核通过' \
  docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md
```

Expected: every command exits 0. If any command fails, stop F-12 and finish F-11; do not create F-12 code or migration files.

- [ ] **Step 2: Re-run the F-11 local and isolated acceptance gates**

Run the exact commands recorded by the approved F-11 plan. At minimum:

```bash
cd backend && go test ./migrations ./internal/model ./internal/app ./tests -count=1
cd backend && go test -race ./internal/app ./tests -count=1
cd backend && go vet ./...
```

Expected: all exit 0, and the retained MySQL report identifies MySQL `8.4.x`, the dedicated F-11 Compose project, source hashes, one-open/many-closed behavior, and unchanged production container identity/state/restart counts.

- [ ] **Step 3: Record the exact prerequisite commit range before F-12 starts**

Run:

```bash
git log --oneline --decorate -- \
  backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql \
  backend/migrations/0009_buyer_intent_open_uniqueness.up.sql \
  backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
git status --short
```

Expected: the F-11 commits are reviewable and the only unrelated untracked paths are the known protected paths. Do not make a no-op commit for this gate.

---

### Task 2: Make production provider policy fail closed

**Files:**
- Create: `backend/internal/app/miniapp_auth_test.go`
- Modify: `backend/internal/app/config.go`
- Modify: `backend/internal/app/server_security_test.go`
- Modify: `backend/internal/app/miniapp_auth.go`
- Modify: `backend/internal/app/buyer_handlers.go`
- Modify: `backend/configs/.env.example`
- Modify: `backend/configs/.env.production.mysql.example`
- Modify: `backend/configs/.env.production.sqlite.example`

**Interfaces:**
- Consumes: `Config.IsProduction`, the current WeChat/Douyin HTTP payloads, `common.ErrForbidden`, `common.ErrConflict`, and `common.ErrInternal`.
- Produces:

```go
type miniProgramIdentity struct {
	Provider    string
	OpenID      string
	UnionID     *string
	AllowCreate bool
}

func (c Config) validateBuyerIdentityPolicy() error
func (s *Server) resolveMiniProgramLoginIdentity(provider, code string) (miniProgramIdentity, error)
func (s *Server) resolveRealMiniProgramIdentity(provider, code string) (miniProgramIdentity, error)
```

- [ ] **Step 1: Write the RED configuration matrix**

Add table-driven tests in `server_security_test.go` that explicitly cover:

```go
func TestBuyerIdentityPolicy(t *testing.T) {
	tests := []struct {
		name       string
		env        string
		wechatMode string
		douyinMode string
		wantErr    string
	}{
		{name: "development mock", env: "development", wechatMode: "mock", douyinMode: "disabled"},
		{name: "test mock", env: "test", wechatMode: "mock", douyinMode: "disabled"},
		{name: "production blank", env: "production", wechatMode: "", douyinMode: "disabled", wantErr: "BUYER_WECHAT_LOGIN_MODE"},
		{name: "production unknown", env: "production", wechatMode: "other", douyinMode: "disabled", wantErr: "BUYER_WECHAT_LOGIN_MODE"},
		{name: "production mock", env: "production", wechatMode: "mock", douyinMode: "disabled", wantErr: "must not use mock"},
		{name: "production all disabled", env: "production", wechatMode: "disabled", douyinMode: "disabled", wantErr: "at least one"},
		{name: "production migration real exchange", env: "production", wechatMode: "migration", douyinMode: "disabled"},
		{name: "production real", env: "production", wechatMode: "real", douyinMode: "disabled"},
	}
	_ = tests
}
```

For the two valid production rows, populate strong JWT/upload/DB test values, exact official WeChat URL, non-empty test-only AppID/AppSecret, and positive timeout. Add separate cases proving blank credentials, zero timeout, HTTP URL, wrong host/path/query/fragment/userinfo, and the corresponding Douyin violations fail without echoing a secret.

- [ ] **Step 2: Run the policy tests and verify RED**

Run:

```bash
cd backend && go test ./internal/app -run 'TestBuyerIdentityPolicy|TestProductionBuyerIdentity' -count=1 -v
```

Expected: FAIL because `validateBuyerIdentityPolicy` is absent and current production validation accepts mock mode.

- [ ] **Step 3: Implement exact mode and endpoint validation**

Add constants and call the new validator from `Config.Validate` before the non-production early return:

```go
const (
	buyerLoginModeMock      = "mock"
	buyerLoginModeMigration = "migration"
	buyerLoginModeReal      = "real"
	buyerLoginModeDisabled  = "disabled"
	wechatCode2SessionURL   = "https://api.weixin.qq.com/sns/jscode2session"
	douyinCode2SessionURL   = "https://developer.toutiao.com/api/apps/v2/jscode2session"
)

func (c Config) validateBuyerIdentityPolicy() error {
	// Normalize each mode once; validate its allowed environment and credentials.
	// Compare production endpoints to the exact constants above.
	return nil
}
```

The implementation must not include AppID, AppSecret, endpoint query values, or submitted code in an error. Non-production `real`/`migration` may use an `httptest` URL, but still requires credentials and positive timeout. `disabled` requires no credentials.

- [ ] **Step 4: Write RED resolver and login-creation tests**

Add tests that use two `httptest.Server` instances and assert:

```go
func TestResolveMiniProgramLoginIdentityModes(t *testing.T) {}
func TestResolveRealMiniProgramIdentityAlwaysUsesPlatformExchange(t *testing.T) {}
func TestMigrationModeNeverCreatesUnboundBuyer(t *testing.T) {}
func TestMigrationModeLogsInExistingRealBuyer(t *testing.T) {}
func TestDisabledProviderReturnsForbidden(t *testing.T) {}
func TestBothBuyerLoginRoutesUseSharedPolicy(t *testing.T) {}
```

Verify `mock` returns `AllowCreate=true` only outside production, `migration` returns `AllowCreate=false`, `real` returns `AllowCreate=true`, and forced-real resolution never accepts or synthesizes a `mock_wx_`/`mock_tt_` identity.

- [ ] **Step 5: Run resolver tests and verify RED**

Run:

```bash
cd backend && go test ./internal/app ./tests -run 'MiniProgram|MigrationMode|BuyerLoginRoutes|DisabledProvider' -count=1 -v
```

Expected: FAIL because the shared identity value and `AllowCreate` behavior do not exist.

- [ ] **Step 6: Implement the shared identity resolvers and login behavior**

Replace the tuple return with `miniProgramIdentity`, make provider normalization explicit, and split platform exchange from mode selection:

```go
func (s *Server) resolveMiniProgramLoginIdentity(provider, code string) (miniProgramIdentity, error) {
	// disabled -> forbidden; mock -> synthetic non-production identity;
	// migration/real -> resolveRealMiniProgramIdentity with AllowCreate policy.
	return miniProgramIdentity{}, nil
}

func (s *Server) resolveRealMiniProgramIdentity(provider, code string) (miniProgramIdentity, error) {
	// Normalize provider and call only the official-provider HTTP exchange path.
	return miniProgramIdentity{}, nil
}
```

In `handleBuyerMiniProgramLoginRequest`, return HTTP 409/code `10010` when lookup misses and `AllowCreate` is false. Do not create a buyer, device binding, or session on that path. Keep both `/miniapp-login` and `/wechat-login` routed through this helper.

- [ ] **Step 7: Update examples, format, and run GREEN**

Document all four modes and both providers in the three example files. Production examples must use `migration`/`disabled`, exact official URLs, blank credential placeholders, and comments that startup will fail until deployment secrets are supplied.

Run:

```bash
cd backend && gofmt -w internal/app/config.go internal/app/server_security_test.go internal/app/miniapp_auth.go internal/app/miniapp_auth_test.go internal/app/buyer_handlers.go
cd backend && go test ./internal/app ./tests -run 'BuyerIdentityPolicy|MiniProgram|MigrationMode|BuyerLoginRoutes|DisabledProvider' -count=1
git diff --check
```

Expected: all exit 0.

- [ ] **Step 8: Commit the provider policy**

```bash
git add backend/internal/app/config.go backend/internal/app/server_security_test.go \
  backend/internal/app/miniapp_auth.go backend/internal/app/miniapp_auth_test.go \
  backend/internal/app/buyer_handlers.go backend/configs/.env.example \
  backend/configs/.env.production.mysql.example \
  backend/configs/.env.production.sqlite.example
git commit -m "fix(buyer): fail closed on production identity modes"
```

---

### Task 3: Add the `0010` migration control and audit schema

**Files:**
- Modify: `backend/internal/model/models.go`
- Modify: `backend/internal/model/models_test.go`
- Modify: `backend/internal/app/server.go`
- Create: `backend/migrations/0010_buyer_identity_migration.preflight.sql`
- Create: `backend/migrations/0010_buyer_identity_migration.up.sql`
- Create: `backend/migrations/0010_buyer_identity_migration.postflight.sql`
- Create: `backend/migrations/buyer_identity_migration_test.go`

**Interfaces:**
- Consumes: F-11 final `buyer_intents.open_marker` and `uk_buyer_intent_open` schema.
- Produces: `model.BuyerIdentityMigration`, `model.BuyerIdentityMigrationMember`, and `model.BuyerIdentityMigrationEvent` with exact table names.

- [ ] **Step 1: Write RED model and migration artifact tests**

Add model schema assertions and static migration checks for these exact values:

```go
const (
	BuyerIdentityMigrationPending   = "PENDING"
	BuyerIdentityMigrationSucceeded = "SUCCEEDED"
	BuyerIdentityMigrationCancelled = "CANCELLED"
	BuyerIdentityMemberCanonical    = "CANONICAL"
	BuyerIdentityMemberSource       = "SOURCE"
	BuyerIdentityActorAdmin         = "ADMIN"
	BuyerIdentityActorBuyer         = "BUYER"
)
```

The artifact test must require `SIGNAL SQLSTATE '45000'`, all three table names, all status/role check constraints, `open_marker`, `uk_buyer_intent_open`, unique `migration_no`, unique `(migration_id,buyer_id)`, indexes on `buyer_id`, `status`, `recovery_expires_at`, and `created_at`; it must assert `0010_buyer_identity_migration.down.sql` does not exist.

- [ ] **Step 2: Run migration tests and verify RED**

Run:

```bash
cd backend && go test ./internal/model ./migrations -run 'BuyerIdentityMigration' -count=1 -v
```

Expected: FAIL because the models and `0010` files do not exist.

- [ ] **Step 3: Add the exact GORM models**

Implement focused structs with these fields and no secret-bearing plaintext field:

```go
type BuyerIdentityMigration struct {
	ID                   uint64 `gorm:"primaryKey"`
	MigrationNo          string `gorm:"size:32;not null;uniqueIndex"`
	Provider             string `gorm:"size:16;not null"`
	CanonicalBuyerID     uint64 `gorm:"not null;index"`
	Status               string `gorm:"size:16;not null;index"`
	CreatedByAdminID     uint64 `gorm:"not null;index"`
	RecoveryTokenHash    *string `gorm:"size:64"`
	RecoveryExpiresAt    *time.Time `gorm:"index"`
	RecoveryUsedAt       *time.Time
	LastAttemptAt        *time.Time
	LastFailureCode      *string `gorm:"size:32"`
	FavoritesMerged      uint64 `gorm:"not null;default:0"`
	HistoriesMerged      uint64 `gorm:"not null;default:0"`
	DeviceBindingsMerged uint64 `gorm:"not null;default:0"`
	IntentsMoved         uint64 `gorm:"not null;default:0"`
	SourceBuyersDisabled uint64 `gorm:"not null;default:0"`
	CompletedAt          *time.Time
	CancelledAt          *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type BuyerIdentityMigrationMember struct {
	ID          uint64 `gorm:"primaryKey"`
	MigrationID uint64 `gorm:"not null;uniqueIndex:uk_buyer_identity_migration_member,priority:1;index"`
	BuyerID     uint64 `gorm:"not null;uniqueIndex:uk_buyer_identity_migration_member,priority:2;index"`
	MemberRole  string `gorm:"size:16;not null"`
	CreatedAt   time.Time
}

type BuyerIdentityMigrationEvent struct {
	ID          uint64 `gorm:"primaryKey"`
	MigrationID uint64 `gorm:"not null;index"`
	EventType   string `gorm:"size:24;not null"`
	ActorType   string `gorm:"size:16;not null"`
	ActorID     *uint64
	ResultCode  string `gorm:"size:32;not null"`
	CreatedAt   time.Time `gorm:"index"`
}
```

Add explicit `TableName()` methods and register the three models after `BuyerIntent` in `migrate`.

- [ ] **Step 4: Write guarded preflight/up/postflight SQL**

The preflight must use `information_schema` to reject:

```text
missing buyer_users, admin_users, buyer_intents, or auth_sessions
missing/generated-expression drift for buyer_intents.open_marker
missing/key-column drift for uk_buyer_intent_open
one or two existing 0010 tables instead of none or all three
existing table column/index/check/FK drift from the final 0010 shape
```

The up migration creates all three InnoDB/utf8mb4 tables with foreign keys to the migration row and referenced buyer/admin rows, exact status/role checks, and no cascade delete. The postflight counts exact columns, primary/unique/index keys, check constraints, and foreign keys and fails unless every expected count matches.

- [ ] **Step 5: Run GREEN and verify AutoMigrate agreement**

Run:

```bash
cd backend && gofmt -w internal/model/models.go internal/model/models_test.go internal/app/server.go migrations/buyer_identity_migration_test.go
cd backend && go test ./internal/model ./internal/app ./migrations -run 'BuyerIdentityMigration|ModelSchema' -count=1
git diff --check
```

Expected: all exit 0; SQLite/GORM exposes the three tables and expected indexes, and no down script exists.

- [ ] **Step 6: Commit the additive schema**

```bash
git add backend/internal/model/models.go backend/internal/model/models_test.go \
  backend/internal/app/server.go backend/migrations/0010_buyer_identity_migration.preflight.sql \
  backend/migrations/0010_buyer_identity_migration.up.sql \
  backend/migrations/0010_buyer_identity_migration.postflight.sql \
  backend/migrations/buyer_identity_migration_test.go
git commit -m "feat(buyer): add identity migration control schema"
```

---

### Task 4: Build the SUPER_ADMIN task and recovery control plane

**Files:**
- Modify: `backend/internal/dto/dto.go`
- Create: `backend/internal/app/buyer_identity_migration.go`
- Create: `backend/internal/app/buyer_identity_migration_handlers.go`
- Create: `backend/internal/app/buyer_identity_migration_test.go`
- Modify: `backend/internal/app/server.go`

**Interfaces:**
- Consumes: the three Task 3 models, F-14 current actor state, `common.BuildBizNo`, `common.SHA256`, `crypto/rand`, and GORM row locking.
- Produces:

```go
type BuyerIdentityMigrationCreateRequest struct {
	Provider         string   `json:"provider" binding:"required"`
	CanonicalBuyerID uint64   `json:"canonical_buyer_id" binding:"required"`
	SourceBuyerIDs   []uint64 `json:"source_buyer_ids" binding:"required,min=1"`
}

type BuyerIdentityMigrationCompleteRequest struct {
	Provider string `json:"provider" binding:"required"`
	Code     string `json:"code" binding:"required"`
}

type BuyerIdentityMigrationRecoverRequest struct {
	Provider      string `json:"provider" binding:"required"`
	Code          string `json:"code" binding:"required"`
	RecoveryToken string `json:"recovery_token" binding:"required"`
}

func (s *Server) createBuyerIdentityMigration(tx *gorm.DB, actor common.Actor, req dto.BuyerIdentityMigrationCreateRequest, now time.Time) (model.BuyerIdentityMigration, error)
func (s *Server) issueBuyerIdentityRecoveryToken(tx *gorm.DB, actor common.Actor, migrationID uint64, now time.Time) (string, error)
func (s *Server) cancelBuyerIdentityMigration(tx *gorm.DB, actor common.Actor, migrationID uint64, now time.Time) error
```

- [ ] **Step 1: Write RED authorization and task-validation tests**

Create focused tests for SUPER_ADMIN success and explicit failures for ADMIN, merchant, buyer, and anonymous actors. Add cases for missing buyer, duplicate ID, canonical repeated as source, cross-provider member, non-mock prefix, disabled/deleted member, zero or multiple canonical roles, overlapping PENDING task, and a buyer reused after the previous task is CANCELLED.

Concurrency test shape:

```go
func TestCreateBuyerIdentityMigrationOverlappingMembershipHasOneWinner(t *testing.T) {
	// Two transactions propose the same source buyer in different tasks.
	// Start them together, require one PENDING task and one conflict.
}
```

- [ ] **Step 2: Run task tests and verify RED**

Run:

```bash
cd backend && go test ./internal/app -run 'BuyerIdentityMigration.*(Authorization|Create|Membership|Cancel)' -count=1 -v
```

Expected: FAIL because DTOs, service, handlers, and routes do not exist.

- [ ] **Step 3: Implement immutable membership under stable row locks**

Normalize and deduplicate all IDs in memory, sort ascending, then lock `buyer_users` with `clause.Locking{Strength: "UPDATE"}`. Validate every database row rather than request labels. Use exact mock prefixes `mock_wx_` for WeChat and `mock_tt_` for Douyin. Insert the task, one CANONICAL member, SOURCE members, and mandatory CREATED event in the same transaction.

The read response may return IDs, provider, roles, state, counts, and timestamps; it must omit OpenID, UnionID, recovery-token hash, and every profile field.

- [ ] **Step 4: Write RED recovery-token tests**

Add tests proving 32 random bytes are base64url encoded, only a 64-character SHA-256 hex digest is stored, expiry is exactly `now.Add(15*time.Minute)`, reissue invalidates the previous digest, cancellation blocks issue, and the plaintext token appears only in the successful issue response.

- [ ] **Step 5: Implement recovery issue and cancellation events**

Use:

```go
raw := make([]byte, 32)
if _, err := rand.Read(raw); err != nil {
	return "", common.ErrInternal
}
token := base64.RawURLEncoding.EncodeToString(raw)
digest := common.SHA256(token)
expiresAt := now.Add(15 * time.Minute)
```

Lock the task, require PENDING, replace its digest/expiry, clear `recovery_used_at`, and insert RECOVERY_ISSUED atomically. Cancellation locks the task, requires PENDING, clears all recovery fields, marks CANCELLED, and inserts CANCELLED atomically. Mandatory event insertion errors abort the transaction.

- [ ] **Step 6: Register exact HTTP routes and role checks**

Add authenticated SUPER_ADMIN-only routes:

```go
admin.POST("/buyer-identity-migrations", s.handleCreateBuyerIdentityMigration)
admin.GET("/buyer-identity-migrations/:id", s.handleGetBuyerIdentityMigration)
admin.POST("/buyer-identity-migrations/:id/recovery-token", s.handleIssueBuyerIdentityRecoveryToken)
admin.POST("/buyer-identity-migrations/:id/cancel", s.handleCancelBuyerIdentityMigration)
```

The handler must reject current role other than `model.AdminRoleSuper` with HTTP 403/code `10003`. Do not rely on the role originally embedded in a stale token; F-14 middleware supplies the current role.

- [ ] **Step 7: Run GREEN, format, and commit**

Run:

```bash
cd backend && gofmt -w internal/dto/dto.go internal/app/buyer_identity_migration.go \
  internal/app/buyer_identity_migration_handlers.go internal/app/buyer_identity_migration_test.go internal/app/server.go
cd backend && go test ./internal/app -run 'BuyerIdentityMigration.*(Authorization|Create|Membership|Recovery|Cancel)' -count=1
git diff --check
```

Expected: all exit 0.

```bash
git add backend/internal/dto/dto.go backend/internal/app/buyer_identity_migration.go \
  backend/internal/app/buyer_identity_migration_handlers.go \
  backend/internal/app/buyer_identity_migration_test.go backend/internal/app/server.go
git commit -m "feat(buyer): add identity migration control plane"
```

---

### Task 5: Commit the complete merge and replacement session atomically

**Files:**
- Modify: `backend/internal/app/auth_handlers.go`
- Modify: `backend/internal/app/auth_handlers_test.go`
- Modify: `backend/internal/app/buyer_identity_migration.go`
- Modify: `backend/internal/app/buyer_identity_migration_handlers.go`
- Modify: `backend/internal/app/buyer_identity_migration_test.go`
- Modify: `backend/internal/app/server.go`

**Interfaces:**
- Consumes: `miniProgramIdentity`, Task 4 PENDING task/membership, F-11 uniqueness, owner-key helpers, F-14 session rules, and `auth.BuildAccessToken`/`auth.BuildRefreshToken`.
- Produces:

```go
func (s *Server) issueTokensWithDB(
	db *gorm.DB,
	ip, userType string,
	userID uint64,
	role string,
	merchantID uint64,
	scope string,
	now time.Time,
) (gin.H, error)

type buyerIdentityCompletionCredential struct {
	ActorSessionID *uint64
	ActorBuyerID   *uint64
	RecoveryHash   *string
}

func (s *Server) completeBuyerIdentityMigration(
	tx *gorm.DB,
	migrationID uint64,
	identity miniProgramIdentity,
	credential buyerIdentityCompletionCredential,
	ip string,
	now time.Time,
) (gin.H, error)
```

- [ ] **Step 1: Write RED transaction-aware token tests**

Test that `issueTokensWithDB` creates the session and refresh hash on the supplied transaction, uses the supplied `now` for expiry, and leaves zero session rows when the outer transaction rolls back. Keep `issueTokens(c, ...)` as a wrapper that passes `s.DB`, `c.ClientIP()`, and `time.Now()`.

- [ ] **Step 2: Run token tests and verify RED**

Run:

```bash
cd backend && go test ./internal/app -run 'IssueTokensWithDB' -count=1 -v
```

Expected: FAIL because the helper does not exist and current issuance writes through `s.DB`.

- [ ] **Step 3: Implement transaction-aware issuance**

Move every session create/update in `issueTokens` to the provided `db`. Build refresh and access tokens only after the session ID exists; return `common.ErrInternal` for any create, token-build, or hash-update error. The wrapper must preserve all existing admin/merchant/buyer behavior.

- [ ] **Step 4: Write RED merge-preflight and exact-aggregate tests**

Build a fixture with one canonical and two sources containing:

```text
favorite overlap: active + inactive rows for the same product
history overlap: different first/last timestamps and counts 2, 3, and 5
device overlap: duplicate and distinct devices with different bind times
intents: multiple closed rows for one product and one open row total
sessions: active and already-revoked rows for all members
profile: canonical nickname/avatar/phone and different source values
```

Assert final active canonical rows, `view_count=10`, min/max history timestamps, source favorite/history merge markers, stable intent numbers and fields, canonical profile preservation, real identity assignment, SOURCE status DISABLED, all old sessions revoked, one new active canonical session, exact counters, SUCCEEDED state, consumed recovery credential, and one COMPLETED event.

Add a conflict fixture with two open intents for the same product across members and assert no business row changes.

- [ ] **Step 5: Run merge tests and verify RED**

Run:

```bash
cd backend && go test ./internal/app -run 'BuyerIdentityMigration.*(Merge|OpenIntent|SessionHandoff)' -count=1 -v
```

Expected: FAIL because completion is not implemented.

- [ ] **Step 6: Implement locked revalidation and all preflight checks**

Inside the transaction, in this exact order:

```text
lock migration row
require PENDING
load and lock all members and buyer rows in ascending buyer ID
revalidate canonical/member/provider/mock-prefix/active/not-deleted invariants
revalidate exact active session identity or recovery digest/expiry/unused state
reject mock/blank verified identity
lock/check any existing owner of the verified provider/OpenID
count open intents grouped by product across members; reject count > 1
```

Set `last_attempt_at` only inside the successful transaction. After a rollback, update only categorical `last_failure_code` in a separate conditional `WHERE id=? AND status='PENDING'` statement; never persist raw database/platform text.

- [ ] **Step 7: Implement deterministic merge helpers**

Add focused unexported helpers in `buyer_identity_migration.go`:

```go
func mergeBuyerIdentityFavorites(tx *gorm.DB, canonicalID uint64, memberIDs []uint64, now time.Time) (uint64, error)
func mergeBuyerIdentityHistories(tx *gorm.DB, canonicalID uint64, memberIDs []uint64, now time.Time) (uint64, error)
func mergeBuyerIdentityDeviceBindings(tx *gorm.DB, canonicalID uint64, memberIDs []uint64, now time.Time) (uint64, error)
func moveBuyerIdentityIntents(tx *gorm.DB, canonicalID uint64, memberIDs []uint64) (uint64, error)
```

Lock source rows in primary-key order. For favorites use `ownerKeyForBuyer(canonicalID)` and preserve product merchant ownership; active wins. For histories use min first, max last, summed count, and active wins. For devices preserve minimum first bind and maximum last bind/merge time, upsert canonical before deleting source bindings. Reassign intents without changing any field except `buyer_id`; F-11 permits closed history and preflight guarantees at most one open target.

- [ ] **Step 8: Complete identity/session/event state and expose buyer routes**

After merge helpers:

```text
update canonical auth_provider/openid/unionid only
disable each SOURCE buyer
revoke every currently active member auth session
issue one new canonical session using issueTokensWithDB
persist exact counters and SUCCEEDED/completed_at/recovery_used_at
insert mandatory COMPLETED event
```

Register:

```go
buyerAuth.POST("/auth/identity-migrations/:id/complete", s.handleCompleteBuyerIdentityMigration)
buyer.POST("/auth/identity-migrations/:id/recover", s.handleRecoverBuyerIdentityMigration)
```

The session endpoint requires current actor membership. The recovery endpoint uses no actor and applies exactly `5` attempts per IP per 15 minutes and `5` attempts per task per 15 minutes. The authenticated completion endpoint applies `10` attempts per buyer and per task per 15 minutes. Both call `resolveRealMiniProgramIdentity`, compare provider to the task, and never accept profile or device fields.

- [ ] **Step 9: Run GREEN, regression tests, format, and commit**

Run:

```bash
cd backend && gofmt -w internal/app/auth_handlers.go internal/app/auth_handlers_test.go \
  internal/app/buyer_identity_migration.go internal/app/buyer_identity_migration_handlers.go \
  internal/app/buyer_identity_migration_test.go internal/app/server.go
cd backend && go test ./internal/app -run 'IssueTokensWithDB|BuyerIdentityMigration' -count=1
cd backend && go test ./tests -run 'BuyerFlow|SessionRevocation' -count=1
git diff --check
```

Expected: all exit 0.

```bash
git add backend/internal/app/auth_handlers.go backend/internal/app/auth_handlers_test.go \
  backend/internal/app/buyer_identity_migration.go \
  backend/internal/app/buyer_identity_migration_handlers.go \
  backend/internal/app/buyer_identity_migration_test.go backend/internal/app/server.go
git commit -m "fix(buyer): migrate identities atomically"
```

---

### Task 6: Prove API, retry, concurrency, and rollback behavior

**Files:**
- Create: `backend/tests/buyer_identity_migration_test.go`
- Create: `backend/tests/buyer_identity_migration_mysql_test.go`
- Modify: `backend/internal/app/buyer_identity_migration_test.go`

**Interfaces:**
- Consumes: Tasks 2-5 public routes and service contracts.
- Produces: executable evidence for every F-12 HTTP/error/atomicity/concurrency acceptance criterion.

- [ ] **Step 1: Add the RED end-to-end HTTP matrix**

Use real Gin, middleware, SQLite models, and `httptest` platform exchange. Cover exact HTTP/business codes:

```text
disabled provider: 403 / 10003
migration-mode unbound login: 409 / 10010
missing task or nonmember actor: 404 / 10004
invalid/expired/used recovery token: 401 / 10002
disabled member: 403 / 10007
identity owned by another buyer: 409 / 10010
duplicate open target intent: 409 / 10010
cancelled/succeeded retry: 409 / 10010
invariant/database failure: 500 / 20001
```

Assert every failure leaves buyer, ownership, intent, session, task, and event state unchanged except an allowed categorical pending-task failure code.

- [ ] **Step 2: Add concurrent completion and response-loss tests**

Implement tests named:

```go
func TestBuyerIdentityMigrationConcurrentSessionCompletionHasOneWinner(t *testing.T) {}
func TestBuyerIdentityMigrationSessionRecoveryRaceHasOneWinner(t *testing.T) {}
func TestBuyerIdentityMigrationResponseLossThenRealLoginUsesCanonicalBuyer(t *testing.T) {}
func TestBuyerIdentityMigrationSessionRevokedDuringExchangeRollsBack(t *testing.T) {}
```

Synchronize platform-exchange handlers with channels, never `time.Sleep`. Require one successful completion, one conflict/unauthorized loser, exactly one COMPLETED event, no duplicate canonical owner rows, and no second buyer for the real identity.

- [ ] **Step 3: Inject failures at every irreversible boundary**

Use scoped GORM callbacks that return a categorical synthetic error before writes to `buyer_users`, `auth_sessions`, and `buyer_identity_migration_events`. Add a service test seam for merge-stage failure with a closed enum, not arbitrary SQL/error text. Cover failures before identity replacement, source disablement, session revocation, replacement-session create, and COMPLETED-event insert. Compare normalized database snapshots before/after and require equality.

- [ ] **Step 4: Add the opt-in MySQL test**

Guard the test before connecting:

```go
if os.Getenv("BUYER_IDENTITY_MIGRATION_MYSQL_TEST") != "1" {
	t.Skip("set BUYER_IDENTITY_MIGRATION_MYSQL_TEST=1 for isolated MySQL acceptance")
}
if os.Getenv("ACCEPTANCE_DB_ENGINE") != "mysql8.4" {
	t.Fatal("ACCEPTANCE_DB_ENGINE must be mysql8.4")
}
if os.Getenv("COMPOSE_PROJECT_NAME") != "secondhand-buyer-identity-acceptance" {
	t.Fatal("unexpected Compose project")
}
```

The test uses only synthetic mock/real identities and proves migration-only startup with `AUTO_MIGRATE=false` and agreement with `AUTO_MIGRATE=true`, row-lock one-winner behavior, open-intent conflict, exact aggregates, rollback snapshots, and useful EXPLAIN indexes.

- [ ] **Step 5: Run focused, full, race, and vet gates**

Run:

```bash
cd backend && gofmt -w internal/app/buyer_identity_migration_test.go \
  tests/buyer_identity_migration_test.go tests/buyer_identity_migration_mysql_test.go
cd backend && go test ./internal/app ./tests -run 'BuyerIdentityMigration' -count=1 -v
cd backend && go test ./... -count=1
cd backend && go test -race ./internal/app ./tests -count=1
cd backend && go vet ./...
git diff --check
```

Expected: all local commands exit 0; the opt-in MySQL test skips locally unless the exact isolated environment is present.

- [ ] **Step 6: Commit the regression matrix**

```bash
git add backend/internal/app/buyer_identity_migration_test.go \
  backend/tests/buyer_identity_migration_test.go \
  backend/tests/buyer_identity_migration_mysql_test.go
git commit -m "test(buyer): cover identity migration boundaries"
```

---

### Task 7: Build the guarded MySQL 8.4 acceptance harness

**Files:**
- Create: `backend/tests/buyer_identity_acceptance_contract_test.go`
- Create: `deploy/acceptance/buyer-identity-migration-smoke.sh`
- Modify: `deploy/acceptance/docker-compose.yml`
- Modify: `deploy/acceptance/README.md`
- Modify: `Makefile`

**Interfaces:**
- Consumes: the complete `0001..0010` migration chain and Task 6 MySQL test.
- Produces: `make acceptance-buyer-identity-migration-smoke` for dedicated Compose project `secondhand-buyer-identity-acceptance`.

- [ ] **Step 1: Write the RED pre-Docker guard contract**

Use a stub `docker` binary and assert no Docker invocation unless all exact values match:

```text
BUYER_IDENTITY_MIGRATION_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_IDENTITY_DATA
ACCEPTANCE_DB_ENGINE=mysql8.4
COMPOSE_PROJECT_NAME unset or secondhand-buyer-identity-acceptance
```

Also assert the script refuses existing containers, volumes, networks, or evidence for that project and never accepts `secondhand-market` as project name.

- [ ] **Step 2: Run guard tests and verify RED**

Run:

```bash
cd backend && go test ./tests -run 'BuyerIdentityAcceptanceRejectsUnsafeEnvironmentBeforeDocker' -count=1 -v
```

Expected: FAIL because the harness does not exist.

- [ ] **Step 3: Implement the isolated harness**

Follow the existing session-revocation harness pattern with these exact changes:

```text
project_name=secondhand-buyer-identity-acceptance
evidence_dir=deploy/acceptance/evidence/buyer-identity-migration
remote directory=/home/yu/services/secondhand-buyer-identity-acceptance-20260727
focused env=BUYER_IDENTITY_MIGRATION_MYSQL_TEST=1
focused test=^TestBuyerIdentityMigrationMySQLAcceptance$
migration chain includes 0009 and 0010 preflight/up/postflight
```

Create a 0700 temporary build context from only `Makefile`, `backend/Dockerfile`, `backend/go.mod`, `backend/go.sum`, Go sources, migration SQL, and non-secret acceptance scripts/config/docs. Explicitly exclude `.env`, secrets, evidence, `.git`, caches, uploads, `node_modules`, `backend/app.db`, `.tmp/`, and the three protected review documents.

Collect only sanitized command summaries, MySQL version, source SHA-256, migration markers, test names/PASS lines, EXPLAIN access/key summaries, and production container identity/state/restart counts before/after. Scan evidence for credentials, tokens, codes, raw identity values, IDs, DSNs, passwords, and JWT-shaped text; fail if any match.

- [ ] **Step 4: Add Compose test settings and Make guard**

The tools container uses `APP_ENV=test`, both providers `mock`, and no real credentials. The production-mode `api` service must not be started by this harness because its normal acceptance mock configuration is intentionally invalid under F-12. Add:

```make
acceptance-buyer-identity-migration-smoke:
	@test "$${BUYER_IDENTITY_MIGRATION_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_IDENTITY_DATA" || { echo "set BUYER_IDENTITY_MIGRATION_ACCEPTANCE_CONFIRM for isolated buyer identity tests" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	./deploy/acceptance/buyer-identity-migration-smoke.sh
```

- [ ] **Step 5: Run local contract and shell checks**

Run:

```bash
bash -n deploy/acceptance/buyer-identity-migration-smoke.sh
cd backend && go test ./tests ./migrations -run 'BuyerIdentityAcceptance|BuyerIdentityMigrationArtifacts' -count=1
git diff --check
```

Expected: all exit 0. Do not run Docker or transfer source in this task.

- [ ] **Step 6: Commit the harness**

```bash
git add backend/tests/buyer_identity_acceptance_contract_test.go \
  deploy/acceptance/buyer-identity-migration-smoke.sh \
  deploy/acceptance/docker-compose.yml deploy/acceptance/README.md Makefile
git commit -m "test(acceptance): guard buyer identity migration"
```

---

### Task 8: Document code-side operation and status

**Files:**
- Create: `docs/runbooks/buyer-identity-migration.md`
- Create: `docs/superpowers/reviews/2026-07-27-buyer-identity-migration-code-review.md`
- Modify: `docs/full-project-code-review-2026-07-24.md`
- Modify: `docs/release-readiness.md`

**Interfaces:**
- Consumes: Tasks 1-7 exact commits and fresh local verification output.
- Produces: traceable code-side status without claiming server, real-platform, or production closure.

- [ ] **Step 1: Write the non-secret runbook**

Document the fixed order:

```text
verify approved F-11 and F-12 commit hashes
verify production read-only inventory and recoverable backup under separate approval
configure WeChat=migration and Douyin=disabled without recording secrets
prepare one explicitly approved user task
verify old-session or separately delivered recovery possession
complete with a fresh real platform code
verify canonical ID, exact aggregate counts, disabled sources, revoked old sessions, and new session
stop on mismatch; use forward repair only
repeat for the second explicitly approved user
switch WeChat to real only after no approved user remains and no mock creation is observed
```

State that the runbook itself authorizes none of these production actions.

- [ ] **Step 2: Create the code review report from fresh evidence**

Record branch, base/head commits, changed-file whitelist, exact test commands/counts, race/vet results, migration artifact checks, forbidden-path scan, and unresolved gates. Use these exact status lines until remote work occurs:

```text
代码侧状态：已修复
隔离 MySQL 8.4 测试服务器状态：未审核
真实平台测试环境状态：未审核
生产状态：未迁移、未部署、未修改生产买家或 session
```

- [ ] **Step 3: Append tracked status without rewriting history**

Append a dated F-12 follow-up to `docs/full-project-code-review-2026-07-24.md` and update `docs/release-readiness.md` to the same four independent states. Preserve the original finding text and do not touch any protected review document.

- [ ] **Step 4: Verify docs and commit**

Run:

```bash
rg -n '代码侧状态|隔离 MySQL 8.4|真实平台测试环境状态|生产状态' \
  docs/runbooks/buyer-identity-migration.md \
  docs/superpowers/reviews/2026-07-27-buyer-identity-migration-code-review.md \
  docs/full-project-code-review-2026-07-24.md docs/release-readiness.md
git diff --check
```

Expected: the four states are explicit and no document claims a remote, real-platform, or production pass.

```bash
git add docs/runbooks/buyer-identity-migration.md \
  docs/superpowers/reviews/2026-07-27-buyer-identity-migration-code-review.md \
  docs/full-project-code-review-2026-07-24.md docs/release-readiness.md
git commit -m "docs(buyer): record identity migration code closure"
```

---

### Task 9: Run separately authorized isolated and real-platform gates

**Files:**
- Create after successful authorized run: `docs/superpowers/reviews/2026-07-27-buyer-identity-migration-isolated-acceptance.md`
- Modify after successful authorized run: `docs/full-project-code-review-2026-07-24.md`
- Modify after successful authorized run: `docs/release-readiness.md`

**Interfaces:**
- Consumes: an exact later transfer authorization and the Task 7 guard values.
- Produces: isolated MySQL review status and, under a different authorization, real-platform test-environment status.

- [ ] **Step 1: Stop and request the exact source-transfer authorization**

The request must name only:

```text
source whitelist: backend/, deploy/acceptance/, Makefile
remote directory: /home/yu/services/secondhand-buyer-identity-acceptance-20260727
Compose project: secondhand-buyer-identity-acceptance
purpose: isolated MySQL 8.4 synthetic buyer identity acceptance only
explicit exclusions: .env, keys, databases, uploads, evidence, .git, caches,
node_modules, backend/app.db, .tmp/, and the three protected review documents
```

Do not transfer or execute remotely until the user grants that exact authorization.

- [ ] **Step 2: After authorization, verify the remote target before mutation**

Read only remote path type/ownership, project-label collisions, Docker/MySQL availability, free space, and production container identity/state/restart counts. Refuse a symlink, non-empty target, existing project resource, wrong MySQL image/version, or target outside the exact directory.

- [ ] **Step 3: Transfer the whitelist and run the isolated gate**

Set the exact guard values and run:

```bash
BUYER_IDENTITY_MIGRATION_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_IDENTITY_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
COMPOSE_PROJECT_NAME=secondhand-buyer-identity-acceptance \
make acceptance-buyer-identity-migration-smoke
```

Expected: MySQL `8.4.x`, migration-only and AutoMigrate modes pass, concurrency has one winner, rollback snapshots match, source hashes match local, evidence scan has zero forbidden matches, and production container snapshots are byte-identical.

- [ ] **Step 4: Inspect retained isolated resources and record sanitized evidence**

Review `docker compose ps`, project labels, volumes, logs with secret filters, evidence hashes, and production snapshots. Record the remote path, Compose project, MySQL version, source/evidence hashes, test names/counts, and unchanged production snapshot in the review document. Do not commit the evidence directory.

Only now update status to:

```text
代码侧状态：已修复
隔离 MySQL 8.4 测试服务器状态：审核通过
真实平台测试环境状态：未审核
生产状态：未迁移、未部署、未修改生产买家或 session
```

- [ ] **Step 5: Commit isolated-review status**

```bash
git add docs/superpowers/reviews/2026-07-27-buyer-identity-migration-isolated-acceptance.md \
  docs/full-project-code-review-2026-07-24.md docs/release-readiness.md
git commit -m "docs(buyer): record isolated identity acceptance"
```

- [ ] **Step 6: Stop for separate real-platform authorization and credentials**

Real WeChat/Douyin test-environment credentials and real-device codes are outside source, shell history, and retained evidence. Request a separate authorization defining provider, test environment, credential injection method, test accounts/devices, and evidence redaction. The real-platform gate proves iOS and Android code exchange, migration-mode no-create, approved completion, response-loss login recovery, and no secret/identity leakage.

Do not mark production F-12 closed after this gate. Production backup, task membership for each real user, deployment, identity mutation, and mode switch remain separately authorized maintenance actions.

---

### Task 10: Perform final F-12 review and return to F-15/F-10

**Files:**
- Modify only if findings require it: files listed in Tasks 2-9
- Verify: entire F-12 commit range

**Interfaces:**
- Consumes: every available local/isolated/real-platform result and precise remaining gates.
- Produces: a review-ready F-12 commit range, then resumes F-15 design/specification work; F-10 history rewrite remains last and unauthorized.

- [ ] **Step 1: Run fresh verification from the review head**

Run:

```bash
cd backend && go test ./... -count=1
cd backend && go test -race ./internal/app ./tests -count=1
cd backend && go vet ./...
bash -n deploy/acceptance/buyer-identity-migration-smoke.sh
git diff --check
git status --short
```

Expected: all commands exit 0; only known protected untracked paths remain.

- [ ] **Step 2: Audit changed paths and secrets**

Run against the exact F-12 base commit recorded in the code review:

```bash
f12_policy_commit="$(git log -1 --format=%H --grep='^fix(buyer): fail closed on production identity modes$')"
test -n "$f12_policy_commit"
f12_base_commit="$(git rev-parse "$f12_policy_commit^")"
git diff --name-only "$f12_base_commit"..HEAD
git diff --check "$f12_base_commit"..HEAD
git diff --binary "$f12_base_commit"..HEAD -- backend/app.db
git diff "$f12_base_commit"..HEAD | rg -n '(APP_SECRET|ACCESS_TOKEN|REFRESH_TOKEN|RECOVERY_TOKEN|BEGIN [A-Z ]*PRIVATE KEY|eyJ[A-Za-z0-9_-]+\.)'
```

Expected: only planned source/test/docs paths, no `backend/app.db` content diff, and no committed secret value. The resolved base is the parent of the first F-12 implementation commit and must match the code-review record.

- [ ] **Step 3: Request code review and resolve every finding**

Use `superpowers:requesting-code-review` for the full F-12 range. For each actionable finding, use `superpowers:receiving-code-review`, reproduce it with a failing test, fix it, rerun focused/full/race/vet gates, and append the review resolution to the code-review document.

- [ ] **Step 4: Reconcile statuses without overclaiming**

The final handoff must state exact commit hashes and each of the four independent statuses. A missing isolated authorization leaves the server status `未审核`; a missing real-platform authorization leaves that status `未审核`; absent production maintenance evidence leaves production `未迁移、未部署、未修改生产买家或 session`.

- [ ] **Step 5: Continue the first-round sequence**

Proceed to the already architecture-approved F-15 design sections and written specification. Keep F-10 for the final operation: its architecture approval does not authorize history rewriting, documentation hash remapping, ref updates, reflog expiry, object cleanup, or force-push.

---

## Plan Self-Review Record

- Spec coverage: Tasks 2-10 cover all F-12 sections: mode policy, provider exchange, control/audit schema, SUPER_ADMIN authorization, recovery, exact merge contracts, F-11 dependency, session handoff, retries/concurrency, rollback, acceptance, deployment runbook, and four-way status tracking.
- Placeholder scan: clean; every implementation, test, command, path, interface, and authorization boundary is concrete.
- Type consistency: `miniProgramIdentity`, `issueTokensWithDB`, the three migration models, DTO names, completion credential, route paths, mode/status constants, migration number `0010`, remote path, and Compose project are consistent across tasks.
- Scope check: F-11 is a hard prerequisite; F-15 and F-10 remain separate sub-projects. No F-10 destructive history action is included or authorized.
