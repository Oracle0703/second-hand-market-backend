# License File Privacy and Admin Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close F-04 and F-13 by keeping product images public, making merchant licenses private, and allowing only active administrators to preview bound, scanned licenses through a fail-closed audited endpoint.

**Architecture:** Replace the local upload directory static mount with an exact `product_image/` allowlist handler. Store merchant licenses with an object key but no public URL, resolve all local reads inside the canonical upload root, and proxy validated license bytes through an authenticated admin route only after its operation log is durably written. The frontend requests the license as an authenticated Blob and holds only a revocable in-memory object URL.

**Tech Stack:** Go 1.22, Gin, GORM, MySQL 8.4 SQL migrations, SQLite test fixtures, React 18, TypeScript 5.7, Axios, TanStack Query, Ant Design, Vitest, Testing Library, Docker Compose, Bash.

## Global Constraints

- Implement only F-04 and F-13 plus the minimum F-02 compatibility adjustment required by private license URLs; do not claim F-06 or any other open finding is closed.
- Do not execute SQL against production, deploy backend/frontend, modify production files, or mutate production data.
- Never modify, stage, or commit `.tmp/`, `docs/architecture-evolution-plan-2026-07-24.md`, `docs/first-round-fix-review-2026-07-24.md`, or `docs/second-round-fix-review-2026-07-24.md`.
- Keep `PRODUCT_IMAGE` public URL behavior unchanged; only exact lowercase `product_image/` paths are anonymously readable.
- Return HTTP 404 / code `10004` for invalid or undisclosable file states so callers cannot enumerate private files.
- Require a valid, active admin session for `GET /api/v1/admin/files/:id/content`; merchant, buyer, missing, expired, and revoked credentials must not read bytes.
- Write `admin_file_read` before response headers or bytes; an audit insert failure returns HTTP 500 / code `20001` and zero file bytes.
- Never log or persist access tokens, capability tokens, object keys, local paths, file content, public license URLs, or merchant sensitive data in `admin_file_read`.
- New `MERCHANT_LICENSE` uploads and confirms persist `url=''` and omit `url` from success data; `PRODUCT_IMAGE` continues to require, persist, and return a nonempty URL.
- Add irreversible `0007` preflight/up/postflight artifacts; no down migration is permitted.
- Run every behavior change RED -> GREEN, use `gofmt`, preserve TypeScript strict mode, and make focused Conventional Commits.
- Isolated server acceptance must use MySQL 8.4.x and a distinct Compose project, retain sanitized evidence, and prove production container identity/state/restart counts are unchanged.

---

## File Map

- `backend/migrations/0007_license_file_privacy.preflight.sql`: fail closed before any URL update when canonical schema, F-02 ownership structure, or license invariants are invalid.
- `backend/migrations/0007_license_file_privacy.up.sql`: clear only nonempty `MERCHANT_LICENSE.url` values.
- `backend/migrations/0007_license_file_privacy.postflight.sql`: prove private license URLs, intact product URLs, unchanged row counts, and exact F-02 indexes.
- `backend/migrations/license_file_privacy_migration_test.go`: contract-test the three SQL artifacts, irreversible policy, and acceptance harness markers.
- `backend/internal/app/file_binding.go`: apply type-aware completion rules to merchant binding and public license claim.
- `backend/internal/app/file_binding_test.go`: unit regression for product URL and license object-key readiness.
- `backend/internal/app/file_handlers.go`: persist and return public URLs only for product images; provide strict local object-key resolution helpers.
- `backend/internal/app/public_file_handlers.go`: expose only exact product-image object keys from local storage.
- `backend/internal/app/private_file_handlers.go`: validate and stream admin license content only after mandatory audit insertion.
- `backend/internal/app/server.go`: register controlled public/admin routes and split operation-log construction/insertion from best-effort legacy calls.
- `backend/tests/file_upload_test.go`: prove license upload/confirm privacy and product-image public compatibility.
- `backend/tests/license_file_privacy_test.go`: integration matrix for anonymous paths, authorization, binding, physical-file safety, headers, content, and audit failure.
- `frontend/src/services/http.ts`: bypass JSON envelope validation only for successful Blob responses while retaining the existing 401 refresh/retry path.
- `frontend/src/services/http.test.ts`: verify Blob success, ordinary envelope rejection, and single-flight Blob retry.
- `frontend/src/services/api.ts`: expose `adminLicenseContent(fileID): Promise<AxiosResponse<Blob>>`.
- `frontend/src/pages/admin/merchants/ReviewDetailPage.tsx`: load, display, replace, and revoke the private license preview object URL.
- `frontend/src/pages/admin/merchants/ReviewDetailPage.test.tsx`: verify loading/success/empty/error states and object URL lifetime.
- `deploy/acceptance/license-file-privacy-smoke.sh`: isolated MySQL 8.4 migration and API privacy matrix with explicit confirmation.
- `deploy/acceptance/README.md`: document the isolated F-04/F-13 workflow and production prohibition.
- `Makefile`: add the guarded `acceptance-license-file-privacy-smoke` target.
- `docs/superpowers/reviews/2026-07-26-license-file-privacy-isolated-acceptance.md`: record sanitized hashes, commands, results, retained resources, and production non-change proof.
- `docs/backend-api-checklist.md`, `docs/data-model.md`, `docs/release-readiness.md`, `docs/full-project-code-review-2026-07-24.md`, `docs/production-hardening-repair-plan-2026-07-24.md`: record code-side, isolated-server, and production statuses without rewriting historical findings.

### Task 1: Add the irreversible `0007` migration gate

**Files:**
- Create: `backend/migrations/0007_license_file_privacy.preflight.sql`
- Create: `backend/migrations/0007_license_file_privacy.up.sql`
- Create: `backend/migrations/0007_license_file_privacy.postflight.sql`
- Create: `backend/migrations/license_file_privacy_migration_test.go`

