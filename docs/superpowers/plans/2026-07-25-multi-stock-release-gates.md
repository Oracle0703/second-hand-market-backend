# Multi-Stock Production Release Gates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Promote the already-proven migration checks into the authoritative migration area, add a repeatable isolated MySQL smoke entry point, and prepare an exact production release gate without changing the validated inventory/order behavior.

**Architecture:** `backend/migrations/` becomes the single source for migration 0004 preflight and postflight SQL. The acceptance stack invokes those same files, while its environment-specific `yaner` fingerprint remains an acceptance/release protection rather than a generic schema requirement. Makefile and release documentation expose only an explicitly isolated smoke entry; production deployment remains a separate approval-gated operation.

**Tech Stack:** MySQL 8.4 SQL, Docker Compose v2, GNU Make, Node.js 22, Go 1.22, existing Gin/GORM backend and Vitest projects.

## Global Constraints

- Continue on branch `codex/reconcile-code-reviews`; expected starting HEAD is `51cd7ed`.
- Do not modify `backend/internal/**`, `frontend/src/**`, `miniapp/src/**`, or the 0004 up/down migration semantics.
- Do not add a `LOCKED` application transition and do not automatically rewrite any `LOCKED` row. A nonzero count must stop the release for row-level review.
- Do not alter, disable, rename, reset, or use `yaner` for test orders. Preserve the before/after fingerprint workflow.
- Do not run `scripts/smoke-mysql-concurrency.mjs` against production. It intentionally writes durable test rows.
- Do not claim `frontend npm test` is green. The Ant Design/Vitest hang remains a separate investigation.
- Leave the pre-existing untracked review documents untouched unless the user separately requests their inclusion:
  - `docs/architecture-evolution-plan-2026-07-24.md`
  - `docs/first-round-fix-review-2026-07-24.md`
  - `docs/second-round-fix-review-2026-07-24.md`
- Do not push, merge, deploy, migrate production, rotate production credentials, or delete retained acceptance evidence without explicit user approval.
- Production rollback must be forward-fix once new multi-stock orders exist; never restore `uk_product_active` after multiple active or historical rows have been created for one product.

---

## File Map

**Create**

- `backend/migrations/0004_merchant_multi_stock.preflight.sql`: generic fail-fast checks that must pass before the 0004 up migration.
- `backend/migrations/0004_merchant_multi_stock.postflight.sql`: generic schema and data checks that must pass after the 0004 up migration and before the new API starts.

**Modify**

- `deploy/acceptance/README.md`: invoke the canonical migration companion SQL instead of acceptance-only copies.
- `deploy/acceptance/sql/protected-fingerprint.sql`: fail if the protected `yaner` account is absent or duplicated before printing stable fingerprints.
- `Makefile`: add a discoverable, explicitly isolated MySQL concurrency smoke target.
- `README.md`: document the isolated smoke target and link the release gates.
- `docs/release-readiness.md`: replace stale MySQL/frontend test status and reference the canonical gates.
- `docs/production-hardening-repair-plan-2026-07-24.md`: record isolated acceptance completion, exact gate paths, and production stop conditions.

**Delete after references are switched**

- `deploy/acceptance/sql/preflight.sql`
- `deploy/acceptance/sql/post-migration.sql`

**Do not modify**

- `backend/migrations/0004_merchant_multi_stock.up.sql`
- `backend/migrations/0004_merchant_multi_stock.down.sql`
- `deploy/acceptance/sql/post-smoke.sql`
- The validated order/product handlers and their frontend pages.

---

### Task 1: Establish the Baseline and Protect Existing Work

**Files:**
- Read: repository status, current branch, recent commits
- Preserve: the three untracked review documents listed in Global Constraints

**Interfaces:**
- Consumes: committed acceptance baseline `51cd7ed`
- Produces: a recorded clean baseline for the release-tooling-only diff

- [ ] **Step 1: Confirm branch and commit**

Run:

```bash
git branch --show-current
git log -3 --oneline --decorate
```

Expected: branch is `codex/reconcile-code-reviews`; HEAD is `51cd7ed`. If either differs, stop and reconcile before editing.

