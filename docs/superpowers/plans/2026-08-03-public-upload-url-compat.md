# Public Upload URL Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a response-layer absolute upload URL compatibility switch so old miniapp clients can display product images without a miniapp release.

**Architecture:** Keep database and file storage canonical as relative `/uploads/...`. Add `PUBLIC_UPLOAD_BASE_URL` to `app.Config`, validate it at startup, and have the existing `Server.publicFileURL` generate absolute URLs only for API responses when configured.

**Tech Stack:** Go 1.22, Gin, GORM, existing backend test helpers.

## Global Constraints

- Do not restore or weaken `FILE_PUBLIC_BASE_URL`; local storage must still reject it when non-empty.
- `PUBLIC_UPLOAD_BASE_URL` only changes response formatting, not object keys, storage paths, DB migrations, or `/uploads` route handling.
- First production rollout must keep `REQUIRE_DETAIL_V1_PRODUCT_IMAGES=false`.
- Use TDD: write failing tests before production code.

---

### Task 1: Config and URL formatter

**Files:**
- Modify: `backend/internal/app/config.go`
- Modify: `backend/internal/app/file_handlers.go`
- Test: `backend/tests/file_upload_test.go`

**Interfaces:**
- Produces: `Config.PublicUploadBaseURL string`
- Produces: `(*Server).publicFileURL(objectKey string) string` returning either `/uploads/<key>` or `<PUBLIC_UPLOAD_BASE_URL>/<key>`

- [ ] **Step 1: Write failing tests**

Add tests that configure `cfg.PublicUploadBaseURL = "https://market.meaningful.ink/uploads"` and assert:

```go
url := str(upload.Data["url"])
if !strings.HasPrefix(url, "https://market.meaningful.ink/uploads/") {
    t.Fatalf("upload response should expose absolute compatible URL: %s", url)
}
if record.URL != "/uploads/"+objectKey {
    t.Fatalf("database URL must remain relative: %q", record.URL)
}
```

Add a startup guard test:

```go
cfg.PublicUploadBaseURL = "https://market.meaningful.ink/static"
if _, err := app.NewServer(cfg); err == nil {
    t.Fatal("PUBLIC_UPLOAD_BASE_URL outside /uploads must fail closed")
}
```

- [ ] **Step 2: Run tests and verify red**

Run:

```powershell
$env:GOCACHE='E:\allsite\second-hand-market-backend\runtime\go-build-cache'
$env:GOMODCACHE='E:\allsite\second-hand-market-backend\runtime\go-mod-cache'
go test ./tests -run 'TestPublicUploadBaseURL|TestServerRejectsInvalidPublicUploadBaseURL' -v
```

Expected: fail to compile because `Config.PublicUploadBaseURL` does not exist.

- [ ] **Step 3: Implement minimal config and formatter**

Add `PublicUploadBaseURL string` to `Config`, load `PUBLIC_UPLOAD_BASE_URL`, validate non-empty values as HTTP(S) URL with normalized path `/uploads`, no query, no fragment, and use it in `publicFileURL`.

- [ ] **Step 4: Verify green**

Run the same targeted test command. Expected: PASS.

### Task 2: API response coverage

**Files:**
- Modify: `backend/tests/file_upload_test.go`
- Existing code paths use `publicFileRecordURL`, so no extra production file should be needed unless coverage reveals a direct persisted URL response.

**Interfaces:**
- Consumes: `(*Server).publicFileRecordURL(file model.FileRecord) string`

- [ ] **Step 1: Write failing coverage test**

Extend or add a test that creates a product with `cfg.PublicUploadBaseURL = "https://market.meaningful.ink/uploads"` and asserts all of these responses contain the absolute URL and do not contain a persisted legacy external URL:

```go
GET /api/v1/merchant/products/:id
GET /api/v1/buyer/products
GET /api/v1/buyer/products/:id
```

- [ ] **Step 2: Run test and verify red/green status**

Run:

```powershell
$env:GOCACHE='E:\allsite\second-hand-market-backend\runtime\go-build-cache'
$env:GOMODCACHE='E:\allsite\second-hand-market-backend\runtime\go-mod-cache'
go test ./tests -run 'TestLocalFileResponsesIgnorePersistedExternalURL|TestPublicUploadBaseURL' -v
```

Expected after Task 1 implementation: PASS because all response paths already go through `publicFileRecordURL`.

- [ ] **Step 3: Fix any uncovered direct URL usage**

If the test finds a response still exposing persisted `files.url`, replace that response path with `publicFileRecordURL`.

### Task 3: Runtime examples and release docs

**Files:**
- Modify: `backend/configs/.env.production.mysql.example`
- Modify: `backend/configs/.env.production.sqlite.example`
- Modify: `docs/release-readiness.md`
- Modify: `docs/superpowers/specs/2026-07-30-image-delivery-optimization-design.md`

**Interfaces:**
- Consumes: `PUBLIC_UPLOAD_BASE_URL`

- [ ] **Step 1: Write failing docs/config test**

Update `TestProductionEnvExamplesEnableRuntimeGuardrails` to require:

```go
if values["PUBLIC_UPLOAD_BASE_URL"] != "https://market.meaningful.ink/uploads" {
    t.Fatalf("PUBLIC_UPLOAD_BASE_URL = %q", values["PUBLIC_UPLOAD_BASE_URL"])
}
```

- [ ] **Step 2: Run test and verify red**

Run:

```powershell
$env:GOCACHE='E:\allsite\second-hand-market-backend\runtime\go-build-cache'
$env:GOMODCACHE='E:\allsite\second-hand-market-backend\runtime\go-mod-cache'
go test ./internal/app -run TestProductionEnvExamplesEnableRuntimeGuardrails -v
```

Expected: FAIL because examples do not yet include the new variable.

- [ ] **Step 3: Update examples and release docs**

Add `PUBLIC_UPLOAD_BASE_URL=https://market.meaningful.ink/uploads` to production examples and update release docs to state that `FILE_PUBLIC_BASE_URL` remains empty while `PUBLIC_UPLOAD_BASE_URL` is used only for old-client response compatibility.

- [ ] **Step 4: Verify green**

Run the same `go test ./internal/app ...` command. Expected: PASS.

### Task 4: Full verification and deployment continuation

**Files:**
- No additional code files expected.

- [ ] **Step 1: Run focused backend verification**

Run:

```powershell
$env:GOCACHE='E:\allsite\second-hand-market-backend\runtime\go-build-cache'
$env:GOMODCACHE='E:\allsite\second-hand-market-backend\runtime\go-mod-cache'
go test ./internal/app ./tests ./scripts/backfill_product_images
```

On Windows, Linux-only `miniapp_auth_refresh_acceptance_contract_test.go` may fail if included by `./tests`; if so, record it and verify the same package on the Linux test server before production deployment.

- [ ] **Step 2: Deploy to acceptance**

Sync code to `/home/yu/services/secondhand-market-acceptance`, rebuild acceptance API/Web as needed, set `PUBLIC_UPLOAD_BASE_URL` to the acceptance URL or production-compatible test URL, and run smoke checks against `127.0.0.1:18082`.

- [ ] **Step 3: Resume production checklist**

After acceptance passes, continue the production first-stage checklist: backup, config, image build with `/srv/migrate` and `/srv/backfill-product-images`, dry-run, write freeze, legacy `file_records` to `files` migration when required, ledger migration, deploy, canary, batch backfill, smoke.