**Interfaces:**
- Consumes: `file_records`, `merchants.license_file_id`, and the exact `0006` columns/indexes `owner_merchant_id`, `capability_token_hash`, `capability_expires_at`, `idx_file_owner_biz_scan`, `uk_file_capability_token`, `idx_file_capability_expires`.
- Produces: markers `license_file_privacy_preflight_passed`, `license_file_privacy_migration_applied`, and `license_file_privacy_postflight_passed`; outputs `file_record_count` and `merchant_license_count` for the acceptance harness.

- [ ] **Step 1: Write the failing migration artifact tests**

Create `license_file_privacy_migration_test.go` with table-driven file-content checks that require:

```go
func TestLicenseFilePrivacyMigrationArtifacts(t *testing.T) {
	tests := map[string][]string{
		"0007_license_file_privacy.preflight.sql": {
			"license_file_privacy_preflight", "table_name = 'file_records'",
			"table_name = 'files'", "owner_merchant_id", "capability_token_hash",
			"capability_expires_at", "idx_file_owner_biz_scan",
			"uk_file_capability_token", "idx_file_capability_expires",
			"MERCHANT_LICENSE", "object_key", "license_file_id",
			"SIGNAL SQLSTATE '45000'", "license_file_privacy_preflight_passed",
		},
		"0007_license_file_privacy.up.sql": {
			"UPDATE file_records", "SET url = ''",
			"biz_type = 'MERCHANT_LICENSE'", "url <> ''",
			"license_file_privacy_migration_applied",
		},
		"0007_license_file_privacy.postflight.sql": {
			"license_file_privacy_postflight", "MERCHANT_LICENSE",
			"PRODUCT_IMAGE", "file_record_count", "merchant_license_count",
			"license_file_privacy_postflight_passed", "SIGNAL SQLSTATE '45000'",
		},
	}
	for name, required := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			for _, snippet := range required {
				if !strings.Contains(string(raw), snippet) {
					t.Errorf("%s missing %q", name, snippet)
				}
			}
		})
	}
}

func TestLicenseFilePrivacyMigrationHasNoDownScript(t *testing.T) {
	_, err := os.Stat("0007_license_file_privacy.down.sql")
	if !os.IsNotExist(err) {
		t.Fatalf("0007 down migration must not exist; stat error = %v", err)
	}
}
```

Also normalize whitespace in the `.up.sql` test and require exactly one `UPDATE file_records`, while rejecting `PRODUCT_IMAGE`, `owner_merchant_id =`, `object_key =`, `scan_status =`, and `updated_at =` in that artifact.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `cd backend && go test ./migrations -run '^TestLicenseFilePrivacyMigration' -count=1 -v`

Expected: FAIL because all three `0007_license_file_privacy` SQL artifacts are absent.

- [ ] **Step 3: Implement the preflight procedure**

Create a MySQL procedure that verifies the canonical table shape and exact F-02 structure before querying data. It must signal SQLSTATE `45000` for each invariant and include these data predicates:

```sql
SELECT COUNT(*) INTO v_bad_license
FROM file_records
WHERE biz_type = 'MERCHANT_LICENSE'
  AND (COALESCE(object_key, '') = ''
    OR mime_type NOT IN ('image/jpeg', 'image/png', 'image/webp', 'image/heic', 'image/heif')
    OR scan_status NOT IN ('PENDING', 'PASS', 'BLOCKED'));

SELECT COUNT(*) INTO v_bad_bound_license
FROM merchants m
LEFT JOIN file_records f ON f.id = m.license_file_id
LEFT JOIN merchant_accounts ma ON ma.id = f.uploader_id
WHERE m.license_file_id IS NOT NULL
  AND (f.id IS NULL
    OR f.biz_type <> 'MERCHANT_LICENSE'
    OR f.scan_status <> 'PASS'
    OR COALESCE(f.object_key, '') = ''
    OR f.owner_merchant_id IS NULL
    OR f.owner_merchant_id <> m.id
    OR (f.uploader_type = 'MERCHANT' AND (ma.id IS NULL OR ma.merchant_id <> m.id)));
```

For each required index, query `information_schema.statistics`, group by index name, and require the same ordered columns and uniqueness as `0006` postflight. End with `SELECT 'license_file_privacy_preflight_passed' AS migration_gate;`.

- [ ] **Step 4: Implement the one-purpose up migration**

Create `0007_license_file_privacy.up.sql` containing only the guarded procedure and this mutation:

```sql
UPDATE file_records
SET url = ''
WHERE biz_type = 'MERCHANT_LICENSE' AND url <> '';

SELECT 'license_file_privacy_migration_applied' AS migration_gate;
```

Do not update timestamps or any ownership, object-key, scan, MIME, or product-image column.

- [ ] **Step 5: Implement postflight invariants and count output**

The procedure must signal when any license URL remains, when any `PASS` product image has an empty URL, or when the exact `0006` columns/indexes have drifted. End with:

```sql
SELECT COUNT(*) AS file_record_count FROM file_records;
SELECT COUNT(*) AS merchant_license_count
FROM file_records WHERE biz_type = 'MERCHANT_LICENSE';
SELECT 'license_file_privacy_postflight_passed' AS migration_gate;
```

- [ ] **Step 6: Run migration tests and full backend package compile**

Run: `cd backend && go test ./migrations -run '^TestLicenseFilePrivacyMigration' -count=1 -v`

Expected: PASS.

Run: `cd backend && go test ./... -run '^$' -count=1`

Expected: PASS with every backend package compiling.

- [ ] **Step 7: Commit the migration gate**

```bash
git add backend/migrations/0007_license_file_privacy.preflight.sql \
  backend/migrations/0007_license_file_privacy.up.sql \
  backend/migrations/0007_license_file_privacy.postflight.sql \
  backend/migrations/license_file_privacy_migration_test.go
git commit -m "feat(files): add license privacy migration gate"
```