- [ ] **Step 2: Record the dirty-worktree boundary**

Run:

```bash
git status --short
```

Expected before this plan is implemented: only the three review documents and this plan may be untracked. Do not stage the review documents.

- [ ] **Step 3: Reconfirm that runtime business code is frozen**

Run:

```bash
git diff 51cd7ed -- backend/internal frontend/src miniapp/src backend/migrations/0004_merchant_multi_stock.up.sql backend/migrations/0004_merchant_multi_stock.down.sql
```

Expected: no output.

---

### Task 2: Promote Generic Preflight and Postflight SQL

**Files:**
- Create: `backend/migrations/0004_merchant_multi_stock.preflight.sql`
- Create: `backend/migrations/0004_merchant_multi_stock.postflight.sql`
- Source for reviewed assertions: `deploy/acceptance/sql/preflight.sql`
- Source for reviewed assertions: `deploy/acceptance/sql/post-migration.sql`

**Interfaces:**
- Consumes: pre-0004 schema for preflight and post-0004 schema for postflight
- Produces: fail-fast MySQL scripts returning a nonzero client exit status through `SIGNAL SQLSTATE '45000'`

- [ ] **Step 1: Prove the canonical files do not yet exist**

Run:

```bash
test ! -e backend/migrations/0004_merchant_multi_stock.preflight.sql
test ! -e backend/migrations/0004_merchant_multi_stock.postflight.sql
```

Expected: both commands exit 0 at the starting commit.

- [ ] **Step 2: Create the canonical preflight**

Create `backend/migrations/0004_merchant_multi_stock.preflight.sql` by copying the reviewed `deploy/acceptance/sql/preflight.sql` and applying these exact structural edits:

- Rename `acceptance_preflight` to `merchant_multi_stock_preflight` in the `DROP PROCEDURE`, `CREATE PROCEDURE`, `CALL`, and final `DROP PROCEDURE` statements.
- Change the required-table query to require only `products` and `orders`, with expected count `2`.
- Remove the `merchant_accounts` / `yaner` count block.
- Remove the final account/password-hash output query.
- Keep `DELIMITER`, `SIGNAL SQLSTATE '45000'`, and cleanup behavior unchanged.
- Change the final marker alias value from `preflight_passed` under `acceptance_gate` to `preflight_passed` under `migration_gate`.

The procedure must fail unless all of these are true:

- `products` and `orders` exist in `DATABASE()`.
- Neither `products.reserved_stock` nor `orders.quantity` exists yet.
- `orders.is_active = 1` count is zero.
- No product has negative stock.
- No duplicate `order_no` exists.
- No order references a missing product.
- No product has `status = 'LOCKED'`.
- Unique index `uk_product_active(product_id,is_active)` exists with exactly that column order and is unique.

The generic migration script must not reference `merchant_accounts`, `yaner`, passwords, hashes, or production-specific row IDs. Its final output may include only aggregate product/order/status counts, the MySQL version, and a `preflight_passed` marker.

- [ ] **Step 3: Create the canonical postflight**

Create `backend/migrations/0004_merchant_multi_stock.postflight.sql` from the reviewed assertions in `deploy/acceptance/sql/post-migration.sql`, using procedure name `merchant_multi_stock_postflight` and a `postflight_passed` marker.

It must fail unless all of these are true:

- `products.reserved_stock` is signed `INT NOT NULL DEFAULT 0`; check `column_type = 'int'`, not only `data_type = 'int'`.
- `orders.quantity` is signed `INT NOT NULL DEFAULT 1`; check `column_type = 'int'`.
- Every historical order has `quantity = 1` immediately after migration.
- Every product has `reserved_stock = 0`, not null, and not greater than stock.
- Active order count is zero and all `active_order_id` values are null.
- `LOCKED` product count is zero.
- `uk_product_active` is absent.
- Non-unique `idx_order_product_active(product_id,is_active)` exists in exactly that order.
- All three named CHECK constraints exist, are enforced, and have the reviewed expressions.

The script must remain generic and output only aggregate counts plus the pass marker.

- [ ] **Step 4: Run static safety checks**

Run:

