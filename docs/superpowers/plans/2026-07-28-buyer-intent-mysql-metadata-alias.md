# F-11 MySQL Metadata Alias Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make F-11 startup inspect the final MySQL 8.4 buyer-intent schema correctly by stabilizing information-schema result labels.

**Architecture:** Keep the existing strict validator and migrations unchanged. Add explicit lowercase aliases at both MySQL metadata query boundaries, then use the already failing opt-in MySQL acceptance test as the behavioral RED/GREEN proof before rerunning the complete isolated harness.

**Tech Stack:** Go 1.22, GORM 1.30, go-sql-driver/mysql, MySQL 8.4, Docker Compose, Bash.

## Global Constraints

- Modify only the F-11 MySQL metadata scan in `backend/internal/app/buyer_intent_schema.go` plus traceability/status documents after verified GREEN.
- Do not loosen accepted column layouts, generated-marker semantics, index definitions, or row-state validation.
- Do not modify `0008`, `0009`, models, business handlers, or SQLite behavior.
- The real regression gate is `TestBuyerIntentMySQLAcceptance`; do not replace it with a source-text assertion or mock.
- Remote work remains limited to `/home/yu/services/secondhand-buyer-intent-acceptance-20260727` and Compose project `secondhand-buyer-intent-acceptance`.
- Transfer only a freshly generated committed whitelist with matching local and remote SHA-256 manifests.
- Do not read `.env`, secret contents, databases, uploads, backups, old evidence, `.git`, caches, `node_modules`, `backend/app.db`, `.tmp`, or the protected review documents.
- Do not execute production SQL, read production logs/configuration/mounts/environment, or modify production containers, data, services, migrations, or deployments.
- Production interaction remains limited to the acceptance harness's three fixed container name/ID/state/restart-count snapshots.

---

### Task 1: Stabilize MySQL Metadata Mapping

**Files:**
- Modify: `backend/internal/app/buyer_intent_schema.go`
- Verify: `backend/internal/app/buyer_intent_schema_test.go`
- Verify: `backend/tests/buyer_intent_mysql_test.go`

**Interfaces:**
- Consumes: `mysqlBuyerIntentColumn`, `mysqlBuyerIntentIndexColumn`, and the existing strict validators.
- Produces: populated metadata structs on MySQL 8.4 without changing validator semantics.

- [x] **Step 1: Capture the behavioral RED and root cause**

Run in the exact isolated project with the committed pre-fix source:

```bash
docker compose --project-name secondhand-buyer-intent-acceptance \
  --env-file deploy/acceptance/.env \
  --file deploy/acceptance/docker-compose.yml \
  --profile tools run --rm \
  -e BUYER_INTENT_MYSQL_TEST=1 -e AUTO_MIGRATE=false \
  bootstrap-admin \
  go test ./tests -run '^TestBuyerIntentMySQLAcceptance$' -count=1 -v
```

Observed RED: `NewServer` returns `buyer intent columns are missing or drifted`.
A controlled scan using the same GORM/MySQL boundary returned 20 rows but empty
direct metadata fields; only the explicit lowercase `is_generated` alias was
populated. MySQL CLI inspection showed the actual schema itself was final.

- [ ] **Step 2: Add explicit aliases to the columns query**

Change the query in `inspectMySQLBuyerIntentSchema` to select:

```sql
SELECT
    column_name AS column_name,
    data_type AS data_type,
    column_type AS column_type,
    is_nullable AS is_nullable,
    generation_expression AS generation_expression,
    extra AS extra,
    CASE
        WHEN generation_expression IS NOT NULL AND generation_expression <> '' THEN 'ALWAYS'
        ELSE 'NEVER'
    END AS is_generated
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'buyer_intents'
```

Do not change `verifyMySQLBuyerIntentColumns` or its accepted formal/development
layouts.

- [ ] **Step 3: Add explicit aliases to the statistics query**

Change the query in the same function to select:

```sql
SELECT
    index_name AS index_name,
    non_unique AS non_unique,
    seq_in_index AS seq_in_index,
    column_name AS column_name
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = 'buyer_intents'
ORDER BY index_name, seq_in_index
```

Do not change index grouping, exact key order, lookalike rejection, or error
messages.

