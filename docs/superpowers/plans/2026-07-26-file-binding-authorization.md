# File Binding Authorization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close F-02 by preventing a merchant from binding missing, unfinished, wrongly typed, or foreign files to products and merchant licenses, including safe one-time claiming of anonymously uploaded registration licenses.

**Architecture:** Add explicit `owner_merchant_id` ownership to canonical `file_records`, plus a short-lived hashed capability for anonymous license uploads. Centralize row-locked binding validation in `backend/internal/app/file_binding.go`; registration claims a public license atomically, while reapply and product paths validate already-owned files inside their existing transactions.

**Tech Stack:** Go 1.22, Gin, GORM, SQLite tests, MySQL 8.4 migration gates, React 18, TypeScript, Axios, Vitest, Testing Library, Node 22.22.2.

## Global Constraints

- Execute only after F-09 is committed and `0005_file_records_table.{preflight,up,postflight}.sql` is stable.
- Work on `codex/reconcile-code-reviews` in an isolated worktree created with `superpowers:using-git-worktrees` when execution begins.
- Do not modify `backend/internal/app/order_handlers.go`, `backend/migrations/0004_merchant_multi_stock.*`, or any inventory/order behavior.
- Do not implement F-04 admin license preview, F-06 upload quotas/cleanup, or F-13 private license delivery.
- Do not deploy applications, run production migrations, or write production data.
- Never stage or commit the three protected untracked review documents: `docs/architecture-evolution-plan-2026-07-24.md`, `docs/first-round-fix-review-2026-07-24.md`, and `docs/second-round-fix-review-2026-07-24.md`.
- Raw capability tokens must never be stored, logged, placed in URLs, or persisted in browser storage.
- Keep API field names exactly `file_token` for upload/confirm and `license_file_token` for registration.
- All invalid binding/capability cases return HTTP 400 with business code `10012`; database failures remain `20001`.
- Production schema changes use `0006_file_binding_ownership.preflight.sql`, `.up.sql`, and `.postflight.sql`; no down migration is allowed.
- Use TDD for every behavior change and run `gofmt` on all changed Go files.

## Pre-Execution Gate

- [ ] Confirm F-09 is committed and the worktree contains no overlapping uncommitted changes:

```bash
git log -1 --oneline
git status --short
git ls-files backend/migrations/0005_file_records_table.preflight.sql backend/migrations/0005_file_records_table.up.sql backend/migrations/0005_file_records_table.postflight.sql
```

Expected: all three `0005` paths are tracked; Grok's F-09 implementation is no longer an uncommitted worktree diff. If this gate fails, stop before Task 1.

- [ ] Establish the green baseline:

```bash
make test
bash -lc 'source ~/.nvm/nvm.sh && nvm use 22.22.2 >/dev/null && cd frontend && npm test && npm run build'
```

Expected: backend, frontend tests, and frontend build exit 0. Record existing Rollup warnings, but do not expand F-02 to fix them.

## File Map

| Path | Responsibility |
| --- | --- |
| `backend/internal/model/models.go` | Persisted ownership and capability fields |
| `backend/internal/common/errors.go` | Stable `10012` API error |
| `backend/internal/dto/dto.go` | Registration and confirm token request fields |
| `backend/internal/app/file_binding.go` | Token hashing/generation, row-locked ownership validation, atomic license claim |
| `backend/internal/app/file_handlers.go` | Presign ownership, anonymous upload/confirm capability authorization |
| `backend/internal/app/auth_handlers.go` | Atomic first-registration license claim |
| `backend/internal/app/merchant_handlers.go` | Reapply license validation |
| `backend/internal/app/product_handlers.go` | Product image validation before relationship writes |
| `backend/migrations/0006_file_binding_ownership.*.sql` | Fail-closed preflight, schema/backfill, postflight |
| `backend/tests/file_binding_security_test.go` | Cross-merchant API regressions and transactional rollback |
| `frontend/src/pages/auth/RegisterPage.tsx` | Keep and submit anonymous license capability in memory |
| `frontend/src/pages/auth/RegisterPage.test.tsx` | Registration capability UI contract |
| `deploy/acceptance/file-binding-authorization-smoke.sh` | Disposable MySQL 8.4 matrix and concurrency replay |

---

### Task 1: Add the Ownership Schema and Migration Gates

**Files:**
- Modify: `backend/internal/model/models.go`
- Create: `backend/internal/model/file_binding_test.go`
- Create: `backend/migrations/file_binding_migration_test.go`
- Create: `backend/migrations/0006_file_binding_ownership.preflight.sql`
- Create: `backend/migrations/0006_file_binding_ownership.up.sql`
- Create: `backend/migrations/0006_file_binding_ownership.postflight.sql`

**Interfaces:**
- Consumes: canonical `model.FileRecord.TableName() == "file_records"` from F-09.
- Produces: `FileRecord.OwnerMerchantID *uint64`, `CapabilityTokenHash *string`, and `CapabilityExpiresAt *time.Time`; MySQL markers `file_binding_ownership_preflight_passed`, `file_binding_ownership_migration_applied`, and `file_binding_ownership_postflight_passed`.

- [ ] **Step 1: Write failing model-contract tests**

Create `backend/internal/model/file_binding_test.go`:

```go
package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestFileRecordHasBindingOwnershipFields(t *testing.T) {
	typ := reflect.TypeOf(FileRecord{})
	wantTags := map[string][]string{
		"OwnerMerchantID":     {"idx_file_owner_biz_scan", "priority:1"},
		"CapabilityTokenHash": {"type:char(64)", "uk_file_capability_token"},
		"CapabilityExpiresAt": {"idx_file_capability_expires"},
	}
	for fieldName, snippets := range wantTags {
		field, ok := typ.FieldByName(fieldName)
		if !ok {
			t.Fatalf("FileRecord missing %s", fieldName)
		}
		tag := field.Tag.Get("gorm")
		for _, snippet := range snippets {
			if !strings.Contains(tag, snippet) {
				t.Errorf("%s gorm tag %q missing %q", fieldName, tag, snippet)
			}
		}
	}
}
```

