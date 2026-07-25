# File Record Schema Alignment Design

**Date:** 2026-07-26
**Finding:** F-09 - SQL migration and GORM use different file table names
**Status:** 本地修复并通过隔离 MySQL 8.4 测试服务器审核；生产未执行 0005
**Scope:** File metadata table naming, migration gates, and isolated verification

## Implementation status

| Item | Status |
| --- | --- |
| `FileRecord.TableName()` → `file_records` | Done |
| `0005` preflight / up / postflight | Done (up re-validates columns/indexes before rename/no-op) |
| Local model + migration artifact tests | Done |
| Opt-in MySQL file-flow test | Done (skips without `FILE_SCHEMA_MYSQL_TEST=1`) |
| `make acceptance-file-schema-smoke` harness | Done (row-count no-op + preflight→up drift cases) |
| Isolated MySQL 8.4 matrix | Done on dedicated acceptance host; MySQL 8.4.8, exit 0 |
| Production `0005` execution | Not run; requires separate authorization |

## Context

Before this change, the repository had two schema contracts for the same model:

- `backend/migrations/0001_init.up.sql` creates `files`.
- `backend/internal/model/models.go` defined `FileRecord` without an explicit
  `TableName()`, so GORM resolved it to `file_records`.
- Production already contained `file_records`, not `files`, because it had
  run with `AUTO_MIGRATE=true`.

That split made a migration-only database incompatible with the application when
`AUTO_MIGRATE=false`. The file API then queried `file_records`, while the SQL
migration created only `files`.

## Goals

- Make `file_records` the single canonical table name used by GORM and the
  complete SQL migration chain.
- Preserve all rows when upgrading a legacy migration-only database that has
  `files` but not `file_records`.
- Make ambiguous or incomplete schema states fail before any rename.
- Prove that a database built through SQL migrations works with
  `AUTO_MIGRATE=false` and the file upload flow.
- Keep this work independent of inventory and order behavior.

## Non-goals

- No production deployment or migration execution.
- No changes to file ownership, license privacy, upload quotas, object storage,
  or cleanup policy; those remain F-02, F-06, and F-13 work.
- No column, data-type, or index redesign beyond checks required to identify
  the existing file metadata table safely.
- No rewrite of migration `0001`; it remains an immutable historical baseline.
- No broad replacement of GORM `AutoMigrate`. This change only prevents the
  file table name from depending on GORM pluralization.
- No modification or staging of the three protected untracked review
  documents.

## Decision

`file_records` is canonical because it is already the application runtime
contract and the production table name. Changing the model to `files` would
require an unnecessary production rename and would increase rollback risk.

The model will explicitly return `file_records` from `FileRecord.TableName()`.
The migration chain will retain `0001` and add a gated `0005` compatibility
migration. The migration renames `files` only when that is the sole matching
table and otherwise either performs a verified no-op or fails closed.

## Schema State Machine

| `files` | `file_records` | Preflight result | Up migration |
| --- | --- | --- | --- |
| present | absent | Pass after shape checks | Atomically rename to `file_records` |
| absent | present | Pass after shape checks | Verified no-op |
| present | present | Fail | No change |
| absent | absent | Fail | No change |

The migration must not merge, copy, truncate, or drop tables. Two tables could
contain different business data, so their coexistence requires manual
investigation outside this implementation.

## Components

### Explicit GORM contract

Add the following method beside `FileRecord`:

```go
func (FileRecord) TableName() string {
	return "file_records"
}
```

This preserves current runtime behavior while making it independent of model
renames or changes in GORM naming rules.

### Migration artifacts

Add three MySQL migration-gate files:

- `backend/migrations/0005_file_records_table.preflight.sql`
- `backend/migrations/0005_file_records_table.up.sql`
- `backend/migrations/0005_file_records_table.postflight.sql`