### Task 2: Make upload completion rules business-type aware

**Files:**
- Modify: `backend/internal/app/file_binding.go`
- Modify: `backend/internal/app/file_binding_test.go`
- Modify: `backend/internal/app/file_handlers.go`
- Modify: `backend/tests/file_upload_test.go`
- Modify: `backend/tests/file_binding_helpers_test.go`
- Modify: `backend/tests/file_binding_security_test.go`

**Interfaces:**
- Consumes: `model.FileBizProductImage`, `model.FileBizMerchantLicense`, `model.FileScanPass`, `publicFileURL`, anonymous capability claims, merchant owner validation.
- Produces: `fileHasCompletedStorage(file model.FileRecord) bool`; license success maps without `url`; product-image success maps with nonempty `url`.

- [ ] **Step 1: Write RED binding-readiness tests**

Extend `file_binding_test.go` so a `MERCHANT_LICENSE` with `ScanStatus=PASS`, nonempty `ObjectKey`, empty `URL`, and matching owner is valid, while a license with an empty object key is invalid. Keep an explicit `PRODUCT_IMAGE` case proving an empty URL is invalid:

```go
tests := []struct {
	name     string
	bizType  string
	objectKey string
	url      string
	wantOK   bool
}{
	{"product image requires url", model.FileBizProductImage, "product_image/a.jpg", "", false},
	{"license accepts private object", model.FileBizMerchantLicense, "merchant_license/a.jpg", "", true},
	{"license requires object key", model.FileBizMerchantLicense, "", "", false},
}
```

Change the public claim invalid-case test from `empty URL` to `empty object key`, and add a positive claim test whose URL is empty.

- [ ] **Step 2: Write RED upload and confirm response tests**

Update `TestFileUploadLocalPublicLicense` to require:

```go
if _, exists := upload.Data["url"]; exists {
	t.Fatal("merchant license upload exposed a public url")
}
if record.URL != "" || record.ObjectKey == "" || record.ScanStatus != model.FileScanPass {
	t.Fatalf("private license state = %+v", record)
}
```

Add `TestProductImageUploadRemainsPublic` and a confirm pair proving product images contain `/uploads/product_image/...`, while merchant-license confirm omits `url` and leaves the database URL empty.

- [ ] **Step 3: Run focused tests and verify RED**

Run: `cd backend && go test ./internal/app ./tests -run 'Test.*(Binding|Claim|FileUploadLocalPublicLicense|ProductImageUploadRemainsPublic|Confirm)' -count=1 -v`

Expected: FAIL because binding still requires URL globally and license upload/confirm still persist and return a public URL.

- [ ] **Step 4: Implement type-aware binding readiness**

Add and use this exact helper:

```go
func fileHasCompletedStorage(file model.FileRecord) bool {
	switch file.BizType {
	case model.FileBizProductImage:
		return strings.TrimSpace(file.URL) != ""
	case model.FileBizMerchantLicense:
		return strings.TrimSpace(file.ObjectKey) != ""
	default:
		return false
	}
}
```

Keep type, PASS status, owner, missing-ID, duplicate-ID, and row-lock checks unchanged. Change `claimPublicMerchantLicense` SQL from `url <> ''` to `object_key <> ''` only.

- [ ] **Step 5: Implement private license upload/confirm persistence**

In both upload and confirm paths, derive a URL only for product images:

```go
url := ""
if file.BizType == model.FileBizProductImage {
	url = s.publicFileURL(file.ObjectKey)
}
updates := map[string]interface{}{
	"url": url,
	"scan_status": model.FileScanPass,
}
response := gin.H{"file_id": file.ID, "object_key": file.ObjectKey, "status": model.FileScanPass}
if url != "" {
	response["url"] = url
}
common.Success(c, response)
```

Preserve processed MIME and size updates in the upload path. Ensure confirm also returns `object_key` so both success contracts agree.

- [ ] **Step 6: Update F-02 fixtures and run regressions**

Change license fixtures in `file_binding_helpers_test.go` and `file_binding_security_test.go` to use `merchant_license/...` object keys with empty URLs. Keep product fixtures on `product_image/...` with nonempty URLs. Run:

`cd backend && go test ./internal/app ./tests -run 'Test.*(FileBinding|License|ProductImage|Register|Reapply|FileUpload|FileConfirm)' -count=1 -v`

Expected: PASS, including replay, expiry, owner mismatch, wrong type, non-PASS, and product ownership regressions.

- [ ] **Step 7: Format and commit**

Run: `gofmt -w backend/internal/app/file_binding.go backend/internal/app/file_binding_test.go backend/internal/app/file_handlers.go backend/tests/file_upload_test.go backend/tests/file_binding_helpers_test.go backend/tests/file_binding_security_test.go`

```bash
git add backend/internal/app/file_binding.go backend/internal/app/file_binding_test.go \
  backend/internal/app/file_handlers.go backend/tests/file_upload_test.go \
  backend/tests/file_binding_helpers_test.go backend/tests/file_binding_security_test.go
git commit -m "fix(files): keep license uploads private"
```

### Task 3: Replace the public upload directory with an exact product-image handler

**Files:**
- Create: `backend/internal/app/public_file_handlers.go`
- Modify: `backend/internal/app/file_handlers.go`
- Modify: `backend/internal/app/server.go`
- Create: `backend/tests/license_file_privacy_test.go`

**Interfaces:**
- Consumes: `Server.cfg.FileUploadLocalDir`, exact object-key prefix `product_image/`, and Gin wildcard `c.Param("path")`.
- Produces: `normalizeObjectKey(raw string) (string, error)`, `openLocalRegularFile(objectKey string) (*os.File, os.FileInfo, error)`, and `handlePublicProductImage(c *gin.Context)`.