```bash
rg -n "SIGNAL SQLSTATE|is_active = 1|status = 'LOCKED'|uk_product_active|idx_order_product_active|column_type = 'int'" backend/migrations/0004_merchant_multi_stock.preflight.sql backend/migrations/0004_merchant_multi_stock.postflight.sql
rg -n "yaner|password|merchant_accounts" backend/migrations/0004_merchant_multi_stock.preflight.sql backend/migrations/0004_merchant_multi_stock.postflight.sql
```

Expected: the first command finds every required gate; the second command returns exit 1 with no matches.

---

### Task 3: Make Acceptance Use the Canonical SQL Without Weakening `yaner` Protection

**Files:**
- Modify: `deploy/acceptance/README.md`
- Modify: `deploy/acceptance/sql/protected-fingerprint.sql`
- Delete: `deploy/acceptance/sql/preflight.sql`
- Delete: `deploy/acceptance/sql/post-migration.sql`

**Interfaces:**
- Consumes: `/acceptance/migrations/0004_merchant_multi_stock.preflight.sql` and `.postflight.sql`, already mounted read-only by Compose
- Produces: one migration gate implementation shared by acceptance and production release procedures

- [ ] **Step 1: Add an explicit protected-account guard**

At the start of `deploy/acceptance/sql/protected-fingerprint.sql`, add a procedure that counts active `merchant_accounts` rows with `username = 'yaner'` and signals unless the count is exactly one:

```sql
DROP PROCEDURE IF EXISTS acceptance_protected_account_guard;

DELIMITER //
CREATE PROCEDURE acceptance_protected_account_guard()
BEGIN
  DECLARE protected_accounts BIGINT DEFAULT 0;
  SELECT COUNT(*) INTO protected_accounts
  FROM merchant_accounts
  WHERE username = 'yaner' AND deleted_at IS NULL;
  IF protected_accounts <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'protected fingerprint: yaner account is absent or duplicated';
  END IF;
END//
DELIMITER ;

CALL acceptance_protected_account_guard();
DROP PROCEDURE acceptance_protected_account_guard;
```

Keep both existing stable fingerprint queries unchanged.

- [ ] **Step 2: Switch acceptance commands to canonical paths**

In `deploy/acceptance/README.md`, replace the two SQL inputs as follows:

```text
/acceptance/sql/preflight.sql
-> /acceptance/migrations/0004_merchant_multi_stock.preflight.sql

/acceptance/sql/post-migration.sql
-> /acceptance/migrations/0004_merchant_multi_stock.postflight.sql
```

Keep evidence filenames (`preflight.txt`, `post-migration.txt`) unchanged so existing evidence references remain understandable.

- [ ] **Step 3: Remove the duplicated acceptance gate files**

Delete only:

```text
deploy/acceptance/sql/preflight.sql
deploy/acceptance/sql/post-migration.sql
```

Do not delete `post-smoke.sql` or `protected-fingerprint.sql`.

- [ ] **Step 4: Verify no stale path remains**

Run:

```bash
rg -n "/acceptance/sql/(preflight|post-migration)\.sql|deploy/acceptance/sql/(preflight|post-migration)\.sql" .
```

Expected: no stale executable reference. Historical review prose may mention the old location; do not rewrite untracked reviewer-authored documents.

---

### Task 4: Add a Safe Makefile Entry for the MySQL Smoke

**Files:**
- Modify: `Makefile`
- Modify: `README.md`

**Interfaces:**
- Consumes: the existing environment contract enforced by `scripts/smoke-mysql-concurrency.mjs`
- Produces: `make acceptance-mysql-smoke`, which remains unusable without explicit isolation confirmation

- [ ] **Step 1: Prove the target is absent**

Run:

```bash
make -n acceptance-mysql-smoke
```

Expected: Make exits nonzero with `No rule to make target`.

- [ ] **Step 2: Add the target without loading or inventing credentials**

Extend `.PHONY` and add exactly this target shape:

```make
.PHONY: test backend-test backend-run frontend-dev acceptance-mysql-smoke

acceptance-mysql-smoke:
	@test "$${ACCEPTANCE_CONFIRM_ISOLATED:-}" = "I_UNDERSTAND_THIS_WRITES_TEST_DATA" || { echo "set ACCEPTANCE_CONFIRM_ISOLATED for the isolated destructive smoke" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	node scripts/smoke-mysql-concurrency.mjs
```

