# F-09 File Record Schema Alignment Code Review

**Date:** 2026-07-26

**Branch:** `codex/reconcile-code-reviews`

**Reviewed baseline:** `252d494` plus the current uncommitted F-09 implementation

**Verdict:** Not merge-ready; no Critical findings, three Important findings and two Minor findings remain

## Scope and constraints

This review covers only the F-09 file metadata table schema alignment work:

- explicit `FileRecord.TableName()` contract;
- migration `0005` preflight, up, and postflight gates;
- migration artifact and opt-in MySQL tests;
- isolated file-schema acceptance harness;
- F-09 status and release documentation.

The repair must keep these constraints:

- Do not change inventory, product stock, order state transitions, or other inventory business paths.
- Do not deploy, migrate, or write to production.
- Run executable MySQL checks only in a dedicated isolated acceptance environment.
- Do not modify, stage, or commit these protected untracked documents:
  - `docs/architecture-evolution-plan-2026-07-24.md`
  - `docs/first-round-fix-review-2026-07-24.md`
  - `docs/second-round-fix-review-2026-07-24.md`
- Do not include the separate F-02 design and plan in the F-09 commit:
  - `docs/superpowers/specs/2026-07-26-file-binding-authorization-design.md`
  - `docs/superpowers/plans/2026-07-26-file-binding-authorization.md`

## Important findings

### F09-R1: Up migration can rename a schema that drifted after preflight

**Location:** `backend/migrations/0005_file_records_table.up.sql:9-19`

**Related requirement:** `docs/superpowers/specs/2026-07-26-file-record-schema-alignment-design.md:40,113-115,174-181`

The preflight script validates required columns and indexes, but the up script
only repeats the `files` / `file_records` existence count. Because preflight and
up are separate MySQL client executions, the legacy table can lose a required
column or index between them. The current up procedure would still rename it,
and only postflight would report the drift after the mutation.

This violates the design requirement that incomplete schemas fail before any
rename and can leave migration `0005` partially applied.

**Required repair:**

1. Repeat the required column, primary-key, `object_key` uniqueness, and leading
   `(biz_type, created_at)` index validation inside the up procedure before
   `RENAME TABLE` or the canonical no-op marker.
2. Keep both-table and neither-table states fail-closed.
3. Do not add a destructive down migration.
4. Add executable isolated MySQL cases that run preflight, deliberately remove
   a required column or index, then prove up fails before rename and preserves
   the original table name.

**Acceptance:** An invalid legacy table remains named `files`; an invalid
canonical table remains named `file_records`; up exits nonzero without emitting
a renamed/no-op success marker.

### F09-R2: Required executable MySQL 8.4 evidence is absent

**Locations:**

- `backend/migrations/file_records_migration_test.go:9`
- `backend/tests/file_schema_mysql_test.go:13`
- `docs/isolated-acceptance-results-2026-07-24.md:161-182`

The local migration test checks only string fragments. It cannot validate
stored-procedure syntax, MySQL behavior, row preservation, or application
startup with `AUTO_MIGRATE=false`. The opt-in file-flow test currently skips
without `FILE_SCHEMA_MYSQL_TEST=1`, and the tracked acceptance record explicitly
says the six-state matrix has not run.

**Required repair and verification:**

Run `make acceptance-file-schema-smoke` on the dedicated isolated MySQL 8.4
host after implementing F09-R1. It must cover:

1. legacy `files`-only rename with the sentinel row unchanged;
2. canonical `file_records`-only verified no-op;
3. both-table preflight failure with both tables and sentinels preserved;
4. neither-table preflight failure without creating a table;
5. complete SQL chain plus file presign/upload/confirm with
   `AUTO_MIGRATE=false`;
6. subsequent `AUTO_MIGRATE=true` startup without creating `files`;
7. the invalid-shape preflight-to-up drift cases added for F09-R1.

Record sanitized commands, result markers, MySQL version, and evidence paths in
`docs/isolated-acceptance-results-2026-07-24.md`. Do not record credentials,
DSNs, tokens, full file URLs, or production data.

F-09 is not merge-ready until this matrix passes. Production `0005` execution
remains separately authorized and is not part of this repair.

### F09-R3: Tracked reviews close F-09 before its documented acceptance gate

**Locations include:**

- `docs/deep-code-review-2026-07-24.md:77,207`
- `docs/full-project-code-review-2026-07-24.md:52,238,441`
- `docs/production-hardening-repair-plan-2026-07-24.md:555`
- `docs/release-readiness.md:91`

The governing design states that F-09 may be marked fixed only after the model
test, isolated MySQL matrix, and `AUTO_MIGRATE=false` file flow all pass. Current
tracked documents use `已修复`, strikethrough, or `代码侧关闭` while the required
matrix is still pending.

**Required repair:**

- Before isolated acceptance passes, use a consistent status such as
  `本地实现完成，隔离 MySQL 验收待执行；生产未执行 0005`.
- Do not strike through F-09 or remove it from the pending acceptance list.
- After the complete isolated matrix passes, update the documents to say
  `本地修复并完成隔离验证`; continue to state that production deployment and
  production migration have not occurred.

## Minor findings

### F09-R4: Canonical no-op does not compare total row counts

**Location:** `deploy/acceptance/file-record-schema-smoke.sh:83-92`

The script compares only the canonical sentinel hash. The implementation plan
also requires the total row count to remain unchanged.

Capture `COUNT(*)` before and after `run_0005`, compare both values, and keep the
sentinel hash assertion and `file_records_migration_noop` marker check.

### F09-R5: Repository diff formatting gate fails

`git diff --check` currently reports trailing whitespace in:

- `docs/full-project-code-review-2026-07-24.md:240`
- `docs/isolated-acceptance-results-2026-07-24.md:147,165-170,181,186-187`
- `docs/release-readiness.md:79-80`

Remove the trailing whitespace without changing unrelated document content.

## Required verification

Run locally without Docker:

```bash
make test

cd frontend
source ~/.nvm/nvm.sh
nvm use 22.22.2
npm test
npm run build

cd ../miniapp
source ~/.nvm/nvm.sh
nvm use 22.22.2
npm test

cd ..
bash -n deploy/acceptance/file-record-schema-smoke.sh
git diff --check
git status --short
```

Run only on the dedicated isolated acceptance host:

```bash
FILE_SCHEMA_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-file-schema-smoke
```

Before handing the work back, verify that:

- all local suites and the frontend production build exit zero;
- the opt-in MySQL file-flow test actually runs rather than skips in acceptance;
- all required migration markers and drift failures appear in fresh evidence;
- `git diff --check` exits zero;
- no inventory or order business-path file changed;
- the three protected documents remain untracked and unchanged;
- no production deployment or migration was performed.

## Review baseline evidence

The review that produced this report observed:

- backend `make test`: passed;
- focused model and migration artifact tests: passed;
- opt-in MySQL file-flow test: skipped without isolated acceptance variables;
- frontend Node `22.22.2`: 6 test files / 8 tests passed;
- frontend production build: passed with existing Rollup chunk warnings;
- miniapp Node `22.22.2`: 11 test files / 17 tests passed;
- `bash -n deploy/acceptance/file-record-schema-smoke.sh`: passed;
- unconfirmed `make acceptance-file-schema-smoke`: correctly failed closed;
- `git diff --check`: failed on the whitespace listed in F09-R5;
- isolated MySQL 8.4 matrix: not executed in this review.