- [ ] **Step 2: Write failing migration-artifact tests**

Create `backend/migrations/file_binding_migration_test.go` with a table asserting these exact snippets:

```go
tests := map[string][]string{
	"0006_file_binding_ownership.preflight.sql": {
		"file_binding_ownership_preflight",
		"table_name = 'file_records'",
		"table_name = 'files'",
		"product_images",
		"license_file_id",
		"COUNT(DISTINCT merchant_id)",
		"file_binding_ownership_preflight_passed",
		"SIGNAL SQLSTATE '45000'",
	},
	"0006_file_binding_ownership.up.sql": {
		"owner_merchant_id",
		"capability_token_hash",
		"capability_expires_at",
		"idx_file_owner_biz_scan",
		"uk_file_capability_token",
		"idx_file_capability_expires",
		"file_binding_ownership_migration_applied",
	},
	"0006_file_binding_ownership.postflight.sql": {
		"file_binding_ownership_postflight",
		"owner_merchant_id",
		"file_binding_ownership_postflight_passed",
		"SIGNAL SQLSTATE '45000'",
	},
}
```

Also assert `os.Stat("0006_file_binding_ownership.down.sql")` returns `os.IsNotExist(err)`.

- [ ] **Step 3: Run the focused tests and observe the red state**

```bash
cd backend && GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/model ./migrations -run 'TestFileRecordHasBindingOwnershipFields|TestFileBindingMigration' -count=1 -v
```

Expected: FAIL because the fields and `0006` artifacts do not exist.

- [ ] **Step 4: Add the model fields**

Append these fields to `FileRecord` without changing its existing table name or columns:

```go
OwnerMerchantID     *uint64    `gorm:"index:idx_file_owner_biz_scan,priority:1"`
CapabilityTokenHash *string    `gorm:"type:char(64);uniqueIndex:uk_file_capability_token"`
CapabilityExpiresAt *time.Time `gorm:"index:idx_file_capability_expires"`
```

Add `index:idx_file_owner_biz_scan,priority:2` to `BizType` and `index:idx_file_owner_biz_scan,priority:3` to `ScanStatus`. Preserve `idx_biz_type_created`.

- [ ] **Step 5: Implement fail-closed preflight**

Use a stored procedure named `file_binding_ownership_preflight`. Before any DDL, it must run concrete queries equivalent to:

```sql
SELECT COUNT(*) INTO v_file_records
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = 'file_records';

SELECT COUNT(*) INTO v_files
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = 'files';

IF v_file_records <> 1 OR v_files <> 0 THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'canonical file_records table is required';
END IF;

SELECT COUNT(*) INTO v_bad_product_files
FROM product_images pi
JOIN products p ON p.id = pi.product_id
LEFT JOIN file_records f ON f.id = pi.file_id
WHERE f.id IS NULL OR f.biz_type <> 'PRODUCT_IMAGE'
   OR f.scan_status <> 'PASS' OR COALESCE(f.url, '') = '';

SELECT COUNT(*) INTO v_bad_license_files
FROM merchants m
LEFT JOIN file_records f ON f.id = m.license_file_id
WHERE m.license_file_id IS NOT NULL
  AND (f.id IS NULL OR f.biz_type <> 'MERCHANT_LICENSE'
       OR f.scan_status <> 'PASS' OR COALESCE(f.url, '') = '');

SELECT COUNT(*) INTO v_cross_merchant_files
FROM (
  SELECT refs.file_id
  FROM (
    SELECT pi.file_id, p.merchant_id
    FROM product_images pi JOIN products p ON p.id = pi.product_id
    UNION ALL
    SELECT m.license_file_id, m.id
    FROM merchants m WHERE m.license_file_id IS NOT NULL
  ) refs
  GROUP BY refs.file_id
  HAVING COUNT(DISTINCT refs.merchant_id) > 1
) conflicts;
```

Before each `SIGNAL`, select the affected file IDs and merchant IDs as a diagnostic result set. Add a separate mismatch query for bound rows whose `uploader_type='MERCHANT'` but uploader account is missing or belongs to a different merchant. End only the clean path with `SELECT 'file_binding_ownership_preflight_passed' AS status;`.

- [ ] **Step 6: Implement idempotent up and postflight gates**

The up procedure must conditionally add each column/index through `information_schema` checks, then backfill in this order:

```sql
UPDATE file_records f
JOIN (
  SELECT refs.file_id, MIN(refs.merchant_id) AS merchant_id
  FROM (
    SELECT pi.file_id, p.merchant_id
    FROM product_images pi JOIN products p ON p.id = pi.product_id
    UNION ALL
    SELECT m.license_file_id, m.id
    FROM merchants m WHERE m.license_file_id IS NOT NULL
  ) refs
  GROUP BY refs.file_id
) bound ON bound.file_id = f.id
SET f.owner_merchant_id = bound.merchant_id
WHERE f.owner_merchant_id IS NULL;

UPDATE file_records f
JOIN merchant_accounts ma ON ma.id = f.uploader_id
SET f.owner_merchant_id = ma.merchant_id
WHERE f.owner_merchant_id IS NULL
  AND f.uploader_type = 'MERCHANT';
```

Create the exact indexes from the design. Postflight must verify all three columns, all three indexes, no legacy `files` table, and every existing business reference has the matching `owner_merchant_id`. It must emit only `file_binding_ownership_postflight_passed` on success.

- [ ] **Step 7: Run focused and backend tests**