Do not read a password file in Make, set a default administrator password, or provide a production URL. The caller must inject `SMOKE_ADMIN_USERNAME`, `SMOKE_ADMIN_PASSWORD`, and loopback `API_BASE_URL`.

- [ ] **Step 3: Document the exact isolated invocation**

Add this command near `README.md`'s release regression commands, explicitly stating that it writes durable test rows and must never target production:

```bash
ACCEPTANCE_DB_ENGINE=mysql8.4 \
ACCEPTANCE_CONFIRM_ISOLATED=I_UNDERSTAND_THIS_WRITES_TEST_DATA \
SMOKE_ADMIN_USERNAME="$SMOKE_ADMIN_USERNAME" \
SMOKE_ADMIN_PASSWORD="$SMOKE_ADMIN_PASSWORD" \
API_BASE_URL=http://127.0.0.1:18082/api/v1 \
make acceptance-mysql-smoke
```

- [ ] **Step 4: Verify both safety barriers**

Run without confirmation:

```bash
make acceptance-mysql-smoke
```

Expected: exits before Node starts and prints the confirmation requirement.

Then prove the Node script still rejects a non-loopback target before authentication:

```bash
ACCEPTANCE_DB_ENGINE=mysql8.4 \
ACCEPTANCE_CONFIRM_ISOLATED=I_UNDERSTAND_THIS_WRITES_TEST_DATA \
SMOKE_ADMIN_USERNAME=never_used \
SMOKE_ADMIN_PASSWORD=never_used \
API_BASE_URL=https://market.meaningful.ink/api/v1 \
make acceptance-mysql-smoke
```

Expected: exits nonzero with the loopback-only error and makes no request.

---

### Task 5: Correct Release Documentation and Freeze the Deployment Boundary

