# F-06 Anonymous Upload Resource Governance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close F-06 by enforcing one 10 MiB file contract, database-serialized anonymous/merchant/global upload limits, trusted HMAC source identifiers, and bounded cleanup that can never select historical or bound files.

**Architecture:** Add nullable governance metadata to canonical `file_records` and serialize every quota-increasing transaction through one `file_quota_guards` row. Keep HTTP parsing, quota reservation, and cleanup in focused `internal/app` units; use SQLite for deterministic behavior tests and a dedicated MySQL 8.4 Compose project for lock/isolation, migration, proxy, and cleanup acceptance.

**Tech Stack:** Go 1.22, Gin, GORM, SQLite, MySQL 8.4, React 18, TypeScript 5.7, Vitest 2, Nginx, Docker Compose, Bash

## Global Constraints

- The business file limit is exactly `10 * 1024 * 1024 = 10,485,760` bytes at the frontend, presign, multipart file, actual-read, and processor boundaries.
- The multipart request-body limit is exactly `11 * 1024 * 1024 = 11,534,336` bytes; it exists only for multipart overhead and is never user-facing.
- Anonymous defaults are 20 successful presigns/source/hour, 5 active files/source, and 50 MiB active bytes/source.
- Merchant quota is 2 GiB per `owner_merchant_id`; global quota is 20 GiB across all `file_records`.
- Production must explicitly configure every governance number, `FILE_UPLOAD_IP_HASH_SECRET`, and `TRUSTED_PROXY_CIDRS`; invalid, missing, weak, zero, negative, or overflow values fail startup.
- Store only lowercase HMAC-SHA256 of canonical trusted client IP bytes. Never store or log the raw upload-governance IP and never fall back to plain SHA-256.
- Preserve the F-02 one-time `file_token`, 15-minute capability, constant-time comparison, owner/type/PASS/storage validation, and transaction rollback behavior.
- Cleanup may select only post-`0008` anonymous rows with non-NULL `cleanup_after`, NULL owner, expired grace, and an available/stale claim. Migration-era rows with NULL governance fields are permanently excluded.
- Never automatically delete authenticated uploads, bound files, or the 13 historical production observations. Do not read, mutate, migrate, or deploy production data/files/configuration in this plan.
- Do not modify, stage, commit, or transfer `.tmp/`, the three protected untracked review documents, `.env`, secrets, databases, upload directories, evidence directories, `.git`, caches, `node_modules`, build output, `backend/app.db`, or `miniapp/project.private.config.json`.
- Every behavior task follows RED -> verify RED -> minimal GREEN -> focused regression -> commit. Record the failing test name and failure cause in execution notes.
- Status remains distinct: `代码侧状态`, `测试服务器状态`, and `生产状态`. No design/plan commit closes F-06, and isolated acceptance is not production release.

## File Structure

### New files

| File | Responsibility |
| --- | --- |
| `backend/migrations/0008_anonymous_upload_governance.preflight.sql` | Fail closed before governance DDL on schema/data drift |
| `backend/migrations/0008_anonymous_upload_governance.up.sql` | Add nullable governance fields/indexes and the one-row quota guard |
| `backend/migrations/0008_anonymous_upload_governance.postflight.sql` | Verify exact `0008` shape and historical NULL protection |
| `backend/migrations/anonymous_upload_governance_migration_test.go` | Pin SQL artifacts, non-destructive contract, and acceptance guards |
| `backend/internal/model/upload_governance_test.go` | Pin GORM fields, tags, and guard table name |
| `backend/internal/app/upload_governance.go` | Canonical IP HMAC, quota transaction, guard lock, aggregates, reservation |
| `backend/internal/app/upload_governance_test.go` | SQLite tests for hashing, rate/quota boundaries, rollback, overflow |
| `backend/internal/app/upload_cleanup.go` | Candidate claims, symlink-safe deletion, retries, summaries, scheduler |
| `backend/internal/app/upload_cleanup_test.go` | SQLite/filesystem tests for history protection, races, retries, logging |
| `backend/internal/app/upload_governance_mysql_test.go` | MySQL 8.4 multi-connection lock, cleanup, and migration-only API proof |
| `frontend/src/utils/upload.ts` | Shared 10 MiB constant and deterministic local file validator |
| `frontend/src/utils/upload.test.ts` | Exact 10 MiB and invalid-file unit contract |
| `frontend/src/pages/merchant/products/CreatePage.test.tsx` | Product-create upload boundary integration |
| `frontend/src/pages/merchant/products/EditPage.test.tsx` | Product-edit upload boundary integration |
| `deploy/acceptance/anonymous-upload-governance-smoke.sh` | Fixed-project MySQL/proxy/concurrency/cleanup acceptance harness |
| `docs/superpowers/reviews/2026-07-26-anonymous-upload-governance-isolated-acceptance.md` | Sanitized test-server evidence and three-state F-06 result; create only after the run |

### Modified files

| File | Responsibility in this plan |
| --- | --- |
| `backend/internal/model/models.go` | Persist governance fields and `FileQuotaGuard` |
| `backend/internal/common/errors.go` | Add HTTP 413 upload-too-large and code `10013` quota errors |
| `backend/internal/app/config.go` | Strict governance env parsing and production validation |
| `backend/internal/app/server.go` | AutoMigrate/seed guard, trusted proxies, cleanup lifecycle |
| `backend/internal/app/server_security_test.go` | Production config and trusted-proxy startup tests |
| `backend/internal/app/file_handlers.go` | Reserve presigns and parse capped multipart before form access |
| `backend/internal/app/file_binding.go` | Guarded merchant quota on anonymous capability claim |
| `backend/internal/app/file_binding_test.go` | Binding quota and cleanup-claim regression tests |
| `backend/internal/app/auth_handlers.go` | Use quota-aware READ COMMITTED registration transaction |
| `backend/internal/media/processor.go` | Change default original-size constant to 10 MiB |
| `backend/internal/media/processor_test.go` | Replace historical 40 MiB executable tests with 10 MiB boundary |
| `backend/tests/integration_flow_test.go` | Central test Config defaults/helper |
| `backend/tests/file_upload_test.go` | API size, source HMAC, quota, and multipart regression tests |
| `backend/tests/file_binding_helpers_test.go` | Exact declared-size fixtures and governance fields |
| `backend/tests/file_binding_security_test.go` | Registration quota rollback and capability regression |
| `backend/tests/file_schema_mysql_test.go` | New Config fields for existing migration-only tests |
| `backend/configs/.env.example` | Safe development governance examples |
| `backend/configs/.env.production.mysql.example` | Explicit production governance placeholders/limits |
| `backend/configs/.env.production.sqlite.example` | Explicit production governance placeholders/limits |
| `frontend/src/pages/auth/RegisterPage.tsx` | Reject invalid/oversize license before presign; 10 MiB copy |
| `frontend/src/pages/auth/RegisterPage.test.tsx` | Registration upload size and token-storage regression |
| `frontend/src/pages/merchant/products/CreatePage.tsx` | Shared validator and 10 MiB copy |
| `frontend/src/pages/merchant/products/EditPage.tsx` | Shared validator and 10 MiB copy |
| `deploy/acceptance/prepare.sh` | Generate independent acceptance-only IP HMAC secret |
| `deploy/acceptance/.env.example` | Declare acceptance-only secret without a value |
| `deploy/acceptance/docker-compose.yml` | Explicit production-mode F-06 config in isolated API |
| `deploy/acceptance/nginx.conf` | 11 MiB request cap and JSON 413 envelope |
| `deploy/acceptance/README.md` | Dedicated F-06 command, isolation, retention, and prohibition |
| `Makefile` | Guarded `acceptance-anonymous-upload-governance-smoke` target |
| `README.md` | Current 10 MiB/config contract |
| `docs/backend-api-checklist.md` | Presign/upload errors and limits |
| `docs/data-model.md` | Governance columns, indexes, guard, cleanup semantics |
| `docs/release-readiness.md` | F-06 code/test-server/production state |
| `docs/full-project-code-review-2026-07-24.md` | Dated F-06 follow-up without rewriting original finding |
| `docs/production-hardening-repair-plan-2026-07-24.md` | Trace implementation and remaining production gate |
| `docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md` | Advance approved/implemented state accurately |