```bash
gofmt -w backend/internal/model/models.go backend/internal/model/file_binding_test.go backend/migrations/file_binding_migration_test.go
make test
```

Expected: all backend tests pass; the migration artifact tests pass; no `0006` down script exists.

- [ ] **Step 8: Commit only Task 1 files**

```bash
git add backend/internal/model/models.go backend/internal/model/file_binding_test.go backend/migrations/file_binding_migration_test.go backend/migrations/0006_file_binding_ownership.preflight.sql backend/migrations/0006_file_binding_ownership.up.sql backend/migrations/0006_file_binding_ownership.postflight.sql
git commit -m "feat(files): add binding ownership schema gates"
```

---

### Task 2: Build the Central Binding and Claim Component

**Files:**
- Modify: `backend/internal/common/errors.go`
- Create: `backend/internal/app/file_binding.go`
- Create: `backend/internal/app/file_binding_test.go`

**Interfaces:**
- Consumes: Task 1 `FileRecord` fields.
- Produces: `common.CodeInvalidFileBinding`, `common.ErrInvalidFileBinding`, `fileCapabilityHash`, `validateMerchantFilesForBinding`, and `claimPublicMerchantLicense` with the exact signatures below.

- [ ] **Step 1: Write failing validation tests**

Create an in-memory SQLite fixture in package `app` and table-drive these cases:

```go
func TestValidateMerchantFilesForBinding(t *testing.T) {
	db := newFileBindingTestDB(t)
	merchantID := uint64(10)
	otherMerchantID := uint64(20)
	valid := createBindingFile(t, db, model.FileBizProductImage, model.FileScanPass, "/uploads/ok.jpg", &merchantID)
	foreign := createBindingFile(t, db, model.FileBizProductImage, model.FileScanPass, "/uploads/foreign.jpg", &otherMerchantID)
	wrongType := createBindingFile(t, db, model.FileBizMerchantLicense, model.FileScanPass, "/uploads/license.jpg", &merchantID)
	pending := createBindingFile(t, db, model.FileBizProductImage, model.FileScanPending, "", &merchantID)

	tests := []struct {
		name string
		ids  []uint64
		want error
	}{
		{name: "valid", ids: []uint64{valid.ID}},
		{name: "missing", ids: []uint64{999999}, want: common.ErrInvalidFileBinding},
		{name: "duplicate", ids: []uint64{valid.ID, valid.ID}, want: common.ErrInvalidFileBinding},
		{name: "foreign", ids: []uint64{foreign.ID}, want: common.ErrInvalidFileBinding},
		{name: "wrong type", ids: []uint64{wrongType.ID}, want: common.ErrInvalidFileBinding},
		{name: "pending", ids: []uint64{pending.ID}, want: common.ErrInvalidFileBinding},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Transaction(func(tx *gorm.DB) error {
				return validateMerchantFilesForBinding(tx, merchantID, tt.ids, model.FileBizProductImage)
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v want %v", err, tt.want)
			}
		})
	}
}
```

Add explicit `BLOCKED`, empty URL, zero ID, empty list, and mixed valid/invalid multi-file cases.

- [ ] **Step 2: Write failing claim tests**

Use a fixed raw token only in tests and persist only its hash:

```go
func TestClaimPublicMerchantLicenseConsumesTokenOnce(t *testing.T) {
	db := newFileBindingTestDB(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	raw := "test-only-public-license-token"
	hash := fileCapabilityHash(raw)
	expires := now.Add(15 * time.Minute)
	file := model.FileRecord{
		BizType: model.FileBizMerchantLicense, ObjectKey: "license/1.jpg",
		URL: "/uploads/license/1.jpg", MimeType: "image/jpeg", SizeBytes: 10,
		UploaderType: model.UserTypePublic, ScanStatus: model.FileScanPass,
		CapabilityTokenHash: &hash, CapabilityExpiresAt: &expires,
	}
	if err := db.Create(&file).Error; err != nil { t.Fatal(err) }

	if err := db.Transaction(func(tx *gorm.DB) error {
		return claimPublicMerchantLicense(tx, file.ID, raw, 77, now)
	}); err != nil { t.Fatalf("first claim: %v", err) }
	if err := db.Transaction(func(tx *gorm.DB) error {
		return claimPublicMerchantLicense(tx, file.ID, raw, 88, now)
	}); !errors.Is(err, common.ErrInvalidFileBinding) {
		t.Fatalf("second claim = %v", err)
	}
}
```