- [ ] **Step 1: Write the RED anonymous-path matrix**

Create test helpers that write a product image and a merchant license beneath the test server upload root. Add table cases:

```go
tests := []struct {
	path       string
	wantStatus int
}{
	{"/uploads/product_image/public.jpg", http.StatusOK},
	{"/uploads/merchant_license/private.jpg", http.StatusNotFound},
	{"/uploads/PRODUCT_IMAGE/public.jpg", http.StatusNotFound},
	{"/uploads/other/public.jpg", http.StatusNotFound},
	{"/uploads/product_image/../merchant_license/private.jpg", http.StatusNotFound},
	{"/uploads/product_image/%2e%2e/merchant_license/private.jpg", http.StatusNotFound},
	{"/uploads/product_image", http.StatusNotFound},
}
```

For the 200 case, assert exact bytes and `X-Content-Type-Options: nosniff`. For all 404 cases, assert the response does not contain the object key or local absolute path.

- [ ] **Step 2: Add a RED symlink-escape test**

On platforms supporting symlinks, create a file outside the upload root and a symlink under `product_image/escape.jpg`. Request it and require 404 with no outside bytes. Skip only when `os.Symlink` itself is unsupported.

- [ ] **Step 3: Run the public-handler tests and verify RED**

Run: `cd backend && go test ./tests -run '^TestPublicUpload' -count=1 -v`

Expected: FAIL because `r.Static` still serves the whole directory and follows the existing broad mapping.

- [ ] **Step 4: Implement strict object-key normalization and canonical file opening**

`normalizeObjectKey` must reject leading slash after wildcard removal, backslashes, empty segments, `.`, `..`, and any value whose `path.Clean` output differs from the input. `openLocalRegularFile` must:

```go
root, err := filepath.Abs(s.cfg.FileUploadLocalDir)
realRoot, err := filepath.EvalSymlinks(root)
target, err := s.localUploadPath(objectKey)
realTarget, err := filepath.EvalSymlinks(target)
rel, err := filepath.Rel(realRoot, realTarget)
if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
	return nil, nil, common.ErrNotFound
}
file, err := os.Open(realTarget)
stat, err := file.Stat()
if err != nil || !stat.Mode().IsRegular() {
	file.Close()
	return nil, nil, common.ErrNotFound
}
```

Return generic not-found errors for invalid paths; do not expose filesystem details.

- [ ] **Step 5: Implement and register the allowlist handler**

Remove the exact current call `r.Static("/uploads", cfg.FileUploadLocalDir)`. After constructing `s`, register only local storage with `r.GET("/uploads/*path", s.handlePublicProductImage)`. The handler must require `strings.HasPrefix(key, "product_image/")`, set `nosniff`, determine the image MIME from the file bytes or extension, and stream with `http.ServeContent`. All validation/open failures return `c.Status(http.StatusNotFound)` with an empty body.

- [ ] **Step 6: Run focused and upload compatibility tests**

Run: `cd backend && go test ./tests -run 'Test(PublicUpload|ProductImageUploadRemainsPublic|FileUploadLocalPublicLicense)' -count=1 -v`

Expected: PASS; product bytes remain public, license paths and traversal/symlink cases are 404.

- [ ] **Step 7: Format and commit**

Run: `gofmt -w backend/internal/app/public_file_handlers.go backend/internal/app/file_handlers.go backend/internal/app/server.go backend/tests/license_file_privacy_test.go`

```bash
git add backend/internal/app/public_file_handlers.go backend/internal/app/file_handlers.go \
  backend/internal/app/server.go backend/tests/license_file_privacy_test.go
git commit -m "fix(files): restrict anonymous uploads to product images"
```

### Task 4: Add the active-admin private content endpoint with fail-closed audit

**Files:**
- Create: `backend/internal/app/private_file_handlers.go`
- Modify: `backend/internal/app/server.go`
- Modify: `backend/tests/license_file_privacy_test.go`

**Interfaces:**
- Consumes: `parseUintParam`, `common.GetActor`, `middleware.RequireAuth(model.UserTypeAdmin)`, `middleware.RequireActiveAdminSession`, `openLocalRegularFile`, `model.FileRecord`, `model.Merchant`, `model.OperationLog`.
- Produces: `GET /api/v1/admin/files/:id/content`; `buildOperationLog(c *gin.Context, resourceType string, resourceID uint64, action string, fromStatus, toStatus *string, code int, merchantID *uint64, detail map[string]interface{}) model.OperationLog`; `insertOperationLog(tx *gorm.DB, log *model.OperationLog) error`; `handleAdminFileContent(c *gin.Context)`.

- [ ] **Step 1: Write RED authorization and undisclosable-state tests**

Add a fixture that creates a merchant, a bound PASS license with an empty URL, a physical JPEG, and an active admin token. Assert success for the admin and these failures:

```go
tests := []struct {
	name     string
	mutate   func(*testing.T, *app.Server, *model.FileRecord, *model.Merchant)
	wantHTTP int
	wantCode int
}{
	{"missing record", func(t *testing.T, s *app.Server, f *model.FileRecord, _ *model.Merchant) {
		t.Helper(); requireNoDBError(t, s.DB.Delete(f).Error)
	}, http.StatusNotFound, common.CodeNotFound},
	{"wrong type", func(t *testing.T, s *app.Server, f *model.FileRecord, _ *model.Merchant) {
		t.Helper(); requireNoDBError(t, s.DB.Model(f).Update("biz_type", model.FileBizProductImage).Error)
	}, http.StatusNotFound, common.CodeNotFound},
	{"non pass", func(t *testing.T, s *app.Server, f *model.FileRecord, _ *model.Merchant) {
		t.Helper(); requireNoDBError(t, s.DB.Model(f).Update("scan_status", model.FileScanPending).Error)
	}, http.StatusNotFound, common.CodeNotFound},
	{"empty object key", func(t *testing.T, s *app.Server, f *model.FileRecord, _ *model.Merchant) {
		t.Helper(); requireNoDBError(t, s.DB.Model(f).Update("object_key", "").Error)
	}, http.StatusNotFound, common.CodeNotFound},
	{"owner missing", func(t *testing.T, s *app.Server, f *model.FileRecord, _ *model.Merchant) {
		t.Helper(); requireNoDBError(t, s.DB.Model(f).Update("owner_merchant_id", nil).Error)
	}, http.StatusNotFound, common.CodeNotFound},
	{"merchant reference missing", func(t *testing.T, s *app.Server, _ *model.FileRecord, m *model.Merchant) {
		t.Helper(); requireNoDBError(t, s.DB.Model(m).Update("license_file_id", nil).Error)
	}, http.StatusNotFound, common.CodeNotFound},
	{"merchant reference mismatch", func(t *testing.T, s *app.Server, f *model.FileRecord, m *model.Merchant) {
		t.Helper(); other := f.ID + 1000; requireNoDBError(t, s.DB.Model(m).Update("license_file_id", other).Error)
	}, http.StatusNotFound, common.CodeNotFound},
	{"physical file missing", func(t *testing.T, _ *app.Server, f *model.FileRecord, _ *model.Merchant) {
		t.Helper(); requireNoFileError(t, os.Remove(licenseFixturePath(t, f.ObjectKey)))
	}, http.StatusNotFound, common.CodeNotFound},
}
```

Define `requireNoDBError`, `requireNoFileError`, and `licenseFixturePath` in the same test file: each helper calls `t.Helper()`, fails immediately on a nonnil error, and `licenseFixturePath` joins the fixture upload root saved by `newLicensePrivacyFixture` with the normalized object key.

Use raw `httptest.ResponseRecorder` assertions because success is binary, not the JSON envelope. Require no license bytes in every failure response.

- [ ] **Step 2: Write RED actor/session tests**

Call the endpoint with no token, a merchant token, a buyer token, and a revoked admin session token. Require existing 401/403 contracts and zero file bytes. For revoked admin, update only that `auth_sessions.revoked_at` row and prove a fresh active admin token still succeeds.

- [ ] **Step 3: Write RED success headers and audit assertions**

For a valid admin require exact content bytes, image MIME, `Content-Disposition: inline`, `Cache-Control: private, no-store`, `Pragma: no-cache`, and `X-Content-Type-Options: nosniff`. Load the single `admin_file_read` log and assert:

```go
if log.OperatorType != model.UserTypeAdmin || log.OperatorID != adminID ||
	log.MerchantID == nil || *log.MerchantID != merchant.ID ||
	log.ResourceType != "file" || log.ResourceID != file.ID ||
	log.ResultCode != common.CodeOK {
	t.Fatalf("unexpected admin_file_read log: %+v", log)
}
detail := string(log.DetailJSON)
if detail != `{"biz_type":"MERCHANT_LICENSE","scan_status":"PASS"}` {
	t.Fatalf("unsafe audit detail: %s", detail)
}
```

Also reject detail containing `object_key`, `uploads`, `token`, or the file bytes.

- [ ] **Step 4: Write the RED audit-failure test**

Register a GORM create callback scoped to `model.OperationLog` that returns `errors.New("forced operation log failure")`. Request a valid file and require HTTP 500 / code `20001`, zero license bytes, and no success headers. Remove the callback with `t.Cleanup`.

- [ ] **Step 5: Run admin content tests and verify RED**

Run: `cd backend && go test ./tests -run '^TestAdminLicenseContent' -count=1 -v`

Expected: FAIL with route 404 because the endpoint does not exist.

- [ ] **Step 6: Split operation-log construction from persistence**

Refactor existing logging without changing best-effort callers:

```go
func (s *Server) buildOperationLog(c *gin.Context, resourceType string, resourceID uint64,
	action string, fromStatus, toStatus *string, code int, merchantID *uint64,
	detail map[string]interface{}) model.OperationLog

func (s *Server) insertOperationLog(tx *gorm.DB, log *model.OperationLog) error

func (s *Server) writeOperationLog(/* existing signature */) {
	log := s.buildOperationLog(/* existing arguments */)
	_ = s.insertOperationLog(tx, &log)
}
```

Marshal `nil` detail consistently with existing behavior. The new admin handler must call `insertOperationLog` and check its error.

- [ ] **Step 7: Implement exact validation order and binary response**

In `handleAdminFileContent`, perform: ID parse; record load; local-provider check; type/PASS/object-key/MIME checks; nonnil owner; `merchants` lookup by both owner ID and `license_file_id`; safe physical open/stat; mandatory log insert; headers; stream. Map record/state/path absence to `common.ErrNotFound`, infrastructure/provider/audit failures to `common.ErrInternal`.

Use only allowed image MIME values and stream after the audit succeeds:

```go
log := s.buildOperationLog(c, "file", file.ID, "admin_file_read", nil, nil,
	common.CodeOK, file.OwnerMerchantID,
	map[string]interface{}{"biz_type": model.FileBizMerchantLicense, "scan_status": model.FileScanPass})
if err := s.insertOperationLog(nil, &log); err != nil {
	common.Fail(c, common.ErrInternal)
	return
}
c.Header("Content-Disposition", "inline")
c.Header("Cache-Control", "private, no-store")
c.Header("Pragma", "no-cache")
c.Header("X-Content-Type-Options", "nosniff")
c.DataFromReader(http.StatusOK, stat.Size(), file.MimeType, openedFile, nil)
```

Register `admin.GET("/files/:id/content", s.handleAdminFileContent)` inside the existing authenticated admin group.

- [ ] **Step 8: Run focused tests, security regressions, and format**