---

### Task 1: Persist the governance schema without enrolling historical files

**Files:**
- Create: `backend/internal/model/upload_governance_test.go`
- Create: `backend/migrations/anonymous_upload_governance_migration_test.go`
- Create: `backend/migrations/0008_anonymous_upload_governance.preflight.sql`
- Create: `backend/migrations/0008_anonymous_upload_governance.up.sql`
- Create: `backend/migrations/0008_anonymous_upload_governance.postflight.sql`
- Modify: `backend/internal/model/models.go`
- Modify: `backend/internal/app/server.go`

**Interfaces:**
- Produces `model.FileRecord.SourceIPHash *string`, `CleanupAfter *time.Time`, `CleanupClaimedAt *time.Time`, `CleanupClaimToken *string`, and `CleanupAttempts uint32`.
- Produces `model.FileQuotaGuard{ID uint8, GuardName string, CreatedAt time.Time}` with `TableName() string == "file_quota_guards"`.
- Produces exact indexes `idx_file_source_created(source_ip_hash,created_at)` and `idx_file_cleanup_candidate(uploader_type,owner_merchant_id,cleanup_after,cleanup_claimed_at)`.
- Later tasks require guard row `id=1, guard_name='file_records'` whenever schema migration/AutoMigrate is enabled.

- [ ] **Step 1: Write failing model and SQL artifact tests**

```go
func TestFileRecordHasAnonymousUploadGovernanceFields(t *testing.T) {
    typ := reflect.TypeOf(FileRecord{})
    for _, name := range []string{
        "SourceIPHash", "CleanupAfter", "CleanupClaimedAt",
        "CleanupClaimToken", "CleanupAttempts",
    } {
        if _, ok := typ.FieldByName(name); !ok {
            t.Fatalf("FileRecord missing %s", name)
        }
    }
}

func TestFileQuotaGuardTableName(t *testing.T) {
    if got := (FileQuotaGuard{}).TableName(); got != "file_quota_guards" {
        t.Fatalf("table = %q", got)
    }
}
```

The migration artifact test must require all five columns, both exact index names and ordered columns, SQLSTATE `45000`, `file_quota_guards`, fixed ID `1`, the exact markers `anonymous_upload_governance_preflight_passed`, `anonymous_upload_governance_migration_applied`, and `anonymous_upload_governance_postflight_passed`, and no `0008*.down.sql`. It must reject any `UPDATE file_records SET source_ip_hash` or `UPDATE file_records SET cleanup_after` in the up script.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
cd backend
go test ./internal/model ./migrations -run 'Test(FileRecordHasAnonymousUploadGovernanceFields|FileQuotaGuardTableName|AnonymousUploadGovernanceMigration)' -count=1 -v
```

Expected: FAIL because the model fields, guard type, migration files, and tests do not exist.

- [ ] **Step 3: Add the exact GORM model contract**

Add the following shapes while preserving existing index tags:

```go
type FileRecord struct {
    // existing fields...
    SourceIPHash      *string    `gorm:"type:char(64);index:idx_file_source_created,priority:1"`
    CleanupAfter      *time.Time `gorm:"index:idx_file_cleanup_candidate,priority:3"`
    CleanupClaimedAt  *time.Time `gorm:"index:idx_file_cleanup_candidate,priority:4"`
    CleanupClaimToken *string    `gorm:"type:char(64)"`
    CleanupAttempts   uint32     `gorm:"not null;default:0"`
    CreatedAt         time.Time  `gorm:"index:idx_biz_type_created,priority:2;index:idx_file_source_created,priority:2"`
}

type FileQuotaGuard struct {
    ID        uint8     `gorm:"primaryKey;autoIncrement:false"`
    GuardName string    `gorm:"size:32;not null;uniqueIndex"`
    CreatedAt time.Time `gorm:"not null"`
}

func (FileQuotaGuard) TableName() string { return "file_quota_guards" }
```

Add `idx_file_cleanup_candidate` priority 1 to `UploaderType` and priority 2 to `OwnerMerchantID`; retain `idx_file_owner_biz_scan` on owner.

Include `&model.FileQuotaGuard{}` in `migrate(db)`. After AutoMigrate, insert only the fixed guard with `clause.OnConflict{DoNothing: true}` and fail server startup on any insert/query error.

- [ ] **Step 4: Implement the three-part `0008` migration**

Preflight must verify canonical `file_records`, absent `files`, exact `0006/0007` prerequisites, nonnegative `size_bytes`, and either zero `0008` objects or one complete exact `0008` shape. Any partial/wrong column, index, table, or guard row signals `45000` before DDL.

Up must add exactly:

```sql
ALTER TABLE file_records
  ADD COLUMN source_ip_hash CHAR(64) NULL,
  ADD COLUMN cleanup_after DATETIME(3) NULL,
  ADD COLUMN cleanup_claimed_at DATETIME(3) NULL,
  ADD COLUMN cleanup_claim_token CHAR(64) NULL,
  ADD COLUMN cleanup_attempts INT UNSIGNED NOT NULL DEFAULT 0,
  ADD INDEX idx_file_source_created (source_ip_hash, created_at),
  ADD INDEX idx_file_cleanup_candidate
    (uploader_type, owner_merchant_id, cleanup_after, cleanup_claimed_at);