Add wrong token, expired token, wrong biz type, non-PASS, non-public uploader, already-owned file, and transaction rollback/retry cases.

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd backend && GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/app -run 'TestValidateMerchantFilesForBinding|TestClaimPublicMerchantLicense' -count=1 -v
```

Expected: FAIL because the error and functions are undefined.

- [ ] **Step 4: Add the stable error contract**

Add to `backend/internal/common/errors.go`:

```go
CodeInvalidFileBinding = 10012
```

and:

```go
ErrInvalidFileBinding = NewBizError(CodeInvalidFileBinding, "invalid file binding", http.StatusBadRequest)
```

- [ ] **Step 5: Implement row-locked validation**

Create `backend/internal/app/file_binding.go`:

```go
func validateMerchantFilesForBinding(tx *gorm.DB, merchantID uint64, fileIDs []uint64, wantBizType string) error {
	if merchantID == 0 || len(fileIDs) == 0 {
		return common.ErrInvalidFileBinding
	}
	seen := make(map[uint64]struct{}, len(fileIDs))
	for _, id := range fileIDs {
		if id == 0 {
			return common.ErrInvalidFileBinding
		}
		if _, exists := seen[id]; exists {
			return common.ErrInvalidFileBinding
		}
		seen[id] = struct{}{}
	}
	var files []model.FileRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", fileIDs).Find(&files).Error; err != nil {
		return err
	}
	if len(files) != len(fileIDs) {
		return common.ErrInvalidFileBinding
	}
	for _, file := range files {
		if file.BizType != wantBizType || file.ScanStatus != model.FileScanPass || strings.TrimSpace(file.URL) == "" ||
			file.OwnerMerchantID == nil || *file.OwnerMerchantID != merchantID {
			return common.ErrInvalidFileBinding
		}
	}
	return nil
}
```

- [ ] **Step 6: Implement hashing and atomic claim**

```go
func fileCapabilityHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func claimPublicMerchantLicense(tx *gorm.DB, fileID uint64, rawToken string, merchantID uint64, now time.Time) error {
	if fileID == 0 || merchantID == 0 || strings.TrimSpace(rawToken) == "" {
		return common.ErrInvalidFileBinding
	}
	result := tx.Model(&model.FileRecord{}).
		Where("id = ? AND biz_type = ? AND scan_status = ? AND url <> '' AND uploader_type = ? AND owner_merchant_id IS NULL AND capability_token_hash = ? AND capability_expires_at > ?",
			fileID, model.FileBizMerchantLicense, model.FileScanPass, model.UserTypePublic, fileCapabilityHash(rawToken), now).
		Updates(map[string]interface{}{
			"owner_merchant_id":      merchantID,
			"capability_token_hash":  nil,
			"capability_expires_at": nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.ErrInvalidFileBinding
	}
	return nil
}
```

- [ ] **Step 7: Verify and commit**

```bash
gofmt -w backend/internal/common/errors.go backend/internal/app/file_binding.go backend/internal/app/file_binding_test.go
cd backend && GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/app ./internal/common -count=1
git add backend/internal/common/errors.go backend/internal/app/file_binding.go backend/internal/app/file_binding_test.go
git commit -m "feat(files): add binding authorization core"
```

Expected: focused tests pass and database errors are not converted to `10012`.

---

### Task 3: Secure Anonymous Presign, Upload, and Confirm

**Files:**
- Modify: `backend/internal/dto/dto.go`
- Modify: `backend/internal/app/file_binding.go`
- Modify: `backend/internal/app/file_handlers.go`
- Modify: `backend/tests/file_upload_test.go`

**Interfaces:**
- Consumes: `fileCapabilityHash` and Task 1 capability fields.
- Produces: `newFileCapability(now) (raw, hash string, expiresAt time.Time, err error)`; anonymous presign response `file_token`; multipart/JSON `file_token` authorization.

- [ ] **Step 1: Write failing anonymous-capability API tests**

Extend `TestFileUploadLocalPublicLicense` to assert `file_token` is non-empty, add it to multipart fields, and verify the database stores only its hash. Add separate tests:

```go
func TestAnonymousFileUploadRequiresMatchingCapability(t *testing.T) {
	srv := newTestServer(t)
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE", "file_name": "license.jpg",
		"file_size": 22, "mime_type": "image/jpeg",
	}, nil)
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	resp := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey, "file_token": "wrong-token",
	}, "file", "license.jpg", minimalJPEG(), nil)
	if resp.Code != common.CodeInvalidFileBinding {
		t.Fatalf("wrong token response: %+v", resp)
	}
}
```

Also test missing token, expired token, correct token, and anonymous confirm. Add an authenticated merchant presign assertion that `owner_merchant_id` is set and `file_token` is absent. Add an admin assertion that ownership stays `NULL`.

- [ ] **Step 2: Run the file tests and confirm the red state**

```bash
cd backend && GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./tests -run 'TestFileUploadLocalPublicLicense|TestAnonymousFileUploadRequiresMatchingCapability|TestMerchantPresignSetsOwner' -count=1 -v
```

Expected: FAIL because presign has no token and anonymous upload accepts no capability.

- [ ] **Step 3: Add request fields and capability generation**

Add to `ConfirmUploadRequest`:

```go
FileToken string `json:"file_token"`
```

Add to `file_binding.go`:

```go
const fileCapabilityTTL = 15 * time.Minute

func newFileCapability(now time.Time) (string, string, time.Time, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", "", time.Time{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(random)
	return raw, fileCapabilityHash(raw), now.Add(fileCapabilityTTL), nil
}
```

- [ ] **Step 4: Persist presign ownership or capability**

In `handlePresign`, compute one `now := time.Now()`. For a merchant actor set `OwnerMerchantID = &actor.MerchantID`. For an anonymous request call `newFileCapability(now)`, store only the hash/expiry, and add `file_token` to the response. For admin actors leave owner and capability fields nil. Set `expire_at` from the same `expiresAt` value for anonymous requests.

- [ ] **Step 5: Require capability for public upload/confirm**

Change the loader signature exactly to:

```go
func (s *Server) loadFileRecordAndAuthorize(c *gin.Context, fileID uint64, rawToken string) (*model.FileRecord, error)
```

For `PUBLIC` records require non-empty token, non-nil hash/expiry, `expiry.After(time.Now())`, and constant-time equality between stored and computed hashes. Authenticated merchant/admin behavior remains unchanged. Pass `c.PostForm("file_token")` from upload and `req.FileToken` from confirm. Return `ErrInvalidFileBinding` for every public-token mismatch.

- [ ] **Step 6: Run targeted and full backend tests**

```bash
gofmt -w backend/internal/dto/dto.go backend/internal/app/file_binding.go backend/internal/app/file_handlers.go backend/tests/file_upload_test.go
make test
```

Expected: all backend tests pass; anonymous calls without the correct token fail with `10012`; raw tokens are absent from persisted rows.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/dto/dto.go backend/internal/app/file_binding.go backend/internal/app/file_handlers.go backend/tests/file_upload_test.go
git commit -m "feat(files): secure anonymous upload capabilities"
```

---

### Task 4: Claim the License Atomically During Registration