Run: `cd backend && go test ./tests -run 'Test(AdminLicenseContent|AdminSession|FileBinding|FileUpload)' -count=1 -v`

Expected: PASS.

Run: `gofmt -w backend/internal/app/private_file_handlers.go backend/internal/app/server.go backend/tests/license_file_privacy_test.go`

- [ ] **Step 9: Commit the private endpoint**

```bash
git add backend/internal/app/private_file_handlers.go backend/internal/app/server.go \
  backend/tests/license_file_privacy_test.go
git commit -m "feat(admin): stream audited private licenses"
```

### Task 5: Add authenticated Blob preview to the admin review page

**Files:**
- Modify: `frontend/src/services/http.ts`
- Create: `frontend/src/services/http.test.ts`
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/pages/admin/merchants/ReviewDetailPage.tsx`
- Create: `frontend/src/pages/admin/merchants/ReviewDetailPage.test.tsx`

**Interfaces:**
- Consumes: Axios `responseType: 'blob'`, existing request bearer-token injection, existing single-flight `refreshPromise`, `MerchantDetail.license_file_id`.
- Produces: `api.adminLicenseContent(fileID: string | number): Promise<AxiosResponse<Blob>>`; an in-memory `licensePreviewURL: string | null` that is revoked on replacement and cleanup.

- [ ] **Step 1: Write RED HTTP interceptor tests**

Mock the Axios adapter rather than replacing `http` itself. Verify:

```ts
it('returns successful blob responses without API envelope parsing', async () => {
  const blob = new Blob(['license'], { type: 'image/jpeg' })
  const response = await http.get('/admin/files/7/content', {
    responseType: 'blob',
    adapter: async (config) => ({ data: blob, status: 200, statusText: 'OK', headers: {}, config })
  })
  expect(response.data).toBe(blob)
})
```

Keep a non-Blob case with `{code: 10003}` that rejects using the mapped error. Add two concurrent Blob requests that first return 401, then pass after one mocked `/auth/refresh`; require one refresh and both retries retain `responseType: 'blob'` and the new Authorization header.

- [ ] **Step 2: Run HTTP tests and verify RED**

Run: `cd frontend && npm test -- --run src/services/http.test.ts`

Expected: FAIL because the success interceptor reads `payload.code` from a Blob and the typed Blob API is absent.

- [ ] **Step 3: Implement the narrow Blob bypass and API method**

At the first line of the success interceptor add:

```ts
if (response.config.responseType === 'blob' && response.data instanceof Blob) {
  return response
}
```

Do not bypass error handling; 401 Blob responses must still enter the existing refresh/retry branch. Add to `api`:

```ts
adminLicenseContent(fileID: string | number) {
  return http.get<Blob>(`/admin/files/${fileID}/content`, { responseType: 'blob' })
},
```

- [ ] **Step 4: Write RED review-page lifecycle tests**

Mock `api.adminMerchantReviewDetail` and `api.adminLicenseContent`, plus `URL.createObjectURL` and `URL.revokeObjectURL`. Cover:

- a stable loading region while Blob fetch is pending;
- `暂无营业执照` when `license_file_id` is null;
- successful Ant Design image rendering using the generated `blob:` URL and visible file ID;
- `营业执照不可用` on 403, 404, or rejected Blob request without falling back to `/uploads`;
- revocation when the file ID changes and when the component unmounts;
- no writes to `localStorage` or `sessionStorage` containing Blob/object URL data.

Use a QueryClient with retries disabled and a `MemoryRouter` route `/admin/merchants/:merchantId`.

- [ ] **Step 5: Run page tests and verify RED**

Run: `cd frontend && npm test -- --run src/pages/admin/merchants/ReviewDetailPage.test.tsx`

Expected: FAIL because the page currently renders only the numeric file ID.

- [ ] **Step 6: Implement preview state with deterministic cleanup**

Use a query keyed by file ID and derive/revoke the object URL in an effect:

```tsx
const licenseFileID = detailQuery.data?.merchant_detail.license_file_id
const licenseQuery = useQuery({
  queryKey: ['admin-license-content', licenseFileID],
  enabled: Boolean(licenseFileID),
  retry: false,
  queryFn: async () => (await api.adminLicenseContent(licenseFileID!)).data
})
const [licensePreviewURL, setLicensePreviewURL] = useState<string | null>(null)

useEffect(() => {
  if (!licenseQuery.data) {
    setLicensePreviewURL(null)
    return
  }
  const nextURL = URL.createObjectURL(licenseQuery.data)
  setLicensePreviewURL(nextURL)
  return () => URL.revokeObjectURL(nextURL)
}, [licenseQuery.data])
```

Import `useEffect`, `Image`, `Spin`, and `Typography`. Render an un-nested `ProCard` titled `营业执照` with a fixed `minHeight` preview region. Show the image with Ant Design preview on success, compact error/empty states otherwise, and keep the existing audit/actions layout unchanged.

- [ ] **Step 7: Run frontend tests and production build**

Run: `cd frontend && npm test -- --run src/services/http.test.ts src/pages/admin/merchants/ReviewDetailPage.test.tsx src/pages/auth/RegisterPage.test.tsx`

Expected: PASS.

Run: `cd frontend && npm run build`

Expected: PASS with TypeScript and Vite production output complete.

- [ ] **Step 8: Commit the frontend preview**

```bash
git add frontend/src/services/http.ts frontend/src/services/http.test.ts \
  frontend/src/services/api.ts \
  frontend/src/pages/admin/merchants/ReviewDetailPage.tsx \
  frontend/src/pages/admin/merchants/ReviewDetailPage.test.tsx