Preflight checks `information_schema` and stops unless exactly one candidate
table exists. It also verifies the columns required by the current
`FileRecord` model before allowing a rename or no-op. The required columns are
`id`, `biz_type`, `object_key`, `url`, `mime_type`, `size_bytes`,
`uploader_type`, `uploader_id`, `scan_status`, and `created_at`. The required
keys are the primary key on `id`, uniqueness of `object_key`, and a composite
index whose leading columns are `biz_type, created_at`.

The up migration repeats the existence check and the required column, primary
key, `object_key` uniqueness, and leading `(biz_type, created_at)` index
validation before any rename or canonical no-op marker. This closes
preflight-to-up drift: an incomplete schema fails inside up without emitting a
success marker and without renaming the table. It uses
`RENAME TABLE files TO file_records` only for the legacy state. MySQL performs
that single-table rename atomically. If `file_records` already exists alone and
still matches the required shape, the migration returns a no-op marker. The
stable output markers are `file_records_preflight_passed`,
`file_records_migration_renamed` or `file_records_migration_noop`, and
`file_records_postflight_passed`.

Postflight requires `file_records` to exist, `files` to be absent, and the
required columns and key constraints to remain available. It emits a stable
success marker that the acceptance script can require.

There is intentionally no destructive down migration. Both the current and
the corrected application versions use `file_records`; renaming it back would
recreate F-09 and break application rollback. Operational rollback keeps the
canonical table and rolls back only application artifacts.

### Documentation contract

Update the tracked data-model and release documentation to name
`file_records`, explain that the full SQL migration chain is authoritative,
and record that `0005` must be executed only through its preflight/up/postflight
sequence.

F-09 is marked fixed only after the model test, isolated MySQL matrix, and
`AUTO_MIGRATE=false` file-flow smoke all pass. Documentation must not describe
the migration as deployed to production.

## Isolated Verification

Local verification must not require Docker:

- A Go unit test asserts the explicit `FileRecord.TableName()` contract.
- Repository checks validate that the migration gates and documented sequence
  are present and consistently named.
- The normal backend test suite continues to run with repository-local Go
  caches.

Executable MySQL verification runs only in the retained isolated acceptance
environment. It uses disposable schemas or an isolated volume and covers:

1. Legacy state: apply the old baseline so only `files` exists, insert a
   sentinel file row, run `0005`, and verify the same row and ID exist in
   `file_records`.
2. Canonical state: prepare only `file_records`, run `0005`, and verify a
   no-op with unchanged row count and sentinel data.
3. Ambiguous state: create both tables and verify preflight fails without
   modifying either table.
4. Missing state: create neither table and verify preflight fails without
   creating a table.
5. Clean full-chain state: apply all SQL migrations through `0005`, start the
   API with `AUTO_MIGRATE=false`, and complete a file presign/upload/confirm
   smoke against `file_records`.
6. Compatibility state: after successful SQL migration, start once with
   `AUTO_MIGRATE=true` and verify GORM does not create `files` or a second file
   metadata table.

The acceptance process must require an explicit isolated-environment
confirmation variable. It must not accept a production host, production DSN,
or shared production database name.

## Failure Handling

- Any preflight mismatch stops before the up migration.
- Any up-migration state other than the two approved states raises a MySQL
  error and makes no table change.
- Any missing column or key stops before the API is started.
- If postflight or the file-flow smoke fails, keep the isolated environment for
  inspection and do not describe F-09 as complete.
- Production execution remains a separately authorized operation even though
  the known production state would take the verified no-op branch.

## Acceptance Criteria

- `FileRecord.TableName()` explicitly returns `file_records`.
- The complete migration chain ends with exactly one file metadata table named
  `file_records`.
- A legacy `files` row survives the compatibility rename with the same primary
  key and field values.
- Both-table and neither-table states fail before mutation.
- The file API works against a clean MySQL schema with
  `AUTO_MIGRATE=false`.
- Enabling `AUTO_MIGRATE=true` after migration creates no competing file table.
- Backend tests and migration contract checks pass locally without Docker.
- The isolated MySQL matrix passes without accessing or modifying production.
- Inventory and order business paths are unchanged.
- The three protected untracked review documents remain unmodified and
  unstaged.