CREATE TABLE file_quota_guards (
  id TINYINT UNSIGNED NOT NULL,
  guard_name VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_file_quota_guard_name (guard_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO file_quota_guards (id, guard_name) VALUES (1, 'file_records');
```

Implement the already-complete branch as a verified no-op and reject partial state; never blindly rerun `ALTER TABLE`. Postflight must verify exact types/nullability/defaults/order, exact indexes, one guard row, no legacy table, and zero rows older than `file_quota_guards.created_at` with any non-NULL source/cleanup claim field. This timestamp predicate preserves historical proof while allowing a verified complete migration to be rechecked after newer governed rows exist.

- [ ] **Step 5: Run focused tests and formatting**

Run:

```bash
cd backend
gofmt -w internal/model/models.go internal/model/upload_governance_test.go migrations/anonymous_upload_governance_migration_test.go
go test ./internal/model ./migrations -run 'Test(FileRecordHasAnonymousUploadGovernanceFields|FileQuotaGuardTableName|AnonymousUploadGovernanceMigration)' -count=1 -v
```

Expected: PASS. Also run `git diff --check`.

- [ ] **Step 6: Commit the schema unit**

```bash
git add backend/internal/model/models.go \
  backend/internal/model/upload_governance_test.go \
  backend/internal/app/server.go \
  backend/migrations/0008_anonymous_upload_governance.preflight.sql \
  backend/migrations/0008_anonymous_upload_governance.up.sql \
  backend/migrations/0008_anonymous_upload_governance.postflight.sql \
  backend/migrations/anonymous_upload_governance_migration_test.go
git commit -m "feat(files): add upload governance schema"
```

### Task 2: Enforce strict configuration and trusted proxy identity

**Files:**
- Modify: `backend/internal/app/config.go`
- Modify: `backend/internal/app/server.go`
- Modify: `backend/internal/app/server_security_test.go`
- Modify: `backend/tests/integration_flow_test.go`
- Modify: `backend/tests/file_schema_mysql_test.go`
- Modify: `backend/configs/.env.example`
- Modify: `backend/configs/.env.production.mysql.example`
- Modify: `backend/configs/.env.production.sqlite.example`

**Interfaces:**
- Produces Config fields `FileUploadMultipartMaxBytes int64`, `FileUploadIPHashSecret string`, `FileUploadAnonPresignPerHour int64`, `FileUploadAnonActiveFiles int64`, `FileUploadAnonActiveBytes int64`, `FileUploadMerchantQuotaBytes int64`, `FileUploadGlobalQuotaBytes int64`, `FileUploadCleanupInterval time.Duration`, `FileUploadCleanupBatchSize int`, `FileUploadCleanupClaimTTL time.Duration`, `FileUploadCleanupGrace time.Duration`, and `TrustedProxyCIDRs []string`.
- Produces unexported `uploadGovernanceEnvExplicit bool` and `loadErr error`, set only by strict `LoadConfig` parsing; `Config.Validate` returns `loadErr` before checking production requirements.
- Produces test helper `configureTestUploadGovernance(*app.Config)` in package `tests`, used by every direct Config literal.
- `NewServer` calls `Router.SetTrustedProxies(cfg.TrustedProxyCIDRs)` and fails startup on invalid values; empty slice means trust no proxy.

The environment-to-field mapping is fixed:

| Environment variable | Config field / unit |
| --- | --- |
| `FILE_UPLOAD_MAX_MB` | `FileUploadMaxBytes`, MiB -> bytes |
| `FILE_UPLOAD_MULTIPART_MAX_MB` | `FileUploadMultipartMaxBytes`, MiB -> bytes |
| `FILE_UPLOAD_IP_HASH_SECRET` | `FileUploadIPHashSecret`, raw secret string |
| `FILE_UPLOAD_ANON_PRESIGN_PER_HOUR` | `FileUploadAnonPresignPerHour`, count |
| `FILE_UPLOAD_ANON_ACTIVE_FILES` | `FileUploadAnonActiveFiles`, count |
| `FILE_UPLOAD_ANON_ACTIVE_MB` | `FileUploadAnonActiveBytes`, MiB -> bytes |
| `FILE_UPLOAD_MERCHANT_QUOTA_MB` | `FileUploadMerchantQuotaBytes`, MiB -> bytes |
| `FILE_UPLOAD_GLOBAL_QUOTA_MB` | `FileUploadGlobalQuotaBytes`, MiB -> bytes |
| `FILE_UPLOAD_CLEANUP_INTERVAL_SECONDS` | `FileUploadCleanupInterval`, seconds -> duration |
| `FILE_UPLOAD_CLEANUP_BATCH_SIZE` | `FileUploadCleanupBatchSize`, count |
| `FILE_UPLOAD_CLEANUP_CLAIM_TTL_SECONDS` | `FileUploadCleanupClaimTTL`, seconds -> duration |
| `FILE_UPLOAD_CLEANUP_GRACE_SECONDS` | `FileUploadCleanupGrace`, seconds -> duration |
| `TRUSTED_PROXY_CIDRS` | `TrustedProxyCIDRs`, `none` or validated comma-separated CIDRs |

- [ ] **Step 1: Write failing production/config/proxy tests**

Add table-driven cases that prove invalid numeric text is not silently replaced, production rejects every omitted governance env, a 31-byte/example IP hash secret fails, 10/11 are exact, all limits are positive, `TRUSTED_PROXY_CIDRS=none` is explicit, and invalid CIDR fails.

```go
func TestProductionRequiresExplicitUploadGovernance(t *testing.T) {
    cfg := securityTestConfig(t)
    cfg.AppEnv = "production"
    cfg.uploadGovernanceEnvExplicit = false
    if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "upload governance") {
        t.Fatalf("expected explicit governance error, got %v", err)
    }
}

func TestTrustedProxyConfigurationRejectsSpoofedForwardedFor(t *testing.T) {
    cfg := securityTestConfig(t)
    cfg.TrustedProxyCIDRs = nil
    srv, err := NewServer(cfg)
    if err != nil { t.Fatal(err) }
    srv.Router.GET("/test-client-ip", func(c *gin.Context) { c.String(200, c.ClientIP()) })
    req := httptest.NewRequest(http.MethodGet, "/test-client-ip", nil)
    req.RemoteAddr = "192.0.2.10:12345"
    req.Header.Set("X-Forwarded-For", "198.51.100.7")
    out := httptest.NewRecorder()
    srv.Router.ServeHTTP(out, req)
    if out.Body.String() != "192.0.2.10" { t.Fatalf("client IP = %q", out.Body.String()) }
}
```

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
cd backend
go test ./internal/app -run 'Test(ProductionRequiresExplicitUploadGovernance|ProductionRejectsInvalidUploadGovernance|TrustedProxyConfiguration)' -count=1 -v
```

Expected: FAIL because Config has no strict governance contract and Gin still uses its default proxy trust.

- [ ] **Step 3: Implement strict parsing and production validation**

Use checked multiplication to convert MiB values:

```go
func mibToBytes(value int64) (int64, error) {
    const mib = int64(1024 * 1024)
    if value <= 0 || value > math.MaxInt64/mib {
        return 0, fmt.Errorf("value must be a positive MiB quantity")
    }
    return value * mib, nil
}
```

`LoadConfig` must use `os.LookupEnv` and `strconv.ParseInt` for every governance number. If a provided value is invalid, retain a sanitized `loadErr`; do not default it. Parse `TRUSTED_PROXY_CIDRS=none` to an empty slice, otherwise split commas, trim, and validate each CIDR with `net.ParseCIDR`.

For non-production omitted values, use exactly 10, 11, 20, 5, 50, 2048, 20480, 300, 50, 600, and 1800 from the spec. Do not generate a fixed IP HMAC secret; development/test callers must supply one. Production requires every env to have been present and validates the IP HMAC secret at least 32 bytes and not in an unsafe-example set.

`NewServer` must not repair zero/negative governance fields after validation. Only `LoadConfig` supplies non-production defaults; direct Config callers must be explicit so tests and embedded callers cannot accidentally disable a limit.

- [ ] **Step 4: Apply trusted proxies before route registration**

Immediately after `gin.New()`:

```go
if err := r.SetTrustedProxies(cfg.TrustedProxyCIDRs); err != nil {
    return nil, fmt.Errorf("configure trusted proxies: %w", err)
}
```

Update `securityTestConfig`, `configureTestUploadGovernance`, and both MySQL Config builders with explicit synthetic test values. Use 10 MiB as the shared test default; tests that need small quotas override only the specific field. The shared package-`tests` helper has this exact shape:

```go
func configureTestUploadGovernance(cfg *app.Config) {
    cfg.FileUploadMaxBytes = 10 * 1024 * 1024
    cfg.FileUploadMultipartMaxBytes = 11 * 1024 * 1024
    cfg.FileUploadIPHashSecret = "test-only-upload-ip-hmac-secret-32-bytes"
    cfg.FileUploadAnonPresignPerHour = 20
    cfg.FileUploadAnonActiveFiles = 5
    cfg.FileUploadAnonActiveBytes = 50 * 1024 * 1024
    cfg.FileUploadMerchantQuotaBytes = 2 * 1024 * 1024 * 1024
    cfg.FileUploadGlobalQuotaBytes = 20 * 1024 * 1024 * 1024
    cfg.FileUploadCleanupInterval = 5 * time.Minute
    cfg.FileUploadCleanupBatchSize = 50
    cfg.FileUploadCleanupClaimTTL = 10 * time.Minute
    cfg.FileUploadCleanupGrace = 30 * time.Minute
    cfg.TrustedProxyCIDRs = nil
}
```

- [ ] **Step 5: Update repository-owned config examples**

Write every production key explicitly with approved numeric values and the intentionally rejected placeholder `FILE_UPLOAD_IP_HASH_SECRET=replace-with-a-strong-random-upload-ip-hmac-secret`. Use `TRUSTED_PROXY_CIDRS=none` in development/acceptance examples. Use the valid documentation-only address `TRUSTED_PROXY_CIDRS=192.0.2.10/32` in production examples with a comment requiring replacement by the exact deployed proxy CIDR. Never add a real secret.

- [ ] **Step 6: Run focused and package tests**

Run:

```bash
cd backend
gofmt -w internal/app/config.go internal/app/server.go internal/app/server_security_test.go \
  tests/integration_flow_test.go tests/file_schema_mysql_test.go
go test ./internal/app ./tests -run 'Test(Production|TrustedProxy|NewServerDoesNotSeed|FilePresign)' -count=1 -v
```

Expected: PASS. Search the examples to confirm `FILE_UPLOAD_MAX_MB=40` is gone and no secret value was introduced.

- [ ] **Step 7: Commit the configuration unit**

```bash
git add backend/internal/app/config.go backend/internal/app/server.go \
  backend/internal/app/server_security_test.go backend/tests/integration_flow_test.go \
  backend/tests/file_schema_mysql_test.go backend/configs/.env.example \
  backend/configs/.env.production.mysql.example backend/configs/.env.production.sqlite.example
git commit -m "fix(config): require upload governance limits"
```

### Task 3: Serialize quota reservations and hash anonymous sources

**Files:**
- Create: `backend/internal/app/upload_governance.go`
- Create: `backend/internal/app/upload_governance_test.go`
- Modify: `backend/internal/common/errors.go`

**Interfaces:**
- Produces `func (s *Server) anonymousSourceHash(rawIP string) (string, error)`.
- Produces `func (s *Server) withQuotaTransaction(fn func(*gorm.DB) error) error`; it uses `sql.LevelReadCommitted` for MySQL and the driver default for SQLite.
- Produces `func lockFileQuotaGuard(tx *gorm.DB) error` and `func (s *Server) reserveFileRecord(file *model.FileRecord, now time.Time) error`.
- Produces `common.CodeUploadQuotaExceeded = 10013`, `ErrUploadQuotaExceeded` as HTTP 409, and `ErrUploadTooLarge` as code `10008` / HTTP 413.
- `reserveFileRecord` creates the row in the same transaction as guard lock and checks global plus the actor-specific limits.

- [ ] **Step 1: Write failing HMAC and quota service tests**

Use a test server with limits `rate=2`, `activeFiles=2`, `activeBytes=10`, `merchantBytes=20`, and `globalBytes=30`.

```go
func governanceUnitServer(t *testing.T) *Server {
    t.Helper()
    cfg := securityTestConfig(t)
    cfg.FileUploadAnonPresignPerHour = 2
    cfg.FileUploadAnonActiveFiles = 2
    cfg.FileUploadAnonActiveBytes = 10
    cfg.FileUploadMerchantQuotaBytes = 20
    cfg.FileUploadGlobalQuotaBytes = 30
    srv, err := NewServer(cfg)
    if err != nil { t.Fatalf("NewServer: %v", err) }
    return srv
}

func TestAnonymousSourceHashCanonicalizesAndUsesHMAC(t *testing.T) {
    srv := governanceUnitServer(t)
    a, _ := srv.anonymousSourceHash("2001:db8::1")
    b, _ := srv.anonymousSourceHash("2001:0db8:0:0:0:0:0:1")
    if a != b || len(a) != 64 || a == common.SHA256("2001:db8::1") {
        t.Fatalf("unsafe/noncanonical hashes: %q %q", a, b)
    }
}

func TestReserveFileRecordRollsBackAtEachQuotaBoundary(t *testing.T) {
    // Fill rate, active count, active bytes, merchant, and global boundaries
    // in subtests. Assert the expected 10009 or 10013 and unchanged row count.
}
```

Add an overflow case with a current sum greater than `limit-newSize`, a missing guard case that returns an internal error, and a restart case that creates a second `Server` over the same SQLite database and still observes prior rows.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
cd backend
go test ./internal/app -run 'Test(AnonymousSourceHash|ReserveFileRecord|QuotaTransaction)' -count=1 -v
```

Expected: FAIL because the governance service and error contracts do not exist.

- [ ] **Step 3: Implement canonical HMAC**

```go
func (s *Server) anonymousSourceHash(rawIP string) (string, error) {
    parsed := net.ParseIP(strings.TrimSpace(rawIP))
    if parsed == nil { return "", common.ErrInternal }
    canonical := parsed.To16()
    if v4 := parsed.To4(); v4 != nil { canonical = v4 }
    mac := hmac.New(sha256.New, []byte(s.cfg.FileUploadIPHashSecret))
    _, _ = mac.Write(canonical)
    return hex.EncodeToString(mac.Sum(nil)), nil
}
```

Never accept an empty secret and never include `rawIP` in returned errors.

- [ ] **Step 4: Implement the guard and quota aggregates**

Inside `reserveFileRecord`:

```text
withQuotaTransaction
  -> SELECT id FROM file_quota_guards WHERE id=1 FOR UPDATE
  -> global SUM(size_bytes), reject if current > globalLimit-newSize
  -> PUBLIC: rolling COUNT, active COUNT, active SUM
  -> owner merchant: merchant SUM
  -> tx.Create(file)
```

Use `COALESCE(SUM(size_bytes),0)` into `int64`; reject any negative stored size as internal corruption. The rate query is `uploader_type='PUBLIC' AND source_ip_hash=? AND created_at > ?` and includes bound records. Active queries add `owner_merchant_id IS NULL AND cleanup_after IS NOT NULL`. Do not include expiry in the active predicate.

- [ ] **Step 5: Run focused tests and race-safe regressions**

Run:

```bash
cd backend
gofmt -w internal/common/errors.go internal/app/upload_governance.go internal/app/upload_governance_test.go
go test ./internal/app -run 'Test(AnonymousSourceHash|ReserveFileRecord|QuotaTransaction)' -count=1 -v
go test ./internal/app -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the quota service**

```bash
git add backend/internal/common/errors.go backend/internal/app/upload_governance.go \
  backend/internal/app/upload_governance_test.go
git commit -m "feat(files): serialize upload quota reservations"
```

### Task 4: Wire quotas into presign and anonymous merchant registration

**Files:**
- Modify: `backend/internal/app/file_handlers.go`
- Modify: `backend/internal/app/file_binding.go`
- Modify: `backend/internal/app/file_binding_test.go`
- Modify: `backend/internal/app/auth_handlers.go`
- Modify: `backend/tests/file_upload_test.go`
- Modify: `backend/tests/file_binding_helpers_test.go`
- Modify: `backend/tests/file_binding_security_test.go`

**Interfaces:**
- `handlePresign` sets `SourceIPHash` and `CleanupAfter` only for anonymous uploads, then calls `reserveFileRecord` instead of `DB.Create`.
- Replaces free function `claimPublicMerchantLicense` with method `func (s *Server) claimPublicMerchantLicense(tx *gorm.DB, fileID uint64, rawToken string, merchantID uint64, now time.Time) error`.
- Registration runs through `s.withQuotaTransaction`; `claimPublicMerchantLicense` assumes that transaction, locks guard once, checks merchant bytes, and requires `cleanup_claim_token IS NULL`.
- Authenticated merchant/admin rows keep all source/cleanup fields NULL.

- [ ] **Step 1: Write failing API and binding tests**

Add these named tests:

```text
TestAnonymousPresignPersistsHMACAndCleanupAfter
TestAnonymousPresignRatePersistsAcrossServerInstances
TestAnonymousPresignRejectsActiveFileAndByteQuotaWithoutCreatingRows
TestMerchantAndGlobalPresignQuotaRejectWithoutCreatingRows
TestAuthenticatedPresignDoesNotPersistAnonymousGovernanceFields
TestRegisterRejectsMerchantQuotaAndRollsBackAllRows
TestClaimPublicMerchantLicenseRejectsCleanupClaim
```

For the HMAC test set `RemoteAddr`, include a spoofed `X-Forwarded-For`, reload `FileRecord`, and assert the raw/spoofed IP appears nowhere in the row. Verify `CleanupAfter == CapabilityExpiresAt + CleanupGrace`.

For registration quota, set the test merchant quota below the candidate license size, call `/auth/register`, expect code `10013`, then assert merchant, account, audit, owner, capability, and claim fields are unchanged.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
cd backend
go test ./internal/app ./tests -run 'Test(AnonymousPresign|MerchantAndGlobalPresign|AuthenticatedPresign|RegisterRejectsMerchantQuota|ClaimPublicMerchantLicenseRejectsCleanupClaim)' -count=1 -v
```

Expected: FAIL because handlers still create records directly and registration does not lock/check quota.

- [ ] **Step 3: Reserve anonymous and authenticated presigns atomically**

Use one `now := time.Now()` and build the existing capability before reservation. For anonymous rows:

```go
sourceHash, err := s.anonymousSourceHash(c.ClientIP())
cleanupAfter := expiresAt.Add(s.cfg.FileUploadCleanupGrace)
file.SourceIPHash = &sourceHash
file.CleanupAfter = &cleanupAfter
```

For merchant/admin leave governance fields nil. Set `file.CreatedAt = now` so the rolling window and reservation share one timestamp. Replace `s.DB.Create(&file)` with `s.reserveFileRecord(&file, now)`. Keep capability response behavior exactly unchanged.

- [ ] **Step 4: Make registration quota-aware without weakening F-02**

Run the registration transaction through `s.withQuotaTransaction`. Lock the guard before merchant-sum/current-file checks. The conditional owner update must include:

```sql
cleanup_claim_token IS NULL
AND capability_expires_at > ?
AND owner_merchant_id IS NULL
```

The merchant sum plus candidate file size must not exceed `FileUploadMerchantQuotaBytes`. Preserve the exact one-row update requirement and clear only capability hash/expiry on success; retain `source_ip_hash` for the rolling-hour query.

- [ ] **Step 5: Update fixtures and run all file/binding tests**

Update helpers to use actual content lengths and governance test defaults. Run:

```bash
cd backend
gofmt -w internal/app/file_handlers.go internal/app/file_binding.go internal/app/file_binding_test.go \
  internal/app/auth_handlers.go tests/file_upload_test.go tests/file_binding_helpers_test.go \
  tests/file_binding_security_test.go
go test ./internal/app ./tests -run 'Test(AnonymousPresign|MerchantAndGlobalPresign|AuthenticatedPresign|Register|ClaimPublicMerchantLicense|FileUpload)' -count=1 -v
```

Expected: PASS, including existing capability replay/rollback tests.

- [ ] **Step 6: Commit the handler integration**

```bash
git add backend/internal/app/file_handlers.go backend/internal/app/file_binding.go \
  backend/internal/app/file_binding_test.go backend/internal/app/auth_handlers.go \
  backend/tests/file_upload_test.go backend/tests/file_binding_helpers_test.go \
  backend/tests/file_binding_security_test.go
git commit -m "fix(files): enforce upload quotas at presign"
```

### Task 5: Reject oversized multipart bodies before parsing

**Files:**
- Modify: `backend/internal/media/processor.go`
- Modify: `backend/internal/media/processor_test.go`
- Modify: `backend/internal/app/file_handlers.go`
- Modify: `backend/tests/file_upload_test.go`
- Modify: `backend/tests/file_binding_helpers_test.go`
- Modify: `backend/tests/license_file_privacy_test.go`

**Interfaces:**
- `media.DefaultMaxOriginalBytes` becomes exactly 10 MiB.
- Produces `func parseCappedMultipart(c *gin.Context, requestLimit int64) error` in `file_handlers.go`; it is called before any `PostForm`/`FormFile` access.
- Upload returns `ErrUploadTooLarge` for request/body/file size excess and `ErrInvalidUpload` for empty, corrupt, MIME-invalid, or declared/actual mismatch.
- The processor is never called and no temp/final file is written on any early rejection.

- [ ] **Step 1: Replace 40 MiB executable tests and add parser RED cases**

Rename existing tests to exact 10 MiB names and add:

```text
TestFilePresignAllowsImageAt10MiB
TestFilePresignRejectsImageOver10MiBWithoutRow
TestFileUploadRejectsContentLengthOver11MiBBeforeMultipartParsing
TestFileUploadRejectsChunkedBodyOver11MiBWithJSON413
TestFileUploadRejectsFileOver10MiBWithJSON413
TestFileUploadRejectsDeclaredActualSizeMismatchBeforeProcessor
TestFileUploadRejectsProcessorExpansionWithoutWrite
TestFileUploadAcceptsExact10MiB
```

Extend the multipart helper to return HTTP status as well as decoded API response. Use a counting fake processor and assert calls remain zero for every rejected request.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
cd backend
go test ./internal/media ./tests -run 'Test(Policy.*10MiB|FilePresign.*10MiB|FileUploadRejects|FileUploadAcceptsExact10MiB)' -count=1 -v
```

Expected: FAIL because the default is 40 MiB, form access triggers parsing before a request cap, and declared size is not checked against actual bytes.

- [ ] **Step 3: Install and classify the body cap before form access**

At the first line of upload handling:

```go
c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.cfg.FileUploadMultipartMaxBytes)
if c.Request.ContentLength > s.cfg.FileUploadMultipartMaxBytes {
    common.Fail(c, common.ErrUploadTooLarge)
    return
}
if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
    var maxErr *http.MaxBytesError
    if errors.As(err, &maxErr) { common.Fail(c, common.ErrUploadTooLarge) } else { common.Fail(c, common.ErrInvalidUpload) }
    return
}
defer c.Request.MultipartForm.RemoveAll()
```

Only after this block read fields. Map `formFile.Size > FileUploadMaxBytes` and actual bytes `> FileUploadMaxBytes` to 413/`10008`; map actual length not equal to reserved `file.SizeBytes` to 400/`10008`. After processing, reject an empty result, a result over 10 MiB, or a result larger than the validated original before creating/renaming any destination file. This preserves the invariant that updating `size_bytes` can only release quota.

- [ ] **Step 4: Update every existing upload fixture to declare exact bytes**

Replace hard-coded values such as `32`, `2048`, and `40*1024*1024` whenever an actual upload follows with `len(content)`. Preserve presign-only quota fixtures that intentionally reserve a synthetic size. Ensure F-02 and F-04/F-13 tests still exercise real capabilities and private URLs.

- [ ] **Step 5: Run focused and full backend tests**

Run:

```bash
cd backend
gofmt -w internal/media/processor.go internal/media/processor_test.go \
  internal/app/file_handlers.go tests/file_upload_test.go tests/file_binding_helpers_test.go \
  tests/license_file_privacy_test.go
go test ./internal/media ./tests -run 'Test(Policy|FilePresign|FileUpload|PublicUpload|AdminLicense)' -count=1 -v
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the request-size boundary**

```bash
git add backend/internal/media/processor.go backend/internal/media/processor_test.go \
  backend/internal/app/file_handlers.go backend/tests/file_upload_test.go \
  backend/tests/file_binding_helpers_test.go backend/tests/license_file_privacy_test.go
git commit -m "fix(files): cap multipart uploads before parsing"
```

### Task 6: Clean only claimed post-migration anonymous orphans

**Files:**
- Create: `backend/internal/app/upload_cleanup.go`
- Create: `backend/internal/app/upload_cleanup_test.go`
- Modify: `backend/internal/app/server.go`
- Modify: `backend/internal/app/file_binding.go`

**Interfaces:**
- Produces `type uploadCleanupSummary struct { Claimed int; Deleted int; Failed int; FailureCategories map[string]int }`.
- Produces `func (s *Server) runUploadCleanupBatch(ctx context.Context, now time.Time) (uploadCleanupSummary, error)`.
- Produces `func (s *Server) runUploadCleanupLoop(ctx context.Context)` and `func (s *Server) removeManagedLocalFile(objectKey string) error`.
- Uses one random lowercase 64-character batch claim token; every delete/update predicate includes ID, token, `PUBLIC`, and NULL owner.
- `Server.Run` starts one cancellable loop and cancels it when the HTTP runner returns. `NewServer` alone starts no goroutine.

- [ ] **Step 1: Write failing candidate, filesystem, retry, and race tests**

Add these named tests:

```text
TestUploadCleanupNeverClaimsHistoricalAuthenticatedOrBoundFiles
TestUploadCleanupClaimsOnlyExpiredGraceInBoundedOrder
TestUploadCleanupDeletesManagedFileAndRow
TestUploadCleanupTreatsMissingPhysicalFileAsSuccess
TestUploadCleanupReleasesClaimAndRetriesAfterDeleteFailure
TestUploadCleanupReclaimsOnlyStaleClaims
TestUploadCleanupRejectsParentSymlinkEscape
TestUploadCleanupFailsClosedForUnsupportedProvider
TestUploadCleanupCannotDeleteFileClaimedByRegistration
TestUploadCleanupLogsOnlySanitizedSummary
TestUploadCleanupLoopStopsWhenContextCancelled
```

For history protection, create records with `CleanupAfter=nil`, including a row shaped like each historical uploader/scan state, run multiple batches, and compare complete rows and fixture bytes. For symlink escape, place a symlinked parent below upload root pointing to a second temp directory; assert the external file survives.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
cd backend
go test ./internal/app -run 'TestUploadCleanup' -count=1 -v
```

Expected: FAIL because no cleanup claim/worker exists.

- [ ] **Step 3: Implement bounded claims**

Within a short transaction, select at most `FileUploadCleanupBatchSize`, ordered by `cleanup_after,id`, with the exact candidate predicate from the spec. On MySQL add `FOR UPDATE SKIP LOCKED`; SQLite uses its serialized test path. Set claim token/time and increment attempts before committing.

Every later lookup and delete must include:

```sql
id = ? AND cleanup_claim_token = ?
AND uploader_type = 'PUBLIC' AND owner_merchant_id IS NULL
```

On processing failure, clear only `cleanup_claim_token` and `cleanup_claimed_at` under the same predicate. A stale claim is eligible only at `claimed_at <= now - claimTTL`.

- [ ] **Step 4: Implement symlink-safe local deletion and fail-closed providers**

Resolve the real upload root, then walk every normalized parent segment from that root with `os.Lstat`. Reject any symlink or non-directory parent. If a parent segment does not exist, treat the never-created target as idempotently absent; otherwise `Lstat` the target, reject a symlink/directory/non-regular file, and treat target `os.IsNotExist` as success. For providers other than `local`, return a sentinel categorized only as `unsupported_provider`; do not delete the DB row.

- [ ] **Step 5: Add scheduler and sanitized summary logging**

Use an immediate bounded batch followed by `time.NewTicker(FileUploadCleanupInterval)`. Log only batch counts and normalized category counts. Add an unexported injectable `cleanupLogf func(string, ...interface{})` defaulting to `log.Printf`; tests capture it and search for raw IP, HMAC, token, object key, URL, and absolute path fixtures.

- [ ] **Step 6: Run cleanup and F-02 regression tests**

Run:

```bash
cd backend
gofmt -w internal/app/upload_cleanup.go internal/app/upload_cleanup_test.go \
  internal/app/server.go internal/app/file_binding.go
go test ./internal/app -run 'Test(UploadCleanup|ClaimPublicMerchantLicense)' -count=1 -v
go test ./internal/app -count=1
```

Expected: PASS with no real sleeps and no goroutine leaks.

- [ ] **Step 7: Commit cleanup**

```bash
git add backend/internal/app/upload_cleanup.go backend/internal/app/upload_cleanup_test.go \
  backend/internal/app/server.go backend/internal/app/file_binding.go
git commit -m "feat(files): clean expired anonymous uploads"
```

### Task 7: Reject oversize files consistently in all frontend upload entry points

**Files:**
- Create: `frontend/src/utils/upload.ts`
- Create: `frontend/src/utils/upload.test.ts`
- Create: `frontend/src/pages/merchant/products/CreatePage.test.tsx`
- Create: `frontend/src/pages/merchant/products/EditPage.test.tsx`
- Modify: `frontend/src/pages/auth/RegisterPage.tsx`
- Modify: `frontend/src/pages/auth/RegisterPage.test.tsx`
- Modify: `frontend/src/pages/merchant/products/CreatePage.tsx`
- Modify: `frontend/src/pages/merchant/products/EditPage.tsx`

**Interfaces:**
- Produces `export const MAX_UPLOAD_FILE_BYTES = 10 * 1024 * 1024`.
- Produces `export function validateUploadFile(file: File): string | null`, returning `图片文件不能为空` for zero bytes, `图片不能超过 10 MiB` for excess, and `null` at valid sizes.
- Every page calls the validator after clearing the input value and before `uploadMutation.mutate(file)`.

- [ ] **Step 1: Write failing shared-boundary tests**

```ts
it('accepts exactly 10 MiB and rejects one byte more', () => {
  const exact = new File([new Uint8Array(MAX_UPLOAD_FILE_BYTES)], 'exact.jpg', { type: 'image/jpeg' })
  const over = new File([new Uint8Array(MAX_UPLOAD_FILE_BYTES + 1)], 'over.jpg', { type: 'image/jpeg' })
  expect(validateUploadFile(exact)).toBeNull()
  expect(validateUploadFile(over)).toBe('图片不能超过 10 MiB')
})
```

In each page test, select a 10 MiB + 1 file, assert the stable message, and assert `api.presign`/`api.uploadFile` were never called. Preserve the RegisterPage test that proves `file_token` remains memory-only.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
cd frontend
npm test -- src/utils/upload.test.ts src/pages/auth/RegisterPage.test.tsx \
  src/pages/merchant/products/CreatePage.test.tsx src/pages/merchant/products/EditPage.test.tsx
```

Expected: FAIL because the shared validator and product page tests do not exist and current pages call presign for oversize files.

- [ ] **Step 3: Implement the shared validator and wire all pages**

```ts
export const MAX_UPLOAD_FILE_BYTES = 10 * 1024 * 1024

export function validateUploadFile(file: File) {
  if (file.size <= 0) return '图片文件不能为空'
  if (file.size > MAX_UPLOAD_FILE_BYTES) return '图片不能超过 10 MiB'
  return null
}
```

Each selection handler uses `message.error(validationError)` or the existing registration error state and returns before mutation. Replace every visible `40MB` upload statement with `原图最大 10 MiB，服务端自动压缩。` Do not add storage, navigation, or new UI panels.

- [ ] **Step 4: Run focused tests, full frontend tests, and build**

Run:

```bash
cd frontend
npm test -- src/utils/upload.test.ts src/pages/auth/RegisterPage.test.tsx \
  src/pages/merchant/products/CreatePage.test.tsx src/pages/merchant/products/EditPage.test.tsx
npm test
npm run build
```

Expected: all tests and TypeScript/Vite build PASS.

- [ ] **Step 5: Commit frontend enforcement**

```bash
git add frontend/src/utils/upload.ts frontend/src/utils/upload.test.ts \
  frontend/src/pages/auth/RegisterPage.tsx frontend/src/pages/auth/RegisterPage.test.tsx \
  frontend/src/pages/merchant/products/CreatePage.tsx \
  frontend/src/pages/merchant/products/CreatePage.test.tsx \
  frontend/src/pages/merchant/products/EditPage.tsx \
  frontend/src/pages/merchant/products/EditPage.test.tsx
git commit -m "fix(frontend): enforce 10 MiB upload limit"
```

### Task 8: Build the isolated MySQL 8.4 and proxy acceptance harness

**Files:**
- Create: `backend/internal/app/upload_governance_mysql_test.go`
- Create: `deploy/acceptance/anonymous-upload-governance-smoke.sh`
- Modify: `backend/migrations/anonymous_upload_governance_migration_test.go`
- Modify: `deploy/acceptance/prepare.sh`
- Modify: `deploy/acceptance/.env.example`
- Modify: `deploy/acceptance/docker-compose.yml`
- Modify: `deploy/acceptance/nginx.conf`
- Modify: `deploy/acceptance/README.md`
- Modify: `Makefile`

**Interfaces:**
- Produces guarded make target `acceptance-anonymous-upload-governance-smoke` requiring `ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA` and `ACCEPTANCE_DB_ENGINE=mysql8.4`.
- Uses fixed Compose project `secondhand-upload-governance-acceptance` and evidence directory `deploy/acceptance/evidence/anonymous-upload-governance/`.
- Go test runs only when `UPLOAD_GOVERNANCE_MYSQL_TEST=1`; it accepts only `DB_DSN` supplied inside the isolated Compose network.
- Nginx uses `client_max_body_size 11m`, `application/json`, and code `10008`/request ID for proxy-generated 413.

- [ ] **Step 1: Extend migration artifact tests with acceptance guards and verify RED**

Require the exact confirmation, project, MySQL 8.4 check, full `0001..0008` chain, historical fingerprint, concurrency test name, both AutoMigrate modes, production before/after snapshots, source SHA-256, success marker, and retention message.

Run:

```bash
cd backend
go test ./migrations -run TestAnonymousUploadGovernanceAcceptanceScriptContracts -count=1 -v
```

Expected: FAIL because the script and Make target do not exist.

- [ ] **Step 2: Write the MySQL multi-connection acceptance test**

`TestUploadGovernanceMySQLConcurrencyAndCleanup` in package `app` must:

1. start two `Server` values over independent GORM pools and the same migrated MySQL schema;
2. set deliberately small synthetic limits and independent test HMAC secret;
3. synchronize two READ COMMITTED presigns at one remaining global/merchant/anonymous slot and prove exactly one succeeds;
4. prove the waiter sees the committed reservation and returns `10009` or `10013`, with no extra row;
5. race expired cleanup claim against registration and prove no bound file is deleted;
6. run two cleanup claimers concurrently and prove each candidate is processed at most once;
7. prove stale claim retry, missing physical file idempotency, unsupported provider fail-closed, and historical NULL row preservation;
8. rerun once with `AUTO_MIGRATE=false` and once after an `AUTO_MIGRATE=true` restart.

Do not print DSN, IP hash, tokens, object keys, local paths, or row payloads.

- [ ] **Step 3: Write the fixed-project smoke script**

The script must refuse any pre-existing project-labeled container/volume/network, verify MySQL `8.4.*`, snapshot only production container ID/state/restart count, and create synthetic fixtures only. For each dirty schema case, restore `0001..0007`, induce one partial/wrong `0008` shape, assert SQLSTATE `45000`, and prove row fingerprints unchanged.

The clean path must:

```text
apply 0001..0007
insert synthetic historical NULL-governance rows and fixture files
record sanitized ID/count/non-governance fingerprint
run 0008 preflight/up/postflight
run Go MySQL concurrency/cleanup test with AUTO_MIGRATE=false
restart test with AUTO_MIGRATE=true and rerun
exercise API and Nginx 10/11 MiB boundaries
compare historical fingerprint/files and production snapshot
write SHA-256 manifest and retain resources
```

Use `trap` only for diagnostics/stopping test services; never `down --volumes` automatically.

- [ ] **Step 4: Configure acceptance-only secrets and limits**

`prepare.sh` generates a new independent 32-byte-or-longer `FILE_UPLOAD_IP_HASH_SECRET` into ignored `.env` without printing it. Compose passes all explicit production numeric values, uses `TRUSTED_PROXY_CIDRS=none`, and keeps MySQL internal and API/Web ports loopback-only.

Configure Nginx:

```nginx
client_max_body_size 11m;
error_page 413 = @upload_too_large;
location @upload_too_large {
    internal;
    default_type application/json;
    return 413 '{"code":10008,"message":"upload file too large","request_id":"$request_id"}';
}
```

- [ ] **Step 5: Add Make/README contracts and run local static validation**

Run:

```bash
bash -n deploy/acceptance/anonymous-upload-governance-smoke.sh
docker compose --project-name secondhand-upload-governance-acceptance \
  --env-file deploy/acceptance/.env.example \
  --file deploy/acceptance/docker-compose.yml config --quiet
cd backend && go test ./migrations -run 'TestAnonymousUploadGovernance' -count=1 -v
```

The compose check may use temporary synthetic non-secret env values if blank required values make `.env.example` intentionally invalid; do not write a real `.env` or print resolved secrets.

- [ ] **Step 6: Commit the acceptance harness**

```bash
git add backend/internal/app/upload_governance_mysql_test.go \
  backend/migrations/anonymous_upload_governance_migration_test.go \
  deploy/acceptance/anonymous-upload-governance-smoke.sh deploy/acceptance/prepare.sh \
  deploy/acceptance/.env.example deploy/acceptance/docker-compose.yml \
  deploy/acceptance/nginx.conf deploy/acceptance/README.md Makefile
git commit -m "test(acceptance): verify upload governance"
```

### Task 9: Complete local verification, review, and code-side status documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/backend-api-checklist.md`
- Modify: `docs/data-model.md`
- Modify: `docs/release-readiness.md`
- Modify: `docs/full-project-code-review-2026-07-24.md`
- Modify: `docs/production-hardening-repair-plan-2026-07-24.md`
- Modify: `docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md`

**Interfaces:**
- Current docs state `代码侧状态：已修复`, `测试服务器状态：未审核`, and `生产状态：未迁移未部署` only after all local gates and review pass.
- The original F-06/D-03 finding text remains intact; append a dated follow-up.
- No protected untracked review document is modified or staged.

- [ ] **Step 1: Run formatting and stale-contract scans**

Run:

```bash
git diff --name-only --diff-filter=ACM 39aed02..HEAD -- 'backend/**/*.go' | \
  while IFS= read -r file; do gofmt -w "$file"; done
rg -n '40MB|40 MB|40 MiB|FILE_UPLOAD_MAX_MB=40|client_max_body_size 45m' \
  README.md backend frontend/src deploy/acceptance docs/backend-api-checklist.md docs/data-model.md
git diff --check
```

Expected: no current executable/config/UI 40 MiB contract remains. Historical `2026-04-02` design/plan references may remain and must not be rewritten.

- [ ] **Step 2: Run all local verification gates from clean dependency state**

Run:

```bash
make test
cd frontend && npm test && npm run build
cd ..
bash -n deploy/acceptance/anonymous-upload-governance-smoke.sh
```

Expected: backend full suite, frontend full suite/build, and shell syntax all PASS. Record exact test counts, tool versions, and any pre-existing warning; do not claim MySQL concurrency from SQLite results.

- [ ] **Step 3: Perform a code review before status closure**

Invoke `superpowers:requesting-code-review` against the complete F-06 commit range. Review specifically for:

```text
quota increase path missing guard
REPEATABLE READ stale snapshot
integer overflow or negative sum
trusted-proxy spoofing
raw IP/hash/token/path logging
multipart parsing before MaxBytesReader
declared/actual mismatch
historical cleanup enrollment
symlink parent escape
cleanup/binding race
unsupported provider row-only deletion
goroutine/ticker leak
production config defaulting
```

Resolve every High/Medium finding with a new RED/GREEN commit, rerun focused and full gates, and preserve reviewer evidence under tracked `docs/superpowers/reviews/` only if it is sanitized.

- [ ] **Step 4: Update current-state documentation**

Document exact byte units, code `10008`/413 vs `10013`/409, schema fields/indexes/guard, cleanup boundary, config contract, and isolated acceptance command. Append a dated F-06 follow-up to the tracked full-project review; do not rewrite the original finding.

Set status only to:

```text
代码侧状态：已修复
测试服务器状态：未审核
生产状态：未执行 0008、未部署、未修改生产数据或文件
```

- [ ] **Step 5: Commit local documentation/status**

```bash
git add README.md docs/backend-api-checklist.md docs/data-model.md \
  docs/release-readiness.md docs/full-project-code-review-2026-07-24.md \
  docs/production-hardening-repair-plan-2026-07-24.md \
  docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md
git commit -m "docs(files): record F-06 code closure"
```

- [ ] **Step 6: Verify commit scope and protected files**

Run:

```bash
git status --short --branch
git diff --check HEAD~1 HEAD
git show --stat --oneline HEAD
git ls-files --error-unmatch backend/app.db >/dev/null 2>&1 && echo 'backend/app.db remains tracked for separate F-10 work'
```

Expected: only known untracked `.tmp/` and the three protected documents remain; F-10 is still open and no production state is marked complete.

### Task 10: Obtain path-specific authorization and run isolated test-server acceptance

**Files:**
- Create after successful run: `docs/superpowers/reviews/2026-07-26-anonymous-upload-governance-isolated-acceptance.md`
- Modify after successful run: `docs/release-readiness.md`
- Modify after successful run: `docs/full-project-code-review-2026-07-24.md`
- Modify after successful run: `docs/production-hardening-repair-plan-2026-07-24.md`
- Modify after successful run: `docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md`

**Interfaces:**
- Remote path: `/home/yu/services/secondhand-upload-governance-acceptance-20260726`.
- Compose project: `secondhand-upload-governance-acceptance`.
- Required command: `ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA ACCEPTANCE_DB_ENGINE=mysql8.4 make acceptance-anonymous-upload-governance-smoke`.
- Produces final F-06 state `代码侧已修复 / 隔离 MySQL 8.4 测试服务器审核通过 / 生产未迁移未部署`.

- [ ] **Step 1: Stop and obtain exact transfer authorization**

Request authorization for only the exact remote path/project above and this whitelist:

```text
backend/ source, migrations, tests, Dockerfile
frontend/ source and package manifests required for test/build
deploy/acceptance harness files excluding .env, secrets, evidence
Makefile, go.mod, go.sum
```

Explicitly exclude `.env`, credentials, databases, upload files, evidence, `.git`, caches, `node_modules`, frontend build output, `backend/app.db`, miniapp private config, and all three protected review documents. Earlier F-02/F-04/F-05 authorizations do not apply. Do not transfer until the user grants this exact authorization.

- [ ] **Step 2: Build a source manifest and transfer only the whitelist**

Record the exact reviewed commit and local SHA-256 manifest outside tracked source. Transfer using explicit include paths and exclusions; never use an unrestricted repository copy. On the server, recompute the same manifest and require byte-for-byte equality before running anything.

- [ ] **Step 3: Verify isolation before writes**

Read-only checks must prove no container, volume, or network carries the fixed project label and loopback ports are free. Snapshot only ID/state/restart count for `secondhand-market-api`, `secondhand-market-web`, and `secondhand-market-mysql`. Abort rather than delete/reuse any collision.

- [ ] **Step 4: Run the acceptance command and retain resources**

Run the exact command above from the authorized directory. Do not pass an external DSN, mount a production volume/path, or execute any command from a production checkout. On success stop services to release memory but retain project resources, isolated volumes, source, and sanitized evidence for review.

- [ ] **Step 5: Audit evidence before making a success claim**

Require MySQL 8.4.x, migration dirty/clean matrix, exact source SHA, concurrent one-winner results, historical fingerprints/files unchanged, cleanup race/retry results, proxy 413 JSON, full backend/frontend gates, AutoMigrate false/true, matching production snapshots, and a SHA-256 evidence manifest. Search evidence for DSN/password/JWT/raw IP/IP hash/token/object key/absolute upload path and redact by rerunning the harness, never by silently editing evidence.

- [ ] **Step 6: Write the tracked acceptance report and update status**

The report records commit SHA, server/tool versions, command/test counts, evidence filenames/hashes, retained resource names, and explicit statements:

```text
no production SQL executed
no production application/proxy deployed
no production database or upload file read/modified
production container identity/state/restart count unchanged
production 0008 and proxy maintenance remain open
```

Only now set `测试服务器状态：审核通过`. Keep production open.

- [ ] **Step 7: Commit server-reviewed evidence**

```bash
git add docs/superpowers/reviews/2026-07-26-anonymous-upload-governance-isolated-acceptance.md \
  docs/release-readiness.md docs/full-project-code-review-2026-07-24.md \
  docs/production-hardening-repair-plan-2026-07-24.md \
  docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md
git commit -m "docs(acceptance): record F-06 server approval"
```

- [ ] **Step 8: Re-run final evidence and branch audit**

Run:

```bash
make test
cd frontend && npm test && npm run build
cd ..
git diff --check
git status --short --branch
git log --oneline --decorate -15
```

Report F-06 commit hashes, local and remote test counts, isolated evidence paths/hashes, retained Compose resources, and remaining production gate. Then continue with the next open finding; do not represent F-01..F-15 as complete while F-10/F-11/F-12/F-14/F-15 or remote approvals remain open.