git commit -m "feat(frontend): preview private merchant licenses"
```

### Task 6: Build and run the isolated MySQL 8.4 privacy acceptance matrix

**Files:**
- Create: `deploy/acceptance/license-file-privacy-smoke.sh`
- Modify: `deploy/acceptance/README.md`
- Modify: `Makefile`
- Modify: `backend/migrations/license_file_privacy_migration_test.go`
- Modify: `backend/tests/file_schema_mysql_test.go`

**Interfaces:**
- Consumes: new isolated Compose project `secondhand-license-privacy-acceptance`, migration chain `0001..0007`, `FILE_SCHEMA_MYSQL_TEST=1`, isolated test upload directory, and read-only production container snapshots.
- Produces: guard `LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA`; final marker `isolated license file privacy acceptance passed`; sanitized evidence files and their SHA-256 hashes.

- [ ] **Step 1: Add RED acceptance-script contract checks**

Extend the Go migration test to read the new script and require exact snippets:

```go
required := []string{
	"LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM",
	"I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA",
	"secondhand-license-privacy-acceptance",
	"[[ \"$mysql_version\" == 8.4.* ]]",
	"license_file_privacy_preflight_passed",
	"license_file_privacy_postflight_passed",
	"ERROR 1644 \\\\(45000\\\\)",
	"TestLicenseFilePrivacyWithMigrationOnlyMySQL",
	"AUTO_MIGRATE=false",
	"AUTO_MIGRATE=true",
	"isolated license file privacy acceptance passed",
}
```

- [ ] **Step 2: Add RED migration-only MySQL API coverage**

In `file_schema_mysql_test.go`, add `TestLicenseFilePrivacyWithMigrationOnlyMySQL`. It must create physical product/license bytes in the configured isolated upload root, create an owner merchant and exact license reference, then assert anonymous product 200, anonymous license 404, active-admin content 200, private headers, exact bytes, and one safe `admin_file_read` row. Also assert new license upload stores empty URL.

- [ ] **Step 3: Run contract tests and verify RED**

Run: `cd backend && go test ./migrations -run 'TestLicenseFilePrivacy.*Acceptance' -count=1 -v`

Expected: FAIL because `license-file-privacy-smoke.sh` does not exist.

- [ ] **Step 4: Implement fail-closed SQL fixture matrix**

The script must refuse absent confirmation, non-MySQL-8.4 configuration, missing `.env`, and any Compose project name other than `secondhand-license-privacy-acceptance`. It must snapshot production API/Web/MySQL container ID, state, and restart count using read-only `docker inspect` before and after the run.

For every failure case, rebuild a clean `0001..0006` schema, insert the fixture, run `0007` preflight, require `ERROR 1644 (45000)`, then prove license URL values and row counts are unchanged. Cover: missing `file_records`, simultaneous `files` and `file_records`, missing each F-02 column/index shape, empty license object key, disallowed MIME, illegal scan status, missing owner, owner/reference mismatch, and merchant-uploader mismatch.

- [ ] **Step 5: Implement clean migration and restart matrix**

For valid historical rows, capture product URL, license URL, total rows, and license rows before `0007`; run preflight/up/postflight; assert only license URL becomes empty and all counts/product URL remain exact. Run the migration-only Go API test with `AUTO_MIGRATE=false`, then restart the isolated API/test process with `AUTO_MIGRATE=true` and rerun schema/index/URL assertions to prove GORM does not restore public URLs or duplicate indexes.

Write evidence only under `deploy/acceptance/evidence/license-file-privacy/`, exclude it from Git, and print the final success marker only after the production snapshot comparison passes. Retain the isolated Compose volume/network/container for review.

- [ ] **Step 6: Add the guarded Make target and README procedure**

Add `.PHONY` and:

```make
acceptance-license-file-privacy-smoke:
	@test "$${LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA" || { echo "set LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM for isolated license privacy smoke" >&2; exit 1; }
	./deploy/acceptance/license-file-privacy-smoke.sh
```

The README must state the exact confirmation variable, retained project/path, migration order `0006 -> 0007 -> new API/frontend`, evidence location, and the prohibition on production SQL/deployment/file mutation.

- [ ] **Step 7: Run local syntax and contract checks**

Run: `bash -n deploy/acceptance/license-file-privacy-smoke.sh`

Expected: PASS.

Run: `cd backend && go test ./migrations -run 'TestLicenseFilePrivacy' -count=1 -v`

Expected: PASS.

- [ ] **Step 8: Commit the acceptance harness**

```bash
git add deploy/acceptance/license-file-privacy-smoke.sh deploy/acceptance/README.md \
  Makefile backend/migrations/license_file_privacy_migration_test.go \
  backend/tests/file_schema_mysql_test.go