**Files:**
- Modify: `backend/internal/dto/dto.go`
- Modify: `backend/internal/app/auth_handlers.go`
- Create: `backend/tests/file_binding_helpers_test.go`
- Create: `backend/tests/file_binding_security_test.go`
- Modify: `backend/tests/integration_flow_test.go`
- Modify: `backend/tests/restricted_and_security_test.go`
- Modify: `backend/tests/admin_security_test.go`

**Interfaces:**
- Consumes: `claimPublicMerchantLicense` and presign response `file_token`.
- Produces: required registration field `license_file_token`; helper `uploadReadyPublicLicense(t, srv) (fileID uint64, rawToken string)`.

- [ ] **Step 1: Add a real upload helper for registration tests**

Create `backend/tests/file_binding_helpers_test.go` with a helper that performs presign and upload, rather than treating `PENDING` as complete:

```go
func uploadReadyPublicLicense(t *testing.T, srv *app.Server) (uint64, string) {
	t.Helper()
	jpeg := minimalJPEG()
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": model.FileBizMerchantLicense, "file_name": "license.jpg",
		"file_size": len(jpeg), "mime_type": "image/jpeg",
	}, nil)
	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	rawToken := str(presign.Data["file_token"])
	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey, "file_token": rawToken,
	}, "file", "license.jpg", jpeg, nil)
	if upload.Code != common.CodeOK { t.Fatalf("upload license: %+v", upload) }
	return fileID, rawToken
}
```

Move the minimal JPEG bytes into `minimalJPEG() []byte` so file tests and registration tests share one valid fixture.

- [ ] **Step 2: Write failing registration security tests**

In `file_binding_security_test.go`, cover successful claim and uniform rejection:

```go
func TestRegisterClaimsUploadedLicense(t *testing.T) {
	srv := newTestServer(t)
	fileID, rawToken := uploadReadyPublicLicense(t, srv)
	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register", map[string]interface{}{
		"merchant_name": "Claim Store", "contact_name": "Owner", "phone": "13800138000",
		"username": uniqueUsername("claim"), "password": "Passw0rd!2026",
		"license_file_id": fileID, "license_file_token": rawToken,
	}, nil)
	if resp.Code != common.CodeOK { t.Fatalf("register: %+v", resp) }
	merchantID := numToUint64(resp.Data["merchant_id"])
	var file model.FileRecord
	if err := srv.DB.First(&file, fileID).Error; err != nil { t.Fatal(err) }
	if file.OwnerMerchantID == nil || *file.OwnerMerchantID != merchantID || file.CapabilityTokenHash != nil || file.CapabilityExpiresAt != nil {
		t.Fatalf("license was not consumed atomically: %+v", file)
	}
}
```

Add table cases for missing, wrong, expired, already-consumed token; wrong biz type; `PENDING`; and missing file. For every failure assert zero new merchant, account, and audit rows. Add sequential replay: the first registration succeeds and the second returns `10012`.

- [ ] **Step 3: Run the tests and observe failure**

```bash
cd backend && GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./tests -run 'TestRegisterClaimsUploadedLicense|TestRegisterRejectsInvalidLicenseBinding' -count=1 -v
```

Expected: FAIL because registration ignores the token and saves `license_file_id` before claiming.

- [ ] **Step 4: Make the DTO require the token**

Add:

```go
LicenseFileToken string `json:"license_file_token" binding:"required,min=20,max=256"`
```

to `RegisterRequest`.

- [ ] **Step 5: Reorder the registration transaction**

Remove the pre-transaction assignment `merchant.LicenseFileID = &req.LicenseFileID`. Inside the existing transaction:

```go
if err := tx.Create(&merchant).Error; err != nil { return err }
acct.MerchantID = merchant.ID
if err := tx.Create(&acct).Error; err != nil { return err }
if err := claimPublicMerchantLicense(tx, req.LicenseFileID, req.LicenseFileToken, merchant.ID, time.Now()); err != nil {
	return err
}
if err := tx.Model(&model.Merchant{}).Where("id = ?", merchant.ID).Update("license_file_id", req.LicenseFileID).Error; err != nil {
	return err
}
merchant.LicenseFileID = &req.LicenseFileID
```

Then write the existing audit row. Any failure must return from the transaction without remapping `ErrInvalidFileBinding` to internal error.

- [ ] **Step 6: Update existing registration helpers**

Change `registerMerchant` and all direct registration flows to call `uploadReadyPublicLicense` and send both fields. Do not weaken new production validation to preserve old tests.

- [ ] **Step 7: Verify and commit**

```bash
gofmt -w backend/internal/dto/dto.go backend/internal/app/auth_handlers.go backend/tests/file_binding_helpers_test.go backend/tests/file_binding_security_test.go backend/tests/integration_flow_test.go backend/tests/restricted_and_security_test.go backend/tests/admin_security_test.go
make test
git add backend/internal/dto/dto.go backend/internal/app/auth_handlers.go backend/tests/file_binding_helpers_test.go backend/tests/file_binding_security_test.go backend/tests/integration_flow_test.go backend/tests/restricted_and_security_test.go backend/tests/admin_security_test.go
git commit -m "fix(auth): claim registration license atomically"
```

---

### Task 5: Validate Reapplication Licenses

**Files:**
- Modify: `backend/internal/app/merchant_handlers.go`
- Modify: `backend/tests/file_binding_helpers_test.go`
- Modify: `backend/tests/file_binding_security_test.go`

**Interfaces:**
- Consumes: `validateMerchantFilesForBinding(tx, merchantID, fileIDs, model.FileBizMerchantLicense)`.
- Produces: reapply rejects foreign/type/status-invalid license IDs without changing merchant or audit state.

- [ ] **Step 1: Add a helper for an authenticated PASS file**

The helper may use presign then update only the upload completion fields to keep unrelated tests fast:

```go
func createReadyOwnedFile(t *testing.T, srv *app.Server, ownerMerchantID uint64, bizType string) model.FileRecord {
	t.Helper()
	file := model.FileRecord{
		BizType: bizType, ObjectKey: fmt.Sprintf("test/%d-%d.jpg", ownerMerchantID, time.Now().UnixNano()),
		URL: "/uploads/test.jpg", MimeType: "image/jpeg", SizeBytes: 22,
		UploaderType: model.UserTypeMerchant, ScanStatus: model.FileScanPass,
		OwnerMerchantID: &ownerMerchantID,
	}
	if err := srv.DB.Create(&file).Error; err != nil { t.Fatal(err) }
	return file
}
```

- [ ] **Step 2: Write a failing reapply rollback test**

Create two rejected merchants, create an owned `MERCHANT_LICENSE` for merchant B, then submit it with merchant A's onboarding token. Assert response code `10012`, merchant A retains its original `license_file_id` and `REJECTED` status, and no `REAPPLY` audit row was added. Add wrong type, `PENDING`, and valid own-file cases.

- [ ] **Step 3: Run the focused test and observe failure**

```bash
cd backend && GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./tests -run 'TestMerchantReapplyValidatesLicenseBinding' -count=1 -v
```

Expected: FAIL because the current handler directly assigns the supplied ID.

- [ ] **Step 4: Validate before mutating merchant state**

Inside the existing reapply transaction, immediately after loading the merchant and checking the state transition:

```go
if req.LicenseFileID != nil {
	if err := validateMerchantFilesForBinding(tx, actor.MerchantID, []uint64{*req.LicenseFileID}, model.FileBizMerchantLicense); err != nil {
		return err
	}
}
```

Keep the existing assignment after this check. Do not accept `license_file_token` on reapply; onboarding presign already sets ownership from the bearer actor.

- [ ] **Step 5: Verify and commit**

```bash
gofmt -w backend/internal/app/merchant_handlers.go backend/tests/file_binding_helpers_test.go backend/tests/file_binding_security_test.go
make test
git add backend/internal/app/merchant_handlers.go backend/tests/file_binding_helpers_test.go backend/tests/file_binding_security_test.go
git commit -m "fix(merchant): validate reapplication license ownership"
```

---

### Task 6: Validate Product Images and Preserve Existing Relations on Failure

**Files:**
- Modify: `backend/internal/app/product_handlers.go`
- Modify: `backend/tests/file_binding_helpers_test.go`
- Modify: `backend/tests/restricted_and_security_test.go`
- Modify: `backend/tests/file_binding_security_test.go`

**Interfaces:**
- Consumes: `validateMerchantFilesForBinding(tx, actor.MerchantID, ids, model.FileBizProductImage)`.
- Produces: create/update accepts only unique, owned, `PASS`, non-empty-URL product images.

- [ ] **Step 1: Update general product test fixtures**

Add an authenticated helper that uses the production presign ownership path, then marks the test fixture uploaded without parsing JWTs:

```go
func createReadyOwnedFileForToken(t *testing.T, srv *app.Server, merchantToken, bizType string) model.FileRecord {
	t.Helper()
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": bizType, "file_name": "owned.jpg", "file_size": 22, "mime_type": "image/jpeg",
	}, map[string]string{"Authorization": "Bearer " + merchantToken})
	if presign.Code != common.CodeOK { t.Fatalf("presign owned file: %+v", presign) }
	fileID := numToUint64(presign.Data["file_id"])
	if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Updates(map[string]interface{}{
		"url": "/uploads/test-owned.jpg", "scan_status": model.FileScanPass,
	}).Error; err != nil { t.Fatal(err) }
	var file model.FileRecord
	if err := srv.DB.First(&file, fileID).Error; err != nil { t.Fatal(err) }
	return file
}
```

Change `productImageAndCategory` to call `createReadyOwnedFileForToken(t, srv, merchantToken, model.FileBizProductImage)`. Do not add a token parser or weaken production scan validation for tests.

- [ ] **Step 2: Write failing create and update security tests**

Add cases for foreign owner, `MERCHANT_LICENSE`, `PENDING`, `BLOCKED`, empty URL, missing ID, duplicate IDs, and a mixed list where one of two files is foreign. All return `10012` and create no product/image/operation-log rows.

For update, start with two valid images and submit one foreign replacement:

```go
before := loadProductImageIDs(t, srv.DB, productID)
resp := requestJSON(t, srv.Router, http.MethodPut, fmt.Sprintf("/api/v1/merchant/products/%d", productID), map[string]interface{}{
	"image_file_ids": []uint64{foreign.ID},
}, authHeader(merchantToken))
if resp.Code != common.CodeInvalidFileBinding { t.Fatalf("update: %+v", resp) }
after := loadProductImageIDs(t, srv.DB, productID)
if !reflect.DeepEqual(after, before) { t.Fatalf("images changed: before=%v after=%v", before, after) }
```

Also assert `cover_file_id` is unchanged.

- [ ] **Step 3: Run focused tests and observe failure**

```bash
cd backend && GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./tests -run 'TestProductCreateValidatesImageBindings|TestProductUpdatePreservesImagesWhenBindingFails' -count=1 -v
```

Expected: FAIL because create/update currently writes arbitrary file IDs.

- [ ] **Step 4: Validate inside create transaction**

Move file validation into the existing create transaction before `tx.Create(&product)`:

```go
if err := s.DB.Transaction(func(tx *gorm.DB) error {
	if err := validateMerchantFilesForBinding(tx, actor.MerchantID, req.ImageFileIDs, model.FileBizProductImage); err != nil {
		return err
	}
	if err := tx.Create(&product).Error; err != nil { return err }
	// existing ProductImage and operation-log writes remain unchanged
	return nil
}); err != nil {
	common.Fail(c, err)
	return
}
```