- [ ] **Step 4: Run local focused GREEN and regressions**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./internal/app -run 'BuyerIntent.*(Schema|Column|Index|Uniqueness)|NewServer' -count=1 -v
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./internal/app ./tests ./migrations -count=1
```

Expected: all local tests pass; the opt-in MySQL test still skips without its
explicit isolated flag and DSN.

- [ ] **Step 5: Format, inspect, and commit the focused source fix**

```bash
gofmt -w backend/internal/app/buyer_intent_schema.go
git diff --check
git add backend/internal/app/buyer_intent_schema.go
git commit -m "fix(buyer): stabilize MySQL metadata aliases"
```

---

### Task 2: Prove The Fix In The Isolated MySQL 8.4 Gate

**Files:**
- Generate remotely: dedicated `.env`, secrets, backups, and sanitized evidence under the exact F-11 directory
- Create after success: `docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md`
- Modify after success: `docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md`
- Modify after success: `docs/superpowers/specs/2026-07-28-buyer-intent-mysql-metadata-alias-design.md`
- Modify after success: `docs/release-readiness.md`
- Modify after success: `docs/full-project-code-review-2026-07-24.md`

**Interfaces:**
- Consumes: the committed alias fix and the F-11 source-list modes.
- Produces: accepted MySQL 8.4 RED/GREEN evidence and the prerequisite proof for F-12.

- [ ] **Step 1: Run fresh local full quality gates**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test ./... -count=1
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test -race ./... -count=1
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go vet ./...
```

- [ ] **Step 2: Regenerate and verify the committed whitelist**

```bash
BUYER_INTENT_SOURCE_LIST_ONLY=1 \
  ./deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh > /tmp/f11-source-files.z
BUYER_INTENT_SOURCE_MANIFEST_ONLY=1 \
  ./deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh > /tmp/f11-source-sha256.txt
```

Require every NUL-delimited path to be a committed regular non-symlink file,
zero forbidden paths, and exact local/remote list and manifest hashes.

- [ ] **Step 3: Rebuild only the exact failed test project**

Verify by full ID/name/project label that the retained resources belong only to
`secondhand-buyer-intent-acceptance`. Remove only those verified project
containers, volume, network, and the literal F-11 target directory. Recreate the
directory with mode `0700`, transfer the verified whitelist, compare manifests,
and run `deploy/acceptance/prepare.sh` without reading generated values.

- [ ] **Step 4: Run the complete acceptance command once**

```bash
COMPOSE_PROJECT_NAME=secondhand-buyer-intent-acceptance \
BUYER_INTENT_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-buyer-intent-smoke
```

Expected: migration `0008` and `0009` matrices, API schema, both AutoMigrate
modes, full Go tests, race, and vet all pass; the harness exits zero.

- [ ] **Step 5: Gate evidence and record status**

Require `forbidden_matches=0`, verify `evidence-sha256.txt`, and compare the
before/after production snapshots byte-for-byte before reading the remaining
new sanitized evidence. Record every failed attempt and the accepted attempt,
source commit/manifests, MySQL version, test summaries, retained resources, and
the fact that production remained unchanged.

Set statuses to:

```text
F-11 code-side fixed; isolated MySQL 8.4 test-server review passed;
production migration 0009 and deployment not performed.
F-12 prerequisite satisfied; F-12 implementation not yet complete.
```

- [ ] **Step 6: Commit acceptance evidence and status**

```bash
git add \
  docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md \
  docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md \
  docs/superpowers/specs/2026-07-28-buyer-intent-mysql-metadata-alias-design.md \
  docs/superpowers/plans/2026-07-28-buyer-intent-mysql-metadata-alias.md \
  docs/release-readiness.md docs/full-project-code-review-2026-07-24.md
git commit -m "docs(buyer): record F-11 MySQL acceptance"
```

## Plan Self-Review

- The real MySQL/GORM boundary, not source text or a mock, is the regression gate.
- Every metadata field used by the two destination structs receives an explicit alias.
- No migration, validator rule, model, or business handler changes.
- Remote mutation remains restricted to the exact F-11 project.
- Success status requires both local quality gates and audited remote evidence.