**Files:**
- Modify: `docs/release-readiness.md`
- Modify: `docs/production-hardening-repair-plan-2026-07-24.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: isolated evidence recorded in `docs/isolated-acceptance-results-2026-07-24.md`
- Produces: a production runbook that names the canonical scripts and stops on unsafe state

- [ ] **Step 1: Correct stale verification status**

In `docs/release-readiness.md`, state all of the following exactly and without broadening the claim:

- MySQL 8.4.8 migration, index, CHECK, AutoMigrate compatibility, concurrency, administrator security, and desktop/mobile browser acceptance passed on a production-data clone.
- `frontend npm run build` passed.
- `frontend npm test` did not complete because it hung during Ant Design module initialization; it is not green evidence.
- Production migration, deployment, administrator rotation, and real production write verification remain undone.

Remove the obsolete blocker saying non-production MySQL verification has not happened.

- [ ] **Step 2: Name the mandatory production gate order**

Update both release documents so the production order is unambiguous:

```text
recoverable backup evidence
-> protected yaner fingerprint
-> 0004 preflight
-> 0004 up migration exactly once
-> 0004 postflight
-> deploy API and admin frontend together
-> health/auth/read checks
-> controlled dedicated test product create/close/complete
-> protected yaner fingerprint comparison
-> 30-60 minute observation
```

Reference these exact repository paths:

```text
backend/migrations/0004_merchant_multi_stock.preflight.sql
backend/migrations/0004_merchant_multi_stock.up.sql
backend/migrations/0004_merchant_multi_stock.postflight.sql
```

- [ ] **Step 3: Record stop conditions**

Document that any of these conditions stops the production window before migration:

- Active orders are nonzero.
- `LOCKED` products are nonzero.
- The old index shape differs from the expected unique `(product_id,is_active)` definition.
- Backup restoration evidence is missing.
- `yaner` is absent/duplicated or its pre-release fingerprint cannot be captured.

For `LOCKED > 0`, the runbook must require reporting affected row IDs and active-order counts for explicit business approval. It must not contain a blanket statement that updates every affected product status.

- [ ] **Step 4: State the production smoke boundary**

Document that production must not run `smoke-mysql-concurrency.mjs`. Production write validation uses only a dedicated test merchant/product with small quantities and performs create -> close and create -> complete. It must not use `yaner` data or rotate an existing administrator password merely for testing.

- [ ] **Step 5: Update the repair-plan status line**

Change the status summary to: local implementation and production-clone isolated acceptance completed; production migration/deployment/account rotation still pending. Keep F-12, F-13, license governance, miniapp ordering, MySQL root rotation, and Vitest investigation outside this release.

---

### Task 6: Verify the Tooling Patch Locally and on an Isolated MySQL 8.4 Clone

**Files:**
- Test: all files changed in Tasks 2-5
- Evidence: remote acceptance evidence directory, with new filenames distinct from the original `51cd7ed` evidence

**Interfaces:**
- Consumes: retained production clone and isolated Compose assets
- Produces: proof that promoted SQL has the same successful migration behavior and that production remains untouched

- [ ] **Step 1: Run local syntax and formatting checks**

Run:

```bash
bash -n deploy/acceptance/prepare.sh
node --check scripts/prepare-ui-acceptance.mjs
node --check scripts/smoke-admin-security.mjs
node --check scripts/smoke-mysql-concurrency.mjs
git diff --check
```

Expected: every command exits 0.

- [ ] **Step 2: Validate Compose resolution without printing secrets**

Run with disposable non-secret values and do not save full resolved output:

```bash
MYSQL_DATABASE=acceptance_config_check \
MYSQL_USER=acceptance_config_check \
MYSQL_PASSWORD=acceptance_config_check \
MYSQL_ROOT_PASSWORD=acceptance_config_check \
JWT_ACCESS_SECRET=acceptance_config_check_access \
JWT_REFRESH_SECRET=acceptance_config_check_refresh \
docker compose -f deploy/acceptance/docker-compose.yml config --quiet
```

Expected: exit 0.

- [ ] **Step 3: Run the backend regression**

Run:

```bash
make backend-test
```

Expected: all Go packages pass.

- [ ] **Step 4: Re-run unaffected build/contract checks proportionately**

Run:

```bash
cd frontend && npm run build
cd ../miniapp && npm test
```

Expected: frontend production build passes; miniapp reports 11 files and 17 tests passing. Do not substitute this for frontend Vitest evidence.

- [ ] **Step 5: Rehearse canonical SQL on a separate remote Compose project**

On the remote acceptance host, first synchronize only the committed/reviewed candidate files into the retained checkout at `/home/yu/services/secondhand-market-acceptance`. Then run the following from its `deploy/acceptance` directory. The distinct project name guarantees a separate MySQL volume, and starting only `mysql` publishes no host port:

```bash
cd /home/yu/services/secondhand-market-acceptance/deploy/acceptance
gate_project=secondhand-gatecheck-20260725

docker ps -a --filter "label=com.docker.compose.project=$gate_project"
docker volume ls --filter "label=com.docker.compose.project=$gate_project"
free -h

docker compose -p "$gate_project" up -d mysql
docker compose -p "$gate_project" ps mysql

docker compose -p "$gate_project" exec -T mysql sh -ec \
  'MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 -u"$MYSQL_USER" "$MYSQL_DATABASE"' \
  < backups/production-clone-20260724.sql

set -o pipefail
docker compose -p "$gate_project" exec -T mysql sh -ec \
  'MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h 127.0.0.1 -u"$MYSQL_USER" "$MYSQL_DATABASE" < /acceptance/migrations/0004_merchant_multi_stock.preflight.sql' \
  | tee evidence/release-gate-preflight-20260725.txt

docker compose -p "$gate_project" exec -T mysql sh -ec \
  'MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 -u"$MYSQL_USER" "$MYSQL_DATABASE" < /acceptance/migrations/0004_merchant_multi_stock.up.sql'

docker compose -p "$gate_project" exec -T mysql sh -ec \
  'MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h 127.0.0.1 -u"$MYSQL_USER" "$MYSQL_DATABASE" < /acceptance/migrations/0004_merchant_multi_stock.postflight.sql' \
  | tee evidence/release-gate-postflight-20260725.txt