Do not collapse business errors to `ErrInternal`.

- [ ] **Step 5: Validate update before deleting old relations**

Inside the update transaction and only when `req.ImageFileIDs != nil`, call the validator after the stateflow length checks but before changing `CoverFileID` or deleting `ProductImage` rows. Preserve every stock, reserved-stock, price, status, and version check exactly as-is.

- [ ] **Step 6: Run backend regression and commit**

```bash
gofmt -w backend/internal/app/product_handlers.go backend/tests/restricted_and_security_test.go backend/tests/file_binding_security_test.go
make test
git add backend/internal/app/product_handlers.go backend/tests/restricted_and_security_test.go backend/tests/file_binding_security_test.go
git commit -m "fix(products): enforce image binding ownership"
```

Expected: all F-02 tests and existing multi-stock tests pass without edits to order/inventory code.

---

### Task 7: Carry the Anonymous Capability Through the Frontend Registration Flow

**Files:**
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/constants/error-codes.ts`
- Modify: `frontend/src/pages/auth/RegisterPage.tsx`
- Create: `frontend/src/pages/auth/RegisterPage.test.tsx`

**Interfaces:**
- Consumes: presign `data.file_token`, multipart `file_token`, registration `license_file_token`, code `10012`.
- Produces: in-memory `{fileID, fileToken, fileName}` registration license state and no persistent token storage.

- [ ] **Step 1: Write the failing page test**

Mock `api.presign`, `api.uploadFile`, and `api.register`, select a `File`, fill the form, and submit:

```tsx
it('submits the one-time license token without persisting it', async () => {
  mockPresign.mockResolvedValue({ data: { data: { file_id: 42, object_key: 'merchant_license/f.jpg', file_token: 'raw-capability' } } })
  mockUploadFile.mockResolvedValue({ data: { data: { file_id: 42, status: 'PASS' } } })
  mockRegister.mockResolvedValue({ data: { data: { merchant_id: 9 } } })

  const { container } = render(<MemoryRouter><RegisterPage /></MemoryRouter>)
  const input = container.querySelector('input[type="file"]') as HTMLInputElement
  fireEvent.change(input, { target: { files: [new File(['jpeg'], 'license.jpg', { type: 'image/jpeg' })] } })
  await screen.findByText(/file_id: 42/)
  fireEvent.change(screen.getByLabelText('商家名称'), { target: { value: 'Claim Store' } })
  fireEvent.change(screen.getByLabelText('联系人姓名'), { target: { value: 'Owner' } })
  fireEvent.change(screen.getByLabelText('联系电话'), { target: { value: '13800138000' } })
  fireEvent.change(screen.getByLabelText('登录账号'), { target: { value: 'claim_owner' } })
  fireEvent.change(screen.getByLabelText('登录密码'), { target: { value: 'Passw0rd!2026' } })
  fireEvent.click(screen.getByRole('button', { name: '提交注册' }))

  await waitFor(() => expect(mockRegister).toHaveBeenCalledWith(expect.objectContaining({
    license_file_id: 42,
    license_file_token: 'raw-capability'
  })))
  expect(mockUploadFile.mock.calls[0][0].get('file_token')).toBe('raw-capability')
  expect(localStorage.getItem('file_token')).toBeNull()
})
```

Add a second test where a replacement upload fails after a prior successful upload. The prior complete ID/token/name tuple must remain intact because state changes only after `uploadFile` resolves; never keep a new ID with an old token.

- [ ] **Step 2: Run the page test and observe failure**

```bash
bash -lc 'source ~/.nvm/nvm.sh && nvm use 22.22.2 >/dev/null && cd frontend && npx vitest run src/pages/auth/RegisterPage.test.tsx'
```

Expected: FAIL because `license_file_token` is not tracked or submitted.

- [ ] **Step 3: Update API types and error copy**

Add `license_file_token: string` to `api.register`. Define a typed presign response with optional `file_token?: string`. Add:

```ts
10012: '文件已失效或无权使用，请重新上传'
```

to `ERROR_MESSAGES`.

- [ ] **Step 4: Keep the capability only in component state**

Replace separate license ID/name state with:

```ts
type UploadedLicense = {
  fileID: number
  fileToken: string
  fileName: string
}

