# F-09 File Record Schema Alignment Follow-up Review

**Date:** 2026-07-26

**Branch:** `codex/reconcile-code-reviews`

**Review basis:** Current uncommitted F-09 implementation after the first review fixes

**Verdict:** Not merge-ready; two Important and two Minor findings remain

## Scope and safety boundaries

This is the second review round for F-09. Repair only the findings in this
document and preserve the existing F-09 implementation that already passed
local review.

- Do not change inventory, product stock, order state transitions, or any other
  inventory/order business path.
- Do not deploy, migrate, or write to production.
- Execute MySQL tests only in the dedicated isolated acceptance environment.
- Do not modify, stage, or commit these protected untracked documents:
  - `docs/architecture-evolution-plan-2026-07-24.md`
  - `docs/first-round-fix-review-2026-07-24.md`
  - `docs/second-round-fix-review-2026-07-24.md`
- Do not include these separate F-02 documents in the F-09 commit:
  - `docs/superpowers/specs/2026-07-26-file-binding-authorization-design.md`
  - `docs/superpowers/plans/2026-07-26-file-binding-authorization.md`
- Do not mark F-09 fixed until the complete isolated MySQL matrix passes.

## Important findings

### F09-FU1: Canonical drift state prevents the clean full-chain test

**Locations:**

- `deploy/acceptance/file-record-schema-smoke.sh:136-167`
- `backend/migrations/0001_init.down.sql:4`

The canonical preflight-to-up drift case intentionally finishes with exactly
one `file_records` table present. The next phase runs
`0001_init.down.sql`, but that historical down script deletes only `files` and
does not delete `file_records`. Applying `0001_init.up.sql` then creates a new
`files` table beside the retained `file_records` table. The following `0005`
preflight sees both tables and fails, so the harness cannot reach the
`AUTO_MIGRATE=false` file flow or the compatibility startup.

**Required repair:**

1. Call the existing `reset_file_tables` helper immediately before rebuilding
   the clean full migration chain.
2. Keep historical migration `0001` immutable.
3. Keep the canonical drift assertions proving that the failed up operation
   itself preserves `file_records`; cleanup must occur only after those
   assertions.
4. Add or strengthen a lightweight artifact/script contract test if practical
   so the clean-chain phase cannot begin while a drift test table remains.

**Acceptance:** Immediately before applying `0001_init.up.sql` for the clean
chain, neither `files` nor `file_records` exists. The subsequent `0005`
preflight observes only `files`, the rename succeeds, and the file-flow test is
reached.

### F09-FU2: Required isolated MySQL 8.4 evidence is still missing

**Locations:**

- `docs/isolated-acceptance-results-2026-07-24.md:166-198`
- `backend/tests/file_schema_mysql_test.go:13`

The current document correctly says the matrix is pending. Local execution
still skips `TestFileFlowWithMigrationOnlyMySQL`, and there are no retained F-09
evidence files in the working checkout. Static Go tests and `bash -n` do not
prove MySQL stored-procedure behavior or migration-only application startup.

After fixing F09-FU1, run the complete isolated acceptance command on the
dedicated MySQL 8.4 host:

```bash
FILE_SCHEMA_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-file-schema-smoke
```

The run must prove all eight states currently listed in
`docs/isolated-acceptance-results-2026-07-24.md`, including both drift cases,
the full SQL chain, a non-skipped `AUTO_MIGRATE=false` file flow, and the
subsequent `AUTO_MIGRATE=true` compatibility startup.

Record only sanitized evidence:

- exact command and exit code;
- MySQL `VERSION()`;
- stable preflight, rename/no-op, postflight, and Go test pass markers;
- evidence directory and filenames;
- drift failures with no success marker and unchanged table names.

Do not record credentials, DSNs, tokens, full file URLs, uploaded file content,
or production data. Do not execute `0005` against production.

Only after the complete isolated run exits zero may tracked status documents be
updated from `本地实现完成，隔离 MySQL 验收待执行` to
`本地修复并完成隔离验证`. They must continue to say production was not deployed
and production `0005` was not executed.

## Minor findings

### F09-FU3: Design context still describes the pre-fix repository as current

**Location:** `docs/superpowers/specs/2026-07-26-file-record-schema-alignment-design.md:22-32`

The design status says the implementation is complete locally, but its Context
section says the repository "currently" defines `FileRecord` without an
explicit `TableName()`. That is no longer true.

Change the wording to make this historical context, for example:

```text
Before this change, the repository had two schema contracts for the same model:
```

Keep the original evidence and explanation; only correct its time framing.

### F09-FU4: Tracked release document links to a protected untracked document

**Location:** `docs/release-readiness.md:80`

The tracked release document links to
`docs/second-round-fix-review-2026-07-24.md`. That file is one of the protected
untracked documents and must not be included in the F-09 commit, so the link
would be broken after commit and is unrelated to this repair scope.

Remove that line. Do not solve the broken link by staging or committing the
protected document.

## Already resolved findings to preserve

The following first-round findings are resolved in the current working tree and
must not regress:

- up migration repeats required column, primary-key, `object_key` uniqueness,
  and `(biz_type, created_at)` index checks before rename/no-op;
- invalid-shape acceptance cases require up failure before mutation/success;
- F-09 status remains pending rather than fixed before isolated acceptance;
- canonical no-op compares both sentinel content and total row count;
- `git diff --check` is clean.

## Required verification

Run locally after the repair:

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

Then run the isolated MySQL command from F09-FU2. Before handing the work back,
verify all of the following:

- backend, frontend, and miniapp tests exit zero;
- frontend production build exits zero;
- `git diff --check` exits zero;
- the isolated MySQL 8.4 matrix exits zero;
- `TestFileFlowWithMigrationOnlyMySQL` runs and passes rather than skips;
- the retained evidence contains all required markers and drift failures;
- no inventory or order business-path file changed;
- the three protected documents remain untracked and unstaged;
- the F-02 design and plan remain outside the F-09 change set;
- no production deployment or migration occurred.

## Follow-up review baseline

This review observed:

- backend tests passed locally;
- focused model and migration artifact tests passed;
- the opt-in MySQL file-flow test skipped locally;
- frontend Node `22.22.2`: 6 test files / 8 tests passed;
- frontend production build passed with existing Rollup warnings;
- miniapp Node `22.22.2`: 11 test files / 17 tests passed;
- `bash -n deploy/acceptance/file-record-schema-smoke.sh` passed;
- `git diff --check` passed;
- isolated execution was unavailable because the local acceptance `.env` was
  absent;
- no production operation was performed.