docker compose -p "$gate_project" stop mysql
```

Save sanitized stdout as:

```text
evidence/release-gate-preflight-20260725.txt
evidence/release-gate-postflight-20260725.txt
```

The expected preflight and postflight markers must appear, and the old unique index must be absent afterward. Stop the scratch MySQL service when done. Preserve its explicitly named volume until review; do not delete the original acceptance containers, volume, clone, or evidence.

- [ ] **Step 6: Reconfirm production was not touched**

Perform only read-only checks: production domain HTTP status, container restart counts, and health endpoint. Record that no production SQL, deployment, password rotation, or test order occurred.

---

### Task 7: Review, Commit, and Stop Before Production

**Files:**
- Review: the complete release-tooling diff
- Commit: only the explicit files from this plan

**Interfaces:**
- Consumes: verified tooling patch
- Produces: one auditable commit ready for a separately approved production window

- [ ] **Step 1: Request an independent code review**

Use `superpowers:requesting-code-review`. The review must check:

- Canonical pre/post scripts are generic and fail fast.
- Acceptance no longer has duplicate migration gates.
- `yaner` fingerprint cannot silently succeed with zero protected accounts.
- Make target cannot target a public hostname.
- Documentation does not claim frontend Vitest or production deployment passed.
- No inventory/order runtime file changed.

- [ ] **Step 2: Inspect the final scope**

Run:

```bash
git status --short
git diff --stat
git diff -- backend/internal frontend/src miniapp/src backend/migrations/0004_merchant_multi_stock.up.sql backend/migrations/0004_merchant_multi_stock.down.sql
git diff --check
```

Expected: the runtime-code diff command has no output. The three reviewer documents remain untracked and unstaged.

- [ ] **Step 3: Stage only the tooling patch**

Stage explicit paths rather than `git add .`:

```bash
git add Makefile README.md \
  backend/migrations/0004_merchant_multi_stock.preflight.sql \
  backend/migrations/0004_merchant_multi_stock.postflight.sql \
  deploy/acceptance/README.md \
  deploy/acceptance/sql/protected-fingerprint.sql \
  deploy/acceptance/sql/preflight.sql \
  deploy/acceptance/sql/post-migration.sql \
  docs/release-readiness.md \
  docs/production-hardening-repair-plan-2026-07-24.md \
  docs/superpowers/plans/2026-07-25-multi-stock-release-gates.md
```

Deleted paths are intentionally listed so Git stages their removal.

- [ ] **Step 4: Verify the staged patch and commit**

Run:

```bash
git diff --cached --check
git diff --cached --name-status
git commit -m "chore(release): formalize multi-stock migration gates"
```

Expected: one focused commit; no reviewer-authored untracked document is included.

- [ ] **Step 5: Stop and report**

Report the commit hash, local/remote isolated verification, remaining frontend Vitest limitation, retained acceptance resources, and unchanged production state. Do not push or begin production migration until the user explicitly approves the maintenance window.

---

## Separately Approved Production Phase

This phase is intentionally not authorized by implementing Tasks 1-7. After the tooling commit is reviewed, obtain explicit user approval for production writes and then execute the documented order:

1. Confirm the exact release commit and production deployment directory.
2. Capture recoverable database backup evidence and checksum.
3. Capture the protected `yaner` fingerprint and current read-only inventory/order counts.
4. Pause order writes and run the canonical preflight.
5. Stop immediately if active orders, `LOCKED` products, schema drift, or missing protection evidence is reported.
6. Execute the 0004 up migration exactly once, then canonical postflight.
7. Deploy API and admin frontend together with `APP_ENV=production`; do not rely on AutoMigrate to drop the old index.
8. Verify health, authenticated database reads, index shape, and logs.
9. Use only a dedicated production test merchant/product for small create/close and create/complete checks; do not run the concurrency smoke or touch `yaner` products.
10. Compare the protected fingerprint byte-for-byte and observe logs/resources for 30-60 minutes.
11. If a post-migration application issue appears after new multi-stock rows exist, stop writes and forward-fix; do not run the down migration or restore the old unique index.

After this production checkpoint is accepted, create a separate branch and separate plan for the frontend Vitest hang. Do not combine that investigation with the release-gate patch or production maintenance window.