const [uploadedLicense, setUploadedLicense] = useState<UploadedLicense | null>(null)
```

After presign, require a non-empty `file_token`, append it to `FormData`, await upload, then atomically set all three fields. On register send both `license_file_id` and `license_file_token`. Do not call localStorage/sessionStorage or place the token in visible text.

- [ ] **Step 5: Run frontend verification and commit**

```bash
bash -lc 'source ~/.nvm/nvm.sh && nvm use 22.22.2 >/dev/null && cd frontend && npm test && npm run build'
git add frontend/src/services/api.ts frontend/src/constants/error-codes.ts frontend/src/pages/auth/RegisterPage.tsx frontend/src/pages/auth/RegisterPage.test.tsx
git commit -m "fix(frontend): submit license capability token"
```

Expected: all frontend tests pass and production build exits 0. Existing Rollup chunk warnings may remain unchanged.

---

### Task 8: Add Isolated MySQL Replay, Update Current Docs, and Run Final Review

**Files:**
- Create: `deploy/acceptance/file-binding-authorization-smoke.sh`
- Modify: `deploy/acceptance/README.md`
- Modify: `Makefile`
- Modify: `backend/tests/file_schema_mysql_test.go`
- Modify: `docs/data-model.md`
- Modify: `docs/backend-api-checklist.md`
- Modify: `docs/release-readiness.md`
- Modify: `docs/full-project-code-review-2026-07-24.md`
- Add after approval: `docs/superpowers/specs/2026-07-26-file-binding-authorization-design.md`
- Add after approval: `docs/superpowers/plans/2026-07-26-file-binding-authorization.md`

**Interfaces:**
- Consumes: full `0001..0006` chain and all Tasks 1-7 API behavior.
- Produces: `acceptance-file-binding-smoke`, sanitized evidence, current F-02 status, and a final verification record that explicitly says production was not touched.

- [ ] **Step 1: Add a failing Makefile/static acceptance assertion**

Extend `backend/migrations/file_binding_migration_test.go` to require the acceptance script contains:

```go
for _, snippet := range []string{
	"FILE_BINDING_ACCEPTANCE_CONFIRM",
	"I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_BINDING_DATA",
	"file_binding_ownership_preflight_passed",
	"file_binding_ownership_postflight_passed",
	"TestFileFlowWithMigrationOnlyMySQL",
} {
	if !strings.Contains(script, snippet) { t.Errorf("acceptance script missing %q", snippet) }
}
```

- [ ] **Step 2: Implement the disposable MySQL matrix**

Create `file-binding-authorization-smoke.sh` using a unique Compose project `secondhand-file-binding-acceptance`. Require both:

```bash
[[ "${FILE_BINDING_ACCEPTANCE_CONFIRM:-}" == "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_BINDING_DATA" ]]
[[ "${ACCEPTANCE_DB_ENGINE:-}" == "mysql8.4" ]]
```

Refuse any pre-existing container, volume, or network with that project label. Apply `0001..0005`, then cover these `0006` states independently:

1. Clean product/license references backfill correct merchant ownership.
2. Orphan file reference fails preflight and leaves schema unchanged.
3. Wrong biz type, non-PASS, empty URL, cross-merchant reuse, and uploader-account mismatch each fail preflight.
4. Unbound PUBLIC remains ownerless; unbound MERCHANT backfills from `merchant_accounts`.
5. Full chain plus `AUTO_MIGRATE=false` runs upload, register claim, and product binding.
6. Two concurrent claims of one token result in exactly one successful merchant.
7. `AUTO_MIGRATE=true` restart creates no duplicate columns/indexes/table.

Each failure fixture must record preflight output and verify `owner_merchant_id` was not added before moving to a fresh schema. Keep resources after success for inspection; do not run `down -v` automatically.

- [ ] **Step 3: Wire the isolated target**

Add to `.PHONY` and Makefile:

```make
acceptance-file-binding-smoke:
	@test "$${FILE_BINDING_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_BINDING_DATA" || { echo "set FILE_BINDING_ACCEPTANCE_CONFIRM for isolated file binding smoke" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	./deploy/acceptance/file-binding-authorization-smoke.sh
```

Document exact environment variables, destructive isolated scope, evidence directory, retained resources, and manual cleanup command in `deploy/acceptance/README.md`.

- [ ] **Step 4: Extend migration-only MySQL API coverage**

Update `TestFileFlowWithMigrationOnlyMySQL` to pass `file_token` through upload/confirm, register with `license_file_token`, assert ownership and token clearing, then create an owned product image and product. Keep `AUTO_MIGRATE=false` for the first server and the existing compatibility restart with `true`.

- [ ] **Step 5: Run local gates before remote isolated replay**

```bash
make test
bash -n deploy/acceptance/file-binding-authorization-smoke.sh
bash -lc 'source ~/.nvm/nvm.sh && nvm use 22.22.2 >/dev/null && cd frontend && npm test && npm run build'
git diff --check
```

Expected: all local tests/builds pass, shell syntax passes, and no whitespace errors exist.

- [ ] **Step 6: Run the retained isolated MySQL 8.4 replay**

Run only against the dedicated acceptance project/server:

```bash
FILE_BINDING_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_BINDING_DATA ACCEPTANCE_DB_ENGINE=mysql8.4 make acceptance-file-binding-smoke
```

Expected: all fail-closed fixtures fail before DDL, clean/full-chain cases pass, concurrent claim count is exactly one, and output ends with `isolated file binding acceptance passed`. Do not run this command against production.

- [ ] **Step 7: Update current documentation without rewriting historical facts**

Document exact columns and API fields in `data-model.md` and `backend-api-checklist.md`. In `release-readiness.md`, state “F-02 code-side closed on branch, pending frontend/backend deployment and `0006` production migration” only after all verification passes. In the full-project review add a dated follow-up note; preserve the original finding text.

Do not edit the three protected untracked review documents. Stage the design and plan only after the user has approved them.

- [ ] **Step 8: Request two-stage code review**

Use `superpowers:requesting-code-review` for spec compliance first, then code quality. Fix any confirmed findings and rerun the affected focused tests plus the full gates.

- [ ] **Step 9: Verify staged scope and commit**

```bash
git add Makefile deploy/acceptance/file-binding-authorization-smoke.sh deploy/acceptance/README.md backend/migrations/file_binding_migration_test.go backend/tests/file_schema_mysql_test.go docs/data-model.md docs/backend-api-checklist.md docs/release-readiness.md docs/full-project-code-review-2026-07-24.md docs/superpowers/specs/2026-07-26-file-binding-authorization-design.md docs/superpowers/plans/2026-07-26-file-binding-authorization.md
git diff --cached --check
git diff --cached --name-status
git commit -m "test(acceptance): verify file binding authorization"
```

Expected: staged scope contains no protected untracked review document, no order/inventory source, and no production credential/evidence secret.

- [ ] **Step 10: Final verification and stop**

```bash
make test
bash -lc 'source ~/.nvm/nvm.sh && nvm use 22.22.2 >/dev/null && cd frontend && npm test && npm run build'
git status --short --branch
git log --oneline -8
```

Report commit hashes, test counts, isolated MySQL evidence paths, retained acceptance resources, and remaining F-04/F-06/F-13 scope. Explicitly report that no production deployment or migration occurred, then stop.