git commit -m "test(acceptance): verify private license access"
```

- [ ] **Step 9: Obtain scoped transfer authorization and execute the isolated server matrix**

Before any transfer, obtain explicit user authorization for a new remote path `/home/yu/services/secondhand-license-privacy-acceptance-20260726`, the exact source whitelist, and the new Compose project `secondhand-license-privacy-acceptance`. The earlier F-02 authorization is limited to its original path/project and does not authorize this transfer. Do not transfer `.env`, secrets, databases, uploads, evidence, `.git`, caches, `node_modules`, `backend/app.db`, or the three protected review documents. Transfer only the approved committed whitelist, record local and remote SHA-256 values, and require exact equality before execution.

Run the guarded target against the retained isolated Compose project and MySQL 8.4 instance. Do not run the script from the production service directory. Capture the success marker, MySQL version, migration markers, API test PASS line, AutoMigrate check, and before/after production container snapshot.

Expected final line: `isolated license file privacy acceptance passed`.

### Task 7: Record evidence, update finding statuses, and pass final review gates

**Files:**
- Create: `docs/superpowers/reviews/2026-07-26-license-file-privacy-isolated-acceptance.md`
- Modify: `docs/backend-api-checklist.md`
- Modify: `docs/data-model.md`
- Modify: `docs/release-readiness.md`
- Modify: `docs/full-project-code-review-2026-07-24.md`
- Modify: `docs/production-hardening-repair-plan-2026-07-24.md`

**Interfaces:**
- Consumes: exact commit SHAs, test command output, local/remote source SHA-256 values, isolated evidence SHA-256 values, MySQL version, retained resource identifiers, and production before/after snapshots from Task 6.
- Produces: authoritative F-04/F-13 status distinguishing `代码侧关闭`, `隔离 MySQL 8.4 测试服务器审核通过`, and `生产未迁移/未部署/未改文件`.

- [ ] **Step 1: Write the tracked acceptance report from captured evidence**

Record:

- branch and commit range;
- approved design and this implementation plan paths;
- every local and remote source hash with exact match result;
- MySQL 8.4 version and all migration/API/AutoMigrate matrix cases;
- sanitized evidence filenames with SHA-256;
- retained remote path `/home/yu/services/secondhand-license-privacy-acceptance-20260726` and Compose project `secondhand-license-privacy-acceptance`;
- production API/Web/MySQL container ID/state/restart-count equality;
- explicit statements that production `0006`/`0007` were not executed, applications were not deployed, production files/data were not read or modified, and F-06 remains open.

Do not include IPs, credentials, tokens, object keys, local absolute upload paths, file bytes, `.env` values, or raw database rows.

- [ ] **Step 2: Update API and data-model contracts**

Document `GET /api/v1/admin/files/:id/content`, binary response headers, admin active-session requirement, uniform 404 states, mandatory `admin_file_read`, exact safe log detail, and audit failure behavior. Document that `MERCHANT_LICENSE.url=''` is intentional while `object_key` is required, and that `PRODUCT_IMAGE.url` remains required/public.

- [ ] **Step 3: Append dated status evidence to current review/release documents**

Append a `2026-07-26 F-04/F-13 后续核验` section to `full-project-code-review-2026-07-24.md`; do not rewrite the original finding text. Update readiness and hardening tables to state code-side closure and isolated-server approval only. Preserve the production-open state until the maintenance-window migration/deployment/file verification actually occurs.

- [ ] **Step 4: Run formatting and complete local verification**

Run:

```bash
gofmt -w backend/internal/app/file_binding.go backend/internal/app/file_binding_test.go \
  backend/internal/app/file_handlers.go backend/internal/app/public_file_handlers.go \
  backend/internal/app/private_file_handlers.go backend/internal/app/server.go \
  backend/tests/file_upload_test.go backend/tests/file_binding_helpers_test.go \
  backend/tests/file_binding_security_test.go backend/tests/license_file_privacy_test.go \
  backend/tests/file_schema_mysql_test.go \
  backend/migrations/license_file_privacy_migration_test.go
(cd backend && go test ./...)
(cd frontend && npm test && npm run build)
(cd miniapp && npm test && npm run build:weapp)
bash -n deploy/acceptance/license-file-privacy-smoke.sh
git diff --check
```

Expected: all commands exit 0. If a suite fails or hangs, apply `superpowers:systematic-debugging`, fix the root cause, rerun the focused RED/GREEN case, then rerun this complete gate before documenting success.

- [ ] **Step 5: Audit scope and protected files before commit**

Run:

```bash
git status --short
git diff --name-only HEAD
git diff --check
git ls-files --error-unmatch backend/app.db
```

Confirm no evidence directory, `.env`, secret, database, upload, `.tmp/`, or protected review document is staged. `backend/app.db` may still be tracked because F-10 is outside this repair, but it must have no content change in this commit range.

- [ ] **Step 6: Perform two-stage code review**

Use `superpowers:requesting-code-review` for:

1. specification compliance against the approved design and this plan;
2. code quality/security review of path handling, authorization, audit ordering, Blob lifecycle, migration fail-closed behavior, and evidence claims.

Resolve every High/Medium issue, add regression coverage, rerun focused and full gates, and record reviewer outcome in the acceptance report. Do not mark F-04/F-13 closed while any required issue or test remains unresolved.

- [ ] **Step 7: Commit documentation and evidence**

```bash
git add docs/superpowers/reviews/2026-07-26-license-file-privacy-isolated-acceptance.md \
  docs/backend-api-checklist.md docs/data-model.md docs/release-readiness.md \
  docs/full-project-code-review-2026-07-24.md \
  docs/production-hardening-repair-plan-2026-07-24.md
git commit -m "docs(acceptance): record private license evidence"
```

- [ ] **Step 8: Verify the review-ready branch head**

Run:

```bash
git status --short --branch
git log --oneline --decorate -12
git diff --stat 1db0e18..HEAD
git diff --check 1db0e18..HEAD
```

Expected: only the known untracked `.tmp/` and three protected documents remain; all F-04/F-13 implementation/evidence changes are committed on `codex/reconcile-code-reviews`; no production action is represented as completed.

## Plan Self-Review Record

- Spec coverage: Tasks 2-5 cover all runtime rules; Task 1 covers irreversible migration; Task 6 covers every isolated MySQL/API/AutoMigrate and production non-change gate; Task 7 covers traceability and precise status language.
- Placeholder scan: no deferred implementation markers or unspecified error-handling steps remain; every code-changing task includes concrete interfaces, RED/GREEN commands, and commit boundaries.
- Type consistency: `fileHasCompletedStorage(model.FileRecord) bool`, `normalizeObjectKey(string) (string, error)`, `openLocalRegularFile(string) (*os.File, os.FileInfo, error)`, `insertOperationLog(*gorm.DB, *model.OperationLog) error`, and `adminLicenseContent(string | number)` use the same names and contracts at every consumer.
- Security order: physical file validation and successful mandatory audit insertion both occur before headers or bytes; all invalid record/binding/path states collapse to 404.
- Scope review: F-06 and production rollout remain explicitly open; protected/untracked documents and production data are excluded from every staged path.
